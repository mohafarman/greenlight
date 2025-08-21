package main

import (
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mohafarman/greenlight/internal/data"
	"github.com/mohafarman/greenlight/internal/validator"
	"github.com/tomasen/realip"
	"golang.org/x/time/rate"
)

func (app *application) rateLimiter(next http.Handler) http.Handler {
	// Any code here will run only once, when we wrap something with the middleware.
	// Allow 2 requests per second, with a maximum of 4 requests in a burst.
	type client struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}

	var (
		mu      sync.Mutex
		clients = make(map[string]*client)
	)

	// Launch a background goroutine which removes old entries from the clients map once
	// every minute.
	go func() {
		for {
			time.Sleep(time.Minute)

			/* Lock to prevent any rate limiter checks while cleaning up */
			mu.Lock()

			/* Delete client if they haven't been since within the last 3 minutes */
			for ip, client := range clients {
				if time.Since(client.lastSeen) > 3*time.Minute {
					delete(clients, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Any code here will run for every request that the middleware handles.

		if app.config.limiter.enabled {
			// ip, _, err := net.SplitHostPort(r.RemoteAddr)
			// if err != nil {
			// 	app.serverErrorResponse(w, r, err)
			// 	return
			// }

			ip := realip.FromRequest(r)

			// INFO: Because each request spins up its own goroutine we need to lock
			// before writing to the map
			mu.Lock()

			// If no rate limiter (client) exists for the current user, create one
			if _, found := clients[ip]; !found {
				clients[ip] = &client{
					limiter: rate.NewLimiter(rate.Limit(app.config.limiter.rps), app.config.limiter.burst),
				}
			}
			clients[ip].lastSeen = time.Now()

			if !clients[ip].limiter.Allow() {
				mu.Unlock()
				app.rateLimitExceededResponse(w, r)
				return
			}
			// DON'T use defer to unlock the mutex, as that would mean
			// that the mutex isn't unlocked until all the handlers downstream of this
			// middleware have also returned.
			mu.Unlock()
		}
		next.ServeHTTP(w, r)
	})
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		/* INFO: A deferred function will always be run in the event
		   of a panic as Go unwinds the stack.
		Only works in the same goroutine that executed the recoverPanic() middleware */
		defer func() {
			if err := recover(); err != nil {
				/* INFO: Tells the client that the connection is closed.
				   Works with HTTP/2 as well. */
				w.Header().Set("Connection", "close")
				app.serverErrorResponse(w, r, fmt.Errorf("%s", err))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (app *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		/* Tells the caches that this kv pair may vary */
		w.Header().Add("Vary", "Authorization")

		/* empty == "" */
		authorizationHeader := r.Header.Get("Authorization")

		/* Set anonymous user if header is empty */
		if authorizationHeader == "" {
			r = app.contextSetUser(r, data.AnonymousUser)
			next.ServeHTTP(w, r)
			return
		}

		/* Expected format: "Bearer <token>" */
		headerParts := strings.Split(authorizationHeader, " ")
		if len(headerParts) != 2 && headerParts[0] != "Bearer" {
			app.invalidAuthenticationTokenResponse(w, r)
			return
		}

		token := headerParts[1]

		v := validator.New()

		if data.ValidateTokenPlaintext(v, token); !v.Valid() {
			app.invalidAuthenticationTokenResponse(w, r)
			return
		}

		user, err := app.models.Users.GetForToken(data.ScopeAuthentication, token)
		if err != nil {
			switch {
			case errors.Is(err, data.ErrRecordNotFound):
				app.invalidAuthenticationTokenResponse(w, r)
			default:
				app.serverErrorResponse(w, r, err)
			}
			return
		}

		r = app.contextSetUser(r, user)

		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAuthenticatedUser(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := app.contextGetUser(r)

		/* If user is anonymous */
		if user.IsAnonymous() {
			app.authenticationRequiredResponse(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

/* Checks that a user is both authenticated and activated */
func (app *application) requireActivatedUser(next http.HandlerFunc) http.HandlerFunc {
	/* store the function in fn, don't return */
	fn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := app.contextGetUser(r)

		/* If user is not activated */
		if !user.Activated {
			app.inactiveAccountResponse(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})

	/* wrap fn with requireAuthenticatedUser middleware before returning */
	/* requireAuthenticatedUser() will be executed first before this middleware executes itself */
	return app.requireAuthenticatedUser(fn)
}

func (app *application) requirePermission(code string, next http.HandlerFunc) http.HandlerFunc {
	fn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := app.contextGetUser(r)

		/* Get slices of permissions */
		permissions, err := app.models.Permissions.GetAllForUser(int64(user.ID))
		if err != nil {
			app.serverErrorResponse(w, r, err)
			return
		}

		/* Check if slice includes (contains) required permissions, otherwise */
		/* return a 403 forbidden response */
		if !permissions.Include(code) {
			app.notPermittedResponse(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})

	/* requireActivatedUser() will be executed first before this middleware executes itself */
	/* thus when we call requirePermission() we will be carrying out three checks: */
	/* authenticed (non-anonymous) -> activated user -> specific permission */
	return app.requireActivatedUser(fn)
}

func (app *application) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		/* must be added if what we return depends on a header */
		/* otherwise might be cause of subtle bugs */
		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Access-Control-Request-Method")

		origin := r.Header.Get("Origin")

		/* only run if there's an Origin request header */
		if origin != "" {
			/* checks to see if the trusted origins contains origin, only then */
			/* allow CORS */
			if slices.Contains(app.config.cors.trustedOrigins, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)

				/* Check if it's a preflight CORS request */
				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					/* Set necessary preflight response headers */
					w.Header().Set("Access-Control-Allow-Method", "OPTIONS, PUT, PATCH, DELETE")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

					/* Write the headers with a 200 OK status */
					/* Instead of 204 No Content because we actualy don't have a body */
					/* because som browsers don't support the 204 No Conent and may still block */
					w.WriteHeader(http.StatusOK)
					return
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

/* Embeds http.ResponseWriter so we can record the http status codes */
/* that are sent to the client. We record the status code and a bool to indicate */
/* wether we have stored it or not */
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func (mw *metricsResponseWriter) WriteHeader(statusCode int) {
	/* Pass through WriteHeader first before recording the status code */
	/* because WriteHeader might panic if incorrect status code is sent */
	mw.ResponseWriter.WriteHeader(statusCode)

	if !mw.headerWritten {
		mw.statusCode = statusCode
		mw.headerWritten = true
	}
}

func (mw *metricsResponseWriter) Write(b []byte) (int, error) {
	if !mw.headerWritten {
		mw.statusCode = http.StatusOK
		mw.headerWritten = true
	}

	return mw.ResponseWriter.Write(b)
}

func (mw *metricsResponseWriter) Unwrap() http.ResponseWriter {
	return mw.ResponseWriter
}

func (app *application) metrics(next http.Handler) http.Handler {
	var (
		totalRequestsReceived           = expvar.NewInt("total_requests_received")
		totalResponsesSent              = expvar.NewInt("total_responses_sent")
		totalProcessingTimeMicroseconds = expvar.NewInt("total_processing_µs")
		totalResponsesSentByStatus      = expvar.NewMap("total_responses_sent_by_status")
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		totalRequestsReceived.Add(1)

		mw := &metricsResponseWriter{ResponseWriter: w}

		next.ServeHTTP(mw, r)

		// On the way back up the middleware chain, increment the number of responses
		// sent by 1.
		totalResponsesSent.Add(1)

		// Now the status code should be stored in mw.statusCode. expvar map is string-keyed
		// so we need to change the code into a string
		totalResponsesSentByStatus.Add(strconv.Itoa(mw.statusCode), 1)

		dur := time.Since(start).Milliseconds()
		totalProcessingTimeMicroseconds.Add(dur)
	})
}
