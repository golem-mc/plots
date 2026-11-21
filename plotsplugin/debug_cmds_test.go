package plotsplugin_test

import (
	"strings"
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/plotsplugin"
)

func TestDebugAllowUnsafeExecLoadAndSaveTests(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Admin"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	area, w := setupCompactPlotWorld(t, srv, ps)
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

	for _, id := range []plots.PlotId{{X: 3, Y: 1}, {X: 5, Y: 1}} {
		if _, err := ps.ClaimPlot(area, id, alice.Handle().UUID(), alice.Handle(), false); err != nil {
			t.Fatalf("ClaimPlot %v: %v", id, err)
		}
	}

	// debugexec remove-flag clears the flag on every plot.
	if _, err := ps.SetFlag(p1, "greeting", "hello"); err != nil {
		t.Fatalf("SetFlag: %v", err)
	}
	mustDispatch(t, host, admin.Handle(), "p debugexec remove-flag greeting")
	if p1.Flags.HasLocal("greeting") {
		t.Error("greeting still set after debugexec remove-flag")
	}

	// debugexec analyze reports the changes per column and that it is done.
	bb := area.PlotInteriorBounds(p1.ID)
	admin.Handle().Teleport(plugin.Location{World: w, Pos: plugin.Vec3{X: float64(bb.Min.X + 2), Y: 63, Z: float64(bb.Min.Z + 2)}}, plugin.TeleportPlugin)
	srv.RunTicks(2)
	admin.Messages(1)
	mustDispatch(t, host, admin.Handle(), "p exec analyze")
	out := strings.Join(admin.Messages(1), "\n")
	for _, want := range []string{"Changes/column:", "Done. Use /plot debugexec analyze"} {
		if !strings.Contains(out, want) {
			t.Errorf("debugexec analyze messages %q lack %q", out, want)
		}
	}

	// debugexec expiry task messages.
	expect := func(cmd, want string) {
		t.Helper()
		mustDispatch(t, host, admin.Handle(), cmd)
		if got := strings.Join(admin.Messages(1), "\n"); !strings.Contains(got, want) {
			t.Errorf("%q sent %q, want it to contain %q", cmd, got, want)
		}
	}
	expect("p exec stop-expire", "Task already halted.")
	expect("p exec start-expire", "Started plot expiry task.")
	expect("p exec start-expire", "already started")
	expect("p exec stop-expire", "Task cancelled.")
	expect("p exec stop-expire", "Task already halted.")
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
	if n != 3 {
		t.Errorf("plots in DB after debugsavetest = %d, want 3 (no duplicates)", n)
	}

	// debugexec start-rgar regenerates four chunks per tick.
	srv.HoldTicks()
	// With ticks held a command that hands work to the main thread waits for
	// the next tick, so each runs in a goroutine the test advances.
	expect = func(cmd, want string) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = host.Dispatch(admin.Handle(), cmd)
		}()
		srv.RunTicks(0)
		<-done
		if got := strings.Join(admin.Messages(0), "\n"); !strings.Contains(got, want) {
			t.Errorf("%q sent %q, want it to contain %q", cmd, got, want)
		}
	}
	expect("p exec stop-rgar", "Task not running.")
	expect("p exec start-rgar plotworld", "PlotSquared will now attempt to generate roads")
	expect("p exec start-rgar plotworld", "Cannot schedule mass schematic update")
	expect("p exec stop-rgar", "Task cancelled.")
	expect("p exec start-rgar plotworld", "PlotSquared will now attempt to generate roads")
	chunks := map[[2]int]bool{}
	for _, pl := range area.BasePlots() {
		b := area.PlotInteriorBounds(pl.ID)
		for cx := (b.Min.X - area.RoadWidth) >> 4; cx <= (b.Max.X+area.RoadWidth)>>4; cx++ {
			for cz := (b.Min.Z - area.RoadWidth) >> 4; cz <= (b.Max.Z+area.RoadWidth)>>4; cz++ {
				chunks[[2]int{cx, cz}] = true
			}
		}
	}
	if len(chunks) <= 4 {
		t.Fatalf("%d road chunks, need more than 4 to see batching", len(chunks))
	}
	ticks := (len(chunks) + 3) / 4
	srv.RunTicks(ticks - 1)
	expect("p exec start-rgar plotworld", "Cannot schedule mass schematic update")
	srv.RunTicks(1)
	expect("p exec start-rgar plotworld", "PlotSquared will now attempt to generate roads")
	expect("p exec stop-rgar", "Task cancelled.")

	// Leave while ticks are held, so the cleanup does not wait for a tick.
	left := make(chan struct{}, 2)
	for _, p := range []*plugintest.Player{alice, admin} {
		go func() { p.Disconnect(); left <- struct{}{} }()
	}
	for n := 0; n < 2; {
		select {
		case <-left:
			n++
		default:
			srv.RunTicks(1)
		}
	}
}
