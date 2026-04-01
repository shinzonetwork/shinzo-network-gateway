package host

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type ConnectionChecker interface {
	CheckConnection(ctx context.Context, h Host) ConnectionStatus
}

type ConnectionStatus struct {
	Online bool
	RTT    time.Duration
}

type HttpConnectionChecker struct {
	client *http.Client
	logger *zap.Logger
}

var _ ConnectionChecker = &HttpConnectionChecker{}

func NewHttpConnectionChecker(timeout time.Duration, logger *zap.Logger) *HttpConnectionChecker {
	return &HttpConnectionChecker{
		client: &http.Client{Timeout: timeout},
		logger: logger.Named("connection-checker"),
	}
}

func (cc *HttpConnectionChecker) CheckConnection(ctx context.Context, h Host) ConnectionStatus {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, string(h), nil)
	if err != nil {
		return ConnectionStatus{Online: false}
	}
	start := time.Now()
	resp, err := cc.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		return ConnectionStatus{Online: false}
	}
	_ = resp.Body.Close()

	cc.logger.Sugar().Debugw("host check result", "url", string(h), "status", resp.StatusCode, "rtt", duration)
	return ConnectionStatus{
		Online: resp.StatusCode < 400,
		RTT:    duration,
	}
}
