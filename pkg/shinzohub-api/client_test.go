package shinzohub_api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetHosts(t *testing.T) {
	fixture, err := os.ReadFile("testdata/hosts.json")
	require.NoError(t, err)

	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.GetHosts(ctx, &PageRequest{Key: "abc", Limit: 10})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// the request is built correctly
	require.Equal(t, "/shinzonetwork/host/v1/hosts", gotPath)
	require.Contains(t, gotQuery, "pagination.key=abc")
	require.Contains(t, gotQuery, "pagination.limit=10")

	// the response is decoded correctly
	require.Len(t, resp.Hosts, 10)

	// this is hardcoded from the fixture
	first := resp.Hosts[0]
	require.Equal(t, "shinzo168x4ff4e05md6c43djcqavtf3kxjgznzgxeyt2", first.Address)
	require.Equal(t, "did:key:z7r8otXUgTfe4Z7QwqKreVS4D3vh7AiuWKuFWSctr596B16NBTByoVoScoQN4GgwfjDicLqTpwTkEJjJbvEEWf8dPDCrU", first.Did)
	require.Equal(t, "192.168.1.1:8080", first.ConnectionString)
	require.Equal(t, "https://192.168.1.1/api/v0/graphql", first.EndpointAddress)

	require.NotNil(t, resp.Pagination)
	require.Equal(t, "c2hpbnpvMXVwZ2g0cmt6bWNsZnI3ZnB5aDBjMmZtdXI5dDg1eW5qYXljendu", resp.Pagination.NextKey)
	require.Equal(t, "0", resp.Pagination.Total)
}

func TestGetAllHosts(t *testing.T) {
	// cursor chain extracted from the page fixtures: "" -> keyA -> keyB -> done.
	const (
		keyA = "c2hpbnpvMWRzZ2w4aDBuOWR5NnFnZGNxc3Bodzg2azNncjg0enBkeThka3Ew"
		keyB = "c2hpbnpvMXY0cGp0bjN0a3J2NzNlcDNkamhmOGx5MjRmeTZtOHZkdnQ1ZmN6"
	)
	pages := map[string]string{
		"":   "testdata/hosts_page0.json",
		keyA: "testdata/hosts_page1.json",
		keyB: "testdata/hosts_page2.json",
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

	client := NewClient(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.GetAllHosts(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// the cursor was followed across all three pages, in order, then stopped
	require.Equal(t, []string{"", keyA, keyB}, gotKeys)

	// every page was accumulated (10 + 10 + 4)
	require.Len(t, resp.Hosts, 24)
	require.Equal(t, "shinzo12hu78xu3cu78l7fwkrfdk4sve0vfurcelu5k69", resp.Hosts[0].Address)
	require.Equal(t, "shinzo1dsgl8h0n9dy6qgdcqsphw86k3gr84zpdy8dkq0", resp.Hosts[10].Address)
	require.Equal(t, "shinzo1v4pjtn3tkrv73ep3djhf8ly24fy6m8vdvt5fcz", resp.Hosts[20].Address)
}
