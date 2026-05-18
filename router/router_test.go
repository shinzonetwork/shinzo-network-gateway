package router

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/shinzonetwork/shinzo-network-gateway/host"
)

func TestSelectHosts(t *testing.T) {
	t.Parallel()

	fixture := map[host.Host][]string{
		"host1.test": {"col1"},
		"host2.test": {"col1", "col2"},
		"host3.test": {"col1", "col2", "col3"},
	}

	// those test cases are mostly about testing router-related logic
	// actuall pool selection is not teste
	cases := []struct {
		name     string
		colls    []string
		expected []host.Host
		err      error
	}{
		{
			name:     "all hosts",
			colls:    []string{"col1"},
			expected: []host.Host{"host1.test", "host2.test", "host3.test"},
		},
		{
			name:     "single host",
			colls:    []string{"col3"},
			expected: []host.Host{"host3.test"},
		},
		{
			name:  "pool not found",
			colls: []string{"col123"},
			err:   ErrPoolNotFound,
		},
		{
			name:  "unsupported pool (2 collections)",
			colls: []string{"col1", "col2"},
			err:   ErrPoolNotSupported,
		},
		{
			name:  "unsupported pool (0 collections)",
			colls: []string{},
			err:   ErrPoolNotSupported,
		},
		{
			name: "unsupported pool (`nil` collections)",
			err:  ErrPoolNotSupported,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			logger, _ := zap.NewDevelopment()
			r := New(logger)

			for h, colls := range fixture {
				r.CollectionsAdded(h, colls)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			actual, err := r.SelectHosts(ctx, c.colls)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				require.Nil(t, actual)
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, actual, c.expected)
			}
		})
	}
}

func TestRouterDownClearsPool(t *testing.T) {
	t.Parallel()
	logger, _ := zap.NewDevelopment()
	r := New(logger)
	h := host.Host("test.host")
	r.CollectionsAdded(h, []string{"col1", "col2"})
	r.Down(h)
	hosts, err := r.SelectHosts(context.Background(), []string{"col1"})
	require.ErrorIs(t, err, ErrNoHostsAvailable)
	require.Nil(t, hosts)
}

func TestRouterCallbacks(t *testing.T) {
	t.Parallel()
	logger, _ := zap.NewDevelopment()
	r := New(logger)
	h1 := host.Host("test1.host")
	h2 := host.Host("test2.host")

	// make sure Up is idempotent
	r.Up(h1)
	r.Up(h2)
	r.Up(h1)
	r.Up(h2)

	// add new collections
	r.CollectionsAdded(h1, []string{"col1", "col2"})
	require.Len(t, r.pools, 2)

	// add to existing collections
	r.CollectionsAdded(h2, []string{"col2", "col1"})
	require.Len(t, r.pools, 2)

	// one new collection
	r.CollectionsAdded(h1, []string{"col1", "col2", "col3"})
	require.Len(t, r.pools, 3)

	// even if host is down, all pools are still alive
	r.Down(h1)
	require.Len(t, r.pools, 3)

	// make sure that repeated calls to down are idempotent
	require.NotPanics(t, func() { r.Down(h1) })

	// even if host is down, all pools are still alive
	r.Down(h2)
	require.Len(t, r.pools, 3)

	r.CollectionsRemoved(h1, []string{"col1", "col2"})
	require.Len(t, r.pools, 3)

	// even if host is removed from the pool, pool continues to exist
	r.CollectionsRemoved(h1, []string{"col1", "col2", "col3"})
	require.Len(t, r.pools, 3)
}
