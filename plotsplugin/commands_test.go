package plotsplugin_test

import (
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/plotsplugin"
)

func mustDispatch(t *testing.T, host plugin.Server, sender plugin.CommandSender, cmd string) {
	t.Helper()
	if _, err := host.Dispatch(sender, cmd); err != nil {
		t.Fatalf("Dispatch(%q): %v", cmd, err)
	}
}

func grantPerms(p plugin.Player, perms ...string) {
	att := p.AddAttachment()
	for _, perm := range perms {
		att.Set(perm, true)
	}
}

func TestCorePlotCommandsClaimTrustFlagsAndTeleport(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Admin"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	area, w := setupCompactPlotWorld(t, srv, ps)

	alice := srv.Join("Alice")
	bob := srv.Join("Bob")
	charlie := srv.Join("Charlie")
	admin := srv.Join("Admin")

	// Give Alice basic plot permissions (including plots.plot.2 and flag/set perms)
	grantPerms(alice.Handle(),
		"plots.permpack.basic",
		"plots.plot.2",
		"plots.set.floor",
		"plots.set.flag.*",
		"plots.target",
		"plots.leave",
		"plots.visit.other",
		"plots.alias.set",
		"plots.alias.remove",
		"plots.set.desc",
	)
	grantPerms(bob.Handle(), "plots.permpack.basic", "plots.leave", "plots.visit.other")

	// Move Alice inside plot (1;1) and run /plot claim
	b11 := area.PlotInteriorBounds(plots.PlotId{X: 1, Y: 1})
	aliceLoc := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(b11.Min.X + 2), Y: 63, Z: float64(b11.Min.Z + 2)}}
	alice.Handle().Teleport(aliceLoc, plugin.TeleportPlugin)

	mustDispatch(t, host, alice.Handle(), "plot claim")
	p1 := area.PlotAbs(plots.PlotId{X: 1, Y: 1})
	if p1 == nil || !p1.IsOwner(alice.Handle().UUID()) {
		t.Fatalf("expected Alice to own (1;1) after /plot claim, got %v", p1)
	}
	// teleport.on-claim moves Alice to the default plot home ("side" road); step back into (1;1).
	alice.Handle().Teleport(aliceLoc, plugin.TeleportPlugin)

	// /p trust Bob, /p add Charlie, /p deny Charlie (moves Charlie from members to denied and kicks)
	charlie.Handle().Teleport(aliceLoc, plugin.TeleportPlugin)
	mustDispatch(t, host, alice.Handle(), "p trust Bob")
	if !p1.IsTrusted(bob.Handle().UUID()) {
		t.Errorf("expected Bob trusted")
	}
	mustDispatch(t, host, alice.Handle(), "p add Charlie")
	if !p1.IsMember(charlie.Handle().UUID()) {
		t.Errorf("expected Charlie member")
	}
	mustDispatch(t, host, alice.Handle(), "p deny Charlie")
	if !p1.IsDenied(charlie.Handle().UUID()) || p1.IsMember(charlie.Handle().UUID()) {
		t.Errorf("expected Charlie denied and removed from members")
	}
	mustDispatch(t, host, alice.Handle(), "p remove Charlie")
	if p1.IsDenied(charlie.Handle().UUID()) {
		t.Errorf("expected Charlie un-denied after /p remove")
	}

	// Bob leaves Alice's plot with /p leave
	bob.Handle().Teleport(aliceLoc, plugin.TeleportPlugin)
	mustDispatch(t, host, bob.Handle(), "p leave")
	if p1.IsTrusted(bob.Handle().UUID()) {
		t.Errorf("expected Bob removed after /p leave")
	}

	// Alias, desc, sethome, home, visit, middle, target
	mustDispatch(t, host, alice.Handle(), "p alias set wonderland")
	if p1.Settings.Alias != "wonderland" {
		t.Errorf("alias = %q, want wonderland", p1.Settings.Alias)
	}
	mustDispatch(t, host, alice.Handle(), "p desc Cozy cottage")
	if p1.FlagString("description") != "Cozy cottage" {
		t.Errorf("description = %q", p1.FlagString("description"))
	}
	mustDispatch(t, host, alice.Handle(), "p sethome")
	if p1.Settings.Position == (plots.BlockLoc{}) {
		t.Errorf("expected custom home set")
	}
	mustDispatch(t, host, bob.Handle(), "p visit wonderland")
	if got := ps.PlotAt(bob.Handle().Location()); got == nil || got.ID != p1.ID {
		t.Errorf("Bob after /p visit wonderland at %v, want (1;1)", got)
	}
	mustDispatch(t, host, alice.Handle(), "p middle")
	mustDispatch(t, host, alice.Handle(), "p target 1;1")
	if ct := alice.Handle().CompassTarget(); ct.World == nil || ct.World.Key() != w.Key() {
		t.Errorf("compass target world = %v, want %v", ct.World, w.Key())
	}

	// Set component floor, biome, flags (set/add/remove/list/info), music, toggle, chat, info, list, near, caps, help
	mustDispatch(t, host, alice.Handle(), "p set floor gold_block")
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 62, Z: b11.Min.Z}); st.Block() != registry.BlockGoldBlock {
		t.Errorf("floor after /p set floor = %v, want gold_block", st.Block())
	}
	mustDispatch(t, host, alice.Handle(), "p biome desert")
	mustDispatch(t, host, alice.Handle(), "p flag set pvp true")
	if !p1.FlagBool("pvp") {
		t.Errorf("expected pvp=true")
	}
	mustDispatch(t, host, alice.Handle(), "p flag add break stone")
	mustDispatch(t, host, alice.Handle(), "p flag add break dirt")
	if bl, _ := p1.FlagValue("break").([]string); len(bl) != 2 {
		t.Errorf("break flag = %v, want 2 items", bl)
	}
	mustDispatch(t, host, alice.Handle(), "p flag remove break stone")
	if bl, _ := p1.FlagValue("break").([]string); len(bl) != 1 || bl[0] != "minecraft:dirt" {
		t.Errorf("break flag after partial remove = %v, want [minecraft:dirt]", bl)
	}
	mustDispatch(t, host, alice.Handle(), "p flag remove pvp")
	if p1.FlagBool("pvp") {
		t.Errorf("expected pvp removed")
	}
	mustDispatch(t, host, alice.Handle(), "p flag list")
	mustDispatch(t, host, alice.Handle(), "p flag info pvp")
	mustDispatch(t, host, alice.Handle(), "p music music_disc_cat")
	if p1.FlagString("music") != "minecraft:music_disc_cat" {
		t.Errorf("music flag = %q, want minecraft:music_disc_cat", p1.FlagString("music"))
	}
	mustDispatch(t, host, alice.Handle(), "p toggle titles")
	mustDispatch(t, host, alice.Handle(), "p chat Hello plot!")
	mustDispatch(t, host, alice.Handle(), "p info")
	mustDispatch(t, host, alice.Handle(), "p list mine")
	mustDispatch(t, host, alice.Handle(), "p near")
	mustDispatch(t, host, alice.Handle(), "p caps")
	mustDispatch(t, host, alice.Handle(), "p help")
	mustDispatch(t, host, admin.Handle(), "plotsquared list all")

	// /p auto claims second plot for Alice
	mustDispatch(t, host, alice.Handle(), "p auto")
	if len(ps.PlotsByOwner(alice.Handle().UUID())) != 2 {
		t.Errorf("expected Alice to own 2 plots after /p auto, got %d", len(ps.PlotsByOwner(alice.Handle().UUID())))
	}
}

func TestConfirmMergeUnlinkClearDeleteGrantAndCopySwap(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Admin"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	area, w := setupCompactPlotWorld(t, srv, ps)

	alice := srv.Join("Alice")
	admin := srv.Join("Admin")
	grantPerms(alice.Handle(),
		"plots.permpack.basic",
		"plots.plot.2",
		"plots.set.floor",
		"plots.copy",
		"plots.move",
		"plots.swap",
	)

	// Claim (1;1) and (2;1) for Alice
	p1, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot (1;1): %v", err)
	}
	_, err = ps.ClaimPlot(area, plots.PlotId{X: 2, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot (2;1): %v", err)
	}

	b11 := area.PlotInteriorBounds(p1.ID)
	alice.Handle().Teleport(plugin.Location{World: w, Pos: plugin.Vec3{X: float64(b11.Min.X + 2), Y: 63, Z: float64(b11.Min.Z + 2)}}, plugin.TeleportPlugin)

	// Non-op /p clear requires confirmation: floor stays diamond until /p confirm
	if err := ps.SetComponent(p1, plots.ComponentFloor, "diamond_block"); err != nil {
		t.Fatal(err)
	}
	mustDispatch(t, host, alice.Handle(), "p clear")
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 62, Z: b11.Min.Z}); st.Block() != registry.BlockDiamondBlock {
		t.Fatalf("expected clear to wait for /p confirm, got %v", st.Block())
	}
	mustDispatch(t, host, alice.Handle(), "p confirm")
	if st := w.BlockState(plugin.BlockPos{X: b11.Min.X, Y: 62, Z: b11.Min.Z}); st.Block() != registry.BlockGrassBlock {
		t.Fatalf("expected grass_block after /p confirm, got %v", st.Block())
	}

	// /p merge east & /p unlink + /p confirm
	mustDispatch(t, host, alice.Handle(), "p merge east")
	if !p1.IsMerged() {
		t.Fatalf("expected (1;1) merged east")
	}
	mustDispatch(t, host, alice.Handle(), "p unlink")
	mustDispatch(t, host, alice.Handle(), "p confirm")
	if p1.IsMerged() {
		t.Fatalf("expected (1;1) unlinked after /p confirm")
	}

	// /p grant add Alice & check -> Alice can now claim a 3rd plot via /p auto!
	mustDispatch(t, host, admin.Handle(), "p grant add Alice")
	mustDispatch(t, host, admin.Handle(), "p grant check Alice")
	mustDispatch(t, host, alice.Handle(), "p auto")
	if got := len(ps.PlotsByOwner(alice.Handle().UUID())); got != 3 {
		t.Errorf("expected 3 plots after grant + /p auto, got %d", got)
	}

	// /p copy (1;1) -> (3;1)
	if err := ps.SetComponent(p1, plots.ComponentFloor, "emerald_block"); err != nil {
		t.Fatal(err)
	}
	alice.Handle().Teleport(plugin.Location{World: w, Pos: plugin.Vec3{X: float64(b11.Min.X + 2), Y: 63, Z: float64(b11.Min.Z + 2)}}, plugin.TeleportPlugin)
	mustDispatch(t, host, alice.Handle(), "p copy 3;1")
	p3 := area.PlotAbs(plots.PlotId{X: 3, Y: 1})
	if p3 == nil || !p3.IsOwner(alice.Handle().UUID()) {
		t.Fatalf("expected (3;1) created by /p copy")
	}

	// /p delete + /p confirm removes (1;1)
	mustDispatch(t, host, alice.Handle(), "p delete")
	mustDispatch(t, host, alice.Handle(), "p confirm")
	if area.PlotAbs(plots.PlotId{X: 1, Y: 1}) != nil {
		t.Errorf("expected (1;1) deleted after /p confirm")
	}
}
