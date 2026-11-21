package plotsplugin

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/event"
	"github.com/golem-mc/plots/internal/generator"
	"github.com/google/uuid"
)

func (ps *Plugin) registerMemberAndTeleportCommands() {
	ps.addSubCommand(&subCommand{
		name:     "home",
		aliases:  []string{"h"},
		perm:     "plots.home",
		usage:    "/plot home [<page> | <alias> | <area;x;y> | <area> <x;y> | <area> <page>]",
		desc:     "Teleport to your plot",
		category: catTeleport,
		required: reqPlayer,
		run:      ps.cmdHome,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				return filterPrefix(append(ps.allAreaNames(), "1", "2", "3"), c.args[0])
			}
			return filterPrefix([]string{"1", "2", "3"}, lastArg(c.args))
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "visit",
		aliases:  []string{"v", "tp", "teleport", "goto", "warp"},
		perm:     "plots.visit",
		usage:    "/plot visit <player | alias | plot> [area] [#]",
		desc:     "Visit another player's plot",
		category: catTeleport,
		required: reqPlayer,
		run:      ps.cmdVisit,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				return filterPrefix(ps.onlinePlayerNames(), c.args[0])
			}
			if len(c.args) == 2 {
				return filterPrefix(append(ps.allAreaNames(), "1", "2", "last"), c.args[1])
			}
			return filterPrefix([]string{"1", "2", "last"}, lastArg(c.args))
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "middle",
		aliases:  []string{"center", "centre"},
		perm:     "plots.middle",
		usage:    "/plot middle",
		desc:     "Teleport to the center of the current plot",
		category: catTeleport,
		required: reqPlayer,
		run:      ps.cmdMiddle,
	})

	ps.addSubCommand(&subCommand{
		name:     "sethome",
		aliases:  []string{"sh", "seth"},
		perm:     "plots.set.home",
		usage:    "/plot sethome [none]",
		desc:     "Set the plot home to your current position",
		category: catSettings,
		required: reqPlayer,
		run:      ps.cmdSetHome,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix([]string{"none"}, lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "kick",
		aliases:  []string{"k"},
		perm:     "plots.kick",
		usage:    "/plot kick <player | *>",
		desc:     "Kick a player from your plot",
		category: catTeleport,
		required: reqPlayer,
		run:      ps.cmdKick,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix(append(ps.onlinePlayerNames(), "*"), lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "target",
		perm:     "plots.target",
		usage:    "/plot target <<X;Z> | nearest>",
		desc:     "Point your compass at a plot",
		category: catInfo,
		required: reqPlayer,
		run:      ps.cmdTarget,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix([]string{"nearest"}, lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "trust",
		aliases:  []string{"t"},
		perm:     "plots.trust",
		usage:    "/plot trust <player | *>",
		desc:     "Allow a player to build in your plot even when you are offline",
		category: catSettings,
		required: reqPlayer,
		run:      ps.cmdTrust,
		tab: func(c *cmdCtx) []string {
			return filterPrefix(append(ps.onlinePlayerNames(), "*"), lastArg(c.args))
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "add",
		perm:     "plots.add",
		usage:    "/plot add <player | *>",
		desc:     "Allow a player to build in your plot while you are online",
		category: catSettings,
		required: reqPlayer,
		run:      ps.cmdAdd,
		tab: func(c *cmdCtx) []string {
			return filterPrefix(append(ps.onlinePlayerNames(), "*"), lastArg(c.args))
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "deny",
		aliases:  []string{"d", "ban"},
		perm:     "plots.deny",
		usage:    "/plot deny <player | *>",
		desc:     "Deny a player from entering your plot",
		category: catSettings,
		required: reqPlayer,
		run:      ps.cmdDeny,
		tab: func(c *cmdCtx) []string {
			return filterPrefix(append(ps.onlinePlayerNames(), "*"), lastArg(c.args))
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "remove",
		aliases:  []string{"r", "untrust", "ut", "undeny", "unban", "ud", "pardon"},
		perm:     "plots.remove",
		usage:    "/plot remove <player | *>",
		desc:     "Remove a trusted, added, or denied player from your plot",
		category: catSettings,
		run:      ps.cmdRemove,
		tab: func(c *cmdCtx) []string {
			return filterPrefix(append(ps.onlinePlayerNames(), "*"), lastArg(c.args))
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "merge",
		aliases:  []string{"m"},
		perm:     "plots.merge",
		usage:    "/plot merge <all | n | e | s | w> [removeroads]",
		desc:     "Merge your plot with adjacent plots",
		category: catSettings,
		run:      ps.cmdMerge,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				return filterPrefix([]string{"all", "north", "east", "south", "west"}, c.args[0])
			}
			if len(c.args) == 2 {
				return filterPrefix([]string{"true", "false"}, c.args[1])
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "unlink",
		aliases:  []string{"u", "unmerge"},
		perm:     "plots.unlink",
		usage:    "/plot unlink [createroads]",
		desc:     "Unlink a merged plot",
		category: catSettings,
		required: reqPlayer,
		run:      ps.cmdUnlink,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix([]string{"true", "false"}, lastArg(c.args))
			}
			return nil
		},
	})
}

func (ps *Plugin) cmdHome(c *cmdCtx) {
	p := c.player
	if !HasPerm(p, "plots.home") && !HasPerm(p, "plots.visit.owned") {
		ps.sendNoPerm(p, "plots.home")
		return
	}
	if len(c.args) > 2 {
		ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot home [<page> | <alias> | <area;x;y>]")
		return
	}

	var list []*plots.Plot
	page := 1
	switch len(c.args) {
	case 0:
		list = ps.ownedPlotsSorted(p.UUID(), ps.currentArea(c))
	case 1:
		arg := c.args[0]
		if n, err := strconv.Atoi(arg); err == nil {
			page = n
			list = ps.ownedPlotsSorted(p.UUID(), ps.currentArea(c))
		} else if found := ps.resolvePlotFromArg(c, arg); found != nil && found.IsOwner(p.UUID()) {
			list = []*plots.Plot{found}
		} else if area := ps.PlotAreaByName(arg); area != nil {
			list = ps.ownedPlotsInArea(p.UUID(), area)
		} else {
			list = ps.plotsByAlias(arg, p.UUID())
		}
	case 2:
		area := ps.PlotAreaByName(c.args[0])
		if area == nil {
			ps.sendCaption(p, "invalid.found_no_plots")
			return
		}
		if n, err := strconv.Atoi(c.args[1]); err == nil {
			page = n
			list = ps.ownedPlotsInArea(p.UUID(), area)
		} else if id, ok := plots.ParsePlotId(c.args[1]); ok {
			if found := area.PlotAbs(id); found != nil && found.IsOwner(p.UUID()) {
				list = []*plots.Plot{found}
			}
		}
	}

	if len(list) == 0 {
		ps.sendCaption(p, "invalid.found_no_plots")
		return
	}
	if page < 1 || page > len(list) {
		ps.sendCaption(p, "invalid.number_not_in_range", "min", "1", "max", strconv.Itoa(len(list)))
		return
	}
	if ps.teleportToPlot(p, list[page-1], "home") {
		ps.sendCaption(p, "teleport.teleported_to_plot")
	}
}

func (ps *Plugin) cmdVisit(c *cmdCtx) {
	p := c.player
	if len(c.args) == 0 || len(c.args) > 3 {
		ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot visit <player | alias | plot> [area] [#]")
		return
	}
	args := c.args
	if len(args) == 1 && strings.Contains(args[0], ":") {
		args = strings.Split(args[0], ":")
	}

	var area *plots.PlotArea
	page := 1
	hasPage := false
	parsePage := func(s string) (int, bool) {
		if strings.EqualFold(s, "last") || strings.EqualFold(s, "n") {
			return -1, true
		}
		n, err := strconv.Atoi(s)
		return n, err == nil
	}

	switch len(args) {
	case 3:
		area = ps.PlotAreaByName(args[1])
		if area == nil {
			ps.sendCaption(p, "invalid.found_no_plots")
			return
		}
		n, ok := parsePage(args[2])
		if !ok {
			ps.sendCaption(p, "invalid.not_a_number", "value", args[2])
			return
		}
		page, hasPage = n, true
	case 2:
		if n, ok := parsePage(args[1]); ok {
			page, hasPage = n, true
		} else {
			area = ps.PlotAreaByName(args[1])
			if area == nil {
				ps.sendCaption(p, "invalid.not_a_number", "value", args[1])
				return
			}
		}
	}

	var list []*plots.Plot
	targetArg := args[0]
	if strings.ContainsAny(targetArg, ";,") {
		if found := ps.resolvePlotFromArg(c, targetArg); found != nil {
			list = []*plots.Plot{found}
		}
	} else if uid, ok := ps.ResolvePlayerUUID(targetArg); ok && uid != plots.EveryoneUUID {
		if area != nil {
			list = ps.ownedPlotsInArea(uid, area)
		} else {
			list = ps.ownedPlotsSorted(uid, ps.currentArea(c))
		}
		if len(list) == 0 {
			ps.sendCaption(p, "errors.player_no_plots")
			return
		}
	} else if !hasPage {
		list = ps.plotsByAlias(targetArg, uuid.Nil)
	} else {
		ps.sendCaption(p, "errors.invalid_player", "value", targetArg)
		return
	}

	if len(list) == 0 {
		ps.sendCaption(p, "invalid.found_no_plots")
		return
	}
	if page < 0 {
		page = len(list) + 1 + page
	}
	if page < 1 || page > len(list) {
		ps.sendCaption(p, "invalid.number_not_in_range", "min", "1", "max", strconv.Itoa(len(list)))
		return
	}
	plot := list[page-1]
	if !ps.canVisitPlot(p, plot) {
		return
	}
	if ps.teleportToPlot(p, plot, "visit") {
		ps.sendCaption(p, "teleport.teleported_to_plot")
	}
}

func (ps *Plugin) canVisitPlot(p plugin.Player, plot *plots.Plot) bool {
	if !plot.HasOwner() {
		if !HasPerm(p, "plots.visit.unowned") {
			ps.sendNoPerm(p, "plots.visit.unowned")
			return false
		}
		return true
	}
	if plot.IsOwner(p.UUID()) {
		if !HasPerm(p, "plots.visit.owned") && !HasPerm(p, "plots.home") {
			ps.sendNoPerm(p, "plots.visit.owned")
			return false
		}
		return true
	}
	if plot.IsAdded(p.UUID()) {
		if !HasPerm(p, "plots.visit.shared") {
			ps.sendNoPerm(p, "plots.visit.shared")
			return false
		}
		return true
	}
	if !plot.FlagBool("untrusted-visit") && !HasPerm(p, "plots.visit.other") && !HasPerm(p, "plots.admin.visit.untrusted") {
		ps.sendNoPerm(p, "plots.visit.other")
		return false
	}
	if plot.IsDenied(p.UUID()) && !HasPerm(p, "plots.visit.denied") {
		ps.sendNoPerm(p, "plots.visit.denied")
		return false
	}
	return true
}

func (ps *Plugin) cmdMiddle(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	w, ok := ps.WrapWorld(plot.Area.WorldName)
	if !ok {
		return
	}
	center := generator.PlotCenter(w, plot.Area, plot)
	c.player.Teleport(center, plugin.TeleportCommand)
	ps.sendCaption(c.sender, "teleport.teleported_to_plot")
}

func (ps *Plugin) cmdSetHome(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "info.plot_unowned")
		return
	}
	if !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	base := plot.BasePlot()
	if len(c.args) >= 1 {
		arg := strings.ToLower(c.args[0])
		if arg == "none" || arg == "unset" || arg == "reset" || arg == "remove" {
			base.Settings.Position = plots.BlockLoc{}
			if ps.DB != nil {
				_ = ps.DB.SetPosition(base, plots.BlockLoc{})
			}
			ps.sendCaption(c.sender, "position.position_unset")
			return
		}
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot sethome [none]")
		return
	}
	loc := c.player.Location()
	bounds := base.Area.PlotInteriorBounds(base.ID)
	bp := loc.Pos.Block()
	rel := plots.BlockLoc{
		X:     bp.X - bounds.Min.X,
		Y:     bp.Y,
		Z:     bp.Z - bounds.Min.Z,
		Yaw:   loc.Yaw,
		Pitch: loc.Pitch,
		Set:   true,
	}
	base.Settings.Position = rel
	if ps.DB != nil {
		_ = ps.DB.SetPosition(base, rel)
	}
	ps.sendCaption(c.sender, "position.position_set")
}

func (ps *Plugin) cmdKick(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if (!plot.HasOwner() || !plot.IsOwner(c.player.UUID())) && !HasPerm(c.sender, "plots.admin.command.kick") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	if len(c.args) != 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot kick <player | *>")
		return
	}
	var targets []plugin.Player
	if c.args[0] == "*" {
		for _, pl := range ps.playersInPlot(plot) {
			if pl.UUID() != c.player.UUID() && !HasPerm(pl, "plots.admin.entry.denied") {
				targets = append(targets, pl)
			}
		}
	} else {
		target, ok := ps.Server.PlayerNamed(c.args[0])
		if !ok || target.UUID() == c.player.UUID() {
			ps.sendCaption(c.sender, "errors.invalid_player", "value", c.args[0])
			return
		}
		cur := ps.PlotAt(target.Location())
		if cur == nil || cur.BasePlot() != plot.BasePlot() {
			ps.sendCaption(c.sender, "kick.player_not_in_plot", "player", target.Name())
			return
		}
		if HasPerm(target, "plots.admin.entry.denied") {
			ps.sendCaption(c.sender, "kick.cannot_kick_player", "player", target.Name())
			return
		}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		ps.sendCaption(c.sender, "errors.invalid_player", "value", c.args[0])
		return
	}
	for _, t := range targets {
		ps.sendCaption(t, "kick.you_got_kicked")
		ps.kickFromPlot(t, plot)
	}
}

func (ps *Plugin) cmdTarget(c *cmdCtx) {
	loc := c.player.Location()
	area := ps.PlotAreaAt(loc)
	if area == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot_world")
		return
	}
	if len(c.args) == 0 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot target <<X;Z> | nearest>")
		return
	}
	w, ok := ps.WrapWorld(area.WorldName)
	if !ok {
		return
	}
	var target *plots.Plot
	if strings.EqualFold(c.args[0], "nearest") || strings.EqualFold(c.args[0], "near") {
		bestDist := math.MaxFloat64
		for _, pl := range area.BasePlots() {
			center := generator.PlotCenter(w, area, pl)
			dx := center.Pos.X - loc.Pos.X
			dz := center.Pos.Z - loc.Pos.Z
			d2 := dx*dx + dz*dz
			if d2 < bestDist {
				bestDist = d2
				target = pl
			}
		}
	} else {
		target = ps.resolvePlotFromArg(c, c.args[0])
	}
	if target == nil {
		ps.sendCaption(c.sender, "invalid.found_no_plots")
		return
	}
	center := generator.PlotCenter(w, target.Area, target)
	c.player.SetCompassTarget(&center)
	ps.sendCaption(c.sender, "compass.compass_target", "target", target.ID.String())
}

func (ps *Plugin) cmdTrust(c *cmdCtx) {
	ps.modifyPlotTier(c, "trust")
}

func (ps *Plugin) cmdAdd(c *cmdCtx) {
	ps.modifyPlotTier(c, "add")
}

func (ps *Plugin) cmdDeny(c *cmdCtx) {
	ps.modifyPlotTier(c, "deny")
}

func (ps *Plugin) modifyPlotTier(c *cmdCtx, kind string) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "info.plot_unowned")
		return
	}
	adminPerm := "plots.admin.command." + kind
	if kind == "add" {
		adminPerm = "plots.admin.command.trust"
	}
	if !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, adminPerm) {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	if len(c.args) != 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot "+kind+" <player | *>")
		return
	}
	uid, ok := ps.ResolvePlayerUUID(c.args[0])
	if !ok {
		ps.sendCaption(c.sender, "errors.invalid_player", "value", c.args[0])
		return
	}
	base := plot.BasePlot()
	if uid == plots.EveryoneUUID {
		evPerm := "plots.trust.everyone"
		if kind == "deny" {
			evPerm = "plots.deny.everyone"
		}
		if !HasPerm(c.sender, evPerm) && !HasPerm(c.sender, adminPerm) {
			ps.sendCaption(c.sender, "errors.invalid_player", "value", c.args[0])
			return
		}
	}
	if base.IsOwner(uid) {
		if kind == "deny" {
			ps.sendCaption(c.sender, "deny.cant_remove_owner")
		} else {
			ps.sendCaption(c.sender, "member.already_added", "player", ps.PlayerName(uid))
		}
		return
	}

	switch kind {
	case "trust":
		if _, exists := base.Trusted[uid]; exists {
			ps.sendCaption(c.sender, "member.already_added", "player", ps.PlayerName(uid))
			return
		}
		if uid != plots.EveryoneUUID {
			delete(base.Members, uid)
			delete(base.Denied, uid)
			if ps.DB != nil {
				_ = ps.DB.SetMember(base, uid, false)
				_ = ps.DB.SetDenied(base, uid, false)
			}
		}
		base.Trusted[uid] = struct{}{}
		if ps.DB != nil {
			_ = ps.DB.SetTrusted(base, uid, true)
		}
		ps.Server.Events().Fire(&event.PlayerPlotTrustedEvent{Initiator: c.player, Plot: base, Target: uid, Added: true})
		ps.sendCaption(c.sender, "trusted.trusted_added")
	case "add":
		if _, exists := base.Members[uid]; exists {
			ps.sendCaption(c.sender, "member.already_added", "player", ps.PlayerName(uid))
			return
		}
		if uid != plots.EveryoneUUID {
			delete(base.Trusted, uid)
			delete(base.Denied, uid)
			if ps.DB != nil {
				_ = ps.DB.SetTrusted(base, uid, false)
				_ = ps.DB.SetDenied(base, uid, false)
			}
		}
		base.Members[uid] = struct{}{}
		if ps.DB != nil {
			_ = ps.DB.SetMember(base, uid, true)
		}
		ps.Server.Events().Fire(&event.PlayerPlotHelperEvent{Initiator: c.player, Plot: base, Target: uid, Added: true})
		ps.sendCaption(c.sender, "member.member_added")
	case "deny":
		if _, exists := base.Denied[uid]; exists {
			ps.sendCaption(c.sender, "member.already_added", "player", ps.PlayerName(uid))
			return
		}
		if uid != plots.EveryoneUUID {
			delete(base.Trusted, uid)
			delete(base.Members, uid)
			if ps.DB != nil {
				_ = ps.DB.SetTrusted(base, uid, false)
				_ = ps.DB.SetMember(base, uid, false)
			}
		}
		base.Denied[uid] = struct{}{}
		if ps.DB != nil {
			_ = ps.DB.SetDenied(base, uid, true)
		}
		ps.Server.Events().Fire(&event.PlayerPlotDeniedEvent{Initiator: c.player, Plot: base, Target: uid, Added: true})
		if uid == plots.EveryoneUUID {
			for _, pl := range ps.playersInPlot(base) {
				if base.IsDenied(pl.UUID()) {
					ps.sendCaption(pl, "deny.you_got_denied")
					ps.kickFromPlot(pl, base)
				}
			}
		} else if target, online := ps.Server.Player(uid); online {
			if cur := ps.PlotAt(target.Location()); cur != nil && cur.BasePlot() == base {
				ps.sendCaption(target, "deny.you_got_denied")
				ps.kickFromPlot(target, base)
			}
		}
		ps.sendCaption(c.sender, "deny.denied_added")
	}
}

func (ps *Plugin) cmdRemove(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "info.plot_unowned")
		return
	}
	if c.player != nil && !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin.command.remove") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	if len(c.args) != 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot remove <player | *>")
		return
	}
	uid, ok := ps.ResolvePlayerUUID(c.args[0])
	if !ok {
		ps.sendCaption(c.sender, "errors.invalid_player", "value", c.args[0])
		return
	}
	base := plot.BasePlot()
	count := 0
	if uid == plots.EveryoneUUID {
		for u := range base.Trusted {
			delete(base.Trusted, u)
			if ps.DB != nil {
				_ = ps.DB.SetTrusted(base, u, false)
			}
			count++
		}
		for u := range base.Members {
			delete(base.Members, u)
			if ps.DB != nil {
				_ = ps.DB.SetMember(base, u, false)
			}
			count++
		}
		for u := range base.Denied {
			delete(base.Denied, u)
			if ps.DB != nil {
				_ = ps.DB.SetDenied(base, u, false)
			}
			count++
		}
	} else {
		if _, ex := base.Trusted[uid]; ex {
			delete(base.Trusted, uid)
			if ps.DB != nil {
				_ = ps.DB.SetTrusted(base, uid, false)
			}
			ps.Server.Events().Fire(&event.PlayerPlotTrustedEvent{Initiator: c.player, Plot: base, Target: uid, Added: false})
			count++
		}
		if _, ex := base.Members[uid]; ex {
			delete(base.Members, uid)
			if ps.DB != nil {
				_ = ps.DB.SetMember(base, uid, false)
			}
			ps.Server.Events().Fire(&event.PlayerPlotHelperEvent{Initiator: c.player, Plot: base, Target: uid, Added: false})
			count++
		}
		if _, ex := base.Denied[uid]; ex {
			delete(base.Denied, uid)
			if ps.DB != nil {
				_ = ps.DB.SetDenied(base, uid, false)
			}
			ps.Server.Events().Fire(&event.PlayerPlotDeniedEvent{Initiator: c.player, Plot: base, Target: uid, Added: false})
			count++
		}
	}
	if count == 0 {
		ps.sendCaption(c.sender, "member.player_not_removed", "player", c.args[0])
		return
	}
	ps.sendCaption(c.sender, "member.removed_players", "amount", strconv.Itoa(count))
}

func (ps *Plugin) cmdMerge(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "info.plot_unowned")
		return
	}
	if c.player != nil && !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin.command.merge") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}

	dir := plots.DirAll
	if len(c.args) == 0 {
		if c.player != nil {
			dir = facingDirection(c.player.Location().Yaw)
		}
	} else {
		d, ok := plots.ParseDirection(c.args[0])
		if !ok {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot merge <all | n | e | s | w> [removeroads]")
			if c.player != nil {
				ps.sendCaption(c.sender, "help.direction", "dir", strings.ToUpper(facingDirection(c.player.Location().Yaw).String()))
			}
			return
		}
		dir = d
	}

	removeRoads := true
	if len(c.args) >= 2 {
		removeRoads = strings.EqualFold(c.args[1], "true")
	}
	maxPlots := 127
	if ps.Settings != nil && ps.Settings.Limit.MaxPlots > 0 {
		maxPlots = ps.Settings.Limit.MaxPlots
	}
	maxMerge := HasPermRange(c.sender, "plots.merge", maxPlots)
	if maxMerge == 0 {
		maxMerge = maxPlots
	}

	confirmOn := false
	ps.runWithConfirm(c, confirmOn, "/plot merge "+dir.String(), func() {
		cost, afford, err := ps.ChargeAreaPrice(c.player, plot.Area, plot.Area.PriceMerge)
		if !afford || err != nil {
			ps.sendCaption(c.sender, "economy.cannot_afford_merge", "money", formatEcoAmount(ps, cost))
			return
		}
		merged, err := ps.MergePlot(plot, dir, maxMerge, c.player, removeRoads)
		if err != nil || merged == 0 {
			if cost > 0 && c.player != nil {
				if eco, ok := ps.Economy(); ok {
					eco.Deposit(c.player.UUID(), cost)
				}
			}
			ps.sendCaption(c.sender, "merge.no_available_automerge")
			return
		}
		if cost > 0 {
			ps.sendCaption(c.sender, "economy.removed_balance", "money", formatEcoAmount(ps, cost))
		}
		ps.sendCaption(c.sender, "merge.success_merge")
	})
}

func (ps *Plugin) cmdUnlink(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "info.plot_unowned")
		return
	}
	if !plot.IsMerged() {
		ps.sendCaption(c.sender, "merge.unlink_impossible")
		return
	}
	if !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin.command.unlink") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	createRoads := true
	if len(c.args) >= 1 {
		if strings.EqualFold(c.args[0], "false") {
			createRoads = false
		} else if !strings.EqualFold(c.args[0], "true") {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot unlink [createroads]")
			return
		}
	}
	confirmOn := true
	ps.runWithConfirm(c, confirmOn, "/plot unlink "+plot.ID.String(), func() {
		if err := ps.UnlinkPlot(plot, createRoads, "command"); err != nil {
			ps.sendCaption(c.sender, "merge.unmerge_cancelled")
			return
		}
		ps.sendCaption(c.sender, "merge.unlink_success")
	})
}

func (ps *Plugin) ownedPlotsSorted(owner uuid.UUID, preferArea *plots.PlotArea) []*plots.Plot {
	list := ps.PlotsByOwner(owner)
	sort.Slice(list, func(i, j int) bool {
		if preferArea != nil {
			pi := list[i].Area == preferArea
			pj := list[j].Area == preferArea
			if pi != pj {
				return pi
			}
		}
		if list[i].Timestamp != list[j].Timestamp {
			return list[i].Timestamp < list[j].Timestamp
		}
		return list[i].ID.String() < list[j].ID.String()
	})
	return list
}

func (ps *Plugin) ownedPlotsInArea(owner uuid.UUID, area *plots.PlotArea) []*plots.Plot {
	list := area.PlotsByOwner(owner)
	sort.Slice(list, func(i, j int) bool {
		if list[i].Timestamp != list[j].Timestamp {
			return list[i].Timestamp < list[j].Timestamp
		}
		return list[i].ID.String() < list[j].ID.String()
	})
	return list
}

func (ps *Plugin) plotsByAlias(alias string, ownerFilter uuid.UUID) []*plots.Plot {
	var out []*plots.Plot
	for _, a := range ps.PlotAreas() {
		for _, p := range a.BasePlots() {
			if ownerFilter != uuid.Nil && !p.IsOwner(ownerFilter) {
				continue
			}
			if p.Settings.Alias != "" && strings.EqualFold(p.Settings.Alias, alias) {
				out = append(out, p)
			}
		}
	}
	return out
}
