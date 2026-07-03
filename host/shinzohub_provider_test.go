package host

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestShinzohubProvider(t *testing.T) {
	t.Parallel()

	// this code is borrowed from shinzohub-api/client_test.go
	const (
		keyA = "c2hpbnpvMWRzZ2w4aDBuOWR5NnFnZGNxc3Bodzg2azNncjg0enBkeThka3Ew"
		keyB = "c2hpbnpvMXY0cGp0bjN0a3J2NzNlcDNkamhmOGx5MjRmeTZtOHZkdnQ1ZmN6"
	)
	pages := map[string]string{
		"":   "../pkg/shinzohub-api/testdata/hosts_page0.json",
		keyA: "../pkg/shinzohub-api/testdata/hosts_page1.json",
		keyB: "../pkg/shinzohub-api/testdata/hosts_page2.json",
	}

	var gotKeys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("pagination.key")
		gotKeys = append(gotKeys, key)

		file, ok := pages[key]
		require.True(t, ok, "unexpected pagination.key %q", key)

		body, err := os.ReadFile(file)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	provider, err := NewShinzohubProvider(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, provider)
	provider.SetLogger(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var registered, deregistered []Host
	register := func(h Host) {
		registered = append(registered, h)
	}

	deregister := func(h Host) {
		deregistered = append(deregistered, h)
	}

	provider.updateHosts(ctx, register, deregister)

	require.Equal(t, 24, len(registered))
	require.Equal(t, 0, len(deregistered))

	// unfortunately, all hosts in testdata have the same endpoint address
	require.Contains(t, registered, Host("https://192.168.1.1/api/v0/graphql"))
}
