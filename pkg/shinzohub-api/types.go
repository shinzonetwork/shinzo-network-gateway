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

// HostsResponse is the response from the hosts listing endpoint.
type HostsResponse struct {
	Hosts      []Host        `json:"hosts"`
	Pagination *PageResponse `json:"pagination,omitempty"`
}

// Host describes a single host in the network.
type Host struct {
	Address          string `json:"address"`
	DID              string `json:"did"`
	ConnectionString string `json:"connection_string"`
	EndpointAddress  string `json:"endpoint_address"`
}

// PoolsResponse is the response from the pools listing endpoint.
type PoolsResponse struct {
	Pools      []Pool        `json:"pools"`
	Pagination *PageResponse `json:"pagination,omitempty"`
}

// PoolResponse wraps a single pool, returned by the pool-by-address endpoint.
type PoolResponse struct {
	Pool Pool `json:"pool"`
}

// PoolDetailResponse wraps a pool's detail.
type PoolDetailResponse struct {
	Detail PoolDetail `json:"detail"`
}

// PoolDetail describes a pool together with its hosts and demands.
type PoolDetail struct {
	Pool    Pool          `json:"pool"`
	Hosts   []PoolHost    `json:"hosts"`
	Demands []DemandEntry `json:"demands"`
}

// Pool holds the on-chain pool record.
type Pool struct {
	PoolAddress string     `json:"pool_address"`
	ViewAddress string     `json:"view_address"`
	Config      PoolConfig `json:"config"`
	CreatedAt   string     `json:"created_at"`
}

// PoolConfig holds the pool configuration parameters.
type PoolConfig struct {
	WindowSize string `json:"window_size"`
}

// PoolHost associates a host with the pool it joined.
type PoolHost struct {
	PoolAddress string         `json:"pool_address"`
	HostAddress string         `json:"host_address"`
	Host        HostMembership `json:"host"`
}

// HostMembership holds per-host membership data within a pool.
type HostMembership struct {
	JoinedAt string `json:"joined_at"`
}

// DemandEntry associates a demand with the pool and its registrant.
type DemandEntry struct {
	PoolAddress       string `json:"pool_address"`
	RegistrantAddress string `json:"registrant_address"`
	Demand            Demand `json:"demand"`
}

// Demand holds the terms of a registered demand.
type Demand struct {
	Bond      string `json:"bond"`
	PricePref string `json:"price_pref"`
	Binding   bool   `json:"binding"`
	ExpiresAt string `json:"expires_at"`
}
