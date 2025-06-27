package main

import (
	"fmt"
	"net/http"
)

func (app *application) showMovieHandler(w http.ResponseWriter, r *http.Request) {

	id, err := app.readIDParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	fmt.Fprintf(w, "OK: Show detail of movie with ID = %d\n", id)
}

func (app *application) createMovieHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Create a new movie..."))
}
