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

func (mock *MockProvider) Start(ctx context.Context, updatesCH chan<- Event) error {
	for _, h := range mock.hosts {
		event := Event{
			Type: HostRegistered,
			Host: h,
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		case updatesCH <- event:
			mock.logger.Sugar().Debugw("event submitted", "host", h)
		}
	}
	return nil
}

func (mock *MockProvider) Close() error {
	return nil
}

func (mock *MockProvider) SetLogger(logger *zap.Logger) {
	mock.logger = logger.Named("mock-provider")
}

func TestFileProvider(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	defer func() {
		_ = logger.Sync()
	}()

	p := NewFileProvider("./testdata/hosts.txt")
	p.SetLogger(logger)

	events := make(chan Event, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err = p.Start(ctx, events)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, p.Close())
	}()
}
