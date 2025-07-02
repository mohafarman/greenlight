package validator

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type Integer interface {
	~int | ~int32
}

var EmailRX = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

type Validator struct {
	Errors map[string]string
}

func New() *Validator {
	return &Validator{
		Errors: make(map[string]string),
	}
}

func (v *Validator) Valid() bool {
	return len(v.Errors) == 0
}

func (v *Validator) AddError(key, message string) {
	if _, exists := v.Errors[key]; !exists {
		v.Errors[key] = message
	}
}

func (v *Validator) CheckField(ok bool, key, message string) {
	if !ok {
		v.AddError(key, message)
	}
}

func PermittedValue[T comparable](value T, permittedValues ...T) bool {
	for i := range permittedValues {
		if value == permittedValues[i] {
			return true
		}
	}

	return false
}

/* TODO: NotBlank & NotEmpty can be generic? */
func NotBlank(value string) bool {
	/* Return true if value is not empty string */
	return strings.TrimSpace(value) != ""
}

func NotEmpty[T Integer](value T) bool {
	/* Return true if value is not empty */
	return value != 0
}

func NotNegative[T Integer](value T) bool {
	return value > 0
}

func MaxChars(value string, n int) bool {
	/* Return true if value contains no more than n characters */
	return utf8.RuneCountInString(value) <= n
}

func NotFuture(year int32) bool {
	return year <= int32(time.Now().Year())
}

func Matches(value string, rx *regexp.Regexp) bool {
	return rx.MatchString(value)
}

/* Returns true if all values in a generic slice are unique */
func Unique[T comparable](values []T) bool {
	uniqueValues := make(map[T]bool)

	for _, v := range values {
		uniqueValues[v] = true
	}

	return len(values) == len(uniqueValues)
}
