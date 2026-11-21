package plotsquared

import (
	"github.com/golem-mc/golem/plugin"
	"github.com/google/uuid"
)

// API is the PlotSquared service provided on golem's service registry.
// Other plugins look it up with [Lookup].
type API interface {
	// PlotAreas returns every loaded plot area across all worlds.
	PlotAreas() []*PlotArea
	// PlotAreasInWorld returns all plot areas in the named world.
	PlotAreasInWorld(worldName string) []*PlotArea
	// PlotAreaAt returns the plot area covering loc, or nil if loc is not in a plot area.
	PlotAreaAt(loc plugin.Location) *PlotArea
	// PlotAt returns the plot at loc (including merged connecting roads), or nil.
	PlotAt(loc plugin.Location) *Plot
	// PlotsByOwner returns all base plots owned by owner across all worlds.
	PlotsByOwner(owner uuid.UUID) []*Plot
	// WorldName returns the Bukkit-style world name for w.
	WorldName(w plugin.World) string
	// WrapWorld returns the loaded plugin.World for a Bukkit-style world name.
	WrapWorld(worldName string) (plugin.World, bool)
}

// Lookup returns the registered PlotSquared API service, or false if the plugin is not enabled.
func Lookup(s plugin.Server) (API, bool) {
	return plugin.Service[API](s)
}
