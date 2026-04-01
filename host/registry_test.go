package host

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewRegistry(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	reg := NewRegistry(logger)
	require.NotNil(t, reg)
	require.NotNil(t, reg.events)
	require.NotNil(t, reg.hosts)
}

func TestRegistryStartStop(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	defer func() {
		_ = logger.Sync()
	}()

	reg := NewRegistry(logger)
	require.NotNil(t, reg)

	providers := []Provider{
		NewMockProvider([]Host{"a.b.c", "127.0.0.1"}),
		NewMockProvider([]Host{"x.y.z", "192.168.0.1"}),
		NewMockProvider([]Host{"shinzo.network", "127.0.0.1"}),
	}
	for _, provider := range providers {
		provider.SetLogger(logger)
	}

	// TODO(tzdybal): add to constructor or something
	reg.providers = append(reg.providers, providers...)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err = reg.Start(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(reg.hosts) == 5
	}, 200*time.Millisecond, 10*time.Millisecond)

	err = reg.Close()
	require.NoError(t, err)
}
