package shinzohub_api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultTimeout is the HTTP client timeout used when none is configured.
const DefaultTimeout = 10 * time.Second

const (
	defaultPageSize = 100
	maxDrainSize    = 1024 * 1024 // 1MiB
)

// ErrRequestError is returned when the upstream responds with a non-success HTTP status code.
var ErrRequestError = errors.New("HTTP status code")

// Client is a Shinzohub REST API client.
type Client struct {
	baseURL string
	client  *http.Client
}

// NewClient creates new Shinzohub API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		client:  &http.Client{Timeout: DefaultTimeout},
	}
}

// GetHosts returns the paginated list of hosts known to the network.
func (c *Client) GetHosts(ctx context.Context, pagination *PageRequest) (*QueryHostsResponse, error) {
	return doRequest[QueryHostsResponse](ctx, c, "shinzonetwork/host/v1/hosts", pagination)
}

// GetAllHosts fetches every host by following pagination cursors until exhausted.
func (c *Client) GetAllHosts(ctx context.Context) ([]Host, error) {
	extractor := func(resp *QueryHostsResponse) ([]Host, *PageResponse) {
		return resp.Hosts, resp.Pagination
	}
	return getAll(ctx, c.GetHosts, extractor)
}

// GetPools returns the paginated list of pools.
func (c *Client) GetPools(ctx context.Context, pagination *PageRequest) (*QueryPoolsResponse, error) {
	return doRequest[QueryPoolsResponse](ctx, c, "shinzonetwork/host/v1/pools", pagination)
}

// GetAllPools fetches every pool by following pagination cursors until exhausted.
func (c *Client) GetAllPools(ctx context.Context) ([]Pool, error) {
	extractor := func(resp *QueryPoolsResponse) ([]Pool, *PageResponse) {
		return resp.Pools, resp.Pagination
	}
	return getAll(ctx, c.GetPools, extractor)
}

// GetPool returns a single pool identified by its address.
func (c *Client) GetPool(ctx context.Context, poolAddress string) (*QueryPoolResponse, error) {
	path := fmt.Sprintf("shinzonetwork/host/v1/pool/%s", poolAddress)
	return doRequest[QueryPoolResponse](ctx, c, path, nil)
}

// GetPoolDetail returns a pool together with its hosts and demands, identified by the pool address.
func (c *Client) GetPoolDetail(ctx context.Context, poolAddress string) (*QueryDetailResponse, error) {
	path := fmt.Sprintf("shinzonetwork/host/v1/pool/%s/detail", poolAddress)
	return doRequest[QueryDetailResponse](ctx, c, path, nil)
}

// GetPoolDetails returns the paginated list of pools together with their hosts and demands.
func (c *Client) GetPoolDetails(ctx context.Context, pagination *PageRequest) (*QueryDetailsResponse, error) {
	return doRequest[QueryDetailsResponse](ctx, c, "shinzonetwork/host/v1/details", pagination)
}

// GetAllPoolDetails fetches every pool detail by following pagination cursors until exhausted.
func (c *Client) GetAllPoolDetails(ctx context.Context) ([]PoolDetail, error) {
	extractor := func(resp *QueryDetailsResponse) ([]PoolDetail, *PageResponse) {
		return resp.Details, resp.Pagination
	}
	return getAll(ctx, c.GetPoolDetails, extractor)
}

// GetPoolHosts returns the paginated list of hosts belonging to a pool, identified by the pool address.
func (c *Client) GetPoolHosts(ctx context.Context, poolAddress string, pagination *PageRequest) (*QueryPoolHostsResponse, error) {
	path := fmt.Sprintf("shinzonetwork/host/v1/pools/%s/hosts", poolAddress)
	return doRequest[QueryPoolHostsResponse](ctx, c, path, pagination)
}

// GetAllPoolHosts fetches every host of a pool by following pagination cursors until exhausted.
func (c *Client) GetAllPoolHosts(ctx context.Context, poolAddress string) ([]PoolHostEntry, error) {
	fetch := func(ctx context.Context, pagination *PageRequest) (*QueryPoolHostsResponse, error) {
		return c.GetPoolHosts(ctx, poolAddress, pagination)
	}
	extractor := func(resp *QueryPoolHostsResponse) ([]PoolHostEntry, *PageResponse) {
		return resp.Hosts, resp.Pagination
	}
	return getAll(ctx, fetch, extractor)
}

// GetHost returns a single host identified by its address.
func (c *Client) GetHost(ctx context.Context, address string) (*QueryHostResponse, error) {
	path := fmt.Sprintf("shinzonetwork/host/v1/host/%s", address)
	return doRequest[QueryHostResponse](ctx, c, path, nil)
}

// GetViews returns the paginated list of registered views.
// TODO(tzdybal): impelmeent full filtering from QueryViewsRequest.
func (c *Client) GetViews(ctx context.Context, pagination *PageRequest) (*QueryViewsResponse, error) {
	return doRequest[QueryViewsResponse](ctx, c, "shinzonetwork/view/v1/views", pagination)
}

// GetAllViews fetches every view by following pagination cursors until exhausted.
func (c *Client) GetAllViews(ctx context.Context) ([]View, error) {
	extractor := func(resp *QueryViewsResponse) ([]View, *PageResponse) {
		return resp.Views, resp.Pagination
	}
	return getAll(ctx, c.GetViews, extractor)
}

// GetView returns a single view identified by its contract address.
// TODO(tzdybal): impelmeent full filtering from QueryViewRequest.
func (c *Client) GetView(ctx context.Context, contractAddress string) (*QueryViewResponse, error) {
	path := fmt.Sprintf("shinzonetwork/view/v1/views/%s", contractAddress)
	return doRequest[QueryViewResponse](ctx, c, path, nil)
}

func doRequest[RespT any](ctx context.Context, c *Client, path string, pagination *PageRequest) (*RespT, error) {
	fullPath, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullPath, nil)
	if err != nil {
		return nil, err
	}

	prepareRequest(req, pagination)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		// always to drain the Body, but with reasonable limit
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainSize))
		_ = resp.Body.Close()
	}()

	// everything >= HTTP 300 is an error
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: %d", ErrRequestError, resp.StatusCode)
	}

	var out RespT
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return &out, nil
}

func prepareRequest(req *http.Request, pagination *PageRequest) {
	if pagination != nil {
		q := req.URL.Query()
		if pagination.Key != "" {
			q.Set("pagination.key", pagination.Key)
		}
		if pagination.Limit != 0 {
			q.Set("pagination.limit", strconv.FormatUint(pagination.Limit, 10))
		}
		req.URL.RawQuery = q.Encode()
	}

	req.Header.Set("Accept", "application/json")
}

func getAll[T, RespT any](
	ctx context.Context,
	fetch func(context.Context, *PageRequest) (*RespT, error),
	extract func(*RespT) ([]T, *PageResponse),
) ([]T, error) {
	var all []T
	key := ""

	for {
		resp, err := fetch(ctx, &PageRequest{Key: key, Limit: defaultPageSize})
		if err != nil {
			return nil, err
		}
		items, pagination := extract(resp)
		all = append(all, items...)

		if pagination != nil && len(pagination.NextKey) > 0 {
			key = pagination.NextKey
		} else {
			break
		}
	}

	return all, nil
}
