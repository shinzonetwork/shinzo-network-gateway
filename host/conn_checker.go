package host

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// ConnectionChecker probes whether a host is reachable.
type ConnectionChecker interface {
	CheckConnection(ctx context.Context, h Host) ConnectionStatus
}

// ConnectionStatus reports the result of a connectivity check.
type ConnectionStatus struct {
	Online bool
	RTT    time.Duration
}

// HTTPConnectionChecker checks host reachability via an HTTP GET request.
type HTTPConnectionChecker struct {
	client *http.Client
	logger *zap.Logger
}

var _ ConnectionChecker = &HTTPConnectionChecker{}

// NewHTTPConnectionChecker creates an HttpConnectionChecker with the given request timeout.
func NewHTTPConnectionChecker(timeout time.Duration, logger *zap.Logger) *HTTPConnectionChecker {
	return &HTTPConnectionChecker{
		client: &http.Client{Timeout: timeout},
		logger: logger.Named("conn-checker"),
	}
}

// CheckConnection performs an HTTP GET to the host and returns its online status and RTT.
// A response with status code < 400 (BadRequest) is considered online.
func (cc *HTTPConnectionChecker) CheckConnection(ctx context.Context, h Host) ConnectionStatus {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, string(h), nil)
	if err != nil {
		cc.logger.Debug("failed to build connection check request", zap.String("host", string(h)), zap.Error(err))
		return ConnectionStatus{Online: false}
	}

	cc.logger.Debug("checking host connection", zap.String("host", string(h)))
	start := time.Now()
	resp, err := cc.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		cc.logger.Debug("host connection check failed", zap.String("host", string(h)), zap.Duration("duration", duration), zap.Error(err))
		return ConnectionStatus{Online: false}
	}
	_ = resp.Body.Close()

	cc.logger.Debug("host connection check result", zap.String("host", string(h)), zap.Int("status", resp.StatusCode), zap.Duration("rtt", duration))
	return ConnectionStatus{
		Online: resp.StatusCode < http.StatusBadRequest,
		RTT:    duration,
	}
}
