package host

import (
	"context"
)

// CollectionsFetcher retrieves the list of collections served by a host.
type CollectionsFetcher interface {
	FetchCollections(ctx context.Context, h Host) ([]string, error)
}
