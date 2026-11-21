package plots

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/google/uuid"
)

// EveryoneUUID is upstream PlotSquared's wildcard UUID ("*", DBFunc.EVERYONE = UUID.fromString("1-1-3-3-7")).
var EveryoneUUID = uuid.MustParse("00000001-0001-0003-0003-000000000007")

// ServerUUID is upstream PlotSquared's server owner UUID (DBFunc.SERVER).
var ServerUUID = uuid.Nil

// Plot represents a single grid plot cell (possibly merged with neighbours) in a PlotArea.
type Plot struct {
	// DBID is the row id in the SQLite `plot` table; 0 if unclaimed or not yet persisted.
	DBID int64
	// ID is this plot's grid coordinate within Area.
	ID PlotId
	// Area is the owning PlotArea.
	Area *PlotArea
	// Owner is the plot owner's UUID, or uuid.Nil if unowned.
	Owner uuid.UUID
	// Trusted are players who may build even when the owner is offline
	// (stored in upstream's `plot_helpers` table).
	Trusted map[uuid.UUID]struct{}
	// Members are players who may build while an owner is online
	// (stored in upstream's `plot_trusted` table).
	Members map[uuid.UUID]struct{}
	// Denied are players barred from entering the plot.
	Denied map[uuid.UUID]struct{}
	// Settings holds alias, merge edges, home position, ratings and comments.
	Settings PlotSettings
	// Flags holds plot-specific flags (with Area.Flags as parent).
	Flags *FlagContainer
	// Timestamp is when the plot was claimed/modified (Unix milliseconds).
	Timestamp int64
}

// PlotSettings stores per-plot metadata persisted in `plot_settings`, `plot_rating`, and `plot_comments`.
type PlotSettings struct {
	Merged   [4]bool // N, E, S, W
	Alias    string
	Position BlockLoc
	Ratings  map[uuid.UUID]int
	Comments []PlotComment
}

// PlotComment is a comment in a plot inbox ("public", "owner", "report").
type PlotComment struct {
	World     string
	ID        PlotId
	Comment   string
	Inbox     string
	Timestamp int64
	Sender    string
}

// NewPlot constructs an unowned or owned Plot bound to area.
func NewPlot(area *PlotArea, id PlotId, owner uuid.UUID) *Plot {
	var parent *FlagContainer
	if area != nil {
		parent = area.Flags
	} else {
		parent = GlobalFlags
	}
	return &Plot{
		ID:        id,
		Area:      area,
		Owner:     owner,
		Trusted:   make(map[uuid.UUID]struct{}),
		Members:   make(map[uuid.UUID]struct{}),
		Denied:    make(map[uuid.UUID]struct{}),
		Flags:     NewFlagContainer(parent),
		Timestamp: time.Now().UnixMilli(),
	}
}

// HasOwner reports whether this plot is claimed.
func (p *Plot) HasOwner() bool {
	return p != nil && p.Owner != uuid.Nil
}

// IsOwner reports whether u owns this plot (or any connected plot if merged).
func (p *Plot) IsOwner(u uuid.UUID) bool {
	if p == nil || u == uuid.Nil {
		return false
	}
	if p.Owner == u {
		return true
	}
	if p.IsMerged() {
		for _, cp := range p.ConnectedPlots() {
			if cp.Owner == u {
				return true
			}
		}
	}
	return false
}

// IsTrusted reports whether u (or EveryoneUUID) is trusted on this plot.
func (p *Plot) IsTrusted(u uuid.UUID) bool {
	if p == nil {
		return false
	}
	base := p.BasePlot()
	if _, ok := base.Trusted[u]; ok {
		return true
	}
	_, ok := base.Trusted[EveryoneUUID]
	return ok
}

// IsMember reports whether u (or EveryoneUUID) is a member on this plot.
func (p *Plot) IsMember(u uuid.UUID) bool {
	if p == nil {
		return false
	}
	base := p.BasePlot()
	if _, ok := base.Members[u]; ok {
		return true
	}
	_, ok := base.Members[EveryoneUUID]
	return ok
}

// IsDenied reports whether u (or EveryoneUUID, unless owner/added) is denied from this plot.
func (p *Plot) IsDenied(u uuid.UUID) bool {
	if p == nil || p.IsOwner(u) || p.IsTrusted(u) || p.IsMember(u) {
		return false
	}
	base := p.BasePlot()
	if _, ok := base.Denied[u]; ok {
		return true
	}
	_, ok := base.Denied[EveryoneUUID]
	return ok
}

// IsAdded reports whether u is owner, trusted, or member of this plot.
func (p *Plot) IsAdded(u uuid.UUID) bool {
	if p == nil || !p.HasOwner() {
		return false
	}
	return p.IsOwner(u) || p.IsTrusted(u) || p.IsMember(u)
}

// IsMerged reports whether this plot is merged in any direction.
func (p *Plot) IsMerged() bool {
	if p == nil {
		return false
	}
	return p.Settings.Merged[0] || p.Settings.Merged[1] || p.Settings.Merged[2] || p.Settings.Merged[3]
}

// IsMergedDir reports whether this plot is merged in direction d (0..3).
func (p *Plot) IsMergedDir(d Direction) bool {
	if p == nil || d < 0 || d > 3 {
		return false
	}
	return p.Settings.Merged[d]
}

// ConnectedPlots returns all plots in the same merged group (including p), sorted by (X, Y).
func (p *Plot) ConnectedPlots() []*Plot {
	if p == nil {
		return nil
	}
	if !p.IsMerged() || p.Area == nil {
		return []*Plot{p}
	}
	visited := map[PlotId]*Plot{p.ID: p}
	queue := []*Plot{p}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dir := range CardinalDirections {
			if !cur.Settings.Merged[dir] {
				continue
			}
			nid := cur.ID.Relative(dir)
			if _, seen := visited[nid]; seen {
				continue
			}
			if nb := p.Area.PlotAbs(nid); nb != nil {
				visited[nid] = nb
				queue = append(queue, nb)
			}
		}
	}
	out := make([]*Plot, 0, len(visited))
	for _, cp := range visited {
		out = append(out, cp)
	}
	slices.SortFunc(out, func(a, b *Plot) int {
		if a.ID.X != b.ID.X {
			return a.ID.X - b.ID.X
		}
		return a.ID.Y - b.ID.Y
	})
	return out
}

// BasePlot returns the canonical base plot of a merged group (lowest X, then lowest Y).
func (p *Plot) BasePlot() *Plot {
	if p == nil || !p.IsMerged() {
		return p
	}
	connected := p.ConnectedPlots()
	if len(connected) == 0 {
		return p
	}
	return connected[0]
}

// IsBasePlot reports whether p is the canonical base plot of its group.
func (p *Plot) IsBasePlot() bool {
	if p == nil {
		return false
	}
	return p.BasePlot() == p
}

// Owners returns all distinct owners across this plot's connected group.
func (p *Plot) Owners() []uuid.UUID {
	if p == nil {
		return nil
	}
	seen := make(map[uuid.UUID]struct{})
	var out []uuid.UUID
	for _, cp := range p.ConnectedPlots() {
		if cp.Owner == uuid.Nil {
			continue
		}
		if _, ok := seen[cp.Owner]; !ok {
			seen[cp.Owner] = struct{}{}
			out = append(out, cp.Owner)
		}
	}
	return out
}

// FlagValue returns the effective value of flag name on this plot (falling back to area and global defaults).
func (p *Plot) FlagValue(name string) any {
	if p == nil {
		return GlobalFlags.GetValue(name)
	}
	base := p.BasePlot()
	if base.Flags != nil {
		return base.Flags.GetValue(name)
	}
	if base.Area != nil && base.Area.Flags != nil {
		return base.Area.Flags.GetValue(name)
	}
	return GlobalFlags.GetValue(name)
}

// FlagBool is a convenience helper to read a boolean flag on p.
func (p *Plot) FlagBool(name string) bool {
	v, _ := p.FlagValue(name).(bool)
	return v
}

// FlagInt is a convenience helper to read an int flag on p.
func (p *Plot) FlagInt(name string) int {
	v, _ := p.FlagValue(name).(int)
	return v
}

// FlagString is a convenience helper to read a string flag on p.
func (p *Plot) FlagString(name string) string {
	v, _ := p.FlagValue(name).(string)
	return v
}

// AverageRating returns the mean rating (0 if unrated) and rating count.
func (p *Plot) AverageRating() (float64, int) {
	if p == nil {
		return 0, 0
	}
	base := p.BasePlot()
	if len(base.Settings.Ratings) == 0 {
		return 0, 0
	}
	var sum int
	for _, r := range base.Settings.Ratings {
		sum += r
	}
	return float64(sum) / float64(len(base.Settings.Ratings)), len(base.Settings.Ratings)
}

// LikeCount returns likes and dislikes (ratings >= 10 are likes, <= 1 are dislikes, matching upstream).
func (p *Plot) LikeCount() (likes, dislikes int) {
	if p == nil {
		return 0, 0
	}
	base := p.BasePlot()
	for _, r := range base.Settings.Ratings {
		if r >= 10 {
			likes++
		} else {
			dislikes++
		}
	}
	return likes, dislikes
}

// String formats the plot as "area;x;y".
func (p *Plot) String() string {
	if p == nil {
		return "<nil>"
	}
	if p.Area == nil {
		return p.ID.String()
	}
	return p.Area.FullID() + ";" + p.ID.String()
}

// CloneSets returns copies of Trusted, Members, Denied for safe iteration.
func (p *Plot) CloneSets() (trusted, members, denied map[uuid.UUID]struct{}) {
	return maps.Clone(p.Trusted), maps.Clone(p.Members), maps.Clone(p.Denied)
}

// PlotCluster is a named rectangular group of plots (pos1..pos2) owned by a player.
type PlotCluster struct {
	DBID     int64
	Area     *PlotArea
	Pos1     PlotId
	Pos2     PlotId
	Owner    uuid.UUID
	Helpers  map[uuid.UUID]struct{}
	Invited  map[uuid.UUID]struct{}
	Settings PlotSettings
	Name     string
}

// Contains reports whether id falls inside this cluster's bounds (inclusive).
func (c *PlotCluster) Contains(id PlotId) bool {
	if c == nil {
		return false
	}
	return id.X >= c.Pos1.X && id.X <= c.Pos2.X && id.Y >= c.Pos1.Y && id.Y <= c.Pos2.Y
}

// Intersects reports whether two clusters overlap.
func (c *PlotCluster) Intersects(pos1, pos2 PlotId) bool {
	return c.Pos1.X <= pos2.X && c.Pos2.X >= pos1.X && c.Pos1.Y <= pos2.Y && c.Pos2.Y >= pos1.Y
}

// AreaCount returns the number of plots covered by the cluster.
func (c *PlotCluster) AreaCount() int {
	return (c.Pos2.X - c.Pos1.X + 1) * (c.Pos2.Y - c.Pos1.Y + 1)
}

// IsAdded reports whether u is owner, helper, or invited to the cluster.
func (c *PlotCluster) IsAdded(u uuid.UUID) bool {
	if c == nil {
		return false
	}
	if c.Owner == u {
		return true
	}
	if _, ok := c.Helpers[u]; ok {
		return true
	}
	if _, ok := c.Invited[u]; ok {
		return true
	}
	_, ok := c.Invited[EveryoneUUID]
	return ok
}

// BlockBucketEntry is one weighted block state entry in a BlockBucket pattern.
type BlockBucketEntry struct {
	Block  string // normalized block string (e.g. "minecraft:stone" or "minecraft:oak_slab[type=bottom]")
	Weight int
}

// BlockBucket is PlotSquared's weighted block pattern string, e.g. "stone", "50%stone,50%cobblestone".
type BlockBucket struct {
	Raw     string
	Entries []BlockBucketEntry
	Total   int
}

// ParseBlockBucket parses a BlockBucket string such as "stone" or "80%stone,20%cobblestone".
func ParseBlockBucket(raw string) (BlockBucket, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return BlockBucket{Raw: "air", Entries: []BlockBucketEntry{{Block: "minecraft:air", Weight: 1}}, Total: 1}, nil
	}
	parts := splitOutsideBrackets(raw, ',')
	entries := make([]BlockBucketEntry, 0, len(parts))
	total := 0
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		weight := 1
		blockStr := p
		if idx := strings.IndexByte(p, '%'); idx > 0 {
			w, err := strconv.Atoi(strings.TrimSpace(p[:idx]))
			if err != nil || w <= 0 {
				return BlockBucket{}, fmt.Errorf("invalid weight in block bucket %q", raw)
			}
			weight = w
			blockStr = strings.TrimSpace(p[idx+1:])
		}
		if blockStr == "" {
			return BlockBucket{}, fmt.Errorf("empty block in block bucket %q", raw)
		}
		if !strings.Contains(blockStr, ":") {
			blockStr = "minecraft:" + strings.ToLower(blockStr)
		} else {
			blockStr = strings.ToLower(blockStr)
		}
		entries = append(entries, BlockBucketEntry{Block: blockStr, Weight: weight})
		total += weight
	}
	if len(entries) == 0 {
		return BlockBucket{}, fmt.Errorf("empty block bucket %q", raw)
	}
	return BlockBucket{Raw: raw, Entries: entries, Total: total}, nil
}

// MustBlockBucket parses raw or panics.
func MustBlockBucket(raw string) BlockBucket {
	b, err := ParseBlockBucket(raw)
	if err != nil {
		panic(err)
	}
	return b
}

// String returns the canonical string form of b.
func (b BlockBucket) String() string {
	if b.Raw != "" {
		return b.Raw
	}
	if len(b.Entries) == 1 && b.Entries[0].Weight == 1 {
		return b.Entries[0].Block
	}
	var sb strings.Builder
	for i, e := range b.Entries {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%d%%%s", e.Weight, e.Block)
	}
	return sb.String()
}

// splitOutsideBrackets splits s on sep only when not inside '[' ... ']'.
func splitOutsideBrackets(s string, sep byte) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}

// PlotComponent identifies one of the customizable plot components for /plot set.
type PlotComponent string

const (
	ComponentFloor   PlotComponent = "floor"
	ComponentMain    PlotComponent = "main"
	ComponentWall    PlotComponent = "wall"
	ComponentBorder  PlotComponent = "border"
	ComponentAll     PlotComponent = "all"
	ComponentAir     PlotComponent = "air"
	ComponentOutline PlotComponent = "outline"
	ComponentMiddle  PlotComponent = "middle"
)

// RegionBounds is an inclusive axis-aligned 3D block bounding box in a world.
type RegionBounds struct {
	Min plugin.BlockPos
	Max plugin.BlockPos
}

// Contains reports whether pos lies inside b (inclusive).
func (b RegionBounds) Contains(pos plugin.BlockPos) bool {
	return pos.X >= b.Min.X && pos.X <= b.Max.X &&
		pos.Y >= b.Min.Y && pos.Y <= b.Max.Y &&
		pos.Z >= b.Min.Z && pos.Z <= b.Max.Z
}

// ContainsXZ reports whether (x, z) lies inside b's horizontal extent (inclusive).
func (b RegionBounds) ContainsXZ(x, z int) bool {
	return x >= b.Min.X && x <= b.Max.X && z >= b.Min.Z && z <= b.Max.Z
}
