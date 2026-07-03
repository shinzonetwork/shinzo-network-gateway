package host

import (
	"context"

	"go.uber.org/zap"
)

// Provider calls register and deregister callbacks to notify when hosts are registered/deregistered.
type Provider interface {
	Run(ctx context.Context, register func(Host), deregister func(Host)) error
	SetLogger(logger *zap.Logger)
}
