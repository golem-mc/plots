// Package generator implements PlotSquared's HybridPlotWorld/ClassicPlotWorld
// chunk generator, road/plot schematic overlays, and plot block operations.
package generator

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plotsquared"
	"github.com/golem-mc/worldedit/block"
)

// CompiledEntry is a resolved block state with its cumulative weight.
type CompiledEntry struct {
	State  plugin.BlockState
	Weight int
}

// CompiledPattern is a pre-parsed BlockBucket ready for fast lock-free chunk generation.
type CompiledPattern struct {
	Entries []CompiledEntry
	Total   int
}

// CompileBucket resolves every block entry in bucket into a plugin.BlockState.
// Falls back to fallback if bucket is empty or invalid.
func CompileBucket(bucket plotsquared.BlockBucket, fallback registry.Block) CompiledPattern {
	entries := make([]CompiledEntry, 0, len(bucket.Entries))
	total := 0
	for _, e := range bucket.Entries {
		st, ok := ParseBlockSpec(e.Block)
		if !ok {
			continue
		}
		w := e.Weight
		if w <= 0 {
			w = 1
		}
		entries = append(entries, CompiledEntry{State: st, Weight: w})
		total += w
	}
	if len(entries) == 0 {
		return CompiledPattern{
			Entries: []CompiledEntry{{State: plugin.DefaultState(fallback), Weight: 1}},
			Total:   1,
		}
	}
	return CompiledPattern{Entries: entries, Total: total}
}

// ParseBlockSpec resolves a block state string (modern name, properties, or legacy id:data).
func ParseBlockSpec(spec string) (plugin.BlockState, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, false
	}
	if st, err := block.ParseState(spec, true); err == nil {
		return st, true
	}
	if st, ok := block.LegacyBlockState(spec); ok {
		return st, true
	}
	// Handle older Bukkit/PlotSquared aliases
	low := strings.ToLower(strings.TrimPrefix(spec, "minecraft:"))
	switch low {
	case "stone_slab":
		return plugin.DefaultState(registry.BlockSmoothStoneSlab), true
	case "grass":
		return plugin.DefaultState(registry.BlockShortGrass), true
	}
	return 0, false
}

// Pick chooses a block state using rng.
func (p CompiledPattern) Pick(rng plugin.Random) plugin.BlockState {
	if len(p.Entries) == 1 {
		return p.Entries[0].State
	}
	r := rng.Int(p.Total)
	for _, e := range p.Entries {
		r -= e.Weight
		if r < 0 {
			return e.State
		}
	}
	return p.Entries[0].State
}

// PickCoord chooses a block state deterministically/pseudo-randomly for live world ops.
func (p CompiledPattern) PickCoord(x, y, z int) plugin.BlockState {
	if len(p.Entries) == 1 {
		return p.Entries[0].State
	}
	h := uint64(x)*73856093 ^ uint64(y)*19349663 ^ uint64(z)*83492791
	r := int(h % uint64(p.Total))
	for _, e := range p.Entries {
		r -= e.Weight
		if r < 0 {
			return e.State
		}
	}
	return p.Entries[0].State
}

// PickRand chooses a block state using math/rand/v2.
func (p CompiledPattern) PickRand() plugin.BlockState {
	if len(p.Entries) == 1 {
		return p.Entries[0].State
	}
	r := rand.IntN(p.Total)
	for _, e := range p.Entries {
		r -= e.Weight
		if r < 0 {
			return e.State
		}
	}
	return p.Entries[0].State
}

// ResolveBiome resolves a biome name like "forest" or "minecraft:plains" to a registry.Biome.
func ResolveBiome(name string) (registry.Biome, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return registry.BiomeForest, true
	}
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}
	b, ok := registry.BiomeByName(name)
	if !ok {
		return registry.BiomeForest, false
	}
	return b, true
}

// ApplyGeneratorSpec applies a Bukkit generator argument string such as
// "s=42,g=7,h=62,f=grass_block,m=stone,w=stone,b=smooth_stone_slab" to area.
// Matches upstream PlotSquared.java setupWorldArgs.
func ApplyGeneratorSpec(area *plotsquared.PlotArea, spec string) error {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	// Tokens may have commas inside block patterns (e.g. f=50%stone,50%dirt,g=7).
	// Split on commas that precede a known "key=" pair.
	knownKeys := []string{"s=", "size=", "g=", "gap=", "h=", "height=", "minh=", "minheight=", "maxh=", "maxheight=", "f=", "floor=", "m=", "main=", "w=", "wall=", "b=", "border="}
	rawParts := strings.Split(spec, ",")
	var pairs []string
	var cur strings.Builder
	for _, part := range rawParts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		low := strings.ToLower(p)
		isNewKey := false
		for _, k := range knownKeys {
			if strings.HasPrefix(low, k) {
				isNewKey = true
				break
			}
		}
		if isNewKey && cur.Len() > 0 {
			pairs = append(pairs, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(',')
		}
		cur.WriteString(p)
	}
	if cur.Len() > 0 {
		pairs = append(pairs, cur.String())
	}

	for _, pair := range pairs {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return fmt.Errorf("invalid generator argument %q (expected key=value)", pair)
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		switch k {
		case "s", "size":
			var n int
			if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 1 {
				return fmt.Errorf("invalid plot size %q", v)
			}
			area.PlotWidth = n
		case "g", "gap":
			var n int
			if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 0 {
				return fmt.Errorf("invalid road gap %q", v)
			}
			area.RoadWidth = n
		case "h", "height":
			var n int
			if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
				return fmt.Errorf("invalid height %q", v)
			}
			area.PlotHeight = n
			area.RoadHeight = n
			area.WallHeight = n
		case "minh", "minheight":
			var n int
			if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
				return fmt.Errorf("invalid minheight %q", v)
			}
			area.MinGenHeight = n
		case "maxh", "maxheight":
			var n int
			if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
				return fmt.Errorf("invalid maxheight %q", v)
			}
			area.MaxGenHeight = n
		case "f", "floor":
			b, err := plotsquared.ParseBlockBucket(v)
			if err != nil {
				return err
			}
			area.TopBlock = b
		case "m", "main":
			b, err := plotsquared.ParseBlockBucket(v)
			if err != nil {
				return err
			}
			area.MainBlock = b
		case "w", "wall":
			b, err := plotsquared.ParseBlockBucket(v)
			if err != nil {
				return err
			}
			area.WallFilling = b
		case "b", "border":
			b, err := plotsquared.ParseBlockBucket(v)
			if err != nil {
				return err
			}
			area.WallBlock = b
		default:
			return fmt.Errorf("unknown generator argument %q", k)
		}
	}
	area.ComputeGeometry()
	return nil
}
