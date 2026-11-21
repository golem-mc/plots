package plotsplugin

import (
	"testing"

	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/plots"
)

// A restart loads the stored single plots before any world exists: their areas
// must be registered so the plots resolve and their worlds load on demand.
func TestRegisterSingleOwnedAfterRestart(t *testing.T) {
	t.Chdir(t.TempDir())
	plugintest.Start(t)
	ps, _ := Get()
	ps.CreateSingleArea(nil)

	id := plots.PlotId{X: 4, Y: -1}
	ps.plotsTmp = map[string]map[plots.PlotId]*plots.Plot{plots.SingleAreaName: {id: {ID: id}}}
	ps.registerSingleOwned()

	list := ps.PlotAreasInWorld(plots.SingleWorldName(id))
	if len(list) != 1 || list[0].SingleID == nil || *list[0].SingleID != id {
		t.Fatalf("expected a registered area for %v, got %v", id, list)
	}
}
