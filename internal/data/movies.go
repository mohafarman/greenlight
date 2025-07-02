package data

import (
	"time"

	"github.com/mohafarman/greenlight/internal/validator"
)

type Movie struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"-"`
	Title     string    `json:"title"`
	Year      int32     `json:"year,omitempty"`
	Runtime   Runtime   `json:"runtime,omitempty"`
	Genres    []string  `json:"genres,omitempty"`
	Version   int32     `json:"version"`
}

func ValidateMovie(v *validator.Validator, movie *Movie) {
	v.CheckField(validator.NotBlank(movie.Title), "title", "must be provided")
	v.CheckField(validator.MaxChars(movie.Title, 100), "title", "must not be longer than 100 characters")

	v.CheckField(validator.NotEmpty(movie.Year), "year", "must be provided")
	v.CheckField(movie.Year > 1888, "year", "must be greatar than 1888")
	v.CheckField(validator.NotFuture(movie.Year), "year", "must not be in the future")

	v.CheckField(validator.NotEmpty(movie.Runtime), "runtime", "must be provided")
	v.CheckField(validator.NotNegative(movie.Runtime), "runtime", "must be a positive integer")

	v.CheckField(movie.Genres != nil, "genres", "must be provided")
	v.CheckField(len(movie.Genres) < 1, "genres", "must contain at least 1 genre")
	v.CheckField(len(movie.Genres) <= 5, "genres", "must contain at max 5 genres")
	v.CheckField(validator.Unique(movie.Genres), "genres", "must not contain duplicate values")
}
