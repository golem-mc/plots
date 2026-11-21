// Package expiry implements plot complexity analysis and automatic plot
// expiration matching upstream PlotSquared's ExpireManager & PlotAnalysis.
package expiry

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/config"
	"github.com/google/uuid"
)

// Analysis summarizes a plot's build complexity metrics (upstream PlotAnalysis).
type Analysis struct {
	Changes int
	Faces   int
	Data    int
	Air     int
	Variety int
}

// Complexity evaluates the calibrated complexity score for cal.
func (a Analysis) Complexity(cal config.AutoClearCalibration) int {
	return a.Changes*cal.Changes +
		a.Faces*cal.Faces +
		a.Data*cal.Data +
		a.Air*cal.Air +
		a.Variety*cal.Variety
}

// AnalyzePlot computes block-change and variety statistics for plot in w.
func AnalyzePlot(w plugin.World, area *plots.PlotArea, plot *plots.Plot) Analysis {
	if w == nil || area == nil || plot == nil {
		return Analysis{}
	}
	var a Analysis
	seen := make(map[registry.Block]struct{})
	minY := max(area.MinBuildHeight, w.MinY())
	maxY := min(area.MaxBuildHeight, w.MinY()+w.Height()-1)

	for _, reg := range area.PlotRegions(plot.BasePlot()) {
		for x := reg.Min.X; x <= reg.Max.X; x++ {
			for z := reg.Min.Z; z <= reg.Max.Z; z++ {
				for y := minY; y <= maxY; y++ {
					st := w.BlockState(plugin.BlockPos{X: x, Y: y, Z: z})
					blk := st.Block()
					if blk == registry.BlockAir || blk == registry.BlockCaveAir || blk == registry.BlockVoidAir {
						if y <= area.PlotHeight {
							a.Changes++
							a.Air++
						}
						continue
					}
					if y > area.PlotHeight {
						a.Changes++
						seen[blk] = struct{}{}
					} else if y == area.PlotHeight {
						if blk != registry.BlockGrassBlock {
							a.Changes++
							seen[blk] = struct{}{}
						}
					} else if blk != registry.BlockStone && blk != registry.BlockBedrock {
						a.Changes++
						seen[blk] = struct{}{}
					}
				}
			}
		}
	}
	a.Variety = len(seen)
	return a
}

// Manager tracks player activity timestamps and evaluates plot expiration rules.
type Manager struct {
	mu       sync.RWMutex
	lastSeen map[uuid.UUID]time.Time
}

// NewManager creates an expiry manager.
func NewManager() *Manager {
	return &Manager{lastSeen: make(map[uuid.UUID]time.Time)}
}

// TouchPlayer records player activity at t.
func (m *Manager) TouchPlayer(u uuid.UUID, t time.Time) {
	m.mu.Lock()
	m.lastSeen[u] = t
	m.mu.Unlock()
}

// LastSeen returns the recorded last activity timestamp for u, if any.
func (m *Manager) LastSeen(u uuid.UUID) (time.Time, bool) {
	m.mu.RLock()
	t, ok := m.lastSeen[u]
	m.mu.RUnlock()
	return t, ok
}

// IsPlotExpired evaluates whether plot has expired under task at time now.
func (m *Manager) IsPlotExpired(plot *plots.Plot, task config.AutoClearTask, now time.Time) bool {
	if plot == nil || !plot.HasOwner() {
		return false
	}
	base := plot.BasePlot()
	if base.FlagBool("server-plot") {
		return false
	}
	if kv, ok := base.FlagValue("keep").(plots.KeepValue); ok && kv.IsKept(now) {
		return false
	}
	if len(task.Worlds) > 0 && !slices.Contains(task.Worlds, "*") {
		found := false
		for _, wn := range task.Worlds {
			if strings.EqualFold(wn, base.Area.WorldName) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if task.Days <= 0 {
		return false
	}
	// Determine most recent activity among owners (from lastSeen or plot timestamp).
	var latest time.Time
	for _, owner := range base.Owners() {
		if ts, ok := m.LastSeen(owner); ok && ts.After(latest) {
			latest = ts
		}
	}
	if latest.IsZero() && base.Timestamp > 0 {
		latest = time.UnixMilli(base.Timestamp)
	}
	if latest.IsZero() {
		return task.DeleteIfOwnerIsUnknown
	}
	return now.Sub(latest) >= time.Duration(task.Days)*24*time.Hour
}

// ExpiredPlots returns all expired base plots across areas according to tasks.
func (m *Manager) ExpiredPlots(areas []*plots.PlotArea, tasks map[string]config.AutoClearTask, now time.Time) []*plots.Plot {
	if len(tasks) == 0 {
		return nil
	}
	seen := make(map[*plots.Plot]bool)
	var out []*plots.Plot
	for _, a := range areas {
		for _, p := range a.BasePlots() {
			if seen[p] {
				continue
			}
			for _, task := range tasks {
				if m.IsPlotExpired(p, task, now) {
					seen[p] = true
					out = append(out, p)
					break
				}
			}
		}
	}
	return out
}
