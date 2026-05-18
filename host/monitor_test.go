package host

import (
	"context"
	"slices"
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

func TestGetSliceDiffs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		old, next      []string
		expectedAdds   []string
		expectedRmoves []string
	}{
		{
			name: "both empty",
			old:  []string{},
			next: []string{},
		},
		{
			name: "both nil",
		},
		{
			name:         "old empty, all added",
			old:          []string{},
			next:         []string{"bar", "foo"},
			expectedAdds: []string{"bar", "foo"},
		},
		{
			name:           "next empty, all removed",
			old:            []string{"bar", "foo"},
			next:           []string{},
			expectedRmoves: []string{"bar", "foo"},
		},
		{
			name:         "add one",
			old:          []string{"foo", "bar"},
			next:         []string{"foo", "bar", "baz"},
			expectedAdds: []string{"baz"},
		},
		{
			name: "no change",
			old:  []string{"foo", "bar"},
			next: []string{"foo", "bar"},
		},
		{
			name:           "remove one",
			old:            []string{"bar", "baz", "foo"},
			next:           []string{"bar", "foo"},
			expectedRmoves: []string{"baz"},
		},
		{
			name:           "adds and removals",
			old:            []string{"bar", "foo"},
			next:           []string{"baz", "qux"},
			expectedAdds:   []string{"baz", "qux"},
			expectedRmoves: []string{"bar", "foo"},
		},
		{
			name:         "add in front",
			old:          []string{"foo"},
			next:         []string{"bar", "baz", "foo"},
			expectedAdds: []string{"bar", "baz"},
		},
		{
			name:         "add at back",
			old:          []string{"bar"},
			next:         []string{"bar", "foo", "qux"},
			expectedAdds: []string{"foo", "qux"},
		},
		{
			name:           "remove from front",
			old:            []string{"bar", "baz", "foo"},
			next:           []string{"foo"},
			expectedRmoves: []string{"bar", "baz"},
		},
		{
			name:           "remove from back",
			old:            []string{"bar", "foo", "qux"},
			next:           []string{"bar"},
			expectedRmoves: []string{"foo", "qux"},
		},
		{
			name:           "mixed adds, removes, and unchanged",
			old:            []string{"alpha", "bravo", "delta", "echo", "golf", "hotel"},
			next:           []string{"alpha", "charlie", "delta", "foxtrot", "golf", "india"},
			expectedAdds:   []string{"charlie", "foxtrot", "india"},
			expectedRmoves: []string{"bravo", "echo", "hotel"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			slices.Sort(c.old)
			slices.Sort(c.next)
			adds, removes := getSliceDiffs(c.old, c.next)

			require.Equal(t, c.expectedAdds, adds)
			require.Equal(t, c.expectedRmoves, removes)
		})
	}
}
