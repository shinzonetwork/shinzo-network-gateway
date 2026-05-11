package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"
)

const collectionsPath = "/api/v0/collections"

var errUnexpectedStatus = errors.New("unexpected HTTP status")

// CollectionsFetcher retrieves the list of collections served by a host.
type CollectionsFetcher interface {
	FetchCollections(ctx context.Context, h Host) ([]string, error)
}

// HTTPCollectionsFetcher fetches collections via GET <host>/api/v0/collections.
type HTTPCollectionsFetcher struct {
	client *http.Client
	logger *zap.Logger
}

var _ CollectionsFetcher = &HTTPCollectionsFetcher{}

// NewHTTPCollectionsFetcher creates an HTTPCollectionsFetcher with the given request timeout.
func NewHTTPCollectionsFetcher(timeout time.Duration, logger *zap.Logger) *HTTPCollectionsFetcher {
	return &HTTPCollectionsFetcher{
		client: &http.Client{Timeout: timeout},
		logger: logger.Named("collections-fetcher"),
	}
}

// FetchCollections returns the names of active collections served by the host.
func (f *HTTPCollectionsFetcher) FetchCollections(ctx context.Context, h Host) ([]string, error) {
	base, err := url.Parse(string(h))
	if err != nil {
		return nil, fmt.Errorf("invalid host URL %q: %w", string(h), err)
	}
	endpoint := base.JoinPath(collectionsPath).String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: %d", errUnexpectedStatus, resp.StatusCode)
	}

	var entries []struct {
		Name     string
		IsActive bool
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode collections: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsActive {
			names = append(names, e.Name)
		}
	}

	f.logger.Sugar().Debugw("collections fetched", "url", endpoint, "active", len(names), "total", len(entries))
	return names, nil
}
