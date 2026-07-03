package shinzohub_api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultTimeout is the HTTP client timeout used when none is configured.
const DefaultTimeout = 10 * time.Second

const defaultPageSize = 100

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
func (c *Client) GetAllHosts(ctx context.Context) (*QueryHostsResponse, error) {
	allResp := &QueryHostsResponse{}
	key := ""
	for {
		resp, err := c.GetHosts(ctx, &PageRequest{Key: key, Limit: defaultPageSize})
		if err != nil {
			return nil, err
		}
		allResp.Hosts = append(allResp.Hosts, resp.Hosts...)

		if resp.Pagination != nil && len(resp.Pagination.NextKey) > 0 {
			key = resp.Pagination.NextKey
		} else {
			break
		}
	}

	return allResp, nil
}

// GetPools returns the paginated list of pools.
func (c *Client) GetPools(ctx context.Context, pagination *PageRequest) (*QueryPoolsResponse, error) {
	return doRequest[QueryPoolsResponse](ctx, c, "shinzonetwork/host/v1/pools", pagination)
}

// GetPool returns a single pool identified by its address.
func (c *Client) GetPool(ctx context.Context, poolAddress string, pagination *PageRequest) (*QueryPoolResponse, error) {
	path := fmt.Sprintf("shinzonetwork/host/v1/pool/%s", poolAddress)
	return doRequest[QueryPoolResponse](ctx, c, path, pagination)
}

// GetPoolDetail returns a pool together with its hosts and demands, identified by the pool address.
func (c *Client) GetPoolDetail(ctx context.Context, poolAddress string, pagination *PageRequest) (*QueryDetailResponse, error) {
	path := fmt.Sprintf("shinzonetwork/host/v1/pool/%s/detail", poolAddress)
	return doRequest[QueryDetailResponse](ctx, c, path, pagination)
}

// GetPoolDetails returns the paginated list of pools together with their hosts and demands.
func (c *Client) GetPoolDetails(ctx context.Context, pagination *PageRequest) (*QueryDetailsResponse, error) {
	return doRequest[QueryDetailsResponse](ctx, c, "shinzonetwork/host/v1/details", pagination)
}

// GetPoolHosts returns the paginated list of hosts belonging to a pool, identified by the pool address.
func (c *Client) GetPoolHosts(ctx context.Context, poolAddress string, pagination *PageRequest) (*QueryPoolHostsResponse, error) {
	path := fmt.Sprintf("shinzonetwork/host/v1/pools/%s/hosts", poolAddress)
	return doRequest[QueryPoolHostsResponse](ctx, c, path, pagination)
}

// GetHost returns a single host identified by its address.
func (c *Client) GetHost(ctx context.Context, address string, pagination *PageRequest) (*QueryHostResponse, error) {
	path := fmt.Sprintf("shinzonetwork/host/v1/host/%s", address)
	return doRequest[QueryHostResponse](ctx, c, path, pagination)
}

// GetViews returns the paginated list of registered views.
// TODO(tzdybal): impelmeent full filtering from QueryViewsRequest.
func (c *Client) GetViews(ctx context.Context, pagination *PageRequest) (*QueryViewsResponse, error) {
	return doRequest[QueryViewsResponse](ctx, c, "shinzonetwork/view/v1/views", pagination)
}

// GetView returns a single view identified by its contract address.
// TODO(tzdybal): impelmeent full filtering from QueryViewRequest.
func (c *Client) GetView(ctx context.Context, contractAddress string, pagination *PageRequest) (*QueryViewResponse, error) {
	path := fmt.Sprintf("shinzonetwork/view/v1/views/%s", contractAddress)
	return doRequest[QueryViewResponse](ctx, c, path, pagination)
}

// GetViewCount returns the total number of registered views.
func (c *Client) GetViewCount(ctx context.Context) (*QueryViewCountResponse, error) {
	return doRequest[QueryViewCountResponse](ctx, c, "shinzonetwork/view/v1/view_count", nil)
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
