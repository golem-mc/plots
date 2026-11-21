package generator

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/config"
	"github.com/golem-mc/worldedit/block"
	westorage "github.com/golem-mc/worldedit/storage"
)

// schemCell stores one column of overlay blocks relative to SchemY.
type schemCell struct {
	blocks []plugin.BlockState
	set    []bool
}

// HybridGenerator implements plugin.Generator for a HybridPlotWorld/ClassicPlotWorld.
// It is safe for concurrent chunk generation calls from golem's generation workers.
type HybridGenerator struct {
	mu sync.RWMutex

	Area *plots.PlotArea

	mainPat   CompiledPattern
	topPat    CompiledPattern
	wallFill  CompiledPattern
	wallTop   CompiledPattern
	roadPat   CompiledPattern
	plotBiome registry.Biome

	// Schematic overlay cache (matching HybridPlotWorld.G_SCH / G_SCH_B).
	roadSchemEnabled bool
	plotSchemEnabled bool
	pasteOnTop       bool
	pasteRoadOnTop   bool
	schemY           int
	plotY            int
	roadY            int
	overlayBlocks    map[int64]*schemCell
	overlayBiomes    map[int64]registry.Biome
}

// NewHybridGenerator builds a thread-safe HybridGenerator for area.
func NewHybridGenerator(area *plots.PlotArea, dataDir string, settings *config.Settings) *HybridGenerator {
	g := &HybridGenerator{Area: area}
	g.Reload(dataDir, settings)
	return g
}

// Reload recompiles block patterns and reloads any road/plot schematics from disk.
func (g *HybridGenerator) Reload(dataDir string, settings *config.Settings) {
	a := g.Area
	a.ComputeGeometry()

	mainPat := CompileBucket(a.MainBlock, registry.BlockStone)
	topPat := CompileBucket(a.TopBlock, registry.BlockGrassBlock)
	wallFill := CompileBucket(a.WallFilling, registry.BlockStone)
	wallTop := CompileBucket(a.WallBlock, registry.BlockSmoothStoneSlab)
	roadPat := CompileBucket(a.RoadBlock, registry.BlockQuartzBlock)
	biome, _ := ResolveBiome(a.PlotBiome)

	pasteOnTop := true
	pasteRoadOnTop := true
	useWallHeight := true
	schemFolder := "schematics"
	if settings != nil {
		pasteOnTop = settings.Schematics.PasteOnTop
		pasteRoadOnTop = settings.Schematics.PasteRoadOnTop
		useWallHeight = settings.Schematics.UseWallInRoadSchemHigh
		if settings.Paths.UseSchematicsPathForGenSchem && settings.Paths.Schematics != "" {
			schemFolder = settings.Paths.Schematics
		}
	}

	overlayBlocks := make(map[int64]*schemCell)
	overlayBiomes := make(map[int64]registry.Biome)
	var roadSchemEnabled, plotSchemEnabled bool
	schemY := schematicStartHeight(a, useWallHeight)
	plotY := a.PlotHeight - schemY
	minRoadWall := a.RoadHeight
	if useWallHeight && a.WallHeight < minRoadWall {
		minRoadWall = a.WallHeight
	}
	roadY := minRoadWall - schemY

	if dataDir != "" {
		root := findGenSchematicDir(dataDir, schemFolder, a.WorldName, a.ID)
		if root != "" {
			s1 := loadOptionalSchematic(root, "sideroad")
			s2 := loadOptionalSchematic(root, "intersection")
			s3 := loadOptionalSchematic(root, "plot")

			worldGenHeight := a.MaxGenHeight - a.MinGenHeight + 1
			var plotSchemH, roadSchemH int
			if s3 != nil {
				plotSchemH = s3.Height
				if plotSchemH == worldGenHeight {
					schemY = a.MinGenHeight
					plotY = 0
				} else if !pasteOnTop {
					schemY = a.MinBuildHeight
					plotY = 0
				}
			}
			if s1 != nil || s2 != nil {
				if s1 != nil {
					roadSchemH = max(roadSchemH, s1.Height)
				}
				if s2 != nil {
					roadSchemH = max(roadSchemH, s2.Height)
				}
				if roadSchemH == worldGenHeight || !pasteRoadOnTop {
					schemY = a.MinGenHeight
					roadY = 0
					if s3 != nil && s3.Height != worldGenHeight {
						basePlotY := a.MinBuildHeight
						if pasteOnTop {
							basePlotY = a.PlotHeight
						}
						plotY = basePlotY - schemY
					}
				} else {
					roadY = minRoadWall - schemY
				}
			}
			maxH := max(plotY+plotSchemH, roadY+roadSchemH)
			shift := a.RoadWidth / 2
			oddShift := a.RoadWidth & 1

			if s3 != nil && maxH > 0 {
				plotSchemEnabled = true
				if s3.Width > a.PlotWidth || s3.Length > a.PlotWidth {
					roadSchemEnabled = true
				}
				centerX := (a.PlotWidth - s3.Width) / 2
				centerZ := (a.PlotWidth - s3.Length) / 2
				for x := 0; x < s3.Width; x++ {
					for z := 0; z < s3.Length; z++ {
						rx := plots.FloorMod(x+shift+oddShift+centerX, a.Size)
						rz := plots.FloorMod(z+shift+oddShift+centerZ, a.Size)
						for y := 0; y < s3.Height; y++ {
							st := s3.GetRel(x, y, z)
							addOverlay(overlayBlocks, rx, y+plotY, rz, st, maxH)
						}
						if len(s3.Biomes) > 0 {
							overlayBiomes[pairXZ(rx, rz)] = s3.GetRelBiome(x, 0, z)
						}
					}
				}
			}

			if (s1 != nil || s2 != nil) && a.RoadWidth > 0 && maxH > 0 {
				roadSchemEnabled = true
				if s1 != nil {
					// Sideroad: unrotated along Z, and rotated 90 deg CCW (270 deg CW,
					// matching upstream WorldEdit AffineTransform.rotateY(90)) for the perpendicular road.
					for x := 0; x < s1.Width; x++ {
						for z := 0; z < s1.Length; z++ {
							rx1 := plots.FloorMod(x-shift, a.Size)
							rz1 := plots.FloorMod(z+shift+oddShift, a.Size)
							rx2 := plots.FloorMod(z+shift+oddShift, a.Size)
							rz2 := plots.FloorMod(shift-x+(oddShift-1), a.Size)
							for y := 0; y < s1.Height; y++ {
								st := s1.GetRel(x, y, z)
								addOverlay(overlayBlocks, rx1, y+roadY, rz1, st, maxH)
								addOverlay(overlayBlocks, rx2, y+roadY, rz2, block.RotateStateY(st, 270), maxH)
							}
							if len(s1.Biomes) > 0 {
								b := s1.GetRelBiome(x, 0, z)
								overlayBiomes[pairXZ(rx1, rz1)] = b
								overlayBiomes[pairXZ(rx2, rz2)] = b
							}
						}
					}
				}
				if s2 != nil {
					// Intersection
					for x := 0; x < s2.Width; x++ {
						for z := 0; z < s2.Length; z++ {
							rx := plots.FloorMod(x-shift, a.Size)
							rz := plots.FloorMod(z-shift, a.Size)
							for y := 0; y < s2.Height; y++ {
								st := s2.GetRel(x, y, z)
								addOverlay(overlayBlocks, rx, y+roadY, rz, st, maxH)
							}
							if len(s2.Biomes) > 0 {
								overlayBiomes[pairXZ(rx, rz)] = s2.GetRelBiome(x, 0, z)
							}
						}
					}
				}
			}
		}
	}

	g.mu.Lock()
	g.mainPat = mainPat
	g.topPat = topPat
	g.wallFill = wallFill
	g.wallTop = wallTop
	g.roadPat = roadPat
	g.plotBiome = biome
	g.roadSchemEnabled = roadSchemEnabled
	g.plotSchemEnabled = plotSchemEnabled
	g.pasteOnTop = pasteOnTop
	g.pasteRoadOnTop = pasteRoadOnTop
	g.schemY = schemY
	g.plotY = plotY
	g.roadY = roadY
	g.overlayBlocks = overlayBlocks
	g.overlayBiomes = overlayBiomes
	a.RoadSchematicEnabled = roadSchemEnabled
	a.PlotSchematicEnabled = plotSchemEnabled
	g.mu.Unlock()
}

// Generate fills a new chunk's blocks and biomes (plugin.Generator).
// Mirrors upstream HybridGen.generateChunk block-for-block.
func (g *HybridGenerator) Generate(c plugin.ChunkWriter, rng plugin.Random) {
	g.mu.RLock()
	a := g.Area
	mainPat := g.mainPat
	topPat := g.topPat
	wallFill := g.wallFill
	wallTop := g.wallTop
	roadPat := g.roadPat
	plotBiome := g.plotBiome
	roadSchem := g.roadSchemEnabled
	plotSchem := g.plotSchemEnabled
	pasteOnTop := g.pasteOnTop
	pasteRoadOnTop := g.pasteRoadOnTop
	schemY := g.schemY
	overlayBlocks := g.overlayBlocks
	overlayBiomes := g.overlayBiomes
	g.mu.RUnlock()

	if a.Size <= 0 {
		return
	}

	cMinY := c.MinY()
	cMaxY := cMinY + c.Height() - 1
	minGenY := max(a.MinGenHeight, cMinY)
	maxGenY := min(a.MaxGenHeight, cMaxY)

	// Fill default biome across the chunk (4x4x4 cells).
	for x := 0; x < 16; x += 4 {
		for z := 0; z < 16; z += 4 {
			for y := cMinY; y <= cMaxY; y += 4 {
				c.SetBiome(x, y, z, plotBiome)
			}
		}
	}

	pos := c.Pos()
	baseX := pos.X << 4
	baseZ := pos.Z << 4

	// Compute relative coordinates and road/wall masks for the 16x16 chunk.
	var relX, relZ [16]int
	var inRoadX, inWallX, inRoadZ, inWallZ [16]bool
	offX := plots.FloorMod(baseX-a.RoadOffsetX, a.Size)
	offZ := plots.FloorMod(baseZ-a.RoadOffsetZ, a.Size)
	for i := range 16 {
		if offX >= a.Size {
			offX -= a.Size
		}
		relX[i] = offX
		if a.RoadWidth != 0 {
			inRoadX[i] = offX < a.PathWidthLower || offX > a.PathWidthUpper
			inWallX[i] = offX == a.PathWidthLower || offX == a.PathWidthUpper
		}
		offX++

		if offZ >= a.Size {
			offZ -= a.Size
		}
		relZ[i] = offZ
		if a.RoadWidth != 0 {
			inRoadZ[i] = offZ < a.PathWidthLower || offZ > a.PathWidthUpper
			inWallZ[i] = offZ == a.PathWidthLower || offZ == a.PathWidthUpper
		}
		offZ++
	}

	bedrock := plugin.DefaultState(registry.BlockBedrock)
	startY := minGenY
	if a.PlotBedrock && minGenY >= cMinY && minGenY <= cMaxY {
		for x := range 16 {
			for z := range 16 {
				c.Set(x, minGenY, z, bedrock)
			}
		}
		startY = minGenY + 1
	}

	applyOverlay := func(rx, rz, lx, lz int, isRoad bool) {
		minY := a.MinBuildHeight
		if (isRoad && pasteRoadOnTop) || (!isRoad && pasteOnTop) {
			minY = schemY
		}
		key := pairXZ(rx, rz)
		if cell := overlayBlocks[key]; cell != nil {
			for y, st := range cell.blocks {
				if cell.set[y] {
					wy := minY + y
					if wy >= cMinY && wy <= cMaxY {
						c.Set(lx, wy, lz, st)
					}
				}
			}
		}
		if b, ok := overlayBiomes[key]; ok {
			for y := cMinY; y <= cMaxY; y += 4 {
				c.SetBiome(lx, y, lz, b)
			}
		}
	}

	roadH := min(a.RoadHeight, maxGenY)
	wallMaxY := maxGenY
	if a.PlaceTopBlock {
		wallMaxY = maxGenY - 1
	}
	wallH := min(a.WallHeight, wallMaxY)
	plotH := min(a.PlotHeight, maxGenY)

	for x := range 16 {
		for z := range 16 {
			isRoad := inRoadX[x] || inRoadZ[z]
			isWall := !isRoad && (inWallX[x] || inWallZ[z])

			switch {
			case isRoad:
				for y := startY; y <= roadH; y++ {
					c.Set(x, y, z, roadPat.Pick(rng))
				}
				if roadSchem {
					applyOverlay(relX[x], relZ[z], x, z, true)
				}
			case isWall:
				for y := startY; y <= wallH; y++ {
					c.Set(x, y, z, wallFill.Pick(rng))
				}
				if !roadSchem {
					if a.PlaceTopBlock && wallH+1 <= maxGenY {
						c.Set(x, wallH+1, z, wallTop.Pick(rng))
					}
				} else {
					applyOverlay(relX[x], relZ[z], x, z, true)
				}
			default:
				for y := startY; y < plotH; y++ {
					c.Set(x, y, z, mainPat.Pick(rng))
				}
				if plotH >= cMinY && plotH <= cMaxY {
					c.Set(x, plotH, z, topPat.Pick(rng))
				}
				if plotSchem {
					applyOverlay(relX[x], relZ[z], x, z, false)
				}
			}
		}
	}
}

// Populate is called right after Generate (plugin.Generator).
func (g *HybridGenerator) Populate(_ plugin.RegionWriter, _ plugin.ChunkPos, _ plugin.Random) {}

// SpawnPoint returns the spawn location on the road at (0, RoadHeight+1, 0).
func (g *HybridGenerator) SpawnPoint(w plugin.World) plugin.Location {
	g.mu.RLock()
	a := g.Area
	g.mu.RUnlock()
	return plugin.Location{
		World: w,
		Pos:   plugin.Vec3{X: 0.5, Y: float64(a.RoadHeight + 1), Z: 0.5},
	}
}

// AugmentChunk writes plot walls/roads/plots into a newly loaded chunk for
// AUGMENTED or PARTIAL areas (matching BukkitAugmentedGenerator).
func (g *HybridGenerator) AugmentChunk(w plugin.World, chunk plugin.Chunk) {
	g.mu.RLock()
	a := g.Area
	mainPat := g.mainPat
	topPat := g.topPat
	wallFill := g.wallFill
	wallTop := g.wallTop
	roadPat := g.roadPat
	plotBiome := g.plotBiome
	g.mu.RUnlock()

	if a.Size <= 0 || a.Type == plots.AreaNormal {
		return
	}
	pos := chunk.Pos()
	baseX := pos.X << 4
	baseZ := pos.Z << 4
	air := plugin.DefaultState(registry.BlockAir)
	bedrock := plugin.DefaultState(registry.BlockBedrock)
	startY := a.MinGenHeight
	if a.PlotBedrock {
		startY++
	}

	for lx := range 16 {
		wx := baseX + lx
		for lz := range 16 {
			wz := baseZ + lz
			if !a.ContainsXZ(wx, wz) {
				continue
			}
			rx := plots.FloorMod(wx-a.RoadOffsetX, a.Size)
			rz := plots.FloorMod(wz-a.RoadOffsetZ, a.Size)
			isRoad := a.RoadWidth > 0 && (rx < a.PathWidthLower || rx > a.PathWidthUpper || rz < a.PathWidthLower || rz > a.PathWidthUpper)
			isWall := a.RoadWidth > 0 && !isRoad && (rx == a.PathWidthLower || rx == a.PathWidthUpper || rz == a.PathWidthLower || rz == a.PathWidthUpper)

			// Respect terrain preservation mode
			switch a.Terrain {
			case plots.TerrainAll:
				continue
			case plots.TerrainRoad:
				if !isRoad && !isWall {
					continue
				}
			case plots.TerrainOre:
				if !isRoad && !isWall {
					continue
				}
			}

			if a.PlotBedrock {
				w.SetBlockState(plugin.BlockPos{X: wx, Y: a.MinGenHeight, Z: wz}, bedrock, plugin.UpdateClients)
			}
			switch {
			case isRoad:
				for y := startY; y <= a.RoadHeight; y++ {
					w.SetBlockState(plugin.BlockPos{X: wx, Y: y, Z: wz}, roadPat.PickCoord(wx, y, wz), plugin.UpdateClients)
				}
				for y := a.RoadHeight + 1; y <= a.MaxGenHeight; y++ {
					p := plugin.BlockPos{X: wx, Y: y, Z: wz}
					if w.BlockState(p) != air {
						w.SetBlockState(p, air, plugin.UpdateClients)
					}
				}
			case isWall:
				for y := startY; y <= a.WallHeight; y++ {
					w.SetBlockState(plugin.BlockPos{X: wx, Y: y, Z: wz}, wallFill.PickCoord(wx, y, wz), plugin.UpdateClients)
				}
				if a.PlaceTopBlock {
					w.SetBlockState(plugin.BlockPos{X: wx, Y: a.WallHeight + 1, Z: wz}, wallTop.PickCoord(wx, a.WallHeight+1, wz), plugin.UpdateClients)
				}
				for y := a.WallHeight + 2; y <= a.MaxGenHeight; y++ {
					p := plugin.BlockPos{X: wx, Y: y, Z: wz}
					if w.BlockState(p) != air {
						w.SetBlockState(p, air, plugin.UpdateClients)
					}
				}
			default:
				for y := startY; y < a.PlotHeight; y++ {
					w.SetBlockState(plugin.BlockPos{X: wx, Y: y, Z: wz}, mainPat.PickCoord(wx, y, wz), plugin.UpdateClients)
				}
				w.SetBlockState(plugin.BlockPos{X: wx, Y: a.PlotHeight, Z: wz}, topPat.PickCoord(wx, a.PlotHeight, wz), plugin.UpdateClients)
				for y := a.PlotHeight + 1; y <= a.MaxGenHeight; y++ {
					p := plugin.BlockPos{X: wx, Y: y, Z: wz}
					if w.BlockState(p) != air {
						w.SetBlockState(p, air, plugin.UpdateClients)
					}
				}
			}
			if lx%4 == 0 && lz%4 == 0 {
				w.SetBiome(plugin.BlockPos{X: wx, Y: a.PlotHeight, Z: wz}, plotBiome)
			}
		}
	}
}

// ApplySchematicOverlay applies the generator's road or plot schematic overlay to a live world box.
func (g *HybridGenerator) ApplySchematicOverlay(w plugin.World, minP, maxP plugin.BlockPos, isRoad bool) {
	g.mu.RLock()
	a := g.Area
	enabled := (isRoad && g.roadSchemEnabled) || (!isRoad && g.plotSchemEnabled)
	pasteOnTop := g.pasteOnTop
	pasteRoadOnTop := g.pasteRoadOnTop
	schemY := g.schemY
	overlayBlocks := g.overlayBlocks
	overlayBiomes := g.overlayBiomes
	g.mu.RUnlock()

	if !enabled || a.Size <= 0 {
		return
	}
	minY := a.MinBuildHeight
	if (isRoad && pasteRoadOnTop) || (!isRoad && pasteOnTop) {
		minY = schemY
	}
	for x := minP.X; x <= maxP.X; x++ {
		rx := plots.FloorMod(x-a.RoadOffsetX, a.Size)
		for z := minP.Z; z <= maxP.Z; z++ {
			rz := plots.FloorMod(z-a.RoadOffsetZ, a.Size)
			key := pairXZ(rx, rz)
			if cell := overlayBlocks[key]; cell != nil {
				for y, st := range cell.blocks {
					if cell.set[y] {
						w.SetBlockState(plugin.BlockPos{X: x, Y: minY + y, Z: z}, st, plugin.UpdateClients)
					}
				}
			}
			if b, ok := overlayBiomes[key]; ok {
				w.SetBiome(plugin.BlockPos{X: x, Y: a.PlotHeight, Z: z}, b)
			}
		}
	}
}

func schematicStartHeight(a *plots.PlotArea, useWall bool) int {
	m := min(a.PlotHeight, a.RoadHeight)
	if useWall && a.WallHeight < m {
		return a.WallHeight
	}
	return m
}

func findGenSchematicDir(dataDir, schemFolder, worldName, areaID string) string {
	if areaID != "" {
		p := filepath.Join(dataDir, schemFolder, "GEN_ROAD_SCHEMATIC", worldName, areaID)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	p := filepath.Join(dataDir, schemFolder, "GEN_ROAD_SCHEMATIC", worldName)
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p
	}
	return ""
}

func loadOptionalSchematic(dir, base string) *westorage.Clipboard {
	for _, ext := range []string{".schem", ".schematic"} {
		path := filepath.Join(dir, base+ext)
		if _, err := os.Stat(path); err == nil {
			cb, err := westorage.ReadSchematicFile(path, "")
			if err == nil {
				return cb
			}
		}
	}
	return nil
}

func pairXZ(x, z int) int64 {
	return (int64(x) << 32) | int64(uint32(z))
}

func addOverlay(m map[int64]*schemCell, rx, y, rz int, st plugin.BlockState, maxH int) {
	if y < 0 || y >= maxH {
		return
	}
	key := pairXZ(rx, rz)
	cell := m[key]
	if cell == nil {
		cell = &schemCell{
			blocks: make([]plugin.BlockState, maxH),
			set:    make([]bool, maxH),
		}
		m[key] = cell
	}
	cell.blocks[y] = st
	cell.set[y] = true
}
