package generator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/worldedit/region"
	westorage "github.com/golem-mc/worldedit/storage"
)

// ClearPlot clears all regions of plot back to default terrain (or regenerates for augmented terrain)
// and removes non-player entities inside the plot.
func ClearPlot(w plugin.World, area *plots.PlotArea, plot *plots.Plot, gen *HybridGenerator) {
	if w == nil || area == nil || plot == nil {
		return
	}
	regions := area.PlotRegions(plot)
	air := plugin.DefaultState(registry.BlockAir)
	bedrock := plugin.DefaultState(registry.BlockBedrock)
	mainPat := CompileBucket(area.MainBlock, registry.BlockStone)
	topPat := CompileBucket(area.TopBlock, registry.BlockGrassBlock)
	biome, _ := ResolveBiome(area.PlotBiome)

	minY := max(area.MinGenHeight, w.MinY())
	maxY := min(area.MaxGenHeight, w.MinY()+w.Height()-1)
	maxBuildY := min(area.MaxBuildHeight-1, w.MinY()+w.Height()-1)
	startY := minY
	if area.PlotBedrock {
		startY++
	}

	for _, reg := range regions {
		// Remove non-player entities inside the plot region.
		box := plugin.Box{
			Min: plugin.Vec3{X: float64(reg.Min.X), Y: float64(w.MinY()), Z: float64(reg.Min.Z)},
			Max: plugin.Vec3{X: float64(reg.Max.X + 1), Y: float64(w.MinY() + w.Height()), Z: float64(reg.Max.Z + 1)},
		}
		for _, ent := range w.EntitiesIn(box) {
			if _, isPlayer := ent.(plugin.Player); !isPlayer {
				ent.Remove()
			}
		}

		for x := reg.Min.X; x <= reg.Max.X; x++ {
			for z := reg.Min.Z; z <= reg.Max.Z; z++ {
				if area.PlotBedrock {
					w.SetBlockState(plugin.BlockPos{X: x, Y: minY, Z: z}, bedrock, plugin.UpdateClients)
				}
				for y := startY; y < area.PlotHeight && y <= maxY; y++ {
					w.SetBlockState(plugin.BlockPos{X: x, Y: y, Z: z}, mainPat.PickCoord(x, y, z), plugin.UpdateClients)
				}
				if area.PlotHeight >= minY && area.PlotHeight <= maxY {
					w.SetBlockState(plugin.BlockPos{X: x, Y: area.PlotHeight, Z: z}, topPat.PickCoord(x, area.PlotHeight, z), plugin.UpdateClients)
				}
				for y := area.PlotHeight + 1; y <= max(maxY, maxBuildY); y++ {
					p := plugin.BlockPos{X: x, Y: y, Z: z}
					if w.BlockState(p) != air {
						w.SetBlockState(p, air, plugin.UpdateClients)
					}
				}
				if (x-reg.Min.X)%4 == 0 && (z-reg.Min.Z)%4 == 0 {
					for y := minY; y <= maxY; y += 4 {
						w.SetBiome(plugin.BlockPos{X: x, Y: y, Z: z}, biome)
					}
				}
			}
		}
		if gen != nil {
			gen.ApplySchematicOverlay(w, reg.Min, reg.Max, false)
		}
	}
}

// SetComponent applies a BlockBucket pattern to a plot component (floor, main, wall, border, all, air, outline, middle).
func SetComponent(w plugin.World, area *plots.PlotArea, plot *plots.Plot, comp plots.PlotComponent, bucket plots.BlockBucket) bool {
	if w == nil || area == nil || plot == nil {
		return false
	}
	pat := CompileBucket(bucket, registry.BlockStone)
	base := plot.BasePlot()
	regions := area.PlotRegions(base)
	minCompY := max(area.MinComponentHeight(), w.MinY())
	maxY := min(area.MaxBuildHeight-1, w.MinY()+w.Height()-1)

	switch comp {
	case plots.ComponentFloor:
		setCuboids(w, regions, area.PlotHeight, area.PlotHeight, pat)
		return true
	case plots.ComponentMain:
		setCuboids(w, regions, minCompY, area.PlotHeight-1, pat)
		return true
	case plots.ComponentAir:
		setCuboids(w, regions, area.PlotHeight+1, maxY, pat)
		return true
	case plots.ComponentAll:
		setCuboids(w, regions, minCompY, maxY, pat)
		return true
	case plots.ComponentMiddle:
		bMin, bMax := mergedOuterBounds(area, base)
		mx := (bMin.X + bMax.X) / 2
		mz := (bMin.Z + bMax.Z) / 2
		w.SetBlockState(plugin.BlockPos{X: mx, Y: area.PlotHeight, Z: mz}, pat.PickCoord(mx, area.PlotHeight, mz), plugin.UpdateClients)
		return true
	case plots.ComponentWall:
		return setWalls(w, area, base, minCompY, area.WallHeight, pat)
	case plots.ComponentBorder:
		return setWalls(w, area, base, area.WallHeight+1, area.WallHeight+1, pat)
	case plots.ComponentOutline:
		return setOutline(w, area, base, area.PlotHeight, maxY, pat)
	default:
		return false
	}
}

func setCuboids(w plugin.World, regions []plots.RegionBounds, yMin, yMax int, pat CompiledPattern) {
	if yMin > yMax {
		return
	}
	for _, r := range regions {
		for x := r.Min.X; x <= r.Max.X; x++ {
			for z := r.Min.Z; z <= r.Max.Z; z++ {
				for y := yMin; y <= yMax; y++ {
					w.SetBlockState(plugin.BlockPos{X: x, Y: y, Z: z}, pat.PickCoord(x, y, z), plugin.UpdateClients)
				}
			}
		}
	}
}

func setWalls(w plugin.World, area *plots.PlotArea, base *plots.Plot, yMin, yMax int, pat CompiledPattern) bool {
	if area.RoadWidth == 0 || area.Terrain == plots.TerrainAll {
		return area.Terrain == plots.TerrainAll
	}
	if yMin > yMax {
		return true
	}
	for _, cp := range base.ConnectedPlots() {
		b := area.PlotInteriorBounds(cp.ID)
		xMin := b.Min.X
		if !cp.IsMergedDir(plots.DirWest) {
			xMin--
		}
		zMin := b.Min.Z
		if !cp.IsMergedDir(plots.DirNorth) {
			zMin--
		}
		xMax := b.Max.X + 1
		zMax := b.Max.Z + 1

		if !cp.IsMergedDir(plots.DirNorth) {
			for x := xMin; x <= xMax; x++ {
				for y := yMin; y <= yMax; y++ {
					w.SetBlockState(plugin.BlockPos{X: x, Y: y, Z: zMin}, pat.PickCoord(x, y, zMin), plugin.UpdateClients)
				}
			}
		}
		if !cp.IsMergedDir(plots.DirWest) {
			for z := zMin; z <= zMax; z++ {
				for y := yMin; y <= yMax; y++ {
					w.SetBlockState(plugin.BlockPos{X: xMin, Y: y, Z: z}, pat.PickCoord(xMin, y, z), plugin.UpdateClients)
				}
			}
		}
		if !cp.IsMergedDir(plots.DirSouth) {
			for x := xMin; x <= xMax; x++ {
				for y := yMin; y <= yMax; y++ {
					w.SetBlockState(plugin.BlockPos{X: x, Y: y, Z: zMax}, pat.PickCoord(x, y, zMax), plugin.UpdateClients)
				}
			}
		}
		if !cp.IsMergedDir(plots.DirEast) {
			for z := zMin; z <= zMax; z++ {
				for y := yMin; y <= yMax; y++ {
					w.SetBlockState(plugin.BlockPos{X: xMax, Y: y, Z: z}, pat.PickCoord(xMax, y, z), plugin.UpdateClients)
				}
			}
		}
	}
	return true
}

func setOutline(w plugin.World, area *plots.PlotArea, base *plots.Plot, yMin, yMax int, pat CompiledPattern) bool {
	if area.RoadWidth == 0 {
		return false
	}
	for _, cp := range base.ConnectedPlots() {
		b := area.PlotInteriorBounds(cp.ID)
		if !cp.IsMergedDir(plots.DirNorth) {
			for x := b.Min.X; x <= b.Max.X; x++ {
				for y := yMin; y <= yMax; y++ {
					w.SetBlockState(plugin.BlockPos{X: x, Y: y, Z: b.Min.Z}, pat.PickCoord(x, y, b.Min.Z), plugin.UpdateClients)
				}
			}
		}
		if !cp.IsMergedDir(plots.DirSouth) {
			for x := b.Min.X; x <= b.Max.X; x++ {
				for y := yMin; y <= yMax; y++ {
					w.SetBlockState(plugin.BlockPos{X: x, Y: y, Z: b.Max.Z}, pat.PickCoord(x, y, b.Max.Z), plugin.UpdateClients)
				}
			}
		}
		if !cp.IsMergedDir(plots.DirWest) {
			for z := b.Min.Z; z <= b.Max.Z; z++ {
				for y := yMin; y <= yMax; y++ {
					w.SetBlockState(plugin.BlockPos{X: b.Min.X, Y: y, Z: z}, pat.PickCoord(b.Min.X, y, z), plugin.UpdateClients)
				}
			}
		}
		if !cp.IsMergedDir(plots.DirEast) {
			for z := b.Min.Z; z <= b.Max.Z; z++ {
				for y := yMin; y <= yMax; y++ {
					w.SetBlockState(plugin.BlockPos{X: b.Max.X, Y: y, Z: z}, pat.PickCoord(b.Max.X, y, z), plugin.UpdateClients)
				}
			}
		}
	}
	return true
}

// SetPlotBiome sets the biome across all regions of plot.
func SetPlotBiome(w plugin.World, area *plots.PlotArea, plot *plots.Plot, biome registry.Biome) {
	if w == nil || area == nil || plot == nil {
		return
	}
	minY := w.MinY()
	maxY := minY + w.Height() - 1
	for _, r := range area.PlotRegions(plot) {
		for x := r.Min.X; x <= r.Max.X; x += 4 {
			for z := r.Min.Z; z <= r.Max.Z; z += 4 {
				for y := minY; y <= maxY; y += 4 {
					w.SetBiome(plugin.BlockPos{X: x, Y: y, Z: z}, biome)
				}
			}
		}
	}
}

// SignLocation returns the world block position of a plot's claim sign.
// Matches upstream HybridPlotWorld.getSignLocation: (bottom.X - 1, RoadHeight + 1, bottom.Z - 2).
func SignLocation(area *plots.PlotArea, plot *plots.Plot) plugin.BlockPos {
	base := plot.BasePlot()
	b := area.PlotInteriorBounds(base.ID)
	return plugin.BlockPos{X: b.Min.X - 1, Y: area.RoadHeight + 1, Z: b.Min.Z - 2}
}

// ClaimPlotBlocks updates a claimed plot's border wall and places a claim sign if enabled.
func ClaimPlotBlocks(w plugin.World, area *plots.PlotArea, plot *plots.Plot, ownerName string) {
	if w == nil || area == nil || plot == nil {
		return
	}
	if area.PlaceTopBlock && area.RoadWidth > 0 {
		SetComponent(w, area, plot, plots.ComponentBorder, area.ClaimedWallBlock)
	}
	if area.AllowSigns && area.RoadWidth >= 2 {
		pos := SignLocation(area, plot)
		signSt, ok := ParseBlockSpec(area.SignMaterial)
		if !ok {
			signSt = plugin.DefaultState(registry.BlockOakWallSign)
		}
		if faced, ok := signSt.With(registry.PropFacing, "north"); ok {
			signSt = faced
		}
		w.SetBlockState(pos, signSt, plugin.UpdateAll)
		if be, ok := w.BlockEntity(pos); ok {
			if sign, ok := be.(plugin.Sign); ok {
				sign.SetLines(true, [4]plugin.Text{
					plugin.Plain(fmt.Sprintf("ID: %s", plot.ID.String())),
					plugin.Plain("Claimed"),
					plugin.Plain(ownerName),
					plugin.Plain(""),
				})
			}
		}
	}
}

// UnclaimPlotBlocks restores a plot's wall/border to unclaimed blocks and removes the claim sign.
func UnclaimPlotBlocks(w plugin.World, area *plots.PlotArea, plot *plots.Plot) {
	if w == nil || area == nil || plot == nil {
		return
	}
	if area.RoadWidth > 0 {
		SetComponent(w, area, plot, plots.ComponentWall, area.WallFilling)
		if area.PlaceTopBlock {
			SetComponent(w, area, plot, plots.ComponentBorder, area.WallBlock)
		}
	}
	if area.AllowSigns && area.RoadWidth >= 2 {
		pos := SignLocation(area, plot)
		w.SetBlockState(pos, plugin.DefaultState(registry.BlockAir), plugin.UpdateAll)
	}
}

// RemoveRoadEast replaces the road strip east of id with plot filling/floor/air.
func RemoveRoadEast(w plugin.World, area *plots.PlotArea, id plots.PlotId) {
	if w == nil || area.RoadWidth <= 0 {
		return
	}
	b := area.PlotInteriorBounds(id)
	sx := b.Max.X + 1
	ex := sx + area.RoadWidth - 1
	fillRoadWithPlot(w, area, sx, ex, b.Min.Z, b.Max.Z)
}

// RemoveRoadSouth replaces the road strip south of id with plot filling/floor/air.
func RemoveRoadSouth(w plugin.World, area *plots.PlotArea, id plots.PlotId) {
	if w == nil || area.RoadWidth <= 0 {
		return
	}
	b := area.PlotInteriorBounds(id)
	sz := b.Max.Z + 1
	ez := sz + area.RoadWidth - 1
	fillRoadWithPlot(w, area, b.Min.X, b.Max.X, sz, ez)
}

// RemoveRoadSouthEast replaces the 2x2 intersection southeast of id with plot filling/floor/air.
func RemoveRoadSouthEast(w plugin.World, area *plots.PlotArea, id plots.PlotId) {
	if w == nil || area.RoadWidth <= 0 {
		return
	}
	b := area.PlotInteriorBounds(id)
	sx := b.Max.X + 1
	ex := sx + area.RoadWidth - 1
	sz := b.Max.Z + 1
	ez := sz + area.RoadWidth - 1
	fillRoadWithPlot(w, area, sx, ex, sz, ez)
}

func fillRoadWithPlot(w plugin.World, area *plots.PlotArea, sx, ex, sz, ez int) {
	air := plugin.DefaultState(registry.BlockAir)
	mainPat := CompileBucket(area.MainBlock, registry.BlockStone)
	topPat := CompileBucket(area.TopBlock, registry.BlockGrassBlock)
	minY := max(area.MinGenHeight, w.MinY())
	maxY := min(area.MaxGenHeight, w.MinY()+w.Height()-1)
	startY := minY
	if area.PlotBedrock {
		startY++
	}
	for x := sx; x <= ex; x++ {
		for z := sz; z <= ez; z++ {
			for y := startY; y < area.PlotHeight && y <= maxY; y++ {
				w.SetBlockState(plugin.BlockPos{X: x, Y: y, Z: z}, mainPat.PickCoord(x, y, z), plugin.UpdateClients)
			}
			if area.PlotHeight >= minY && area.PlotHeight <= maxY {
				w.SetBlockState(plugin.BlockPos{X: x, Y: area.PlotHeight, Z: z}, topPat.PickCoord(x, area.PlotHeight, z), plugin.UpdateClients)
			}
			for y := area.PlotHeight + 1; y <= maxY; y++ {
				p := plugin.BlockPos{X: x, Y: y, Z: z}
				if w.BlockState(p) != air {
					w.SetBlockState(p, air, plugin.UpdateClients)
				}
			}
		}
	}
}

// CreateRoadEast restores the road and walls east of id (used when unlinking plots).
func CreateRoadEast(w plugin.World, area *plots.PlotArea, id plots.PlotId, gen *HybridGenerator) {
	if w == nil || area.RoadWidth <= 0 {
		return
	}
	b := area.PlotInteriorBounds(id)
	sx := b.Max.X + 1
	ex := sx + area.RoadWidth - 1
	sz := b.Min.Z - 1
	ez := b.Max.Z + 1
	restoreRoadBox(w, area, sx, ex, sz, ez, true, false, gen)
}

// CreateRoadSouth restores the road and walls south of id.
func CreateRoadSouth(w plugin.World, area *plots.PlotArea, id plots.PlotId, gen *HybridGenerator) {
	if w == nil || area.RoadWidth <= 0 {
		return
	}
	b := area.PlotInteriorBounds(id)
	sz := b.Max.Z + 1
	ez := sz + area.RoadWidth - 1
	sx := b.Min.X - 1
	ex := b.Max.X + 1
	restoreRoadBox(w, area, sx, ex, sz, ez, false, true, gen)
}

// CreateRoadSouthEast restores the 2x2 intersection southeast of id.
func CreateRoadSouthEast(w plugin.World, area *plots.PlotArea, id plots.PlotId, gen *HybridGenerator) {
	if w == nil || area.RoadWidth <= 0 {
		return
	}
	b := area.PlotInteriorBounds(id)
	sx := b.Max.X + 1
	ex := sx + area.RoadWidth - 1
	sz := b.Max.Z + 1
	ez := sz + area.RoadWidth - 1
	restoreRoadBox(w, area, sx, ex, sz, ez, false, false, gen)
}

func writeRoadOrWallColumn(w plugin.World, area *plots.PlotArea, x, z, startY, maxY int, isWall bool, roadPat, wallFill, wallTop CompiledPattern, air plugin.BlockState) {
	if isWall {
		for y := startY; y <= area.WallHeight && y <= maxY; y++ {
			w.SetBlockState(plugin.BlockPos{X: x, Y: y, Z: z}, wallFill.PickCoord(x, y, z), plugin.UpdateClients)
		}
		topY := area.WallHeight + 1
		if area.PlaceTopBlock && topY <= maxY {
			w.SetBlockState(plugin.BlockPos{X: x, Y: topY, Z: z}, wallTop.PickCoord(x, topY, z), plugin.UpdateClients)
			topY++
		}
		for y := topY; y <= maxY; y++ {
			p := plugin.BlockPos{X: x, Y: y, Z: z}
			if w.BlockState(p) != air {
				w.SetBlockState(p, air, plugin.UpdateClients)
			}
		}
		return
	}
	for y := startY; y <= area.RoadHeight && y <= maxY; y++ {
		w.SetBlockState(plugin.BlockPos{X: x, Y: y, Z: z}, roadPat.PickCoord(x, y, z), plugin.UpdateClients)
	}
	for y := area.RoadHeight + 1; y <= maxY; y++ {
		p := plugin.BlockPos{X: x, Y: y, Z: z}
		if w.BlockState(p) != air {
			w.SetBlockState(p, air, plugin.UpdateClients)
		}
	}
}

func restoreRoadBox(w plugin.World, area *plots.PlotArea, sx, ex, sz, ez int, wallX, wallZ bool, gen *HybridGenerator) {
	air := plugin.DefaultState(registry.BlockAir)
	bedrock := plugin.DefaultState(registry.BlockBedrock)
	roadPat := CompileBucket(area.RoadBlock, registry.BlockQuartzBlock)
	wallFill := CompileBucket(area.WallFilling, registry.BlockStone)
	wallTop := CompileBucket(area.WallBlock, registry.BlockSmoothStoneSlab)

	minY := max(area.MinGenHeight, w.MinY())
	maxY := min(area.MaxGenHeight, w.MinY()+w.Height()-1)
	startY := minY
	if area.PlotBedrock {
		startY++
	}

	for x := sx; x <= ex; x++ {
		for z := sz; z <= ez; z++ {
			if area.PlotBedrock {
				w.SetBlockState(plugin.BlockPos{X: x, Y: minY, Z: z}, bedrock, plugin.UpdateClients)
			}
			isWall := (wallX && (x == sx || x == ex)) || (wallZ && (z == sz || z == ez))
			writeRoadOrWallColumn(w, area, x, z, startY, maxY, isWall, roadPat, wallFill, wallTop, air)
		}
	}
	if gen != nil {
		gen.ApplySchematicOverlay(w, plugin.BlockPos{X: sx, Y: minY, Z: sz}, plugin.BlockPos{X: ex, Y: maxY, Z: ez}, true)
	}
}

// CopyPlotBlocks copies all blocks from src plot to dst plot (single plots of matching width).
func CopyPlotBlocks(srcW plugin.World, srcArea *plots.PlotArea, srcID plots.PlotId, dstW plugin.World, dstArea *plots.PlotArea, dstID plots.PlotId) {
	sb := srcArea.PlotInteriorBounds(srcID)
	db := dstArea.PlotInteriorBounds(dstID)
	minY := max(srcArea.MinBuildHeight, srcW.MinY(), dstW.MinY())
	maxY := min(srcArea.MaxBuildHeight-1, srcW.MinY()+srcW.Height()-1, dstW.MinY()+dstW.Height()-1)
	dx := db.Min.X - sb.Min.X
	dz := db.Min.Z - sb.Min.Z
	for x := sb.Min.X; x <= sb.Max.X; x++ {
		for z := sb.Min.Z; z <= sb.Max.Z; z++ {
			for y := minY; y <= maxY; y++ {
				st := srcW.BlockState(plugin.BlockPos{X: x, Y: y, Z: z})
				dstW.SetBlockState(plugin.BlockPos{X: x + dx, Y: y, Z: z + dz}, st, plugin.UpdateClients)
			}
		}
	}
}

// SwapPlotBlocks swaps all blocks between two plots of equal size.
func SwapPlotBlocks(w1 plugin.World, a1 *plots.PlotArea, id1 plots.PlotId, w2 plugin.World, a2 *plots.PlotArea, id2 plots.PlotId) {
	b1 := a1.PlotInteriorBounds(id1)
	b2 := a2.PlotInteriorBounds(id2)
	minY := max(a1.MinBuildHeight, w1.MinY(), w2.MinY())
	maxY := min(a1.MaxBuildHeight-1, w1.MinY()+w1.Height()-1, w2.MinY()+w2.Height()-1)
	dx := b2.Min.X - b1.Min.X
	dz := b2.Min.Z - b1.Min.Z
	for x := b1.Min.X; x <= b1.Max.X; x++ {
		for z := b1.Min.Z; z <= b1.Max.Z; z++ {
			for y := minY; y <= maxY; y++ {
				p1 := plugin.BlockPos{X: x, Y: y, Z: z}
				p2 := plugin.BlockPos{X: x + dx, Y: y, Z: z + dz}
				s1 := w1.BlockState(p1)
				s2 := w2.BlockState(p2)
				if s1 != s2 {
					w1.SetBlockState(p1, s2, plugin.UpdateClients)
					w2.SetBlockState(p2, s1, plugin.UpdateClients)
				}
			}
		}
	}
}

// CreateRoadSchematic captures sideroad.schem and intersection.schem from plot's surrounding roads
// into plugins/plots/schematics/GEN_ROAD_SCHEMATIC/<world>/.
func CreateRoadSchematic(w plugin.World, area *plots.PlotArea, plot *plots.Plot, dataDir string) error {
	if w == nil || area == nil || plot == nil || area.RoadWidth <= 0 {
		return fmt.Errorf("cannot create road schematic for road width %d", area.RoadWidth)
	}
	b := area.PlotInteriorBounds(plot.ID)
	minY := min(area.RoadHeight, area.WallHeight)
	maxY := min(area.MaxGenHeight, w.MinY()+w.Height()-1)

	outDir := filepath.Join(dataDir, "schematics", "GEN_ROAD_SCHEMATIC", area.WorldName)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// Sideroad: east road strip along Z of the plot
	sideMin := plugin.BlockPos{X: b.Max.X + 1, Y: minY, Z: b.Min.Z}
	sideMax := plugin.BlockPos{X: b.Max.X + area.RoadWidth, Y: maxY, Z: b.Max.Z}
	if err := saveBoxSchematic(w, sideMin, sideMax, filepath.Join(outDir, "sideroad.schem")); err != nil {
		return err
	}

	// Intersection: southeast square
	interMin := plugin.BlockPos{X: b.Max.X + 1, Y: minY, Z: b.Max.Z + 1}
	interMax := plugin.BlockPos{X: b.Max.X + area.RoadWidth, Y: maxY, Z: b.Max.Z + area.RoadWidth}
	return saveBoxSchematic(w, interMin, interMax, filepath.Join(outDir, "intersection.schem"))
}

// SavePlotSchematic saves a plot's interior blocks to a Sponge v3 .schem file.
func SavePlotSchematic(w plugin.World, area *plots.PlotArea, plot *plots.Plot, path string) error {
	if w == nil || area == nil || plot == nil {
		return fmt.Errorf("invalid plot")
	}
	bMin, bMax := mergedOuterBounds(area, plot.BasePlot())
	bMin.Y = max(area.MinBuildHeight, w.MinY())
	bMax.Y = min(area.MaxBuildHeight-1, w.MinY()+w.Height()-1)
	return saveBoxSchematic(w, bMin, bMax, path)
}

// PastePlotSchematic pastes a WorldEdit Clipboard into plot, clipped to plot bounds.
func PastePlotSchematic(w plugin.World, area *plots.PlotArea, plot *plots.Plot, cb *westorage.Clipboard, pasteOnTop bool) int {
	if w == nil || area == nil || plot == nil || cb == nil {
		return 0
	}
	bMin, bMax := mergedOuterBounds(area, plot.BasePlot())
	baseY := max(area.MinBuildHeight, w.MinY())
	if pasteOnTop {
		baseY = area.PlotHeight
	}
	maxY := min(area.MaxBuildHeight-1, w.MinY()+w.Height()-1)
	changed := 0
	for y := 0; y < cb.Height; y++ {
		wy := baseY + y
		if wy < w.MinY() || wy > maxY {
			continue
		}
		for z := 0; z < cb.Length; z++ {
			wz := bMin.Z + z
			if wz > bMax.Z {
				continue
			}
			for x := 0; x < cb.Width; x++ {
				wx := bMin.X + x
				if wx > bMax.X {
					continue
				}
				w.SetBlockState(plugin.BlockPos{X: wx, Y: wy, Z: wz}, cb.GetRel(x, y, z), plugin.UpdateClients)
				changed++
			}
		}
	}
	return changed
}

func saveBoxSchematic(w plugin.World, minP, maxP plugin.BlockPos, path string) error {
	// Trim top empty air layers so saved schematics stay compact.
	air := plugin.DefaultState(registry.BlockAir)
	topY := minP.Y
	for y := maxP.Y; y >= minP.Y; y-- {
		hasNonAir := false
		for x := minP.X; x <= maxP.X && !hasNonAir; x++ {
			for z := minP.Z; z <= maxP.Z; z++ {
				if w.BlockState(plugin.BlockPos{X: x, Y: y, Z: z}) != air {
					hasNonAir = true
					break
				}
			}
		}
		if hasNonAir {
			topY = y
			break
		}
	}
	maxP.Y = topY
	cub := region.NewCuboid(minP, maxP)
	cb := westorage.NewClipboard(cub, minP)
	for y := minP.Y; y <= maxP.Y; y++ {
		for z := minP.Z; z <= maxP.Z; z++ {
			for x := minP.X; x <= maxP.X; x++ {
				cb.SetRel(x-minP.X, y-minP.Y, z-minP.Z, w.BlockState(plugin.BlockPos{X: x, Y: y, Z: z}))
			}
		}
	}
	return westorage.SaveSchematicFile(path, cb, westorage.FormatFastV3)
}

// RegenRoadChunk regenerates only unmerged road/wall columns inside chunk (cx, cz).
func RegenRoadChunk(w plugin.World, area *plots.PlotArea, gen *HybridGenerator, cx, cz int) {
	if w == nil || area == nil || area.RoadWidth <= 0 {
		return
	}
	air := plugin.DefaultState(registry.BlockAir)
	bedrock := plugin.DefaultState(registry.BlockBedrock)
	roadPat := CompileBucket(area.RoadBlock, registry.BlockQuartzBlock)
	wallFill := CompileBucket(area.WallFilling, registry.BlockStone)
	wallTop := CompileBucket(area.WallBlock, registry.BlockSmoothStoneSlab)

	baseX := cx << 4
	baseZ := cz << 4
	minY := max(area.MinGenHeight, w.MinY())
	maxY := min(area.MaxGenHeight, w.MinY()+w.Height()-1)
	startY := minY
	if area.PlotBedrock {
		startY++
	}

	for lx := range 16 {
		wx := baseX + lx
		for lz := range 16 {
			wz := baseZ + lz
			// Skip anything belonging to a plot (interior or merged road)
			if area.PlotAt(wx, wz) != nil {
				continue
			}
			rx := plots.FloorMod(wx-area.RoadOffsetX, area.Size)
			rz := plots.FloorMod(wz-area.RoadOffsetZ, area.Size)
			isRoad := rx < aPathLower(area) || rx > area.PathWidthUpper || rz < aPathLower(area) || rz > area.PathWidthUpper
			isWall := !isRoad && (rx == aPathLower(area) || rx == area.PathWidthUpper || rz == aPathLower(area) || rz == area.PathWidthUpper)
			if !isRoad && !isWall {
				continue
			}
			if area.PlotBedrock {
				w.SetBlockState(plugin.BlockPos{X: wx, Y: minY, Z: wz}, bedrock, plugin.UpdateClients)
			}
			writeRoadOrWallColumn(w, area, wx, wz, startY, maxY, isWall, roadPat, wallFill, wallTop, air)
			if gen != nil {
				gen.ApplySchematicOverlay(w, plugin.BlockPos{X: wx, Y: minY, Z: wz}, plugin.BlockPos{X: wx, Y: maxY, Z: wz}, true)
			}
		}
	}
}

func aPathLower(a *plots.PlotArea) int {
	return a.PathWidthLower
}

// SafePlotHome computes the teleport destination for a plot, honoring custom /plot sethome,
// area.DefaultHome, area.NonmemberHome, or the road side in front of the plot.
func SafePlotHome(w plugin.World, area *plots.PlotArea, plot *plots.Plot, isMember bool) plugin.Location {
	base := plot.BasePlot()
	bMin, bMax := mergedOuterBounds(area, base)

	var rel plots.BlockLoc
	if base.Settings.Position.Set && (isMember || area.HomeAllowNonmember) {
		rel = base.Settings.Position
	} else if isMember && area.DefaultHome.Set {
		rel = area.DefaultHome
	} else if !isMember && area.NonmemberHome.Set {
		rel = area.NonmemberHome
	}

	var tx, ty, tz int
	var yaw, pitch float32
	switch {
	case rel.Center:
		tx = (bMin.X + bMax.X) / 2
		tz = (bMin.Z + bMax.Z) / 2
		ty = safeYAt(w, tx, tz, area.PlotHeight+1)
	case rel.Set:
		tx = bMin.X + rel.X
		ty = rel.Y
		tz = bMin.Z + rel.Z
		yaw = rel.Yaw
		pitch = rel.Pitch
		if ty <= w.MinY() {
			ty = safeYAt(w, tx, tz, area.PlotHeight+1)
		}
	default:
		// Default side of plot on the northern road looking south into the plot
		tx = bMin.X + area.PlotWidth/2
		tz = bMin.Z - 2
		ty = safeYAt(w, tx, tz, area.RoadHeight+1)
		yaw = 0
	}

	return plugin.Location{
		World: w,
		Pos:   plugin.Vec3{X: float64(tx) + 0.5, Y: float64(ty), Z: float64(tz) + 0.5},
		Yaw:   yaw,
		Pitch: pitch,
	}
}

// PlotCenter returns the center Location of plot.
func PlotCenter(w plugin.World, area *plots.PlotArea, plot *plots.Plot) plugin.Location {
	bMin, bMax := mergedOuterBounds(area, plot.BasePlot())
	tx := (bMin.X + bMax.X) / 2
	tz := (bMin.Z + bMax.Z) / 2
	ty := safeYAt(w, tx, tz, area.PlotHeight+1)
	return plugin.Location{
		World: w,
		Pos:   plugin.Vec3{X: float64(tx) + 0.5, Y: float64(ty), Z: float64(tz) + 0.5},
	}
}

func safeYAt(w plugin.World, x, z, fallback int) int {
	if w == nil {
		return fallback
	}
	top := w.HighestBlockY(x, z, plugin.MotionBlocking)
	if top < w.MinY() {
		return fallback
	}
	return top + 1
}

func mergedOuterBounds(area *plots.PlotArea, base *plots.Plot) (minP, maxP plugin.BlockPos) {
	first := true
	for _, r := range area.PlotRegions(base) {
		if first {
			minP = r.Min
			maxP = r.Max
			first = false
			continue
		}
		minP.X = min(minP.X, r.Min.X)
		minP.Y = min(minP.Y, r.Min.Y)
		minP.Z = min(minP.Z, r.Min.Z)
		maxP.X = max(maxP.X, r.Max.X)
		maxP.Y = max(maxP.Y, r.Max.Y)
		maxP.Z = max(maxP.Z, r.Max.Z)
	}
	return minP, maxP
}
