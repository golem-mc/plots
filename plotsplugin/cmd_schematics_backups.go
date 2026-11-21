package plotsplugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/plots/internal/generator"
	"github.com/golem-mc/plots/internal/lang"
	westorage "github.com/golem-mc/worldedit/storage"
)

func (ps *Plugin) registerSchematicAndBackupCommands() {
	ps.addSubCommand(&subCommand{
		name:     "schematic",
		aliases:  []string{"schem"},
		perm:     "plots.schematic",
		usage:    "/plot schematic <save [name] | saveall <area> | paste <name> | list>",
		desc:     "Save, paste, or list plot schematics",
		category: catSchematic,
		run:      ps.cmdSchematic,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				return filterPrefix([]string{"save", "saveall", "paste", "list"}, c.args[0])
			}
			if len(c.args) == 2 && strings.EqualFold(c.args[0], "paste") {
				return filterPrefix(ps.listSchematicFiles(), c.args[1])
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "backup",
		perm:     "plots.backup",
		usage:    "/plot backup <save | list | load <#>>",
		desc:     "Save, list, or restore plot backups",
		category: catSettings,
		required: reqPlayer,
		run:      ps.cmdBackup,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				return filterPrefix([]string{"save", "list", "load"}, c.args[0])
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "createroadschematic",
		aliases:  []string{"crs"},
		perm:     "plots.createroadschematic",
		usage:    "/plot createroadschematic",
		desc:     "Create a road schematic from the roads around the current plot",
		category: catAdministration,
		required: reqPlayer,
		run:      ps.cmdCreateRoadSchematic,
	})

	ps.addSubCommand(&subCommand{
		name:     "debugroadregen",
		perm:     "plots.debugroadregen",
		usage:    "/plot debugroadregen <plot | region [height]>",
		desc:     "Regenerate roads around the current plot or region",
		category: catDebug,
		required: reqPlayer,
		run:      ps.cmdDebugRoadRegen,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix([]string{"plot", "region"}, lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "regenallroads",
		aliases:  []string{"rgar"},
		perm:     "plots.regenallroads",
		usage:    "/plot regenallroads <world> [height]",
		desc:     "Regenerate roads for all loaded chunks in a plot world",
		category: catAdministration,
		run:      ps.cmdRegenAllRoads,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix(ps.allAreaNames(), lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "download",
		aliases:  []string{"dl"},
		perm:     "plots.download",
		usage:    "/plot download [schem | world]",
		desc:     "Web upload is omitted on golem; use /plot schematic save instead",
		category: catSchematic,
		run: func(c *cmdCtx) {
			c.sender.SendMessage(lang.ParseMiniMessage("<gold>[Plots]</gold> <gray>Web upload/download services are not supported on golem. Use <gold>/plot schematic save</gold> to save schematics locally to <gold>plugins/plots/schematics/</gold>.</gray>"))
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "load",
		aliases:  []string{"restore"},
		perm:     "plots.load",
		usage:    "/plot load <schematic>",
		desc:     "Paste a saved schematic into your plot",
		category: catSchematic,
		required: reqPlayer,
		run: func(c *cmdCtx) {
			if len(c.args) < 1 {
				ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot load <schematic>")
				return
			}
			sub := *c
			sub.args = append([]string{"paste"}, c.args...)
			ps.cmdSchematic(&sub)
		},
	})
}

func (ps *Plugin) schematicsDir() string {
	sub := "schematics"
	if ps.Settings != nil && ps.Settings.Paths.Schematics != "" {
		sub = ps.Settings.Paths.Schematics
	}
	return filepath.Join(ps.Server.DataDir(), sub)
}

func (ps *Plugin) listSchematicFiles() []string {
	dir := ps.schematicsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if strings.HasSuffix(name, ".schem") || strings.HasSuffix(name, ".schematic") {
			out = append(out, strings.TrimSuffix(strings.TrimSuffix(name, ".schem"), ".schematic"))
		}
	}
	sort.Strings(out)
	return out
}

func (ps *Plugin) cmdSchematic(c *cmdCtx) {
	if len(c.args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot schematic <save [name] | saveall <area> | paste <name> | list>")
		return
	}
	switch strings.ToLower(c.args[0]) {
	case "list":
		if !HasPerm(c.sender, "plots.schematic.list") {
			ps.sendNoPerm(c.sender, "plots.schematic.list")
			return
		}
		files := ps.listSchematicFiles()
		if len(files) == 0 {
			ps.sendCaption(c.sender, "schematics.schematic_none")
			return
		}
		c.sender.SendMessage(lang.ParseMiniMessage("<gold>Schematics:</gold> <gray>" + strings.Join(files, ", ") + "</gray>"))
	case "save", "export":
		if !HasPerm(c.sender, "plots.schematic.save") {
			ps.sendNoPerm(c.sender, "plots.schematic.save")
			return
		}
		plot := ps.currentPlot(c)
		if plot == nil {
			ps.sendCaption(c.sender, "errors.not_in_plot")
			return
		}
		if !plot.HasOwner() {
			ps.sendCaption(c.sender, "info.plot_unowned")
			return
		}
		if c.player != nil && !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin.command.schematic.save") {
			ps.sendCaption(c.sender, "permission.no_plot_perms")
			return
		}
		w, ok := ps.WrapWorld(plot.Area.WorldName)
		if !ok {
			return
		}
		name := fmt.Sprintf("%s_%d_%d", plot.Area.WorldName, plot.ID.X, plot.ID.Y)
		if len(c.args) >= 2 {
			name = filepath.Base(c.args[1])
		}
		if !strings.HasSuffix(name, ".schem") && !strings.HasSuffix(name, ".schematic") {
			name += ".schem"
		}
		path := filepath.Join(ps.schematicsDir(), name)
		if err := generator.SavePlotSchematic(w, plot.Area, plot, path); err != nil {
			ps.sendCaption(c.sender, "errors.error", "value", err.Error())
			return
		}
		ps.sendCaption(c.sender, "schematics.schematic_valid")
	case "saveall", "exportall":
		if !HasPerm(c.sender, "plots.admin.command.schematic.save") {
			ps.sendNoPerm(c.sender, "plots.admin.command.schematic.save")
			return
		}
		if len(c.args) < 2 {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot schematic saveall <area>")
			return
		}
		area := ps.PlotAreaByName(c.args[1])
		if area == nil {
			ps.sendCaption(c.sender, "errors.not_valid_plot_world", "value", c.args[1])
			return
		}
		w, ok := ps.WrapWorld(area.WorldName)
		if !ok {
			return
		}
		basePlots := area.BasePlots()
		ps.sendCaption(c.sender, "schematics.schematic_exportall_started")
		saved := 0
		for _, pl := range basePlots {
			name := fmt.Sprintf("%s_%d_%d_%s.schem", area.WorldName, pl.ID.X, pl.ID.Y, ps.PlayerName(pl.Owner))
			if err := generator.SavePlotSchematic(w, area, pl, filepath.Join(ps.schematicsDir(), name)); err == nil {
				saved++
			}
		}
		ps.sendCaption(c.sender, "schematics.plot_to_schem", "amount", strconv.Itoa(saved))
		ps.sendCaption(c.sender, "schematics.schematic_exportall_finished")
	case "paste":
		if !HasPerm(c.sender, "plots.schematic.paste") {
			ps.sendNoPerm(c.sender, "plots.schematic.paste")
			return
		}
		if len(c.args) < 2 {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot schematic paste <name>")
			return
		}
		plot := ps.currentPlot(c)
		if plot == nil {
			ps.sendCaption(c.sender, "errors.not_in_plot")
			return
		}
		if !plot.HasOwner() {
			ps.sendCaption(c.sender, "info.plot_unowned")
			return
		}
		if c.player != nil && !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin.command.schematic.paste") {
			ps.sendCaption(c.sender, "permission.no_plot_perms")
			return
		}
		w, ok := ps.WrapWorld(plot.Area.WorldName)
		if !ok {
			return
		}
		name := filepath.Base(c.args[1])
		dir := ps.schematicsDir()
		var cb *westorage.Clipboard
		for _, cand := range []string{
			filepath.Join(dir, name),
			filepath.Join(dir, name+".schem"),
			filepath.Join(dir, name+".schematic"),
		} {
			if loaded, err := westorage.ReadSchematicFile(cand, ""); err == nil {
				cb = loaded
				break
			}
		}
		if cb == nil {
			ps.sendCaption(c.sender, "schematics.schematic_invalid", "reason", "not found")
			return
		}
		generator.PastePlotSchematic(w, plot.Area, plot, cb, false)
		ps.sendCaption(c.sender, "schematics.schematic_paste_success")
	default:
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot schematic <save [name] | saveall <area> | paste <name> | list>")
	}
}

func (ps *Plugin) cmdBackup(c *cmdCtx) {
	p := c.player
	if len(c.args) < 1 {
		ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot backup <save | list | load <#>>")
		return
	}
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(p, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(p, "backups.backup_impossible", "plot", "unowned")
		return
	}
	if plot.IsMerged() {
		ps.sendCaption(p, "backups.backup_impossible", "plot", "merged")
		return
	}
	if !plot.IsOwner(p.UUID()) && !HasPerm(p, "plots.admin.backup.other") {
		ps.sendNoPerm(p, "plots.admin.backup.other")
		return
	}
	w, ok := ps.WrapWorld(plot.Area.WorldName)
	if !ok {
		return
	}

	switch strings.ToLower(c.args[0]) {
	case "save":
		if !HasPerm(p, "plots.backup.save") {
			ps.sendNoPerm(p, "plots.backup.save")
			return
		}
		if _, err := ps.Backups.Save(w, plot.Area, plot); err != nil {
			ps.sendCaption(p, "backups.backup_save_failed", "reason", err.Error())
			return
		}
		ps.sendCaption(p, "backups.backup_save_success")
	case "list":
		if !HasPerm(p, "plots.backup.list") {
			ps.sendNoPerm(p, "plots.backup.list")
			return
		}
		entries, err := ps.Backups.List(plot)
		if err != nil {
			ps.sendCaption(p, "backups.backup_list_failed", "reason", err.Error())
			return
		}
		ps.sendCaption(p, "backups.backup_list_header", "plot", plot.ID.String())
		for i, e := range entries {
			ps.sendCaption(p, "backups.backup_list_entry",
				"number", strconv.Itoa(i+1),
				"value", e.CreatedAt.UTC().Format(time.RFC1123),
			)
		}
	case "load":
		if !HasPerm(p, "plots.backup.load") {
			ps.sendNoPerm(p, "plots.backup.load")
			return
		}
		if len(c.args) < 2 {
			ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot backup load <#>")
			return
		}
		num, err := strconv.Atoi(c.args[1])
		if err != nil || num < 1 {
			ps.sendCaption(p, "invalid.not_a_number", "value", c.args[1])
			return
		}
		ps.runWithConfirm(c, true, "/plot backup load "+c.args[1], func() {
			if err := ps.Backups.Restore(w, plot.Area, plot, num, false); err != nil {
				ps.sendCaption(p, "backups.backup_load_failure", "reason", err.Error())
				return
			}
			ps.sendCaption(p, "backups.backup_load_success")
		})
	default:
		ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot backup <save | list | load <#>>")
	}
}

func (ps *Plugin) cmdCreateRoadSchematic(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	w, ok := ps.WrapWorld(plot.Area.WorldName)
	if !ok {
		return
	}
	if err := generator.CreateRoadSchematic(w, plot.Area, plot, ps.Server.DataDir()); err != nil {
		ps.sendCaption(c.sender, "errors.error", "value", err.Error())
		return
	}
	ps.generatorForArea(plot.Area).Reload(ps.Server.DataDir(), ps.Settings)
	ps.sendCaption(c.sender, "schematics.schematic_road_created")
}

func (ps *Plugin) cmdDebugRoadRegen(c *cmdCtx) {
	if len(c.args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot debugroadregen <plot | region [height]>")
		return
	}
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	w, ok := ps.WrapWorld(plot.Area.WorldName)
	if !ok {
		return
	}
	gen := ps.generatorForArea(plot.Area)
	switch strings.ToLower(c.args[0]) {
	case "plot":
		if plot.IsMerged() {
			ps.sendCaption(c.sender, "debug.requires_unmerged")
			return
		}
		generator.CreateRoadEast(w, plot.Area, plot.ID, gen)
		generator.CreateRoadSouth(w, plot.Area, plot.ID, gen)
		generator.CreateRoadSouthEast(w, plot.Area, plot.ID, gen)
		ps.sendCaption(c.sender, "debugroadregen.regen_done", "value", plot.ID.String())
	case "region":
		bp := c.player.Location().Pos.Block()
		cx, cz := bp.X>>4, bp.Z>>4
		for dx := -1; dx <= 1; dx++ {
			for dz := -1; dz <= 1; dz++ {
				generator.RegenRoadChunk(w, plot.Area, gen, cx+dx, cz+dz)
			}
		}
		ps.sendCaption(c.sender, "debugroadregen.regen_done", "value", fmt.Sprintf("chunk %d,%d", cx, cz))
	default:
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot debugroadregen <plot | region [height]>")
	}
}

func (ps *Plugin) cmdRegenAllRoads(c *cmdCtx) {
	if len(c.args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot regenallroads <world> [height]")
		return
	}
	area := ps.PlotAreaByName(c.args[0])
	if area == nil {
		ps.sendCaption(c.sender, "errors.not_valid_plot_world", "value", c.args[0])
		return
	}
	w, ok := ps.WrapWorld(area.WorldName)
	if !ok {
		return
	}
	gen := ps.generatorForArea(area)
	gen.Reload(ps.Server.DataDir(), ps.Settings)
	seen := make(map[[2]int]bool)
	for _, pl := range area.BasePlots() {
		b := area.PlotInteriorBounds(pl.ID)
		for cx := (b.Min.X - area.RoadWidth) >> 4; cx <= (b.Max.X+area.RoadWidth)>>4; cx++ {
			for cz := (b.Min.Z - area.RoadWidth) >> 4; cz <= (b.Max.Z+area.RoadWidth)>>4; cz++ {
				key := [2]int{cx, cz}
				if seen[key] {
					continue
				}
				seen[key] = true
				generator.RegenRoadChunk(w, area, gen, cx, cz)
			}
		}
	}
	ps.sendCaption(c.sender, "debugroadregen.regen_done", "value", area.WorldName)
}
