package plotsquaredplugin

import (
	"slices"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plotsquared"
	"github.com/golem-mc/worldedit/block"
)

// registerProtection attaches all block, player, entity, and movement protection listeners.
func (ps *PlotSquared) registerProtection() {
	if ps.Settings != nil && !ps.Settings.EnabledComponents.Events {
		return
	}
	ps.registerBlockListeners()
	ps.registerPlayerListeners()
	ps.registerEntityListeners()
	ps.registerMovementListeners()
}

func (ps *PlotSquared) canBuild(p plugin.Player, area *plotsquared.PlotArea, plot *plotsquared.Plot, pos plugin.BlockPos, isBreak bool, st plugin.BlockState) bool {
	if area == nil {
		return true
	}
	if plot == nil {
		perm := "plots.admin.build.road"
		if isBreak {
			perm = "plots.admin.destroy.road"
		}
		if HasPerm(p, perm) {
			return true
		}
		ps.sendNoPerm(p, perm)
		return false
	}

	if isBreak && pos.Y == area.MinGenHeight && !HasPerm(p, "plots.admin.destroy.groundlevel") {
		ps.sendNoPerm(p, "plots.admin.destroy.groundlevel")
		return false
	}
	if (pos.Y < area.MinBuildHeight || pos.Y >= area.MaxBuildHeight) && !HasPerm(p, "plots.admin.build.heightlimit") {
		ps.sendCaption(p, "height.height_limit", "min", itoa(area.MinBuildHeight), "max", itoa(area.MaxBuildHeight))
		return false
	}

	if !plot.HasOwner() {
		perm := "plots.admin.build.unowned"
		if isBreak {
			perm = "plots.admin.destroy.unowned"
		}
		if HasPerm(p, perm) {
			return true
		}
		ps.sendNoPerm(p, perm)
		return false
	}

	if ps.isAddedAndAllowed(p, plot) {
		if ps.Settings != nil && ps.Settings.Done.RestrictBuilding && plot.FlagString("done") != "" {
			if !HasPerm(p, "plots.admin.build.other") {
				ps.sendCaption(p, "done.building_restricted")
				return false
			}
		}
		return true
	}

	// Non-member: check break / place block list flags
	flagName := "place"
	perm := "plots.admin.build.other"
	if isBreak {
		flagName = "break"
		perm = "plots.admin.destroy.other"
	}
	if matchesBlockListFlag(plot, flagName, st) {
		return true
	}
	if HasPerm(p, perm) {
		return true
	}
	ps.sendNoPerm(p, perm)
	return false
}

// isAddedAndAllowed returns true if player is owner, trusted, or a member with an owner online.
func (ps *PlotSquared) isAddedAndAllowed(p plugin.Player, plot *plotsquared.Plot) bool {
	if p == nil || plot == nil || !plot.HasOwner() {
		return false
	}
	uid := p.UUID()
	if plot.IsOwner(uid) || plot.IsTrusted(uid) {
		return true
	}
	if plot.IsMember(uid) {
		for _, owner := range plot.Owners() {
			if _, online := ps.Server.Player(owner); online {
				return true
			}
		}
	}
	return false
}

func matchesBlockListFlag(plot *plotsquared.Plot, flagName string, st plugin.BlockState) bool {
	if plot == nil {
		return false
	}
	list, _ := plot.FlagValue(flagName).([]string)
	if len(list) == 0 {
		return false
	}
	blkName := strings.ToLower(block.FormatState(st))
	if idx := strings.IndexByte(blkName, '['); idx > 0 {
		blkName = blkName[:idx]
	}
	if !strings.Contains(blkName, ":") {
		blkName = "minecraft:" + blkName
	}
	for _, entry := range list {
		e := strings.ToLower(entry)
		if e == "*" || e == "any" || e == blkName || e == strings.TrimPrefix(blkName, "minecraft:") {
			return true
		}
		if after, ok := strings.CutPrefix(e, "#"); ok {
			tagName := after
			if registry.BlockInTag(blkName, tagName) {
				return true
			}
		}
	}
	return false
}

func (ps *PlotSquared) roadFlagBool(area *plotsquared.PlotArea, flagName string) bool {
	if area == nil || !area.HasRoadFlags {
		return false
	}
	v, _ := area.RoadFlags.GetValue(flagName).(bool)
	return v
}

func (ps *PlotSquared) registerBlockListeners() {
	ps.registerBlockBuildListeners()
	ps.registerBlockNatureListeners()
	ps.registerBlockMechanicListeners()
}

func (ps *PlotSquared) registerBlockBuildListeners() {
	s := ps.Server
	opts := plugin.ListenOptions{Priority: plugin.High, IgnoreCancelled: true}

	plugin.Listen(s, func(e *plugin.BlockPlaceEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if !ps.canBuild(e.Player, area, plot, e.Block.Pos, false, e.Placed) {
			e.SetCancelled(true)
			return
		}
		// Check multi-block placements (beds, doors) do not spill outside plot
		for _, pos := range e.Blocks {
			p2 := area.PlotAt(pos.X, pos.Z)
			if p2 != plot && !ps.canBuild(e.Player, area, p2, pos, false, e.Placed) {
				e.SetCancelled(true)
				return
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.BlockBreakEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if !ps.canBuild(e.Player, area, plot, e.Block.Pos, true, e.State) {
			e.SetCancelled(true)
			return
		}
		if plot != nil && plot.HasOwner() && !plot.FlagBool("tile-drop") {
			e.DropItems = false
		}
	}, plugin.ListenOptions{Priority: plugin.Lowest, IgnoreCancelled: true})

	plugin.Listen(s, func(e *plugin.BlockDropItemEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot != nil && plot.HasOwner() && !plot.FlagBool("tile-drop") {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.BlockDamageEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot != nil && plot.HasOwner() && plot.FlagBool("instabreak") {
			if ps.isAddedAndAllowed(e.Player, plot) || matchesBlockListFlag(plot, "break", e.Block.State()) {
				e.InstaBreak = true
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.SignChangeEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil {
			if !HasPerm(e.Player, "plots.admin.interact.road") {
				e.SetCancelled(true)
			}
			return
		}
		if !plot.HasOwner() {
			if !HasPerm(e.Player, "plots.admin.interact.unowned") {
				e.SetCancelled(true)
			}
			return
		}
		if !ps.isAddedAndAllowed(e.Player, plot) && !plot.FlagBool("edit-sign") && !HasPerm(e.Player, "plots.admin.interact.other") {
			ps.sendNoPerm(e.Player, "plots.admin.interact.other")
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.BlockBurnEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil || !plot.HasOwner() {
			if !ps.roadFlagBool(area, "block-burn") {
				e.SetCancelled(true)
			}
			return
		}
		if !plot.FlagBool("block-burn") {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.BlockIgniteEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil || !plot.HasOwner() {
			if e.Player != nil && HasPerm(e.Player, "plots.admin.build.road") {
				return
			}
			e.SetCancelled(true)
			return
		}
		if !plot.FlagBool("block-ignition") {
			e.SetCancelled(true)
			return
		}
		if e.Player != nil && !ps.isAddedAndAllowed(e.Player, plot) && !HasPerm(e.Player, "plots.admin.build.other") {
			e.SetCancelled(true)
		}
	}, opts)
}

func (ps *PlotSquared) registerBlockNatureListeners() {
	s := ps.Server
	opts := plugin.ListenOptions{Priority: plugin.High, IgnoreCancelled: true}

	plugin.Listen(s, func(e *plugin.BlockSpreadEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil || !plot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		srcPlot := area.PlotAt(e.Source.Pos.X, e.Source.Pos.Z)
		if srcPlot != plot {
			e.SetCancelled(true)
			return
		}
		srcBlk := e.Source.State().Block()
		switch srcBlk {
		case registry.BlockGrassBlock:
			if !plot.FlagBool("grass-grow") {
				e.SetCancelled(true)
			}
		case registry.BlockMycelium:
			if !plot.FlagBool("mycel-grow") {
				e.SetCancelled(true)
			}
		case registry.BlockVine, registry.BlockWeepingVines, registry.BlockTwistingVines, registry.BlockCaveVines:
			if !plot.FlagBool("vine-grow") {
				e.SetCancelled(true)
			}
		case registry.BlockKelp, registry.BlockKelpPlant:
			if !plot.FlagBool("kelp-grow") {
				e.SetCancelled(true)
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.BlockFormEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil || !plot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		nb := e.NewState.Block()
		switch {
		case nb == registry.BlockSnow || nb == registry.BlockSnowBlock:
			if !plot.FlagBool("snow-form") {
				e.SetCancelled(true)
			}
		case nb == registry.BlockIce || nb == registry.BlockFrostedIce:
			if !plot.FlagBool("ice-form") {
				e.SetCancelled(true)
			}
		case strings.HasSuffix(nb.Name(), "_concrete"):
			if !plot.FlagBool("concrete-harden") {
				e.SetCancelled(true)
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.BlockFadeEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil || !plot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		cur := e.Block.State().Block()
		name := cur.Name()
		switch {
		case cur == registry.BlockIce:
			if !plot.FlagBool("ice-melt") {
				e.SetCancelled(true)
			}
		case cur == registry.BlockSnow:
			if !plot.FlagBool("snow-melt") {
				e.SetCancelled(true)
			}
		case cur == registry.BlockFarmland:
			if !plot.FlagBool("soil-dry") {
				e.SetCancelled(true)
			}
		case strings.Contains(name, "coral"):
			if !plot.FlagBool("coral-dry") {
				e.SetCancelled(true)
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.MoistureChangeEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil || !plot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		if !plot.FlagBool("soil-dry") {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.BlockGrowEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil || !plot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		if !plot.FlagBool("crop-grow") {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.LeavesDecayEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil || !plot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		if !plot.FlagBool("leaf-decay") {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.BlockFertilizeEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		srcPlot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if srcPlot == nil || !srcPlot.HasOwner() {
			if e.Player == nil || !HasPerm(e.Player, "plots.admin.build.road") {
				e.SetCancelled(true)
				return
			}
		}
		if e.Player != nil && srcPlot != nil && !ps.isAddedAndAllowed(e.Player, srcPlot) && !HasPerm(e.Player, "plots.admin.build.other") {
			e.SetCancelled(true)
			return
		}
		e.Blocks = slices.DeleteFunc(e.Blocks, func(bc plugin.BlockChange) bool {
			return area.PlotAt(bc.Pos.X, bc.Pos.Z) != srcPlot
		})
	}, opts)

	plugin.Listen(s, func(e *plugin.StructureGrowEvent) {
		area := ps.PlotAreaAtBlock(e.World, e.Pos)
		if area == nil {
			return
		}
		srcPlot := area.PlotAt(e.Pos.X, e.Pos.Z)
		if srcPlot == nil || !srcPlot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		e.Blocks = slices.DeleteFunc(e.Blocks, func(bc plugin.BlockChange) bool {
			return area.PlotAt(bc.Pos.X, bc.Pos.Z) != srcPlot
		})
	}, opts)
}

func (ps *PlotSquared) registerBlockMechanicListeners() {
	s := ps.Server
	opts := plugin.ListenOptions{Priority: plugin.High, IgnoreCancelled: true}

	plugin.Listen(s, func(e *plugin.BlockFromToEvent) {
		area := ps.PlotAreaAtBlock(e.From.World, e.From.Pos)
		if area == nil {
			return
		}
		fromPlot := area.PlotAt(e.From.Pos.X, e.From.Pos.Z)
		toPlot := area.PlotAt(e.To.Pos.X, e.To.Pos.Z)
		// Never allow liquid or dragon egg to cross into a different plot or onto a road
		if toPlot == nil || !toPlot.HasOwner() || fromPlot != toPlot {
			e.SetCancelled(true)
			return
		}
		if toPlot.FlagString("liquid-flow") == "disabled" {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.BlockPistonEvent) {
		area := ps.PlotAreaAtBlock(e.Piston.World, e.Piston.Pos)
		if area == nil {
			return
		}
		srcPlot := area.PlotAt(e.Piston.Pos.X, e.Piston.Pos.Z)
		if srcPlot == nil || !srcPlot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		moveDir := e.Direction
		if !e.Extending {
			moveDir = moveDir.Opposite()
		}
		// Check piston head destination
		headPos := e.Piston.Pos.Offset(e.Direction, 1)
		if area.PlotAt(headPos.X, headPos.Z) != srcPlot {
			e.SetCancelled(true)
			return
		}
		for _, blk := range e.Blocks {
			if area.PlotAt(blk.Pos.X, blk.Pos.Z) != srcPlot {
				e.SetCancelled(true)
				return
			}
			dest := blk.Pos.Offset(moveDir, 1)
			if area.PlotAt(dest.X, dest.Z) != srcPlot {
				e.SetCancelled(true)
				return
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.BlockDispenseEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		srcPlot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if srcPlot == nil || !srcPlot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		// Check target block in front of dispenser
		if facing, ok := e.Block.State().Prop(registry.PropFacing); ok {
			var face plugin.BlockFace
			switch facing {
			case "north":
				face = plugin.North
			case "south":
				face = plugin.South
			case "east":
				face = plugin.East
			case "west":
				face = plugin.West
			case "up":
				face = plugin.Up
			case "down":
				face = plugin.Down
			}
			front := e.Block.Pos.Offset(face, 1)
			if area.PlotAt(front.X, front.Z) != srcPlot {
				e.SetCancelled(true)
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.BlockExplodeEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		srcPlot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if srcPlot == nil || !srcPlot.HasOwner() || !srcPlot.FlagBool("explosion") {
			e.SetCancelled(true)
			return
		}
		e.Blocks = slices.DeleteFunc(e.Blocks, func(b plugin.Block) bool {
			return area.PlotAt(b.Pos.X, b.Pos.Z) != srcPlot
		})
	}, opts)

	plugin.Listen(s, func(e *plugin.SpongeAbsorbEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		srcPlot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if srcPlot == nil || !srcPlot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		e.Blocks = slices.DeleteFunc(e.Blocks, func(pos plugin.BlockPos) bool {
			return area.PlotAt(pos.X, pos.Z) != srcPlot
		})
	}, opts)

	plugin.Listen(s, func(e *plugin.TNTPrimeEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil || !plot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		if !plot.FlagBool("explosion") {
			e.SetCancelled(true)
			return
		}
		if p, ok := e.Entity.(plugin.Player); ok && !ps.isAddedAndAllowed(p, plot) && !HasPerm(p, "plots.admin.build.other") {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.PortalCreateEvent) {
		area := ps.PlotAreaAtBlock(e.World, firstPortalPos(e.Blocks))
		if area == nil {
			return
		}
		for _, bc := range e.Blocks {
			plot := area.PlotAt(bc.Pos.X, bc.Pos.Z)
			if plot == nil || !plot.HasOwner() || plot.FlagBool("deny-portals") {
				e.SetCancelled(true)
				return
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.CauldronLevelChangeEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if p, ok := e.Entity.(plugin.Player); ok {
			if plot == nil || !plot.HasOwner() {
				if !HasPerm(p, "plots.admin.interact.road") {
					e.SetCancelled(true)
				}
				return
			}
			if !ps.isAddedAndAllowed(p, plot) && !matchesBlockListFlag(plot, "use", e.Block.State()) && !HasPerm(p, "plots.admin.interact.other") {
				e.SetCancelled(true)
			}
		}
	}, opts)

	// High-frequency listeners: disable-physics and redstone
	if ps.Settings == nil || ps.Settings.HighFrequencyListener {
		plugin.Listen(s, func(e *plugin.BlockPhysicsEvent) {
			area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
			if area == nil {
				return
			}
			plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
			if plot != nil && plot.HasOwner() && plot.FlagBool("disable-physics") {
				e.SetCancelled(true)
			}
		}, opts)

		plugin.Listen(s, func(e *plugin.BlockRedstoneEvent) {
			if e.New == 0 {
				return
			}
			area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
			if area == nil {
				return
			}
			plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
			if plot == nil || !plot.HasOwner() {
				e.New = 0
				return
			}
			if !plot.FlagBool("redstone") {
				e.New = 0
				return
			}
			if ps.Settings != nil && ps.Settings.Redstone.DisableOffline {
				if !ps.anyPlotMemberOnline(plot) {
					e.New = 0
					return
				}
			}
			if ps.Settings != nil && ps.Settings.Redstone.DisableUnoccupied {
				if !ps.anyPlayerInPlot(plot) {
					e.New = 0
					return
				}
			}
		})
	}
}

func firstPortalPos(blocks []plugin.BlockChange) plugin.BlockPos {
	if len(blocks) == 0 {
		return plugin.BlockPos{}
	}
	return blocks[0].Pos
}

func (ps *PlotSquared) anyPlotMemberOnline(plot *plotsquared.Plot) bool {
	for _, owner := range plot.Owners() {
		if _, ok := ps.Server.Player(owner); ok {
			return true
		}
	}
	base := plot.BasePlot()
	for u := range base.Trusted {
		if _, ok := ps.Server.Player(u); ok {
			return true
		}
	}
	for u := range base.Members {
		if _, ok := ps.Server.Player(u); ok {
			return true
		}
	}
	return false
}

func (ps *PlotSquared) anyPlayerInPlot(plot *plotsquared.Plot) bool {
	if plot == nil || plot.Area == nil {
		return false
	}
	w, ok := ps.WrapWorld(plot.Area.WorldName)
	if !ok {
		return false
	}
	base := plot.BasePlot()
	for _, p := range w.Players() {
		bp := p.Pos().Block()
		if cur := plot.Area.PlotAt(bp.X, bp.Z); cur != nil && cur.BasePlot() == base {
			return true
		}
	}
	return false
}
