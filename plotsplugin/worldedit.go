package plotsplugin

import (
	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/worldedit"
	"github.com/golem-mc/worldedit/region"
)

type weRegionManager struct {
	ps *Plugin
}

func (ps *Plugin) registerWorldEditHook() {
	we, ok := plugin.Service[worldedit.API](ps.Server)
	if !ok {
		return
	}
	we.RegisterRegionManager(&weRegionManager{ps: ps})
}

func (m *weRegionManager) Name() string {
	return "plots"
}

// AllowedRegions returns the WorldEdit regions p may modify in w:
// nil if w has no plot areas, restrictions are disabled, or p has bypass;
// otherwise the cuboid regions of every plot in w where p is owner/trusted/member
// (with an owner online for members) and no-worldedit is not set.
func (m *weRegionManager) AllowedRegions(p plugin.Player, w plugin.World) []region.Region {
	if p == nil || w == nil {
		return nil
	}
	if m.ps.Settings != nil && !m.ps.Settings.EnabledComponents.WorldEditRestrictions {
		return nil
	}
	areas := m.ps.PlotAreasInWorld(m.ps.WorldName(w))
	if len(areas) == 0 {
		return nil
	}
	if HasPerm(p, "plots.worldedit.bypass") {
		return nil
	}

	uid := p.UUID()
	allowed := make([]region.Region, 0)
	for _, area := range areas {
		for _, plot := range area.BasePlots() {
			if plot.FlagBool("no-worldedit") {
				continue
			}
			canBuild := plot.IsOwner(uid) || plot.IsTrusted(uid)
			if !canBuild && plot.IsMember(uid) {
				for _, owner := range plot.Owners() {
					if _, online := m.ps.Server.Player(owner); online {
						canBuild = true
						break
					}
				}
			}
			if !canBuild {
				continue
			}
			for _, b := range area.PlotRegions(plot) {
				allowed = append(allowed, region.NewCuboid(b.Min, b.Max))
			}
		}
	}
	return allowed
}
