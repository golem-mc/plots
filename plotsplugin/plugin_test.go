package plotsplugin_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/event"
	"github.com/golem-mc/plots/internal/generator"
	"github.com/golem-mc/plots/plotsplugin"
	"github.com/golem-mc/worldedit"
	"github.com/golem-mc/worldedit/region"
	westorage "github.com/golem-mc/worldedit/storage"
)

func TestGeneratorLifecycleAndPlotOps(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Admin"))
	host := srv.Host()

	// Verify API service is provided
	api, ok := plots.Lookup(host)
	if !ok {
		t.Fatal("expected plots.API service to be registered")
	}
	ps, ok := plotsplugin.Get()
	if !ok {
		t.Fatal("expected PlotSquared plugin instance")
	}

	// Verify GeneratorProvider resolution via Server.Generator
	gen, err := host.Generator(plugin.MinecraftKey("gen_spec_world"), "plots:s=10,g=5,h=62,f=stone")
	if err != nil {
		t.Fatalf("Server.Generator: %v", err)
	}
	if gen == nil {
		t.Fatal("expected non-nil generator")
	}

	// Create a compact plot world (plot=10, road=5, height=62) so chunk gen & ops stay ultra-fast.
	area := plots.DefaultPlotArea("plotworld", "")
	area.PlotWidth = 10
	area.RoadWidth = 5
	area.PlotHeight = 62
	area.RoadHeight = 62
	area.WallHeight = 62
	area.MinGenHeight = 0
	area.MaxGenHeight = 80
	area.MinBuildHeight = 1
	area.MaxBuildHeight = 80
	area.ComputeGeometry()

	w, err := ps.CreatePlotWorld(area)
	if err != nil {
		t.Fatalf("CreatePlotWorld: %v", err)
	}
	// Hold chunks covering plots (1;1) and (2;1) loaded with a ticket.
	w.AddTicket(plugin.ChunkPos{X: 1, Z: 0}, 3)
	loaded := 0
	for cx := 0; cx <= 2; cx++ {
		for cz := 0; cz <= 1; cz++ {
			w.LoadChunk(plugin.ChunkPos{X: cx, Z: cz}, true, func(plugin.Chunk) { loaded++ })
		}
	}
	srv.RunTicks(5)
	if loaded < 6 {
		t.Fatalf("expected 6 chunks loaded, got %d", loaded)
	}

	// Check generated blocks in plot (1;1): road, wall, plot floor, filling, bedrock, biome
	b11 := area.PlotInteriorBounds(plots.PlotId{X: 1, Y: 1})
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 0, Z: b11.Min.Z}); st.Block() != registry.BlockBedrock {
		t.Errorf("y=0 block = %v, want bedrock", st.Block())
	}
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 61, Z: b11.Min.Z}); st.Block() != registry.BlockStone {
		t.Errorf("y=61 filling = %v, want stone", st.Block())
	}
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 62, Z: b11.Min.Z}); st.Block() != registry.BlockGrassBlock {
		t.Errorf("y=62 floor = %v, want grass_block", st.Block())
	}
	// Wall border (unclaimed) just west of plot (1;1)
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X - 1, Y: 63, Z: b11.Min.Z}); st.Block() != registry.BlockSmoothStoneSlab {
		t.Errorf("unclaimed wall border = %v, want smooth_stone_slab", st.Block())
	}
	// Road 2 blocks west of plot (1;1)
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X - 2, Y: 62, Z: b11.Min.Z}); st.Block() != registry.BlockQuartzBlock {
		t.Errorf("road block = %v, want quartz_block", st.Block())
	}

	// Listen for PlayerClaimPlotEvent
	claimFired := false
	plugin.Listen(host, func(e *event.PlayerClaimPlotEvent) {
		claimFired = true
	})

	alice := srv.Join("Alice")
	bob := srv.Join("Bob")

	// Claim (1;1) and (2;1) for Alice
	p1, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot (1;1): %v", err)
	}
	if !claimFired {
		t.Errorf("expected PlayerClaimPlotEvent to fire")
	}
	// Claimed border should now be sandstone_slab
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X - 1, Y: 63, Z: b11.Min.Z}); st.Block() != registry.BlockSandstoneSlab {
		t.Errorf("claimed border = %v, want sandstone_slab", st.Block())
	}
	// API PlotAt should find Alice's plot
	if got := api.PlotAt(plugin.Location{World: w, Pos: plugin.Vec3{X: float64(b11.Min.X), Y: 63, Z: float64(b11.Min.Z)}}); got == nil || got.Owner != alice.Handle().UUID() {
		t.Errorf("api.PlotAt = %v", got)
	}

	// WorldEdit region restrictions: Alice gets her plot, Bob gets empty slice
	we, ok := plugin.Service[worldedit.API](host)
	if !ok {
		t.Fatal("expected worldedit.API service")
	}
	aliceRegs := we.AllowedRegions(alice.Handle(), w)
	if len(aliceRegs) != 1 {
		t.Errorf("Alice AllowedRegions len = %d, want 1", len(aliceRegs))
	}
	bobRegs := we.AllowedRegions(bob.Handle(), w)
	if len(bobRegs) != 0 {
		t.Errorf("Bob AllowedRegions len = %d, want 0", len(bobRegs))
	}

	// Claim (2;1) and merge east!
	_, err = ps.ClaimPlot(area, plots.PlotId{X: 2, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot (2;1): %v", err)
	}
	merged, err := ps.MergePlot(p1, plots.DirEast, 4, alice.Handle(), true)
	if err != nil || merged == 0 {
		t.Fatalf("MergePlot east: merged=%d err=%v", merged, err)
	}
	// Road between (1;1) and (2;1) should now be grass_block at y=62 and belong to p1
	roadMidX := b11.Max.X + 2
	if st := w.BlockState(plugin.BlockPos{X: roadMidX, Y: 62, Z: b11.Min.Z}); st.Block() != registry.BlockGrassBlock {
		t.Errorf("merged road y=62 = %v, want grass_block", st.Block())
	}
	if got := area.PlotAt(roadMidX, b11.Min.Z); got == nil || got.ID != p1.ID {
		t.Errorf("PlotAt merged road = %v, want %v", got, p1.ID)
	}

	// SetComponent floor -> gold_block
	if err := ps.SetComponent(p1, plots.ComponentFloor, "gold_block"); err != nil {
		t.Fatalf("SetComponent floor: %v", err)
	}
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 62, Z: b11.Min.Z}); st.Block() != registry.BlockGoldBlock {
		t.Errorf("floor after set = %v, want gold_block", st.Block())
	}

	// Unlink restores quartz_block road between (1;1) and (2;1)
	if err := ps.UnlinkPlot(p1, true, "test"); err != nil {
		t.Fatalf("UnlinkPlot: %v", err)
	}
	if st := w.BlockState(plugin.BlockPos{X: roadMidX, Y: 62, Z: b11.Min.Z}); st.Block() != registry.BlockQuartzBlock {
		t.Errorf("unlinked road y=62 = %v, want quartz_block", st.Block())
	}

	// ClearPlot restores grass_block floor
	if err := ps.ClearPlot(p1); err != nil {
		t.Fatalf("ClearPlot: %v", err)
	}
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 62, Z: b11.Min.Z}); st.Block() != registry.BlockGrassBlock {
		t.Errorf("floor after clear = %v, want grass_block", st.Block())
	}

	// DeletePlot removes plot and restores smooth_stone_slab unclaimed border
	if err := ps.DeletePlot(p1, true); err != nil {
		t.Fatalf("DeletePlot: %v", err)
	}
	if area.PlotAbs(p1.ID) != nil {
		t.Errorf("expected (1;1) removed after DeletePlot")
	}
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X - 1, Y: 63, Z: b11.Min.Z}); st.Block() != registry.BlockSmoothStoneSlab {
		t.Errorf("border after delete = %v, want smooth_stone_slab", st.Block())
	}
}

func TestRoadSchematicGeneration(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t)
	ps, ok := plotsplugin.Get()
	if !ok {
		t.Fatal("expected PlotSquared plugin instance")
	}

	// Create a road schematic (gold_block) for world "schemworld" before creating it.
	schemDir := filepath.Join(ps.Server.DataDir(), "schematics", "GEN_ROAD_SCHEMATIC", "schemworld")
	if err := os.MkdirAll(schemDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// RoadWidth=5, PlotWidth=10: sideroad is 5x1x10, intersection is 5x1x5
	makeSchem := func(w, h, l int, blk registry.Block, name string) {
		cb := westorage.NewClipboard(region.NewCuboid(plugin.BlockPos{}, plugin.BlockPos{X: w - 1, Y: h - 1, Z: l - 1}), plugin.BlockPos{})
		st := plugin.DefaultState(blk)
		for y := range h {
			for z := range l {
				for x := range w {
					cb.SetRel(x, y, z, st)
				}
			}
		}
		if err := westorage.SaveSchematicFile(filepath.Join(schemDir, name), cb, westorage.FormatFastV3); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}
	makeSchem(5, 1, 10, registry.BlockGoldBlock, "sideroad.schem")
	makeSchem(5, 1, 5, registry.BlockDiamondBlock, "intersection.schem")

	area := plots.DefaultPlotArea("schemworld", "")
	area.PlotWidth = 10
	area.RoadWidth = 5
	area.PlotHeight = 62
	area.RoadHeight = 62
	area.WallHeight = 62
	area.MinGenHeight = 0
	area.MaxGenHeight = 80
	area.ComputeGeometry()

	w, err := ps.CreatePlotWorld(area)
	if err != nil {
		t.Fatalf("CreatePlotWorld schemworld: %v", err)
	}
	if !area.RoadSchematicEnabled {
		t.Fatal("expected RoadSchematicEnabled=true")
	}

	w.AddTicket(plugin.ChunkPos{X: 0, Z: 0}, 2)
	ready := false
	w.LoadChunk(plugin.ChunkPos{X: 0, Z: 0}, true, func(plugin.Chunk) { ready = true })
	srv.RunTicks(5)
	if !ready {
		t.Fatal("chunk (0,0) did not load")
	}

	// (0, 62, 0) is inside the intersection -> diamond_block!
	if st := w.BlockState(plugin.BlockPos{X: 0, Y: 62, Z: 0}); st.Block() != registry.BlockDiamondBlock {
		t.Errorf("intersection (0,62,0) = %v, want diamond_block", st.Block())
	}
	// (0, 62, 5) is inside the sideroad -> gold_block!
	if st := w.BlockState(plugin.BlockPos{X: 0, Y: 62, Z: 5}); st.Block() != registry.BlockGoldBlock {
		t.Errorf("sideroad (0,62,5) = %v, want gold_block", st.Block())
	}
	// Verify CreateRoadSchematic captures valid files
	p := plots.NewPlot(area, plots.PlotId{X: 1, Y: 1}, plots.EveryoneUUID)
	if err := generator.CreateRoadSchematic(w, area, p, t.TempDir()); err != nil {
		t.Fatalf("CreateRoadSchematic: %v", err)
	}
}

func TestLegacyDataDirHint(t *testing.T) {
	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	// Neither directory exists -> no hint.
	if plotsplugin.CheckLegacyDataDir(pluginsDir, nil) {
		t.Fatal("expected false when plugins/PlotSquared does not exist")
	}
	// Create plugins/PlotSquared without plugins/plots -> hint fires.
	if err := os.MkdirAll(filepath.Join(pluginsDir, "PlotSquared"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !plotsplugin.CheckLegacyDataDir(pluginsDir, nil) {
		t.Fatal("expected true when plugins/PlotSquared exists without plugins/plots")
	}
	// Once plugins/plots exists -> no hint.
	if err := os.MkdirAll(filepath.Join(pluginsDir, "plots"), 0o755); err != nil {
		t.Fatal(err)
	}
	if plotsplugin.CheckLegacyDataDir(pluginsDir, nil) {
		t.Fatal("expected false when plugins/plots already exists")
	}
}
