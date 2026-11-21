package plotsquaredplugin

import (
	"math"
	"slices"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plotsquared"
	"github.com/golem-mc/plotsquared/event"
)

func (ps *PlotSquared) registerEntityListeners() {
	ps.registerEntityCombatAndSpawnListeners()
	ps.registerEntityWorldListeners()
}

func (ps *PlotSquared) registerEntityCombatAndSpawnListeners() {
	s := ps.Server
	opts := plugin.ListenOptions{Priority: plugin.High, IgnoreCancelled: true}

	// Damage (invincible, pvp, pve, animal-attack, hostile-attack, tamed-attack, misc-break, hanging-break, vehicle-break)
	plugin.Listen(s, func(e *plugin.EntityDamageEvent) {
		loc := e.Entity.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)

		// Invincible flag for players
		if _, isPlayer := e.Entity.(plugin.Player); isPlayer {
			if plot != nil && plot.HasOwner() && plot.FlagBool("invincible") {
				e.SetCancelled(true)
				return
			}
			if plot == nil && ps.roadFlagBool(area, "invincible") {
				e.SetCancelled(true)
				return
			}
		}

		// Attribute causing attacker (including projectile shooters)
		attacker := resolveAttacker(e.Source.Causing, e.Source.Direct)
		if attacker == nil {
			return
		}
		if p, ok := attacker.(plugin.Player); ok {
			if !ps.allowEntityDamageByPlayer(p, e.Entity) {
				e.SetCancelled(true)
			}
			return
		}
		// Non-player entity damaging another entity (e.g. explosion or mob on protected plot)
		if _, isPlayer := e.Entity.(plugin.Player); !isPlayer {
			if plot == nil || !plot.HasOwner() || !plot.FlagBool("pve") {
				if isExplosiveEntity(attacker.Type()) && (plot == nil || !plot.FlagBool("explosion")) {
					e.SetCancelled(true)
				}
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.EntityCombustEvent) {
		if e.ByEntity == nil {
			return
		}
		attacker := resolveAttacker(e.ByEntity, e.ByEntity)
		if p, ok := attacker.(plugin.Player); ok {
			if !ps.allowEntityDamageByPlayer(p, e.Entity) {
				e.SetCancelled(true)
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.EntityKnockbackEvent) {
		if e.Pusher == nil {
			return
		}
		attacker := resolveAttacker(e.Pusher, e.Pusher)
		if p, ok := attacker.(plugin.Player); ok {
			if !ps.allowEntityDamageByPlayer(p, e.Entity) {
				e.SetCancelled(true)
			}
		}
	}, opts)

	// Entity explosions
	plugin.Listen(s, func(e *plugin.EntityExplodeEvent) {
		area := ps.PlotAreaAt(e.Location)
		if area == nil {
			return
		}
		srcPlot := area.PlotAt(e.Location.Pos.Block().X, e.Location.Pos.Block().Z)
		if srcPlot == nil || !srcPlot.HasOwner() || !srcPlot.FlagBool("explosion") {
			e.SetCancelled(true)
			return
		}
		e.Blocks = slices.DeleteFunc(e.Blocks, func(b plugin.Block) bool {
			return area.PlotAt(b.Pos.X, b.Pos.Z) != srcPlot
		})
	}, opts)

	// Entity block changes (endermen, sheep, falling blocks, ravagers, wither, weaving cobweb)
	plugin.Listen(s, func(e *plugin.EntityChangeBlockEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)

		if e.Entity.Type() == registry.EntityTypeFallingBlock {
			if plot == nil || !plot.HasOwner() || plot.FlagBool("disable-physics") {
				e.SetCancelled(true)
				return
			}
			if orig, ok := e.Entity.Origin(); ok {
				origPlot := area.PlotAt(orig.Pos.Block().X, orig.Pos.Block().Z)
				if origPlot != plot {
					e.SetCancelled(true)
					e.Entity.Remove()
				}
			}
			return
		}

		if e.NewState.Block() == registry.BlockCobweb {
			if plot == nil || !plot.HasOwner() || !plot.FlagBool("weaving-death-place") {
				e.SetCancelled(true)
			}
			return
		}

		if plot == nil || !plot.HasOwner() {
			e.SetCancelled(true)
			return
		}
		if !plot.FlagBool("entity-change-block") && !plot.FlagBool("mob-break") {
			e.SetCancelled(true)
		}
	}, opts)

	// Entity spawns & per-plot entity caps
	plugin.Listen(s, func(e *plugin.EntitySpawnEvent) {
		if e.Entity.Type() == registry.EntityTypeArmorStand {
			return
		}
		loc := e.Entity.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		switch e.Reason {
		case plugin.SpawnEgg, plugin.SpawnSpawnerEgg, plugin.SpawnDispenser:
			if !area.SpawnEggs {
				e.SetCancelled(true)
				return
			}
		case plugin.SpawnNatural, plugin.SpawnChunkGeneration, plugin.SpawnPatrol, plugin.SpawnRaid, plugin.SpawnReinforcements, plugin.SpawnVillageDefense, plugin.SpawnJockey:
			if !area.MobSpawning {
				e.SetCancelled(true)
				return
			}
		case plugin.SpawnBreeding:
			if !area.SpawnBreeding {
				e.SetCancelled(true)
				return
			}
		case plugin.SpawnSpawner, plugin.SpawnTrialSpawner:
			if !area.MobSpawnerSpawning {
				e.SetCancelled(true)
				return
			}
		case plugin.SpawnPlugin, plugin.SpawnCommand:
			if !area.SpawnCustom {
				e.SetCancelled(true)
				return
			}
			return
		case plugin.SpawnBuildIronGolem, plugin.SpawnBuildSnowGolem, plugin.SpawnBuildWither:
			if !area.SpawnCustom {
				e.SetCancelled(true)
				return
			}
		}

		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		if plot == nil || !plot.HasOwner() {
			if _, isMob := e.Entity.(plugin.Mob); isMob && !area.MobSpawning {
				e.SetCancelled(true)
				return
			}
			if _, isItem := e.Entity.(plugin.ItemEntity); isItem && ps.Settings != nil && ps.Settings.EnabledComponents.KillRoadItems {
				e.SetCancelled(true)
				return
			}
			if !area.MiscSpawnUnowned && !area.MobSpawning {
				if _, isVeh := e.Entity.(plugin.Vehicle); isVeh {
					e.SetCancelled(true)
					return
				}
			}
			return
		}

		if ps.exceedsEntityCap(loc.World, area, plot.BasePlot(), e.Entity) {
			e.SetCancelled(true)
		}
	}, opts)
}

func (ps *PlotSquared) registerEntityWorldListeners() {
	s := ps.Server
	opts := plugin.ListenOptions{Priority: plugin.High, IgnoreCancelled: true}

	// Hanging place/break (paintings, item frames, leash knots)
	plugin.Listen(s, func(e *plugin.HangingPlaceEvent) {
		area := ps.PlotAreaAtBlock(e.Block.World, e.Block.Pos)
		if area == nil {
			return
		}
		plot := area.PlotAt(e.Block.Pos.X, e.Block.Pos.Z)
		if plot == nil || !plot.HasOwner() {
			if e.Player == nil || !HasPerm(e.Player, "plots.admin.build.road") {
				e.SetCancelled(true)
			}
			return
		}
		if e.Player != nil && !ps.isAddedAndAllowed(e.Player, plot) && !plot.FlagBool("hanging-place") && !HasPerm(e.Player, "plots.admin.build.other") {
			ps.sendNoPerm(e.Player, "plots.admin.build.other")
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.HangingBreakEvent) {
		loc := e.Entity.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		if e.Cause == plugin.HangingBreakExplosion {
			if plot == nil || !plot.HasOwner() || !plot.FlagBool("explosion") {
				e.SetCancelled(true)
			}
			return
		}
		attacker := resolveAttacker(e.Remover, e.Remover)
		if p, ok := attacker.(plugin.Player); ok {
			if plot == nil || !plot.HasOwner() {
				if !HasPerm(p, "plots.admin.destroy.road") {
					e.SetCancelled(true)
				}
				return
			}
			if !ps.isAddedAndAllowed(p, plot) && !plot.FlagBool("hanging-break") && !HasPerm(p, "plots.admin.destroy.other") {
				ps.sendNoPerm(p, "plots.admin.destroy.other")
				e.SetCancelled(true)
			}
		}
	}, opts)

	// Vehicles (enter/damage/destroy)
	plugin.Listen(s, func(e *plugin.VehicleEvent) {
		loc := e.Vehicle.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		p, isPlayer := e.Entity.(plugin.Player)
		switch e.Action {
		case plugin.VehicleEnter:
			if isPlayer && plot != nil && plot.HasOwner() {
				if !ps.isAddedAndAllowed(p, plot) && !plot.FlagBool("vehicle-use") && !HasPerm(p, "plots.admin.interact.other") {
					e.SetCancelled(true)
				}
			}
		case plugin.VehicleDamage, plugin.VehicleDestroy:
			if !isPlayer {
				return
			}
			if plot == nil || !plot.HasOwner() {
				if !HasPerm(p, "plots.admin.vehicle.break.unowned") && !HasPerm(p, "plots.admin.destroy.road") {
					e.SetCancelled(true)
				}
				return
			}
			if !ps.isAddedAndAllowed(p, plot) && !plot.FlagBool("vehicle-break") && !HasPerm(p, "plots.admin.destroy.other") {
				ps.sendNoPerm(p, "plots.admin.destroy.other")
				e.SetCancelled(true)
			}
		}
	}, opts)

	// Projectiles & wind charges
	plugin.Listen(s, func(e *plugin.ProjectileLaunchEvent) {
		loc := e.Projectile.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		shooter, ok := e.Projectile.Shooter()
		if !ok {
			return
		}
		p, ok := shooter.(plugin.Player)
		if !ok {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		if plot == nil || !plot.HasOwner() {
			if !HasPerm(p, "plots.projectile.unowned") && !HasPerm(p, "plots.admin.build.road") {
				e.SetCancelled(true)
			}
			return
		}
		if ps.isAddedAndAllowed(p, plot) || HasPerm(p, "plots.projectile.other") {
			return
		}
		if e.Projectile.Type() == registry.EntityTypeWindCharge || e.Projectile.Type() == registry.EntityTypeBreezeWindCharge {
			if !plot.FlagBool("wind-charge") {
				e.SetCancelled(true)
			}
			return
		}
		if !plot.FlagBool("projectiles") {
			e.SetCancelled(true)
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.ProjectileHitEvent) {
		if e.Hit.Entity != nil {
			if shooter, ok := e.Projectile.Shooter(); ok {
				if p, ok := shooter.(plugin.Player); ok {
					if !ps.allowEntityDamageByPlayer(p, e.Hit.Entity) {
						e.SetCancelled(true)
					}
				}
			}
		}
	}, opts)

	// Splash & lingering potions
	plugin.Listen(s, func(e *plugin.PotionSplashEvent) {
		shooter, hasShooter := e.Potion.Shooter()
		p, isPlayer := shooter.(plugin.Player)
		for victim := range e.Affected {
			loc := victim.Location()
			area := ps.PlotAreaAt(loc)
			if area == nil {
				continue
			}
			if hasShooter && isPlayer && !ps.allowEntityDamageByPlayer(p, victim) {
				e.Affected[victim] = 0
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.AreaEffectCloudApplyEvent) {
		p, isPlayer := e.Source.(plugin.Player)
		if !isPlayer {
			return
		}
		e.Affected = slices.DeleteFunc(e.Affected, func(victim plugin.LivingEntity) bool {
			return !ps.allowEntityDamageByPlayer(p, victim)
		})
	}, opts)

	// Beacon effects & cross-plot overflow prevention
	plugin.Listen(s, func(e *plugin.EntityPotionEffectEvent) {
		if e.Cause != plugin.EffectBeacon || e.New == nil {
			return
		}
		loc := e.Entity.Location()
		area := ps.PlotAreaAt(loc)
		if area == nil {
			return
		}
		plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)
		if plot != nil && plot.HasOwner() && !plot.FlagBool("beacon-effects") {
			e.SetCancelled(true)
		}
	}, opts)

	// Entity & vehicle movement: keep entities inside plots / kill road mobs & vehicles
	plugin.Listen(s, func(e *plugin.EntityMoveEvent) {
		area := ps.PlotAreaAt(e.To)
		if area == nil {
			return
		}
		fromPlot := area.PlotAt(e.From.Pos.Block().X, e.From.Pos.Block().Z)
		toPlot := area.PlotAt(e.To.Pos.Block().X, e.To.Pos.Block().Z)
		if fromPlot == toPlot {
			return
		}
		if ps.Settings != nil && ps.Settings.PaperComponents.EntityMovement {
			e.SetCancelled(true)
			return
		}
		if toPlot == nil && ps.Settings != nil && ps.Settings.EnabledComponents.KillRoadMobs {
			if !ps.Settings.EnabledComponents.KillNamedRoadMobs && e.Entity.CustomName() != nil {
				return
			}
			if !ps.Settings.EnabledComponents.KillOwnedRoadMobs && len(e.Entity.Passengers()) > 0 {
				return
			}
			ev := &event.RemoveRoadEntityEvent{Entity: e.Entity}
			s.Events().Fire(ev)
			if !ev.Cancelled() {
				e.Entity.Remove()
			}
		}
	}, opts)

	plugin.Listen(s, func(e *plugin.VehicleMoveEvent) {
		if ps.Settings == nil || !ps.Settings.EnabledComponents.KillRoadVehicles {
			return
		}
		area := ps.PlotAreaAt(e.To)
		if area == nil {
			return
		}
		if area.PlotAt(e.To.Pos.Block().X, e.To.Pos.Block().Z) == nil {
			ev := &event.RemoveRoadEntityEvent{Entity: e.Vehicle}
			s.Events().Fire(ev)
			if !ev.Cancelled() {
				e.Vehicle.Remove()
			}
		}
	})
}

func (ps *PlotSquared) allowEntityDamageByPlayer(attacker plugin.Player, victim plugin.Entity) bool {
	if attacker == nil || victim == nil {
		return true
	}
	loc := victim.Location()
	area := ps.PlotAreaAt(loc)
	if area == nil {
		return true
	}
	plot := area.PlotAt(loc.Pos.Block().X, loc.Pos.Block().Z)

	// Player vs Player (PvP)
	if _, isVictimPlayer := victim.(plugin.Player); isVictimPlayer {
		if victim == attacker {
			return true
		}
		if plot == nil || !plot.HasOwner() {
			return ps.roadFlagBool(area, "pvp")
		}
		return plot.FlagBool("pvp")
	}

	// Player vs non-player entity
	if plot == nil || !plot.HasOwner() {
		if HasPerm(attacker, "plots.admin.pve.unowned") || HasPerm(attacker, "plots.admin.destroy.road") {
			return true
		}
		return ps.roadFlagBool(area, "pve")
	}
	if ps.isAddedAndAllowed(attacker, plot) || HasPerm(attacker, "plots.admin.destroy.other") {
		return true
	}
	if plot.FlagBool("pve") {
		return true
	}
	switch victim.(type) {
	case plugin.Vehicle:
		return plot.FlagBool("vehicle-break")
	case plugin.Hanging:
		return plot.FlagBool("hanging-break")
	case plugin.ArmorStand:
		return plot.FlagBool("misc-break")
	default:
		if _, ok := victim.(plugin.Tameable); ok && plot.FlagBool("tamed-attack") {
			return true
		}
		switch victim.Type().Category() {
		case registry.MobCategoryMonster:
			return plot.FlagBool("hostile-attack")
		case registry.MobCategoryCreature, registry.MobCategoryWaterCreature, registry.MobCategoryAxolotls, registry.MobCategoryWaterAmbient, registry.MobCategoryAmbient:
			return plot.FlagBool("animal-attack")
		default:
			return plot.FlagBool("misc-break")
		}
	}
}

func resolveAttacker(causing, direct plugin.Entity) plugin.Entity {
	if causing != nil {
		if proj, ok := causing.(plugin.Projectile); ok {
			if shooter, ok := proj.Shooter(); ok {
				return shooter
			}
		}
		return causing
	}
	if direct != nil {
		if proj, ok := direct.(plugin.Projectile); ok {
			if shooter, ok := proj.Shooter(); ok {
				return shooter
			}
		}
		return direct
	}
	return nil
}

func isExplosiveEntity(t registry.EntityType) bool {
	return t == registry.EntityTypeTnt ||
		t == registry.EntityTypeTntMinecart ||
		t == registry.EntityTypeCreeper ||
		t == registry.EntityTypeEndCrystal ||
		t == registry.EntityTypeFireball ||
		t == registry.EntityTypeWither ||
		t == registry.EntityTypeWitherSkull
}

func (ps *PlotSquared) exceedsEntityCap(w plugin.World, area *plotsquared.PlotArea, base *plotsquared.Plot, spawning plugin.Entity) bool {
	entCap := base.FlagInt("entity-cap")
	mobCap := base.FlagInt("mob-cap")
	animalCap := base.FlagInt("animal-cap")
	hostileCap := base.FlagInt("hostile-cap")
	vehCap := base.FlagInt("vehicle-cap")
	miscCap := base.FlagInt("misc-cap")

	if entCap == math.MaxInt32 && mobCap == math.MaxInt32 && animalCap == math.MaxInt32 &&
		hostileCap == math.MaxInt32 && vehCap == math.MaxInt32 && miscCap == math.MaxInt32 {
		return false
	}

	counts := ps.CountPlotEntities(w, area, base)
	if counts.Total >= entCap {
		return true
	}
	switch spawning.(type) {
	case plugin.Vehicle:
		return counts.Vehicles >= vehCap
	case plugin.Mob:
		if counts.Mobs >= mobCap {
			return true
		}
		if spawning.Type().Category() == registry.MobCategoryMonster {
			return counts.Hostiles >= hostileCap
		}
		return counts.Animals >= animalCap
	default:
		return counts.Misc >= miscCap
	}
}

// PlotEntityCounts holds categorized entity counts inside a plot (for caps and /plot caps).
type PlotEntityCounts struct {
	Total    int
	Mobs     int
	Animals  int
	Hostiles int
	Vehicles int
	Misc     int
}

// CountPlotEntities counts all non-player entities currently inside base plot.
func (ps *PlotSquared) CountPlotEntities(w plugin.World, area *plotsquared.PlotArea, base *plotsquared.Plot) PlotEntityCounts {
	var c PlotEntityCounts
	if w == nil || area == nil || base == nil {
		return c
	}
	seen := make(map[int32]bool)
	for _, r := range area.PlotRegions(base) {
		box := plugin.Box{
			Min: plugin.Vec3{X: float64(r.Min.X), Y: float64(w.MinY()), Z: float64(r.Min.Z)},
			Max: plugin.Vec3{X: float64(r.Max.X + 1), Y: float64(w.MinY() + w.Height()), Z: float64(r.Max.Z + 1)},
		}
		for _, ent := range w.EntitiesIn(box) {
			if _, isPlayer := ent.(plugin.Player); isPlayer {
				continue
			}
			if seen[ent.ID()] {
				continue
			}
			seen[ent.ID()] = true
			c.Total++
			switch ent.(type) {
			case plugin.Vehicle:
				c.Vehicles++
			case plugin.Mob:
				c.Mobs++
				if ent.Type().Category() == registry.MobCategoryMonster {
					c.Hostiles++
				} else {
					c.Animals++
				}
			default:
				c.Misc++
			}
		}
	}
	return c
}
