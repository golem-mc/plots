package expiry

import (
	"testing"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/config"
	"github.com/golem-mc/plots/internal/generator"
	"github.com/google/uuid"
)

func TestAnalyzePlotAndComplexity(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t)
	host := srv.Host()

	area := plots.DefaultPlotArea("expiryworld", "")
	area.PlotWidth = 6
	area.RoadWidth = 5
	area.PlotHeight = 10
	area.RoadHeight = 10
	area.WallHeight = 10
	area.MinGenHeight = 0
	area.MaxGenHeight = 20
	area.MinBuildHeight = 1
	area.MaxBuildHeight = 20
	area.ComputeGeometry()

	gen := generator.NewHybridGenerator(area, "", nil)
	w, err := host.CreateWorld(plugin.WorldOptions{
		Key:       plugin.MinecraftKey("expiryworld"),
		Dimension: plugin.MinecraftKey("overworld"),
		Generator: gen,
	})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}
	w.AddTicket(plugin.ChunkPos{X: 0, Z: 0}, 2)
	loaded := 0
	w.LoadChunk(plugin.ChunkPos{X: 0, Z: 0}, true, func(plugin.Chunk) { loaded++ })
	srv.RunTicks(5)
	if loaded != 1 {
		t.Fatalf("expected chunk (0,0) loaded")
	}

	p := plots.NewPlot(area, plots.PlotId{X: 1, Y: 1}, uuid.New())
	area.SetPlot(p)
	b := area.PlotInteriorBounds(p.ID)

	// Fresh default plot has zero changes, zero air holes, zero variety
	fresh := AnalyzePlot(w, area, p)
	if fresh.Changes != 0 || fresh.Air != 0 || fresh.Variety != 0 {
		t.Fatalf("fresh plot analysis = %+v, want all zeros", fresh)
	}

	// Build 2 distinct blocks above PlotHeight, replace 1 floor block, dig 1 air hole below PlotHeight
	w.SetBlockState(plugin.BlockPos{X: b.Min.X, Y: 11, Z: b.Min.Z}, plugin.DefaultState(registry.BlockOakPlanks), plugin.UpdateClients)
	w.SetBlockState(plugin.BlockPos{X: b.Min.X + 1, Y: 11, Z: b.Min.Z}, plugin.DefaultState(registry.BlockGoldBlock), plugin.UpdateClients)
	w.SetBlockState(plugin.BlockPos{X: b.Min.X + 2, Y: 10, Z: b.Min.Z}, plugin.DefaultState(registry.BlockDiamondBlock), plugin.UpdateClients)
	w.SetBlockState(plugin.BlockPos{X: b.Min.X + 3, Y: 9, Z: b.Min.Z}, plugin.DefaultState(registry.BlockAir), plugin.UpdateClients)

	got := AnalyzePlot(w, area, p)
	if got.Changes != 4 || got.Air != 1 || got.Variety != 3 {
		t.Errorf("modified plot analysis = %+v, want Changes=4 Air=1 Variety=3", got)
	}
	cal := config.AutoClearCalibration{Changes: 2, Air: 1, Variety: 10}
	if score := got.Complexity(cal); score != 4*2+1*1+3*10 {
		t.Errorf("Complexity = %d, want %d", score, 4*2+1*1+3*10)
	}
}

func TestExpiryRulesAndExpiredPlots(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	oldTime := now.Add(-10 * 24 * time.Hour)
	recentTime := now.Add(-2 * 24 * time.Hour)

	area1 := plots.DefaultPlotArea("plots", "")
	area2 := plots.DefaultPlotArea("other", "")

	alice := uuid.New()
	bob := uuid.New()
	unknown := uuid.New()

	mgr := NewManager()
	mgr.TouchPlayer(alice, oldTime)
	mgr.TouchPlayer(bob, recentTime)
	if ts, ok := mgr.LastSeen(alice); !ok || !ts.Equal(oldTime) {
		t.Fatalf("LastSeen(alice) = (%v, %v), want %v", ts, ok, oldTime)
	}

	task := config.AutoClearTask{
		Days:                   7,
		Threshold:              -1,
		Worlds:                 []string{"plots"},
		DeleteIfOwnerIsUnknown: false,
	}

	// 1. Unowned plot -> never expires
	unowned := plots.NewPlot(area1, plots.PlotId{X: 1, Y: 1}, uuid.Nil)
	if mgr.IsPlotExpired(unowned, task, now) {
		t.Errorf("unowned plot should not expire")
	}

	// 2. Old owner in matching world -> expires
	pOld := plots.NewPlot(area1, plots.PlotId{X: 1, Y: 2}, alice)
	area1.SetPlot(pOld)
	if !mgr.IsPlotExpired(pOld, task, now) {
		t.Errorf("expected Alice's 10-day-old plot to expire under 7-day task")
	}

	// 3. Recent owner -> does not expire
	pRecent := plots.NewPlot(area1, plots.PlotId{X: 1, Y: 3}, bob)
	area1.SetPlot(pRecent)
	if mgr.IsPlotExpired(pRecent, task, now) {
		t.Errorf("Bob's 2-day-old plot should not expire")
	}

	// 4. server-plot=true protects plot from expiry
	pServer := plots.NewPlot(area1, plots.PlotId{X: 1, Y: 4}, alice)
	_ = pServer.Flags.ParseAndSet("server-plot", "true")
	area1.SetPlot(pServer)
	if mgr.IsPlotExpired(pServer, task, now) {
		t.Errorf("server-plot should not expire")
	}

	// 5. keep flag: permanent ("true"), future duration, and expired timestamp
	pKeepPerm := plots.NewPlot(area1, plots.PlotId{X: 1, Y: 5}, alice)
	_ = pKeepPerm.Flags.ParseAndSet("keep", "true")
	area1.SetPlot(pKeepPerm)
	if mgr.IsPlotExpired(pKeepPerm, task, now) {
		t.Errorf("keep=true plot should not expire")
	}

	pKeepFuture := plots.NewPlot(area1, plots.PlotId{X: 1, Y: 6}, alice)
	pKeepFuture.Flags.Set("keep", plots.KeepValue{UntilMs: now.Add(48 * time.Hour).UnixMilli()})
	area1.SetPlot(pKeepFuture)
	if mgr.IsPlotExpired(pKeepFuture, task, now) {
		t.Errorf("plot kept until future date should not expire")
	}

	pKeepPast := plots.NewPlot(area1, plots.PlotId{X: 1, Y: 7}, alice)
	pKeepPast.Flags.Set("keep", plots.KeepValue{UntilMs: now.Add(-1 * time.Hour).UnixMilli()})
	area1.SetPlot(pKeepPast)
	if !mgr.IsPlotExpired(pKeepPast, task, now) {
		t.Errorf("plot whose keep deadline passed should expire")
	}

	// 6. Non-matching world -> does not expire for task scoped to "plots", expires for wildcard "*"
	pOtherWorld := plots.NewPlot(area2, plots.PlotId{X: 1, Y: 1}, alice)
	area2.SetPlot(pOtherWorld)
	if mgr.IsPlotExpired(pOtherWorld, task, now) {
		t.Errorf("plot in 'other' should not match task scoped to 'plots'")
	}
	wildTask := task
	wildTask.Worlds = []string{"*"}
	if !mgr.IsPlotExpired(pOtherWorld, wildTask, now) {
		t.Errorf("plot in 'other' should match wildcard task")
	}

	// 7. Unknown owner (no LastSeen, Timestamp=0) respects DeleteIfOwnerIsUnknown
	pUnknown := plots.NewPlot(area1, plots.PlotId{X: 1, Y: 8}, unknown)
	pUnknown.Timestamp = 0
	if mgr.IsPlotExpired(pUnknown, task, now) {
		t.Errorf("unknown owner should not expire when DeleteIfOwnerIsUnknown=false")
	}
	delUnknownTask := task
	delUnknownTask.DeleteIfOwnerIsUnknown = true
	if !mgr.IsPlotExpired(pUnknown, delUnknownTask, now) {
		t.Errorf("unknown owner should expire when DeleteIfOwnerIsUnknown=true")
	}

	// 8. Fallback to plot.Timestamp when owner not in LastSeen
	pTimestamp := plots.NewPlot(area1, plots.PlotId{X: 1, Y: 9}, unknown)
	pTimestamp.Timestamp = oldTime.UnixMilli()
	if !mgr.IsPlotExpired(pTimestamp, task, now) {
		t.Errorf("plot with old Timestamp should expire even without LastSeen entry")
	}

	// 9. days <= 0 never expires
	zeroDays := task
	zeroDays.Days = 0
	if mgr.IsPlotExpired(pOld, zeroDays, now) {
		t.Errorf("days=0 task should never expire plots")
	}

	// 10. ExpiredPlots across areas with multiple overlapping tasks returns deduplicated set
	expired := mgr.ExpiredPlots([]*plots.PlotArea{area1, area2}, map[string]config.AutoClearTask{
		"task1": task,
		"task2": task, // duplicate task must not duplicate plots
	}, now)
	if len(expired) != 2 {
		t.Errorf("ExpiredPlots returned %d plots, want 2 (pOld and pKeepPast)", len(expired))
	}
}
