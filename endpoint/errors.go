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

// ErrInvalidLimit is returned when a limit argument is present but invalid:
// not a positive 32-bit integer literal, duplicated, or exceeding the maximum.
var ErrInvalidLimit = errors.New("invalid limit")

// ErrMissingOrder is returned when a root field is missing a required order argument.
var ErrMissingOrder = errors.New("order not specified")

// ErrInvalidOrder is returned when an order argument is present but invalid:
// not a DefraDB-compatible object/list of single-field {field: ASC|DESC}
// conditions, duplicated, or carrying an unknown direction.
var ErrInvalidOrder = errors.New("invalid order")

// ErrUnsupportedSelection is returned when a root selection is not a plain field
// (a fragment spread or inline fragment).
var ErrUnsupportedSelection = errors.New("unsupported root selection")

// ErrMissingPool is returned when a query carries no signed pool address. Every
// query must name the pool it bills to.
var ErrMissingPool = errors.New("query is missing a signed pool address")

// ErrUnknownPool is returned when the signed pool address is not known to the gateway.
var ErrUnknownPool = errors.New("unknown pool")

// ErrPoolInactive is returned when the signed pool exists but is not active.
var ErrPoolInactive = errors.New("pool is not active")

// ErrPoolViewMismatch is returned when the query does not target the view the
// signed pool serves.
var ErrPoolViewMismatch = errors.New("query does not match the signed pool's view")

// ErrUnexpectedStatus is returned when an upstream host responds with a non-2xx status.
var ErrUnexpectedStatus = errors.New("unexpected HTTP status")

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
