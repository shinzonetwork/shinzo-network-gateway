package endpoint

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLimitValidator(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		limit int
		query string
		err   error
	}{
		{
			name:  "valid limit",
			limit: 100,
			query: `{ users(limit: 10) { id } }`,
		},
		{
			name:  "limit equal to max",
			limit: 100,
			query: `{ users(limit: 100) { id } }`,
		},
		{
			name:  "limit over range",
			limit: 100,
			query: `{ users(limit: 101) { id } }`,
			err:   ErrLimitTooLarge,
		},
		{
			name:  "negative limit",
			limit: 100,
			query: `{ users(limit: -5) { id } }`,
			err:   ErrInvalidLimit,
		},
		{
			name:  "limit equal to zero",
			limit: 100,
			query: `{ users(limit: 0) { id } }`,
			err:   ErrInvalidLimit,
		},
		{
			name:  "limit as string",
			limit: 100,
			query: `{ users(limit: "10") { id } }`,
			err:   ErrInvalidLimit,
		},
		{
			name:  "limit as float",
			limit: 100,
			query: `{ users(limit: 1.5) { id } }`,
			err:   ErrInvalidLimit,
		},
		{
			name:  "limit as variable",
			limit: 100,
			query: `query ($n: Int) { users(limit: $n) { id } }`,
			err:   ErrInvalidLimit,
		},
		{
			name:  "missing limit",
			limit: 100,
			query: `{ users { id } }`,
			err:   ErrMissingLimit,
		},
		{
			name:  "multiple root fields all valid",
			limit: 100,
			query: `{ users(limit: 10) { id } posts(limit: 20) { id } }`,
		},
		{
			name:  "multiple root fields one missing limit",
			limit: 100,
			query: `{ users(limit: 10) { id } posts { id } }`,
			err:   ErrMissingLimit,
		},
		{
			name:  "nested field without limit is allowed",
			limit: 100,
			query: `{ users(limit: 10) { id posts { id } } }`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query, err := parseQuery(tc.query)
			require.NoError(t, err)

			validator := &LimitValidator{limit: tc.limit}

			err = validator.Validate(&ValidationRequest{Query: query})
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
