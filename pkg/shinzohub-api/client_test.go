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
	t.Parallel()
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

	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, client)

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
	require.Equal(t, "did:key:z7r8otXUgTfe4Z7QwqKreVS4D3vh7AiuWKuFWSctr596B16NBTByoVoScoQN4GgwfjDicLqTpwTkEJjJbvEEWf8dPDCrU", first.DID)
	require.Equal(t, "192.168.1.1:8080", first.ConnectionString)
	require.Equal(t, "https://192.168.1.1/api/v0/graphql", first.EndpointAddress)

	require.NotNil(t, resp.Pagination)
	require.Equal(t, "c2hpbnpvMXVwZ2g0cmt6bWNsZnI3ZnB5aDBjMmZtdXI5dDg1eW5qYXljendu", resp.Pagination.NextKey)
	require.Equal(t, "0", resp.Pagination.Total)
}

func TestGetAllHosts(t *testing.T) {
	t.Parallel()
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

	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.GetAllHosts(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// the cursor was followed across all three pages, in order, then stopped
	require.Equal(t, []string{"", keyA, keyB}, gotKeys)

	// every page was accumulated (10 + 10 + 4)
	require.Len(t, resp, 24)
	require.Equal(t, "shinzo12hu78xu3cu78l7fwkrfdk4sve0vfurcelu5k69", resp[0].Address)
	require.Equal(t, "shinzo1dsgl8h0n9dy6qgdcqsphw86k3gr84zpdy8dkq0", resp[10].Address)
	require.Equal(t, "shinzo1v4pjtn3tkrv73ep3djhf8ly24fy6m8vdvt5fcz", resp[20].Address)
}

func TestGetHost(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile("testdata/host.json")
	require.NoError(t, err)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.GetHost(ctx, "shinzo168x4ff4e05md6c43djcqavtf3kxjgznzgxeyt2")
	require.NoError(t, err)
	require.NotNil(t, resp)

	// the request is built correctly
	require.Equal(t, "/shinzonetwork/host/v1/host/shinzo168x4ff4e05md6c43djcqavtf3kxjgznzgxeyt2", gotPath)

	// the response is decoded correctly
	require.Equal(t, "shinzo168x4ff4e05md6c43djcqavtf3kxjgznzgxeyt2", resp.Host.Address)
	require.Equal(t, "did:key:z7r8otXUgTfe4Z7QwqKreVS4D3vh7AiuWKuFWSctr596B16NBTByoVoScoQN4GgwfjDicLqTpwTkEJjJbvEEWf8dPDCrU", resp.Host.DID)
	require.Equal(t, "192.168.1.1:8080", resp.Host.ConnectionString)
	require.Equal(t, "https://192.168.1.1/api/v0/graphql", resp.Host.EndpointAddress)
}

func TestGetPoolDetail(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile("testdata/pool_detail.json")
	require.NoError(t, err)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.GetPoolDetail(ctx, "0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92")
	require.NoError(t, err)
	require.NotNil(t, resp)

	// the request is built correctly
	require.Equal(t, "/shinzonetwork/host/v1/pool/0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92/detail", gotPath)

	// the response is decoded correctly
	detail := resp.Detail
	require.True(t, detail.IsActive)

	require.Equal(t, "0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92", detail.Pool.PoolAddress)
	require.Equal(t, "0x4F2d9A1c6B8e0F3d7A2c5B9e1D4f8A0c3B6e2D97", detail.Pool.ViewAddress)
	require.EqualValues(t, 256, detail.Pool.Config.WindowSize)
	require.EqualValues(t, 1024, detail.Pool.CreatedAt)

	require.Len(t, detail.Hosts, 1)
	host := detail.Hosts[0]
	require.Equal(t, "0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92", host.PoolAddress)
	require.Equal(t, "shinzo1nk9z4r3vq8x5acjs0w8tg3hp9lqz1bd2mn7k9", host.HostAddress)
	require.EqualValues(t, 1090, host.Host.JoinedAt)

	require.Len(t, detail.Demands, 1)
	demand := detail.Demands[0]
	require.Equal(t, "0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92", demand.PoolAddress)
	require.Equal(t, "shinzo1dev3k9c2x7r0qy4m8v5zn6jp1ts2gh8dlqmnp", demand.RegistrantAddress)
	require.Equal(t, "1000000000000000000", demand.Demand.Bond)
	require.Equal(t, "0", demand.Demand.PricePref)
	require.False(t, demand.Demand.Binding)
	require.EqualValues(t, 0, demand.Demand.ExpiresAt)

	require.Equal(t, "0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92", detail.Stats.PoolAddress)
	require.Equal(t, "100000000000000", detail.Stats.Price)
	require.EqualValues(t, 73, detail.Stats.Utilization)
	require.EqualValues(t, 184920, detail.Stats.TotalQueries)
	require.Equal(t, "46230000", detail.Stats.TotalRewards)
	require.EqualValues(t, 412, detail.Stats.LastUpdatedEpoch)
}

func TestGetPoolDetails(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile("testdata/pool_details.json")
	require.NoError(t, err)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.GetPoolDetails(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// the request is built correctly
	require.Equal(t, "/shinzonetwork/host/v1/details", gotPath)

	// the response is decoded correctly
	require.Len(t, resp.Details, 1)
	detail := resp.Details[0]
	require.True(t, detail.IsActive)
	require.Equal(t, "0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92", detail.Pool.PoolAddress)
	require.Len(t, detail.Hosts, 1)
	require.Equal(t, "shinzo1nk9z4r3vq8x5acjs0w8tg3hp9lqz1bd2mn7k9", detail.Hosts[0].HostAddress)
	require.Len(t, detail.Demands, 1)
	require.Equal(t, "shinzo1dev3k9c2x7r0qy4m8v5zn6jp1ts2gh8dlqmnp", detail.Demands[0].RegistrantAddress)
	require.EqualValues(t, 73, detail.Stats.Utilization)

	require.NotNil(t, resp.Pagination)
	require.Empty(t, resp.Pagination.NextKey)
	require.Equal(t, "1", resp.Pagination.Total)
}

func TestGetPoolHosts(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile("testdata/pool_hosts.json")
	require.NoError(t, err)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.GetPoolHosts(ctx, "0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92", nil)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// the request is built correctly
	require.Equal(t, "/shinzonetwork/host/v1/pools/0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92/hosts", gotPath)

	// the response is decoded correctly
	require.Len(t, resp.Hosts, 1)
	host := resp.Hosts[0]
	require.Equal(t, "0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92", host.PoolAddress)
	require.Equal(t, "shinzo1nk9z4r3vq8x5acjs0w8tg3hp9lqz1bd2mn7k9", host.HostAddress)
	require.EqualValues(t, 1090, host.Host.JoinedAt)

	require.NotNil(t, resp.Pagination)
	require.Empty(t, resp.Pagination.NextKey)
	require.Equal(t, "1", resp.Pagination.Total)
}

func TestGetPools(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile("testdata/pools.json")
	require.NoError(t, err)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.GetPools(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// the request is built correctly
	require.Equal(t, "/shinzonetwork/host/v1/pools", gotPath)

	// the response is decoded correctly
	require.Len(t, resp.Pools, 1)
	pool := resp.Pools[0]
	require.Equal(t, "0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92", pool.PoolAddress)
	require.Equal(t, "0x4F2d9A1c6B8e0F3d7A2c5B9e1D4f8A0c3B6e2D97", pool.ViewAddress)
	require.EqualValues(t, 256, pool.Config.WindowSize)
	require.EqualValues(t, 1024, pool.CreatedAt)

	require.NotNil(t, resp.Pagination)
	require.Empty(t, resp.Pagination.NextKey)
	require.Equal(t, "1", resp.Pagination.Total)
}

func TestGetPool(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile("testdata/pool.json")
	require.NoError(t, err)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.GetPool(ctx, "0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92")
	require.NoError(t, err)
	require.NotNil(t, resp)

	// the request is built correctly
	require.Equal(t, "/shinzonetwork/host/v1/pool/0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92", gotPath)

	// the response is decoded correctly
	require.Equal(t, "0x7A3b9C2e1F4d8A6b0C5e9F2d3A7b1C8e4F6D0a92", resp.Pool.PoolAddress)
	require.Equal(t, "0x4F2d9A1c6B8e0F3d7A2c5B9e1D4f8A0c3B6e2D97", resp.Pool.ViewAddress)
	require.EqualValues(t, 256, resp.Pool.Config.WindowSize)
	require.EqualValues(t, 1024, resp.Pool.CreatedAt)
}

func TestGetViews(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile("testdata/views.json")
	require.NoError(t, err)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.GetViews(ctx, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// the request is built correctly
	require.Equal(t, "/shinzonetwork/view/v1/views", gotPath)

	// the response is decoded correctly
	require.Len(t, resp.Views, 1)
	view := resp.Views[0]
	require.Equal(t, "usdc_transfers_0x...", view.Name)
	require.Equal(t, "0x3a7b1c8e4f6d0a925a3b9c2e1f4d8a6b0c5e9f2d", view.Creator)
	require.Equal(t, "0x4F2d9A1c6B8e0F3d7A2c5B9e1D4f8A0c3B6e2D97", view.Address)
	require.EqualValues(t, 1024, view.Height)
	require.Nil(t, view.Metadata)

	require.NotNil(t, resp.Pagination)
	require.Empty(t, resp.Pagination.NextKey)
	require.Equal(t, "1", resp.Pagination.Total)
}

func TestGetView(t *testing.T) {
	t.Parallel()
	fixture, err := os.ReadFile("testdata/view.json")
	require.NoError(t, err)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, err := client.GetView(ctx, "0x4F2d9A1c6B8e0F3d7A2c5B9e1D4f8A0c3B6e2D97", nil)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// the request is built correctly
	require.Equal(t, "/shinzonetwork/view/v1/views/0x4F2d9A1c6B8e0F3d7A2c5B9e1D4f8A0c3B6e2D97", gotPath)

	// the response is decoded correctly
	require.Equal(t, "usdc_transfers_0x", resp.View.Name)
	require.Equal(t, "0x3a7b1c8e4f6d0a925a3b9c2e1f4d8a6b0c5e9f2d", resp.View.Creator)
	require.Equal(t, "0x4F2d9A1c6B8e0F3d7A2c5B9e1D4f8A0c3B6e2D97", resp.View.Address)
	require.EqualValues(t, 1024, resp.View.Height)
	require.Nil(t, resp.View.Metadata)
}
