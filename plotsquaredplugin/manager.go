package plotsquaredplugin

import (
	"fmt"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plotsquared"
	"github.com/golem-mc/plotsquared/event"
	"github.com/golem-mc/plotsquared/internal/config"
	"github.com/golem-mc/plotsquared/internal/generator"
	"github.com/google/uuid"
)

// ClaimPlot claims an unclaimed plot for player (firing PlayerClaimPlotEvent,
// persisting to SQLite, updating claimed wall blocks and claim sign, and optionally auto-merging).
func (ps *PlotSquared) ClaimPlot(area *plotsquared.PlotArea, id plotsquared.PlotId, owner uuid.UUID, actor plugin.Player, auto bool) (*plotsquared.Plot, error) {
	if area == nil {
		return nil, fmt.Errorf("not in a plot area")
	}
	if !area.ContainsPlotId(id) {
		return nil, fmt.Errorf("plot %s is outside area bounds", id)
	}
	if existing := area.PlotAbs(id); existing != nil && existing.HasOwner() {
		return nil, fmt.Errorf("plot %s is already claimed", id)
	}
	p := plotsquared.NewPlot(area, id, owner)
	p.Timestamp = time.Now().UnixMilli()

	ev := &event.PlayerClaimPlotEvent{Player: actor, Plot: p, Auto: auto}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		return nil, fmt.Errorf("claim cancelled by plugin")
	}

	area.SetPlot(p)
	if ps.DB != nil {
		if err := ps.DB.CreatePlot(p); err != nil {
			area.RemovePlot(id)
			return nil, fmt.Errorf("save plot: %w", err)
		}
	}

	if w, ok := ps.WrapWorld(area.WorldName); ok {
		ownerName := ps.PlayerName(owner)
		generator.ClaimPlotBlocks(w, area, p, ownerName)
	}

	if area.AutoMerge {
		_, _ = ps.MergePlot(p, plotsquared.DirAll, 16, actor, true)
	}
	return p, nil
}

// AutoClaim finds the next free plot (or NxM block) in area and claims it.
func (ps *PlotSquared) AutoClaim(area *plotsquared.PlotArea, owner uuid.UUID, actor plugin.Player, sizeX, sizeZ int) (*plotsquared.Plot, error) {
	if area == nil {
		return nil, fmt.Errorf("not in a plot area")
	}
	if sizeX <= 1 && sizeZ <= 1 {
		id, ok := area.NextFreePlotId()
		if !ok {
			return nil, fmt.Errorf("no free plots available in %s", area.FullID())
		}
		return ps.ClaimPlot(area, id, owner, actor, true)
	}
	start, ok := area.NextFreePlotBlock(sizeX, sizeZ)
	if !ok {
		return nil, fmt.Errorf("no free %dx%d plot block available in %s", sizeX, sizeZ, area.FullID())
	}
	var claimed []*plotsquared.Plot
	for dx := range sizeX {
		for dz := range sizeZ {
			p, err := ps.ClaimPlot(area, plotsquared.PlotId{X: start.X + dx, Y: start.Y + dz}, owner, actor, true)
			if err != nil {
				return nil, err
			}
			claimed = append(claimed, p)
		}
	}
	// Merge all plots in the NxM block
	if len(claimed) > 1 {
		_, _ = ps.mergeGroup(area, claimed, actor, true)
	}
	return claimed[0].BasePlot(), nil
}

// ClearPlot clears a plot back to default terrain, firing PlotClearEvent.
func (ps *PlotSquared) ClearPlot(plot *plotsquared.Plot) error {
	if plot == nil || plot.Area == nil {
		return fmt.Errorf("invalid plot")
	}
	base := plot.BasePlot()
	ev := &event.PlotClearEvent{Plot: base}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		return fmt.Errorf("clear cancelled")
	}
	if w, ok := ps.WrapWorld(base.Area.WorldName); ok {
		gen := ps.generatorForArea(base.Area)
		generator.ClearPlot(w, base.Area, base, gen)
	}
	return nil
}

// DeletePlot unlinks (if merged), clears, removes from DB and memory, and restores unclaimed walls.
func (ps *PlotSquared) DeletePlot(plot *plotsquared.Plot, clearBlocks bool) error {
	if plot == nil || plot.Area == nil {
		return fmt.Errorf("invalid plot")
	}
	base := plot.BasePlot()
	ev := &event.PlotDeleteEvent{Plot: base}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		return fmt.Errorf("delete cancelled")
	}
	if base.IsMerged() {
		if err := ps.UnlinkPlot(base, true, "delete"); err != nil {
			return err
		}
	}
	connected := base.ConnectedPlots()
	area := base.Area
	w, hasWorld := ps.WrapWorld(area.WorldName)
	gen := ps.generatorForArea(area)

	for _, cp := range connected {
		if clearBlocks && hasWorld {
			generator.ClearPlot(w, area, cp, gen)
		}
		if hasWorld {
			generator.UnclaimPlotBlocks(w, area, cp)
		}
		if ps.DB != nil {
			if err := ps.DB.DeletePlot(cp); err != nil {
				return err
			}
		}
		area.RemovePlot(cp.ID)
	}
	return nil
}

// MergePlot merges plot with adjacent same-owner (or allowed) plot(s) in dir (or DirAll).
// Returns the number of merges performed.
func (ps *PlotSquared) MergePlot(plot *plotsquared.Plot, dir plotsquared.Direction, maxSize int, actor plugin.Player, removeRoads bool) (int, error) {
	if plot == nil || plot.Area == nil || !plot.HasOwner() {
		return 0, fmt.Errorf("plot is not owned")
	}
	base := plot.BasePlot()
	area := base.Area

	ev := &event.PlotMergeEvent{Plot: base, Direction: dir, Max: maxSize, Player: actor}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		return 0, fmt.Errorf("merge cancelled")
	}

	dirs := []plotsquared.Direction{dir}
	if dir == plotsquared.DirAll {
		dirs = plotsquared.CardinalDirections[:]
	}

	mergedCount := 0
	for {
		curGroup := base.ConnectedPlots()
		if maxSize > 0 && len(curGroup) >= maxSize {
			break
		}
		// Find all candidate neighbours in the requested direction(s).
		groupMap := make(map[plotsquared.PlotId]*plotsquared.Plot, len(curGroup))
		for _, cp := range curGroup {
			groupMap[cp.ID] = cp
		}
		var additions []*plotsquared.Plot
		seenAdd := make(map[plotsquared.PlotId]bool)
		for _, cp := range curGroup {
			for _, d := range dirs {
				if d < 0 || d > 3 || cp.IsMergedDir(d) {
					continue
				}
				nid := cp.ID.Relative(d)
				if groupMap[nid] != nil || seenAdd[nid] {
					continue
				}
				nb := area.PlotAbs(nid)
				if nb == nil || !nb.HasOwner() {
					continue
				}
				if nb.Owner != base.Owner && (actor == nil || !HasPerm(actor, "plots.admin.command.merge")) {
					continue
				}
				for _, ncp := range nb.ConnectedPlots() {
					if !seenAdd[ncp.ID] && groupMap[ncp.ID] == nil {
						seenAdd[ncp.ID] = true
						additions = append(additions, ncp)
					}
				}
			}
		}
		if len(additions) == 0 {
			break
		}
		if maxSize > 0 && len(curGroup)+len(additions) > maxSize {
			break
		}
		combined := append(curGroup, additions...)
		// Enforce rectangular bounding box closure (matching upstream Plot.mergePlot):
		// ensure all plots inside the bounding box of combined exist and are compatible.
		minX, maxX := combined[0].ID.X, combined[0].ID.X
		minY, maxY := combined[0].ID.Y, combined[0].ID.Y
		for _, cp := range combined {
			minX = min(minX, cp.ID.X)
			maxX = max(maxX, cp.ID.X)
			minY = min(minY, cp.ID.Y)
			maxY = max(maxY, cp.ID.Y)
		}
		var rect []*plotsquared.Plot
		rectOK := true
		for x := minX; x <= maxX && rectOK; x++ {
			for y := minY; y <= maxY; y++ {
				rp := area.PlotAbs(plotsquared.PlotId{X: x, Y: y})
				if rp == nil || !rp.HasOwner() || (rp.Owner != base.Owner && (actor == nil || !HasPerm(actor, "plots.admin.command.merge"))) {
					rectOK = false
					break
				}
				rect = append(rect, rp)
			}
		}
		if !rectOK || (maxSize > 0 && len(rect) > maxSize) {
			break
		}
		n, err := ps.mergeGroup(area, rect, actor, removeRoads)
		if err != nil || n == 0 {
			return mergedCount, err
		}
		mergedCount += n
		base = rect[0].BasePlot()
		if dir != plotsquared.DirAll {
			break
		}
	}
	return mergedCount, nil
}

func (ps *PlotSquared) mergeGroup(area *plotsquared.PlotArea, group []*plotsquared.Plot, _ plugin.Player, removeRoads bool) (int, error) {
	byID := make(map[plotsquared.PlotId]*plotsquared.Plot, len(group))
	for _, p := range group {
		byID[p.ID] = p
	}
	w, hasWorld := ps.WrapWorld(area.WorldName)
	changedEdges := 0

	// Merge shared data onto the canonical base plot.
	base := group[0]
	for _, p := range group {
		if p.ID.X < base.ID.X || (p.ID.X == base.ID.X && p.ID.Y < base.ID.Y) {
			base = p
		}
	}
	for _, p := range group {
		if p == base {
			continue
		}
		for u := range p.Trusted {
			base.Trusted[u] = struct{}{}
		}
		for u := range p.Members {
			base.Members[u] = struct{}{}
		}
		for u := range p.Denied {
			base.Denied[u] = struct{}{}
		}
	}

	// Update edge flags and remove intervening roads/intersections.
	for _, p := range group {
		dirty := false
		for _, d := range plotsquared.CardinalDirections {
			nid := p.ID.Relative(d)
			if _, ok := byID[nid]; ok && !p.Settings.Merged[d] {
				p.Settings.Merged[d] = true
				dirty = true
				changedEdges++
				if removeRoads && hasWorld {
					switch d {
					case plotsquared.DirEast:
						generator.RemoveRoadEast(w, area, p.ID)
					case plotsquared.DirSouth:
						generator.RemoveRoadSouth(w, area, p.ID)
					}
				}
			}
		}
		// Remove 2x2 southeast intersection if all 4 cells are in group.
		if removeRoads && hasWorld && p.Settings.Merged[plotsquared.DirEast] && p.Settings.Merged[plotsquared.DirSouth] {
			if _, ok := byID[plotsquared.PlotId{X: p.ID.X + 1, Y: p.ID.Y + 1}]; ok {
				generator.RemoveRoadSouthEast(w, area, p.ID)
			}
		}
		if dirty && ps.DB != nil {
			if err := ps.DB.SetMerged(p); err != nil {
				return changedEdges, err
			}
		}
	}

	if hasWorld && changedEdges > 0 && area.RoadWidth > 0 {
		if area.PlaceTopBlock {
			generator.SetComponent(w, area, base, plotsquared.ComponentBorder, area.ClaimedWallBlock)
		}
		if ps.Settings == nil || ps.Settings.General.MergeReplaceWall {
			generator.SetComponent(w, area, base, plotsquared.ComponentWall, area.WallFilling)
		}
	}
	return changedEdges, nil
}

// UnlinkPlot separates a merged plot group back into individual plots, optionally restoring roads.
func (ps *PlotSquared) UnlinkPlot(plot *plotsquared.Plot, createRoads bool, reason string) error {
	if plot == nil || plot.Area == nil || !plot.IsMerged() {
		return fmt.Errorf("plot is not merged")
	}
	base := plot.BasePlot()
	area := base.Area

	ev := &event.PlotUnlinkEvent{Area: area, Plot: base, Reason: reason}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		return fmt.Errorf("unlink cancelled")
	}

	connected := base.ConnectedPlots()
	w, hasWorld := ps.WrapWorld(area.WorldName)
	gen := ps.generatorForArea(area)

	// Record which roads/intersections existed before clearing merged bits.
	type edgeRec struct {
		id     plotsquared.PlotId
		east   bool
		south  bool
		seQuad bool
	}
	idSet := make(map[plotsquared.PlotId]*plotsquared.Plot, len(connected))
	for _, cp := range connected {
		idSet[cp.ID] = cp
	}
	var recs []edgeRec
	for _, cp := range connected {
		seQuad := cp.IsMergedDir(plotsquared.DirEast) && cp.IsMergedDir(plotsquared.DirSouth) &&
			idSet[plotsquared.PlotId{X: cp.ID.X + 1, Y: cp.ID.Y + 1}] != nil
		recs = append(recs, edgeRec{
			id:     cp.ID,
			east:   cp.IsMergedDir(plotsquared.DirEast),
			south:  cp.IsMergedDir(plotsquared.DirSouth),
			seQuad: seQuad,
		})
		cp.Settings.Merged = [4]bool{}
		if ps.DB != nil {
			if err := ps.DB.SetMerged(cp); err != nil {
				return err
			}
		}
	}

	if createRoads && hasWorld && area.RoadWidth > 0 {
		for _, r := range recs {
			if r.east {
				generator.CreateRoadEast(w, area, r.id, gen)
			}
			if r.south {
				generator.CreateRoadSouth(w, area, r.id, gen)
			}
			if r.seQuad {
				generator.CreateRoadSouthEast(w, area, r.id, gen)
			}
		}
		for _, cp := range connected {
			if area.PlaceTopBlock {
				generator.SetComponent(w, area, cp, plotsquared.ComponentBorder, area.ClaimedWallBlock)
			}
		}
	}
	return nil
}

// SetOwner changes the owner of a plot (and all its connected merged cells).
func (ps *PlotSquared) SetOwner(plot *plotsquared.Plot, newOwner uuid.UUID, initiator plugin.CommandSender) error {
	if plot == nil || plot.Area == nil {
		return fmt.Errorf("invalid plot")
	}
	base := plot.BasePlot()
	ev := &event.PlotChangeOwnerEvent{
		Initiator: initiator,
		Plot:      base,
		OldOwner:  base.Owner,
		NewOwner:  newOwner,
		HasOwner:  base.HasOwner(),
	}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		return fmt.Errorf("owner change cancelled")
	}
	for _, cp := range base.ConnectedPlots() {
		wasUnowned := !cp.HasOwner()
		cp.Owner = newOwner
		if wasUnowned {
			cp.Area.SetPlot(cp)
			if ps.DB != nil {
				if err := ps.DB.CreatePlot(cp); err != nil {
					return err
				}
			}
		} else if ps.DB != nil {
			if err := ps.DB.SetOwner(cp, newOwner); err != nil {
				return err
			}
		}
	}
	if w, ok := ps.WrapWorld(base.Area.WorldName); ok {
		generator.ClaimPlotBlocks(w, base.Area, base, ps.PlayerName(newOwner))
	}
	return nil
}

// SetFlag parses and sets flagName=rawVal on plot, persisting to SQLite.
func (ps *PlotSquared) SetFlag(plot *plotsquared.Plot, flagName, rawVal string) (any, error) {
	if plot == nil || !plot.HasOwner() {
		return nil, fmt.Errorf("plot is unowned")
	}
	def := plotsquared.LookupFlag(flagName)
	if def == nil {
		return nil, fmt.Errorf("unknown flag %q", flagName)
	}
	val, err := def.Parse(rawVal)
	if err != nil {
		return nil, err
	}
	base := plot.BasePlot()
	ev := &event.PlotFlagAddEvent{Plot: base, Flag: def.Name, Value: val}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		return nil, fmt.Errorf("flag change cancelled")
	}
	base.Flags.Set(def.Name, val)
	if ps.DB != nil {
		if err := ps.DB.SetFlag(base, def.Name, def.Format(val)); err != nil {
			return nil, err
		}
	}
	return val, nil
}

// RemoveFlag removes a flag from plot, persisting to SQLite.
func (ps *PlotSquared) RemoveFlag(plot *plotsquared.Plot, flagName string) (bool, error) {
	if plot == nil || !plot.HasOwner() {
		return false, fmt.Errorf("plot is unowned")
	}
	base := plot.BasePlot()
	key := strings.ToLower(flagName)
	if !base.Flags.HasLocal(key) {
		return false, nil
	}
	ev := &event.PlotFlagRemoveEvent{Plot: base, Flag: key}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		return false, fmt.Errorf("flag removal cancelled")
	}
	base.Flags.Remove(key)
	if ps.DB != nil {
		if err := ps.DB.RemoveFlag(base, key); err != nil {
			return false, err
		}
	}
	return true, nil
}

// SetComponent updates a plot component (floor, wall, border, etc.) and fires PlotComponentSetEvent.
func (ps *PlotSquared) SetComponent(plot *plotsquared.Plot, comp plotsquared.PlotComponent, pattern string) error {
	if plot == nil || plot.Area == nil {
		return fmt.Errorf("invalid plot")
	}
	bucket, err := plotsquared.ParseBlockBucket(pattern)
	if err != nil {
		return err
	}
	w, ok := ps.WrapWorld(plot.Area.WorldName)
	if !ok {
		return fmt.Errorf("world %s not loaded", plot.Area.WorldName)
	}
	if !generator.SetComponent(w, plot.Area, plot, comp, bucket) {
		return fmt.Errorf("failed to set component %s", comp)
	}
	ps.Server.Events().Fire(&event.PlotComponentSetEvent{
		Plot:      plot.BasePlot(),
		Component: comp,
		Pattern:   pattern,
	})
	return nil
}

// SetBiome sets the biome across plot.
func (ps *PlotSquared) SetBiome(plot *plotsquared.Plot, biomeName string) error {
	if plot == nil || plot.Area == nil {
		return fmt.Errorf("invalid plot")
	}
	b, ok := generator.ResolveBiome(biomeName)
	if !ok {
		return fmt.Errorf("unknown biome %q", biomeName)
	}
	w, ok := ps.WrapWorld(plot.Area.WorldName)
	if !ok {
		return fmt.Errorf("world %s not loaded", plot.Area.WorldName)
	}
	generator.SetPlotBiome(w, plot.Area, plot, b)
	return nil
}

// CopyPlot copies blocks and metadata from src to an unowned target plot.
func (ps *PlotSquared) CopyPlot(src *plotsquared.Plot, dstArea *plotsquared.PlotArea, dstID plotsquared.PlotId) (*plotsquared.Plot, error) {
	if src == nil || src.Area == nil || dstArea == nil {
		return nil, fmt.Errorf("invalid plot")
	}
	if src.IsMerged() {
		return nil, fmt.Errorf("merged plots cannot be copied")
	}
	if src.Area.PlotWidth != dstArea.PlotWidth {
		return nil, fmt.Errorf("plot areas have incompatible sizes (%d vs %d)", src.Area.PlotWidth, dstArea.PlotWidth)
	}
	if existing := dstArea.PlotAbs(dstID); existing != nil && existing.HasOwner() {
		return nil, fmt.Errorf("target plot %s is already occupied", dstID)
	}
	srcW, ok1 := ps.WrapWorld(src.Area.WorldName)
	dstW, ok2 := ps.WrapWorld(dstArea.WorldName)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("world not loaded")
	}
	dst, err := ps.ClaimPlot(dstArea, dstID, src.Owner, nil, false)
	if err != nil {
		return nil, err
	}
	generator.CopyPlotBlocks(srcW, src.Area, src.ID, dstW, dstArea, dstID)
	return dst, nil
}

// MovePlot moves src plot (blocks + DB record) to an unowned destination plot.
func (ps *PlotSquared) MovePlot(src *plotsquared.Plot, dstArea *plotsquared.PlotArea, dstID plotsquared.PlotId) error {
	if src == nil || src.Area == nil || dstArea == nil {
		return fmt.Errorf("invalid plot")
	}
	if src.IsMerged() {
		return fmt.Errorf("merged plots cannot be moved")
	}
	if src.Area.PlotWidth != dstArea.PlotWidth {
		return fmt.Errorf("plot areas have incompatible sizes (%d vs %d)", src.Area.PlotWidth, dstArea.PlotWidth)
	}
	if existing := dstArea.PlotAbs(dstID); existing != nil && existing.HasOwner() {
		return fmt.Errorf("target plot %s is already occupied", dstID)
	}
	srcW, ok1 := ps.WrapWorld(src.Area.WorldName)
	dstW, ok2 := ps.WrapWorld(dstArea.WorldName)
	if !ok1 || !ok2 {
		return fmt.Errorf("world not loaded")
	}
	oldArea := src.Area
	oldID := src.ID

	generator.CopyPlotBlocks(srcW, oldArea, oldID, dstW, dstArea, dstID)
	generator.ClearPlot(srcW, oldArea, src, ps.generatorForArea(oldArea))
	generator.UnclaimPlotBlocks(srcW, oldArea, src)

	oldArea.RemovePlot(oldID)
	src.ID = dstID
	dstArea.SetPlot(src)
	if ps.DB != nil {
		if err := ps.DB.MovePlot(src, dstArea.FullID(), dstID); err != nil {
			return err
		}
	}
	generator.ClaimPlotBlocks(dstW, dstArea, src, ps.PlayerName(src.Owner))
	return nil
}

// SwapPlots swaps blocks and ownership between two single plots of equal width.
func (ps *PlotSquared) SwapPlots(p1, p2 *plotsquared.Plot) error {
	if p1 == nil || p2 == nil || p1.Area == nil || p2.Area == nil {
		return fmt.Errorf("invalid plots")
	}
	if p1.IsMerged() || p2.IsMerged() {
		return fmt.Errorf("merged plots cannot be swapped")
	}
	if p1.Area.PlotWidth != p2.Area.PlotWidth {
		return fmt.Errorf("plot areas have incompatible sizes")
	}
	w1, ok1 := ps.WrapWorld(p1.Area.WorldName)
	w2, ok2 := ps.WrapWorld(p2.Area.WorldName)
	if !ok1 || !ok2 {
		return fmt.Errorf("world not loaded")
	}
	generator.SwapPlotBlocks(w1, p1.Area, p1.ID, w2, p2.Area, p2.ID)
	p1.Owner, p2.Owner = p2.Owner, p1.Owner
	if ps.DB != nil {
		if p1.DBID > 0 {
			_ = ps.DB.SetOwner(p1, p1.Owner)
		}
		if p2.DBID > 0 {
			_ = ps.DB.SetOwner(p2, p2.Owner)
		}
	}
	return nil
}

// CreatePlotWorld creates and loads a new plot world with the given PlotArea settings,
// saving it to worlds.yml.
func (ps *PlotSquared) CreatePlotWorld(area *plotsquared.PlotArea) (plugin.World, error) {
	if area == nil || area.WorldName == "" {
		return nil, fmt.Errorf("world name is required")
	}
	area.ComputeGeometry()
	low := strings.ToLower(area.WorldName)
	ps.mu.Lock()
	ps.areas[low] = []*plotsquared.PlotArea{area}
	ps.mu.Unlock()

	if ps.Worlds != nil {
		ps.Worlds.Worlds[area.WorldName] = config.WorldConfigFromArea(area)
		_ = config.SaveWorlds(ps.Server.DataDir(), ps.Worlds)
	}
	gen := ps.generatorForArea(area)
	gen.Reload(ps.Server.DataDir(), ps.Settings)

	if w, ok := ps.WrapWorld(area.WorldName); ok {
		return w, nil
	}
	return ps.Server.CreateWorld(plugin.WorldOptions{
		Key:       ps.WorldKeyForName(area.WorldName),
		Dimension: plugin.MinecraftKey("overworld"),
		Generator: gen,
	})
}

// CreatePartialArea adds a partial plot area inside an existing world and saves worlds.yml.
func (ps *PlotSquared) CreatePartialArea(worldName, id string, minID, maxID plotsquared.PlotId, area *plotsquared.PlotArea) error {
	if area == nil {
		area = plotsquared.DefaultPlotArea(worldName, id)
	}
	area.WorldName = worldName
	area.ID = id
	area.Type = plotsquared.AreaPartial
	m1, m2 := minID, maxID
	area.Min = &m1
	area.Max = &m2
	area.ComputeGeometry()

	low := strings.ToLower(worldName)
	ps.mu.Lock()
	ps.areas[low] = append(ps.areas[low], area)
	ps.mu.Unlock()

	if ps.Worlds != nil {
		wc := ps.Worlds.Worlds[worldName]
		if wc == nil {
			wc = config.DefaultWorldConfig()
			ps.Worlds.Worlds[worldName] = wc
		}
		if wc.Areas == nil {
			wc.Areas = make(map[string]*config.WorldConfig)
		}
		wc.Areas[config.FormatAreaKey(id, minID, maxID)] = config.WorldConfigFromArea(area)
		_ = config.SaveWorlds(ps.Server.DataDir(), ps.Worlds)
	}
	ps.generatorForArea(area)
	return nil
}

// Reload reloads settings.yml, worlds.yml, lang files, and generator patterns/schematics.
func (ps *PlotSquared) Reload() error {
	if err := ps.loadConfigs(); err != nil {
		return err
	}
	if ps.Lang != nil {
		_ = ps.Lang.LoadFromDir(ps.Server.DataDir(), ps.Settings.EnabledComponents.DefaultLocale)
	}
	for _, a := range ps.PlotAreas() {
		ps.generatorForArea(a).Reload(ps.Server.DataDir(), ps.Settings)
	}
	return nil
}

// PlayerName resolves a UUID to a player's name via online players or offline cache.
func (ps *PlotSquared) PlayerName(u uuid.UUID) string {
	if u == uuid.Nil {
		return "Server"
	}
	if u == plotsquared.EveryoneUUID {
		return "*"
	}
	if ps.Server != nil {
		if p, ok := ps.Server.Player(u); ok {
			return p.Name()
		}
		if op, ok := ps.Server.OfflinePlayer(u); ok && op.Name() != "" {
			return op.Name()
		}
	}
	return u.String()[:8]
}

// ResolvePlayerUUID resolves a player name, "*", or UUID string to a UUID.
func (ps *PlotSquared) ResolvePlayerUUID(name string) (uuid.UUID, bool) {
	name = strings.TrimSpace(name)
	if name == "*" || strings.EqualFold(name, "everyone") {
		return plotsquared.EveryoneUUID, true
	}
	if u, err := uuid.Parse(name); err == nil {
		return u, true
	}
	if ps.Server != nil {
		if p, ok := ps.Server.PlayerNamed(name); ok {
			return p.UUID(), true
		}
		if op, ok := ps.Server.OfflinePlayerNamed(name); ok {
			return op.UUID(), true
		}
	}
	return uuid.Nil, false
}
