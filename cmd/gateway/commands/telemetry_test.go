package commands

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/resource"
)

// Tests in this file use t.Setenv, so they cannot run in parallel.

func TestSetupOTel(t *testing.T) {
	tests := []struct {
		name     string
		exporter string
		attrs    string
		wantErr  bool
		errIs    error
	}{
		{name: "disabled ignores malformed attributes", exporter: "", attrs: "service.instance.id"},
		{name: "valid config", exporter: "none", attrs: "service.instance.id=node-1"},
		{name: "malformed attributes", exporter: "none", attrs: "service.instance.id", wantErr: true, errIs: resource.ErrPartialResource},
		{name: "unknown exporter", exporter: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_METRICS_EXPORTER", tt.exporter)
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", tt.attrs)

			shutdown, err := setupOTel(t.Context())
			if !tt.wantErr {
				require.NoError(t, err)
				require.NoError(t, shutdown(t.Context()))
				return
			}
			require.Error(t, err)
			if tt.errIs != nil {
				assert.ErrorIs(t, err, tt.errIs)
			}
		})
	}
}

func TestStart_TelemetryMisconfiguredFails(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.instance.id")

	cmd, err := NewApp().newRootCmd()
	require.NoError(t, err)
	cmd.SetArgs([]string{"start", "--listen", "127.0.0.1:0", "--shinzohub-url", "http://127.0.0.1:1"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	// A canceled context makes start return promptly even if telemetry setup were not fatal.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = cmd.ExecuteContext(ctx)
	require.ErrorIs(t, err, resource.ErrPartialResource)
	assert.Contains(t, err.Error(), "telemetry setup")
}
