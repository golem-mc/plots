package plotsplugin_test

import (
	"strings"
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/plotsplugin"
)

func TestProtectionMatrixRolesAndLocations(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Admin"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	area, w := setupCompactPlotWorld(t, srv, ps)

	alice := srv.Join("Alice")     // owner of (1;1)
	bob := srv.Join("Bob")         // outsider on (1;1), owner of (2;1)
	charlie := srv.Join("Charlie") // added member on (1;1)
	dave := srv.Join("Dave")       // trusted on (1;1)
	eve := srv.Join("Eve")         // denied on (1;1)

	p1, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot (1;1): %v", err)
	}
	p2, err := ps.ClaimPlot(area, plots.PlotId{X: 2, Y: 1}, bob.Handle().UUID(), bob.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot (2;1): %v", err)
	}
	b11 := area.PlotInteriorBounds(p1.ID)
	b21 := area.PlotInteriorBounds(p2.ID)
	inP1 := plugin.BlockPos{X: b11.Min.X + 2, Y: 63, Z: b11.Min.Z + 2}
	inP2 := plugin.BlockPos{X: b21.Min.X + 2, Y: 63, Z: b21.Min.Z + 2}
	roadPos := plugin.BlockPos{X: b11.Min.X - 2, Y: 63, Z: b11.Min.Z + 2}

	grantPerms(alice.Handle(), "plots.permpack.basic")
	alice.Handle().Teleport(plugin.Location{World: w, Pos: plugin.Vec3{X: float64(inP1.X) + 0.5, Y: 63, Z: float64(inP1.Z) + 0.5}}, plugin.TeleportPlugin)
	mustDispatch(t, host, alice.Handle(), "p add Charlie")
	mustDispatch(t, host, alice.Handle(), "p trust Dave")
	mustDispatch(t, host, alice.Handle(), "p deny Eve")

	stone := plugin.DefaultState(registry.BlockStone)
	chest := plugin.DefaultState(registry.BlockChest)
	lever := plugin.DefaultState(registry.BlockLever)
	w.SetBlockState(inP1, chest, 0)
	w.SetBlockState(inP2, chest, 0)
	w.SetBlockState(roadPos, chest, 0)

	tryPlace := func(p plugin.Player, pos plugin.BlockPos) bool {
		ev := &plugin.BlockPlaceEvent{Player: p, Block: plugin.Block{World: w, Pos: pos}, Placed: stone}
		host.Events().Fire(ev)
		return !ev.Cancelled()
	}
	tryBreak := func(p plugin.Player, pos plugin.BlockPos) bool {
		ev := &plugin.BlockBreakEvent{Player: p, Block: plugin.Block{World: w, Pos: pos}, State: stone}
		host.Events().Fire(ev)
		return !ev.Cancelled()
	}
	tryInteract := func(p plugin.Player, pos plugin.BlockPos) bool {
		b := plugin.Block{World: w, Pos: pos}
		ev := &plugin.PlayerInteractEvent{Player: p, Action: plugin.RightClickBlock, Block: &b}
		host.Events().Fire(ev)
		return !ev.Cancelled()
	}
	tryBucket := func(p plugin.Player, pos plugin.BlockPos) bool {
		ev := &plugin.PlayerBucketEvent{Player: p, Block: plugin.Block{World: w, Pos: pos}, Fill: true}
		host.Events().Fire(ev)
		return !ev.Cancelled()
	}
	trySign := func(p plugin.Player, pos plugin.BlockPos) bool {
		ev := &plugin.SignChangeEvent{Player: p, Block: plugin.Block{World: w, Pos: pos}}
		host.Events().Fire(ev)
		return !ev.Cancelled()
	}
	tryLectern := func(p plugin.Player, pos plugin.BlockPos) bool {
		ev := &plugin.PlayerTakeLecternBookEvent{Player: p, Block: plugin.Block{World: w, Pos: pos}}
		host.Events().Fire(ev)
		return !ev.Cancelled()
	}

	// Matrix while owner Alice is ONLINE:
	// - Alice (owner): allowed on (1;1), denied on (2;1) and road
	// - Bob (outsider on 1;1, owner on 2;1): denied on (1;1) and road, allowed on (2;1)
	// - Charlie (added member on 1;1): allowed on (1;1) while Alice online, denied on (2;1) and road
	// - Dave (trusted on 1;1): allowed on (1;1), denied on (2;1) and road
	// - Eve (denied on 1;1): denied everywhere
	roles := []struct {
		name string
		p    plugin.Player
		onP1 bool
		onP2 bool
		onRd bool
	}{
		{"owner_alice", alice.Handle(), true, false, false},
		{"outsider_bob", bob.Handle(), false, true, false},
		{"added_charlie_owner_online", charlie.Handle(), true, false, false},
		{"trusted_dave_owner_online", dave.Handle(), true, false, false},
		{"denied_eve", eve.Handle(), false, false, false},
	}
	for _, r := range roles {
		for _, loc := range []struct {
			label string
			pos   plugin.BlockPos
			want  bool
		}{
			{"p1", inP1, r.onP1},
			{"p2", inP2, r.onP2},
			{"road", roadPos, r.onRd},
		} {
			if got := tryPlace(r.p, loc.pos); got != loc.want {
				t.Errorf("%s place on %s = %v, want %v", r.name, loc.label, got, loc.want)
			}
			if got := tryBreak(r.p, loc.pos); got != loc.want {
				t.Errorf("%s break on %s = %v, want %v", r.name, loc.label, got, loc.want)
			}
			if got := tryInteract(r.p, loc.pos); got != loc.want {
				t.Errorf("%s interact on %s = %v, want %v", r.name, loc.label, got, loc.want)
			}
			if got := tryBucket(r.p, loc.pos); got != loc.want {
				t.Errorf("%s bucket on %s = %v, want %v", r.name, loc.label, got, loc.want)
			}
			if got := trySign(r.p, loc.pos); got != loc.want {
				t.Errorf("%s sign on %s = %v, want %v", r.name, loc.label, got, loc.want)
			}
			if got := tryLectern(r.p, loc.pos); got != loc.want {
				t.Errorf("%s lectern on %s = %v, want %v", r.name, loc.label, got, loc.want)
			}
		}
	}

	// `use` flag and `device-interact` flag allow outsider Bob to interact in Alice's plot
	if _, err := ps.SetFlag(p1, "use", "chest"); err != nil {
		t.Fatal(err)
	}
	if !tryInteract(bob.Handle(), inP1) {
		t.Errorf("expected use=chest flag to allow Bob to interact with chest on (1;1)")
	}
	_, _ = ps.RemoveFlag(p1, "use")

	leverPos := plugin.BlockPos{X: b11.Min.X + 3, Y: 63, Z: b11.Min.Z + 3}
	w.SetBlockState(leverPos, lever, 0)
	if tryInteract(bob.Handle(), leverPos) {
		t.Errorf("expected outsider Bob blocked from lever without device-interact")
	}
	_, _ = ps.SetFlag(p1, "device-interact", "true")
	if !tryInteract(bob.Handle(), leverPos) {
		t.Errorf("expected device-interact=true to allow Bob to use lever")
	}

	// PvP across own plot, other plot, and road (pvp=false default vs pvp=true)
	inP1Loc := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(inP1.X) + 0.5, Y: 63, Z: float64(inP1.Z) + 0.5}}
	inP2Loc := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(inP2.X) + 0.5, Y: 63, Z: float64(inP2.Z) + 0.5}}
	roadLoc := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(roadPos.X) + 0.5, Y: 63, Z: float64(roadPos.Z) + 0.5}}

	tryPvP := func(attacker, victim plugin.Player, loc plugin.Location) bool {
		attacker.Teleport(loc, plugin.TeleportPlugin)
		victim.Teleport(loc, plugin.TeleportPlugin)
		ev := &plugin.PlayerAttackEvent{Player: attacker, Target: victim}
		host.Events().Fire(ev)
		return !ev.Cancelled()
	}
	if tryPvP(alice.Handle(), bob.Handle(), inP1Loc) {
		t.Errorf("PvP in own plot should be blocked when pvp=false")
	}
	if tryPvP(alice.Handle(), bob.Handle(), inP2Loc) {
		t.Errorf("PvP in other plot should be blocked when pvp=false")
	}
	if tryPvP(alice.Handle(), bob.Handle(), roadLoc) {
		t.Errorf("PvP on road should be blocked by default")
	}
	_, _ = ps.SetFlag(p1, "pvp", "true")
	if !tryPvP(alice.Handle(), bob.Handle(), inP1Loc) {
		t.Errorf("PvP in plot should be allowed when pvp=true")
	}
	// Road flags allow road PvP when configured
	_ = area.RoadFlags.ParseAndSet("pvp", "true")
	area.HasRoadFlags = true
	if !tryPvP(alice.Handle(), bob.Handle(), roadLoc) {
		t.Errorf("PvP on road should be allowed when road pvp flag is true")
	}

	// Denied player Eve cannot teleport into (1;1)
	tpEve := &plugin.EntityTeleportEvent{Entity: eve.Handle(), From: roadLoc, To: inP1Loc}
	host.Events().Fire(tpEve)
	if !tpEve.Cancelled() {
		t.Errorf("expected denied player teleport into (1;1) cancelled")
	}

	// Done plot restricts building even for owner Alice when done.restrict-building=true
	ps.Settings.Done.RestrictBuilding = true
	_, _ = ps.SetFlag(p1, "done", "1700000000")
	if tryPlace(alice.Handle(), inP1) {
		t.Errorf("expected building restricted for owner on done plot")
	}
	_, _ = ps.RemoveFlag(p1, "done")
	if !tryPlace(alice.Handle(), inP1) {
		t.Errorf("expected building restored after removing done flag")
	}

	// Owner Alice QUITS -> Added member Charlie loses build/interact rights,
	// while Trusted Dave KEEPS build/interact rights!
	alice.Disconnect()
	srv.RunTicks(2)

	if tryPlace(charlie.Handle(), inP1) || tryBreak(charlie.Handle(), inP1) || tryInteract(charlie.Handle(), inP1) {
		t.Errorf("added member Charlie must be blocked when owner Alice is offline")
	}
	if !tryPlace(dave.Handle(), inP1) || !tryBreak(dave.Handle(), inP1) || !tryInteract(dave.Handle(), inP1) {
		t.Errorf("trusted player Dave must still be allowed when owner Alice is offline")
	}
}

func TestProtectionEnvironmentFlagsAndPlotChat(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t, plugintest.Ops("Admin"))
	host := srv.Host()
	ps, _ := plotsplugin.Get()
	area, w := setupCompactPlotWorld(t, srv, ps)

	alice := srv.Join("Alice")
	bob := srv.Join("Bob")
	charlie := srv.Join("Charlie")
	admin := srv.Join("Admin")

	p1, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot: %v", err)
	}
	b11 := area.PlotInteriorBounds(p1.ID)
	inP1 := plugin.BlockPos{X: b11.Min.X + 2, Y: 63, Z: b11.Min.Z + 2}
	roadPos := plugin.BlockPos{X: b11.Min.X - 2, Y: 63, Z: b11.Min.Z + 2}

	// 1. Block burn / ignite / spread / form / fade / grow / leaf decay / portal / tnt / block explode
	burn := &plugin.BlockBurnEvent{Block: plugin.Block{World: w, Pos: inP1}}
	host.Events().Fire(burn)
	if !burn.Cancelled() {
		t.Errorf("BlockBurnEvent should be cancelled when block-burn=false")
	}
	_, _ = ps.SetFlag(p1, "block-burn", "true")
	burn2 := &plugin.BlockBurnEvent{Block: plugin.Block{World: w, Pos: inP1}}
	host.Events().Fire(burn2)
	if burn2.Cancelled() {
		t.Errorf("BlockBurnEvent should be allowed when block-burn=true")
	}

	ignite := &plugin.BlockIgniteEvent{Block: plugin.Block{World: w, Pos: roadPos}}
	host.Events().Fire(ignite)
	if !ignite.Cancelled() {
		t.Errorf("BlockIgniteEvent on road should be cancelled")
	}

	iceForm := &plugin.BlockFormEvent{Block: plugin.Block{World: w, Pos: inP1}, NewState: plugin.DefaultState(registry.BlockIce)}
	host.Events().Fire(iceForm)
	if !iceForm.Cancelled() {
		t.Errorf("ice-form defaults to false and should cancel BlockFormEvent")
	}
	_, _ = ps.SetFlag(p1, "ice-form", "true")
	iceForm2 := &plugin.BlockFormEvent{Block: plugin.Block{World: w, Pos: inP1}, NewState: plugin.DefaultState(registry.BlockIce)}
	host.Events().Fire(iceForm2)
	if iceForm2.Cancelled() {
		t.Errorf("ice-form=true should allow BlockFormEvent")
	}

	_, _ = ps.SetFlag(p1, "leaf-decay", "false")
	leaf := &plugin.LeavesDecayEvent{Block: plugin.Block{World: w, Pos: inP1}}
	host.Events().Fire(leaf)
	if !leaf.Cancelled() {
		t.Errorf("leaf-decay=false should cancel LeavesDecayEvent")
	}

	_, _ = ps.SetFlag(p1, "crop-grow", "false")
	grow := &plugin.BlockGrowEvent{Block: plugin.Block{World: w, Pos: inP1}}
	host.Events().Fire(grow)
	if !grow.Cancelled() {
		t.Errorf("crop-grow=false should cancel BlockGrowEvent")
	}

	tnt := &plugin.TNTPrimeEvent{Block: plugin.Block{World: w, Pos: inP1}, Entity: bob.Handle()}
	host.Events().Fire(tnt)
	if !tnt.Cancelled() {
		t.Errorf("outsider priming TNT should be cancelled")
	}

	portal := &plugin.PortalCreateEvent{World: w, Blocks: []plugin.BlockChange{{Pos: roadPos}}}
	host.Events().Fire(portal)
	if !portal.Cancelled() {
		t.Errorf("portal creation on road should be cancelled")
	}

	blkExp := &plugin.BlockExplodeEvent{Block: plugin.Block{World: w, Pos: inP1}, Blocks: []plugin.Block{{World: w, Pos: inP1}}}
	host.Events().Fire(blkExp)
	if !blkExp.Cancelled() {
		t.Errorf("BlockExplodeEvent should be cancelled when explosion=false")
	}

	// 2. Item drop flag
	inP1Loc := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(inP1.X) + 0.5, Y: 63, Z: float64(inP1.Z) + 0.5}}
	roadLoc := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(roadPos.X) + 0.5, Y: 63, Z: float64(roadPos.Z) + 0.5}}
	alice.Handle().Teleport(inP1Loc, plugin.TeleportPlugin)
	bob.Handle().Teleport(inP1Loc, plugin.TeleportPlugin)
	charlie.Handle().Teleport(roadLoc, plugin.TeleportPlugin)
	admin.Handle().Teleport(roadLoc, plugin.TeleportPlugin)

	_, _ = ps.SetFlag(p1, "item-drop", "false")
	dropEv := &plugin.PlayerDropItemEvent{Player: bob.Handle()}
	host.Events().Fire(dropEv)
	if !dropEv.Cancelled() {
		t.Errorf("item-drop=false should cancel PlayerDropItemEvent")
	}

	// 3. Plot chat isolation + chatspy + chat=false flag
	// Enable plot chat for Alice, chatspy for Admin
	grantPerms(alice.Handle(), "plots.permpack.basic", "plots.chat")
	mustDispatch(t, host, alice.Handle(), "p chat")
	mustDispatch(t, host, admin.Handle(), "p toggle chatspy")
	bob.Messages(1)
	charlie.Messages(1)
	admin.Messages(1)

	chatEv := &plugin.PlayerChatEvent{Player: alice.Handle(), Message: "secret plot message"}
	host.Events().Fire(chatEv)
	if !chatEv.Cancelled() {
		t.Errorf("plot chat should cancel global PlayerChatEvent and route locally")
	}
	if !strings.Contains(strings.Join(bob.Messages(1), "\n"), "secret plot message") {
		t.Errorf("expected Bob (in same plot) to receive plot chat")
	}
	if !strings.Contains(strings.Join(admin.Messages(1), "\n"), "secret plot message") {
		t.Errorf("expected Admin (with chatspy) to receive plot chat")
	}
	if strings.Contains(strings.Join(charlie.Messages(1), "\n"), "secret plot message") {
		t.Errorf("expected Charlie (on road) NOT to receive plot chat")
	}

	// chat=false flag cancels chat altogether for non-admins
	_, _ = ps.SetFlag(p1, "chat", "false")
	chatBlocked := &plugin.PlayerChatEvent{Player: bob.Handle(), Message: "hello"}
	host.Events().Fire(chatBlocked)
	if !chatBlocked.Cancelled() {
		t.Errorf("chat=false flag should cancel PlayerChatEvent")
	}
}
