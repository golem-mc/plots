package generator

import (
	"testing"

	"github.com/golem-mc/plots"
	"github.com/google/uuid"
)

func TestSafePlotHomeSideSingleAndMerged(t *testing.T) {
	area := plots.DefaultPlotArea("homes", "")
	owner := uuid.New()
	p1 := plots.NewPlot(area, plots.PlotId{X: 1, Y: 1}, owner)
	b1 := area.PlotInteriorBounds(p1.ID)
	loc := SafePlotHome(nil, area, p1, true)
	if want := (b1.Min.X+b1.Max.X)/2 + 0; int(loc.Pos.X) != want || int(loc.Pos.Z) != b1.Min.Z-1 {
		t.Errorf("single home = %v,%v want %d,%d", loc.Pos.X, loc.Pos.Z, want, b1.Min.Z-1)
	}

	p2 := plots.NewPlot(area, plots.PlotId{X: 2, Y: 1}, owner)
	p1.Settings.Merged[plots.DirEast] = true
	p2.Settings.Merged[plots.DirWest] = true
	area.SetPlot(p1)
	area.SetPlot(p2)
	b2 := area.PlotInteriorBounds(p2.ID)
	loc = SafePlotHome(nil, area, p1, true)
	if want := (b1.Min.X + b2.Max.X) / 2; int(loc.Pos.X) != want || int(loc.Pos.Z) != b1.Min.Z-1 {
		t.Errorf("merged home = %v,%v want %d,%d", loc.Pos.X, loc.Pos.Z, want, b1.Min.Z-1)
	}

	p1.Settings.Position = plots.BlockLoc{Set: true, Center: true}
	loc = SafePlotHome(nil, area, p1, true)
	if want := (b1.Min.X + b2.Max.X) / 2; int(loc.Pos.X) != want {
		t.Errorf("center home x = %v want %d", loc.Pos.X, want)
	}
}
