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

func TestSinglePlotWorldLifecycle(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Alice"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	tmpl := plots.DefaultPlotArea(plots.SingleAreaName, "")
	tmpl.MinGenHeight, tmpl.MaxGenHeight = 0, 70
	tmpl.MinBuildHeight, tmpl.MaxBuildHeight = 1, 70
	ps.CreateSingleArea(tmpl)

	alice := srv.Join("Alice")
	grantPerms(alice.Handle(), "plots.permpack.basic", "plots.plot.10", "plots.auto", "plots.clear", "plots.delete", "plots.home")
	mustDispatch(t, host, alice.Handle(), "plot auto")
	owned := ps.PlotsByOwner(alice.Handle().UUID())
	if len(owned) != 1 {
		t.Fatalf("expected one plot, got %d", len(owned))
	}
	id := owned[0].ID
	name := plots.SingleWorldName(id)
	inWorld := func() bool {
		l := alice.Handle().Location()
		return l.World != nil && ps.WorldName(l.World) == name
	}
	if !inWorld() {
		t.Fatal("Alice should be inside her plot world")
	}

	// Clear inside the world: the world is unloaded and Alice is moved out, not stranded.
	mustDispatch(t, host, alice.Handle(), "plot clear")
	srv.RunTicks(2)
	if _, ok := ps.WrapWorld(name); ok {
		t.Error("plot world should be unloaded after /plot clear")
	}
	if l := alice.Handle().Location(); l.World == nil || ps.WorldName(l.World) == name {
		t.Fatalf("Alice must be moved out of the unloaded world, at %v", l.World)
	}
	if len(ps.PlotsByOwner(alice.Handle().UUID())) != 1 {
		t.Error("/plot clear must keep the plot")
	}

	// /plot home reloads the unloaded world.
	mustDispatch(t, host, alice.Handle(), "plot home")
	srv.RunTicks(2)
	if _, ok := ps.WrapWorld(name); !ok || !inWorld() {
		t.Fatal("/plot home should reload the world and bring Alice in")
	}

	// Delete inside the world regenerates nothing stale: plot gone, world gone, Alice moved out.
	mustDispatch(t, host, alice.Handle(), "plot delete")
	srv.RunTicks(2)
	if len(ps.PlotsByOwner(alice.Handle().UUID())) != 0 {
		t.Error("/plot delete should remove the plot")
	}
	if _, ok := ps.WrapWorld(name); ok {
		t.Error("plot world should be gone after /plot delete")
	}
	if l := alice.Handle().Location(); l.World == nil || ps.WorldName(l.World) == name {
		t.Fatal("Alice must be moved out on delete")
	}
}

func TestSinglePlotWorldUnloadsWhenLastPlayerLeaves(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Alice"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	ps.CreateSingleArea(nil)

	alice := srv.Join("Alice")
	grantPerms(alice.Handle(), "plots.permpack.basic", "plots.plot.10", "plots.auto")
	mustDispatch(t, host, alice.Handle(), "plot auto")
	owned := ps.PlotsByOwner(alice.Handle().UUID())
	if len(owned) != 1 {
		t.Fatalf("expected one plot, got %d", len(owned))
	}
	name := plots.SingleWorldName(owned[0].ID)
	if _, ok := ps.WrapWorld(name); !ok {
		t.Fatal("plot world should be loaded while Alice is in it")
	}

	main, _ := host.World(plugin.MinecraftKey("overworld"))
	alice.Handle().Teleport(plugin.Location{World: main, Pos: plugin.Vec3{X: 0.5, Y: 80, Z: 0.5}}, plugin.TeleportPlugin)
	srv.RunTicks(3)
	if _, ok := ps.WrapWorld(name); ok {
		t.Error("empty single plot world should be unloaded")
	}
}
