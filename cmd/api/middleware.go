package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mohafarman/greenlight/internal/data"
	"github.com/mohafarman/greenlight/internal/validator"
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
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				app.serverErrorResponse(w, r, err)
				return
			}

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

		app.logger.Info(fmt.Sprintf("token: %s", headerParts[1]), nil)

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
