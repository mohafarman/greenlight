package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

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

func (app *application) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	maxBytes := 1_048_576 // 1 MB
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))

	dec := json.NewDecoder(r.Body)
	// INFO: If client includes fields that is unknown to our decoder then an error
	// will be raised instead of ignoring the field
	dec.DisallowUnknownFields()

	err := dec.Decode(&dst)
	if err != nil {
		/* Handle errors and turn it into plain-english error messages */
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError
		var maxBytesError *http.MaxBytesError

		switch {
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)

			// Returned by Decode()
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")

		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}
			return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)

			// Returned by Decode()
		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")

			// Decode() returns "json: unknown field "<name>""
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("body contains unknown key %s", fieldName)

		case errors.As(err, &maxBytesError):
			return fmt.Errorf("body must not be larger than %d bytes", maxBytesError.Limit)

			// If a non-nil pointer is passed to Decode(),
			// a server error rather than a client error
		case errors.As(err, &invalidUnmarshalError):
			panic(err)

			// Returns error message as is
		default:
			return err
		}
	}

	// INFO: Decode only reads one JSON "value" (or "body") at a time.
	// If a second one is read, i.e. err is not EOF, then throw error
	err = dec.Decode(&struct{}{})
	if err != io.EOF {
		return errors.New("body must only contain a single JSON value")
	}

	return nil
}
