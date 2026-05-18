package router

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/shinzonetwork/shinzo-network-gateway/host"
)

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
