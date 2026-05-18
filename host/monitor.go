package host

import (
	"context"
	"slices"
	"time"

	"go.uber.org/zap"
)

type monitor struct {
	h      Host
	online bool
	colls  []string

	connChecker ConnectionChecker
	collFetcher CollectionsFetcher
	observers   []Observer

	logger *zap.Logger
}

func newMonitor(h Host, connChecker ConnectionChecker, collFetcher CollectionsFetcher, observers []Observer, logger *zap.Logger) *monitor {
	return &monitor{
		h:           h,
		connChecker: connChecker,
		collFetcher: collFetcher,
		observers:   observers,
		logger:      logger.With(zap.String("host", string(h))),
	}
}

func (m *monitor) checkColls(ctx context.Context) {
	newColls, err := m.collFetcher.FetchCollections(ctx, m.h)
	if err != nil {
		m.logger.Sugar().Errorw("error while fetching collections", "error", err)
		return
	}
	slices.Sort(newColls)
	m.notifyCollsUpdate(m.colls, newColls)
	m.colls = newColls
}

func (m *monitor) checkConn(ctx context.Context) {
	res := m.connChecker.CheckConnection(ctx, m.h)
	if res.Online != m.online {
		if res.Online {
			m.notifyHostUp()
			// fetch collections immediately
			m.checkColls(ctx)
		} else {
			m.notifyCollsUpdate(m.colls, nil)
			m.notifyHostDown()
			m.colls = nil
		}
		m.online = res.Online
	}
}

func (m *monitor) run(ctx context.Context, connCheckInterval, collectionsRefreshInterval time.Duration) {
	// start checking status immediately
	m.checkConn(ctx)

	connTicker := time.NewTicker(connCheckInterval)
	defer connTicker.Stop()
	collTicker := time.NewTicker(collectionsRefreshInterval)
	defer collTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			if m.online {
				m.notifyCollsUpdate(m.colls, nil)
				m.notifyHostDown()
			}
			return
		case <-connTicker.C:
			m.checkConn(ctx)
		case <-collTicker.C:
			if m.online {
				m.checkColls(ctx)
			}
		}
	}
}

func (m *monitor) notifyHostUp() {
	for _, o := range m.observers {
		o.Up(m.h)
	}
}

func (m *monitor) notifyHostDown() {
	for _, o := range m.observers {
		o.Down(m.h)
	}
}

// notifyCollsUpdate compares oldColls with newCools, prepares a list of removed collections and list of added collections
// It assumes that oldColls and newColls are sorted.
func (m *monitor) notifyCollsUpdate(oldColls, newColls []string) {
	added, removed := getSliceDiffs(oldColls, newColls)

	for _, o := range m.observers {
		if len(added) > 0 {
			o.CollectionsAdded(m.h, added)
		}
		if len(removed) > 0 {
			o.CollectionsRemoved(m.h, removed)
		}
	}
}

func getSliceDiffs(prev, next []string) (added []string, removed []string) {
	i, j := 0, 0
	for i < len(prev) && j < len(next) {
		switch {
		case prev[i] < next[j]:
			removed = append(removed, prev[i])
			i++
		case prev[i] > next[j]:
			added = append(added, next[j])
			j++
		default:
			i++
			j++
		}
	}
	removed = append(removed, prev[i:]...)
	added = append(added, next[j:]...)

	return
}
