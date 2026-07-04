package host

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

type MockProvider struct {
	hosts []Host

	logger *zap.Logger
}

func NewMockProvider(initialHosts []Host, logger *zap.Logger) *MockProvider {
	return &MockProvider{
		hosts:  initialHosts,
		logger: logger.Named("mock-provider"),
	}
}

var _ Provider = &MockProvider{}

func (mock *MockProvider) Run(ctx context.Context, register func(Host), _ func(Host)) error {
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
