package host

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const defaultInterval = 5 * time.Second

var defaultConfig = Config{
	ConnCheckInterval:          defaultInterval,
	CollectionsRefreshInterval: defaultInterval,
}

func TestNewRegistry(t *testing.T) {
	t.Parallel()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	providers := make([]Provider, 10)
	reg := NewRegistry(defaultConfig, providers, nil, nil, logger)
	require.NotNil(t, reg)
	require.NotNil(t, reg.events)
	require.NotNil(t, reg.hosts)
	require.NotEmpty(t, reg.providers)
}

func TestRegistryStartStop(t *testing.T) {
	t.Parallel()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	defer func() {
		_ = logger.Sync()
	}()

	providers := []Provider{
		NewMockProvider([]Host{"a.b.c", "127.0.0.1"}),
		NewMockProvider([]Host{"x.y.z", "192.168.0.1"}),
		NewMockProvider([]Host{"shinzo.network", "127.0.0.1"}),
	}
	for _, provider := range providers {
		provider.SetLogger(logger)
	}

	reg := NewRegistry(defaultConfig, providers, nil, nil, logger)
	require.NotNil(t, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err = reg.Run(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(reg.hosts) == 5
	}, 200*time.Millisecond, 10*time.Millisecond)

	err = reg.Close()
	require.NoError(t, err)
}
