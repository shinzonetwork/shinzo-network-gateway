package host

import (
	"context"
)

type MockProvider struct {
	hosts []Host
}

func NewMockProvider(initialHosts []Host) *MockProvider {
	return &MockProvider{
		hosts: initialHosts,
	}
}

var _ Provider = &MockProvider{}

func (mock *MockProvider) Start(ctx context.Context, updatesCH chan<- Event) error {
	for _, h := range mock.hosts {
		updatesCH <- Event{
			Type: HostRegistered,
			Host: h,
		}
	}
	return nil
}

func (mock *MockProvider) Close() error {
	return nil
}
