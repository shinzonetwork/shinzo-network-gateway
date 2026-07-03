package host

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestShinzohubCollectionsFetcher(t *testing.T) {
	t.Parallel()

	const viewsJSONNoMetadata = `{
          "views": [
            {"name": "block_view", "creator": "c1", "address": "0xVIEW_BLOCK", "height": "1", "metadata": null},
            {"name": "tx_view", "creator": "c1", "address": "0xVIEW_TX", "height": "1", "metadata": null}
          ],
          "pagination": {"next_key": null, "total": "2"}
        }`

	const viewsJSONWithMetadata = `{
          "views": [
            {"name": "block_view", "creator": "c1", "address": "0xVIEW_BLOCK", "height": "1", "metadata": {"root_type": "Block"}},
            {"name": "tx_view", "creator": "c1", "address": "0xVIEW_TX", "height": "1", "metadata": {"root_type": "Transaction"}}
          ],
          "pagination": {"next_key": null, "total": "2"}
        }`

	const detailsJSON = `{
		"details": [
			{
				"pool": {"pool_address": "0xPOOL1", "view_address": "0xVIEW_BLOCK", "config": {"window_size": "1"}, "created_at": "1"},
				"hosts": [{"pool_address": "0xPOOL1", "host_address": "shinzo1hosta", "host": {"joined_at": "1"}}],
				"demands": [],
				"stats": {},
				"is_active": true
			},
			{
				"pool": {"pool_address": "0xPOOL2", "view_address": "0xVIEW_BLOCK", "config": {"window_size": "1"}, "created_at": "1"},
				"hosts": [{"pool_address": "0xPOOL2", "host_address": "shinzo1hosta", "host": {"joined_at": "1"}}],
				"demands": [],
				"stats": {},
				"is_active": true
			},
			{
				"pool": {"pool_address": "0xPOOL3", "view_address": "0xVIEW_TX", "config": {"window_size": "1"}, "created_at": "1"},
				"hosts": [{"pool_address": "0xPOOL3", "host_address": "shinzo1hosta", "host": {"joined_at": "1"}}],
				"demands": [],
				"stats": {},
				"is_active": true
			}
		],
		"pagination": {"next_key": null, "total": "3"}
	}`

	const hostsJSON = `{
		"hosts": [
			{"address": "shinzo1hosta", "did": "did:key:x", "connection_string": "1.2.3.4:8080", "endpoint_address": "http://host-a.example/graphql"}
		],
		"pagination": {"next_key": null, "total": "1"}
	}`

	srv := newTestServer(t, map[string]string{
		"/shinzonetwork/view/v1/views":      viewsJSONNoMetadata,
		"/shinzonetwork/view/v1/views?meta": viewsJSONWithMetadata, // the ?meta is special placeholder; it's set if include_metadata=true in actual request
		"/shinzonetwork/host/v1/hosts":      hostsJSON,
		"/shinzonetwork/host/v1/details":    detailsJSON,
	})
	defer srv.Close()

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	f, err := NewShinzohubCollectionsFetcher(srv.URL, time.Minute, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	colls, err := f.FetchCollections(ctx, Host("http://host-a.example/graphql"))
	require.NoError(t, err)
	require.Equal(t, []string{"Block", "Transaction"}, colls)

	// an unknown host has no collections
	colls, err = f.FetchCollections(ctx, Host("http://unknown.example/graphql"))
	require.NoError(t, err)
	require.Empty(t, colls)
}

func newTestServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// this is a special test case
		if r.URL.Query().Has("include_metadata") {
			path += "?meta"
		}
		body, ok := routes[path]
		require.True(t, ok, "unexpected path %q", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}
