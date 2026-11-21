package plotsplugin

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/generator"
	westorage "github.com/golem-mc/worldedit/storage"
	"github.com/google/uuid"
)

func (ps *Plugin) registerClaimingCommands() {
	ps.addSubCommand(&subCommand{
		name:     "claim",
		aliases:  []string{"c"},
		perm:     "plots.claim",
		usage:    "/plot claim [schematic]",
		desc:     "Claim the current plot",
		category: catClaiming,
		required: reqPlayer,
		run:      ps.cmdClaim,
	})

	ps.addSubCommand(&subCommand{
		name:     "auto",
		aliases:  []string{"a"},
		perm:     "plots.auto",
		usage:    "/plot auto [length,width] [schematic]",
		desc:     "Claim the nearest free plot",
		category: catClaiming,
		required: reqPlayer,
		run:      ps.cmdAuto,
	})

	ps.addSubCommand(&subCommand{
		name:     "delete",
		aliases:  []string{"del", "dispose", "unclaim"},
		perm:     "plots.delete",
		usage:    "/plot delete",
		desc:     "Delete and unclaim the current plot",
		category: catClaiming,
		run:      ps.cmdDelete,
	})

	ps.addSubCommand(&subCommand{
		name:     "clear",
		aliases:  []string{"reset"},
		perm:     "plots.clear",
		usage:    "/plot clear",
		desc:     "Reset the current plot to default terrain",
		category: catAppearance,
		run:      ps.cmdClear,
	})

	ps.addSubCommand(&subCommand{
		name:     "setowner",
		aliases:  []string{"owner", "so", "seto"},
		perm:     "plots.admin.command.setowner",
		usage:    "/plot setowner <player | none>",
		desc:     "Set the plot owner",
		category: catClaiming,
		run:      ps.cmdSetOwner,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix(append(ps.onlinePlayerNames(), "none"), lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "buy",
		perm:     "plots.buy",
		usage:    "/plot buy [plot]",
		desc:     "Buy the plot you are standing on if it is for sale",
		category: catClaiming,
		required: reqPlayer,
		run:      ps.cmdBuy,
	})

	ps.addSubCommand(&subCommand{
		name:     "grant",
		perm:     "plots.grant",
		usage:    "/plot grant <check | add> <player>",
		desc:     "Check or add plot grants for a player",
		category: catClaiming,
		run:      ps.cmdGrant,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				return filterPrefix([]string{"check", "add"}, c.args[0])
			}
			if len(c.args) == 2 {
				return filterPrefix(ps.onlinePlayerNames(), c.args[1])
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "copy",
		aliases:  []string{"copypaste"},
		perm:     "plots.copy",
		usage:    "/plot copy <X;Z>",
		desc:     "Copy the current plot to another unowned plot",
		category: catClaiming,
		required: reqPlayer,
		run:      ps.cmdCopy,
	})

	ps.addSubCommand(&subCommand{
		name:     "move",
		perm:     "plots.move",
		usage:    "/plot move <X;Z>",
		desc:     "Move the current plot to another unowned plot",
		category: catClaiming,
		required: reqPlayer,
		run:      ps.cmdMove,
	})

	ps.addSubCommand(&subCommand{
		name:     "swap",
		aliases:  []string{"switch"},
		perm:     "plots.swap",
		usage:    "/plot swap <X;Z>",
		desc:     "Swap the current plot with another plot",
		category: catClaiming,
		required: reqPlayer,
		run:      ps.cmdSwap,
	})

	ps.addSubCommand(&subCommand{
		name:     "leave",
		perm:     "plots.leave",
		usage:    "/plot leave",
		desc:     "Remove yourself from a plot you are added to",
		category: catClaiming,
		required: reqPlayer,
		run:      ps.cmdLeave,
	})
}

func (ps *Plugin) checkPlotLimitAndGrants(p plugin.Player, area *plots.PlotArea, need int) (usedGrants int, ok bool) {
	allowed := ps.allowedPlots(p)
	current := ps.playerPlotCount(p.UUID(), area.WorldName)
	if current+need <= allowed {
		return 0, true
	}
	deficit := (current + need) - allowed
	grants := ps.grantedPlots(p.UUID())
	if grants >= deficit {
		return deficit, true
	}
	ps.sendCaption(p, "permission.cant_claim_more_plots", "amount", strconv.Itoa(allowed))
	return 0, false
}

func (ps *Plugin) applyClaimSchematic(area *plots.PlotArea, plot *plots.Plot, schemName string) {
	if schemName == "" && area.SchematicOnClaim && area.SchematicFile != "" && !strings.EqualFold(area.SchematicFile, "null") {
		schemName = area.SchematicFile
	}
	if schemName == "" {
		return
	}
	schemDir := "schematics"
	if ps.Settings != nil && ps.Settings.Paths.Schematics != "" {
		schemDir = ps.Settings.Paths.Schematics
	}
	base := filepath.Join(ps.Server.DataDir(), schemDir, schemName)
	var cb *westorage.Clipboard
	for _, cand := range []string{base, base + ".schem", base + ".schematic"} {
		if loaded, err := westorage.ReadSchematicFile(cand, ""); err == nil {
			cb = loaded
			break
		}
	}
	if cb == nil {
		return
	}
	if w, ok := ps.WrapWorld(area.WorldName); ok {
		pasteOnTop := ps.Settings != nil && ps.Settings.Schematics.PasteOnTop
		generator.PastePlotSchematic(w, area, plot, cb, pasteOnTop)
	}
}

func (ps *Plugin) cmdClaim(c *cmdCtx) {
	p := c.player
	area := ps.PlotAreaAt(p.Location())
	if area == nil {
		ps.sendCaption(p, "errors.not_in_plot_world")
		return
	}
	bp := p.Location().Pos.Block()
	id, ok := area.PlotIdAt(bp.X, bp.Z)
	if !ok {
		ps.sendCaption(p, "errors.not_in_plot")
		return
	}
	if existing := area.PlotAbs(id); existing != nil && existing.HasOwner() {
		ps.sendCaption(p, "working.plot_is_claimed")
		return
	}
	schem := ""
	if len(c.args) >= 1 {
		schem = c.args[0]
		if area.SchematicClaimSpecify {
			if !HasPerm(p, "plots.claim."+strings.ToLower(schem)) && !HasPerm(p, "plots.admin.command.schematic") {
				ps.sendCaption(p, "permission.no_schematic_permission", "value", schem)
				return
			}
		}
	}
	if !HasPerm(p, "plots.admin.border.bypass") && area.WorldBorder {
		b := area.PlotInteriorBounds(id)
		lim := area.WorldBorderRadius()
		if absInt(b.Min.X) > lim || absInt(b.Max.X) > lim || absInt(b.Min.Z) > lim || absInt(b.Max.Z) > lim {
			ps.sendCaption(p, "border.denied")
			return
		}
	}
	usedGrants, limitOK := ps.checkPlotLimitAndGrants(p, area, 1)
	if !limitOK {
		return
	}

	cost, afford, err := ps.ChargeAreaPrice(p, area, area.PriceClaim)
	if !afford || err != nil {
		bal := 0.0
		if eco, ok := ps.Economy(); ok {
			bal = eco.Balance(p.UUID())
		}
		ps.sendCaption(p, "economy.cannot_afford_plot",
			"money", formatEcoAmount(ps, cost),
			"balance", formatEcoAmount(ps, bal),
		)
		return
	}

	claimed, err := ps.ClaimPlot(area, id, p.UUID(), p, false)
	if err != nil {
		if cost > 0 {
			if eco, ok := ps.Economy(); ok {
				eco.Deposit(p.UUID(), cost)
			}
		}
		ps.sendCaption(p, "working.plot_not_claimed")
		return
	}
	if cost > 0 {
		bal := 0.0
		if eco, ok := ps.Economy(); ok {
			bal = eco.Balance(p.UUID())
		}
		ps.sendCaption(p, "economy.removed_balance",
			"money", formatEcoAmount(ps, cost),
			"balance", formatEcoAmount(ps, bal),
		)
	}
	if usedGrants > 0 {
		rem := ps.grantedPlots(p.UUID()) - usedGrants
		ps.setGrantedPlots(p.UUID(), rem)
		ps.sendCaption(p, "economy.removed_granted_plot",
			"used_grants", strconv.Itoa(usedGrants),
			"remaining_grants", strconv.Itoa(max(rem, 0)),
		)
	}
	ps.applyClaimSchematic(area, claimed, schem)
	ps.sendCaption(p, "working.claimed", "world", area.WorldName, "plot", claimed.ID.String())
	if ps.Settings != nil && ps.Settings.Teleport.OnClaim {
		ps.teleportToPlot(p, claimed, "claim")
	}
}

func (ps *Plugin) cmdAuto(c *cmdCtx) {
	p := c.player
	area := ps.currentArea(c)
	if area == nil {
		ps.sendCaption(p, "errors.not_in_plot_world")
		return
	}
	sizeX, sizeZ := 1, 1
	schem := ""
	mega := false
	if len(c.args) >= 1 {
		arg0 := c.args[0]
		if parts := strings.FieldsFunc(arg0, func(r rune) bool { return r == ',' || r == ';' }); len(parts) == 2 {
			x, err1 := strconv.Atoi(parts[0])
			z, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil {
				if x < 1 || z < 1 {
					ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot auto [length,width]")
					return
				}
				sizeX, sizeZ = x, z
				mega = x*z > 1
				if len(c.args) >= 2 {
					schem = c.args[1]
				}
			} else {
				schem = arg0
			}
		} else {
			schem = arg0
		}
	}
	if mega && !HasPerm(p, "plots.auto.mega") {
		ps.sendNoPerm(p, "plots.auto.mega")
		return
	}
	maxAuto := 4
	if ps.Settings != nil && ps.Settings.Claim.MaxAutoArea > 0 {
		maxAuto = ps.Settings.Claim.MaxAutoArea
	}
	if sizeX*sizeZ > maxAuto {
		ps.sendCaption(p, "permission.cant_claim_more_plots_num", "amount", strconv.Itoa(maxAuto))
		return
	}
	need := sizeX * sizeZ
	usedGrants, limitOK := ps.checkPlotLimitAndGrants(p, area, need)
	if !limitOK {
		return
	}

	// Charge claim price * count (+ merge price if mega)
	totalCost := 0.0
	if area.UseEconomy && !HasPerm(p, "plots.eco.bypass") {
		if eco, ok := ps.Economy(); ok {
			owned := len(ps.PlotsByOwner(p.UUID()))
			cPrice := plots.EvalPrice(area.PriceClaim, owned) * float64(need)
			mPrice := 0.0
			if need > 1 {
				mPrice = plots.EvalPrice(area.PriceMerge, need)
			}
			totalCost = cPrice + mPrice
			if totalCost > 0 {
				if !eco.Has(p.UUID(), totalCost) {
					ps.sendCaption(p, "economy.cannot_afford_plot",
						"money", eco.Format(totalCost),
						"balance", eco.Format(eco.Balance(p.UUID())),
					)
					return
				}
				eco.Withdraw(p.UUID(), totalCost)
			}
		}
	}

	claimed, err := ps.AutoClaim(area, p.UUID(), p, sizeX, sizeZ)
	if err != nil {
		if totalCost > 0 {
			if eco, ok := ps.Economy(); ok {
				eco.Deposit(p.UUID(), totalCost)
			}
		}
		ps.sendCaption(p, "errors.no_free_plots")
		return
	}
	if totalCost > 0 {
		bal := 0.0
		if eco, ok := ps.Economy(); ok {
			bal = eco.Balance(p.UUID())
		}
		ps.sendCaption(p, "economy.removed_balance",
			"money", formatEcoAmount(ps, totalCost),
			"balance", formatEcoAmount(ps, bal),
		)
	}
	if usedGrants > 0 {
		rem := ps.grantedPlots(p.UUID()) - usedGrants
		ps.setGrantedPlots(p.UUID(), rem)
	}
	ps.applyClaimSchematic(area, claimed, schem)
	ps.sendCaption(p, "working.claimed", "world", area.WorldName, "plot", claimed.ID.String())
	if ps.Settings == nil || ps.Settings.Teleport.OnAuto {
		ps.teleportToPlot(p, claimed, "auto")
	}
}

func (ps *Plugin) cmdDelete(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "info.plot_unowned")
		return
	}
	if c.player != nil && !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin.command.delete") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	confirmOn := true
	ps.runWithConfirm(c, confirmOn, "/plot delete "+plot.ID.String(), func() {
		start := time.Now()
		area := plot.Area
		numPlots := len(plot.ConnectedPlots())
		if ps.Settings != nil && ps.Settings.Teleport.OnDelete {
			for _, pl := range ps.playersInPlot(plot) {
				ps.kickFromPlot(pl, plot)
			}
		}
		if err := ps.DeletePlot(plot, true); err != nil {
			ps.sendCaption(c.sender, "errors.error", "value", err.Error())
			return
		}
		if c.player != nil && area.UseEconomy {
			refund := ps.RefundAreaPrice(c.player.UUID(), area, area.PriceSell) * float64(numPlots)
			if refund > 0 {
				ps.sendCaption(c.sender, "economy.added_balance", "money", formatEcoAmount(ps, refund))
			}
		}
		ps.sendCaption(c.sender, "working.deleting_done",
			"amount", strconv.FormatInt(time.Since(start).Milliseconds(), 10),
			"world", area.WorldName,
			"plot", plot.ID.String(),
		)
	})
}

func (ps *Plugin) cmdClear(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if c.player != nil && !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin.command.clear") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	if ps.Settings != nil && ps.Settings.Done.RestrictBuilding && plot.FlagString("done") != "" && !HasPerm(c.sender, "plots.continue") {
		ps.sendCaption(c.sender, "done.done_already_done")
		return
	}
	confirmOn := true
	ps.runWithConfirm(c, confirmOn, "/plot clear "+plot.ID.String(), func() {
		start := time.Now()
		if ps.Settings != nil && ps.Settings.Teleport.OnClear {
			for _, pl := range ps.playersInPlot(plot) {
				ps.teleportToPlot(pl, plot, "clear")
			}
		}
		if err := ps.ClearPlot(plot); err != nil {
			ps.sendCaption(c.sender, "errors.error", "value", err.Error())
			return
		}
		_, _ = ps.RemoveFlag(plot, "done")
		_, _ = ps.RemoveFlag(plot, "analysis")
		ps.sendCaption(c.sender, "working.clearing_done",
			"world", plot.Area.WorldName,
			"amount", strconv.FormatInt(time.Since(start).Milliseconds(), 10),
			"plot", plot.ID.String(),
		)
	})
}

func (ps *Plugin) cmdSetOwner(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if len(c.args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot setowner <player | none>")
		return
	}
	targetStr := c.args[0]
	if strings.EqualFold(targetStr, "none") || strings.EqualFold(targetStr, "null") || targetStr == "-" {
		confirmOn := true
		ps.runWithConfirm(c, confirmOn, "/plot setowner none", func() {
			if err := ps.DeletePlot(plot, false); err != nil {
				ps.sendCaption(c.sender, "errors.error", "value", err.Error())
				return
			}
			ps.sendCaption(c.sender, "owner.set_owner")
		})
		return
	}
	uid, ok := ps.ResolvePlayerUUID(targetStr)
	if !ok || uid == plots.EveryoneUUID {
		ps.sendCaption(c.sender, "errors.invalid_player", "value", targetStr)
		return
	}
	if plot.IsOwner(uid) {
		ps.sendCaption(c.sender, "member.already_owner", "player", ps.PlayerName(uid))
		return
	}
	confirmOn := true
	ps.runWithConfirm(c, confirmOn, "/plot setowner "+targetStr, func() {
		if err := ps.SetOwner(plot, uid, c.sender); err != nil {
			ps.sendCaption(c.sender, "owner.set_owner_cancelled")
			return
		}
		if plot.IsDenied(uid) {
			delete(plot.BasePlot().Denied, uid)
			if ps.DB != nil {
				_ = ps.DB.SetDenied(plot.BasePlot(), uid, false)
			}
		}
		ps.sendCaption(c.sender, "owner.set_owner")
		if other, online := ps.Server.Player(uid); online {
			ps.sendCaption(other, "owner.now_owner", "plot", plot.Area.FullID()+";"+plot.ID.String())
		}
	})
}

func (ps *Plugin) cmdBuy(c *cmdCtx) {
	p := c.player
	var plot *plots.Plot
	if len(c.args) >= 1 {
		plot = ps.resolvePlotFromArg(c, c.args[0])
	} else {
		plot = ps.currentPlot(c)
	}
	if plot == nil {
		ps.sendCaption(p, "errors.not_in_plot")
		return
	}
	if !plot.Area.UseEconomy {
		ps.sendCaption(p, "economy.econ_disabled")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(p, "info.plot_unowned")
		return
	}
	if plot.IsOwner(p.UUID()) {
		ps.sendCaption(p, "economy.cannot_buy_own")
		return
	}
	if _, ok := ps.checkPlotLimitAndGrants(p, plot.Area, len(plot.ConnectedPlots())); !ok {
		return
	}
	oldOwner := plot.BasePlot().Owner
	price, err := ps.BuyPlot(p, plot)
	if err != nil {
		ps.sendCaption(p, "economy.not_for_sale")
		return
	}
	ps.sendCaption(p, "economy.removed_balance", "money", formatEcoAmount(ps, price))
	if oldOwner != uuid.Nil {
		if prev, online := ps.Server.Player(oldOwner); online {
			ps.sendCaption(prev, "economy.plot_sold",
				"plot", plot.ID.String(),
				"player", p.Name(),
				"price", formatEcoAmount(ps, price),
			)
		}
	}
	ps.sendCaption(p, "working.claimed", "world", plot.Area.WorldName, "plot", plot.ID.String())
}

func (ps *Plugin) cmdGrant(c *cmdCtx) {
	if len(c.args) != 2 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot grant <check | add> <player>")
		return
	}
	action := strings.ToLower(c.args[0])
	if action != "check" && action != "add" {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot grant <check | add> <player>")
		return
	}
	perm := "plots.grant." + action
	if !HasPerm(c.sender, perm) {
		ps.sendNoPerm(c.sender, perm)
		return
	}
	uid, ok := ps.ResolvePlayerUUID(c.args[1])
	if !ok || uid == plots.EveryoneUUID {
		ps.sendCaption(c.sender, "errors.invalid_player", "value", c.args[1])
		return
	}
	cur := ps.grantedPlots(uid)
	if action == "check" {
		ps.sendCaption(c.sender, "grants.granted_plots", "amount", strconv.Itoa(cur))
		return
	}
	cur++
	ps.setGrantedPlots(uid, cur)
	ps.sendCaption(c.sender, "grants.added", "grants", strconv.Itoa(cur))
}

func (ps *Plugin) cmdCopy(c *cmdCtx) {
	plot1 := ps.currentPlot(c)
	if plot1 == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot1.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	if len(c.args) != 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot copy <X;Z>")
		return
	}
	plot2 := ps.resolvePlotFromArg(c, c.args[0])
	if plot2 == nil {
		ps.sendCaption(c.sender, "invalid.found_no_plots")
		return
	}
	if plot1.Area == plot2.Area && plot1.ID == plot2.ID {
		ps.sendCaption(c.sender, "invalid.origin_cant_be_target")
		return
	}
	if plot1.Area.PlotWidth != plot2.Area.PlotWidth {
		ps.sendCaption(c.sender, "errors.plotworld_incompatible")
		return
	}
	if _, err := ps.CopyPlot(plot1, plot2.Area, plot2.ID); err != nil {
		ps.sendCaption(c.sender, "move.requires_unowned")
		return
	}
	ps.sendCaption(c.sender, "move.copy_success", "origin", plot1.ID.String(), "target", plot2.ID.String())
}

func (ps *Plugin) cmdMove(c *cmdCtx) {
	plot1 := ps.currentPlot(c)
	if plot1 == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot1.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	if len(c.args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot move <X;Z>")
		return
	}
	if plot1.IsMerged() {
		ps.sendCaption(c.sender, "move.move_merged")
		return
	}
	plot2 := ps.resolvePlotFromArg(c, c.args[0])
	if plot2 == nil {
		if targetArea := ps.PlotAreaByName(c.args[0]); targetArea != nil {
			plot2 = targetArea.GetOrCreatePlot(plot1.ID)
		}
	}
	if plot2 == nil {
		ps.sendCaption(c.sender, "invalid.found_no_plots")
		return
	}
	if plot1.Area == plot2.Area && plot1.ID == plot2.ID {
		ps.sendCaption(c.sender, "invalid.origin_cant_be_target")
		return
	}
	originStr := plot1.ID.String()
	targetStr := plot2.ID.String()
	if err := ps.MovePlot(plot1, plot2.Area, plot2.ID); err != nil {
		ps.sendCaption(c.sender, "move.requires_unowned")
		return
	}
	ps.sendCaption(c.sender, "move.move_success", "origin", originStr, "target", targetStr)
}

func (ps *Plugin) cmdSwap(c *cmdCtx) {
	plot1 := ps.currentPlot(c)
	if plot1 == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot1.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	if len(c.args) != 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot swap <X;Z>")
		return
	}
	plot2 := ps.resolvePlotFromArg(c, c.args[0])
	if plot2 == nil {
		ps.sendCaption(c.sender, "invalid.found_no_plots")
		return
	}
	if plot1.Area == plot2.Area && plot1.ID == plot2.ID {
		ps.sendCaption(c.sender, "invalid.origin_cant_be_target")
		return
	}
	if plot1.IsMerged() || plot2.IsMerged() {
		ps.sendCaption(c.sender, "swap.swap_merged")
		return
	}
	if err := ps.SwapPlots(plot1, plot2); err != nil {
		ps.sendCaption(c.sender, "errors.plotworld_incompatible")
		return
	}
	ps.sendCaption(c.sender, "swap.swap_success", "origin", plot1.ID.String(), "target", plot2.ID.String())
}

func (ps *Plugin) cmdLeave(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "info.plot_unowned")
		return
	}
	base := plot.BasePlot()
	uid := c.player.UUID()
	if base.IsOwner(uid) {
		ps.sendCaption(c.sender, "member.plot_cant_leave_owner")
		return
	}
	removed := false
	if _, ok := base.Trusted[uid]; ok {
		delete(base.Trusted, uid)
		if ps.DB != nil {
			_ = ps.DB.SetTrusted(base, uid, false)
		}
		removed = true
	}
	if _, ok := base.Members[uid]; ok {
		delete(base.Members, uid)
		if ps.DB != nil {
			_ = ps.DB.SetMember(base, uid, false)
		}
		removed = true
	}
	if removed {
		ps.sendCaption(c.sender, "member.plot_left", "player", c.player.Name())
	} else {
		ps.sendCaption(c.sender, "members.not_added_trusted")
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
