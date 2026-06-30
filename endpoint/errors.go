package endpoint

import (
	"errors"
	"net/http"

	json "github.com/goccy/go-json"
)

const (
	// contentTypeGraphQLResponse is the preferred media type per the GraphQL over HTTP spec.
	contentTypeGraphQLResponse = "application/graphql-response+json"
	// contentTypeJSON is the legacy media type for backwards compatibility.
	contentTypeJSON = "application/json"
)

// gqlError is a single GraphQL error object per the GraphQL spec.
type gqlError struct {
	Message    string          `json:"message"`
	Locations  []gqlLocation   `json:"locations,omitempty"`
	Path       []any           `json:"path,omitempty"`
	Extensions json.RawMessage `json:"extensions,omitempty"`
}

// gqlLocation identifies a position in the GraphQL document associated with an error.
type gqlLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// ErrEmptyQuery is returned if GraphQL query is empty.
var ErrEmptyQuery = errors.New("empty GraphQL query")

// ErrParse is returned if GraphQL query cannot be parsed.
var ErrParse = errors.New("GraphQL parse error")

// ErrValidation is returned when a GraphQL query fails validation.
var ErrValidation = errors.New("validation failed")

// ErrMissingLimit is returned when a root field is missing a required limit argument.
var ErrMissingLimit = errors.New("limit not specified")

// ErrInvalidLimit is returned when a limit argument is present but its value
// cannot be statically verified (non-integer literal or a variable) or is not positive.
var ErrInvalidLimit = errors.New("invalid limit")

// ErrLimitTooLarge is returned when a limit value exceeds the configured maximum.
var ErrLimitTooLarge = errors.New("limit exceeds maximum")

// ErrHostHTTP is returned when an upstream host responds with a non-2xx status.
var ErrHostHTTP = errors.New("host HTTP error")

// ErrResponseTooLarge is returned when an upstream host response exceeds the size limit.
var ErrResponseTooLarge = errors.New("response too large")

// requestErrorStatus returns the HTTP status for a GraphQL request error.
// As defined in GraphQL-over-HTTP spec:
//   - application/json responses to well-formed requests SHOULD use 200.
//   - application/graphql-response+json uses 400 for request errors.
func requestErrorStatus(contentType string) int {
	if contentType == contentTypeJSON {
		return http.StatusOK
	}
	return http.StatusBadRequest
}
