package plotsplugin

import (
	"math"
	"strconv"
	"sync"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/event"
	"github.com/google/uuid"
)

type intervalCounter struct {
	interval int
	amount   int
	ticks    int
}

type playerState struct {
	lastPlot        *plots.Plot
	disableTitles   bool
	disableTime     bool
	plotChat        bool
	chatSpy         bool
	worldeditBypass *bool
	clearConfirm    bool
	debug           bool
	savedAllowFly   *bool
	savedGameMode   *plugin.GameMode
	feed            *intervalCounter
	heal            *intervalCounter
}

func (ps *Plugin) stateFor(u uuid.UUID) *playerState {
	ps.statesMu.Lock()
	defer ps.statesMu.Unlock()
	if ps.states == nil {
		ps.states = make(map[uuid.UUID]*playerState)
	}
	st := ps.states[u]
	if st == nil {
		st = &playerState{}
		ps.states[u] = st
	}
	return st
}

func (ps *Plugin) clearPlayerState(u uuid.UUID) {
	ps.statesMu.Lock()
	delete(ps.states, u)
	ps.statesMu.Unlock()
}

func (ps *Plugin) registerMovementListeners() {
	s := ps.Server
	opts := plugin.ListenOptions{Priority: plugin.High, IgnoreCancelled: true}

	// 1-second timer for heal/feed flags
	s.Scheduler().RunTimer(20, 20, ps.tickHealFeed)

	plugin.Listen(s, func(e *plugin.PlayerMoveEvent) {
		if e.From.World == e.To.World && e.From.Pos.Block() == e.To.Pos.Block() {
			return
		}
		if !ps.checkMovement(e.Player, e.From, e.To) {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.EntityTeleportEvent) {
		p, ok := e.Entity.(plugin.Player)
		if !ok {
			return
		}
		if !ps.checkTeleport(p, e.From, e.To, e.Cause) {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.EntityPortalEvent) {
		area := ps.PlotAreaAt(e.From)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.From.Pos.Block().X, e.From.Pos.Block().Z)
		if plot != nil && plot.HasOwner() && plot.FlagBool("deny-portal-travel") {
			e.SetCancelled(true)
		}
	}, opts)
}

func (ps *Plugin) checkMovement(p plugin.Player, from, to plugin.Location) bool {
	toArea := ps.PlotAreaAt(to)
	if toArea != nil && toArea.WorldBorder && !HasPerm(p, "plots.admin.border.bypass") {
		lim := float64(toArea.WorldBorderRadius())
		if math.Abs(to.Pos.X) > lim || math.Abs(to.Pos.Z) > lim {
			ps.sendCaption(p, "border.border")
			return false
		}
	}

	fromPlot := ps.PlotAt(from)
	toPlot := ps.PlotAt(to)
	if fromPlot != nil {
		fromPlot = fromPlot.BasePlot()
	}
	if toPlot != nil {
		toPlot = toPlot.BasePlot()
	}
	if fromPlot == toPlot {
		// Check forcefield while moving inside a plot
		if toPlot != nil && toPlot.HasOwner() && toPlot.FlagBool("forcefield") {
			if !ps.isAddedAndAllowed(p, toPlot) && !HasPerm(p, "plots.admin.entry.forcefield") {
				ps.applyForcefield(p, toPlot)
			}
		}
		return true
	}

	// Check deny-exit on fromPlot
	if fromPlot != nil && fromPlot.HasOwner() && fromPlot.FlagBool("deny-exit") && !HasPerm(p, "plots.admin.exit.denied") {
		ps.sendCaption(p, "deny.no_exit")
		return false
	}

	// Check entry on toPlot
	if toPlot != nil && toPlot.HasOwner() {
		if toPlot.IsDenied(p.UUID()) && !HasPerm(p, "plots.admin.entry.denied") {
			ps.sendCaption(p, "deny.no_enter", "plot", toPlot.String())
			return false
		}
		if !toPlot.FlagBool("untrusted-visit") && !ps.isAddedAndAllowed(p, toPlot) && !HasPerm(p, "plots.admin.visit.untrusted") {
			ps.sendCaption(p, "deny.no_enter", "plot", toPlot.String())
			return false
		}
	}

	if fromPlot != nil {
		ps.handlePlotLeave(p, fromPlot)
	}
	if toPlot != nil {
		ps.handlePlotEnter(p, toPlot)
	}
	return true
}

func (ps *Plugin) checkTeleport(p plugin.Player, from, to plugin.Location, _ plugin.TeleportCause) bool {
	toPlot := ps.PlotAt(to)
	if toPlot != nil {
		toPlot = toPlot.BasePlot()
		if toPlot.HasOwner() {
			if !ps.allowsTeleport(p, toPlot) {
				ps.sendCaption(p, "deny.no_enter", "plot", toPlot.String())
				return false
			}
		}
	}
	return ps.checkMovement(p, from, to)
}

func (ps *Plugin) allowsTeleport(p plugin.Player, plot *plots.Plot) bool {
	if HasPerm(p, "plots.admin.entry.denied") {
		return true
	}
	if plot.IsDenied(p.UUID()) {
		return false
	}
	mode := plot.FlagString("deny-teleport")
	uid := p.UUID()
	switch mode {
	case "trusted":
		return !plot.IsTrusted(uid)
	case "members":
		return !plot.IsMember(uid)
	case "nonmembers":
		return plot.IsAdded(uid)
	case "nontrusted":
		return plot.IsTrusted(uid) || plot.IsOwner(uid)
	case "nonowners":
		return plot.IsOwner(uid)
	default:
		return true
	}
}

func (ps *Plugin) handlePlotEnter(p plugin.Player, plot *plots.Plot) {
	base := plot.BasePlot()
	st := ps.stateFor(p.UUID())
	st.lastPlot = base

	ps.Server.Events().Fire(&event.PlayerEnterPlotEvent{Player: p, Plot: base})

	if !base.HasOwner() {
		return
	}

	// Greeting
	if greeting := base.FlagString("greeting"); greeting != "" {
		txt := ps.Lang.FormatRaw(greeting, "plot", base.ID.String(), "owner", ps.PlayerName(base.Owner), "alias", base.Settings.Alias)
		if ps.Settings != nil && ps.Settings.Chat.NotificationAsActionbar {
			p.SendActionBar(txt)
		} else {
			p.SendMessage(txt)
		}
	}

	// Notify owner on enter
	if base.FlagBool("notify-enter") && !HasPerm(p, "plots.flag.notify-enter.bypass") {
		for _, ownerUUID := range base.Owners() {
			if ownerUUID == p.UUID() {
				continue
			}
			if owner, ok := ps.Server.Player(ownerUUID); ok && owner.CanSee(p) {
				ps.sendCaption(owner, "notification.notify_enter", "player", p.Name(), "plot", base.ID.String())
			}
		}
	}

	// Gamemode / guest-gamemode (must run before flight flag, because SetGameMode resets flight abilities)
	if !HasPerm(p, "plots.gamemode.bypass") {
		gmStr := base.FlagString("gamemode")
		if gmStr == "default" && !base.IsAdded(p.UUID()) {
			gmStr = base.FlagString("guest-gamemode")
		}
		if gm, ok := parseGameModeFlag(gmStr); ok && p.GameMode() != gm {
			if st.savedGameMode == nil {
				cur := p.GameMode()
				st.savedGameMode = &cur
			}
			p.SetGameMode(gm)
		}
	}

	// Flight flag
	if !HasPerm(p, "plots.admin.flight") {
		switch base.FlagString("fly") {
		case "enabled":
			if st.savedAllowFly == nil {
				cur := p.AllowFlight()
				st.savedAllowFly = &cur
			}
			p.SetAllowFlight(true)
		case "disabled":
			if st.savedAllowFly == nil {
				cur := p.AllowFlight()
				st.savedAllowFly = &cur
			}
			p.SetFlying(false)
			p.SetAllowFlight(false)
		}
	}

	// Time flag
	if !st.disableTime {
		if tVal, ok := base.FlagValue("time").(int64); ok && tVal != math.MinInt64 {
			p.SetPlayerTime(tVal, false)
		}
	}

	// Weather flag
	switch base.FlagString("weather") {
	case string(plots.WeatherRain):
		p.SetPlayerWeather(true)
	case string(plots.WeatherClear):
		p.SetPlayerWeather(false)
	case string(plots.WeatherWorld):
		p.ResetPlayerWeather()
	}

	// Titles
	titlesEnabled := ps.Settings == nil || ps.Settings.Titles.DisplayTitles
	switch base.FlagString("titles") {
	case "true":
		titlesEnabled = true
	case "false":
		titlesEnabled = false
	}
	if titlesEnabled && !st.disableTitles {
		pt, _ := base.FlagValue("plot-title").(plots.PlotTitle)
		showDefault := !base.FlagBool("server-plot") || (ps.Settings != nil && ps.Settings.Titles.DisplayDefaultServerPlot)
		var titleTxt, subTxt plugin.Text
		shouldSend := false
		if !pt.Configured && (pt.Title != "" || pt.Subtitle != "") {
			titleTxt = ps.Lang.FormatRaw(pt.Title, "owner", ps.PlayerName(base.Owner), "plot", base.ID.String(), "alias", base.Settings.Alias)
			subTxt = ps.Lang.FormatRaw(pt.Subtitle, "owner", ps.PlayerName(base.Owner), "plot", base.ID.String(), "alias", base.Settings.Alias)
			shouldSend = true
		} else if showDefault {
			titleTxt = ps.Lang.Format("titles.title_entered_plot", "plot", base.ID.String(), "world", base.Area.WorldName, "alias", base.Settings.Alias)
			subTxt = ps.Lang.Format("titles.title_entered_plot_sub", "owner", ps.PlayerName(base.Owner), "plot", base.ID.String(), "alias", base.Settings.Alias)
			shouldSend = true
		}
		if shouldSend {
			if ps.Settings != nil && ps.Settings.Titles.TitlesAsActionbar {
				p.SendActionBar(titleTxt)
			} else {
				fadeIn, stay, fadeOut := 10, 50, 20
				if ps.Settings != nil {
					fadeIn, stay, fadeOut = ps.Settings.Titles.FadeIn, ps.Settings.Titles.Stay, ps.Settings.Titles.FadeOut
				}
				p.SendTitle(titleTxt, subTxt, fadeIn, stay, fadeOut)
			}
		}
	}

	// Feed & Heal interval flags
	if feed, ok := base.FlagValue("feed").(plots.TimedInt); ok && feed.Interval > 0 {
		st.feed = &intervalCounter{interval: feed.Interval, amount: feed.Value}
	}
	if heal, ok := base.FlagValue("heal").(plots.TimedInt); ok && heal.Interval > 0 {
		st.heal = &intervalCounter{interval: heal.Interval, amount: heal.Value}
	}
}

func (ps *Plugin) handlePlotLeave(p plugin.Player, plot *plots.Plot) {
	base := plot.BasePlot()
	st := ps.stateFor(p.UUID())
	st.lastPlot = nil
	st.feed = nil
	st.heal = nil

	ps.Server.Events().Fire(&event.PlayerLeavePlotEvent{Player: p, Plot: base})

	if !base.HasOwner() {
		return
	}

	// Farewell
	if farewell := base.FlagString("farewell"); farewell != "" {
		txt := ps.Lang.FormatRaw(farewell, "plot", base.ID.String(), "owner", ps.PlayerName(base.Owner), "alias", base.Settings.Alias)
		if ps.Settings != nil && ps.Settings.Chat.NotificationAsActionbar {
			p.SendActionBar(txt)
		} else {
			p.SendMessage(txt)
		}
	}

	// Notify owner on leave
	if base.FlagBool("notify-leave") && !HasPerm(p, "plots.flag.notify-leave.bypass") {
		for _, ownerUUID := range base.Owners() {
			if ownerUUID == p.UUID() {
				continue
			}
			if owner, ok := ps.Server.Player(ownerUUID); ok && owner.CanSee(p) {
				ps.sendCaption(owner, "notification.notify_leave", "player", p.Name(), "plot", base.ID.String())
			}
		}
	}

	// Restore gamemode first, then flight
	if st.savedGameMode != nil {
		gm := *st.savedGameMode
		st.savedGameMode = nil
		p.SetGameMode(gm)
	}

	// Restore flight
	if st.savedAllowFly != nil {
		allow := *st.savedAllowFly
		st.savedAllowFly = nil
		if !allow {
			p.SetFlying(false)
		}
		p.SetAllowFlight(allow)
	}

	// Reset time & weather
	if tVal, ok := base.FlagValue("time").(int64); ok && tVal != math.MinInt64 {
		p.ResetPlayerTime()
	}
	if base.FlagString("weather") != string(plots.WeatherOff) {
		p.ResetPlayerWeather()
	}
}

func (ps *Plugin) tickHealFeed() {
	ps.statesMu.Lock()
	defer ps.statesMu.Unlock()
	for uid, st := range ps.states {
		if st.feed == nil && st.heal == nil {
			continue
		}
		p, ok := ps.Server.Player(uid)
		if !ok || !p.IsValid() {
			continue
		}
		if st.heal != nil && p.Health() > 0 {
			st.heal.ticks++
			if st.heal.ticks >= st.heal.interval {
				st.heal.ticks = 0
				next := p.Health() + float32(st.heal.amount)
				if next > p.MaxHealth() {
					next = p.MaxHealth()
				}
				p.SetHealth(next)
			}
		}
		if st.feed != nil {
			st.feed.ticks++
			if st.feed.ticks >= st.feed.interval {
				st.feed.ticks = 0
				next := min(p.Food()+st.feed.amount, 20)
				p.SetFood(next)
			}
		}
	}
}

func (ps *Plugin) applyForcefield(p plugin.Player, plot *plots.Plot) {
	// Push non-added player away from any nearby plot owner/member within 5 blocks.
	pos := p.Pos()
	for _, other := range p.World().Players() {
		if other == p || !plot.IsAdded(other.UUID()) {
			continue
		}
		op := other.Pos()
		dx := pos.X - op.X
		dz := pos.Z - op.Z
		distSq := dx*dx + dz*dz
		if distSq > 0 && distSq < 25 {
			d := math.Sqrt(distSq)
			p.SetVelocity(plugin.Vec3{X: (dx / d) * 1.1, Y: 0.35, Z: (dz / d) * 1.1})
			return
		}
	}
}

func parseGameModeFlag(s string) (plugin.GameMode, bool) {
	switch s {
	case "survival":
		return plugin.Survival, true
	case "creative":
		return plugin.Creative, true
	case "adventure":
		return plugin.Adventure, true
	case "spectator":
		return plugin.Spectator, true
	default:
		return plugin.Survival, false
	}
}

func (ps *Plugin) sendCaption(sender plugin.CommandSender, key string, kv ...string) {
	if sender == nil || ps.Lang == nil {
		return
	}
	sender.SendMessage(ps.Lang.Format(key, kv...))
}

func (ps *Plugin) sendNoPerm(sender plugin.CommandSender, node string) {
	ps.sendCaption(sender, "permission.no_permission_event", "node", node)
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

// Keep sync imported cleanly if statesMu lives on PlotSquared.
var _ sync.Locker = (*sync.Mutex)(nil)
