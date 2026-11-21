package plots_test

import (
	"math"
	"testing"
	"time"

	"github.com/golem-mc/plots"
	"github.com/google/uuid"
)

func TestPlotIdParsingAndSpiral(t *testing.T) {
	cases := []struct {
		in   string
		want plots.PlotId
		ok   bool
	}{
		{"1;2", plots.PlotId{X: 1, Y: 2}, true},
		{"-3,4", plots.PlotId{X: -3, Y: 4}, true},
		{"0_0", plots.PlotId{X: 0, Y: 0}, true},
		{"5.-6", plots.PlotId{X: 5, Y: -6}, true},
		{"invalid", plots.PlotId{}, false},
		{"1", plots.PlotId{}, false},
	}
	for _, tc := range cases {
		got, ok := plots.ParsePlotId(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParsePlotId(%q) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}

	// Verify spiral visits unique PlotIds without duplicates in the first 100 steps.
	seen := make(map[plots.PlotId]bool)
	cur := plots.PlotId{X: 0, Y: 0}
	for i := range 100 {
		cur = cur.NextId()
		if seen[cur] {
			t.Fatalf("spiral revisited %v at step %d", cur, i)
		}
		seen[cur] = true
	}

	// HashCode matches Java (x << 16) | (y & 0xFFFF)
	id := plots.PlotId{X: 2, Y: -1}
	wantHash := int32((2 << 16) | (0xFFFF))
	if got := id.HashCode(); got != wantHash {
		t.Errorf("HashCode = %d, want %d", got, wantHash)
	}
}

func TestHashMergedRoundTrip(t *testing.T) {
	for mask := range 16 {
		arr := plots.UnhashMerged(mask)
		if got := plots.HashMerged(arr); got != mask {
			t.Errorf("mask %d -> %v -> %d", mask, arr, got)
		}
	}
	// North=bit3 (8), East=bit2 (4), South=bit1 (2), West=bit0 (1)
	if got := plots.HashMerged([4]bool{true, false, false, false}); got != 8 {
		t.Errorf("North hash = %d, want 8", got)
	}
}

func TestPlotAreaGeometryOddAndEvenRoads(t *testing.T) {
	for _, rw := range []int{7, 6, 0} {
		area := plots.DefaultPlotArea("plotworld", "")
		area.PlotWidth = 42
		area.RoadWidth = rw
		area.ComputeGeometry()

		for _, id := range []plots.PlotId{{X: 1, Y: 1}, {X: 0, Y: 0}, {X: -2, Y: 3}} {
			b := area.PlotInteriorBounds(id)
			if b.Max.X-b.Min.X+1 != 42 || b.Max.Z-b.Min.Z+1 != 42 {
				t.Fatalf("rw=%d id=%v: bounds width=%d length=%d, want 42x42", rw, id, b.Max.X-b.Min.X+1, b.Max.Z-b.Min.Z+1)
			}
			// Interior corners must map back to id
			for _, pt := range [][2]int{{b.Min.X, b.Min.Z}, {b.Max.X, b.Max.Z}, {b.Min.X, b.Max.Z}, {b.Max.X, b.Min.Z}} {
				gotID, ok := area.PlotIdAt(pt[0], pt[1])
				if !ok || gotID != id {
					t.Errorf("rw=%d PlotIdAt(%d,%d) = (%v,%v), want (%v,true)", rw, pt[0], pt[1], gotID, ok, id)
				}
			}
			// 1 block outside must be wall/road when rw > 0
			if rw > 0 {
				if _, ok := area.PlotIdAt(b.Min.X-1, b.Min.Z); ok {
					t.Errorf("rw=%d expected wall at (%d,%d)", rw, b.Min.X-1, b.Min.Z)
				}
				if _, ok := area.PlotIdAt(b.Max.X+1, b.Max.Z); ok {
					t.Errorf("rw=%d expected wall at (%d,%d)", rw, b.Max.X+1, b.Max.Z)
				}
			}
		}
	}
}

func TestPlotAtMergedRoadsAndIntersection(t *testing.T) {
	area := plots.DefaultPlotArea("plots", "")
	owner := uuid.New()
	p11 := plots.NewPlot(area, plots.PlotId{X: 1, Y: 1}, owner)
	p21 := plots.NewPlot(area, plots.PlotId{X: 2, Y: 1}, owner)
	p12 := plots.NewPlot(area, plots.PlotId{X: 1, Y: 2}, owner)
	p22 := plots.NewPlot(area, plots.PlotId{X: 2, Y: 2}, owner)

	// Merge 2x2 quad
	p11.Settings.Merged[plots.DirEast] = true
	p11.Settings.Merged[plots.DirSouth] = true
	p21.Settings.Merged[plots.DirWest] = true
	p21.Settings.Merged[plots.DirSouth] = true
	p12.Settings.Merged[plots.DirNorth] = true
	p12.Settings.Merged[plots.DirEast] = true
	p22.Settings.Merged[plots.DirNorth] = true
	p22.Settings.Merged[plots.DirWest] = true
	for _, p := range []*plots.Plot{p11, p21, p12, p22} {
		area.SetPlot(p)
	}

	b11 := area.PlotInteriorBounds(p11.ID)
	// East road strip between (1;1) and (2;1)
	if got := area.PlotAt(b11.Max.X+2, b11.Min.Z+5); got == nil || got.ID != p11.ID {
		t.Errorf("east merged road PlotAt = %v, want %v", got, p11.ID)
	}
	// South road strip between (1;1) and (1;2)
	if got := area.PlotAt(b11.Min.X+5, b11.Max.Z+2); got == nil || got.ID != p11.ID {
		t.Errorf("south merged road PlotAt = %v, want %v", got, p11.ID)
	}
	// 2x2 intersection center
	if got := area.PlotAt(b11.Max.X+2, b11.Max.Z+2); got == nil || got.ID != p11.ID {
		t.Errorf("2x2 intersection PlotAt = %v, want %v", got, p11.ID)
	}
	// Outer road west of (1;1) must be nil
	if got := area.PlotAt(b11.Min.X-2, b11.Min.Z+5); got != nil {
		t.Errorf("unmerged west road PlotAt = %v, want nil", got)
	}
	if regs := area.PlotRegions(p11); len(regs) != 9 {
		// 4 plots + 2 east strips + 2 south strips + 1 intersection = 9 regions
		t.Errorf("PlotRegions len = %d, want 9", len(regs))
	}
}

func TestFlagsRegistryAndParsing(t *testing.T) {
	if len(plots.AllFlags()) < 90 {
		t.Fatalf("expected >= 90 builtin flags, got %d", len(plots.AllFlags()))
	}

	// Check specific defaults
	if v, _ := plots.GlobalFlags.GetValue("pvp").(bool); v {
		t.Errorf("pvp default = true, want false")
	}
	if v, _ := plots.GlobalFlags.GetValue("redstone").(bool); !v {
		t.Errorf("redstone default = false, want true")
	}
	if v, _ := plots.GlobalFlags.GetValue("entity-cap").(int); v != math.MaxInt32 {
		t.Errorf("entity-cap default = %d, want MaxInt32", v)
	}

	// Inheritance: plot -> area -> global
	area := plots.DefaultPlotArea("w", "")
	area.Flags.Set("pvp", true)
	p := plots.NewPlot(area, plots.PlotId{X: 1, Y: 1}, uuid.New())
	if !p.FlagBool("pvp") {
		t.Errorf("expected plot to inherit area pvp=true")
	}
	if err := p.Flags.ParseAndSet("pvp", "false"); err != nil {
		t.Fatalf("ParseAndSet pvp: %v", err)
	}
	if p.FlagBool("pvp") {
		t.Errorf("expected plot pvp=false to override area")
	}

	// Special flag types: feed, keep, plot-title, music, break
	if err := p.Flags.ParseAndSet("feed", "5 2"); err != nil {
		t.Fatalf("feed parse: %v", err)
	}
	if got, _ := p.FlagValue("feed").(plots.TimedInt); got != (plots.TimedInt{Interval: 5, Value: 2}) {
		t.Errorf("feed = %+v", got)
	}
	if err := p.Flags.ParseAndSet("keep", "2d 3h"); err != nil {
		t.Fatalf("keep parse: %v", err)
	}
	if kv, _ := p.FlagValue("keep").(plots.KeepValue); !kv.IsKept(time.Now()) {
		t.Errorf("expected keep 2d 3h to be kept now")
	}
	if err := p.Flags.ParseAndSet("plot-title", `"Hello" "World"`); err != nil {
		t.Fatalf("plot-title parse: %v", err)
	}
	if pt, _ := p.FlagValue("plot-title").(plots.PlotTitle); pt.Title != "Hello" || pt.Subtitle != "World" {
		t.Errorf("plot-title = %+v", pt)
	}
	if err := p.Flags.ParseAndSet("music", "ward"); err != nil {
		t.Fatalf("music parse: %v", err)
	}
	if got := p.FlagString("music"); got != "minecraft:music_disc_ward" {
		t.Errorf("music = %q, want minecraft:music_disc_ward", got)
	}
	if err := p.Flags.ParseAndSet("break", "stone, #minecraft:logs"); err != nil {
		t.Fatalf("break parse: %v", err)
	}
	if bl, _ := p.FlagValue("break").([]string); len(bl) != 2 || bl[0] != "minecraft:stone" || bl[1] != "#minecraft:logs" {
		t.Errorf("break = %v", bl)
	}
}

func TestBlockBucketAndEvalPrice(t *testing.T) {
	b, err := plots.ParseBlockBucket("70%stone,30%oak_slab[type=bottom]")
	if err != nil {
		t.Fatalf("ParseBlockBucket: %v", err)
	}
	if b.Total != 100 || len(b.Entries) != 2 || b.Entries[1].Block != "minecraft:oak_slab[type=bottom]" {
		t.Errorf("unexpected BlockBucket: %+v", b)
	}
	if got := plots.EvalPrice("100 + 50 * plots", 3); got != 250 {
		t.Errorf("EvalPrice = %v, want 250", got)
	}
	if got := plots.EvalPrice("100 * 2^plots", 2); got != 400 {
		t.Errorf("EvalPrice pow = %v, want 400", got)
	}
}
