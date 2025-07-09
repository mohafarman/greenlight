package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func (app *application) serve() error {
	server := http.Server{
		Addr:         fmt.Sprintf(":%d", app.config.port),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		Handler:      app.routes(),
		/* Tell http.Server to communicate logs through our logger which implements io.Writer interface */
		// ErrorLog: log.New(logger, "", 0),
	}

	// Channel to receive any errors returned by graceful Shutdown()
	shutdownError := make(chan error)

	go func() {
		// Create a quit channel which carries os.Signal values.
		quit := make(chan os.Signal, 1)

		// Listen for incoming signals which will be relaye to the quit channel.
		// Other signals retain default behaviour.
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

		// Read signal from the quite channel.
		// This code will block until a signal is received.
		s := <-quit

		app.logger.Info("shutting down server", map[string]string{
			"signal": s.String(),
		})

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// Shutdown() returns nil if successful, otherwise if after timeout it will return
		// an error
		shutdownError <- server.Shutdown(ctx)
	}()

	app.logger.Info("Starting server", map[string]string{
		"addr": server.Addr,
		"env":  app.config.env,
	})

	// Calling Shutdown() will cause server.ListenAndServe() to return http.ErrServerClosed,
	// if it does then continue execution to handle graceful shutdown otherwise simply return error
	err := server.ListenAndServe()
	if err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	// Wait to receive return value from Shutdown() on the shutdownError channel
	// If there's an error it means there was a problem with our graceful shutdown
	err = <-shutdownError
	if err != nil {
		return err
	}

	app.logger.Info("stopped server", map[string]string{
		"addr": server.Addr,
	})

	return nil
}
