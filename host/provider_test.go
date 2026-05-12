package host

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type MockProvider struct {
	hosts []Host

	logger *zap.Logger
}

func NewMockProvider(initialHosts []Host) *MockProvider {
	return &MockProvider{
		hosts: initialHosts,
	}
}

var _ Provider = &MockProvider{}

func (mock *MockProvider) Start(ctx context.Context, register func(Host), _ func(Host)) error {
	for _, h := range mock.hosts {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		default:
		}
		register(h)
	}
	return nil
}

func TestFileProvider(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	defer func() {
		_ = logger.Sync()
	}()

	p := NewFileProvider("./testdata/hosts.txt")
	p.SetLogger(logger)

	cnt := 0
	register := func(_ Host) {
		cnt++
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err = p.Start(ctx, register, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return cnt > 1 }, 1*time.Second, 50*time.Millisecond)
}

// SetLogger sets the logger used by the provider.
func (mock *MockProvider) SetLogger(logger *zap.Logger) {
	mock.logger = logger.Named("mock-provider")
}
