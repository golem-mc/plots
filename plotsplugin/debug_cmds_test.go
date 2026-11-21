package plotsplugin_test

import (
	"testing"

	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/plotsplugin"
)

func TestDebugAllowUnsafeExecLoadAndSaveTests(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Admin"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	area, _ := setupCompactPlotWorld(t, srv, ps)
	admin := srv.Join("Admin")
	alice := srv.Join("Alice")
	p1, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot: %v", err)
	}

	// debugallowunsafe toggles per player.
	u := admin.Handle().UUID()
	mustDispatch(t, host, admin.Handle(), "p debugallowunsafe")
	if !ps.UnsafeAllowed(u) {
		t.Error("unsafe not allowed after first /p debugallowunsafe")
	}
	mustDispatch(t, host, admin.Handle(), "p debugallowunsafe")
	if ps.UnsafeAllowed(u) {
		t.Error("unsafe still allowed after second /p debugallowunsafe")
	}
	// Players without the node are refused.
	mustDispatch(t, host, alice.Handle(), "p debugallowunsafe")
	if ps.UnsafeAllowed(alice.Handle().UUID()) {
		t.Error("alice toggled unsafe without plots.debugallowunsafe")
	}

	// debugexec remove-flag clears the flag on every plot.
	if _, err := ps.SetFlag(p1, "greeting", "hello"); err != nil {
		t.Fatalf("SetFlag: %v", err)
	}
	mustDispatch(t, host, admin.Handle(), "p debugexec remove-flag greeting")
	if p1.Flags.HasLocal("greeting") {
		t.Error("greeting still set after debugexec remove-flag")
	}

	// debugexec start-expire / stop-expire / stop-rgar run without a task.
	mustDispatch(t, host, admin.Handle(), "p exec start-expire")
	mustDispatch(t, host, admin.Handle(), "p exec stop-expire")
	mustDispatch(t, host, admin.Handle(), "p exec stop-expire")
	mustDispatch(t, host, admin.Handle(), "p exec stop-rgar")
	mustDispatch(t, host, admin.Handle(), "p exec calibrate-analysis 50")

	// debugsavetest and debugloadtest are console-only.
	if _, err := ps.SetFlag(p1, "greeting", "hi"); err != nil {
		t.Fatalf("SetFlag: %v", err)
	}
	if _, err := ps.DB.LoadPlots(); err != nil {
		t.Fatalf("LoadPlots: %v", err)
	}
	mustDispatch(t, host, host.Console(), "p debugsavetest")
	mustDispatch(t, host, host.Console(), "p debugloadtest")
	loaded, err := ps.DB.LoadPlots()
	if err != nil {
		t.Fatalf("LoadPlots: %v", err)
	}
	got := loaded[area.FullID()][p1.ID]
	if got == nil || got.Owner != alice.Handle().UUID() || got.FlagString("greeting") != "hi" {
		t.Errorf("plot after debugsavetest = %+v, want owner alice with greeting=hi", got)
	}
	n := 0
	for _, m := range loaded {
		n += len(m)
	}
	if n != 1 {
		t.Errorf("plots in DB after debugsavetest = %d, want 1 (no duplicates)", n)
	}
}
