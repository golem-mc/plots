package plotsplugin

import (
	"archive/zip"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/config"
	"github.com/golem-mc/plots/internal/expiry"
	"github.com/golem-mc/plots/internal/generator"
	"github.com/golem-mc/plots/internal/lang"
	"github.com/google/uuid"
	"go.yaml.in/yaml/v4"
)

func (ps *Plugin) registerAdminCommands() {
	ps.addSubCommand(&subCommand{
		name:     "area",
		aliases:  []string{"world"},
		perm:     "plots.area",
		usage:    "/plot area <create <world[:id]> [key=val...] | info [area] | list | tp <area> | regen>",
		desc:     "Create and manage plot worlds and areas",
		category: catAdministration,
		run:      ps.cmdArea,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				return filterPrefix([]string{"create", "info", "list", "tp", "regen"}, c.args[0])
			}
			if len(c.args) == 2 && (strings.EqualFold(c.args[0], "info") || strings.EqualFold(c.args[0], "tp")) {
				return filterPrefix(ps.allAreaNames(), c.args[1])
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "setup",
		aliases:  []string{"create"},
		perm:     "plots.admin.command.setup",
		usage:    "/plot setup <world> [plot.size=N] [road.width=N] [plot.height=N] [type=normal|augmented|partial]",
		desc:     "Create a new plot world",
		category: catAdministration,
		run:      ps.cmdSetup,
	})

	ps.addSubCommand(&subCommand{
		name:     "template",
		perm:     "plots.admin",
		usage:    "/plot template <import <world> <template> | export <world> | list>",
		desc:     "Import or export plot world templates",
		category: catAdministration,
		run:      ps.cmdTemplate,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				return filterPrefix([]string{"import", "export", "list"}, c.args[0])
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "purge",
		perm:     "plots.admin",
		usage:    "/plot purge world:<w> area:<a> id:<x;y> owner:<p> shared:<p> unknown:<bool> clear:<bool>",
		desc:     "Bulk delete plots matching filter criteria",
		category: catAdministration,
		run:      ps.cmdPurge,
	})

	ps.addSubCommand(&subCommand{
		name:     "condense",
		perm:     "plots.admin",
		usage:    "/plot condense <area> <start <radius> | stop | info>",
		desc:     "Move outer plots closer to the origin (0;0)",
		category: catAdministration,
		run:      ps.cmdCondense,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				return filterPrefix(ps.allAreaNames(), c.args[0])
			}
			if len(c.args) == 2 {
				return filterPrefix([]string{"start", "stop", "info"}, c.args[1])
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "trim",
		perm:     "plots.admin",
		usage:    "/plot trim <world> [regenerate]",
		desc:     "Inspect or reset unclaimed plots in a world",
		category: catAdministration,
		run:      ps.cmdTrim,
	})

	ps.addSubCommand(&subCommand{
		name:     "reload",
		aliases:  []string{"rl"},
		perm:     "plots.admin.command.reload",
		usage:    "/plot reload",
		desc:     "Reload configuration, messages, and road schematics",
		category: catAdministration,
		run:      ps.cmdReload,
	})

	ps.addSubCommand(&subCommand{
		name:     "plugin",
		aliases:  []string{"version"},
		perm:     "plots.use",
		usage:    "/plot plugin",
		desc:     "Show plugin version and status",
		category: catInfo,
		run:      ps.cmdPlugin,
	})

	ps.addSubCommand(&subCommand{
		name:     "debug",
		perm:     "plots.admin",
		usage:    "/plot debug",
		desc:     "Show plot areas, loaded plots, and storage stats",
		category: catDebug,
		run:      ps.cmdDebug,
	})

	ps.addSubCommand(&subCommand{
		name:     "database",
		aliases:  []string{"convert"},
		perm:     "plots.database",
		usage:    "/plot database",
		desc:     "Show SQLite database status",
		category: catAdministration,
		run: func(c *cmdCtx) {
			c.sender.SendMessage(lang.ParseMiniMessage("<gold>[Plots]</gold> <gray>Using pure-Go SQLite storage at <gold>plugins/plots/storage.db</gold>. MySQL conversion is omitted on golem.</gray>"))
		},
	})
}

func (ps *Plugin) cmdArea(c *cmdCtx) {
	if len(c.args) == 0 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot area <create | info | list | tp | regen>")
		return
	}
	switch strings.ToLower(c.args[0]) {
	case "list", "l":
		if !HasPerm(c.sender, "plots.area.list") {
			ps.sendNoPerm(c.sender, "plots.area.list")
			return
		}
		areas := ps.PlotAreas()
		sort.Slice(areas, func(i, j int) bool { return areas[i].FullID() < areas[j].FullID() })
		c.sender.SendMessage(lang.ParseMiniMessage(fmt.Sprintf("<gold>Plot Areas (%d):</gold>", len(areas))))
		for _, a := range areas {
			c.sender.SendMessage(lang.ParseMiniMessage(fmt.Sprintf(
				"<gray>- <gold>%s</gold> (type=%s, size=%d, road=%d, plots=%d)</gray>",
				a.FullID(), a.Type.String(), a.PlotWidth, a.RoadWidth, len(a.BasePlots()),
			)))
		}
	case "info", "i":
		if !HasPerm(c.sender, "plots.area.info") {
			ps.sendNoPerm(c.sender, "plots.area.info")
			return
		}
		var area *plots.PlotArea
		if len(c.args) >= 2 {
			area = ps.PlotAreaByName(c.args[1])
		} else {
			area = ps.currentArea(c)
		}
		if area == nil {
			ps.sendCaption(c.sender, "errors.not_in_plot_world")
			return
		}
		c.sender.SendMessage(lang.ParseMiniMessage(fmt.Sprintf(
			"<gold>Area %s:</gold> <gray>world=%s type=%s terrain=%s plot=%d road=%d height=%d biome=%s economy=%t plots=%d</gray>",
			area.FullID(), area.WorldName, area.Type.String(), area.Terrain.String(),
			area.PlotWidth, area.RoadWidth, area.PlotHeight, area.PlotBiome, area.UseEconomy, len(area.BasePlots()),
		)))
	case "tp", "teleport":
		if c.player == nil {
			ps.sendCaption(c.sender, "errors.is_console")
			return
		}
		if !HasPerm(c.sender, "plots.area.tp") {
			ps.sendNoPerm(c.sender, "plots.area.tp")
			return
		}
		if len(c.args) < 2 {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot area tp <area>")
			return
		}
		area := ps.PlotAreaByName(c.args[1])
		if area == nil {
			ps.sendCaption(c.sender, "errors.not_valid_plot_world", "value", c.args[1])
			return
		}
		id := plots.PlotId{X: 1, Y: 1}
		if area.Min != nil {
			id = *area.Min
		}
		ps.teleportToPlot(c.player, area.GetOrCreatePlot(id), "area_tp")
		ps.sendCaption(c.sender, "teleport.teleported_to_plot")
	case "c", "create", "setup":
		if !HasPerm(c.sender, "plots.area.create") {
			ps.sendNoPerm(c.sender, "plots.area.create")
			return
		}
		ps.createAreaFromArgs(c, c.args[1:])
	case "regen":
		if !HasPerm(c.sender, "plots.area.regen") {
			ps.sendNoPerm(c.sender, "plots.area.regen")
			return
		}
		area := ps.currentArea(c)
		if area == nil {
			ps.sendCaption(c.sender, "errors.not_in_plot_world")
			return
		}
		w, ok := ps.WrapWorld(area.WorldName)
		if !ok {
			return
		}
		gen := ps.generatorForArea(area)
		for _, pl := range area.BasePlots() {
			generator.ClearPlot(w, area, pl, gen)
		}
		c.sender.SendMessage(lang.ParseMiniMessage("<gold>[Plots]</gold> <gray>Regenerated plots in area " + area.FullID() + ".</gray>"))
	default:
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot area <create | info | list | tp | regen>")
	}
}

func (ps *Plugin) cmdSetup(c *cmdCtx) {
	if len(c.args) == 0 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot setup <world> [plot.size=N] [road.width=N] [plot.height=N] [type=normal|augmented|partial]")
		return
	}
	ps.createAreaFromArgs(c, c.args)
}

func (ps *Plugin) createAreaFromArgs(c *cmdCtx, args []string) {
	if len(args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot area create <world[:id]> [key=val...]")
		return
	}
	worldName, areaID, hasID := strings.Cut(args[0], ":")
	area := plots.DefaultPlotArea(worldName, areaID)
	var minID, maxID plots.PlotId
	hasMin, hasMax := false, false

	for _, kv := range args[1:] {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		applyAreaModifier(area, strings.ToLower(k), v, &minID, &maxID, &hasMin, &hasMax)
	}

	if hasID || area.Type == plots.AreaPartial {
		if !hasMin {
			minID = plots.PlotId{X: 1, Y: 1}
		}
		if !hasMax {
			maxID = plots.PlotId{X: 5, Y: 5}
		}
		if area.ID == "" {
			area.ID = "area1"
		}
		if err := ps.CreatePartialArea(worldName, area.ID, minID, maxID, area); err != nil {
			ps.sendCaption(c.sender, "errors.error", "value", err.Error())
			return
		}
		ps.sendCaption(c.sender, "setup.setup_finished", "world", area.FullID())
		return
	}

	if _, err := ps.CreatePlotWorld(area); err != nil {
		ps.sendCaption(c.sender, "errors.error_create", "world", worldName)
		return
	}
	ps.sendCaption(c.sender, "setup.setup_finished", "world", worldName)
}

func applyAreaModifier(area *plots.PlotArea, key, val string, minID, maxID *plots.PlotId, hasMin, hasMax *bool) {
	switch key {
	case "plot.size", "size", "s":
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			area.PlotWidth = n
		}
	case "road.width", "road", "g":
		if n, err := strconv.Atoi(val); err == nil && n >= 0 {
			area.RoadWidth = n
		}
	case "plot.height", "height", "h":
		if n, err := strconv.Atoi(val); err == nil {
			area.PlotHeight = n
			area.RoadHeight = n
			area.WallHeight = n
		}
	case "road.height":
		if n, err := strconv.Atoi(val); err == nil {
			area.RoadHeight = n
		}
	case "wall.height":
		if n, err := strconv.Atoi(val); err == nil {
			area.WallHeight = n
		}
	case "plot.floor", "floor":
		if b, err := plots.ParseBlockBucket(val); err == nil {
			area.TopBlock = b
		}
	case "plot.filling", "filling":
		if b, err := plots.ParseBlockBucket(val); err == nil {
			area.MainBlock = b
		}
	case "road.block":
		if b, err := plots.ParseBlockBucket(val); err == nil {
			area.RoadBlock = b
		}
	case "wall.block":
		if b, err := plots.ParseBlockBucket(val); err == nil {
			area.WallBlock = b
		}
	case "type":
		if t, ok := plots.ParsePlotAreaType(val); ok {
			area.Type = t
		}
	case "terrain":
		if tr, ok := plots.ParsePlotAreaTerrainType(val); ok {
			area.Terrain = tr
		}
	case "min":
		if id, ok := plots.ParsePlotId(val); ok {
			*minID = id
			*hasMin = true
		}
	case "max":
		if id, ok := plots.ParsePlotId(val); ok {
			*maxID = id
			*hasMax = true
		}
	case "economy":
		area.UseEconomy = strings.EqualFold(val, "true")
	}
}

func (ps *Plugin) templatesDir() string {
	sub := "templates"
	if ps.Settings != nil && ps.Settings.Paths.Templates != "" {
		sub = ps.Settings.Paths.Templates
	}
	return filepath.Join(ps.Server.DataDir(), sub)
}

func (ps *Plugin) cmdTemplate(c *cmdCtx) {
	if len(c.args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot template <import <world> <template> | export <world> | list>")
		return
	}
	switch strings.ToLower(c.args[0]) {
	case "list":
		entries, _ := os.ReadDir(ps.templatesDir())
		var names []string
		for _, de := range entries {
			if before, ok := strings.CutSuffix(de.Name(), ".template"); ok {
				names = append(names, before)
			}
		}
		sort.Strings(names)
		c.sender.SendMessage(lang.ParseMiniMessage("<gold>Templates:</gold> <gray>" + strings.Join(names, ", ") + "</gray>"))
	case "import":
		if !HasPerm(c.sender, "plots.template.import") {
			ps.sendNoPerm(c.sender, "plots.template.import")
			return
		}
		if len(c.args) != 3 {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot template import <world> <template>")
			return
		}
		worldName := c.args[1]
		tmplName := strings.TrimSuffix(filepath.Base(c.args[2]), ".template")
		if err := ps.importTemplate(worldName, tmplName); err != nil {
			ps.sendCaption(c.sender, "errors.error", "value", err.Error())
			return
		}
		ps.sendCaption(c.sender, "setup.setup_finished", "world", worldName)
	case "export":
		if !HasPerm(c.sender, "plots.template.export") {
			ps.sendNoPerm(c.sender, "plots.template.export")
			return
		}
		if len(c.args) != 2 {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot template export <world>")
			return
		}
		area := ps.PlotAreaByName(c.args[1])
		if area == nil {
			ps.sendCaption(c.sender, "errors.not_valid_plot_world", "value", c.args[1])
			return
		}
		if err := ps.exportTemplate(area); err != nil {
			ps.sendCaption(c.sender, "errors.error", "value", err.Error())
			return
		}
		c.sender.SendMessage(lang.ParseMiniMessage("<gold>[Plots]</gold> <gray>Exported template <gold>" + area.WorldName + ".template</gold>.</gray>"))
	default:
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot template <import <world> <template> | export <world> | list>")
	}
}

func (ps *Plugin) importTemplate(worldName, tmplName string) error {
	zipPath := filepath.Join(ps.templatesDir(), tmplName+".template")
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("template %q not found: %w", tmplName, err)
	}
	defer func() { _ = zr.Close() }()

	dataDir := ps.Server.DataDir()
	var worldCfgBytes []byte
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel := strings.ReplaceAll(f.Name, "__TEMP_DIR__", worldName)
		rel = filepath.Clean(strings.ReplaceAll(rel, "\\", "/"))
		if strings.HasPrefix(rel, "..") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
		if filepath.Base(rel) == "worlds.yml" || filepath.Base(rel) == "settings.yml" {
			worldCfgBytes = data
			continue
		}
		dst := filepath.Join(dataDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	wc := config.DefaultWorldConfig()
	if len(worldCfgBytes) > 0 {
		_ = yaml.Unmarshal(worldCfgBytes, wc)
	}
	area := wc.ToPlotArea(worldName, "", nil, nil)
	_, err = ps.CreatePlotWorld(area)
	return err
}

func (ps *Plugin) exportTemplate(area *plots.PlotArea) error {
	if err := os.MkdirAll(ps.templatesDir(), 0o755); err != nil {
		return err
	}
	outPath := filepath.Join(ps.templatesDir(), area.WorldName+".template")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	defer func() { _ = zw.Close() }()

	wc := config.WorldConfigFromArea(area)
	ymlBytes, err := yaml.Marshal(wc)
	if err != nil {
		return err
	}
	w, err := zw.Create("config/worlds.yml")
	if err != nil {
		return err
	}
	if _, err := w.Write(ymlBytes); err != nil {
		return err
	}

	// Include road schematics if present
	roadDir := filepath.Join(ps.Server.DataDir(), "schematics", "GEN_ROAD_SCHEMATIC", area.WorldName)
	entries, _ := os.ReadDir(roadDir)
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(roadDir, de.Name()))
		if err != nil {
			continue
		}
		ew, err := zw.Create("schematics/GEN_ROAD_SCHEMATIC/__TEMP_DIR__/" + de.Name())
		if err != nil {
			return err
		}
		_, _ = ew.Write(data)
	}
	return nil
}

func (ps *Plugin) cmdPurge(c *cmdCtx) {
	if len(c.args) == 0 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot purge world:<w> area:<a> id:<x;y> owner:<p> shared:<p> unknown:<bool> clear:<bool>")
		return
	}
	var worldFilter string
	var areaFilter *plots.PlotArea
	var idFilter *plots.PlotId
	var ownerFilter, sharedFilter uuid.UUID
	clearBlocks, unknownOnly := false, false

	for _, arg := range c.args {
		k, v, ok := strings.Cut(arg, ":")
		if !ok {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot purge world:<w> area:<a> id:<x;y> owner:<p> shared:<p> unknown:<bool> clear:<bool>")
			return
		}
		switch strings.ToLower(k) {
		case "world", "w":
			worldFilter = v
		case "area", "a":
			areaFilter = ps.PlotAreaByName(v)
			if areaFilter == nil {
				ps.sendCaption(c.sender, "errors.not_valid_plot_world", "value", v)
				return
			}
		case "id", "plotid":
			id, okID := plots.ParsePlotId(v)
			if !okID {
				ps.sendCaption(c.sender, "invalid.not_valid_plot_id")
				return
			}
			idFilter = &id
		case "owner", "o":
			u, okU := ps.ResolvePlayerUUID(v)
			if !okU {
				ps.sendCaption(c.sender, "errors.invalid_player", "value", v)
				return
			}
			ownerFilter = u
		case "shared", "s":
			u, okU := ps.ResolvePlayerUUID(v)
			if !okU {
				ps.sendCaption(c.sender, "errors.invalid_player", "value", v)
				return
			}
			sharedFilter = u
		case "clear", "c", "delete", "d":
			clearBlocks = strings.EqualFold(v, "true")
		case "unknown", "u":
			unknownOnly = strings.EqualFold(v, "true")
		}
	}

	var matched []*plots.Plot
	for _, a := range ps.PlotAreas() {
		if worldFilter != "" && !strings.EqualFold(a.WorldName, worldFilter) {
			continue
		}
		if areaFilter != nil && a != areaFilter {
			continue
		}
		for _, pl := range a.BasePlots() {
			if idFilter != nil && pl.ID != *idFilter {
				continue
			}
			if ownerFilter != uuid.Nil && !pl.IsOwner(ownerFilter) {
				continue
			}
			if sharedFilter != uuid.Nil && !pl.IsAdded(sharedFilter) {
				continue
			}
			if unknownOnly {
				if _, ok := ps.Server.OfflinePlayer(pl.Owner); ok {
					continue
				}
			}
			matched = append(matched, pl)
		}
	}

	ps.runWithConfirm(c, true, fmt.Sprintf("/plot purge (%d plots)", len(matched)), func() {
		deleted := 0
		for _, pl := range matched {
			if err := ps.DeletePlot(pl, clearBlocks); err == nil {
				deleted++
			}
		}
		ps.sendCaption(c.sender, "purge.purge_success", "amount", strconv.Itoa(deleted))
	})
}

func (ps *Plugin) cmdCondense(c *cmdCtx) {
	if len(c.args) < 2 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot condense <area> <start <radius> | stop | info>")
		return
	}
	area := ps.PlotAreaByName(c.args[0])
	if area == nil {
		ps.sendCaption(c.sender, "condense.invalid_area")
		return
	}
	basePlots := area.BasePlots()
	minRadius := int(math.Ceil(math.Sqrt(float64(len(basePlots)))/2.0 + 1))

	switch strings.ToLower(c.args[1]) {
	case "info":
		radius := minRadius
		if len(c.args) >= 3 {
			if r, err := strconv.Atoi(c.args[2]); err == nil && r > 0 {
				radius = r
			}
		}
		outside := 0
		for _, pl := range basePlots {
			if absInt(pl.ID.X) > radius || absInt(pl.ID.Y) > radius {
				outside++
			}
		}
		ps.sendCaption(c.sender, "condense.default_eval")
		ps.sendCaption(c.sender, "condense.minimum_radius", "minimum_radius", strconv.Itoa(minRadius))
		ps.sendCaption(c.sender, "condense.input_radius", "radius", strconv.Itoa(radius))
		ps.sendCaption(c.sender, "condense.estimated_moves", "user_move", strconv.Itoa(outside))
	case "stop":
		ps.sendCaption(c.sender, "condense.task_stopped")
	case "start":
		if len(c.args) < 3 {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot condense <area> start <radius>")
			return
		}
		radius, err := strconv.Atoi(c.args[2])
		if err != nil {
			ps.sendCaption(c.sender, "condense.invalid_radius")
			return
		}
		if radius < minRadius {
			ps.sendCaption(c.sender, "condense.radius_too_small")
			return
		}
		var outer []*plots.Plot
		for _, pl := range basePlots {
			if !pl.IsMerged() && (absInt(pl.ID.X) > radius || absInt(pl.ID.Y) > radius) {
				outer = append(outer, pl)
			}
		}
		var free []plots.PlotId
		cur := plots.PlotId{X: 0, Y: 0}
		limit := (2*radius + 1) * (2*radius + 1)
		for i := 0; i < limit && len(free) < len(outer); i++ {
			cur = cur.NextId()
			if absInt(cur.X) <= radius && absInt(cur.Y) <= radius && area.PlotAbs(cur) == nil {
				free = append(free, cur)
			}
		}
		if len(outer) == 0 || len(free) == 0 {
			ps.sendCaption(c.sender, "condense.no_free_plots_found")
			return
		}
		ps.sendCaption(c.sender, "condense.task_started")
		for i := 0; i < len(outer) && i < len(free); i++ {
			src := outer[i]
			origID := src.ID.String()
			dstID := free[i]
			if err := ps.MovePlot(src, area, dstID); err == nil {
				ps.sendCaption(c.sender, "condense.moving", "origin", origID, "possible", dstID.String())
			}
		}
		ps.sendCaption(c.sender, "condense.task_complete")
	default:
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot condense <area> <start <radius> | stop | info>")
	}
}

func (ps *Plugin) cmdTrim(c *cmdCtx) {
	if len(c.args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot trim <world> [regenerate]")
		return
	}
	area := ps.PlotAreaByName(c.args[0])
	if area == nil {
		ps.sendCaption(c.sender, "errors.not_valid_plot_world", "value", c.args[0])
		return
	}
	c.sender.SendMessage(lang.ParseMiniMessage(fmt.Sprintf(
		"<gold>[Plots]</gold> <gray>World <gold>%s</gold> has <gold>%d</gold> owned plot(s).</gray>",
		area.WorldName, len(area.BasePlots()),
	)))
}

func (ps *Plugin) cmdReload(c *cmdCtx) {
	if err := ps.Reload(); err != nil {
		ps.sendCaption(c.sender, "reload.reload_failed")
		return
	}
	ps.sendCaption(c.sender, "reload.reloaded_configs")
}

func (ps *Plugin) cmdPlugin(c *cmdCtx) {
	totalPlots := 0
	areas := ps.PlotAreas()
	for _, a := range areas {
		totalPlots += len(a.BasePlots())
	}
	c.sender.SendMessage(lang.ParseMiniMessage("<gray>>> </gray><gold><bold>Plots</bold></gold> <gray>(Ported from PlotSquared for golem)</gray>"))
	c.sender.SendMessage(lang.ParseMiniMessage(fmt.Sprintf("<gray>>> </gray><gold>Areas:</gold> <gray>%d</gray> <gold>Plots:</gold> <gray>%d</gray>", len(areas), totalPlots)))
}

func (ps *Plugin) cmdDebug(c *cmdCtx) {
	areas := ps.PlotAreas()
	totalPlots := 0
	for _, a := range areas {
		totalPlots += len(a.BasePlots())
	}
	_, hasEco := ps.Economy()
	c.sender.SendMessage(lang.ParseMiniMessage(fmt.Sprintf(
		"<gold>[Plots Debug]</gold> <gray>areas=%d plots=%d db=%t economy=%t expiry=%t</gray>",
		len(areas), totalPlots, ps.DB != nil, hasEco, ps.Settings != nil && ps.Settings.EnabledComponents.PlotExpiry,
	)))
}

func (ps *Plugin) startExpiryTask() {
	if ps.Expiry == nil || ps.Settings == nil {
		return
	}
	plugin.Listen(ps.Server, func(e *plugin.PlayerJoinEvent) {
		ps.Expiry.TouchPlayer(e.Player.UUID(), ps.now())
	})
	plugin.Listen(ps.Server, func(e *plugin.PlayerQuitEvent) {
		ps.Expiry.TouchPlayer(e.Player.UUID(), ps.now())
	})
	if !ps.Settings.EnabledComponents.PlotExpiry {
		return
	}
	ps.startExpiryTimer()
}

// startExpiryTimer schedules the periodic expiry pass (every 5 minutes) and
// reports false when it is already running.
func (ps *Plugin) startExpiryTimer() bool {
	ps.dbgMu.Lock()
	defer ps.dbgMu.Unlock()
	if ps.expiryTask != nil && !ps.expiryTask.Cancelled() {
		return false
	}
	ps.expiryTask = ps.Server.Scheduler().RunTimer(6000, 6000, func() {
		ps.RunExpiryPass()
	})
	return true
}

// stopExpiryTimer cancels the periodic expiry pass and reports whether one was running.
func (ps *Plugin) stopExpiryTimer() bool {
	ps.dbgMu.Lock()
	defer ps.dbgMu.Unlock()
	if ps.expiryTask == nil || ps.expiryTask.Cancelled() {
		return false
	}
	ps.expiryTask.Cancel()
	ps.expiryTask = nil
	return true
}

// RunExpiryPass clears plots that are expired under settings.yml auto_clear tasks.
func (ps *Plugin) RunExpiryPass() int {
	if ps.Expiry == nil || ps.Settings == nil || len(ps.Settings.AutoClear) == 0 {
		return 0
	}
	expired := ps.Expiry.ExpiredPlots(ps.PlotAreas(), ps.Settings.AutoClear, ps.now())
	cleared := 0
	for _, pl := range expired {
		// Respect complexity threshold if positive
		skip := false
		for _, task := range ps.Settings.AutoClear {
			if task.Threshold > 0 && ps.Expiry.IsPlotExpired(pl, task, ps.now()) {
				if w, ok := ps.WrapWorld(pl.Area.WorldName); ok {
					an := expiry.AnalyzePlot(w, pl.Area, pl)
					if an.Complexity(task.Calibration) >= task.Threshold {
						skip = true
					}
				}
			}
		}
		if skip {
			continue
		}
		if err := ps.DeletePlot(pl, true); err == nil {
			cleared++
		}
	}
	return cleared
}
