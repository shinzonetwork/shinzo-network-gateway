package endpoint

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultCollectionsExtractor(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		expected []string
		err      bool
	}{
		{
			name: "simple",
			query: `{
				hero { name }
			}`,
			expected: []string{"hero"},
		},
		{
			name: "named query",
			query: `query GetHero {
				hero { name }
			}`,
			expected: []string{"hero"},
		},
		{
			name: "multiple root collections",
			query: `{
				hero { name }
				villain { name }
			}`,
			expected: []string{"hero", "villain"},
		},
		{
			name: "multiple root collections named query",
			query: `query GetBoth {
				hero { name }
				villain { name }
			}`,
			expected: []string{"hero", "villain"},
		},
		{
			name: "nested fields do not appear as root collections",
			query: `{
				hero {
					name
					friends { name }
				}
			}`,
			expected: []string{"hero"},
		},
		{
			name: "root collection with arguments",
			query: `{
				hero(id: "1") { name }
			}`,
			expected: []string{"hero"},
		},
		{
			name: "root collection with alias",
			query: `{
				h: hero { name }
			}`,
			expected: []string{"hero"},
		},
		{
			name: "multiple operations",
			query: `query A {
				hero { name }
			}
			query B {
				villain { name }
			}`,
			expected: []string{"hero", "villain"},
		},
		{
			name: "empty selection set",
			query: `query Empty {
			}`,
			err: true,
		},
		{
			name: "invalid graphql - syntax error",
			query: `{
				hero { name }`,
			err: true,
		},
		{
			name:  "empty string",
			query: ``,
			err:   true,
		},
		{
			name:  "invalid graphql - garbage input",
			query: `not graphql at all!!!`,
			err:   true,
		},
		{
			name:  "invalid graphql - unclosed brace",
			query: `{`,
			err:   true,
		},
		{
			name: "deeply nested query - only root returned",
			query: `{
				a {
					b {
						c {
							d { e }
						}
					}
				}
			}`,
			expected: []string{"a"},
		},
		{
			name: "fragment spread is not a field - only fields returned",
			query: `fragment F on Hero { name }
			{
				hero { ...F }
			}`,
			expected: []string{"hero"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			extr := &DefaultCollectionExtractor{}
			collections, err := extr.ExtractCollections(c.query)

			if c.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, collections, c.expected)
			}
		})
	}
}
