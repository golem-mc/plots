package generator

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/config"
	"github.com/golem-mc/worldedit/block"
	"github.com/golem-mc/worldedit/region"
	westorage "github.com/golem-mc/worldedit/storage"
)

type fakeRandom struct {
	r *rand.Rand
}

func newFakeRandom(seed uint64) *fakeRandom {
	return &fakeRandom{r: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}

func (f *fakeRandom) Int(n int) int    { return f.r.IntN(n) }
func (f *fakeRandom) Int64() int64     { return f.r.Int64() }
func (f *fakeRandom) Float64() float64 { return f.r.Float64() }
func (f *fakeRandom) Bool() bool       { return f.r.IntN(2) == 1 }

// memChunk implements plugin.ChunkWriter in memory for fast block-by-block generator verification.
type memChunk struct {
	pos    plugin.ChunkPos
	minY   int
	height int
	blocks []plugin.BlockState
	biomes map[[3]int]registry.Biome
}

func newMemChunk(cx, cz, minY, height int) *memChunk {
	air := plugin.DefaultState(registry.BlockAir)
	blocks := make([]plugin.BlockState, 16*16*height)
	for i := range blocks {
		blocks[i] = air
	}
	return &memChunk{
		pos:    plugin.ChunkPos{X: cx, Z: cz},
		minY:   minY,
		height: height,
		blocks: blocks,
		biomes: make(map[[3]int]registry.Biome),
	}
}

func (c *memChunk) Pos() plugin.ChunkPos { return c.pos }
func (c *memChunk) MinY() int            { return c.minY }
func (c *memChunk) Height() int          { return c.height }

func (c *memChunk) idx(x, y, z int) int {
	return ((y-c.minY)*16+z)*16 + x
}

func (c *memChunk) Set(x, y, z int, s plugin.BlockState) {
	if x < 0 || x >= 16 || z < 0 || z >= 16 || y < c.minY || y >= c.minY+c.height {
		return
	}
	c.blocks[c.idx(x, y, z)] = s
}

func (c *memChunk) Get(x, y, z int) plugin.BlockState {
	if x < 0 || x >= 16 || z < 0 || z >= 16 || y < c.minY || y >= c.minY+c.height {
		return plugin.DefaultState(registry.BlockAir)
	}
	return c.blocks[c.idx(x, y, z)]
}

func (c *memChunk) SetBiome(x, y, z int, b registry.Biome) {
	c.biomes[[3]int{x >> 2, (y - c.minY) >> 2, z >> 2}] = b
}

func (c *memChunk) BiomeAt(x, y, z int) registry.Biome {
	return c.biomes[[3]int{x >> 2, (y - c.minY) >> 2, z >> 2}]
}

// chunkGrid caches generated memChunks for querying arbitrary world (x, y, z) coordinates.
type chunkGrid struct {
	gen    *HybridGenerator
	minY   int
	height int
	chunks map[plugin.ChunkPos]*memChunk
}

func newChunkGrid(gen *HybridGenerator, minY, height int) *chunkGrid {
	return &chunkGrid{
		gen:    gen,
		minY:   minY,
		height: height,
		chunks: make(map[plugin.ChunkPos]*memChunk),
	}
}

func (g *chunkGrid) chunkAt(cx, cz int) *memChunk {
	pos := plugin.ChunkPos{X: cx, Z: cz}
	if ch, ok := g.chunks[pos]; ok {
		return ch
	}
	ch := newMemChunk(cx, cz, g.minY, g.height)
	seed := uint64(cx)*341873128712 + uint64(cz)*132897987541 + 1
	g.gen.Generate(ch, newFakeRandom(seed))
	g.chunks[pos] = ch
	return ch
}

func (g *chunkGrid) BlockAt(wx, wy, wz int) plugin.BlockState {
	cx := wx >> 4
	cz := wz >> 4
	return g.chunkAt(cx, cz).Get(wx&15, wy, wz&15)
}

func (g *chunkGrid) BiomeAt(wx, wy, wz int) registry.Biome {
	cx := wx >> 4
	cz := wz >> 4
	return g.chunkAt(cx, cz).BiomeAt(wx&15, wy, wz&15)
}

// upstreamColumnKind classifies world (wx, wz) directly from upstream HybridPlotWorld.java:166-175
// and HybridGen.java:124-241 without calling any package plots geometry helpers.
func upstreamColumnKind(wx, wz, plotWidth, roadWidth, offsetX, offsetZ int) string {
	size := plotWidth + roadWidth
	mod := func(a, m int) int {
		r := a % m
		if r < 0 {
			r += m
		}
		return r
	}
	if roadWidth == 0 {
		return "plot"
	}
	var lower int
	if roadWidth&1 == 0 {
		lower = roadWidth/2 - 1
	} else {
		lower = roadWidth / 2
	}
	upper := lower + plotWidth + 1

	rx := mod(wx-offsetX, size)
	rz := mod(wz-offsetZ, size)

	inRoadX := rx < lower || rx > upper
	inWallX := rx == lower || rx == upper
	inRoadZ := rz < lower || rz > upper
	inWallZ := rz == lower || rz == upper

	if inRoadX || inRoadZ {
		return "road"
	}
	if inWallX || inWallZ {
		return "wall"
	}
	return "plot"
}

func TestHybridGenDefaultWorld(t *testing.T) {
	// Upstream defaults (ClassicPlotWorld.java + SquarePlotWorld.java):
	// PLOT_WIDTH=42, ROAD_WIDTH=7, SIZE=49, PATH_WIDTH_LOWER=3, PATH_WIDTH_UPPER=46
	// PLOT_HEIGHT=62, ROAD_HEIGHT=62, WALL_HEIGHT=62, MIN_GEN_HEIGHT=0, MAX_GEN_HEIGHT=256
	// MAIN_BLOCK=stone, TOP_BLOCK=grass_block, WALL_FILLING=stone, WALL_BLOCK=smooth_stone_slab, ROAD_BLOCK=quartz_block
	area := plots.DefaultPlotArea("plots", "")
	gen := NewHybridGenerator(area, "", nil)
	grid := newChunkGrid(gen, -64, 384) // standard overworld range -64..319

	air := plugin.DefaultState(registry.BlockAir)
	bedrock := plugin.DefaultState(registry.BlockBedrock)
	stone := plugin.DefaultState(registry.BlockStone)
	grass := plugin.DefaultState(registry.BlockGrassBlock)
	slab := plugin.DefaultState(registry.BlockSmoothStoneSlab)
	quartz := plugin.DefaultState(registry.BlockQuartzBlock)

	// Verify every single column in a full 49x49 cell (0..48 x 0..48), plus crossing into
	// the next cell (49..63) and negative coordinates (-16..-1).
	var plotCols, wallCols, roadCols int
	checkColumn := func(wx, wz int, countCell bool) {
		t.Helper()
		kind := upstreamColumnKind(wx, wz, 42, 7, 0, 0)
		if countCell {
			switch kind {
			case "plot":
				plotCols++
			case "wall":
				wallCols++
			case "road":
				roadCols++
			}
		}

		// Below minGenHeight (0): untouched air
		if got := grid.BlockAt(wx, -1, wz); got != air {
			t.Fatalf("(%d,-1,%d) [%s] = %v, want air", wx, wz, kind, got)
		}
		// Bedrock at minGenHeight (0)
		if got := grid.BlockAt(wx, 0, wz); got != bedrock {
			t.Fatalf("(%d,0,%d) [%s] = %v, want bedrock", wx, wz, kind, got)
		}
		// Default biome everywhere
		if got := grid.BiomeAt(wx, 62, wz); got != registry.BiomeForest {
			t.Fatalf("biome(%d,62,%d) = %v, want forest", wx, wz, got)
		}

		switch kind {
		case "plot":
			for _, y := range []int{1, 30, 61} {
				if got := grid.BlockAt(wx, y, wz); got != stone {
					t.Fatalf("plot(%d,%d,%d) = %v, want stone", wx, y, wz, got)
				}
			}
			if got := grid.BlockAt(wx, 62, wz); got != grass {
				t.Fatalf("plot(%d,62,%d) = %v, want grass_block", wx, wz, got)
			}
			for _, y := range []int{63, 64, 128} {
				if got := grid.BlockAt(wx, y, wz); got != air {
					t.Fatalf("plot(%d,%d,%d) = %v, want air", wx, y, wz, got)
				}
			}
		case "wall":
			for _, y := range []int{1, 30, 61, 62} {
				if got := grid.BlockAt(wx, y, wz); got != stone {
					t.Fatalf("wall(%d,%d,%d) = %v, want stone", wx, y, wz, got)
				}
			}
			if got := grid.BlockAt(wx, 63, wz); got != slab {
				t.Fatalf("wall(%d,63,%d) = %v, want smooth_stone_slab", wx, wz, got)
			}
			if got := grid.BlockAt(wx, 64, wz); got != air {
				t.Fatalf("wall(%d,64,%d) = %v, want air", wx, wz, got)
			}
		case "road":
			for _, y := range []int{1, 30, 61, 62} {
				if got := grid.BlockAt(wx, y, wz); got != quartz {
					t.Fatalf("road(%d,%d,%d) = %v, want quartz_block", wx, y, wz, got)
				}
			}
			if got := grid.BlockAt(wx, 63, wz); got != air {
				t.Fatalf("road(%d,63,%d) = %v, want air", wx, wz, got)
			}
		}
	}

	// Full cell 0..48 x 0..48
	for wx := range 49 {
		for wz := range 49 {
			checkColumn(wx, wz, true)
		}
	}
	// Verify exact column counts in one 49x49 cell:
	// Plot interior: 42*42 = 1764
	// Walls: 44*44 - 42*42 = 172 (including 4 corners)
	// Road: 49*49 - 44*44 = 465 (5-wide road strips + 5x5 intersections)
	if plotCols != 42*42 || wallCols != 44*44-42*42 || roadCols != 49*49-44*44 {
		t.Fatalf("cell column counts: plot=%d wall=%d road=%d, want 1764/172/465", plotCols, wallCols, roadCols)
	}

	// Cross-cell boundary (49..63) and negative chunk (-16..-1)
	for wx := 49; wx < 64; wx++ {
		for wz := range 64 {
			checkColumn(wx, wz, false)
		}
	}
	for wx := -16; wx < 0; wx++ {
		for wz := -16; wz < 0; wz++ {
			checkColumn(wx, wz, false)
		}
	}

	// Explicitly check canonical coordinates of each column type in plot (1;1):
	// Road intersection (0, 0), N-S road (0, 20), E-W road (20, 0),
	// Wall corners (3, 3), (46, 3), (3, 46), (46, 46),
	// North wall z=3 (10, 3), South wall z=46 (10, 46), West wall x=3 (3, 10), East wall x=46 (46, 10),
	// Plot interior corners (4, 4), (45, 4), (4, 45), (45, 45) and center (25, 25).
	explicit := []struct {
		name     string
		wx, wz   int
		y62, y63 plugin.BlockState
	}{
		{"road_intersection", 0, 0, quartz, air},
		{"road_ns", 2, 20, quartz, air},
		{"road_ew", 20, 47, quartz, air},
		{"wall_nw_corner", 3, 3, stone, slab},
		{"wall_ne_corner", 46, 3, stone, slab},
		{"wall_sw_corner", 3, 46, stone, slab},
		{"wall_se_corner", 46, 46, stone, slab},
		{"wall_north_z3", 10, 3, stone, slab},
		{"wall_south_z46", 10, 46, stone, slab},
		{"wall_west_x3", 3, 10, stone, slab},
		{"wall_east_x46", 46, 10, stone, slab},
		{"plot_nw_interior", 4, 4, grass, air},
		{"plot_ne_interior", 45, 4, grass, air},
		{"plot_sw_interior", 4, 45, grass, air},
		{"plot_se_interior", 45, 45, grass, air},
		{"plot_center", 25, 25, grass, air},
		{"plot_neg_interior", -4, -4, grass, air},
		{"wall_neg_east", -3, -4, stone, slab},
		{"road_neg", -2, -4, quartz, air},
	}
	for _, tc := range explicit {
		if got := grid.BlockAt(tc.wx, 62, tc.wz); got != tc.y62 {
			t.Errorf("%s (%d,62,%d) = %v, want %v", tc.name, tc.wx, tc.wz, got, tc.y62)
		}
		if got := grid.BlockAt(tc.wx, 63, tc.wz); got != tc.y63 {
			t.Errorf("%s (%d,63,%d) = %v, want %v", tc.name, tc.wx, tc.wz, got, tc.y63)
		}
	}
}

func TestHybridGenNonDefaultConfigs(t *testing.T) {
	air := plugin.DefaultState(registry.BlockAir)
	bedrock := plugin.DefaultState(registry.BlockBedrock)
	stone := plugin.DefaultState(registry.BlockStone)
	dirt := plugin.DefaultState(registry.BlockDirt)
	sand := plugin.DefaultState(registry.BlockSand)
	bricks := plugin.DefaultState(registry.BlockBricks)
	brickSlab := plugin.DefaultState(registry.BlockBrickSlab)
	cobble := plugin.DefaultState(registry.BlockCobblestone)

	cases := []struct {
		name          string
		plotWidth     int
		roadWidth     int
		offsetX       int
		offsetZ       int
		plotH         int
		wallH         int
		roadH         int
		minGen        int
		maxGen        int
		bedrock       bool
		placeTopBlock bool
		biome         string
		wantBiome     registry.Biome
	}{
		{
			name:      "even_road_6_custom_heights",
			plotWidth: 10, roadWidth: 6,
			plotH: 50, wallH: 54, roadH: 48, minGen: -16, maxGen: 100,
			bedrock: true, placeTopBlock: true, biome: "minecraft:plains", wantBiome: registry.BiomePlains,
		},
		{
			name:      "odd_road_5_with_offsets",
			plotWidth: 12, roadWidth: 5, offsetX: 5, offsetZ: -3,
			plotH: 40, wallH: 42, roadH: 38, minGen: 0, maxGen: 80,
			bedrock: true, placeTopBlock: true, biome: "desert", wantBiome: registry.BiomeDesert,
		},
		{
			name:      "road_width_2_adjacent_walls",
			plotWidth: 8, roadWidth: 2,
			plotH: 20, wallH: 20, roadH: 20, minGen: 0, maxGen: 64,
			bedrock: true, placeTopBlock: true, biome: "forest", wantBiome: registry.BiomeForest,
		},
		{
			name:      "road_width_1_shared_wall",
			plotWidth: 8, roadWidth: 1,
			plotH: 20, wallH: 20, roadH: 20, minGen: 0, maxGen: 64,
			bedrock: true, placeTopBlock: true, biome: "forest", wantBiome: registry.BiomeForest,
		},
		{
			name:      "road_width_0_no_roads_or_walls",
			plotWidth: 8, roadWidth: 0,
			plotH: 20, wallH: 20, roadH: 20, minGen: 0, maxGen: 64,
			bedrock: true, placeTopBlock: true, biome: "forest", wantBiome: registry.BiomeForest,
		},
		{
			name:      "no_bedrock_no_wall_slab",
			plotWidth: 10, roadWidth: 4,
			plotH: 15, wallH: 17, roadH: 14, minGen: 0, maxGen: 64,
			bedrock: false, placeTopBlock: false, biome: "plains", wantBiome: registry.BiomePlains,
		},
		{
			name:      "wall_height_clamped_to_maxGen_minus_1",
			plotWidth: 10, roadWidth: 5,
			plotH: 30, wallH: 30, roadH: 30, minGen: 0, maxGen: 30,
			bedrock: true, placeTopBlock: true, biome: "forest", wantBiome: registry.BiomeForest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			area := plots.DefaultPlotArea("custom_"+tc.name, "")
			area.PlotWidth = tc.plotWidth
			area.RoadWidth = tc.roadWidth
			area.RoadOffsetX = tc.offsetX
			area.RoadOffsetZ = tc.offsetZ
			area.PlotHeight = tc.plotH
			area.WallHeight = tc.wallH
			area.RoadHeight = tc.roadH
			area.MinGenHeight = tc.minGen
			area.MaxGenHeight = tc.maxGen
			area.PlotBedrock = tc.bedrock
			area.PlaceTopBlock = tc.placeTopBlock
			area.PlotBiome = tc.biome
			area.MainBlock = plots.MustBlockBucket("dirt")
			area.TopBlock = plots.MustBlockBucket("sand")
			area.WallFilling = plots.MustBlockBucket("bricks")
			area.WallBlock = plots.MustBlockBucket("brick_slab")
			area.RoadBlock = plots.MustBlockBucket("cobblestone")
			area.ComputeGeometry()

			gen := NewHybridGenerator(area, "", nil)
			grid := newChunkGrid(gen, -64, 384)

			size := tc.plotWidth + tc.roadWidth
			startY := tc.minGen
			if tc.bedrock {
				startY++
			}
			effWallH := tc.wallH
			if tc.placeTopBlock && effWallH >= tc.maxGen {
				effWallH = tc.maxGen - 1
			}

			for wx := -size; wx < size*2; wx++ {
				for wz := -size; wz < size*2; wz++ {
					kind := upstreamColumnKind(wx, wz, tc.plotWidth, tc.roadWidth, tc.offsetX, tc.offsetZ)
					if tc.bedrock {
						if got := grid.BlockAt(wx, tc.minGen, wz); got != bedrock {
							t.Fatalf("(%d,%d,%d) [%s] = %v, want bedrock", wx, tc.minGen, wz, kind, got)
						}
					}
					if got := grid.BiomeAt(wx, tc.plotH, wz); got != tc.wantBiome {
						t.Fatalf("biome(%d,%d,%d) = %v, want %v", wx, tc.plotH, wz, got, tc.wantBiome)
					}
					switch kind {
					case "plot":
						if got := grid.BlockAt(wx, startY, wz); got != dirt {
							t.Fatalf("plot(%d,%d,%d) = %v, want dirt", wx, startY, wz, got)
						}
						if got := grid.BlockAt(wx, tc.plotH-1, wz); got != dirt {
							t.Fatalf("plot(%d,%d,%d) = %v, want dirt", wx, tc.plotH-1, wz, got)
						}
						if got := grid.BlockAt(wx, tc.plotH, wz); got != sand {
							t.Fatalf("plot(%d,%d,%d) = %v, want sand", wx, tc.plotH, wz, got)
						}
						if got := grid.BlockAt(wx, tc.plotH+1, wz); got != air {
							t.Fatalf("plot(%d,%d,%d) = %v, want air", wx, tc.plotH+1, wz, got)
						}
					case "wall":
						if got := grid.BlockAt(wx, startY, wz); got != bricks {
							t.Fatalf("wall(%d,%d,%d) = %v, want bricks", wx, startY, wz, got)
						}
						if got := grid.BlockAt(wx, effWallH, wz); got != bricks {
							t.Fatalf("wall(%d,%d,%d) = %v, want bricks", wx, effWallH, wz, got)
						}
						if tc.placeTopBlock {
							if got := grid.BlockAt(wx, effWallH+1, wz); got != brickSlab {
								t.Fatalf("wall(%d,%d,%d) = %v, want brick_slab", wx, effWallH+1, wz, got)
							}
							if got := grid.BlockAt(wx, effWallH+2, wz); got != air {
								t.Fatalf("wall(%d,%d,%d) = %v, want air", wx, effWallH+2, wz, got)
							}
						} else if got := grid.BlockAt(wx, effWallH+1, wz); got != air {
							t.Fatalf("wall(%d,%d,%d) = %v, want air (placeTopBlock=false)", wx, effWallH+1, wz, got)
						}
					case "road":
						if got := grid.BlockAt(wx, startY, wz); got != cobble {
							t.Fatalf("road(%d,%d,%d) = %v, want cobblestone", wx, startY, wz, got)
						}
						if got := grid.BlockAt(wx, tc.roadH, wz); got != cobble {
							t.Fatalf("road(%d,%d,%d) = %v, want cobblestone", wx, tc.roadH, wz, got)
						}
						if got := grid.BlockAt(wx, tc.roadH+1, wz); got != air {
							t.Fatalf("road(%d,%d,%d) = %v, want air", wx, tc.roadH+1, wz, got)
						}
					}
				}
			}
		})
	}

	// Verify weighted BlockBucket produces both block types in plot interior.
	t.Run("weighted_pattern", func(t *testing.T) {
		area := plots.DefaultPlotArea("weighted", "")
		area.PlotWidth = 10
		area.RoadWidth = 5
		area.PlotHeight = 10
		area.RoadHeight = 10
		area.WallHeight = 10
		area.TopBlock = plots.MustBlockBucket("50%stone,50%cobblestone")
		area.ComputeGeometry()
		gen := NewHybridGenerator(area, "", nil)
		grid := newChunkGrid(gen, 0, 64)

		var sawStone, sawCobble int
		for wx := 3; wx <= 12; wx++ {
			for wz := 3; wz <= 12; wz++ {
				st := grid.BlockAt(wx, 10, wz)
				switch st {
				case stone:
					sawStone++
				case cobble:
					sawCobble++
				default:
					t.Fatalf("unexpected block at (%d,10,%d): %v", wx, wz, st)
				}
			}
		}
		if sawStone == 0 || sawCobble == 0 {
			t.Fatalf("expected mix of stone and cobblestone, got stone=%d cobble=%d", sawStone, sawCobble)
		}
	})
}

func TestHybridGenRoadAndPlotSchematics(t *testing.T) {
	dataDir := t.TempDir()
	area := plots.DefaultPlotArea("schemworld", "")
	area.PlotWidth = 10
	area.RoadWidth = 5 // Size=15, shift=2, oddShift=1; lower=2, upper=13; road={0,1,14}, wall={2,13}, plot={3..12}
	area.PlotHeight = 20
	area.RoadHeight = 20
	area.WallHeight = 20
	area.ComputeGeometry()

	schemDir := filepath.Join(dataDir, "schematics", "GEN_ROAD_SCHEMATIC", area.WorldName)
	if err := os.MkdirAll(schemDir, 0o755); err != nil {
		t.Fatal(err)
	}

	gold := plugin.DefaultState(registry.BlockGoldBlock)
	diamond := plugin.DefaultState(registry.BlockDiamondBlock)
	emerald := plugin.DefaultState(registry.BlockEmeraldBlock)
	stairsEast, err := block.ParseState("oak_stairs[facing=east]", true)
	if err != nil {
		t.Fatalf("ParseState stairs east: %v", err)
	}
	stairsNorth, err := block.ParseState("oak_stairs[facing=north]", true)
	if err != nil {
		t.Fatalf("ParseState stairs north: %v", err)
	}

	// sideroad.schem: width=RoadWidth (5), length=PlotWidth (10), height=2
	// y=0 is gold_block across the road+walls; y=1 at x=2 (road center), z=0 has oak_stairs[facing=east].
	sideCB := westorage.NewClipboard(region.NewCuboid(
		plugin.BlockPos{X: 0, Y: 0, Z: 0},
		plugin.BlockPos{X: 4, Y: 1, Z: 9},
	), plugin.BlockPos{})
	for x := range 5 {
		for z := range 10 {
			sideCB.SetRel(x, 0, z, gold)
		}
	}
	sideCB.SetRel(2, 1, 0, stairsEast)
	if err := westorage.SaveSchematicFile(filepath.Join(schemDir, "sideroad.schem"), sideCB, westorage.FormatFastV3); err != nil {
		t.Fatalf("save sideroad.schem: %v", err)
	}

	// intersection.schem: 5x5x1 diamond_block
	interCB := westorage.NewClipboard(region.NewCuboid(
		plugin.BlockPos{X: 0, Y: 0, Z: 0},
		plugin.BlockPos{X: 4, Y: 0, Z: 4},
	), plugin.BlockPos{})
	for x := range 5 {
		for z := range 5 {
			interCB.SetRel(x, 0, z, diamond)
		}
	}
	if err := westorage.SaveSchematicFile(filepath.Join(schemDir, "intersection.schem"), interCB, westorage.FormatFastV3); err != nil {
		t.Fatalf("save intersection.schem: %v", err)
	}

	// plot.schem: 4x4x2 emerald_block centerpiece (will be centered in the 10x10 plot: centerShift=(10-4)/2=3 -> rel 6..9)
	plotCB := westorage.NewClipboard(region.NewCuboid(
		plugin.BlockPos{X: 0, Y: 0, Z: 0},
		plugin.BlockPos{X: 3, Y: 1, Z: 3},
	), plugin.BlockPos{})
	for x := range 4 {
		for z := range 4 {
			plotCB.SetRel(x, 0, z, emerald)
			plotCB.SetRel(x, 1, z, emerald)
		}
	}
	if err := westorage.SaveSchematicFile(filepath.Join(schemDir, "plot.schem"), plotCB, westorage.FormatFastV3); err != nil {
		t.Fatalf("save plot.schem: %v", err)
	}

	settings := config.DefaultSettings()
	gen := NewHybridGenerator(area, dataDir, settings)
	if !area.RoadSchematicEnabled || !area.PlotSchematicEnabled {
		t.Fatalf("expected both road and plot schematics enabled, got road=%v plot=%v", area.RoadSchematicEnabled, area.PlotSchematicEnabled)
	}

	grid := newChunkGrid(gen, 0, 64)

	// 1. Unrotated sideroad along Z: x=0..4 -> rx=(x-2) mod 15 = {13,14,0,1,2}; z=0..9 -> rz=(z+3) = 3..12.
	//    At (rx=0, rz=3): y=20 is gold_block, y=21 is oak_stairs[facing=east].
	if got := grid.BlockAt(0, 20, 3); got != gold {
		t.Errorf("unrotated sideroad (0,20,3) = %v, want gold_block", got)
	}
	if got := grid.BlockAt(0, 21, 3); got != stairsEast {
		t.Errorf("unrotated sideroad stair (0,21,3) = %v, want oak_stairs[facing=east] (%v)", got, stairsEast)
	}
	// Wall columns covered by sideroad schematic must NOT get the default slab when road schematic is active.
	if got := grid.BlockAt(2, 20, 5); got != gold {
		t.Errorf("sideroad wall column (2,20,5) = %v, want gold_block", got)
	}
	if got := grid.BlockAt(2, 21, 5); got != plugin.DefaultState(registry.BlockAir) {
		t.Errorf("sideroad wall column top (2,21,5) = %v, want air (no default slab when road schem active)", got)
	}

	// 2. Rotated sideroad along X:
	//    x=2, z=0 -> rx2 = z + shift + oddShift = 3, rz2 = shift - x + (oddShift - 1) = 2 - 2 + 0 = 0.
	//    In upstream HybridPlotWorld (AffineTransform.rotateY(90)), East rotates CCW to North!
	if got := grid.BlockAt(3, 20, 0); got != gold {
		t.Errorf("rotated sideroad (3,20,0) = %v, want gold_block", got)
	}
	if got := grid.BlockAt(3, 21, 0); got != stairsNorth {
		t.Errorf("rotated sideroad stair (3,21,0) = %v, want oak_stairs[facing=north] (%v)", got, stairsNorth)
	}

	// 3. Intersection: rx in {13,14,0,1,2}, rz in {13,14,0,1,2} -> diamond_block at y=20.
	for _, coord := range [][2]int{{0, 0}, {1, 1}, {2, 2}, {13, 13}, {14, 0}, {0, 14}} {
		if got := grid.BlockAt(coord[0], 20, coord[1]); got != diamond {
			t.Errorf("intersection (%d,20,%d) = %v, want diamond_block", coord[0], coord[1], got)
		}
	}

	// 4. Plot schematic centered in plot (1;1) interior (3..12):
	//    centerShift = (10-4)/2 = 3 -> emerald_block at rel 6..9, y=20 and y=21; grass_block at (3,20,3).
	if got := grid.BlockAt(3, 20, 3); got != plugin.DefaultState(registry.BlockGrassBlock) {
		t.Errorf("plot corner outside plot.schem (3,20,3) = %v, want grass_block", got)
	}
	for wx := 6; wx <= 9; wx++ {
		for wz := 6; wz <= 9; wz++ {
			if got := grid.BlockAt(wx, 20, wz); got != emerald {
				t.Errorf("plot.schem (%d,20,%d) = %v, want emerald_block", wx, wz, got)
			}
			if got := grid.BlockAt(wx, 21, wz); got != emerald {
				t.Errorf("plot.schem (%d,21,%d) = %v, want emerald_block", wx, wz, got)
			}
		}
	}
}

func TestApplyGeneratorSpec(t *testing.T) {
	area := plots.DefaultPlotArea("specworld", "")
	spec := "s=32,g=5,h=40,minh=-16,maxh=128,f=50%sand,50%sandstone,m=dirt,w=bricks,b=brick_slab"
	if err := ApplyGeneratorSpec(area, spec); err != nil {
		t.Fatalf("ApplyGeneratorSpec: %v", err)
	}
	if area.PlotWidth != 32 || area.RoadWidth != 5 || area.Size != 37 {
		t.Errorf("geometry = (%d, %d, %d), want (32, 5, 37)", area.PlotWidth, area.RoadWidth, area.Size)
	}
	if area.PlotHeight != 40 || area.RoadHeight != 40 || area.WallHeight != 40 {
		t.Errorf("heights = (%d, %d, %d), want 40", area.PlotHeight, area.RoadHeight, area.WallHeight)
	}
	if area.MinGenHeight != -16 || area.MaxGenHeight != 128 {
		t.Errorf("gen bounds = (%d, %d), want (-16, 128)", area.MinGenHeight, area.MaxGenHeight)
	}
	if len(area.TopBlock.Entries) != 2 {
		t.Errorf("TopBlock entries = %+v, want 2 weighted entries", area.TopBlock.Entries)
	}
	if area.MainBlock.String() != "dirt" || area.WallFilling.String() != "bricks" || area.WallBlock.String() != "brick_slab" {
		t.Errorf("buckets = (%s, %s, %s)", area.MainBlock, area.WallFilling, area.WallBlock)
	}

	if err := ApplyGeneratorSpec(area, "unknown=5"); err == nil {
		t.Errorf("expected error for unknown generator argument")
	}
	if err := ApplyGeneratorSpec(area, "s=0"); err == nil {
		t.Errorf("expected error for size=0")
	}
}
