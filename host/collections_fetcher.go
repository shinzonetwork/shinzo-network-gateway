package host

import (
	"context"
	"errors"
)

const collectionsPath = "/api/v0/collections"

var errUnexpectedStatus = errors.New("unexpected HTTP status")

// CollectionsFetcher retrieves the list of collections served by a host.
type CollectionsFetcher interface {
	FetchCollections(ctx context.Context, h Host) ([]string, error)
}
