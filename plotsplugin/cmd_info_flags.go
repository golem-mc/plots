package plotsplugin

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/lang"
)

var musicDiscItems = []struct {
	name string
	item registry.Item
}{
	{"music_disc_13", registry.ItemMusicDisc13},
	{"music_disc_cat", registry.ItemMusicDiscCat},
	{"music_disc_blocks", registry.ItemMusicDiscBlocks},
	{"music_disc_chirp", registry.ItemMusicDiscChirp},
	{"music_disc_far", registry.ItemMusicDiscFar},
	{"music_disc_mall", registry.ItemMusicDiscMall},
	{"music_disc_mellohi", registry.ItemMusicDiscMellohi},
	{"music_disc_stal", registry.ItemMusicDiscStal},
	{"music_disc_strad", registry.ItemMusicDiscStrad},
	{"music_disc_ward", registry.ItemMusicDiscWard},
	{"music_disc_11", registry.ItemMusicDisc11},
	{"music_disc_wait", registry.ItemMusicDiscWait},
	{"music_disc_otherside", registry.ItemMusicDiscOtherside},
	{"music_disc_pigstep", registry.ItemMusicDiscPigstep},
	{"music_disc_5", registry.ItemMusicDisc5},
	{"music_disc_relic", registry.ItemMusicDiscRelic},
	{"music_disc_creator", registry.ItemMusicDiscCreator},
	{"music_disc_creator_music_box", registry.ItemMusicDiscCreatorMusicBox},
	{"music_disc_precipice", registry.ItemMusicDiscPrecipice},
}

type musicMenuHolder struct {
	plot *plots.Plot
}

func (ps *Plugin) registerMusicInventoryListener() {
	plugin.Listen(ps.Server, func(e *plugin.InventoryClickEvent) {
		if e.View == nil || e.View.Top() == nil {
			return
		}
		holder, ok := e.View.Top().Holder().(*musicMenuHolder)
		if !ok {
			return
		}
		e.SetCancelled(true)
		if e.RawSlot < 0 || e.RawSlot >= e.View.Top().Size() {
			return
		}
		st := e.View.Top().Item(e.RawSlot)
		if st.Empty() {
			return
		}
		if st.Item == registry.ItemBedrock {
			_, _ = ps.RemoveFlag(holder.plot, "music")
			ps.sendCaption(e.Player, "flag.flag_removed", "flag", "music", "value", "music_disc")
			e.Player.CloseInventory()
			return
		}
		for _, d := range musicDiscItems {
			if d.item == st.Item {
				_, _ = ps.SetFlag(holder.plot, "music", d.name)
				ps.sendCaption(e.Player, "flag.flag_added", "flag", "music", "value", d.name)
				e.Player.CloseInventory()
				return
			}
		}
	})
}

func (ps *Plugin) registerInfoAndFlagCommands() {
	ps.addSubCommand(&subCommand{
		name:     "info",
		aliases:  []string{"i"},
		perm:     "plots.info",
		usage:    "/plot info [id] [category] [-f]",
		desc:     "Display plot information",
		category: catInfo,
		run:      ps.cmdInfo,
		tab: func(c *cmdCtx) []string {
			opts := []string{"trusted", "alias", "biome", "denied", "flags", "id", "size", "members", "creationdate", "seen", "owner", "rating", "likes", "-f"}
			return filterPrefix(opts, lastArg(c.args))
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "list",
		aliases:  []string{"l", "find", "search"},
		perm:     "plots.list",
		usage:    "/plot list <forsale | mine | shared | world | area | top | all | unowned | done | fuzzy <term> | player> [#]",
		desc:     "List plots",
		category: catInfo,
		run:      ps.cmdList,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				opts := append([]string{"mine", "shared", "world", "area", "all", "top", "forsale", "unowned", "done", "fuzzy"}, ps.onlinePlayerNames()...)
				return filterPrefix(opts, lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "near",
		aliases:  []string{"n"},
		perm:     "plots.use",
		usage:    "/plot near",
		desc:     "List players in the current plot",
		category: catInfo,
		required: reqPlayer,
		run:      ps.cmdNear,
	})

	ps.addSubCommand(&subCommand{
		name:     "caps",
		perm:     "plots.use",
		usage:    "/plot caps",
		desc:     "Display entity caps for the current plot",
		category: catInfo,
		required: reqPlayer,
		run:      ps.cmdCaps,
	})

	ps.addSubCommand(&subCommand{
		name:     "set",
		aliases:  []string{"s"},
		perm:     "plots.set",
		usage:    "/plot set <biome | alias | home | flag | desc | floor | wall | border | all | air | outline | middle | main> <value...>",
		desc:     "Set plot components or properties",
		category: catAppearance,
		run:      ps.cmdSet,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				opts := []string{"biome", "alias", "home", "flag", "desc", "owner", "floor", "wall", "border", "all", "air", "outline", "middle", "main"}
				return filterPrefix(opts, c.args[0])
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "biome",
		aliases:  []string{"setbiome", "sb", "setb", "b"},
		perm:     "plots.set.biome",
		usage:    "/plot biome [biome]",
		desc:     "Set the biome of the current plot",
		category: catAppearance,
		run:      ps.cmdBiome,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix([]string{"plains", "forest", "desert", "taiga", "jungle", "swamp", "savanna", "badlands", "snowy_plains", "cherry_grove", "meadow", "ocean", "mushroom_fields"}, lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "alias",
		aliases:  []string{"setalias", "sa", "name", "rename", "setname", "seta", "nameplot"},
		perm:     "plots.alias",
		usage:    "/plot alias <set | remove> <alias>",
		desc:     "Set or remove the plot alias",
		category: catSettings,
		required: reqPlayer,
		run:      ps.cmdAlias,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix([]string{"set", "remove"}, lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "desc",
		aliases:  []string{"setdescription", "setdesc", "setd", "description"},
		perm:     "plots.set.desc",
		usage:    "/plot desc [description]",
		desc:     "Set or clear the plot description",
		category: catSettings,
		required: reqPlayer,
		run:      ps.cmdDesc,
	})

	ps.addSubCommand(&subCommand{
		name:     "flag",
		aliases:  []string{"f"},
		perm:     "plots.flag",
		usage:    "/plot flag <set | add | remove | list | info> <flag> [value]",
		desc:     "Manage plot flags",
		category: catSettings,
		run:      ps.cmdFlag,
		tab:      ps.tabFlag,
	})

	ps.addSubCommand(&subCommand{
		name:     "music",
		perm:     "plots.music",
		usage:    "/plot music [disc | none]",
		desc:     "Play music in your plot",
		category: catAppearance,
		required: reqPlayer,
		run:      ps.cmdMusic,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				out := make([]string, 0, len(musicDiscItems)+1)
				out = append(out, "none")
				for _, d := range musicDiscItems {
					out = append(out, d.name)
				}
				return filterPrefix(out, lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "toggle",
		aliases:  []string{"attribute"},
		perm:     "plots.toggle",
		usage:    "/plot toggle <chat | chatspy | clear-confirmation | time | titles | worldedit | debug>",
		desc:     "Toggle personal plot settings",
		category: catSettings,
		required: reqPlayer,
		run:      ps.cmdToggle,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix([]string{"chat", "chatspy", "clear-confirmation", "time", "titles", "worldedit", "debug"}, lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "chat",
		perm:     "plots.chat",
		usage:    "/plot chat [message]",
		desc:     "Toggle plot chat or send a message to plot chat",
		category: catChat,
		required: reqPlayer,
		run:      ps.cmdChat,
	})
}

func (ps *Plugin) cmdInfo(c *cmdCtx) {
	var plot *plots.Plot
	var category string
	force := false

	filtered := make([]string, 0, len(c.args))
	for _, a := range c.args {
		if strings.EqualFold(a, "-f") {
			force = true
		} else {
			filtered = append(filtered, a)
		}
	}

	infoCats := map[string]string{
		"trusted":      "info.plot_info_trusted",
		"alias":        "info.plot_info_alias",
		"biome":        "info.plot_info_biome",
		"denied":       "info.plot_info_denied",
		"flags":        "info.plot_info_flags",
		"id":           "info.plot_info_id",
		"size":         "info.plot_info_size",
		"members":      "info.plot_info_members",
		"creationdate": "info.plot_info_creationdate",
		"seen":         "info.plot_info_seen",
		"owner":        "info.plot_info_owner",
		"rating":       "info.plot_info_rating",
		"likes":        "info.plot_info_likes",
	}

	if len(filtered) >= 1 {
		if _, ok := infoCats[strings.ToLower(filtered[0])]; ok {
			category = strings.ToLower(filtered[0])
		} else {
			plot = ps.resolvePlotFromArg(c, filtered[0])
			if len(filtered) >= 2 {
				category = strings.ToLower(filtered[1])
			}
		}
	}
	if plot == nil {
		plot = ps.currentPlot(c)
	}
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}

	base := plot.BasePlot()
	if base.FlagBool("hide-info") {
		if !force {
			ps.sendCaption(c.sender, "info.plot_info_hidden")
			return
		}
		if !HasPerm(c.sender, "plots.admin.info.force") {
			ps.sendNoPerm(c.sender, "plots.admin.info.force")
			return
		}
	}
	if !base.HasOwner() {
		ps.sendCaption(c.sender, "info.plot_info_unclaimed", "plot", base.ID.String())
		return
	}

	ownerName := ps.PlayerName(base.Owner)
	if base.FlagBool("server-plot") {
		ownerName = "Server"
	}
	alias := base.Settings.Alias
	if alias == "" {
		alias = "None"
	}
	desc := base.FlagString("description")
	if desc == "" {
		desc = "None"
	}
	canBuild := "false"
	if c.player != nil && (base.IsOwner(c.player.UUID()) || base.IsAdded(c.player.UUID()) || HasPerm(c.player, "plots.admin.build.other")) {
		canBuild = "true"
	}

	var flagPairs []string
	for k, v := range base.Flags.LocalFlags() {
		if def := plots.LookupFlag(k); def != nil {
			flagPairs = append(flagPairs, k+": "+def.Format(v))
		}
	}
	sort.Strings(flagPairs)
	flagStr := "None"
	if len(flagPairs) > 0 {
		flagStr = strings.Join(flagPairs, ", ")
	}

	avgRating, _ := base.AverageRating()
	ratingStr := fmt.Sprintf("%.1f/10", avgRating)
	likes, dislikes := base.LikeCount()
	likePct := 0.0
	if likes+dislikes > 0 {
		likePct = 100 * float64(likes) / float64(likes+dislikes)
	}
	likesStr := fmt.Sprintf("%.0f%%", likePct)
	seenStr := "online"
	if _, online := ps.Server.Player(base.Owner); !online {
		seenStr = "offline"
	}

	kv := []string{
		"header", ps.Lang.Raw("info.plot_info_header"),
		"footer", ps.Lang.Raw("info.plot_info_footer"),
		"id", base.ID.String(),
		"creationdate", formatTimeAgo(base.Timestamp),
		"area", base.Area.FullID(),
		"alias", alias,
		"owner", ownerName,
		"biome", base.Area.PlotBiome,
		"build", canBuild,
		"rating", ratingStr,
		"likes", likesStr,
		"seen", seenStr,
		"size", strconv.Itoa(len(base.ConnectedPlots())),
		"trusted", ps.formatUUIDList(base.Trusted),
		"members", ps.formatUUIDList(base.Members),
		"denied", ps.formatUUIDList(base.Denied),
		"flags", flagStr,
		"desc", desc,
	}

	if category != "" {
		if capKey, ok := infoCats[category]; ok {
			ps.sendCaption(c.sender, capKey, kv...)
			return
		}
		if ps.Settings != nil && ps.Settings.Ratings.UseLikes {
			ps.sendCaption(c.sender, "info.plot_info_categories.use_likes")
		} else {
			ps.sendCaption(c.sender, "info.plot_info_categories.use_rating")
		}
		return
	}
	ps.sendCaption(c.sender, "info.plot_info_format", kv...)
}

func (ps *Plugin) cmdList(c *cmdCtx) {
	if len(c.args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.subcommand_set_options_header",
			"values", "[mine, shared, world, area, all, top, forsale, unowned, done, fuzzy, <player>]")
		return
	}
	arg0 := strings.ToLower(c.args[0])
	page := 1
	if len(c.args) > 1 {
		if n, err := strconv.Atoi(c.args[len(c.args)-1]); err == nil && n >= 1 {
			page = n
		}
	}

	var matched []*plots.Plot
	switch arg0 {
	case "mine":
		if !HasPerm(c.sender, "plots.list.mine") {
			ps.sendNoPerm(c.sender, "plots.list.mine")
			return
		}
		if c.player != nil {
			matched = ps.PlotsByOwner(c.player.UUID())
		}
	case "shared":
		if !HasPerm(c.sender, "plots.list.shared") {
			ps.sendNoPerm(c.sender, "plots.list.shared")
			return
		}
		if c.player != nil {
			for _, a := range ps.PlotAreas() {
				for _, pl := range a.BasePlots() {
					if !pl.IsOwner(c.player.UUID()) && pl.IsAdded(c.player.UUID()) {
						matched = append(matched, pl)
					}
				}
			}
		}
	case "world", "area":
		perm := "plots.list." + arg0
		if !HasPerm(c.sender, perm) {
			ps.sendNoPerm(c.sender, perm)
			return
		}
		if area := ps.currentArea(c); area != nil {
			if arg0 == "area" {
				matched = area.BasePlots()
			} else {
				for _, a := range ps.PlotAreasInWorld(area.WorldName) {
					matched = append(matched, a.BasePlots()...)
				}
			}
		}
	case "all":
		if !HasPerm(c.sender, "plots.list.all") {
			ps.sendNoPerm(c.sender, "plots.list.all")
			return
		}
		for _, a := range ps.PlotAreas() {
			matched = append(matched, a.BasePlots()...)
		}
	case "top":
		if !HasPerm(c.sender, "plots.list.top") {
			ps.sendNoPerm(c.sender, "plots.list.top")
			return
		}
		for _, a := range ps.PlotAreas() {
			matched = append(matched, a.BasePlots()...)
		}
		sort.Slice(matched, func(i, j int) bool {
			ai, _ := matched[i].AverageRating()
			aj, _ := matched[j].AverageRating()
			return ai > aj
		})
	case "forsale":
		if !HasPerm(c.sender, "plots.list.forsale") {
			ps.sendNoPerm(c.sender, "plots.list.forsale")
			return
		}
		for _, a := range ps.PlotAreas() {
			for _, pl := range a.BasePlots() {
				if price, _ := pl.FlagValue("price").(float64); price > 0 {
					matched = append(matched, pl)
				}
			}
		}
	case "done":
		if !HasPerm(c.sender, "plots.list.done") {
			ps.sendNoPerm(c.sender, "plots.list.done")
			return
		}
		for _, a := range ps.PlotAreas() {
			for _, pl := range a.BasePlots() {
				if pl.FlagString("done") != "" {
					matched = append(matched, pl)
				}
			}
		}
	case "unowned":
		if !HasPerm(c.sender, "plots.list.unowned") {
			ps.sendNoPerm(c.sender, "plots.list.unowned")
			return
		}
		for _, a := range ps.PlotAreas() {
			for _, pl := range a.BasePlots() {
				if !pl.HasOwner() {
					matched = append(matched, pl)
				}
			}
		}
	case "fuzzy":
		if !HasPerm(c.sender, "plots.list.fuzzy") {
			ps.sendNoPerm(c.sender, "plots.list.fuzzy")
			return
		}
		if len(c.args) < 2 {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot list fuzzy <search...> [#]")
			return
		}
		endIdx := len(c.args)
		if _, err := strconv.Atoi(c.args[endIdx-1]); err == nil && endIdx > 2 {
			endIdx--
		}
		term := strings.ToLower(strings.Join(c.args[1:endIdx], " "))
		for _, a := range ps.PlotAreas() {
			for _, pl := range a.BasePlots() {
				if strings.Contains(strings.ToLower(pl.Settings.Alias), term) ||
					strings.Contains(strings.ToLower(pl.FlagString("description")), term) ||
					strings.Contains(strings.ToLower(ps.PlayerName(pl.Owner)), term) ||
					strings.Contains(pl.ID.String(), term) {
					matched = append(matched, pl)
				}
			}
		}
	default:
		if area := ps.PlotAreaByName(c.args[0]); area != nil {
			if !HasPerm(c.sender, "plots.list.world") {
				ps.sendNoPerm(c.sender, "plots.list.world")
				return
			}
			matched = area.BasePlots()
		} else if uid, ok := ps.ResolvePlayerUUID(c.args[0]); ok {
			if !HasPerm(c.sender, "plots.list.player") {
				ps.sendNoPerm(c.sender, "plots.list.player")
				return
			}
			matched = ps.PlotsByOwner(uid)
		} else {
			ps.sendCaption(c.sender, "errors.invalid_player", "value", c.args[0])
			return
		}
	}

	if len(matched) == 0 {
		ps.sendCaption(c.sender, "invalid.found_no_plots")
		return
	}
	pageSize := 12
	maxPages := max((len(matched)+pageSize-1)/pageSize, 1)
	if page > maxPages {
		page = maxPages
	}
	ps.sendCaption(c.sender, "list.plot_list_header_paged",
		"cur", strconv.Itoa(page),
		"max", strconv.Itoa(maxPages),
		"amount", strconv.Itoa(len(matched)),
		"word", arg0,
	)
	start := (page - 1) * pageSize
	end := min(start+pageSize, len(matched))
	for i := start; i < end; i++ {
		pl := matched[i]
		ownerName := ps.PlayerName(pl.Owner)
		if pl.FlagBool("server-plot") {
			ownerName = "Server"
		}
		line := fmt.Sprintf("<gray>[<gold>%d</gold>] <gold>%s;%s</gold> - <gray>%s</gray> (%d)</gray>",
			i+1, pl.Area.FullID(), pl.ID.String(), ownerName, len(pl.ConnectedPlots()))
		c.sender.SendMessage(lang.ParseMiniMessage(line))
	}
}

func (ps *Plugin) cmdNear(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	pls := ps.playersInPlot(plot)
	names := make([]string, 0, len(pls))
	for _, pl := range pls {
		names = append(names, pl.Name())
	}
	sort.Strings(names)
	ps.sendCaption(c.sender, "near.plot_near", "list", strings.Join(names, ", "))
}

func (ps *Plugin) cmdCaps(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.IsAdded(c.player.UUID()) && !HasPerm(c.sender, "plots.admin.caps.other") {
		ps.sendNoPerm(c.sender, "plots.admin.caps.other")
		return
	}
	w, ok := ps.WrapWorld(plot.Area.WorldName)
	if !ok {
		return
	}
	counts := ps.CountPlotEntities(w, plot.Area, plot.BasePlot())
	ps.sendCaption(c.sender, "info.plot_caps_header")
	sendCap := func(name string, cur int, flagName string) {
		lim := plot.FlagInt(flagName)
		limStr := strconv.Itoa(lim)
		pct := "0.0"
		if lim >= math.MaxInt32 || lim <= 0 {
			limStr = "infinite"
		} else {
			pct = fmt.Sprintf("%.1f", 100*float64(cur)/float64(lim))
		}
		ps.sendCaption(c.sender, "info.plot_caps_format",
			"cap", name,
			"current", strconv.Itoa(cur),
			"limit", limStr,
			"percentage", pct,
		)
	}
	sendCap("mobs", counts.Mobs, "mob-cap")
	sendCap("hostile", counts.Hostiles, "hostile-cap")
	sendCap("animals", counts.Animals, "animal-cap")
	sendCap("vehicle", counts.Vehicles, "vehicle-cap")
	sendCap("misc", counts.Misc, "misc-cap")
	sendCap("entities", counts.Total, "entity-cap")
}

func (ps *Plugin) cmdSet(c *cmdCtx) {
	if len(c.args) == 0 {
		ps.sendCaption(c.sender, "commandconfig.subcommand_set_options_header",
			"values", "biome, alias, home, flag, desc, owner, floor, wall, border, all, air, outline, middle, main")
		return
	}
	sub := strings.ToLower(c.args[0])
	rest := c.args[1:]
	subCtx := *c
	subCtx.args = rest

	switch sub {
	case "biome", "b":
		ps.cmdBiome(&subCtx)
		return
	case "alias", "a":
		if len(rest) == 1 {
			subCtx.args = []string{"set", rest[0]}
		}
		ps.cmdAlias(&subCtx)
		return
	case "home", "h":
		ps.cmdSetHome(&subCtx)
		return
	case "flag", "f":
		if len(rest) >= 2 {
			subCtx.args = append([]string{"set"}, rest...)
		}
		ps.cmdFlag(&subCtx)
		return
	case "desc", "description":
		ps.cmdDesc(&subCtx)
		return
	case "owner":
		if !HasPerm(c.sender, "plots.admin.command.setowner") {
			ps.sendNoPerm(c.sender, "plots.admin.command.setowner")
			return
		}
		ps.cmdSetOwner(&subCtx)
		return
	}

	comp, ok := parsePlotComponent(sub)
	if !ok {
		ps.sendCaption(c.sender, "commandconfig.subcommand_set_options_header",
			"values", "biome, alias, home, flag, desc, floor, wall, border, all, air, outline, middle, main")
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
	if c.player != nil && !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	perm := "plots.set." + string(comp)
	if !HasPerm(c.sender, perm) {
		ps.sendNoPerm(c.sender, perm)
		return
	}
	if len(rest) == 0 {
		ps.sendCaption(c.sender, "need.need_block")
		return
	}
	pattern := strings.Join(rest, ",")
	if !HasPerm(c.sender, "plots.admin.unsafe") && ps.Settings != nil {
		for _, forbidden := range ps.Settings.General.InvalidBlocks {
			lowF := strings.TrimPrefix(strings.ToLower(forbidden), "minecraft:")
			if lowF != "" && strings.Contains(strings.ToLower(pattern), lowF) {
				ps.sendCaption(c.sender, "invalid.component_illegal_block", "value", forbidden)
				return
			}
		}
	}
	if err := ps.SetComponent(plot, comp, pattern); err != nil {
		ps.sendCaption(c.sender, "invalid.not_valid_block", "value", pattern)
		return
	}
	ps.sendCaption(c.sender, "working.component_complete", "plot", plot.ID.String())
}

func parsePlotComponent(s string) (plots.PlotComponent, bool) {
	switch s {
	case "floor":
		return plots.ComponentFloor, true
	case "wall", "w":
		return plots.ComponentWall, true
	case "border":
		return plots.ComponentBorder, true
	case "all":
		return plots.ComponentAll, true
	case "air":
		return plots.ComponentAir, true
	case "main", "filling":
		return plots.ComponentMain, true
	case "middle":
		return plots.ComponentMiddle, true
	case "outline":
		return plots.ComponentOutline, true
	default:
		return "", false
	}
}

func (ps *Plugin) cmdBiome(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "info.plot_unowned")
		return
	}
	if c.player != nil && !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	if len(c.args) < 1 {
		ps.sendCaption(c.sender, "biome.need_biome")
		return
	}
	biome := strings.ToLower(c.args[0])
	if err := ps.SetBiome(plot, biome); err != nil {
		ps.sendCaption(c.sender, "biome.need_biome")
		return
	}
	ps.sendCaption(c.sender, "biome.biome_set_to", "value", biome)
}

func (ps *Plugin) cmdAlias(c *cmdCtx) {
	if len(c.args) == 0 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot alias <set | remove> <alias>")
		return
	}
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "working.plot_not_claimed")
		return
	}
	base := plot.BasePlot()
	isOwner := base.IsOwner(c.player.UUID())
	switch strings.ToLower(c.args[0]) {
	case "set":
		if len(c.args) != 2 {
			ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot alias <set | remove> <alias>")
			return
		}
		if !isOwner && !HasPerm(c.sender, "plots.admin.alias.set") {
			ps.sendCaption(c.sender, "permission.no_plot_perms")
			return
		}
		alias := c.args[1]
		if len(alias) >= 50 {
			ps.sendCaption(c.sender, "alias.alias_too_long")
			return
		}
		if _, err := strconv.Atoi(alias); err == nil {
			ps.sendCaption(c.sender, "flag.not_valid_value")
			return
		}
		for _, other := range base.Area.BasePlots() {
			if other != base && strings.EqualFold(other.Settings.Alias, alias) {
				ps.sendCaption(c.sender, "alias.alias_is_taken", "alias", alias)
				return
			}
		}
		base.Settings.Alias = alias
		if ps.DB != nil {
			_ = ps.DB.SetAlias(base, alias)
		}
		ps.sendCaption(c.sender, "alias.alias_set_to", "alias", alias)
	case "remove":
		if !isOwner && !HasPerm(c.sender, "plots.admin.alias.remove") {
			ps.sendCaption(c.sender, "permission.no_plot_perms")
			return
		}
		old := base.Settings.Alias
		if old == "" {
			ps.sendCaption(c.sender, "alias.no_alias_set")
			return
		}
		base.Settings.Alias = ""
		if ps.DB != nil {
			_ = ps.DB.SetAlias(base, "")
		}
		ps.sendCaption(c.sender, "alias.alias_removed", "alias", old)
	default:
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot alias <set | remove> <alias>")
	}
}

func (ps *Plugin) cmdDesc(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "info.plot_unowned")
		return
	}
	if !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	if len(c.args) == 0 {
		_, _ = ps.RemoveFlag(plot, "description")
		ps.sendCaption(c.sender, "desc.desc_unset")
		return
	}
	desc := strings.Join(c.args, " ")
	if _, err := ps.SetFlag(plot, "description", desc); err != nil {
		ps.sendCaption(c.sender, "flag.flag_not_added")
		return
	}
	ps.sendCaption(c.sender, "desc.desc_set")
}

func (ps *Plugin) tabFlag(c *cmdCtx) []string {
	if len(c.args) == 1 {
		return filterPrefix([]string{"set", "add", "remove", "list", "info"}, c.args[0])
	}
	action := strings.ToLower(c.args[0])
	if len(c.args) == 2 && (action == "set" || action == "add" || action == "remove" || action == "delete" || action == "info") {
		all := plots.AllFlags()
		names := make([]string, 0, len(all))
		for _, f := range all {
			if !f.Internal {
				names = append(names, f.Name)
			}
		}
		return filterPrefix(names, c.args[1])
	}
	if len(c.args) == 3 && (action == "set" || action == "add" || action == "remove" || action == "delete") {
		if def := plots.LookupFlag(c.args[1]); def != nil && len(def.TabComplete) > 0 {
			return filterPrefix(def.TabComplete, c.args[2])
		}
	}
	return nil
}

func (ps *Plugin) cmdFlag(c *cmdCtx) {
	if len(c.args) == 0 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot flag <set | add | remove | list | info> <flag> [value]")
		return
	}
	action := strings.ToLower(c.args[0])
	rest := c.args[1:]
	switch action {
	case "list", "l", "flags":
		ps.cmdFlagList(c)
	case "info", "i":
		ps.cmdFlagInfo(c, rest)
	case "set", "s":
		ps.cmdFlagSetOrAdd(c, rest, false)
	case "add", "a":
		ps.cmdFlagSetOrAdd(c, rest, true)
	case "remove", "r", "delete":
		ps.cmdFlagRemove(c, rest)
	default:
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot flag <set | add | remove | list | info> <flag> [value]")
	}
}

func (ps *Plugin) lookupFlagWithSuggestion(sender plugin.CommandSender, name string) *plots.FlagDef {
	def := plots.LookupFlag(name)
	if def != nil && !def.Internal {
		return def
	}
	all := plots.AllFlags()
	names := make([]string, 0, len(all))
	for _, f := range all {
		if !f.Internal {
			names = append(names, f.Name)
		}
	}
	if best := closestMatch(name, names); best != "" {
		ps.sendCaption(sender, "flag.not_valid_flag_suggested", "value", best)
	} else {
		ps.sendCaption(sender, "flag.not_valid_flag")
	}
	return nil
}

func (ps *Plugin) checkFlagPlotOwner(c *cmdCtx, otherPerm string) *plots.Plot {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return nil
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "working.plot_not_claimed")
		return nil
	}
	if c.player != nil && !plot.IsOwner(c.player.UUID()) && !HasPerm(c.sender, otherPerm) {
		ps.sendNoPerm(c.sender, otherPerm)
		return nil
	}
	return plot
}

func (ps *Plugin) cmdFlagSetOrAdd(c *cmdCtx, args []string, merge bool) {
	plot := ps.checkFlagPlotOwner(c, "plots.set.flag.other")
	if plot == nil {
		return
	}
	if len(args) < 2 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot flag set <flag> <value>")
		return
	}
	def := ps.lookupFlagWithSuggestion(c.sender, args[0])
	if def == nil {
		return
	}
	rawVal := strings.Join(args[1:], " ")
	permNode := "plots.set.flag." + def.Name + "." + strings.ToLower(rawVal)
	if !HasPerm(c.sender, "plots.set.flag."+def.Name) && !HasPerm(c.sender, permNode) && !HasPerm(c.sender, "plots.set.flag.*") {
		ps.sendNoPerm(c.sender, permNode)
		return
	}
	rawVal = lang.StripClickEvents(rawVal, []string{"run_command", "suggest_command", "open_url", "open_file"})
	if merge && (def.Kind == plots.FlagStringList || def.Kind == plots.FlagBlockList) {
		existing := flagStringSlice(plot.BasePlot(), def.Name)
		parsed, err := def.Parse(rawVal)
		if err != nil {
			ps.sendCaption(c.sender, "flag.flag_parse_error", "flag_name", def.Name, "flag_value", rawVal, "error", err.Error())
			return
		}
		combined := append([]string(nil), existing...)
		if addList, ok := parsed.([]string); ok {
			for _, item := range addList {
				if !slices.Contains(combined, item) {
					combined = append(combined, item)
				}
			}
		}
		rawVal = strings.Join(combined, ",")
	}
	val, err := ps.SetFlag(plot, def.Name, rawVal)
	if err != nil {
		ps.sendCaption(c.sender, "flag.flag_parse_error", "flag_name", def.Name, "flag_value", rawVal, "error", err.Error())
		return
	}
	ps.sendCaption(c.sender, "flag.flag_added", "flag", def.Name, "value", def.Format(val))
}

func (ps *Plugin) cmdFlagRemove(c *cmdCtx, args []string) {
	plot := ps.checkFlagPlotOwner(c, "plots.set.flag.other")
	if plot == nil {
		return
	}
	if len(args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot flag remove <flag> [values]")
		return
	}
	def := ps.lookupFlagWithSuggestion(c.sender, args[0])
	if def == nil {
		return
	}
	if len(args) >= 2 && (def.Kind == plots.FlagStringList || def.Kind == plots.FlagBlockList) {
		removeVal := strings.Join(args[1:], " ")
		parsed, err := def.Parse(removeVal)
		if err != nil {
			ps.sendCaption(c.sender, "flag.flag_parse_error", "flag_name", def.Name, "flag_value", removeVal, "error", err.Error())
			return
		}
		toRemove, _ := parsed.([]string)
		existing := flagStringSlice(plot.BasePlot(), def.Name)
		var kept []string
		removedAny := false
		for _, item := range existing {
			if slices.Contains(toRemove, item) {
				removedAny = true
			} else {
				kept = append(kept, item)
			}
		}
		if !removedAny {
			ps.sendCaption(c.sender, "flag.flag_not_removed")
			return
		}
		if len(kept) == 0 {
			_, _ = ps.RemoveFlag(plot, def.Name)
			ps.sendCaption(c.sender, "flag.flag_removed", "flag", def.Name, "value", removeVal)
			return
		}
		_, _ = ps.SetFlag(plot, def.Name, strings.Join(kept, ","))
		ps.sendCaption(c.sender, "flag.flag_partially_removed")
		return
	}
	removed, err := ps.RemoveFlag(plot, def.Name)
	if err != nil || !removed {
		ps.sendCaption(c.sender, "flag.flag_not_removed")
		return
	}
	ps.sendCaption(c.sender, "flag.flag_removed", "flag", def.Name, "value", def.Format(def.Default))
}

func flagStringSlice(plot *plots.Plot, name string) []string {
	if plot == nil {
		return nil
	}
	v, _ := plot.FlagValue(name).([]string)
	return v
}

func (ps *Plugin) cmdFlagList(c *cmdCtx) {
	byCat := make(map[string][]string)
	for _, f := range plots.AllFlags() {
		if f.Internal {
			continue
		}
		cat := string(f.Category)
		if cat == "" {
			cat = "misc"
		}
		byCat[cat] = append(byCat[cat], f.Name)
	}
	cats := make([]string, 0, len(byCat))
	for cat := range byCat {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	for _, cat := range cats {
		names := byCat[cat]
		sort.Strings(names)
		line := fmt.Sprintf("<gold>%s:</gold> <gray>%s</gray>", cat, strings.Join(names, ", "))
		c.sender.SendMessage(lang.ParseMiniMessage(line))
	}
}

func (ps *Plugin) cmdFlagInfo(c *cmdCtx, args []string) {
	if len(args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot flag info <flag>")
		return
	}
	def := ps.lookupFlagWithSuggestion(c.sender, args[0])
	if def == nil {
		return
	}
	ps.sendCaption(c.sender, "flag.flag_info_header")
	ps.sendCaption(c.sender, "flag.flag_info_name", "flag", def.Name)
	ps.sendCaption(c.sender, "flag.flag_info_category", "value", string(def.Category))
	ps.sendCaption(c.sender, "flag.flag_info_description")
	c.sender.SendMessage(lang.ParseMiniMessage("<gray>" + def.Description + "</gray>"))
	ps.sendCaption(c.sender, "flag.flag_info_example", "command", "/plot flag set", "flag", def.Name, "value", def.Example)
	ps.sendCaption(c.sender, "flag.flag_info_default_value", "value", def.Format(def.Default))
	ps.sendCaption(c.sender, "flag.flag_info_footer")
}

func (ps *Plugin) cmdMusic(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(c.sender, "info.plot_unowned")
		return
	}
	if !plot.IsAdded(c.player.UUID()) && !HasPerm(c.sender, "plots.admin.music.other") {
		ps.sendNoPerm(c.sender, "plots.admin.music.other")
		return
	}
	if len(c.args) >= 1 {
		arg := strings.ToLower(c.args[0])
		if arg == "none" || arg == "off" || arg == "stop" || arg == "cancel" {
			_, _ = ps.RemoveFlag(plot, "music")
			ps.sendCaption(c.sender, "flag.flag_removed", "flag", "music", "value", "music_disc")
			return
		}
		val, err := ps.SetFlag(plot, "music", arg)
		if err != nil {
			ps.sendCaption(c.sender, "flag.flag_not_added")
			return
		}
		ps.sendCaption(c.sender, "flag.flag_added", "flag", "music", "value", fmt.Sprint(val))
		return
	}
	rows := (len(musicDiscItems) + 1 + 8) / 9
	title := ps.Lang.Format("plotjukebox.jukebox_header")
	inv := ps.Server.CreateInventory(plugin.InvChest, rows, title, &musicMenuHolder{plot: plot.BasePlot()})
	for i, d := range musicDiscItems {
		name := lang.ParseMiniMessage("<gold>" + d.name + "</gold>")
		inv.SetItem(i, plugin.ItemStack{
			Item:  d.item,
			Count: 1,
			Patch: plugin.ItemPatch{CustomName: &name},
		})
	}
	cancelName := ps.Lang.Format("plotjukebox.cancel_music")
	inv.SetItem(len(musicDiscItems), plugin.ItemStack{
		Item:  registry.ItemBedrock,
		Count: 1,
		Patch: plugin.ItemPatch{CustomName: &cancelName},
	})
	c.player.OpenInventory(inv)
}

func (ps *Plugin) cmdToggle(c *cmdCtx) {
	if len(c.args) < 1 {
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot toggle <chat | chatspy | clear-confirmation | time | titles | worldedit | debug>")
		return
	}
	st := ps.stateFor(c.player.UUID())
	opt := strings.ToLower(c.args[0])
	switch opt {
	case "chat":
		if !HasPerm(c.sender, "plots.toggle.chat") {
			ps.sendNoPerm(c.sender, "plots.toggle.chat")
			return
		}
		st.plotChat = !st.plotChat
		ps.sendToggleResult(c.sender, "chat", st.plotChat)
	case "chatspy", "spy":
		if !HasPerm(c.sender, "plots.admin.command.chatspy") {
			ps.sendNoPerm(c.sender, "plots.admin.command.chatspy")
			return
		}
		st.chatSpy = !st.chatSpy
		ps.sendToggleResult(c.sender, "chatspy", st.chatSpy)
	case "titles":
		if !HasPerm(c.sender, "plots.toggle.titles") {
			ps.sendNoPerm(c.sender, "plots.toggle.titles")
			return
		}
		st.disableTitles = !st.disableTitles
		ps.sendToggleResult(c.sender, "titles", !st.disableTitles)
	case "time":
		if !HasPerm(c.sender, "plots.toggle.time") {
			ps.sendNoPerm(c.sender, "plots.toggle.time")
			return
		}
		st.disableTime = !st.disableTime
		ps.sendToggleResult(c.sender, "time", !st.disableTime)
	case "worldedit", "we", "wea":
		if !HasPerm(c.sender, "plots.worldedit.bypass") {
			ps.sendNoPerm(c.sender, "plots.worldedit.bypass")
			return
		}
		next := true
		if st.worldeditBypass != nil {
			next = !*st.worldeditBypass
		} else {
			next = false
		}
		st.worldeditBypass = &next
		ps.sendToggleResult(c.sender, "worldedit", next)
	case "clear-confirmation":
		if !HasPerm(c.sender, "plots.admin.command.autoclear") {
			ps.sendNoPerm(c.sender, "plots.admin.command.autoclear")
			return
		}
		st.clearConfirm = !st.clearConfirm
		ps.sendToggleResult(c.sender, "clear-confirmation", st.clearConfirm)
	case "debug":
		if !HasPerm(c.sender, "plots.toggle.debug") {
			ps.sendNoPerm(c.sender, "plots.toggle.debug")
			return
		}
		st.debug = !st.debug
		ps.sendToggleResult(c.sender, "debug", st.debug)
	default:
		ps.sendCaption(c.sender, "commandconfig.command_syntax", "value", "/plot toggle <chat | chatspy | clear-confirmation | time | titles | worldedit | debug>")
	}
}

func (ps *Plugin) sendToggleResult(sender plugin.CommandSender, name string, enabled bool) {
	if enabled {
		ps.sendCaption(sender, "toggle.toggle_enabled", "setting", name)
	} else {
		ps.sendCaption(sender, "toggle.toggle_disabled", "setting", name)
	}
}

func (ps *Plugin) cmdChat(c *cmdCtx) {
	p := c.player
	if len(c.args) == 0 {
		st := ps.stateFor(p.UUID())
		st.plotChat = !st.plotChat
		ps.sendToggleResult(p, "chat", st.plotChat)
		return
	}
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(p, "errors.not_in_plot")
		return
	}
	if plot.HasOwner() && !plot.FlagBool("chat") && !HasPerm(p, "plots.admin.chat.bypass") {
		ps.sendCaption(p, "chat.plot_chat_denied")
		return
	}
	base := plot.BasePlot()
	msg := ps.Lang.Format("chat.plot_chat_format",
		"world", base.Area.WorldName,
		"plot_id", base.ID.String(),
		"sender", p.Name(),
		"msg", strings.Join(c.args, " "),
	)
	for _, recipient := range ps.Server.Players() {
		rPlot := ps.PlotAt(recipient.Location())
		inSame := rPlot != nil && rPlot.BasePlot() == base
		isSpy := ps.stateFor(recipient.UUID()).chatSpy && HasPerm(recipient, "plots.admin.command.chatspy")
		if inSame || isSpy {
			recipient.SendMessage(msg)
		}
	}
}
