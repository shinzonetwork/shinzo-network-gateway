package host

import (
	"time"

	"go.uber.org/zap"
)

type ConnectionChecker interface {
	CheckConnection(h Host) ConnectionStatus
}

type ConnectionStatus struct {
	Online bool
	RTT    time.Duration
}

type HttpConnectionChecker struct {
	logger *zap.Logger
}

var _ ConnectionChecker = &HttpConnectionChecker{}

func (cc *HttpConnectionChecker) CheckConnection(h Host) ConnectionStatus {
	start := time.Now()

	duration := time.Since(start)

	return ConnectionStatus{
		Online: true,
		RTT:    duration,
	}
}
