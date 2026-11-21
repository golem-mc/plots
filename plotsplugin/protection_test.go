package plotsplugin_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/event"
	"github.com/golem-mc/plots/plotsplugin"
	"github.com/golem-mc/vault"
	"github.com/google/uuid"
)

func setupCompactPlotWorld(t *testing.T, srv *plugintest.Server, ps *plotsplugin.Plugin) (*plots.PlotArea, plugin.World) {
	t.Helper()
	area := plots.DefaultPlotArea("plotworld", "")
	area.PlotWidth = 10
	area.RoadWidth = 5
	area.PlotHeight = 62
	area.RoadHeight = 62
	area.WallHeight = 62
	area.MinGenHeight = 0
	area.MaxGenHeight = 80
	area.MinBuildHeight = 1
	area.MaxBuildHeight = 80
	area.ComputeGeometry()

	w, err := ps.CreatePlotWorld(area)
	if err != nil {
		t.Fatalf("CreatePlotWorld: %v", err)
	}
	w.AddTicket(plugin.ChunkPos{X: 0, Z: 0}, 3)
	loaded := 0
	for cx := 0; cx <= 2; cx++ {
		for cz := 0; cz <= 1; cz++ {
			w.LoadChunk(plugin.ChunkPos{X: cx, Z: cz}, true, func(plugin.Chunk) { loaded++ })
		}
	}
	srv.RunTicks(5)
	if loaded < 6 {
		t.Fatalf("expected 6 chunks loaded, got %d", loaded)
	}
	return area, w
}

func TestBlockAndInteractProtection(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t)
	ps, _ := plotsplugin.Get()
	area, w := setupCompactPlotWorld(t, srv, ps)

	alice := srv.Join("Alice")
	bob := srv.Join("Bob")

	p1, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot: %v", err)
	}
	b11 := area.PlotInteriorBounds(p1.ID)
	inPlot := plugin.BlockPos{X: b11.Min.X + 2, Y: 63, Z: b11.Min.Z + 2}
	roadPos := plugin.BlockPos{X: b11.Min.X - 2, Y: 63, Z: b11.Min.Z + 2}
	stone := plugin.DefaultState(registry.BlockStone)

	// Alice (owner) can place & break inside her plot
	placeAlice := &plugin.BlockPlaceEvent{Player: alice.Handle(), Block: plugin.Block{World: w, Pos: inPlot}, Placed: stone}
	srv.Host().Events().Fire(placeAlice)
	if placeAlice.Cancelled() {
		t.Errorf("owner should be allowed to place in own plot")
	}

	// Bob (outsider) cannot place or break in Alice's plot
	placeBob := &plugin.BlockPlaceEvent{Player: bob.Handle(), Block: plugin.Block{World: w, Pos: inPlot}, Placed: stone}
	srv.Host().Events().Fire(placeBob)
	if !placeBob.Cancelled() {
		t.Errorf("outsider should not be allowed to place in Alice's plot")
	}
	breakBob := &plugin.BlockBreakEvent{Player: bob.Handle(), Block: plugin.Block{World: w, Pos: inPlot}, State: stone}
	srv.Host().Events().Fire(breakBob)
	if !breakBob.Cancelled() {
		t.Errorf("outsider should not be allowed to break in Alice's plot")
	}

	// Setting `break` flag for stone allows Bob to break stone there
	if _, err := ps.SetFlag(p1, "break", "stone"); err != nil {
		t.Fatalf("SetFlag break: %v", err)
	}
	breakBob2 := &plugin.BlockBreakEvent{Player: bob.Handle(), Block: plugin.Block{World: w, Pos: inPlot}, State: stone}
	srv.Host().Events().Fire(breakBob2)
	if breakBob2.Cancelled() {
		t.Errorf("expected break flag to allow Bob to break stone")
	}

	// Nobody without admin perm can build on the road
	placeRoad := &plugin.BlockPlaceEvent{Player: alice.Handle(), Block: plugin.Block{World: w, Pos: roadPos}, Placed: stone}
	srv.Host().Events().Fire(placeRoad)
	if !placeRoad.Cancelled() {
		t.Errorf("non-admin should not be allowed to place on road")
	}

	// Liquid flow across plot boundary onto road must be cancelled
	flowEdge := &plugin.BlockFromToEvent{
		From: plugin.Block{World: w, Pos: plugin.BlockPos{X: b11.Min.X, Y: 63, Z: b11.Min.Z}},
		To:   plugin.Block{World: w, Pos: plugin.BlockPos{X: b11.Min.X - 1, Y: 63, Z: b11.Min.Z}},
	}
	srv.Host().Events().Fire(flowEdge)
	if !flowEdge.Cancelled() {
		t.Errorf("liquid flow onto wall/road must be cancelled")
	}

	// Piston pushing out of plot must be cancelled
	pistonOut := &plugin.BlockPistonEvent{
		Piston:    plugin.Block{World: w, Pos: plugin.BlockPos{X: b11.Min.X, Y: 63, Z: b11.Min.Z}},
		Direction: plugin.West,
		Extending: true,
	}
	srv.Host().Events().Fire(pistonOut)
	if !pistonOut.Cancelled() {
		t.Errorf("piston extending across plot edge must be cancelled")
	}

	// Explosion cancelled by default; allowed & clipped to plot when explosion=true
	expLoc := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(inPlot.X), Y: 63, Z: float64(inPlot.Z)}}
	exp1 := &plugin.EntityExplodeEvent{
		Location: expLoc,
		Blocks: []plugin.Block{
			{World: w, Pos: inPlot},
			{World: w, Pos: roadPos},
		},
	}
	srv.Host().Events().Fire(exp1)
	if !exp1.Cancelled() {
		t.Errorf("explosion should be cancelled when explosion flag is false")
	}
	_, _ = ps.SetFlag(p1, "explosion", "true")
	exp2 := &plugin.EntityExplodeEvent{
		Location: expLoc,
		Blocks: []plugin.Block{
			{World: w, Pos: inPlot},
			{World: w, Pos: roadPos},
		},
	}
	srv.Host().Events().Fire(exp2)
	if exp2.Cancelled() || len(exp2.Blocks) != 1 || exp2.Blocks[0].Pos != inPlot {
		t.Errorf("explosion with flag=true should keep only plot blocks: cancelled=%v blocks=%v", exp2.Cancelled(), exp2.Blocks)
	}
}

func TestMovementFlagsPvPChatAndBlockedCmds(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t)
	ps, _ := plotsplugin.Get()
	area, w := setupCompactPlotWorld(t, srv, ps)

	alice := srv.Join("Alice")
	bob := srv.Join("Bob")

	p1, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot: %v", err)
	}
	_, _ = ps.SetFlag(p1, "greeting", "Hello <plot>!")
	_, _ = ps.SetFlag(p1, "farewell", "Bye!")
	_, _ = ps.SetFlag(p1, "fly", "true")
	_, _ = ps.SetFlag(p1, "gamemode", "adventure")
	_, _ = ps.SetFlag(p1, "blocked-cmds", "sethome,tpa")

	b11 := area.PlotInteriorBounds(p1.ID)
	roadLoc := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(b11.Min.X) - 2, Y: 63, Z: float64(b11.Min.Z)}}
	plotLoc := plugin.Location{World: w, Pos: plugin.Vec3{X: float64(b11.Min.X) + 2, Y: 63, Z: float64(b11.Min.Z) + 2}}

	// Put Bob on the road first
	bob.Handle().Teleport(roadLoc, plugin.TeleportPlugin)
	alice.Handle().Teleport(plotLoc, plugin.TeleportPlugin)
	srv.RunTicks(2)
	bob.Messages(1) // drain

	// Watch enter/leave events
	entered, left := false, false
	plugin.Listen(srv.Host(), func(e *event.PlayerEnterPlotEvent) {
		if e.Player == bob.Handle() {
			entered = true
		}
	})
	plugin.Listen(srv.Host(), func(e *event.PlayerLeavePlotEvent) {
		if e.Player == bob.Handle() {
			left = true
		}
	})

	// Move Bob from road into plot
	moveIn := &plugin.PlayerMoveEvent{Player: bob.Handle(), From: roadLoc, To: plotLoc}
	srv.Host().Events().Fire(moveIn)
	if moveIn.Cancelled() || !entered {
		t.Fatalf("expected Bob to enter plot: cancelled=%v entered=%v", moveIn.Cancelled(), entered)
	}
	if !bob.Handle().AllowFlight() {
		t.Errorf("expected fly flag to grant AllowFlight")
	}
	if bob.Handle().GameMode() != plugin.Adventure {
		t.Errorf("expected gamemode flag to switch Bob to Adventure, got %v", bob.Handle().GameMode())
	}
	msgs := bob.Messages(2)
	if !strings.Contains(strings.Join(msgs, "\n"), "Hello 1;1!") {
		t.Errorf("expected greeting message, got %q", msgs)
	}

	// Put Bob physically inside plot to test PvP & blocked-cmds
	bob.Handle().Teleport(plotLoc, plugin.TeleportPlugin)
	srv.RunTicks(1)

	// PvP is false by default
	dmg := &plugin.EntityDamageEvent{
		Entity: bob.Handle(),
		Source: plugin.DamageSource{Causing: alice.Handle(), Direct: alice.Handle()},
		Amount: 4,
	}
	srv.Host().Events().Fire(dmg)
	if !dmg.Cancelled() {
		t.Errorf("PvP should be cancelled when pvp flag is false")
	}
	_, _ = ps.SetFlag(p1, "pvp", "true")
	dmg2 := &plugin.EntityDamageEvent{
		Entity: bob.Handle(),
		Source: plugin.DamageSource{Causing: alice.Handle(), Direct: alice.Handle()},
		Amount: 4,
	}
	srv.Host().Events().Fire(dmg2)
	if dmg2.Cancelled() {
		t.Errorf("PvP should be allowed when pvp flag is true")
	}

	// Blocked command check
	cmdEv := &plugin.PlayerCommandEvent{Player: bob.Handle(), Command: "sethome base"}
	srv.Host().Events().Fire(cmdEv)
	if !cmdEv.Cancelled() {
		t.Errorf("expected /sethome to be blocked by blocked-cmds flag")
	}

	// Deny-exit blocks leaving
	_, _ = ps.SetFlag(p1, "deny-exit", "true")
	moveOutBlocked := &plugin.PlayerMoveEvent{Player: bob.Handle(), From: plotLoc, To: roadLoc}
	srv.Host().Events().Fire(moveOutBlocked)
	if !moveOutBlocked.Cancelled() {
		t.Errorf("expected deny-exit flag to block leaving")
	}
	_, _ = ps.RemoveFlag(p1, "deny-exit")

	// Leave plot restores flight & gamemode and sends farewell
	moveOut := &plugin.PlayerMoveEvent{Player: bob.Handle(), From: plotLoc, To: roadLoc}
	srv.Host().Events().Fire(moveOut)
	if moveOut.Cancelled() || !left {
		t.Fatalf("expected Bob to leave plot: cancelled=%v left=%v", moveOut.Cancelled(), left)
	}
	if bob.Handle().AllowFlight() {
		t.Errorf("expected AllowFlight restored to false on leave")
	}
	if bob.Handle().GameMode() != plugin.Survival {
		t.Errorf("expected GameMode restored to Survival on leave, got %v", bob.Handle().GameMode())
	}

	// Denied player cannot re-enter
	p1.Denied[bob.Handle().UUID()] = struct{}{}
	moveDenied := &plugin.PlayerMoveEvent{Player: bob.Handle(), From: roadLoc, To: plotLoc}
	srv.Host().Events().Fire(moveDenied)
	if !moveDenied.Cancelled() {
		t.Errorf("expected denied player to be blocked from entering plot")
	}
}

type mockEconomy struct {
	balances map[uuid.UUID]float64
}

func (m *mockEconomy) Name() string                               { return "MockEco" }
func (m *mockEconomy) FractionalDigits() int                      { return 2 }
func (m *mockEconomy) Format(amount float64) string               { return fmt.Sprintf("$%.2f", amount) }
func (m *mockEconomy) CurrencyNameSingular() string               { return "dollar" }
func (m *mockEconomy) CurrencyNamePlural() string                 { return "dollars" }
func (m *mockEconomy) HasAccount(p plugin.UUID) bool              { _, ok := m.balances[p]; return ok }
func (m *mockEconomy) CreateAccount(p plugin.UUID, _ string) bool { m.balances[p] = 0; return true }
func (m *mockEconomy) Balance(p plugin.UUID) float64              { return m.balances[p] }
func (m *mockEconomy) Has(p plugin.UUID, amt float64) bool        { return m.balances[p] >= amt }
func (m *mockEconomy) Withdraw(p plugin.UUID, amt float64) vault.Response {
	if m.balances[p] < amt {
		return vault.Response{Err: fmt.Errorf("insufficient funds")}
	}
	m.balances[p] -= amt
	return vault.Response{Amount: amt, Balance: m.balances[p]}
}
func (m *mockEconomy) Deposit(p plugin.UUID, amt float64) vault.Response {
	m.balances[p] += amt
	return vault.Response{Amount: amt, Balance: m.balances[p]}
}

func TestVaultEconomyBuyPlot(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t)
	ps, _ := plotsplugin.Get()
	ps.Settings.EnabledComponents.Economy = true
	area, _ := setupCompactPlotWorld(t, srv, ps)
	area.UseEconomy = true

	alice := srv.Join("Alice")
	bob := srv.Join("Bob")

	eco := &mockEconomy{balances: map[uuid.UUID]float64{
		alice.Handle().UUID(): 50,
		bob.Handle().UUID():   500,
	}}
	plugin.Provide[vault.Economy](srv.Host(), eco, 10)

	p1, err := ps.ClaimPlot(area, plots.PlotId{X: 1, Y: 1}, alice.Handle().UUID(), alice.Handle(), false)
	if err != nil {
		t.Fatalf("ClaimPlot: %v", err)
	}
	if _, err := ps.SetFlag(p1, "price", "250"); err != nil {
		t.Fatalf("SetFlag price: %v", err)
	}
	paid, err := ps.BuyPlot(bob.Handle(), p1)
	if err != nil || paid != 250 {
		t.Fatalf("BuyPlot: paid=%v err=%v", paid, err)
	}
	if p1.Owner != bob.Handle().UUID() {
		t.Errorf("owner = %v, want Bob", p1.Owner)
	}
	if eco.Balance(bob.Handle().UUID()) != 250 || eco.Balance(alice.Handle().UUID()) != 300 {
		t.Errorf("balances after buy: bob=%.2f alice=%.2f", eco.Balance(bob.Handle().UUID()), eco.Balance(alice.Handle().UUID()))
	}
	if v, _ := p1.FlagValue("price").(float64); v != 0 {
		t.Errorf("price flag should be cleared after purchase, got %v", v)
	}
}
