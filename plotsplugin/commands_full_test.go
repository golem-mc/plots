package plotsplugin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/plotsplugin"
	"github.com/golem-mc/vault"
	"github.com/google/uuid"
)

func TestCommandsBuySetOwnerMoveSwapAutoMegaAndKick(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Admin"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	ps.Settings.EnabledComponents.Economy = true
	area, w := setupCompactPlotWorld(t, srv, ps)
	area.UseEconomy = true
	area.PriceClaim = "0"
	area.PriceMerge = "0"

	alice := srv.Join("Alice")
	bob := srv.Join("Bob")
	charlie := srv.Join("Charlie")
	admin := srv.Join("Admin")

	grantPerms(alice.Handle(),
		"plots.permpack.basic",
		"plots.plot.10",
		"plots.merge.4",
		"plots.auto.mega",
		"plots.set.flag.*",
		"plots.move",
		"plots.swap",
		"plots.kick",
		"plots.alias.set",
		"plots.alias.remove",
	)
	grantPerms(bob.Handle(),
		"plots.permpack.basic",
		"plots.plot.10",
		"plots.buy",
		"plots.swap",
	)

	eco := &mockEconomy{balances: map[uuid.UUID]float64{
		alice.Handle().UUID(): 100,
		bob.Handle().UUID():   500,
	}}
	plugin.Provide[vault.Economy](host, eco, 10)

	// Claim (1;1) for Alice and set price=200 via /p flag set
	p1, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot (1;1): %v", err)
	}
	b11 := area.PlotInteriorBounds(p1.ID)
	inP1 := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(b11.Min.X + 2), Y: 63, Z: float64(b11.Min.Z + 2)}}
	alice.Handle().Teleport(inP1, plugin.TeleportPlugin)
	mustDispatch(t, host, alice.Handle(), "p flag set price 200")

	// 1. /plot buy via player command (Bob buys Alice's plot (1;1) for 200)
	bob.Handle().Teleport(inP1, plugin.TeleportPlugin)
	mustDispatch(t, host, bob.Handle(), "plot buy")
	if !p1.IsOwner(bob.Handle().UUID()) {
		t.Fatalf("expected Bob to own (1;1) after /plot buy, got owner %s", p1.Owner)
	}
	if eco.Balance(bob.Handle().UUID()) != 300 || eco.Balance(alice.Handle().UUID()) != 300 {
		t.Errorf("Vault balances after /plot buy: bob=%.2f alice=%.2f, want 300/300",
			eco.Balance(bob.Handle().UUID()), eco.Balance(alice.Handle().UUID()))
	}

	// 2. /plot setowner (Admin transfers (1;1) back to Alice, then tests setowner none on another plot)
	admin.Handle().Teleport(inP1, plugin.TeleportPlugin)
	mustDispatch(t, host, admin.Handle(), "p setowner Alice")
	if !p1.IsOwner(alice.Handle().UUID()) {
		t.Fatalf("expected Alice to own (1;1) after /p setowner Alice")
	}

	// 3. /plot kick (Alice kicks Charlie out of (1;1))
	charlie.Handle().Teleport(inP1, plugin.TeleportPlugin)
	alice.Handle().Teleport(inP1, plugin.TeleportPlugin)
	mustDispatch(t, host, alice.Handle(), "p kick Charlie")
	if got := ps.PlotAt(charlie.Handle().Location()); got != nil && got.ID == p1.ID {
		t.Errorf("expected Charlie kicked out of (1;1), still at %v", charlie.Handle().Location().Pos)
	}

	// 4. /plot home (Alice teleports back from road to her plot, also by alias, then /p alias remove & /p sethome none)
	mustDispatch(t, host, alice.Handle(), "p alias set mybase")
	roadLoc := plugin.Location{World: w, Pos: plugin.Vec3{X: 0.5, Y: 63, Z: 0.5}}
	alice.Handle().Teleport(roadLoc, plugin.TeleportPlugin)
	mustDispatch(t, host, alice.Handle(), "p home mybase")
	if alice.Handle().Location().Pos.X < float64(b11.Min.X) || alice.Handle().Location().Pos.X > float64(b11.Max.X+1) {
		t.Errorf("expected /p home mybase near plot (1;1) X=[%d..%d], got %.1f", b11.Min.X, b11.Max.X, alice.Handle().Location().Pos.X)
	}
	alice.Handle().Teleport(inP1, plugin.TeleportPlugin)
	mustDispatch(t, host, alice.Handle(), "p sethome")
	mustDispatch(t, host, alice.Handle(), "p sethome none")
	if p1.Settings.Position.Set {
		t.Errorf("expected custom home cleared by /p sethome none")
	}
	mustDispatch(t, host, alice.Handle(), "p alias remove mybase")
	if p1.Settings.Alias != "" {
		t.Errorf("expected alias cleared by /p alias remove, got %q", p1.Settings.Alias)
	}

	// 5. /plot move: place a marker block in (1;1) and move it to unowned (2;1)
	marker1 := plugin.BlockPos{X: b11.Min.X + 1, Y: 63, Z: b11.Min.Z + 1}
	w.SetBlockState(marker1, plugin.DefaultState(registry.BlockDiamondBlock), plugin.UpdateClients)
	mustDispatch(t, host, alice.Handle(), "p move 2;1")
	if area.PlotAbs(plots.PlotId{X: 1, Y: 1}) != nil {
		t.Errorf("expected (1;1) unclaimed after /p move 2;1")
	}
	p2 := area.PlotAbs(plots.PlotId{X: 2, Y: 1})
	if p2 == nil || !p2.IsOwner(alice.Handle().UUID()) {
		t.Fatalf("expected Alice to own (2;1) after /p move 2;1")
	}
	b21 := area.PlotInteriorBounds(p2.ID)
	marker2 := plugin.BlockPos{X: b21.Min.X + 1, Y: 63, Z: b21.Min.Z + 1}
	if got := w.BlockState(marker2).Block(); got != registry.BlockDiamondBlock {
		t.Errorf("expected diamond_block moved to (2;1) at %v, got %v", marker2, got)
	}

	// 6. /plot swap: Bob claims (1;1) with emerald marker, swap (2;1) <-> (1;1)
	p1New, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, bob.Handle().UUID(), bob.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot (1;1) for Bob: %v", err)
	}
	w.SetBlockState(marker1, plugin.DefaultState(registry.BlockEmeraldBlock), plugin.UpdateClients)
	admin.Handle().Teleport(plugin.Location{World: w, Pos: plugin.Vec3{X: float64(b21.Min.X + 2), Y: 63, Z: float64(b21.Min.Z + 2)}}, plugin.TeleportPlugin)
	mustDispatch(t, host, admin.Handle(), "p swap 1;1")
	if !p1New.IsOwner(alice.Handle().UUID()) || !p2.IsOwner(bob.Handle().UUID()) {
		t.Errorf("expected owners swapped: (1;1)=%s (2;1)=%s", p1New.Owner, p2.Owner)
	}
	if got := w.BlockState(marker1).Block(); got != registry.BlockDiamondBlock {
		t.Errorf("after swap, (1;1) marker = %v, want diamond_block", got)
	}
	if got := w.BlockState(marker2).Block(); got != registry.BlockEmeraldBlock {
		t.Errorf("after swap, (2;1) marker = %v, want emerald_block", got)
	}

	// /p setowner none clears owner of (2;1)
	mustDispatch(t, host, admin.Handle(), "p 2;1 setowner none")
	if area.PlotAbs(plots.PlotId{X: 2, Y: 1}) != nil {
		t.Errorf("expected (2;1) unowned after /p 2;1 setowner none")
	}

	// 7. /plot auto 2,2 (mega-plot claim: 1 base plot consisting of 4 merged sub-plots)
	alice.Handle().Teleport(roadLoc, plugin.TeleportPlugin)
	countConnected := func() int {
		n := 0
		for _, pl := range ps.PlotsByOwner(alice.Handle().UUID()) {
			n += len(pl.ConnectedPlots())
		}
		return n
	}
	before := countConnected()
	mustDispatch(t, host, alice.Handle(), "p auto 2,2")
	after := countConnected()
	if after-before != 4 {
		t.Errorf("expected /p auto 2,2 to claim 4 merged sub-plots, claimed %d (before=%d after=%d)", after-before, before, after)
	}
}

func TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Admin"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	area, w := setupCompactPlotWorld(t, srv, ps)

	alice := srv.Join("Alice")
	bob := srv.Join("Bob")
	admin := srv.Join("Admin")

	grantPerms(alice.Handle(),
		"plots.permpack.basic",
		"plots.permpack.basicinbox",
		"plots.plot.4",
		"plots.set.*",
		"plots.set.flag.*",
		"plots.load",
		"plots.schematic",
		"plots.schematic.paste",
		"plots.schematic.save",
		"plots.toggle.*",
	)
	grantPerms(bob.Handle(),
		"plots.permpack.basic",
		"plots.permpack.basicinbox",
		"plots.dislike",
		"plots.rate",
		"plots.cluster",
		"plots.cluster.leave",
	)

	p1, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot: %v", err)
	}
	b11 := area.PlotInteriorBounds(p1.ID)
	inP1 := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(b11.Min.X + 2), Y: 63, Z: float64(b11.Min.Z + 2)}}
	alice.Handle().Teleport(inP1, plugin.TeleportPlugin)
	bob.Handle().Teleport(inP1, plugin.TeleportPlugin)
	admin.Handle().Teleport(inP1, plugin.TeleportPlugin)

	// 1. /plot set for every component & property
	mustDispatch(t, host, alice.Handle(), "p set floor gold_block")
	if got := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 62, Z: b11.Min.Z}).Block(); got != registry.BlockGoldBlock {
		t.Errorf("/p set floor = %v, want gold_block", got)
	}
	mustDispatch(t, host, alice.Handle(), "p set main stone_bricks")
	if got := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 60, Z: b11.Min.Z}).Block(); got != registry.BlockStoneBricks {
		t.Errorf("/p set main = %v, want stone_bricks", got)
	}
	mustDispatch(t, host, alice.Handle(), "p set wall bricks")
	if got := w.BlockState(plugin.BlockPos{X: b11.Min.X - 1, Y: 62, Z: b11.Min.Z}).Block(); got != registry.BlockBricks {
		t.Errorf("/p set wall = %v, want bricks", got)
	}
	mustDispatch(t, host, alice.Handle(), "p set border quartz_slab")
	if got := w.BlockState(plugin.BlockPos{X: b11.Min.X - 1, Y: 63, Z: b11.Min.Z}).Block(); got != registry.BlockQuartzSlab {
		t.Errorf("/p set border = %v, want quartz_slab", got)
	}
	mustDispatch(t, host, alice.Handle(), "p set middle emerald_block")
	mx := (b11.Min.X + b11.Max.X) / 2
	mz := (b11.Min.Z + b11.Max.Z) / 2
	if got := w.BlockState(plugin.BlockPos{X: mx, Y: 62, Z: mz}).Block(); got != registry.BlockEmeraldBlock {
		t.Errorf("/p set middle = %v, want emerald_block", got)
	}
	mustDispatch(t, host, alice.Handle(), "p set outline oak_planks")
	if got := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 65, Z: b11.Min.Z}).Block(); got != registry.BlockOakPlanks {
		t.Errorf("/p set outline = %v, want oak_planks", got)
	}
	mustDispatch(t, host, alice.Handle(), "p set air air")
	if got := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 65, Z: b11.Min.Z}).Block(); got != registry.BlockAir {
		t.Errorf("/p set air = %v, want air", got)
	}
	mustDispatch(t, host, alice.Handle(), "p set all sandstone")
	if got := w.BlockState(plugin.BlockPos{X: b11.Min.X + 1, Y: 63, Z: b11.Min.Z + 1}).Block(); got != registry.BlockSandstone {
		t.Errorf("/p set all = %v, want sandstone", got)
	}
	mustDispatch(t, host, alice.Handle(), "p set biome plains")
	mustDispatch(t, host, alice.Handle(), "p set alias citadel")
	if p1.Settings.Alias != "citadel" {
		t.Errorf("/p set alias = %q, want citadel", p1.Settings.Alias)
	}
	mustDispatch(t, host, alice.Handle(), "p set desc Fortress")
	if p1.FlagString("description") != "Fortress" {
		t.Errorf("/p set desc = %q, want Fortress", p1.FlagString("description"))
	}
	mustDispatch(t, host, alice.Handle(), "p set flag pvp true")
	if !p1.FlagBool("pvp") {
		t.Errorf("/p set flag pvp true did not set flag")
	}
	mustDispatch(t, host, alice.Handle(), "p set home")
	if !p1.Settings.Position.Set {
		t.Errorf("/p set home did not set home position")
	}

	// Reset floor cleanly via -f prefix syntax (/p 1;1 clear -f)
	mustDispatch(t, host, admin.Handle(), "p 1;1 clear -f")
	if got := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 62, Z: b11.Min.Z}).Block(); got != registry.BlockGrassBlock {
		t.Errorf("/p 1;1 clear -f floor = %v, want grass_block", got)
	}

	// 2. /plot dislike, /plot rate next, /plot comment owner & report, /plot inbox clear
	mustDispatch(t, host, bob.Handle(), "p dislike")
	if likes, dislikes := p1.LikeCount(); likes != 0 || dislikes != 1 {
		t.Errorf("after /p dislike LikeCount = (%d,%d), want (0,1)", likes, dislikes)
	}
	mustDispatch(t, host, bob.Handle(), "p rate next")
	mustDispatch(t, host, alice.Handle(), "p comment owner Private note")
	mustDispatch(t, host, bob.Handle(), "p comment report Check this")
	if len(p1.Settings.Comments) != 2 {
		t.Fatalf("expected 2 comments (owner + report), got %d", len(p1.Settings.Comments))
	}
	mustDispatch(t, host, alice.Handle(), "p inbox owner clear")
	for _, cm := range p1.Settings.Comments {
		if cm.Inbox == "owner" {
			t.Errorf("expected owner inbox cleared, still has %+v", cm)
		}
	}

	// 3. /plot cluster kick, leave, helpers remove
	mustDispatch(t, host, admin.Handle(), "p cluster create district 1;1 2;2")
	cl := area.ClusterNamed("district")
	if cl == nil {
		t.Fatal("expected cluster district created")
	}
	mustDispatch(t, host, admin.Handle(), "p cluster invite Bob")
	mustDispatch(t, host, admin.Handle(), "p cluster kick Bob")
	if cl.IsAdded(bob.Handle().UUID()) {
		t.Errorf("expected Bob removed after /p cluster kick")
	}
	mustDispatch(t, host, admin.Handle(), "p cluster invite Bob")
	mustDispatch(t, host, bob.Handle(), "p cluster leave")
	if cl.IsAdded(bob.Handle().UUID()) {
		t.Errorf("expected Bob removed after /p cluster leave")
	}
	mustDispatch(t, host, admin.Handle(), "p cluster helpers add Alice")
	mustDispatch(t, host, admin.Handle(), "p cluster helpers remove Alice")
	if _, ok := cl.Helpers[alice.Handle().UUID()]; ok {
		t.Errorf("expected Alice removed from cluster helpers")
	}
	mustDispatch(t, host, admin.Handle(), "p cluster delete district")

	// 4. /plot schematic saveall & /plot load
	marker := plugin.BlockPos{X: b11.Min.X + 2, Y: 63, Z: b11.Min.Z + 2}
	w.SetBlockState(marker, plugin.DefaultState(registry.BlockGoldBlock), plugin.UpdateClients)
	mustDispatch(t, host, alice.Handle(), "p schematic save tower")
	mustDispatch(t, host, admin.Handle(), "p schematic saveall plotworld")
	w.SetBlockState(marker, plugin.DefaultState(registry.BlockAir), plugin.UpdateClients)
	mustDispatch(t, host, alice.Handle(), "p load tower")
	if got := w.BlockState(marker).Block(); got != registry.BlockGoldBlock {
		t.Errorf("after /p load tower, marker = %v, want gold_block", got)
	}

	// 5. /plot toggle options
	for _, opt := range []string{"chat", "chatspy", "clear-confirmation", "time", "titles", "worldedit", "debug"} {
		mustDispatch(t, host, admin.Handle(), "p toggle "+opt)
	}

	// 6. /plot setup, /plot area create (partial), /plot area tp, /plot area regen, /plot template import, /plot trim
	mustDispatch(t, host, host.Console(), "plot setup freshplots plot.size=8 road.width=5 plot.height=20")
	freshArea := ps.PlotAreaByName("freshplots")
	if freshArea == nil || freshArea.PlotWidth != 8 || freshArea.RoadWidth != 5 || freshArea.PlotHeight != 20 {
		t.Fatalf("freshplots area after /plot setup = %+v", freshArea)
	}
	fw, ok := ps.WrapWorld("freshplots")
	if !ok {
		t.Fatal("expected freshplots world loaded after /plot setup")
	}
	fw.AddTicket(plugin.ChunkPos{X: 0, Z: 0}, 2)
	loaded := 0
	fw.LoadChunk(plugin.ChunkPos{X: 0, Z: 0}, true, func(plugin.Chunk) { loaded++ })
	srv.RunTicks(5)
	if loaded != 1 {
		t.Fatal("expected chunk (0,0) of freshplots loaded")
	}
	// Verify plot interior (3..10), wall (2), road (0) in freshplots
	if got := fw.BlockState(plugin.BlockPos{X: 3, Y: 20, Z: 3}).Block(); got != registry.BlockGrassBlock {
		t.Errorf("freshplots plot floor (3,20,3) = %v, want grass_block", got)
	}
	if got := fw.BlockState(plugin.BlockPos{X: 2, Y: 21, Z: 3}).Block(); got != registry.BlockSmoothStoneSlab {
		t.Errorf("freshplots wall top (2,21,3) = %v, want smooth_stone_slab", got)
	}
	if got := fw.BlockState(plugin.BlockPos{X: 0, Y: 20, Z: 3}).Block(); got != registry.BlockQuartzBlock {
		t.Errorf("freshplots road (0,20,3) = %v, want quartz_block", got)
	}

	// /plot area tp & /plot area regen
	mustDispatch(t, host, admin.Handle(), "p area tp freshplots")
	if admin.Handle().Location().World.Key() != fw.Key() {
		t.Errorf("expected Admin teleported to freshplots by /p area tp")
	}
	mustDispatch(t, host, admin.Handle(), "p area regen")

	// /plot area create partial area
	mustDispatch(t, host, admin.Handle(), "p area create world:sub min=1;1 max=2;2 plot.size=8 road.width=5")
	if sub := ps.PlotAreaByName("world;sub"); sub == nil || sub.Type != plots.AreaPartial {
		t.Errorf("expected partial area world;sub created, got %+v", sub)
	}

	// /plot template import (using bundled skyblock.template copied to templates dir on Enable)
	tmplDir := filepath.Join(host.DataDir(), "templates")
	if _, err := os.Stat(filepath.Join(tmplDir, "skyblock.template")); err == nil {
		mustDispatch(t, host, admin.Handle(), "p template import skyworld skyblock")
		if ps.PlotAreaByName("skyworld") == nil {
			t.Errorf("expected skyworld created by /p template import skyworld skyblock")
		}
	}

	// /plot trim
	mustDispatch(t, host, admin.Handle(), "p trim plotworld")

	// 7. Tab completion
	if cands := ps.TabComplete(admin.Handle(), "fl"); !containsString(cands, "flag") {
		t.Errorf("TabComplete('fl') = %v, want 'flag'", cands)
	}
	if cands := ps.TabComplete(admin.Handle(), "flag "); !containsString(cands, "flag set") {
		t.Errorf("TabComplete('flag ') = %v, want 'flag set'", cands)
	}
	if cands := ps.TabComplete(admin.Handle(), "set "); !containsString(cands, "set floor") {
		t.Errorf("TabComplete('set ') = %v, want 'set floor'", cands)
	}
	if cands := ps.TabComplete(admin.Handle(), "area "); !containsString(cands, "area create") {
		t.Errorf("TabComplete('area ') = %v, want 'area create'", cands)
	}
	if cands := ps.TabComplete(admin.Handle(), "cluster "); !containsString(cands, "cluster create") {
		t.Errorf("TabComplete('cluster ') = %v, want 'cluster create'", cands)
	}
}

func containsString(slice []string, want string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}
