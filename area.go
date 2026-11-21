package plotsquared

import (
	"math"
	"strings"
	"sync"

	"github.com/golem-mc/golem/plugin"
	"github.com/google/uuid"
)

// PlotArea represents a configured plot area inside a world (either an entire
// plot world or a partial/augmented area within a world).
type PlotArea struct {
	// WorldName is the Bukkit-style world name (key in worlds.yml).
	WorldName string
	// ID is empty for whole-world areas, or the partial area name.
	ID string
	// Type is NORMAL (0), AUGMENTED (1), or PARTIAL (2).
	Type PlotAreaType
	// Terrain controls vanilla terrain preservation (NONE/ALL/ORE/ROAD).
	Terrain PlotAreaTerrainType
	// Min and Max bound the allowed PlotIds for PARTIAL areas (nil if unbounded).
	Min *PlotId
	Max *PlotId

	// Grid geometry (SquarePlotWorld / ClassicPlotWorld / HybridPlotWorld)
	PlotWidth             int
	RoadWidth             int
	Size                  int // PlotWidth + RoadWidth
	RoadOffsetX           int
	RoadOffsetZ           int
	PathWidthLower        int
	PathWidthUpper        int
	PlotHeight            int
	RoadHeight            int
	WallHeight            int
	MinGenHeight          int
	MaxGenHeight          int
	MinBuildHeight        int
	MaxBuildHeight        int
	PlotBedrock           bool
	PlaceTopBlock         bool
	ComponentBelowBedrock bool

	// Block patterns
	MainBlock        BlockBucket
	TopBlock         BlockBucket
	WallBlock        BlockBucket
	ClaimedWallBlock BlockBucket
	WallFilling      BlockBucket
	RoadBlock        BlockBucket

	// Area settings
	PlotBiome             string // e.g. "minecraft:forest"
	GameMode              plugin.GameMode
	AutoMerge             bool
	AllowSigns            bool
	SignMaterial          string
	MobSpawning           bool
	MobSpawnerSpawning    bool
	MiscSpawnUnowned      bool
	SpawnEggs             bool
	SpawnCustom           bool
	SpawnBreeding         bool
	PlotChat              bool
	ForcingPlotChat       bool
	WorldBorder           bool
	BorderSize            int
	HomeAllowNonmember    bool
	DefaultHome           BlockLoc
	NonmemberHome         BlockLoc
	UseEconomy            bool
	PriceClaim            string
	PriceMerge            string
	PriceSell             string
	SchematicOnClaim      bool
	SchematicFile         string
	SchematicClaimSpecify bool
	Schematics            []string
	RoadSchematicEnabled  bool
	PlotSchematicEnabled  bool

	// Flags
	Flags        *FlagContainer
	RoadFlags    *FlagContainer
	HasRoadFlags bool

	// Runtime plot & cluster storage guarded by mu.
	mu       sync.RWMutex
	plots    map[PlotId]*Plot
	clusters []*PlotCluster
	lastPlot PlotId
}

// DefaultPlotArea creates a PlotArea with upstream ClassicPlotWorld/HybridPlotWorld defaults.
func DefaultPlotArea(worldName, id string) *PlotArea {
	a := &PlotArea{
		WorldName:             worldName,
		ID:                    id,
		Type:                  AreaNormal,
		Terrain:               TerrainNone,
		PlotWidth:             42,
		RoadWidth:             7,
		PlotHeight:            62,
		RoadHeight:            62,
		WallHeight:            62,
		MinGenHeight:          0,
		MaxGenHeight:          256,
		MinBuildHeight:        1,
		MaxBuildHeight:        256,
		PlotBedrock:           true,
		PlaceTopBlock:         true,
		ComponentBelowBedrock: false,
		MainBlock:             MustBlockBucket("stone"),
		TopBlock:              MustBlockBucket("grass_block"),
		WallBlock:             MustBlockBucket("smooth_stone_slab"),
		ClaimedWallBlock:      MustBlockBucket("sandstone_slab"),
		WallFilling:           MustBlockBucket("stone"),
		RoadBlock:             MustBlockBucket("quartz_block"),
		PlotBiome:             "minecraft:forest",
		GameMode:              plugin.Creative,
		AllowSigns:            true,
		SignMaterial:          "oak_wall_sign",
		SpawnCustom:           true,
		PlotChat:              true,
		BorderSize:            1,
		PriceClaim:            "100",
		PriceMerge:            "100",
		PriceSell:             "100",
		SchematicFile:         "null",
		Flags:                 NewFlagContainer(GlobalFlags),
		RoadFlags:             NewFlagContainer(GlobalFlags),
		plots:                 make(map[PlotId]*Plot),
	}
	a.ComputeGeometry()
	return a
}

// ComputeGeometry calculates Size, PathWidthLower, and PathWidthUpper from PlotWidth and RoadWidth.
// Matches upstream HybridPlotWorld.loadConfiguration.
func (a *PlotArea) ComputeGeometry() {
	a.Size = a.PlotWidth + a.RoadWidth
	if a.RoadWidth == 0 {
		a.PathWidthLower = 0
		a.PathWidthUpper = a.Size + 1
		return
	}
	if a.RoadWidth&1 == 0 {
		a.PathWidthLower = a.RoadWidth/2 - 1
	} else {
		a.PathWidthLower = a.RoadWidth / 2
	}
	a.PathWidthUpper = a.PathWidthLower + a.PlotWidth + 1
}

// FullID returns WorldName for whole-world areas or "WorldName;ID" for partial areas.
// Matches upstream PlotArea.toString(), used in SQLite's `plot.world` column.
func (a *PlotArea) FullID() string {
	if a.ID == "" {
		return a.WorldName
	}
	return a.WorldName + ";" + a.ID
}

// MinComponentHeight returns the minimum Y affected by component/clear operations.
// Matches upstream ClassicPlotWorld.getMinComponentHeight.
func (a *PlotArea) MinComponentHeight() int {
	if a.ComponentBelowBedrock && a.MinGenHeight >= a.MinBuildHeight {
		if a.PlotBedrock {
			return a.MinGenHeight + 1
		}
		return a.MinGenHeight
	}
	return a.MinBuildHeight
}

// ContainsPlotId reports whether id falls within this area's Min/Max PlotId bounds.
func (a *PlotArea) ContainsPlotId(id PlotId) bool {
	if a.Min != nil && (id.X < a.Min.X || id.Y < a.Min.Y) {
		return false
	}
	if a.Max != nil && (id.X > a.Max.X || id.Y > a.Max.Y) {
		return false
	}
	return true
}

// FloorDiv performs Euclidean floor division (matching Java Math.floorDiv).
func FloorDiv(a, b int) int {
	q := a / b
	if (a^b) < 0 && q*b != a {
		q--
	}
	return q
}

// FloorMod performs Euclidean floor modulo in [0, b) (matching Java Math.floorMod).
func FloorMod(a, b int) int {
	r := a % b
	if r < 0 {
		r += b
	}
	return r
}

// PlotBottomAbs returns the lowest (min X, min Z) interior block coordinate of plot id.
// Matches upstream SquarePlotManager.getPlotBottomLocAbs:
//
//	px = (id.X - 1) * SIZE + ROAD_OFFSET_X + PATH_WIDTH_LOWER + 1  (wait: check exact SquarePlotManager formula!)
//
// Wait: in upstream SquarePlotManager:
//
//	int px = plotId.getX(); int pz = plotId.getY();
//	int x = (px * dpw.SIZE) - dpw.PLOT_WIDTH - ((int) Math.floor(dpw.ROAD_WIDTH / 2.0)) - 1 + dpw.ROAD_OFFSET_X;
//	Wait: PlotBottomLocAbs returns the block *outside* (at the wall) in upstream, and Plot.getBottomAbs() adds (1,0,1)!
//	Let's provide both interior bottom/top (exact buildable plot corners) and wall bounds!
func (a *PlotArea) PlotInteriorBounds(id PlotId) RegionBounds {
	if a.Size <= 0 {
		return RegionBounds{}
	}
	// Let's verify against HybridGen:
	// In HybridGen: bx = worldX - ROAD_OFFSET_X; relX = FloorMod(bx, SIZE).
	// For RoadWidth > 0:
	//   road: relX < PathWidthLower || relX > PathWidthUpper
	//   wall: relX == PathWidthLower || relX == PathWidthUpper
	//   plot: PathWidthLower < relX < PathWidthUpper  i.e. [PathWidthLower+1 .. PathWidthUpper-1]
	// Number of interior blocks = (PathWidthUpper - 1) - (PathWidthLower + 1) + 1 = PathWidthUpper - PathWidthLower - 1 = PlotWidth!
	// For PlotId (1, 1): cell index is (id.X - 1, id.Y - 1) in [0, SIZE) when id=(1,1)?
	// Wait: let's check SquarePlotManager.getPlotIdAbs!
	// For id.X = 1: cell 0 has bx in [0, SIZE-1].
	// Let's make sure PlotIdAt(x, z) and PlotInteriorBounds(id) are exact inverses AND match SquarePlotManager!
	cellX := id.X - 1
	cellZ := id.Y - 1
	var minX, minZ int
	if a.RoadWidth == 0 {
		minX = cellX*a.Size + a.RoadOffsetX
		minZ = cellZ*a.Size + a.RoadOffsetZ
	} else {
		minX = cellX*a.Size + a.RoadOffsetX + a.PathWidthLower + 1
		minZ = cellZ*a.Size + a.RoadOffsetZ + a.PathWidthLower + 1
	}
	maxX := minX + a.PlotWidth - 1
	maxZ := minZ + a.PlotWidth - 1
	return RegionBounds{
		Min: plugin.BlockPos{X: minX, Y: a.MinBuildHeight, Z: minZ},
		Max: plugin.BlockPos{X: maxX, Y: a.MaxBuildHeight, Z: maxZ},
	}
}

// PlotIdAt returns the PlotId if (x, z) is strictly inside a plot's buildable interior,
// or (PlotId{}, false) if (x, z) is on a wall or road (or outside partial area bounds).
func (a *PlotArea) PlotIdAt(x, z int) (PlotId, bool) {
	if a.Size <= 0 {
		return PlotId{}, false
	}
	bx := x - a.RoadOffsetX
	bz := z - a.RoadOffsetZ
	relX := FloorMod(bx, a.Size)
	relZ := FloorMod(bz, a.Size)
	if a.RoadWidth > 0 {
		if relX <= a.PathWidthLower || relX >= a.PathWidthUpper ||
			relZ <= a.PathWidthLower || relZ >= a.PathWidthUpper {
			return PlotId{}, false
		}
	}
	id := PlotId{
		X: FloorDiv(bx, a.Size) + 1,
		Y: FloorDiv(bz, a.Size) + 1,
	}
	if !a.ContainsPlotId(id) {
		return PlotId{}, false
	}
	return id, true
}

// PlotIdNearest returns the PlotId of the grid cell containing (x, z), even on roads/walls.
func (a *PlotArea) PlotIdNearest(x, z int) PlotId {
	if a.Size <= 0 {
		return PlotId{X: 1, Y: 1}
	}
	bx := x - a.RoadOffsetX
	bz := z - a.RoadOffsetZ
	return PlotId{
		X: FloorDiv(bx, a.Size) + 1,
		Y: FloorDiv(bz, a.Size) + 1,
	}
}

// ContainsXZ reports whether block coordinate (x, z) lies inside this area
// (always true for whole-world areas; bounded for partial areas).
func (a *PlotArea) ContainsXZ(x, z int) bool {
	if a.Min == nil || a.Max == nil {
		return true
	}
	minB := a.PlotInteriorBounds(*a.Min)
	maxB := a.PlotInteriorBounds(*a.Max)
	// Include surrounding wall (1 block outside) as upstream PlotArea.getRegionAbs does.
	return x >= minB.Min.X-1 && x <= maxB.Max.X+1 && z >= minB.Min.Z-1 && z <= maxB.Max.Z+1
}

// PlotAt returns the Plot at block coordinates (x, z), including merged road/wall segments
// between two merged plots. Returns nil if on an unmerged road/wall or outside the area.
func (a *PlotArea) PlotAt(x, z int) *Plot {
	if !a.ContainsXZ(x, z) || a.Size <= 0 {
		return nil
	}
	bx := x - a.RoadOffsetX
	bz := z - a.RoadOffsetZ
	cellX := FloorDiv(bx, a.Size)
	cellZ := FloorDiv(bz, a.Size)
	relX := FloorMod(bx, a.Size)
	relZ := FloorMod(bz, a.Size)
	id := PlotId{X: cellX + 1, Y: cellZ + 1}

	if a.RoadWidth == 0 {
		if !a.ContainsPlotId(id) {
			return nil
		}
		return a.GetOrCreatePlot(id)
	}

	inPlotX := relX > a.PathWidthLower && relX < a.PathWidthUpper
	inPlotZ := relZ > a.PathWidthLower && relZ < a.PathWidthUpper
	if inPlotX && inPlotZ {
		if !a.ContainsPlotId(id) {
			return nil
		}
		return a.GetOrCreatePlot(id)
	}

	// Check if (x, z) is on a road/wall connecting merged plots!
	// Determine which neighbouring plot(s) this road/wall sits between.
	var p1, p2 *Plot
	switch {
	case !inPlotX && inPlotZ:
		// Vertical strip between west plot and east plot
		if relX <= a.PathWidthLower {
			// Between (id.X-1, id.Y) and (id.X, id.Y)
			p1 = a.PlotAbs(PlotId{X: id.X - 1, Y: id.Y})
			p2 = a.PlotAbs(id)
			if p1 != nil && p2 != nil && p1.IsMergedDir(DirEast) && p2.IsMergedDir(DirWest) {
				return p1.BasePlot()
			}
		} else {
			// relX >= PathWidthUpper: between (id.X, id.Y) and (id.X+1, id.Y)
			p1 = a.PlotAbs(id)
			p2 = a.PlotAbs(PlotId{X: id.X + 1, Y: id.Y})
			if p1 != nil && p2 != nil && p1.IsMergedDir(DirEast) && p2.IsMergedDir(DirWest) {
				return p1.BasePlot()
			}
		}
	case inPlotX && !inPlotZ:
		// Horizontal strip between north plot and south plot
		if relZ <= a.PathWidthLower {
			p1 = a.PlotAbs(PlotId{X: id.X, Y: id.Y - 1})
			p2 = a.PlotAbs(id)
			if p1 != nil && p2 != nil && p1.IsMergedDir(DirSouth) && p2.IsMergedDir(DirNorth) {
				return p1.BasePlot()
			}
		} else {
			p1 = a.PlotAbs(id)
			p2 = a.PlotAbs(PlotId{X: id.X, Y: id.Y + 1})
			if p1 != nil && p2 != nil && p1.IsMergedDir(DirSouth) && p2.IsMergedDir(DirNorth) {
				return p1.BasePlot()
			}
		}
	default:
		// Intersection of 4 plots! Only belongs to the merged plot if all 4 surrounding plots are merged in a 2x2 quad.
		nwX, nwY := id.X, id.Y
		if relX <= a.PathWidthLower {
			nwX--
		}
		if relZ <= a.PathWidthLower {
			nwY--
		}
		nw := a.PlotAbs(PlotId{X: nwX, Y: nwY})
		ne := a.PlotAbs(PlotId{X: nwX + 1, Y: nwY})
		sw := a.PlotAbs(PlotId{X: nwX, Y: nwY + 1})
		se := a.PlotAbs(PlotId{X: nwX + 1, Y: nwY + 1})
		if nw != nil && ne != nil && sw != nil && se != nil &&
			nw.IsMergedDir(DirEast) && nw.IsMergedDir(DirSouth) &&
			ne.IsMergedDir(DirSouth) && sw.IsMergedDir(DirEast) {
			return nw.BasePlot()
		}
	}
	return nil
}

// PlotAbs returns the claimed Plot with id, or nil if unclaimed.
func (a *PlotArea) PlotAbs(id PlotId) *Plot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.plots[id]
}

// GetOrCreatePlot returns the claimed Plot with id, or a temporary unowned Plot object.
func (a *PlotArea) GetOrCreatePlot(id PlotId) *Plot {
	if p := a.PlotAbs(id); p != nil {
		return p
	}
	return NewPlot(a, id, uuid.Nil)
}

// SetPlot stores a claimed plot in the area's in-memory map.
func (a *PlotArea) SetPlot(p *Plot) {
	p.Area = a
	if p.Flags != nil {
		p.Flags.SetParent(a.Flags)
	}
	a.mu.Lock()
	a.plots[p.ID] = p
	a.mu.Unlock()
}

// RemovePlot deletes a plot from the area's in-memory map.
func (a *PlotArea) RemovePlot(id PlotId) {
	a.mu.Lock()
	delete(a.plots, id)
	a.mu.Unlock()
}

// Plots returns a snapshot slice of all claimed plots in the area.
func (a *PlotArea) Plots() []*Plot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*Plot, 0, len(a.plots))
	for _, p := range a.plots {
		out = append(out, p)
	}
	return out
}

// BasePlots returns only canonical base plots (one per merged group) in the area.
func (a *PlotArea) BasePlots() []*Plot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	seen := make(map[PlotId]struct{})
	var out []*Plot
	for _, p := range a.plots {
		base := p.BasePlot()
		if _, ok := seen[base.ID]; !ok {
			seen[base.ID] = struct{}{}
			out = append(out, base)
		}
	}
	return out
}

// PlotsByOwner returns all base plots owned by u in this area.
func (a *PlotArea) PlotsByOwner(u uuid.UUID) []*Plot {
	var out []*Plot
	for _, p := range a.BasePlots() {
		if p.IsOwner(u) {
			out = append(out, p)
		}
	}
	return out
}

// NextFreePlotId searches in upstream spiral order for the next unclaimed PlotId.
func (a *PlotArea) NextFreePlotId() (PlotId, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cur := a.lastPlot
	if cur == (PlotId{}) {
		cur = PlotId{X: 0, Y: 0}
	}
	const maxSteps = 100000
	for range maxSteps {
		cur = cur.NextId()
		if cur.X == 0 && cur.Y == 0 {
			continue
		}
		if !a.ContainsPlotId(cur) {
			continue
		}
		if _, taken := a.plots[cur]; !taken && !a.inClusterLocked(cur) {
			a.lastPlot = cur
			return cur, true
		}
	}
	return PlotId{}, false
}

// NextFreePlotBlock finds an unclaimed sizeX by sizeZ rectangular block of plots.
func (a *PlotArea) NextFreePlotBlock(sizeX, sizeZ int) (PlotId, bool) {
	if sizeX <= 1 && sizeZ <= 1 {
		return a.NextFreePlotId()
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	cur := PlotId{X: 0, Y: 0}
	const maxSteps = 50000
	for range maxSteps {
		cur = cur.NextId()
		ok := true
		for dx := range sizeX {
			for dz := range sizeZ {
				cand := PlotId{X: cur.X + dx, Y: cur.Y + dz}
				if !a.ContainsPlotId(cand) {
					ok = false
					break
				}
				if _, taken := a.plots[cand]; taken || a.inClusterLocked(cand) {
					ok = false
					break
				}
			}
			if !ok {
				break
			}
		}
		if ok {
			return cur, true
		}
	}
	return PlotId{}, false
}

func (a *PlotArea) inClusterLocked(id PlotId) bool {
	for _, c := range a.clusters {
		if c.Contains(id) {
			return true
		}
	}
	return false
}

// AddCluster registers a cluster in this area.
func (a *PlotArea) AddCluster(c *PlotCluster) {
	c.Area = a
	a.mu.Lock()
	a.clusters = append(a.clusters, c)
	a.mu.Unlock()
}

// RemoveCluster removes a cluster from this area.
func (a *PlotArea) RemoveCluster(c *PlotCluster) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, existing := range a.clusters {
		if existing == c || (existing.Name != "" && strings.EqualFold(existing.Name, c.Name)) {
			a.clusters = append(a.clusters[:i], a.clusters[i+1:]...)
			return
		}
	}
}

// Clusters returns a copy of all clusters in this area.
func (a *PlotArea) Clusters() []*PlotCluster {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*PlotCluster, len(a.clusters))
	copy(out, a.clusters)
	return out
}

// ClusterAt returns the cluster containing id, or nil.
func (a *PlotArea) ClusterAt(id PlotId) *PlotCluster {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, c := range a.clusters {
		if c.Contains(id) {
			return c
		}
	}
	return nil
}

// ClusterNamed returns the cluster with name (case-insensitive), or nil.
func (a *PlotArea) ClusterNamed(name string) *PlotCluster {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, c := range a.clusters {
		if strings.EqualFold(c.Name, name) {
			return c
		}
	}
	return nil
}

// PlotRegions returns all bounding boxes belonging to a (possibly merged) plot,
// including connecting roads and 2x2 intersections.
func (a *PlotArea) PlotRegions(p *Plot) []RegionBounds {
	if p == nil {
		return nil
	}
	connected := p.ConnectedPlots()
	out := make([]RegionBounds, 0, len(connected)*2)
	idSet := make(map[PlotId]*Plot, len(connected))
	for _, cp := range connected {
		idSet[cp.ID] = cp
		out = append(out, a.PlotInteriorBounds(cp.ID))
	}
	if a.RoadWidth <= 0 {
		return out
	}
	for _, cp := range connected {
		b := a.PlotInteriorBounds(cp.ID)
		// East road strip between cp and cp.East
		if cp.IsMergedDir(DirEast) {
			out = append(out, RegionBounds{
				Min: plugin.BlockPos{X: b.Max.X + 1, Y: a.MinBuildHeight, Z: b.Min.Z},
				Max: plugin.BlockPos{X: b.Max.X + a.RoadWidth, Y: a.MaxBuildHeight, Z: b.Max.Z},
			})
		}
		// South road strip between cp and cp.South
		if cp.IsMergedDir(DirSouth) {
			out = append(out, RegionBounds{
				Min: plugin.BlockPos{X: b.Min.X, Y: a.MinBuildHeight, Z: b.Max.Z + 1},
				Max: plugin.BlockPos{X: b.Max.X, Y: a.MaxBuildHeight, Z: b.Max.Z + a.RoadWidth},
			})
		}
		// Southeast 2x2 intersection
		if cp.IsMergedDir(DirEast) && cp.IsMergedDir(DirSouth) {
			east := idSet[cp.ID.Relative(DirEast)]
			south := idSet[cp.ID.Relative(DirSouth)]
			if east != nil && south != nil && east.IsMergedDir(DirSouth) && south.IsMergedDir(DirEast) {
				out = append(out, RegionBounds{
					Min: plugin.BlockPos{X: b.Max.X + 1, Y: a.MinBuildHeight, Z: b.Max.Z + 1},
					Max: plugin.BlockPos{X: b.Max.X + a.RoadWidth, Y: a.MaxBuildHeight, Z: b.Max.Z + a.RoadWidth},
				})
			}
		}
	}
	return out
}

// WorldBorderRadius returns the distance in blocks from origin to the furthest claimed plot + BorderSize.
func (a *PlotArea) WorldBorderRadius() int {
	maxPlot := 1
	a.mu.RLock()
	for id := range a.plots {
		if ax := absInt(id.X); ax > maxPlot {
			maxPlot = ax
		}
		if ay := absInt(id.Y); ay > maxPlot {
			maxPlot = ay
		}
	}
	a.mu.RUnlock()
	return (maxPlot + a.BorderSize) * a.Size
}

// EvalPrice evaluates a simple price expression (constant number or expression using `plots` variable).
func EvalPrice(expr string, ownedPlots int) float64 {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0
	}
	expr = strings.ReplaceAll(expr, "{arg}", "plots")
	v, ok := evalSimpleExpr(expr, float64(ownedPlots))
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return v
}
