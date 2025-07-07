package data

import "github.com/mohafarman/greenlight/internal/validator"

type Filters struct {
	Page         int
	PageSize     int
	Sort         string
	SortSafelist []string
}

func ValidateFilters(v *validator.Validator, f Filters) {
	v.CheckField(f.Page > 0, "page", "must be greater than zero")
	v.CheckField(f.Page <= 10_000_000, "page", "must be less than 10 million")
	v.CheckField(f.PageSize > 0, "page_size", "must be greater than zero")
	v.CheckField(f.PageSize < 100, "page_size", "must be less than 100")

	v.CheckField(validator.PermittedValue(f.Sort, f.SortSafelist...), "sort", "invalid sort value")
}
