package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/google/uuid"
)

func TestGeneratorWorldOps(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t)
	host := srv.Host()

	area := plots.DefaultPlotArea("opsworld", "")
	area.PlotWidth = 6
	area.RoadWidth = 5 // Size=11, lower=2, upper=9; plot (1;1) interior = 3..8, walls = 2 & 9, road = 0,1,10
	area.PlotHeight = 10
	area.RoadHeight = 10
	area.WallHeight = 10
	area.MinGenHeight = 0
	area.MaxGenHeight = 32
	area.MinBuildHeight = -4
	area.MaxBuildHeight = 32
	area.ComputeGeometry()

	gen := NewHybridGenerator(area, "", nil)
	w, err := host.CreateWorld(plugin.WorldOptions{
		Key:       plugin.MinecraftKey("opsworld"),
		Dimension: plugin.MinecraftKey("overworld"),
		Generator: gen,
	})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}
	w.AddTicket(plugin.ChunkPos{X: 0, Z: 0}, 3)
	loaded := 0
	for cx := 0; cx <= 1; cx++ {
		for cz := 0; cz <= 1; cz++ {
			w.LoadChunk(plugin.ChunkPos{X: cx, Z: cz}, true, func(plugin.Chunk) { loaded++ })
		}
	}
	srv.RunTicks(5)
	if loaded < 4 {
		t.Fatalf("expected 4 chunks loaded, got %d", loaded)
	}

	air := plugin.DefaultState(registry.BlockAir)
	bedrock := plugin.DefaultState(registry.BlockBedrock)
	stone := plugin.DefaultState(registry.BlockStone)
	grass := plugin.DefaultState(registry.BlockGrassBlock)
	gold := plugin.DefaultState(registry.BlockGoldBlock)
	diamond := plugin.DefaultState(registry.BlockDiamondBlock)
	emerald := plugin.DefaultState(registry.BlockEmeraldBlock)
	quartz := plugin.DefaultState(registry.BlockQuartzBlock)
	slab := plugin.DefaultState(registry.BlockSmoothStoneSlab)
	sandSlab := plugin.DefaultState(registry.BlockSandstoneSlab)

	owner := uuid.New()
	p1 := plots.NewPlot(area, plots.PlotId{X: 1, Y: 1}, owner)
	p2 := plots.NewPlot(area, plots.PlotId{X: 2, Y: 1}, owner)
	area.SetPlot(p1)
	area.SetPlot(p2)

	// 1. Verify generated plot world blocks in live golem world
	if got := w.BlockState(plugin.BlockPos{X: 3, Y: 10, Z: 3}); got != grass {
		t.Fatalf("plot (1;1) corner (3,10,3) = %v, want grass_block", got)
	}
	if got := w.BlockState(plugin.BlockPos{X: 2, Y: 11, Z: 3}); got != slab {
		t.Fatalf("plot (1;1) west wall top (2,11,3) = %v, want smooth_stone_slab", got)
	}
	if got := w.BlockState(plugin.BlockPos{X: 0, Y: 10, Z: 3}); got != quartz {
		t.Fatalf("road (0,10,3) = %v, want quartz_block", got)
	}

	// 2. ClaimPlotBlocks & UnclaimPlotBlocks
	ClaimPlotBlocks(w, area, p1, "Alice")
	if got := w.BlockState(plugin.BlockPos{X: 2, Y: 11, Z: 3}); got != sandSlab {
		t.Errorf("claimed border (2,11,3) = %v, want sandstone_slab", got)
	}
	signPos := SignLocation(area, p1)
	if w.BlockState(signPos).Block() != registry.BlockOakWallSign {
		t.Errorf("claim sign at %v = %v, want oak_wall_sign", signPos, w.BlockState(signPos))
	}

	// 3. SetComponent: floor, main, air, outline, middle, wall, border
	SetComponent(w, area, p1, plots.ComponentFloor, plots.MustBlockBucket("gold_block"))
	if got := w.BlockState(plugin.BlockPos{X: 5, Y: 10, Z: 5}); got != gold {
		t.Errorf("ComponentFloor = %v, want gold_block", got)
	}
	SetComponent(w, area, p1, plots.ComponentMain, plots.MustBlockBucket("diamond_block"))
	if got := w.BlockState(plugin.BlockPos{X: 5, Y: 5, Z: 5}); got != diamond {
		t.Errorf("ComponentMain = %v, want diamond_block", got)
	}
	SetComponent(w, area, p1, plots.ComponentMiddle, plots.MustBlockBucket("emerald_block"))
	if got := w.BlockState(plugin.BlockPos{X: 5, Y: 10, Z: 5}); got != emerald {
		t.Errorf("ComponentMiddle (5,10,5) = %v, want emerald_block", got)
	}
	SetComponent(w, area, p1, plots.ComponentWall, plots.MustBlockBucket("bricks"))
	if got := w.BlockState(plugin.BlockPos{X: 2, Y: 10, Z: 5}); got != plugin.DefaultState(registry.BlockBricks) {
		t.Errorf("ComponentWall = %v, want bricks", got)
	}
	SetComponent(w, area, p1, plots.ComponentOutline, plots.MustBlockBucket("oak_planks"))
	if got := w.BlockState(plugin.BlockPos{X: 3, Y: 12, Z: 5}); got != plugin.DefaultState(registry.BlockOakPlanks) {
		t.Errorf("ComponentOutline = %v, want oak_planks", got)
	}

	// 4. ClearPlot: clears above PlotHeight, restores floor/filling/bedrock, clears below MinGenHeight down to MinBuildHeight
	w.SetBlockState(plugin.BlockPos{X: 5, Y: 15, Z: 5}, diamond, plugin.UpdateClients)
	w.SetBlockState(plugin.BlockPos{X: 5, Y: -2, Z: 5}, diamond, plugin.UpdateClients)
	ClearPlot(w, area, p1, gen)
	if got := w.BlockState(plugin.BlockPos{X: 5, Y: 15, Z: 5}); got != air {
		t.Errorf("ClearPlot above plotH = %v, want air", got)
	}
	if got := w.BlockState(plugin.BlockPos{X: 5, Y: -2, Z: 5}); got != air {
		t.Errorf("ClearPlot below minGenY = %v, want air", got)
	}
	if got := w.BlockState(plugin.BlockPos{X: 5, Y: 10, Z: 5}); got != grass {
		t.Errorf("ClearPlot floor = %v, want grass_block", got)
	}
	if got := w.BlockState(plugin.BlockPos{X: 5, Y: 5, Z: 5}); got != stone {
		t.Errorf("ClearPlot main = %v, want stone", got)
	}
	if got := w.BlockState(plugin.BlockPos{X: 5, Y: 0, Z: 5}); got != bedrock {
		t.Errorf("ClearPlot bedrock = %v, want bedrock", got)
	}

	// 5. RemoveRoadEast, merged SetComponent(border) bridging across road, CreateRoadEast
	p1.Settings.Merged[plots.DirEast] = true
	p2.Settings.Merged[plots.DirWest] = true
	RemoveRoadEast(w, area, p1.ID)
	// Road between (1;1) [3..8] and (2;1) [14..19] is x=9..13
	for x := 9; x <= 13; x++ {
		if got := w.BlockState(plugin.BlockPos{X: x, Y: 10, Z: 5}); got != grass {
			t.Errorf("RemoveRoadEast (%d,10,5) = %v, want grass_block", x, got)
		}
		if got := w.BlockState(plugin.BlockPos{X: x, Y: 11, Z: 5}); got != air {
			t.Errorf("RemoveRoadEast (%d,11,5) = %v, want air", x, got)
		}
	}
	// Border set on merged plot must bridge north wall (z=2) across road strip x=9..13 too
	SetComponent(w, area, p1, plots.ComponentBorder, plots.MustBlockBucket("gold_block"))
	if got := w.BlockState(plugin.BlockPos{X: 11, Y: 11, Z: 2}); got != gold {
		t.Errorf("merged north border across road (11,11,2) = %v, want gold_block", got)
	}

	// Unlink & CreateRoadEast restores walls (x=9, x=13) and quartz road (x=10..12)
	p1.Settings.Merged[plots.DirEast] = false
	p2.Settings.Merged[plots.DirWest] = false
	CreateRoadEast(w, area, p1.ID, gen)
	if got := w.BlockState(plugin.BlockPos{X: 9, Y: 11, Z: 5}); got != slab {
		t.Errorf("CreateRoadEast wall (9,11,5) = %v, want smooth_stone_slab", got)
	}
	if got := w.BlockState(plugin.BlockPos{X: 11, Y: 10, Z: 5}); got != quartz {
		t.Errorf("CreateRoadEast road (11,10,5) = %v, want quartz_block", got)
	}

	// 6. CopyPlotBlocks & SwapPlotBlocks
	w.SetBlockState(plugin.BlockPos{X: 4, Y: 11, Z: 4}, diamond, plugin.UpdateClients)
	CopyPlotBlocks(w, area, p1.ID, w, area, p2.ID)
	// p2 interior starts at x=14, z=3 -> offset (4-3=1, 4-3=1) => (15, 11, 4)
	if got := w.BlockState(plugin.BlockPos{X: 15, Y: 11, Z: 4}); got != diamond {
		t.Errorf("CopyPlotBlocks target (15,11,4) = %v, want diamond_block", got)
	}
	w.SetBlockState(plugin.BlockPos{X: 15, Y: 11, Z: 4}, emerald, plugin.UpdateClients)
	SwapPlotBlocks(w, area, p1.ID, w, area, p2.ID)
	if got := w.BlockState(plugin.BlockPos{X: 4, Y: 11, Z: 4}); got != emerald {
		t.Errorf("SwapPlotBlocks p1 (4,11,4) = %v, want emerald_block", got)
	}
	if got := w.BlockState(plugin.BlockPos{X: 15, Y: 11, Z: 4}); got != diamond {
		t.Errorf("SwapPlotBlocks p2 (15,11,4) = %v, want diamond_block", got)
	}

	// 7. CreateRoadSchematic & RegenRoadChunk
	dataDir := t.TempDir()
	w.SetBlockState(plugin.BlockPos{X: 11, Y: 10, Z: 5}, gold, plugin.UpdateClients)
	if err := CreateRoadSchematic(w, area, p1, dataDir); err != nil {
		t.Fatalf("CreateRoadSchematic: %v", err)
	}
	for _, fn := range []string{"sideroad.schem", "intersection.schem"} {
		if _, err := os.Stat(filepath.Join(dataDir, "schematics", "GEN_ROAD_SCHEMATIC", area.WorldName, fn)); err != nil {
			t.Errorf("missing %s: %v", fn, err)
		}
	}
	gen.Reload(dataDir, nil)
	// Damage the road in chunk (0, 0) at (0, 10, 5) — which corresponds to relX=0 (same as x=11 mod 11)
	w.SetBlockState(plugin.BlockPos{X: 0, Y: 10, Z: 5}, air, plugin.UpdateClients)
	RegenRoadChunk(w, area, gen, 0, 0)
	if got := w.BlockState(plugin.BlockPos{X: 0, Y: 10, Z: 5}); got != gold {
		t.Errorf("RegenRoadChunk with road schematic (0,10,5) = %v, want gold_block", got)
	}

	// 8. UnclaimPlotBlocks restores wall & removes sign
	UnclaimPlotBlocks(w, area, p1)
	if got := w.BlockState(plugin.BlockPos{X: 2, Y: 11, Z: 3}); got != slab {
		t.Errorf("UnclaimPlotBlocks border = %v, want smooth_stone_slab", got)
	}
	if got := w.BlockState(signPos); got != air {
		t.Errorf("UnclaimPlotBlocks sign = %v, want air", got)
	}
}
