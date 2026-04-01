package host

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHttpConnectionChecker(t *testing.T) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	defer func() {
		_ = logger.Sync()
	}()

	const timeout = 100 * time.Millisecond
	const delay = timeout + 10*time.Millisecond

	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(timeout / 10)
		w.WriteHeader(http.StatusOK)
	}))

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
	}))

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		url  string
		ctx  context.Context
		err  bool
	}{
		{
			name: "OK",
			url:  okServer.URL,
			ctx:  context.Background(),
			err:  false,
		},
		{
			name: "not found",
			url:  badServer.URL,
			ctx:  context.Background(),
			err:  true,
		},
		{
			name: "timeoout",
			url:  slowServer.URL,
			ctx:  context.Background(),
			err:  true,
		},
		{
			name: "context canceled",
			url:  okServer.URL,
			ctx:  canceledCtx,
			err:  true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cc := NewHttpConnectionChecker(timeout, logger)
			status := cc.CheckConnection(c.ctx, Host(c.url))
			if c.err {
				assert.False(t, status.Online)
			} else {
				assert.True(t, status.Online)
				assert.NotZero(t, status.RTT)
				assert.LessOrEqual(t, status.RTT, timeout)
			}
		})
	}
}
