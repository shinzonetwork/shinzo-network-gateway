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
			err:   ErrInvalidLimit,
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
			name:  "duplicate limit argument",
			limit: 100,
			query: `{ users(limit: 1, limit: 9999999) { id } }`,
			err:   ErrInvalidLimit,
		},
		{
			name:  "limit exceeds int32 range",
			limit: 100,
			query: `{ users(limit: 3000000000) { id } }`,
			err:   ErrInvalidLimit,
		},
		{
			name:  "limit below int32 range",
			limit: 100,
			query: `{ users(limit: -3000000000) { id } }`,
			err:   ErrInvalidLimit,
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
		{
			name:  "root inline fragment is rejected",
			limit: 100,
			query: `{ ... on Query { users(limit: 10) { id } } }`,
			err:   ErrUnsupportedSelection,
		},
		{
			name:  "root fragment spread is rejected",
			limit: 100,
			query: `{ ...Roots } fragment Roots on Query { users(limit: 10) { id } }`,
			err:   ErrUnsupportedSelection,
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

func TestOrderValidator(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
		err   error
	}{
		{
			name:  "valid single field ascending",
			query: `{ users(order: {name: ASC}) { id } }`,
		},
		{
			name:  "valid single field descending",
			query: `{ users(order: {created: DESC}) { id } }`,
		},
		{
			name:  "valid multi-field list",
			query: `{ users(order: [{name: ASC}, {age: DESC}]) { id } }`,
		},
		{
			name:  "valid nested relation order",
			query: `{ users(order: {author: {birthday: DESC}}) { id } }`,
		},
		{
			name:  "valid nested relation inside list",
			query: `{ users(order: [{name: ASC}, {author: {birthday: DESC}}]) { id } }`,
		},
		{
			name:  "multiple root fields all valid",
			query: `{ users(order: {name: ASC}) { id } posts(order: {title: DESC}) { id } }`,
		},
		{
			name:  "nested field without order is allowed",
			query: `{ users(order: {name: ASC}) { id posts { id } } }`,
		},
		{
			name:  "missing order",
			query: `{ users { id } }`,
			err:   ErrMissingOrder,
		},
		{
			name:  "multiple root fields one missing order",
			query: `{ users(order: {name: ASC}) { id } posts { id } }`,
			err:   ErrMissingOrder,
		},
		{
			name:  "duplicate order argument",
			query: `{ users(order: {name: ASC}, order: {age: DESC}) { id } }`,
			err:   ErrInvalidOrder,
		},
		{
			name:  "object with multiple fields is rejected",
			query: `{ users(order: {name: ASC, age: DESC}) { id } }`,
			err:   ErrInvalidOrder,
		},
		{
			name:  "empty order object",
			query: `{ users(order: {}) { id } }`,
			err:   ErrInvalidOrder,
		},
		{
			name:  "empty order list",
			query: `{ users(order: []) { id } }`,
			err:   ErrInvalidOrder,
		},
		{
			name:  "unknown direction",
			query: `{ users(order: {name: ASCENDING}) { id } }`,
			err:   ErrInvalidOrder,
		},
		{
			name:  "direction as string",
			query: `{ users(order: {name: "ASC"}) { id } }`,
			err:   ErrInvalidOrder,
		},
		{
			name:  "order as scalar",
			query: `{ users(order: 5) { id } }`,
			err:   ErrInvalidOrder,
		},
		{
			name:  "order as variable",
			query: `query ($o: ordering) { users(order: $o) { id } }`,
			err:   ErrInvalidOrder,
		},
		{
			name:  "list element is not an object",
			query: `{ users(order: [ASC]) { id } }`,
			err:   ErrInvalidOrder,
		},
		{
			name:  "nested relation unknown direction",
			query: `{ users(order: {author: {birthday: SIDEWAYS}}) { id } }`,
			err:   ErrInvalidOrder,
		},
		{
			name:  "nested relation with multiple fields is rejected",
			query: `{ users(order: {author: {name: ASC, birthday: DESC}}) { id } }`,
			err:   ErrInvalidOrder,
		},
		{
			name:  "root inline fragment is rejected",
			query: `{ ... on Query { users(order: {name: ASC}) { id } } }`,
			err:   ErrUnsupportedSelection,
		},
		{
			name:  "root fragment spread is rejected",
			query: `{ ...Roots } fragment Roots on Query { users(order: {name: ASC}) { id } }`,
			err:   ErrUnsupportedSelection,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query, err := parseQuery(tc.query)
			require.NoError(t, err)

			validator := &OrderValidator{}

			err = validator.Validate(&ValidationRequest{Query: query})
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
