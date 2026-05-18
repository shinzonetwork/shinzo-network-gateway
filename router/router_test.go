package router

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/shinzonetwork/shinzo-network-gateway/host"
)

func TestRouterDownClearsPool(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	r := New(logger)
	h := host.Host("test.host")
	r.CollectionsAdded(h, []string{"col1", "col2"})
	r.Down(h)
	hosts, err := r.SelectHosts(context.Background(), []string{"col1"})
	require.Error(t, err)
	require.Nil(t, hosts)
}
