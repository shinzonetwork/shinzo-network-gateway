package host

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	require.NotNil(t, reg)
	require.NotNil(t, reg.events)
	require.NotNil(t, reg.hosts)
}

func TestRegistryStartStop(t *testing.T) {
	reg := NewRegistry()
	require.NotNil(t, reg)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := reg.Start(ctx)
	require.NoError(t, err)

	err = reg.Close()
	require.NoError(t, err)
}
