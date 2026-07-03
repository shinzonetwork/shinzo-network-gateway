package shinzohub_api

// PageRequest carries cursor-based pagination parameters for list endpoints.
// It maps to the cosmos.base.query.v1beta1.PageRequest query params.
type PageRequest struct {
	// Key is the next_key from a previous PageResponse, passed back verbatim.
	Key string
	// Limit is the maximum number of results to return in the page.
	Limit uint64
}

// PageResponse holds cursor-based pagination metadata returned by list endpoints.
type PageResponse struct {
	NextKey string `json:"next_key,omitempty"`
	Total   string `json:"total,omitempty"`
}

// QueryPoolsRequest is the request for GET /shinzonetwork/pool/v1/pools.
type QueryPoolsRequest struct {
	Pagination *PageRequest `json:"pagination,omitempty"`
}

// QueryPoolsResponse is the response for GET /shinzonetwork/pool/v1/pools.
type QueryPoolsResponse struct {
	Pools      []Pool        `json:"pools"`
	Pagination *PageResponse `json:"pagination,omitempty"`
}

// QueryPoolRequest is the request for GET /shinzonetwork/pool/v1/pool/{pool_address}.
type QueryPoolRequest struct {
	PoolAddress string `json:"pool_address,omitempty"`
}

// QueryPoolResponse is the response for GET /shinzonetwork/pool/v1/pool/{pool_address}.
type QueryPoolResponse struct {
	Pool Pool `json:"pool"`
}

// QueryDetailRequest is the request for GET /shinzonetwork/pool/v1/pool/{pool_address}/detail.
type QueryDetailRequest struct {
	PoolAddress string `json:"pool_address,omitempty"`
}

// QueryDetailResponse is the response for GET /shinzonetwork/pool/v1/pool/{pool_address}/detail.
type QueryDetailResponse struct {
	Detail PoolDetail `json:"detail"`
}

// QueryDetailsRequest is the request for GET /shinzonetwork/pool/v1/details.
type QueryDetailsRequest struct {
	Pagination *PageRequest `json:"pagination,omitempty"`
}

// QueryDetailsResponse is the response for GET /shinzonetwork/pool/v1/details.
type QueryDetailsResponse struct {
	Details    []PoolDetail  `json:"details"`
	Pagination *PageResponse `json:"pagination,omitempty"`
}

// QueryPoolHostsRequest is the request for GET /shinzonetwork/pool/v1/pools/{pool_address}/hosts.
type QueryPoolHostsRequest struct {
	PoolAddress string       `json:"pool_address,omitempty"`
	Pagination  *PageRequest `json:"pagination,omitempty"`
}

// QueryPoolHostsResponse is the response for GET /shinzonetwork/pool/v1/pools/{pool_address}/hosts.
type QueryPoolHostsResponse struct {
	Hosts      []PoolHostEntry `json:"hosts"`
	Pagination *PageResponse   `json:"pagination,omitempty"`
}

// Pool describes a query pool: its addresses, configuration, and creation time.
type Pool struct {
	PoolAddress string     `json:"pool_address,omitempty"`
	ViewAddress string     `json:"view_address,omitempty"`
	Config      PoolConfig `json:"config"`
	CreatedAt   int64      `json:"created_at,omitempty"`
}

// PoolConfig holds tunable parameters for a Pool.
type PoolConfig struct {
	WindowSize uint64 `json:"window_size,omitempty"`
}

// PoolDetail aggregates a Pool with its hosts, demand entries, and stats.
type PoolDetail struct {
	Pool    Pool              `json:"pool"`
	Hosts   []PoolHostEntry   `json:"hosts"`
	Demands []PoolDemandEntry `json:"demands"`
	Stats   PoolStats         `json:"stats"`
	// NEW: true when pool has >= MinHostsForActive hosts; derived from len(hosts), not stored
	IsActive bool `json:"is_active,omitempty"`
}

// PoolStats holds derived metrics for a Pool.
type PoolStats struct {
	PoolAddress      string `json:"pool_address,omitempty"`
	Price            string `json:"price,omitempty"`
	Utilization      uint64 `json:"utilization,omitempty"`
	TotalQueries     uint64 `json:"total_queries,omitempty"`
	TotalRewards     string `json:"total_rewards,omitempty"`
	LastUpdatedEpoch uint64 `json:"last_updated_epoch,omitempty"`
}

// PoolHostEntry associates a host with the pool it belongs to.
type PoolHostEntry struct {
	PoolAddress string   `json:"pool_address,omitempty"`
	HostAddress string   `json:"host_address,omitempty"`
	Host        PoolHost `json:"host"`
}

// PoolHost holds host-specific membership data within a pool.
type PoolHost struct {
	JoinedAt int64 `json:"joined_at,omitempty"`
}

// PoolDemandEntry associates a registrant's demand with the pool it targets.
type PoolDemandEntry struct {
	PoolAddress       string     `json:"pool_address,omitempty"`
	RegistrantAddress string     `json:"registrant_address,omitempty"`
	Demand            PoolDemand `json:"demand"`
}

// PoolDemand describes a registrant's bonded demand for a pool.
type PoolDemand struct {
	Bond      string `json:"bond,omitempty"`       // sdk.Int as base-10 string
	PricePref string `json:"price_pref,omitempty"` // sdk.Int base-10 string; "0"/empty = no preference
	Binding   bool   `json:"binding,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

// QueryHostsRequest is the request for listing all registered hosts.
type QueryHostsRequest struct {
	Pagination *PageRequest `json:"pagination,omitempty"`
}

// QueryHostsResponse is the response listing all registered hosts.
type QueryHostsResponse struct {
	Hosts      []Host        `json:"hosts"`
	Pagination *PageResponse `json:"pagination,omitempty"`
}

// QueryHostRequest is the request for looking up a single host by address.
type QueryHostRequest struct {
	Address string `json:"address,omitempty"`
}

// QueryHostResponse is the response for looking up a single host by address.
type QueryHostResponse struct {
	Host Host `json:"host"`
}

// Host describes a registered host: its address, DID, and connection details.
type Host struct {
	Address          string `json:"address,omitempty"`
	Did              string `json:"did,omitempty"`
	ConnectionString string `json:"connection_string,omitempty"`
	EndpointAddress  string `json:"endpoint_address,omitempty"`
}
