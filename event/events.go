// Package event defines PlotSquared's plugin events on golem's event bus.
// Other plugins can listen to these events with plugin.Listen.
package event

import (
	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plotsquared"
	"github.com/google/uuid"
)

// PlayerClaimPlotEvent fires before a player claims a plot.
type PlayerClaimPlotEvent struct {
	plugin.Cancellable
	Player plugin.Player
	Plot   *plotsquared.Plot
	Auto   bool
}

// PlotDeleteEvent fires before a plot is deleted/unclaimed.
type PlotDeleteEvent struct {
	plugin.Cancellable
	Plot *plotsquared.Plot
}

// PlotClearEvent fires before a plot is cleared.
type PlotClearEvent struct {
	plugin.Cancellable
	Plot *plotsquared.Plot
}

// PlotMergeEvent fires before plots are merged.
type PlotMergeEvent struct {
	plugin.Cancellable
	Plot      *plotsquared.Plot
	Direction plotsquared.Direction
	Max       int
	Player    plugin.Player
}

// PlotUnlinkEvent fires before merged plots are unlinked.
type PlotUnlinkEvent struct {
	plugin.Cancellable
	Area   *plotsquared.PlotArea
	Plot   *plotsquared.Plot
	Reason string
}

// PlotAutoMergeEvent fires before plots are auto-merged.
type PlotAutoMergeEvent struct {
	plugin.Cancellable
	World string
	Plot  *plotsquared.Plot
	Plots []plotsquared.PlotId
}

// PlotChangeOwnerEvent fires before a plot's owner changes.
type PlotChangeOwnerEvent struct {
	plugin.Cancellable
	Initiator plugin.CommandSender
	Plot      *plotsquared.Plot
	OldOwner  uuid.UUID
	NewOwner  uuid.UUID
	HasOwner  bool
}

// PlayerEnterPlotEvent fires when a player enters a plot.
type PlayerEnterPlotEvent struct {
	plugin.Base
	Player plugin.Player
	Plot   *plotsquared.Plot
}

// PlayerLeavePlotEvent fires when a player leaves a plot.
type PlayerLeavePlotEvent struct {
	plugin.Base
	Player plugin.Player
	Plot   *plotsquared.Plot
}

// PlayerTeleportToPlotEvent fires before a player teleports to a plot.
type PlayerTeleportToPlotEvent struct {
	plugin.Cancellable
	Player plugin.Player
	Plot   *plotsquared.Plot
	Cause  string
}

// PlotFlagAddEvent fires before a flag is added or changed on a plot.
type PlotFlagAddEvent struct {
	plugin.Cancellable
	Plot  *plotsquared.Plot
	Flag  string
	Value any
}

// PlotFlagRemoveEvent fires before a flag is removed from a plot.
type PlotFlagRemoveEvent struct {
	plugin.Cancellable
	Plot *plotsquared.Plot
	Flag string
}

// PlotRateEvent fires before a player rates a plot.
type PlotRateEvent struct {
	plugin.Cancellable
	Player plugin.Player
	Plot   *plotsquared.Plot
	Rating int
}

// PlotDoneEvent fires when a plot is marked as done.
type PlotDoneEvent struct {
	plugin.Cancellable
	Plot *plotsquared.Plot
}

// PlotComponentSetEvent fires when a plot component (floor, wall, border, etc.) is changed.
type PlotComponentSetEvent struct {
	plugin.Base
	Plot      *plotsquared.Plot
	Component plotsquared.PlotComponent
	Pattern   string
}

// PlayerPlotTrustedEvent fires when a player is added to or removed from a plot's trusted list.
type PlayerPlotTrustedEvent struct {
	plugin.Cancellable
	Initiator plugin.CommandSender
	Plot      *plotsquared.Plot
	Target    uuid.UUID
	Added     bool
}

// PlayerPlotHelperEvent fires when a player is added to or removed from a plot's members list.
type PlayerPlotHelperEvent struct {
	plugin.Cancellable
	Initiator plugin.CommandSender
	Plot      *plotsquared.Plot
	Target    uuid.UUID
	Added     bool
}

// PlayerPlotDeniedEvent fires when a player is denied or undenied from a plot.
type PlayerPlotDeniedEvent struct {
	plugin.Cancellable
	Initiator plugin.CommandSender
	Plot      *plotsquared.Plot
	Target    uuid.UUID
	Added     bool
}

// PlayerBuyPlotEvent fires before a player buys a plot for sale.
type PlayerBuyPlotEvent struct {
	plugin.Cancellable
	Player plugin.Player
	Plot   *plotsquared.Plot
	Price  float64
}

// RemoveRoadEntityEvent fires before an entity on a road is removed by road-cleanup rules.
type RemoveRoadEntityEvent struct {
	plugin.Cancellable
	Entity plugin.Entity
}

// DefineAll registers every PlotSquared event type on golem's event bus.
func DefineAll(s plugin.Server) {
	plugin.DefineEvent[PlayerClaimPlotEvent](s)
	plugin.DefineEvent[PlotDeleteEvent](s)
	plugin.DefineEvent[PlotClearEvent](s)
	plugin.DefineEvent[PlotMergeEvent](s)
	plugin.DefineEvent[PlotUnlinkEvent](s)
	plugin.DefineEvent[PlotAutoMergeEvent](s)
	plugin.DefineEvent[PlotChangeOwnerEvent](s)
	plugin.DefineEvent[PlayerEnterPlotEvent](s)
	plugin.DefineEvent[PlayerLeavePlotEvent](s)
	plugin.DefineEvent[PlayerTeleportToPlotEvent](s)
	plugin.DefineEvent[PlotFlagAddEvent](s)
	plugin.DefineEvent[PlotFlagRemoveEvent](s)
	plugin.DefineEvent[PlotRateEvent](s)
	plugin.DefineEvent[PlotDoneEvent](s)
	plugin.DefineEvent[PlotComponentSetEvent](s)
	plugin.DefineEvent[PlayerPlotTrustedEvent](s)
	plugin.DefineEvent[PlayerPlotHelperEvent](s)
	plugin.DefineEvent[PlayerPlotDeniedEvent](s)
	plugin.DefineEvent[PlayerBuyPlotEvent](s)
	plugin.DefineEvent[RemoveRoadEntityEvent](s)
}
