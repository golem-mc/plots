package plotsquared

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// PlotId identifies a plot's (X, Y) grid coordinates within a PlotArea.
// Note that Y is the grid Z-axis (north-south), matching upstream PlotSquared.
type PlotId struct {
	X int
	Y int
}

// ParsePlotId parses a plot ID such as "1;2", "1,2", "1_2", or "1.2".
func ParsePlotId(s string) (PlotId, bool) {
	parts := strings.FieldsFunc(strings.TrimSpace(s), func(r rune) bool {
		return r == ';' || r == '_' || r == ',' || r == '.'
	})
	if len(parts) < 2 {
		return PlotId{}, false
	}
	x, err := strconv.Atoi(parts[0])
	if err != nil {
		return PlotId{}, false
	}
	y, err := strconv.Atoi(parts[1])
	if err != nil {
		return PlotId{}, false
	}
	return PlotId{X: x, Y: y}, true
}

// String formats id as "x;y".
func (id PlotId) String() string {
	return fmt.Sprintf("%d;%d", id.X, id.Y)
}

// CommaString formats id as "x,y".
func (id PlotId) CommaString() string {
	return fmt.Sprintf("%d,%d", id.X, id.Y)
}

// HashCode returns upstream PlotId.hashCode(): (int32(X) << 16) | (int32(Y) & 0xFFFF).
// Used by SQLite plot_comments.hashcode.
func (id PlotId) HashCode() int32 {
	return (int32(id.X) << 16) | (int32(id.Y) & 0xFFFF)
}

// Relative returns the adjacent PlotId in dir.
func (id PlotId) Relative(dir Direction) PlotId {
	switch dir {
	case DirNorth:
		return PlotId{X: id.X, Y: id.Y - 1}
	case DirEast:
		return PlotId{X: id.X + 1, Y: id.Y}
	case DirSouth:
		return PlotId{X: id.X, Y: id.Y + 1}
	case DirWest:
		return PlotId{X: id.X - 1, Y: id.Y}
	default:
		return id
	}
}

// NextId returns the next PlotId along PlotSquared's spiral search order.
// Upstream: PlotId.getNextId()
func (id PlotId) NextId() PlotId {
	x, y := id.X, id.Y
	absX := absInt(x)
	absY := absInt(y)
	switch {
	case absX > absY:
		if x > 0 {
			return PlotId{X: x, Y: y + 1}
		}
		return PlotId{X: x, Y: y - 1}
	case absY > absX:
		if y > 0 {
			return PlotId{X: x - 1, Y: y}
		}
		return PlotId{X: x + 1, Y: y}
	default:
		if x == y && x > 0 {
			return PlotId{X: x, Y: y + 1}
		}
		if x == absX {
			return PlotId{X: x, Y: y + 1}
		}
		if y == absY {
			return PlotId{X: x, Y: y - 1}
		}
		return PlotId{X: x + 1, Y: y}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Direction is a cardinal/ordinal plot direction matching upstream Direction.
type Direction int

const (
	DirAll       Direction = -1
	DirNorth     Direction = 0
	DirEast      Direction = 1
	DirSouth     Direction = 2
	DirWest      Direction = 3
	DirNortheast Direction = 4
	DirSoutheast Direction = 5
	DirSouthwest Direction = 6
	DirNorthwest Direction = 7
)

// CardinalDirections lists the four cardinal merge directions in index order.
var CardinalDirections = [4]Direction{DirNorth, DirEast, DirSouth, DirWest}

// Opposite returns the opposite direction.
func (d Direction) Opposite() Direction {
	switch d {
	case DirNorth:
		return DirSouth
	case DirEast:
		return DirWest
	case DirSouth:
		return DirNorth
	case DirWest:
		return DirEast
	case DirNortheast:
		return DirSouthwest
	case DirSoutheast:
		return DirNorthwest
	case DirSouthwest:
		return DirNortheast
	case DirNorthwest:
		return DirSoutheast
	default:
		return DirAll
	}
}

// String returns the lower-case direction name.
func (d Direction) String() string {
	switch d {
	case DirAll:
		return "all"
	case DirNorth:
		return "north"
	case DirEast:
		return "east"
	case DirSouth:
		return "south"
	case DirWest:
		return "west"
	case DirNortheast:
		return "northeast"
	case DirSoutheast:
		return "southeast"
	case DirSouthwest:
		return "southwest"
	case DirNorthwest:
		return "northwest"
	default:
		return "unknown"
	}
}

// ParseDirection parses a direction name or shorthand (n/e/s/w/all/auto).
func ParseDirection(s string) (Direction, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "n", "north":
		return DirNorth, true
	case "e", "east":
		return DirEast, true
	case "s", "south":
		return DirSouth, true
	case "w", "west":
		return DirWest, true
	case "all", "auto":
		return DirAll, true
	case "ne", "northeast":
		return DirNortheast, true
	case "se", "southeast":
		return DirSoutheast, true
	case "sw", "southwest":
		return DirSouthwest, true
	case "nw", "northwest":
		return DirNorthwest, true
	default:
		return DirAll, false
	}
}

// DirectionFromYaw returns the cardinal direction a player with yaw is facing.
// Matches Minecraft yaw: 0=South, 90=West, 180/-180=North, 270/-90=East.
func DirectionFromYaw(yaw float32) Direction {
	y := math.Mod(float64(yaw), 360)
	if y < 0 {
		y += 360
	}
	switch {
	case y >= 315 || y < 45:
		return DirSouth
	case y >= 45 && y < 135:
		return DirWest
	case y >= 135 && y < 225:
		return DirNorth
	default:
		return DirEast
	}
}

// HashMerged encodes a [4]bool merge array (N, E, S, W) into upstream's
// SQLite `plot_settings.merged` integer bitmask. Upstream: HashUtil.hash.
func HashMerged(m [4]bool) int {
	var h int
	if m[0] {
		h |= 1 << 3
	}
	if m[1] {
		h |= 1 << 2
	}
	if m[2] {
		h |= 1 << 1
	}
	if m[3] {
		h |= 1
	}
	return h
}

// UnhashMerged decodes an integer bitmask from `plot_settings.merged` into [4]bool (N, E, S, W).
func UnhashMerged(h int) [4]bool {
	return [4]bool{
		(h & (1 << 3)) != 0,
		(h & (1 << 2)) != 0,
		(h & (1 << 1)) != 0,
		(h & 1) != 0,
	}
}

// BlockLoc is a relative block position and optional facing stored in plot_settings.position
// or worlds.yml home.default / home.nonmembers.
type BlockLoc struct {
	X, Y, Z    int
	Yaw, Pitch float32
	Set        bool
	Center     bool
}

// BlockLocCenter is the sentinel representing the middle/center of a plot.
var BlockLocCenter = BlockLoc{X: math.MaxInt32, Y: math.MinInt32, Z: math.MaxInt32, Set: true, Center: true}

// ParseBlockLoc parses a BlockLoc string ("side", "center", "x,y,z", "x,y,z,yaw,pitch").
func ParseBlockLoc(s string) (BlockLoc, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "", "default", "side", "0,0,0":
		return BlockLoc{}, nil
	case "center", "centre", "middle":
		return BlockLocCenter, nil
	}
	parts := strings.Split(s, ",")
	if len(parts) != 3 && len(parts) != 5 {
		return BlockLoc{}, fmt.Errorf("invalid block location %q", s)
	}
	x, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return BlockLoc{}, err
	}
	y, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return BlockLoc{}, err
	}
	z, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		return BlockLoc{}, err
	}
	var yaw, pitch float32
	if len(parts) == 5 {
		yf, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 32)
		if err != nil {
			return BlockLoc{}, err
		}
		pf, err := strconv.ParseFloat(strings.TrimSpace(parts[4]), 32)
		if err != nil {
			return BlockLoc{}, err
		}
		yaw = float32(yf)
		pitch = float32(pf)
	}
	if x == 0 && y == 0 && z == 0 && yaw == 0 && pitch == 0 {
		return BlockLoc{}, nil
	}
	return BlockLoc{X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch, Set: true}, nil
}

// String formats b for SQLite `plot_settings.position`.
func (b BlockLoc) String() string {
	if !b.Set {
		return "DEFAULT"
	}
	if b.Center {
		return "center"
	}
	if b.Yaw == 0 && b.Pitch == 0 {
		return fmt.Sprintf("%d,%d,%d", b.X, b.Y, b.Z)
	}
	return fmt.Sprintf("%d,%d,%d,%g,%g", b.X, b.Y, b.Z, b.Yaw, b.Pitch)
}

// PlotAreaType is the generator area type (worlds.yml generator.type).
type PlotAreaType int

const (
	AreaNormal    PlotAreaType = 0
	AreaAugmented PlotAreaType = 1
	AreaPartial   PlotAreaType = 2
)

func (t PlotAreaType) String() string {
	switch t {
	case AreaAugmented:
		return "AUGMENTED"
	case AreaPartial:
		return "PARTIAL"
	default:
		return "NORMAL"
	}
}

// ParsePlotAreaType parses a PlotAreaType from string or integer.
func ParsePlotAreaType(s string) (PlotAreaType, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "0", "normal", "default":
		return AreaNormal, true
	case "1", "augmented":
		return AreaAugmented, true
	case "2", "partial", "vanilla":
		return AreaPartial, true
	default:
		return AreaNormal, false
	}
}

// PlotAreaTerrainType controls terrain preservation in augmented/partial areas (worlds.yml generator.terrain).
type PlotAreaTerrainType int

const (
	TerrainNone PlotAreaTerrainType = 0
	TerrainAll  PlotAreaTerrainType = 1
	TerrainOre  PlotAreaTerrainType = 2
	TerrainRoad PlotAreaTerrainType = 3
)

func (t PlotAreaTerrainType) String() string {
	switch t {
	case TerrainAll:
		return "ALL"
	case TerrainOre:
		return "ORE"
	case TerrainRoad:
		return "ROAD"
	default:
		return "NONE"
	}
}

// ParsePlotAreaTerrainType parses a PlotAreaTerrainType from string or integer.
func ParsePlotAreaTerrainType(s string) (PlotAreaTerrainType, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "0", "none":
		return TerrainNone, true
	case "1", "all":
		return TerrainAll, true
	case "2", "ore":
		return TerrainOre, true
	case "3", "road":
		return TerrainRoad, true
	default:
		return TerrainNone, false
	}
}

// PlotWeather is the value of the weather plot flag.
type PlotWeather string

const (
	WeatherClear PlotWeather = "clear"
	WeatherRain  PlotWeather = "rain"
	WeatherWorld PlotWeather = "world"
	WeatherOff   PlotWeather = "off"
)

// PlotTitle is the value of the plot-title flag.
type PlotTitle struct {
	Title      string
	Subtitle   string
	Configured bool // true for default CONFIGURED sentinel
}

// PlotTitleConfigured is the default PlotTitle sentinel meaning use the global caption.
var PlotTitleConfigured = PlotTitle{Configured: true}
