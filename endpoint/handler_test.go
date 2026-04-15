package endpoint

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetContentType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		request  *http.Request
		expected string
	}{
		{
			name:     "empty header",
			request:  &http.Request{},
			expected: contentTypeGraphQLResponse,
		},
		{
			name:     "accept any",
			request:  &http.Request{Header: map[string][]string{"Accept": {"*/*"}}},
			expected: contentTypeGraphQLResponse,
		},
		{
			name:     "accept GraphQL response json",
			request:  &http.Request{Header: map[string][]string{"Accept": {contentTypeGraphQLResponse}}},
			expected: contentTypeGraphQLResponse,
		},
		{
			name:     "accept GraphQL response json with encoding",
			request:  &http.Request{Header: map[string][]string{"Accept": {contentTypeGraphQLResponse + "; charset=utf-8"}}},
			expected: contentTypeGraphQLResponse,
		},
		{
			name:     "accept application/json",
			request:  &http.Request{Header: map[string][]string{"Accept": {contentTypeJSON}}},
			expected: contentTypeJSON,
		},
		{
			name:     "accept text/html",
			request:  &http.Request{Header: map[string][]string{"Accept": {"text/html"}}},
			expected: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := getContentType(c.request)
			require.Equal(t, c.expected, result)
		})
	}
}
