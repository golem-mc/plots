package plotsplugin

import (
	"maps"
	"strconv"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/expiry"
	"github.com/golem-mc/plots/internal/generator"
	"github.com/google/uuid"
)

// registerDebugCommands registers the upstream DebugAllowUnsafe, DebugExec,
// DebugLoadTest and DebugSaveTest diagnostic commands.
func (ps *Plugin) registerDebugCommands() {
	ps.addSubCommand(&subCommand{
		name:     "debugallowunsafe",
		perm:     "plots.debugallowunsafe",
		usage:    "/plot debugallowunsafe",
		desc:     "Toggle unsafe actions for yourself",
		category: catDebug,
		run:      ps.cmdDebugAllowUnsafe,
	})
	ps.addSubCommand(&subCommand{
		name:     "debugexec",
		aliases:  []string{"exec", "$"},
		perm:     "plots.admin",
		usage:    "/plot debugexec <analyze | calibrate-analysis | start-expire | stop-expire | remove-flag | start-rgar | stop-rgar>",
		desc:     "Multi-purpose debug command",
		category: catDebug,
		run:      ps.cmdDebugExec,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix([]string{"analyze", "calibrate-analysis", "start-expire", "stop-expire", "remove-flag", "start-rgar", "stop-rgar"}, lastArg(c.args))
			}
			return nil
		},
	})
	ps.addSubCommand(&subCommand{
		name:     "debugloadtest",
		perm:     "plots.debugloadtest",
		usage:    "/plot debugloadtest",
		desc:     "Reload all plots from the database (console only)",
		category: catDebug,
		required: reqConsole,
		run:      ps.cmdDebugLoadTest,
	})
	ps.addSubCommand(&subCommand{
		name:     "debugsavetest",
		perm:     "plots.debugsavetest",
		usage:    "/plot debugsavetest",
		desc:     "Force the recreation of all plots in the database (console only)",
		category: catDebug,
		required: reqConsole,
		run:      ps.cmdDebugSaveTest,
	})
}

// UnsafeAllowed reports whether u enabled unsafe actions with /plot debugallowunsafe.
func (ps *Plugin) UnsafeAllowed(u uuid.UUID) bool {
	ps.dbgMu.Lock()
	defer ps.dbgMu.Unlock()
	_, ok := ps.unsafeAllowed[u]
	return ok
}

func (ps *Plugin) cmdDebugAllowUnsafe(c *cmdCtx) {
	u := senderUUID(c.sender)
	ps.dbgMu.Lock()
	if ps.unsafeAllowed == nil {
		ps.unsafeAllowed = make(map[uuid.UUID]struct{})
	}
	_, on := ps.unsafeAllowed[u]
	if on {
		delete(ps.unsafeAllowed, u)
	} else {
		ps.unsafeAllowed[u] = struct{}{}
	}
	ps.dbgMu.Unlock()
	if on {
		ps.sendCaption(c.sender, "unsafe.debugallowunsafe_off")
	} else {
		ps.sendCaption(c.sender, "unsafe.debugallowunsafe_on")
	}
}

func (ps *Plugin) cmdDebugLoadTest(c *cmdCtx) {
	if ps.DB == nil {
		return
	}
	loaded, err := ps.DB.LoadPlots()
	if err != nil {
		ps.sendCaption(c.sender, "errors.error", "value", err.Error())
		return
	}
	ps.mu.Lock()
	ps.plotsTmp = loaded
	ps.mu.Unlock()
}

func (ps *Plugin) cmdDebugSaveTest(c *cmdCtx) {
	ps.sendCaption(c.sender, "debugsavetest.starting")
	if ps.DB != nil {
		for _, area := range ps.PlotAreas() {
			for _, pl := range area.Plots() {
				flags := make(map[string]string)
				for name, v := range pl.Flags.LocalFlags() {
					if def := plots.LookupFlag(name); def != nil {
						flags[name] = def.Format(v)
					}
				}
				maps.Copy(flags, pl.Flags.UnknownFlags())
				if err := ps.DB.ResavePlot(pl, flags); err != nil {
					ps.sendCaption(c.sender, "errors.error", "value", err.Error())
					return
				}
			}
		}
	}
	ps.sendCaption(c.sender, "debugsavetest.done")
}

func (ps *Plugin) cmdDebugExec(c *cmdCtx) {
	if len(c.args) == 0 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot debugexec <analyze | calibrate-analysis | start-expire | stop-expire | remove-flag | start-rgar | stop-rgar>")
		return
	}
	switch strings.ToLower(c.args[0]) {
	case "analyze":
		plot := ps.currentPlot(c)
		if plot == nil {
			ps.sendCaption(c.sender, "errors.not_in_plot")
			return
		}
		w, ok := ps.WrapWorld(plot.Area.WorldName)
		if !ok {
			return
		}
		ps.sendCaption(c.sender, "debugexec.starting_task")
		an := expiry.AnalyzePlot(w, plot.Area, plot)
		ps.sendCaption(c.sender, "debugexec.changes_column", "value", strconv.Itoa(an.Changes))
		ps.sendCaption(c.sender, "debugexec.analyze_done", "command", "/plot debugexec analyze")
	case "calibrate-analysis":
		if len(c.args) != 2 {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot debugexec analyze <threshold>")
			ps.sendCaption(c.sender, "debugexec.threshold_default")
			return
		}
		if _, err := strconv.Atoi(c.args[1]); err != nil {
			ps.sendCaption(c.sender, "debugexec.invalid_threshold", "value", c.args[1])
			ps.sendCaption(c.sender, "debugexec.threshold_default_double")
			return
		}
		// Upstream fits the analysis modifiers to the threshold; the configured
		// auto_clear calibration is kept, so only the acknowledgement is sent.
		ps.sendCaption(c.sender, "debugexec.calibration_done")
	case "start-expire":
		if ps.startExpiryTimer() {
			ps.sendCaption(c.sender, "debugexec.expiry_started")
		} else {
			ps.sendCaption(c.sender, "debugexec.expiry_already_started")
		}
	case "stop-expire":
		if ps.stopExpiryTimer() {
			ps.sendCaption(c.sender, "debugexec.task_cancelled")
		} else {
			ps.sendCaption(c.sender, "debugexec.task_halted")
		}
	case "remove-flag":
		if len(c.args) != 2 {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot debugexec remove-flag <flag>")
			return
		}
		flag := c.args[1]
		if def := plots.LookupFlag(flag); def != nil {
			for _, area := range ps.PlotAreas() {
				for _, pl := range area.BasePlots() {
					_, _ = ps.RemoveFlag(pl, def.Name)
				}
			}
		}
		ps.sendCaption(c.sender, "debugexec.cleared_flag", "value", flag)
	case "start-rgar":
		if len(c.args) != 2 {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "Invalid syntax: /plot debugexec start-rgar <world>")
			return
		}
		area := ps.PlotAreaByName(c.args[1])
		if area == nil {
			ps.sendCaption(c.sender, "errors.not_valid_plot_world", "value", c.args[1])
			return
		}
		if !ps.startRoadRegen(area) {
			ps.sendCaption(c.sender, "debugexec.mass_schematic_update_in_progress")
			return
		}
		ps.sendCaption(c.sender, "debugroadregen.regenallroads_started")
	case "stop-rgar":
		ps.dbgMu.Lock()
		task := ps.rgarTask
		ps.rgarTask = nil
		ps.dbgMu.Unlock()
		if task == nil || task.Cancelled() {
			ps.sendCaption(c.sender, "debugexec.task_not_running")
			return
		}
		task.Cancel()
		ps.sendCaption(c.sender, "debugexec.task_cancelled")
	default:
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot debugexec <analyze | calibrate-analysis | start-expire | stop-expire | remove-flag | start-rgar | stop-rgar>")
	}
}

// startRoadRegen regenerates the roads of area a few chunks per tick and
// reports false when a regeneration is already running.
func (ps *Plugin) startRoadRegen(area *plots.PlotArea) bool {
	ps.dbgMu.Lock()
	defer ps.dbgMu.Unlock()
	if ps.rgarTask != nil && !ps.rgarTask.Cancelled() {
		return false
	}
	w, ok := ps.WrapWorld(area.WorldName)
	if !ok {
		return false
	}
	gen := ps.generatorForArea(area)
	gen.Reload(ps.Server.DataDir(), ps.Settings)
	queue := roadChunks(area)
	var task plugin.Task
	task = ps.Server.Scheduler().RunTimer(1, 1, func() {
		for i := 0; i < 4 && len(queue) > 0; i++ {
			generator.RegenRoadChunk(w, area, gen, queue[0][0], queue[0][1])
			queue = queue[1:]
		}
		if len(queue) == 0 {
			task.Cancel()
		}
	})
	ps.rgarTask = task
	return true
}
