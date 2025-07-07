package main

import (
	"fmt"
	"net/http"
)

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
