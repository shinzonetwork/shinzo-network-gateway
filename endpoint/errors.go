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

// ErrHostHTTP is returned when an upstream host responds with a non-2xx status.
var ErrHostHTTP = errors.New("host HTTP error")

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
