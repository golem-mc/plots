package plotsplugin_test

import (
	"testing"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/config"
	"github.com/golem-mc/plots/plotsplugin"
)

func TestSocialRatingsCommentsDoneAndClusters(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Admin"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	area, w := setupCompactPlotWorld(t, srv, ps)

	alice := srv.Join("Alice")
	bob := srv.Join("Bob")
	admin := srv.Join("Admin")

	grantPerms(alice.Handle(), "plots.permpack.basic", "plots.permpack.basicinbox")
	grantPerms(bob.Handle(), "plots.permpack.basic", "plots.permpack.basicinbox", "plots.rate", "plots.like", "plots.dislike")

	p1, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot: %v", err)
	}
	b11 := area.PlotInteriorBounds(p1.ID)
	inPlot := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(b11.Min.X + 2), Y: 63, Z: float64(b11.Min.Z + 2)}}
	alice.Handle().Teleport(inPlot, plugin.TeleportPlugin)
	bob.Handle().Teleport(inPlot, plugin.TeleportPlugin)

	// Bob rates Alice's plot 8, then Admin purges ratings, then Bob likes it (10)
	mustDispatch(t, host, bob.Handle(), "p rate 8")
	if avg, n := p1.AverageRating(); n != 1 || avg != 8 {
		t.Fatalf("rating after /p rate 8 = (%.1f, %d), want (8.0, 1)", avg, n)
	}
	admin.Handle().Teleport(inPlot, plugin.TeleportPlugin)
	mustDispatch(t, host, admin.Handle(), "p rate purge")
	if _, n := p1.AverageRating(); n != 0 {
		t.Fatalf("expected 0 ratings after purge, got %d", n)
	}
	mustDispatch(t, host, bob.Handle(), "p like")
	if likes, dislikes := p1.LikeCount(); likes != 1 || dislikes != 0 {
		t.Errorf("LikeCount = (%d, %d), want (1, 0)", likes, dislikes)
	}

	// Comments & inbox
	mustDispatch(t, host, bob.Handle(), "p comment public Great build!")
	if len(p1.Settings.Comments) != 1 || p1.Settings.Comments[0].Comment != "Great build!" {
		t.Fatalf("comments = %+v", p1.Settings.Comments)
	}
	mustDispatch(t, host, alice.Handle(), "p inbox")
	mustDispatch(t, host, alice.Handle(), "p inbox public")
	mustDispatch(t, host, alice.Handle(), "p inbox public delete 1")
	if len(p1.Settings.Comments) != 0 {
		t.Errorf("expected comment deleted, got %+v", p1.Settings.Comments)
	}

	// Done & Continue
	mustDispatch(t, host, alice.Handle(), "p done")
	if p1.FlagString("done") == "" {
		t.Errorf("expected done flag set")
	}
	mustDispatch(t, host, alice.Handle(), "p continue")
	if p1.FlagString("done") != "" {
		t.Errorf("expected done flag cleared after /p continue")
	}

	// Clusters: create, info, invite, helpers, resize, list, tp, sethome, delete
	mustDispatch(t, host, admin.Handle(), "p cluster create town 1;1 2;2")
	cl := area.ClusterNamed("town")
	if cl == nil || cl.AreaCount() != 4 {
		t.Fatalf("expected 2x2 cluster town, got %+v", cl)
	}
	mustDispatch(t, host, admin.Handle(), "p cluster invite Bob")
	if !cl.IsAdded(bob.Handle().UUID()) {
		t.Errorf("expected Bob invited to cluster")
	}
	mustDispatch(t, host, admin.Handle(), "p cluster helpers add Alice")
	mustDispatch(t, host, admin.Handle(), "p cluster resize 1;1 3;2")
	if cl.AreaCount() != 6 {
		t.Errorf("expected resized cluster area 6, got %d", cl.AreaCount())
	}
	mustDispatch(t, host, admin.Handle(), "p cluster list")
	mustDispatch(t, host, admin.Handle(), "p cluster info")
	mustDispatch(t, host, admin.Handle(), "p cluster sethome")
	mustDispatch(t, host, admin.Handle(), "p cluster tp town")
	mustDispatch(t, host, admin.Handle(), "p cluster delete town")
	if area.ClusterNamed("town") != nil {
		t.Errorf("expected cluster town deleted")
	}
}

func TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry(t *testing.T) {
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
	b11 := area.PlotInteriorBounds(p1.ID)
	inPlot := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(b11.Min.X + 2), Y: 63, Z: float64(b11.Min.Z + 2)}}
	admin.Handle().Teleport(inPlot, plugin.TeleportPlugin)

	// Place a marker gold_block inside (1;1), save schematic & backup, clear, then restore via backup!
	marker := plugin.BlockPos{X: b11.Min.X + 3, Y: 63, Z: b11.Min.Z + 3}
	w.SetBlockState(marker, plugin.DefaultState(registry.BlockGoldBlock), 0)

	mustDispatch(t, host, admin.Handle(), "p schematic save castle")
	mustDispatch(t, host, admin.Handle(), "p schematic list")
	mustDispatch(t, host, admin.Handle(), "p backup save")
	mustDispatch(t, host, admin.Handle(), "p backup list")

	w.SetBlockState(marker, plugin.DefaultState(registry.BlockAir), 0)
	mustDispatch(t, host, admin.Handle(), "p backup load 1")
	if got := w.BlockState(marker).Block(); got != registry.BlockGoldBlock {
		t.Errorf("marker after /p backup load 1 = %v, want gold_block", got)
	}

	// Clear and restore via /p schematic paste castle
	w.SetBlockState(marker, plugin.DefaultState(registry.BlockAir), 0)
	mustDispatch(t, host, admin.Handle(), "p schematic paste castle")
	if got := w.BlockState(marker).Block(); got != registry.BlockGoldBlock {
		t.Errorf("marker after /p schematic paste = %v, want gold_block", got)
	}

	// Road schematic + regen commands
	mustDispatch(t, host, admin.Handle(), "p createroadschematic")
	mustDispatch(t, host, admin.Handle(), "p debugroadregen plot")
	mustDispatch(t, host, admin.Handle(), "p regenallroads plotworld")

	// Area & template & admin info commands
	mustDispatch(t, host, admin.Handle(), "p area list")
	mustDispatch(t, host, admin.Handle(), "p area info plotworld")
	mustDispatch(t, host, admin.Handle(), "p template list")
	mustDispatch(t, host, admin.Handle(), "p template export plotworld")
	mustDispatch(t, host, admin.Handle(), "p reload")
	mustDispatch(t, host, admin.Handle(), "p plugin")
	mustDispatch(t, host, admin.Handle(), "p debug")
	mustDispatch(t, host, admin.Handle(), "p database")
	mustDispatch(t, host, admin.Handle(), "p download")

	// Condense: claim outer plot (4;4) and condense with radius 2 -> moves inside [-2..2]
	pOuter, err := ps.ClaimPlot(area, plots.PlotId{X: 4, Y: 4}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot (4;4): %v", err)
	}
	mustDispatch(t, host, admin.Handle(), "p condense plotworld info 2")
	mustDispatch(t, host, admin.Handle(), "p condense plotworld start 2")
	if area.PlotAbs(plots.PlotId{X: 4, Y: 4}) != nil {
		t.Errorf("expected (4;4) moved inward by condense")
	}
	if pOuter.ID.X > 2 || pOuter.ID.Y > 2 {
		t.Errorf("expected condensed plot inside radius 2, got %s", pOuter.ID)
	}

	// Expiry: configure 1-day auto_clear, backdate p1 and Alice's last-seen by 3 days, verify keep flag protects!
	ps.Settings.AutoClear["default"] = config.AutoClearTask{Days: 1, Threshold: -1, Worlds: []string{"*"}}
	oldTime := time.Now().Add(-72 * time.Hour)
	p1.Timestamp = oldTime.UnixMilli()
	pOuter.Timestamp = oldTime.UnixMilli()
	ps.Expiry.TouchPlayer(alice.Handle().UUID(), oldTime)

	// Protect p1 with keep=true; pOuter should be expired & cleared while p1 stays!
	if _, err := ps.SetFlag(p1, "keep", "true"); err != nil {
		t.Fatal(err)
	}
	mustDispatch(t, host, admin.Handle(), "p list expired")
	cleared := ps.RunExpiryPass()
	if cleared != 1 {
		t.Errorf("RunExpiryPass cleared %d plots, want 1", cleared)
	}
	if area.PlotAbs(p1.ID) == nil {
		t.Errorf("expected keep=true plot (1;1) preserved by expiry")
	}

	// Purge remaining plot owned by Alice
	mustDispatch(t, host, admin.Handle(), "p purge owner:Alice clear:false")
	if area.PlotAbs(p1.ID) != nil {
		t.Errorf("expected Alice's plot removed by /p purge")
	}
}
