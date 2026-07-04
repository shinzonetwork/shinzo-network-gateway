package host

import (
	"context"
)

// Provider calls register and deregister callbacks to notify when hosts are registered/deregistered.
type Provider interface {
	Run(ctx context.Context, register func(Host), deregister func(Host)) error
}
