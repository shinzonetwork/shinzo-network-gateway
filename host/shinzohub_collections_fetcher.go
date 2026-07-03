package host

import (
	"context"
	"slices"
	"sync"
	"time"

	"go.uber.org/zap"

	shinzohub "github.com/shinzonetwork/shinzo-network-gateway/pkg/shinzohub-api"
)

// ShinzohubCollectionsFetcher fetches collections information from shinzohub.
type ShinzohubCollectionsFetcher struct {
	client *shinzohub.Client

	lastRefresh     time.Time
	refreshInterval time.Duration

	mtx         sync.Mutex
	collections map[Host][]string

	logger *zap.Logger
}

var _ CollectionsFetcher = &ShinzohubCollectionsFetcher{}

// NewShinzohubCollectionsFetcher creates ShinzohubCollectionsFetcher instance configured to read data every refreshInterval from baseURL.
func NewShinzohubCollectionsFetcher(baseURL string, refreshInterval time.Duration, logger *zap.Logger) (*ShinzohubCollectionsFetcher, error) {
	return &ShinzohubCollectionsFetcher{
		client:          shinzohub.NewClient(baseURL),
		refreshInterval: refreshInterval,
		collections:     make(map[Host][]string),
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
			f.logger.Sugar().Errorw("failed to refresh pools", "error", err)
		} else {
			f.collections = colls
		}
	}

	return f.collections[h], nil
}

func (f *ShinzohubCollectionsFetcher) refresh(ctx context.Context) (map[Host][]string, error) {
	f.logger.Sugar().Info("refreshing pools from shinzohub")

	// TODO(tzdybal): metadata is not populated yet!
	views, err := f.client.GetAllViews(ctx, &shinzohub.QueryViewsRequest{IncludeMetadata: true})
	if err != nil {
		return nil, err
	}
	collsByView := make(map[string]string, len(views))
	for _, v := range views {
		if v.Metadata != nil && v.Metadata.RootType != "" {
			collsByView[v.Address] = v.Metadata.RootType
		}
	}

	hosts, err := f.client.GetAllHosts(ctx)
	if err != nil {
		return nil, err
	}
	endpointByAddress := make(map[string]Host, len(hosts))
	for _, h := range hosts {
		endpointByAddress[h.Address] = Host(h.EndpointAddress)
	}

	pools, err := f.client.GetAllPoolDetails(ctx)
	if err != nil {
		return nil, err
	}
	collsByHost := make(map[Host][]string)
	for _, p := range pools {
		coll, ok := collsByView[p.Pool.ViewAddress]
		if !ok {
			f.logger.Sugar().Warnw("pool references unresolved view, skipping",
				"pool", p.Pool.PoolAddress, "view", p.Pool.ViewAddress)
			continue
		}
		for _, h := range p.Hosts {
			endpoint, ok := endpointByAddress[h.HostAddress]
			if !ok {
				f.logger.Sugar().Warnw("pool host has no known endpoint, skipping",
					"pool", p.Pool.PoolAddress, "host", h.HostAddress)
				continue
			}
			collsByHost[endpoint] = append(collsByHost[endpoint], coll)
		}
	}

	// dedup in case same view is served by many pools
	for h, colls := range collsByHost {
		slices.Sort(colls)
		collsByHost[h] = slices.Compact(colls)
	}

	f.lastRefresh = time.Now()
	f.logger.Sugar().Infow("refreshed pools from shinzohub",
		"views", len(views), "hosts", len(hosts), "pools", len(pools), "hostsWithCollections", len(collsByHost))
	return collsByHost, nil
}
