package plotsplugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/config"
	"github.com/golem-mc/plots/internal/lang"
)

// singleWorldNamespace and singleWorldPrefix form the dimension key of a single
// plot's world: plots:single/<x>_<y> (Bukkit world name "<x>,<y>").
const (
	singleWorldNamespace = "plots"
	singleWorldPrefix    = "single/"
)

func singleKey(id plots.PlotId) plugin.Key {
	return plugin.Key{Namespace: singleWorldNamespace, Path: fmt.Sprintf("%s%d_%d", singleWorldPrefix, id.X, id.Y)}
}

// singleIDFromKey parses a plots:single/<x>_<y> key.
func singleIDFromKey(key plugin.Key) (plots.PlotId, bool) {
	if key.Namespace != singleWorldNamespace || !strings.HasPrefix(key.Path, singleWorldPrefix) {
		return plots.PlotId{}, false
	}
	xs, ys, ok := strings.Cut(strings.TrimPrefix(key.Path, singleWorldPrefix), "_")
	if !ok {
		return plots.PlotId{}, false
	}
	x, err1 := strconv.Atoi(xs)
	y, err2 := strconv.Atoi(ys)
	if err1 != nil || err2 != nil {
		return plots.PlotId{}, false
	}
	return plots.PlotId{X: x, Y: y}, true
}

// singleTemplate returns the "*" single plot area from worlds.yml, or nil.
func (ps *Plugin) singleTemplate() *plots.PlotArea {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	for _, a := range ps.areas[plots.SingleAreaName] {
		if a.Type == plots.AreaSingle && a.SingleID == nil {
			return a
		}
	}
	return nil
}

// singleInstance returns (creating it when needed) the area of the world that holds
// single plot id, with the template's settings and the plot's stored data.
func (ps *Plugin) singleInstance(id plots.PlotId) *plots.PlotArea {
	name := plots.SingleWorldName(id)
	if list := ps.PlotAreasInWorld(name); len(list) > 0 {
		return list[0]
	}
	tmpl := ps.singleTemplate()
	if tmpl == nil {
		return nil
	}
	a := config.WorldConfigFromArea(tmpl).ToPlotArea(name, "", nil, nil)
	a.Type = plots.AreaSingle
	sid := id
	a.SingleID = &sid
	// The plot fills the whole dimension: no roads, one huge cell.
	a.RoadWidth = 0
	a.PlotWidth = 1 << 27
	a.ComputeGeometry()

	ps.mu.Lock()
	if list := ps.areas[strings.ToLower(name)]; len(list) > 0 {
		ps.mu.Unlock()
		return list[0]
	}
	ps.areas[strings.ToLower(name)] = []*plots.PlotArea{a}
	ps.mu.Unlock()
	ps.attachLoadedData(a)
	return a
}

// registerSingleOwned creates the area of every stored single plot (their worlds load on demand).
func (ps *Plugin) registerSingleOwned() {
	if ps.singleTemplate() == nil {
		return
	}
	for id := range ps.plotsTmp[plots.SingleAreaName] {
		ps.singleInstance(id)
	}
}

// ensureSingleWorld loads (or creates) the dimension of a single plot area.
func (ps *Plugin) ensureSingleWorld(a *plots.PlotArea) (plugin.World, error) {
	if a == nil || a.Type != plots.AreaSingle || a.SingleID == nil {
		return nil, fmt.Errorf("not a single plot area")
	}
	if w, ok := ps.WrapWorld(a.WorldName); ok {
		return w, nil
	}
	return ps.Server.CreateWorld(plugin.WorldOptions{
		Key:       singleKey(*a.SingleID),
		Dimension: plugin.MinecraftKey("overworld"),
		Generator: ps.generatorForArea(a),
	})
}

// nextFreeSinglePlot returns the first plot id along the spiral that no one owns.
func (ps *Plugin) nextFreeSinglePlot() plots.PlotId {
	cur := plots.PlotId{}
	for {
		cur = cur.NextId()
		if cur.X == 0 && cur.Y == 0 {
			continue
		}
		list := ps.PlotAreasInWorld(plots.SingleWorldName(cur))
		if len(list) == 0 {
			return cur
		}
		if p := list[0].PlotAbs(cur); p == nil {
			return cur
		}
	}
}

// resetSingleWorld wipes a single plot's dimension: unload it without saving,
// delete its folder, so the next entry generates it afresh.
func (ps *Plugin) resetSingleWorld(a *plots.PlotArea) error {
	if a == nil || a.SingleID == nil {
		return fmt.Errorf("not a single plot area")
	}
	if w, ok := ps.WrapWorld(a.WorldName); ok {
		if err := ps.Server.UnloadWorld(w, false); err != nil {
			return err
		}
	}
	return os.RemoveAll(ps.Server.WorldFolder(singleKey(*a.SingleID)))
}

// onSingleLeave unloads a single plot's world once its last player left, as
// upstream's SingleWorldListener keeps unused plot worlds out of memory.
func (ps *Plugin) onSingleLeave(w plugin.World) {
	if w == nil || ps.Server == nil {
		return
	}
	if _, ok := singleIDFromKey(w.Key()); !ok {
		return
	}
	ps.Server.Scheduler().RunLater(1, func() {
		for _, p := range ps.Server.Players() {
			if pw := p.Location().World; pw != nil && pw.Key() == w.Key() {
				return
			}
		}
		if cur, ok := ps.WrapWorld(ps.WorldName(w)); ok {
			_ = ps.Server.UnloadWorld(cur, true)
		}
	})
}

func (ps *Plugin) registerSingleListeners() {
	plugin.Listen(ps.Server, func(e *plugin.PlayerChangedWorldEvent) { ps.onSingleLeave(e.From) })
	plugin.Listen(ps.Server, func(e *plugin.PlayerQuitEvent) {
		if l := e.Player.Location(); l.World != nil {
			ps.onSingleLeave(l.World)
		}
	})
}

// ImportWorlds registers the single plot worlds found in the server's dimension
// folder as owned by the server (upstream /plot debugimportworlds) and returns
// the ids it imported.
func (ps *Plugin) ImportWorlds() ([]plots.PlotId, error) {
	tmpl := ps.singleTemplate()
	if tmpl == nil {
		return nil, fmt.Errorf("no single plot area is configured in worlds.yml")
	}
	dir := filepath.Dir(ps.Server.WorldFolder(singleKey(plots.PlotId{})))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var imported []plots.PlotId
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id, ok := singleIDFromKey(plugin.Key{Namespace: singleWorldNamespace, Path: singleWorldPrefix + e.Name()})
		if !ok {
			continue
		}
		a := ps.singleInstance(id)
		if p := a.PlotAbs(id); p != nil {
			continue
		}
		if _, err := ps.ClaimPlot(a, id, plots.ServerUUID, nil, true); err != nil {
			return imported, err
		}
		imported = append(imported, id)
	}
	sort.Slice(imported, func(i, j int) bool {
		if imported[i].X != imported[j].X {
			return imported[i].X < imported[j].X
		}
		return imported[i].Y < imported[j].Y
	})
	return imported, nil
}

func (ps *Plugin) cmdDebugImportWorlds(c *cmdCtx) {
	ids, err := ps.ImportWorlds()
	if err != nil {
		ps.sendCaption(c.sender, "errors.error", "value", err.Error())
		return
	}
	c.sender.SendMessage(lang.ParseMiniMessage(fmt.Sprintf("<gold>[Plots]</gold> <gray>Imported %d single plot world(s).</gray>", len(ids))))
}

// CreateSingleArea registers area (or the defaults) as the single plot area "*"
// and saves it to worlds.yml. Plot worlds are created when plots are claimed.
func (ps *Plugin) CreateSingleArea(area *plots.PlotArea) *plots.PlotArea {
	if area == nil {
		area = plots.DefaultPlotArea(plots.SingleAreaName, "")
	}
	area.WorldName = plots.SingleAreaName
	area.Type = plots.AreaSingle
	area.SingleID = nil
	area.ComputeGeometry()
	ps.mu.Lock()
	ps.areas[plots.SingleAreaName] = []*plots.PlotArea{area}
	ps.mu.Unlock()
	if ps.Worlds != nil {
		ps.Worlds.Worlds[plots.SingleAreaName] = config.WorldConfigFromArea(area)
		_ = config.SaveWorlds(ps.Server.DataDir(), ps.Worlds)
	}
	return area
}
