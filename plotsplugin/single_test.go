package plotsplugin_test

import (
	"os"
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/plotsplugin"
)

func TestSinglePlotAreaAutoClaimAndProtection(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t)
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	tmpl := plots.DefaultPlotArea(plots.SingleAreaName, "")
	tmpl.MinGenHeight, tmpl.MaxGenHeight = 0, 70
	tmpl.MinBuildHeight, tmpl.MaxBuildHeight = 1, 70
	ps.CreateSingleArea(tmpl)

	alice := srv.Join("Alice")
	bob := srv.Join("Bob")
	grantPerms(alice.Handle(), "plots.permpack.basic", "plots.plot.10", "plots.auto")
	grantPerms(bob.Handle(), "plots.permpack.basic", "plots.plot.10")

	mustDispatch(t, host, alice.Handle(), "plot auto")
	owned := ps.PlotsByOwner(alice.Handle().UUID())
	if len(owned) != 1 || owned[0].Area.Type != plots.AreaSingle {
		t.Fatalf("expected Alice to own one single plot, got %v", owned)
	}
	plot := owned[0]
	name := plots.SingleWorldName(plot.ID)
	if plot.Area.WorldName != name {
		t.Fatalf("plot area world = %q, want %q", plot.Area.WorldName, name)
	}
	w, ok := ps.WrapWorld(name)
	if !ok {
		t.Fatalf("expected world %s to be loaded", name)
	}
	if got := alice.Handle().Location().World; got == nil || ps.WorldName(got) != name {
		t.Fatalf("expected Alice teleported into world %s", name)
	}
	// Anywhere in the dimension belongs to the plot.
	far := plugin.Location{World: w, Pos: plugin.Vec3{X: 5000.5, Y: 70, Z: -3000.5}}
	if got := ps.PlotAt(far); got != plot {
		t.Fatalf("PlotAt far from origin = %v, want %v", got, plot.ID)
	}

	// A second auto claim gets its own dimension.
	mustDispatch(t, host, bob.Handle(), "plot auto")
	bobs := ps.PlotsByOwner(bob.Handle().UUID())
	if len(bobs) != 1 || bobs[0].Area.WorldName == name {
		t.Fatalf("Bob's plot must be in its own world, got %v", bobs)
	}

	// Protection: the plot is protected as usual, so Bob may not build in Alice's world.
	pos := plugin.BlockPos{X: 3, Y: tmpl.PlotHeight, Z: 3}
	if plot.IsAdded(bob.Handle().UUID()) {
		t.Fatal("Bob must not be added to Alice's plot")
	}
	stone := plugin.DefaultState(registry.BlockStone)
	placeBob := &plugin.BlockPlaceEvent{Player: bob.Handle(), Block: plugin.Block{World: w, Pos: pos}, Placed: stone}
	host.Events().Fire(placeBob)
	if !placeBob.Cancelled() {
		t.Error("outsider should not be allowed to place in Alice's plot world")
	}
	placeAlice := &plugin.BlockPlaceEvent{Player: alice.Handle(), Block: plugin.Block{World: w, Pos: pos}, Placed: stone}
	host.Events().Fire(placeAlice)
	if placeAlice.Cancelled() {
		t.Error("owner should be allowed to place in own plot world")
	}
}

func TestDebugImportWorlds(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Admin"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	ps.CreateSingleArea(nil)

	if err := os.MkdirAll(ps.Server.WorldFolder(plugin.Key{Namespace: "plots", Path: "single/3_-2"}), 0o755); err != nil {
		t.Fatal(err)
	}
	admin := srv.Join("Admin")
	mustDispatch(t, host, admin.Handle(), "plot debugimportworlds")
	list := ps.PlotAreasInWorld("3,-2")
	if len(list) != 1 || list[0].SingleID == nil || *list[0].SingleID != (plots.PlotId{X: 3, Y: -2}) {
		t.Fatalf("expected imported area for world 3,-2, got %v", list)
	}
	if list[0].PlotAbs(plots.PlotId{X: 3, Y: -2}) == nil {
		t.Fatal("expected imported plot 3;-2")
	}
}
