package router

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/shinzonetwork/shinzo-network-gateway/host"
)

const defaultSampleSize = 3

func TestSelectHosts(t *testing.T) {
	t.Parallel()

	fixture := map[host.Host][]string{
		"host1.test": {"col1"},
		"host2.test": {"col1", "col2"},
		"host3.test": {"col1", "col2", "col3"},
	}
	// allMembers covers every host, so a query is bounded only by the collection.
	allMembers := []host.Host{"host1.test", "host2.test", "host3.test"}

	cases := []struct {
		name     string
		colls    []string
		members  []host.Host
		expected []host.Host
		err      error
	}{
		{
			name:     "all hosts serving the collection are pool members",
			colls:    []string{"col1"},
			members:  allMembers,
			expected: []host.Host{"host1.test", "host2.test", "host3.test"},
		},
		{
			name:     "single host",
			colls:    []string{"col3"},
			members:  allMembers,
			expected: []host.Host{"host3.test"},
		},
		{
			name:     "restricted to pool members",
			colls:    []string{"col1"},
			members:  []host.Host{"host1.test", "host2.test"},
			expected: []host.Host{"host1.test", "host2.test"},
		},
		{
			name:     "member not serving the collection is excluded",
			colls:    []string{"col2"},
			members:  []host.Host{"host1.test", "host2.test"}, // host1 serves only col1
			expected: []host.Host{"host2.test"},
		},
		{
			name:    "pool not found",
			colls:   []string{"col123"},
			members: allMembers,
			err:     ErrPoolNotFound,
		},
		{
			name:    "no live pool members",
			colls:   []string{"col1"},
			members: []host.Host{"other.test"},
			err:     ErrNoLivePoolMembers,
		},
		{
			name:    "unsupported pool (2 collections)",
			colls:   []string{"col1", "col2"},
			members: allMembers,
			err:     ErrPoolNotSupported,
		},
		{
			name:    "unsupported pool (0 collections)",
			colls:   []string{},
			members: allMembers,
			err:     ErrPoolNotSupported,
		},
		{
			name:    "unsupported pool (`nil` collections)",
			members: allMembers,
			err:     ErrPoolNotSupported,
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
			actual, err := r.SelectHosts(ctx, defaultSampleSize, c.colls, c.members)
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

// TestSelectHostsSamplesRandomly checks the fan-out is a random sample of the
// members rather than the same hosts every call: with n smaller than the
// membership, each call returns exactly n in-pool hosts and every member is
// eventually selected across many calls.
func TestSelectHostsSamplesRandomly(t *testing.T) {
	t.Parallel()
	logger, _ := zap.NewDevelopment()
	r := New(logger)
	members := []host.Host{"h1", "h2", "h3", "h4", "h5", "h6"}
	for _, h := range members {
		r.CollectionsAdded(h, []string{"col1"})
	}

	seen := map[host.Host]bool{}
	for range 200 {
		got, err := r.SelectHosts(context.Background(), 2, []string{"col1"}, members)
		require.NoError(t, err)
		require.Len(t, got, 2)
		for _, h := range got {
			require.Contains(t, members, h)
			seen[h] = true
		}
	}
	require.Len(t, seen, len(members), "every member should be selected across many calls; sampling is not random")
}

// TestRouterDownClearsPool checks that a pool member that has gone offline is
// dropped from the pool and therefore not selected.
func TestRouterDownClearsPool(t *testing.T) {
	t.Parallel()
	logger, _ := zap.NewDevelopment()
	r := New(logger)
	r.CollectionsAdded("host1.test", []string{"col1", "col2"})
	r.CollectionsAdded("host2.test", []string{"col1"})
	members := []host.Host{"host1.test", "host2.test"}

	// with both members live, both can be selected
	hosts, err := r.SelectHosts(context.Background(), defaultSampleSize, []string{"col1"}, members)
	require.NoError(t, err)
	require.ElementsMatch(t, []host.Host{"host1.test", "host2.test"}, hosts)

	// once host1 goes offline it must not be selected
	r.Down("host1.test")
	hosts, err = r.SelectHosts(context.Background(), defaultSampleSize, []string{"col1"}, members)
	require.NoError(t, err)
	require.Equal(t, []host.Host{"host2.test"}, hosts)

	// once every member is offline, there are no live members
	r.Down("host2.test")
	hosts, err = r.SelectHosts(context.Background(), defaultSampleSize, []string{"col1"}, members)
	require.ErrorIs(t, err, ErrNoLivePoolMembers)
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
