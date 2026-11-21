package plotsplugin

import (
	"fmt"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/generator"
)

func (ps *Plugin) registerPlayerListeners() {
	ps.registerPlayerInteractListeners()
	ps.registerPlayerSessionListeners()
}

func (ps *Plugin) registerPlayerInteractListeners() {
	s := ps.Server
	opts := plugin.ListenOptions{Priority: plugin.High, IgnoreCancelled: true}

	plugin.Listen(s, func(e *plugin.PlayerInteractEvent) {
		if e.Block == nil {
			return
		}
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)

		// Physical triggers (pressure plates, tripwires, farmland trampling, sculk sensors)
		if e.Action == plugin.Physical {
			blk := e.Block.State().Block()
			if plot == nil || !plot.HasOwner() {
				if !HasPerm(e.Player, "plots.admin.interact.road") {
					e.SetCancelled(true)
				}
				return
			}
			if ps.isAddedAndAllowed(e.Player, plot) || HasPerm(e.Player, "plots.admin.interact.other") {
				return
			}
			if blk == registry.BlockSculkSensor || blk == registry.BlockCalibratedSculkSensor {
				if !plot.FlagBool("sculk-sensor-interact") {
					e.SetCancelled(true)
				}
				return
			}
			if isDeviceBlock(blk) && plot.FlagBool("device-interact") {
				return
			}
			if matchesBlockListFlag(plot, "use", e.Block.State()) {
				return
			}
			e.SetCancelled(true)
			return
		}

		if e.Action != plugin.RightClickBlock {
			return
		}

		if plot == nil {
			if !HasPerm(e.Player, "plots.admin.interact.road") {
				ps.sendNoPerm(e.Player, "plots.admin.interact.road")
				e.SetCancelled(true)
			}
			return
		}
		if !plot.HasOwner() {
			if !HasPerm(e.Player, "plots.admin.interact.unowned") {
				ps.sendNoPerm(e.Player, "plots.admin.interact.unowned")
				e.SetCancelled(true)
			}
			return
		}
		if ps.isAddedAndAllowed(e.Player, plot) || HasPerm(e.Player, "plots.admin.interact.other") {
			return
		}

		// Non-member right-click checks: special items first (vehicles, armor stands, spawn eggs)
		if !e.Item.Empty() {
			itemName := e.Item.Item.Name()
			switch {
			case strings.Contains(itemName, "boat") || strings.Contains(itemName, "minecart") || strings.Contains(itemName, "raft"):
				if !plot.FlagBool("vehicle-place") {
					ps.sendNoPerm(e.Player, "plots.admin.interact.other")
					e.SetCancelled(true)
					return
				}
			case strings.HasSuffix(itemName, "_spawn_egg"):
				if !plot.FlagBool("mob-place") {
					ps.sendNoPerm(e.Player, "plots.admin.interact.other")
					e.SetCancelled(true)
					return
				}
			case e.Item.Item == registry.ItemArmorStand || e.Item.Item == registry.ItemEndCrystal:
				if !plot.FlagBool("misc-place") {
					ps.sendNoPerm(e.Player, "plots.admin.interact.other")
					e.SetCancelled(true)
					return
				}
			}
		}

		st := e.Block.State()
		blk := st.Block()
		if matchesBlockListFlag(plot, "use", st) {
			return
		}
		if isDeviceBlock(blk) && plot.FlagBool("device-interact") {
			return
		}
		if blk == registry.BlockLectern && plot.FlagBool("lectern-read-book") {
			return
		}
		if isInteractiveBlock(blk) {
			ps.sendNoPerm(e.Player, "plots.admin.interact.other")
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.PlayerBucketEvent) {
		targetPos := e.Block.Pos
		if !e.Fill {
			targetPos = e.Block.Pos.Offset(e.Face, 1)
		}
		area := ps.PlotAreaAtBlock(e.Block.World, targetPos)
		if area == nil {
			return
		}
		plot := area.PlotAt(targetPos.X, targetPos.Z)
		if !ps.canBuild(e.Player, area, plot, targetPos, e.Fill, e.Block.State()) {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.PlayerInteractEntityEvent) {
		loc := e.Target.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		if plot == nil || !plot.HasOwner() {
			if !HasPerm(e.Player, "plots.admin.interact.road") {
				e.SetCancelled(true)
			}
			return
		}
		if ps.isAddedAndAllowed(e.Player, plot) || HasPerm(e.Player, "plots.admin.interact.other") {
			return
		}
		if !canInteractEntity(plot, e.Target) {
			ps.sendNoPerm(e.Player, "plots.admin.interact.other")
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.PlayerArmorStandManipulateEvent) {
		loc := e.Stand.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		if plot == nil || !plot.HasOwner() {
			if !HasPerm(e.Player, "plots.admin.interact.road") {
				e.SetCancelled(true)
			}
			return
		}
		if !ps.isAddedAndAllowed(e.Player, plot) && !plot.FlagBool("misc-interact") && !HasPerm(e.Player, "plots.admin.interact.other") {
			ps.sendNoPerm(e.Player, "plots.admin.interact.other")
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.PlayerTakeLecternBookEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil || !plot.HasOwner() {
			if !HasPerm(e.Player, "plots.admin.interact.road") {
				e.SetCancelled(true)
			}
			return
		}
		if !ps.isAddedAndAllowed(e.Player, plot) && !HasPerm(e.Player, "plots.admin.interact.other") {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.PlayerAttackEvent) {
		if !ps.allowEntityDamageByPlayer(e.Player, e.Target) {
			e.SetCancelled(true)
		}
	}, opts)
}

func (ps *Plugin) registerPlayerSessionListeners() {
	s := ps.Server
	opts := plugin.ListenOptions{Priority: plugin.High, IgnoreCancelled: true}

	plugin.Listen(s, func(e *plugin.PlayerDropItemEvent) {
		loc := e.Player.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		if plot == nil || !plot.HasOwner() {
			if ps.Settings != nil && ps.Settings.EnabledComponents.KillRoadItems {
				e.SetCancelled(true)
			}
			return
		}
		if !plot.FlagBool("item-drop") && !HasPerm(e.Player, "plots.admin.interact.other") {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.EntityPickupItemEvent) {
		p, ok := e.Entity.(plugin.Player)
		if !ok {
			return
		}
		loc := e.Item.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		if plot != nil && plot.HasOwner() && plot.FlagBool("drop-protection") {
			if !ps.isAddedAndAllowed(p, plot) && !HasPerm(p, "plots.admin.interact.other") {
				e.SetCancelled(true)
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.PlayerFishEvent) {
		loc := e.Player.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		if plot == nil || !plot.HasOwner() {
			if !HasPerm(e.Player, "plots.admin.interact.road") {
				e.SetCancelled(true)
			}
			return
		}
		if !ps.isAddedAndAllowed(e.Player, plot) && !plot.FlagBool("fishing") && !HasPerm(e.Player, "plots.admin.interact.other") {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.PlayerLeashEntityEvent) {
		loc := e.Entity.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		if plot == nil || !plot.HasOwner() {
			if !HasPerm(e.Player, "plots.admin.interact.road") {
				e.SetCancelled(true)
			}
			return
		}
		if !ps.isAddedAndAllowed(e.Player, plot) && !canInteractEntity(plot, e.Entity) && !HasPerm(e.Player, "plots.admin.interact.other") {
			e.SetCancelled(true)
		}
	}, opts)

	// Plot chat & chat flag
	plugin.Listen(s, func(e *plugin.PlayerChatEvent) {
		loc := e.Player.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		if plot != nil && plot.HasOwner() && !plot.FlagBool("chat") && !HasPerm(e.Player, "plots.admin.chat.bypass") {
			ps.sendCaption(e.Player, "chat.plot_chat_denied")
			e.SetCancelled(true)
			return
		}
		st := ps.stateFor(e.Player.UUID())
		plotChatOn := (area.PlotChat && st.plotChat) || area.ForcingPlotChat
		if !plotChatOn || plot == nil {
			return
		}
		e.SetCancelled(true)
		msg := ps.Lang.Format("chat.plot_chat_format",
			"plot_id", plot.ID.String(),
			"sender", e.Player.Name(),
			"msg", e.Message,
		)
		base := plot.BasePlot()
		for _, recipient := range s.Players() {
			rPlot := ps.PlotAt(recipient.Location())
			inSamePlot := rPlot != nil && rPlot.BasePlot() == base
			isSpy := ps.stateFor(recipient.UUID()).chatSpy && HasPerm(recipient, "plots.admin.command.chatspy")
			if inSamePlot || isSpy {
				recipient.SendMessage(msg)
			}
		}
		if ps.Settings == nil || ps.Settings.Chat.LogPlotchatToConsole {
			ps.Log.Info(fmt.Sprintf("[PlotChat %s] %s: %s", plot.ID, e.Player.Name(), e.Message))
		}
	}, opts)

	// Blocked commands flag
	plugin.Listen(s, func(e *plugin.PlayerCommandEvent) {
		loc := e.Player.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		var blocked []string
		if plot != nil && plot.HasOwner() {
			if HasPerm(e.Player, "plots.admin.interact.blockedcommands") {
				return
			}
			if ps.isAddedAndAllowed(e.Player, plot) && HasPerm(e.Player, "plots.admin.command.blocked-cmds.shared") {
				return
			}
			blocked, _ = plot.FlagValue("blocked-cmds").([]string)
		} else if area.HasRoadFlags {
			if HasPerm(e.Player, "plots.admin.command.blocked-cmds.road") {
				return
			}
			blocked, _ = area.RoadFlags.GetValue("blocked-cmds").([]string)
		}
		if len(blocked) == 0 {
			return
		}
		cmd := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e.Command), "/"))
		for _, b := range blocked {
			bc := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(b), "/"))
			if bc == "" {
				continue
			}
			if cmd == bc || strings.HasPrefix(cmd, bc+" ") {
				ps.sendCaption(e.Player, "blockedcmds.command_blocked")
				e.SetCancelled(true)
				return
			}
		}
	}, opts)

	// Keep inventory & teleport on death
	plugin.Listen(s, func(e *plugin.PlayerDeathEvent) {
		plot := ps.PlotAt(e.Player.Location())
		if plot != nil && plot.HasOwner() && plot.FlagBool("keep-inventory") {
			e.KeepInventory = true
			e.KeepLevel = true
		}
	})

	plugin.Listen(s, func(e *plugin.PlayerRespawnEvent) {
		if ps.Settings == nil || !ps.Settings.Teleport.OnDeath {
			return
		}
		plots := ps.PlotsByOwner(e.Player.UUID())
		if len(plots) == 0 {
			return
		}
		p0 := plots[0]
		if w, ok := ps.WrapWorld(p0.Area.WorldName); ok {
			e.Location = generator.SafePlotHome(w, p0.Area, p0, true)
		}
	})

	plugin.Listen(s, func(e *plugin.PlayerJoinEvent) {
		if ps.Settings != nil && ps.Settings.Teleport.OnLogin {
			plots := ps.PlotsByOwner(e.Player.UUID())
			if len(plots) > 0 {
				p0 := plots[0]
				if w, ok := ps.WrapWorld(p0.Area.WorldName); ok {
					e.Player.Teleport(generator.SafePlotHome(w, p0.Area, p0, true), plugin.TeleportPlugin)
				}
			}
		}
		if plot := ps.PlotAt(e.Player.Location()); plot != nil {
			ps.handlePlotEnter(e.Player, plot)
		}
	})

	plugin.Listen(s, func(e *plugin.PlayerQuitEvent) {
		ps.clearPlayerState(e.Player.UUID())
	})
}

func isDeviceBlock(b registry.Block) bool {
	name := b.Name()
	return strings.Contains(name, "_button") ||
		strings.Contains(name, "_pressure_plate") ||
		strings.Contains(name, "_door") ||
		strings.Contains(name, "_trapdoor") ||
		strings.Contains(name, "_fence_gate") ||
		b == registry.BlockLever ||
		b == registry.BlockTripwire ||
		b == registry.BlockTripwireHook ||
		b == registry.BlockNoteBlock ||
		b == registry.BlockRepeater ||
		b == registry.BlockComparator ||
		b == registry.BlockDaylightDetector
}

func isInteractiveBlock(b registry.Block) bool {
	if isDeviceBlock(b) {
		return true
	}
	name := b.Name()
	return strings.Contains(name, "chest") ||
		strings.Contains(name, "shulker_box") ||
		strings.Contains(name, "furnace") ||
		strings.Contains(name, "anvil") ||
		strings.Contains(name, "bed") ||
		strings.Contains(name, "sign") ||
		strings.Contains(name, "candle") ||
		strings.Contains(name, "cauldron") ||
		b == registry.BlockBarrel ||
		b == registry.BlockHopper ||
		b == registry.BlockDispenser ||
		b == registry.BlockDropper ||
		b == registry.BlockCrafter ||
		b == registry.BlockBrewingStand ||
		b == registry.BlockBeacon ||
		b == registry.BlockSmoker ||
		b == registry.BlockBlastFurnace ||
		b == registry.BlockJukebox ||
		b == registry.BlockLectern ||
		b == registry.BlockComposter ||
		b == registry.BlockBeehive ||
		b == registry.BlockBeeNest ||
		b == registry.BlockDragonEgg ||
		b == registry.BlockFlowerPot ||
		b == registry.BlockCake ||
		b == registry.BlockRespawnAnchor
}

func canInteractEntity(plot *plots.Plot, ent plugin.Entity) bool {
	switch e := ent.(type) {
	case plugin.Player:
		return plot.FlagBool("player-interact")
	case plugin.Vehicle:
		return plot.FlagBool("vehicle-use")
	case plugin.Interaction:
		return plot.FlagBool("interaction-interact")
	case plugin.Hanging, plugin.ArmorStand:
		return plot.FlagBool("misc-interact")
	case plugin.Villager:
		return plot.FlagBool("villager-interact")
	default:
		if _, ok := e.(plugin.Tameable); ok {
			if plot.FlagBool("tamed-interact") {
				return true
			}
		}
		switch ent.Type().Category() {
		case registry.MobCategoryMonster:
			return plot.FlagBool("hostile-interact")
		case registry.MobCategoryCreature, registry.MobCategoryWaterCreature, registry.MobCategoryAxolotls, registry.MobCategoryWaterAmbient, registry.MobCategoryAmbient:
			return plot.FlagBool("animal-interact")
		default:
			return plot.FlagBool("misc-interact")
		}
	}
}
