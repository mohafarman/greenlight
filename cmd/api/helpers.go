package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
)

type envelope map[string]any

func (app *application) readIDParam(r *http.Request) (int64, error) {
	params := httprouter.ParamsFromContext(r.Context())

	// INFO: Convert string to int using a base 10 and bite size 64,
	// if param can not be converted or id is < 0 we know it is invalid so
	// return error
	id, err := strconv.ParseInt(params.ByName("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id parameter")
	}

	return id, nil
}

func (app *application) writeJSON(w http.ResponseWriter, status int, data envelope, headers http.Header) error {
	// INFO: Prints out with whitespace for a prettier print to terminals
	// NB! Slower performance compared to json.Marshal
	js, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}

	// INFO: For nice print to terminals
	js = append(js, '\n')

	for k, v := range headers {
		w.Header()[k] = v
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)

	return nil
}
