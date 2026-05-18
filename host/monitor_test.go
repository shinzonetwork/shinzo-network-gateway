package host

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type flapChecker struct{ online bool }

func (f *flapChecker) CheckConnection(_ context.Context, _ Host) ConnectionStatus {
	return ConnectionStatus{Online: f.online}
}

type fixedCollFetcher struct{ colls []string }

func (f *fixedCollFetcher) FetchCollections(_ context.Context, _ Host) ([]string, error) {
	return f.colls, nil
}

type recordingObserver struct {
	ups, downs, adds, removes int
	lastAdd                   []string
}

func (r *recordingObserver) Up(Host)   { r.ups++ }
func (r *recordingObserver) Down(Host) { r.downs++ }
func (r *recordingObserver) CollectionsAdded(_ Host, c []string) {
	r.adds++
	r.lastAdd = c
}
func (r *recordingObserver) CollectionsRemoved(Host, []string) { r.removes++ }

func newTestMonitor(t *testing.T) (*monitor, *flapChecker, *fixedCollFetcher, *recordingObserver) {
	t.Helper()
	cc := &flapChecker{}
	cf := &fixedCollFetcher{}
	obs := &recordingObserver{}
	logger, _ := zap.NewDevelopment()
	m := newMonitor(Host("flap.host"), cc, cf, []Observer{obs}, logger)
	return m, cc, cf, obs
}

func TestMonitorFlapSameCollections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m, cc, cf, obs := newTestMonitor(t)

	cc.online = true
	cf.colls = []string{"Block"}
	m.checkConn(ctx)
	require.Equal(t, 1, obs.ups)
	require.Equal(t, 1, obs.adds)
	require.Equal(t, []string{"Block"}, obs.lastAdd)

	cc.online = false
	m.checkConn(ctx)
	require.Equal(t, 1, obs.downs)
	require.Equal(t, 1, obs.removes)

	cc.online = true
	m.checkConn(ctx)
	require.Equal(t, 2, obs.ups)
	require.Equal(t, 2, obs.adds)
	require.Equal(t, []string{"Block"}, obs.lastAdd)
}

func TestMonitorFlapChangedCollections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m, cc, cf, obs := newTestMonitor(t)

	cc.online = true
	cf.colls = []string{"Block"}
	m.checkConn(ctx)
	require.Equal(t, []string{"Block"}, obs.lastAdd)

	cc.online = false
	m.checkConn(ctx)
	require.Equal(t, 1, obs.removes)

	cc.online = true
	cf.colls = []string{"Block", "Tx"}
	m.checkConn(ctx)
	require.Equal(t, 2, obs.adds)
	require.Equal(t, []string{"Block", "Tx"}, obs.lastAdd)
}
