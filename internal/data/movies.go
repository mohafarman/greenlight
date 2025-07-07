package data

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
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

type MovieModel struct {
	DB *sql.DB
}

func (m *MovieModel) Insert(movie *Movie) error {
	query := `
		INSERT INTO movies (title, year, runtime, genres)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, version
		`

	args := []any{movie.Title, movie.Year, movie.Runtime, pq.Array(movie.Genres)}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.DB.QueryRowContext(ctx, query, args...).Scan(&movie.ID, &movie.CreatedAt, &movie.Version)
}

func (m *MovieModel) Get(id int64) (*Movie, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	query := `
		SELECT id, created_at, title, year, runtime, genres, version
		FROM movies
		WHERE id = $1;`

	var movie Movie

	/* Context w/ 3-second timeout */
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&[]byte{},
		&movie.ID,
		&movie.CreatedAt,
		&movie.Title,
		&movie.Year,
		&movie.Runtime,
		pq.Array(&movie.Genres),
		&movie.Version)

	/* Scan may return sql.ErrNoRows */
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &movie, nil
}

/* Filter parameters as arguments */
func (m *MovieModel) GetAll(title string, genres []string, f Filters) ([]*Movie, error) {
	query := `
		SELECT id, created_at, title, year, runtime, genres, version
		FROM movies
		WHERE (LOWER(title) = LOWER($1) OR $1 = '')
		AND (genres @> $2 OR $2 = '{}')
		ORDER BY id`

	/* Context w/ 3-second timeout */
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := m.DB.QueryContext(ctx, query, title, pq.Array(genres))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	movies := []*Movie{}

	for rows.Next() {
		var movie Movie

		err := rows.Scan(
			&movie.ID,
			&movie.CreatedAt,
			&movie.Title,
			&movie.Year,
			&movie.Runtime,
			pq.Array(&movie.Genres),
			&movie.Version,
		)

		if err != nil {
			return nil, err
		}

		movies = append(movies, &movie)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return movies, nil
}

func (m *MovieModel) Update(movie *Movie) error {
	query := `
		UPDATE movies
		SET title = $1, year = $2, runtime = $3, genres = $4, version = version + 1
		WHERE id = $5 AND version = $6
		RETURNING version
		`

	args := []any{
		movie.Title,
		movie.Year,
		movie.Runtime,
		pq.Array(movie.Genres),
		movie.ID,
		movie.Version}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	/* If no matching row could be found either the row does not exist
	   or the version has changed. I.e. optimistic locking based on version
	   to prevent data race conditions */
	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&movie.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}

	return nil
}

func (m *MovieModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	query := `
		DELETE FROM movies
		WHERE id = $1`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := m.DB.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	/* INFO: If no rows are affected that means nothing was deleted */
	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}

func ValidateMovie(v *validator.Validator, movie *Movie) {
	v.CheckField(validator.NotBlank(movie.Title), "title", "must be provided")
	v.CheckField(validator.MaxChars(movie.Title, 100), "title", "must not be longer than 100 characters")

	v.CheckField(validator.NotEmpty(movie.Year), "year", "must be provided")
	v.CheckField(movie.Year >= 1888, "year", "must be greatar than 1888")
	v.CheckField(validator.NotFuture(movie.Year), "year", "must not be in the future")

	v.CheckField(validator.NotEmpty(movie.Runtime), "runtime", "must be provided")
	v.CheckField(validator.NotNegative(movie.Runtime), "runtime", "must be a positive integer")

	v.CheckField(movie.Genres != nil, "genres", "must be provided")
	v.CheckField(len(movie.Genres) >= 1, "genres", "must contain at least 1 genre")
	v.CheckField(len(movie.Genres) <= 5, "genres", "must contain at max 5 genres")
	v.CheckField(validator.Unique(movie.Genres), "genres", "must not contain duplicate values")
}
