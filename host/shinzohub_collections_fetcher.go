package host

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	shinzohub "github.com/shinzonetwork/shinzo-network-gateway/pkg/shinzohub-api"
)

// ShinzohubCollectionsFetcher fetches collections information from shinzohub.
type ShinzohubCollectionsFetcher struct {
	client *shinzohub.Client

	lastRefresh     time.Time
	refreshInterval time.Duration

	mtx         sync.Mutex
	collections map[Host][]string

	poolMtx    sync.RWMutex
	poolRoutes map[string]PoolRoute

	logger *zap.Logger
}

var _ CollectionsFetcher = &ShinzohubCollectionsFetcher{}

// NewShinzohubCollectionsFetcher creates ShinzohubCollectionsFetcher instance configured to read data every refreshInterval from baseURL.
func NewShinzohubCollectionsFetcher(baseURL string, refreshInterval time.Duration, logger *zap.Logger) (*ShinzohubCollectionsFetcher, error) {
	c, err := shinzohub.NewClient(baseURL)
	if err != nil {
		return nil, err
	}
	return &ShinzohubCollectionsFetcher{
		client:          c,
		refreshInterval: refreshInterval,
		collections:     make(map[Host][]string),
		poolRoutes:      make(map[string]PoolRoute),
		logger:          logger.Named("shinzohub-collections-fetcher"),
	}, nil
}

// FetchCollections retrieves the list of collections served by a host.
// This information is build using following information:
// - viewAddress -> collection name is retrieved from /views endpoint,
// - hostAddress -> endpointAddress is retrieved from /hosts endpoint,
// - pools information is retrieved from /details endpoint.
func (f *ShinzohubCollectionsFetcher) FetchCollections(ctx context.Context, h Host) ([]string, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	stale := time.Since(f.lastRefresh) >= f.refreshInterval
	if stale {
		colls, err := f.refresh(ctx)
		if err != nil {
			f.logger.Warn("failed to refresh collections from shinzohub, serving stale data", zap.Error(err))
		} else {
			f.collections = colls
		}
	}

	return f.collections[h], nil
}

func (f *ShinzohubCollectionsFetcher) refresh(ctx context.Context) (map[Host][]string, error) {
	f.logger.Debug("refreshing collections from shinzohub")
	f.lastRefresh = time.Now()

	var views []shinzohub.View
	var hosts []shinzohub.Host
	var pools []shinzohub.PoolDetail

	// fetch
	grp, ctx := errgroup.WithContext(ctx)
	grp.Go(func() (err error) {
		views, err = f.client.GetAllViews(ctx, &shinzohub.QueryViewsRequest{IncludeMetadata: true})
		return
	})
	grp.Go(func() (err error) { hosts, err = f.client.GetAllHosts(ctx); return })
	grp.Go(func() (err error) { pools, err = f.client.GetAllPoolDetails(ctx); return })
	if err := grp.Wait(); err != nil {
		return nil, err
	}

	collsByView := make(map[string]string, len(views))
	for _, v := range views {
		if v.Metadata != nil && v.Metadata.RootType != "" {
			collsByView[v.Address] = v.Metadata.RootType
		}
	}

	endpointByAddress := make(map[string]Host, len(hosts))
	for _, h := range hosts {
		endpointByAddress[h.Address] = Host(h.EndpointAddress)
	}

	collsByHost := make(map[Host][]string)
	poolRoutes := make(map[string]PoolRoute, len(pools))
	for _, p := range pools {
		coll, ok := collsByView[p.Pool.ViewAddress]
		if !ok {
			f.logger.Warn("pool references unresolved view, skipping",
				zap.String("pool", p.Pool.PoolAddress), zap.String("view", p.Pool.ViewAddress))
			continue
		}
		members := make([]Host, 0, len(p.Hosts))
		for _, h := range p.Hosts {
			endpoint, ok := endpointByAddress[h.HostAddress]
			if !ok {
				f.logger.Warn("pool host has no known endpoint, skipping",
					zap.String("pool", p.Pool.PoolAddress), zap.String("host", h.HostAddress))
				continue
			}
			collsByHost[endpoint] = append(collsByHost[endpoint], coll)
			members = append(members, endpoint)
		}
		// Key by lowercase address so a checksummed pool_address from the client
		// resolves regardless of the case shinzohub stores.
		poolRoutes[strings.ToLower(p.Pool.PoolAddress)] = PoolRoute{
			Collection: coll,
			Members:    members,
			IsActive:   p.IsActive,
		}
	}
	f.setPoolRoutes(poolRoutes)

	// dedup in case same view is served by many pools
	for h, colls := range collsByHost {
		slices.Sort(colls)
		collsByHost[h] = slices.Compact(colls)
	}

	f.logger.Debug("refreshed collections from shinzohub",
		zap.Int("views", len(views)), zap.Int("hosts", len(hosts)), zap.Int("pools", len(pools)),
		zap.Int("hostsWithCollections", len(collsByHost)))
	return collsByHost, nil
}

func (f *ShinzohubCollectionsFetcher) setPoolRoutes(routes map[string]PoolRoute) {
	f.poolMtx.Lock()
	defer f.poolMtx.Unlock()
	f.poolRoutes = routes
}

// ResolvePool returns the routing information for a pool by its address. The
// lookup is case-insensitive. ok is false if no pool with that address was seen
// in the last refresh.
func (f *ShinzohubCollectionsFetcher) ResolvePool(poolAddress string) (PoolRoute, bool) {
	f.poolMtx.RLock()
	defer f.poolMtx.RUnlock()
	r, ok := f.poolRoutes[strings.ToLower(poolAddress)]
	return r, ok
}
