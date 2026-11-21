package plotsplugin

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/event"
	"github.com/golem-mc/plots/internal/generator"
	"github.com/google/uuid"
)

type cmdCategory string

const (
	catClaiming       cmdCategory = "claiming"
	catTeleport       cmdCategory = "teleport"
	catSettings       cmdCategory = "settings"
	catChat           cmdCategory = "chat"
	catSchematic      cmdCategory = "schematic"
	catAppearance     cmdCategory = "appearance"
	catInfo           cmdCategory = "info"
	catDebug          cmdCategory = "debug"
	catAdministration cmdCategory = "administration"
)

var allCategories = []cmdCategory{
	catClaiming, catTeleport, catSettings, catChat,
	catSchematic, catAppearance, catInfo, catDebug, catAdministration,
}

type requiredSender uint8

const (
	reqAny requiredSender = iota
	reqPlayer
	reqConsole
)

type cmdCtx struct {
	ps           *Plugin
	sender       plugin.CommandSender
	player       plugin.Player // nil if console
	args         []string
	forceConfirm bool
	scopedPlot   *plots.Plot
	rawCmd       string
}

type subCommand struct {
	name     string
	aliases  []string
	perm     string
	usage    string
	desc     string
	category cmdCategory
	required requiredSender
	run      func(c *cmdCtx)
	tab      func(c *cmdCtx) []string
}

type pendingConfirm struct {
	cmdStr    string
	createdAt time.Time
	fn        func()
}

var consoleUUID = uuid.MustParse("00000000-0000-0000-0000-000000000000")

func (ps *Plugin) registerCommands() {
	ps.cmds = make(map[string]*subCommand)
	ps.subCmds = nil
	ps.pendingConfirms = make(map[uuid.UUID]*pendingConfirm)

	ps.registerCoreSubcommands()
	ps.registerClaimingCommands()
	ps.registerMemberAndTeleportCommands()
	ps.registerInfoAndFlagCommands()
	ps.registerSocialAndClusterCommands()
	ps.registerSchematicAndBackupCommands()
	ps.registerAdminCommands()
	ps.registerMusicInventoryListener()

	run := func(ctx *plugin.CommandContext) (int, error) {
		rawArgs := strings.TrimSpace(plugin.Arg[string](ctx, "args"))
		var args []string
		if rawArgs != "" {
			args = strings.Fields(rawArgs)
		}
		ps.dispatchPlotCommand(ctx.Sender, args)
		return 1, nil
	}

	suggest := func(ctx *plugin.CommandContext, partial string) []string {
		return ps.tabCompletePlotCommand(ctx.Sender, partial)
	}

	cmd := plugin.Command{
		Name:        "plot",
		Aliases:     []string{"plots", "p", "plotsquared", "plot2", "p2", "ps", "2", "plotme", "plotz", "ap"},
		Description: "Plot management commands",
		Usage:       "/plot <subcommand> [args...]",
		Permission:  "plots.use",
		Run:         run,
		Children: []plugin.Node{
			{
				Arg:     plugin.GreedyArg("args"),
				Run:     run,
				Suggest: suggest,
			},
		},
	}
	if err := ps.Server.Commands().Register(cmd); err != nil && !errors.Is(err, plugin.ErrNameTaken) {
		ps.Log.Warn("failed registering /plot command", "err", err)
	}
}

func (ps *Plugin) addSubCommand(sc *subCommand) {
	ps.subCmds = append(ps.subCmds, sc)
	ps.cmds[strings.ToLower(sc.name)] = sc
	for _, a := range sc.aliases {
		low := strings.ToLower(a)
		if _, exists := ps.cmds[low]; !exists {
			ps.cmds[low] = sc
		}
	}
}

func (ps *Plugin) lookupSubCommand(name string) *subCommand {
	return ps.cmds[strings.ToLower(name)]
}

func (ps *Plugin) registerCoreSubcommands() {
	ps.addSubCommand(&subCommand{
		name:     "help",
		aliases:  []string{"?"},
		perm:     "plots.use",
		usage:    "/plot help [category | page | command]",
		desc:     "Display plot command help",
		category: catInfo,
		run:      ps.cmdHelp,
		tab: func(c *cmdCtx) []string {
			if len(c.args) > 1 {
				return nil
			}
			out := make([]string, 0, len(allCategories)+len(ps.subCmds))
			for _, cat := range allCategories {
				out = append(out, string(cat))
			}
			for _, sc := range ps.subCmds {
				if sc.perm == "" || HasPerm(c.sender, sc.perm) {
					out = append(out, sc.name)
				}
			}
			return filterPrefix(out, lastArg(c.args))
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "confirm",
		perm:     "plots.confirm",
		usage:    "/plot confirm",
		desc:     "Confirm a pending command",
		category: catInfo,
		run:      ps.cmdConfirm,
	})
}

func (ps *Plugin) dispatchPlotCommand(sender plugin.CommandSender, args []string) {
	if !HasPerm(sender, "plots.use") {
		ps.sendNoPerm(sender, "plots.use")
		return
	}
	if len(args) == 0 {
		p, _ := sender.(plugin.Player)
		ps.cmdHelp(&cmdCtx{ps: ps, sender: sender, player: p})
		return
	}

	// Support upstream colon shorthand: /p h:2 Alice -> /p h Alice 2
	if len(args) >= 1 && strings.Contains(args[0], ":") {
		if before, after, ok := strings.Cut(args[0], ":"); ok && before != "" && after != "" && !strings.Contains(after, ":") {
			rebuilt := make([]string, 0, len(args)+1)
			rebuilt = append(rebuilt, before)
			if len(args) > 1 {
				rebuilt = append(rebuilt, args[1:]...)
			}
			rebuilt = append(rebuilt, after)
			args = rebuilt
		}
	}

	ctx := &cmdCtx{
		ps:     ps,
		sender: sender,
	}
	if p, ok := sender.(plugin.Player); ok {
		ctx.player = p
	}

	// Leading plot-scope prefix: /plot <plotId> <subcmd> ...
	if len(args) >= 2 && ps.lookupSubCommand(args[0]) == nil {
		if scoped := ps.resolvePlotFromArg(ctx, args[0]); scoped != nil {
			isAdmin := ctx.player == nil || HasPerm(sender, "plots.admin") || HasPerm(sender, "plots.admin.area.sudo")
			if isAdmin || !scoped.IsDenied(ctx.player.UUID()) {
				ctx.scopedPlot = scoped
				args = args[1:]
			}
		}
	}

	// Leading -f force flag: /plot -f <subcmd> ...
	if len(args) >= 2 && strings.HasPrefix(args[0], "-") {
		if strings.EqualFold(args[0], "-f") {
			ctx.forceConfirm = true
			args = args[1:]
		} else {
			ps.sendCaption(sender, "errors.invalid_command_flag")
			return
		}
	}

	if len(args) == 0 {
		ps.cmdHelp(ctx)
		return
	}

	sub := ps.lookupSubCommand(args[0])
	if sub == nil {
		best := closestMatch(args[0], ps.allSubCommandNames(sender))
		if best != "" {
			ps.sendCaption(sender, "commandconfig.did_you_mean", "value", "/plot "+best)
		} else {
			ps.sendCaption(sender, "commandconfig.not_valid_subcommand")
		}
		return
	}

	if sub.perm != "" && !HasPerm(sender, sub.perm) {
		ps.sendNoPerm(sender, sub.perm)
		return
	}
	if sub.required == reqPlayer && ctx.player == nil {
		ps.sendCaption(sender, "errors.is_console")
		return
	}
	if sub.required == reqConsole && ctx.player != nil {
		ps.sendCaption(sender, "errors.not_console")
		return
	}

	ctx.rawCmd = sub.name
	ctx.args = args[1:]
	sub.run(ctx)
}

// TabComplete returns tab completion candidates for a /plot command argument string.
func (ps *Plugin) TabComplete(sender plugin.CommandSender, partial string) []string {
	return ps.tabCompletePlotCommand(sender, partial)
}

func (ps *Plugin) tabCompletePlotCommand(sender plugin.CommandSender, partial string) []string {
	if !HasPerm(sender, "plots.use") {
		return nil
	}
	endsSpace := strings.HasSuffix(partial, " ")
	tokens := strings.Fields(partial)
	if len(tokens) == 0 {
		return ps.allSubCommandNames(sender)
	}
	if len(tokens) == 1 && !endsSpace {
		return filterPrefix(ps.allSubCommandNames(sender), tokens[0])
	}

	sub := ps.lookupSubCommand(tokens[0])
	if sub == nil || (sub.perm != "" && !HasPerm(sender, sub.perm)) || sub.tab == nil {
		return nil
	}

	subArgs := tokens[1:]
	if endsSpace {
		subArgs = append(subArgs, "")
	}
	ctx := &cmdCtx{ps: ps, sender: sender, args: subArgs}
	if p, ok := sender.(plugin.Player); ok {
		ctx.player = p
	}
	prefix := ""
	if len(tokens) > 1 && !endsSpace {
		prefix = strings.Join(tokens[:len(tokens)-1], " ") + " "
	} else if endsSpace {
		prefix = strings.Join(tokens, " ") + " "
	}
	cands := sub.tab(ctx)
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, prefix+c)
	}
	return out
}

func (ps *Plugin) allSubCommandNames(sender plugin.CommandSender) []string {
	out := make([]string, 0, len(ps.subCmds))
	for _, sc := range ps.subCmds {
		if sc.perm == "" || HasPerm(sender, sc.perm) {
			out = append(out, sc.name)
		}
	}
	sort.Strings(out)
	return out
}

func (ps *Plugin) cmdHelp(c *cmdCtx) {
	pageSize := 10
	filterCat := cmdCategory("")
	page := 1

	if len(c.args) >= 1 {
		arg0 := strings.ToLower(c.args[0])
		if n, err := strconv.Atoi(arg0); err == nil && n >= 1 {
			page = n
		} else if isCategory(arg0) {
			filterCat = cmdCategory(arg0)
			if len(c.args) >= 2 {
				if n2, err := strconv.Atoi(c.args[1]); err == nil && n2 >= 1 {
					page = n2
				}
			}
		} else if sc := ps.lookupSubCommand(arg0); sc != nil {
			ps.sendCaption(c.sender, "help.help_header")
			aliasStr := ""
			if len(sc.aliases) > 0 {
				aliasStr = strings.Join(sc.aliases, ", ")
			}
			ps.sendCaption(c.sender, "help.help_item",
				"usage", sc.usage,
				"alias", aliasStr,
				"desc", sc.desc,
			)
			ps.sendCaption(c.sender, "help.help_footer")
			return
		} else {
			ps.sendCaption(c.sender, "help.help_header")
			for _, cat := range allCategories {
				ps.sendCaption(c.sender, "help.help_info_item",
					"command", "/plot help "+string(cat),
					"category", string(cat),
					"category_desc", "Show "+string(cat)+" commands",
				)
			}
			ps.sendCaption(c.sender, "help.help_footer")
			return
		}
	}

	var matched []*subCommand
	for _, sc := range ps.subCmds {
		if sc.perm != "" && !HasPerm(c.sender, sc.perm) {
			continue
		}
		if filterCat != "" && sc.category != filterCat {
			continue
		}
		matched = append(matched, sc)
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].name < matched[j].name })

	maxPages := max((len(matched)+pageSize-1)/pageSize, 1)
	if page > maxPages {
		page = maxPages
	}
	catLabel := "all"
	if filterCat != "" {
		catLabel = string(filterCat)
	}

	ps.sendCaption(c.sender, "help.help_header")
	ps.sendCaption(c.sender, "help.help_page_header",
		"category", catLabel,
		"current", strconv.Itoa(page),
		"max", strconv.Itoa(maxPages),
	)
	start := (page - 1) * pageSize
	end := min(start+pageSize, len(matched))
	for _, sc := range matched[start:end] {
		aliasStr := strings.Join(sc.aliases, ", ")
		ps.sendCaption(c.sender, "help.help_item",
			"usage", sc.usage,
			"alias", aliasStr,
			"desc", sc.desc,
		)
	}
	ps.sendCaption(c.sender, "help.help_footer")
}

func isCategory(s string) bool {
	for _, c := range allCategories {
		if string(c) == s {
			return true
		}
	}
	return false
}

// --- Confirmation helpers ---

func senderUUID(sender plugin.CommandSender) uuid.UUID {
	if p, ok := sender.(plugin.Player); ok {
		return p.UUID()
	}
	return consoleUUID
}

func (ps *Plugin) needsConfirm(c *cmdCtx, enabled bool) bool {
	if !enabled || c.forceConfirm || c.player == nil {
		return false
	}
	return !HasPerm(c.sender, "plots.confirm.bypass")
}

func (ps *Plugin) runWithConfirm(c *cmdCtx, enabled bool, cmdDesc string, fn func()) {
	if !ps.needsConfirm(c, enabled) {
		fn()
		return
	}
	timeout := 20
	if ps.Settings != nil && ps.Settings.Confirmation.TimeoutSeconds > 0 {
		timeout = ps.Settings.Confirmation.TimeoutSeconds
	}
	uid := senderUUID(c.sender)
	ps.confirmMu.Lock()
	ps.pendingConfirms[uid] = &pendingConfirm{
		cmdStr:    cmdDesc,
		createdAt: time.Now(),
		fn:        fn,
	}
	ps.confirmMu.Unlock()

	ps.sendCaption(c.sender, "confirm.requires_confirm",
		"command", cmdDesc,
		"value", "/plot confirm",
		"timeout", strconv.Itoa(timeout),
	)
}

func (ps *Plugin) cmdConfirm(c *cmdCtx) {
	uid := senderUUID(c.sender)
	ps.confirmMu.Lock()
	pc := ps.pendingConfirms[uid]
	delete(ps.pendingConfirms, uid)
	ps.confirmMu.Unlock()

	if pc == nil {
		ps.sendCaption(c.sender, "confirm.failed_confirm")
		return
	}
	timeout := 20
	if ps.Settings != nil && ps.Settings.Confirmation.TimeoutSeconds > 0 {
		timeout = ps.Settings.Confirmation.TimeoutSeconds
	}
	if time.Since(pc.createdAt) > time.Duration(timeout)*time.Second {
		ps.sendCaption(c.sender, "confirm.expired_confirm")
		return
	}
	pc.fn()
}

// --- Plot & Player command helpers ---

func (ps *Plugin) currentPlot(c *cmdCtx) *plots.Plot {
	if c.scopedPlot != nil {
		return c.scopedPlot
	}
	if c.player == nil {
		return nil
	}
	return ps.PlotAt(c.player.Location())
}

func (ps *Plugin) currentArea(c *cmdCtx) *plots.PlotArea {
	if c.scopedPlot != nil {
		return c.scopedPlot.Area
	}
	if c.player != nil {
		if a := ps.PlotAreaAt(c.player.Location()); a != nil {
			return a
		}
	}
	all := ps.PlotAreas()
	if len(all) == 1 {
		return all[0]
	}
	return nil
}

// resolvePlotFromArg resolves "x;y", "world;x;y", "world;area;x;y", or an alias.
func (ps *Plugin) resolvePlotFromArg(c *cmdCtx, arg string) *plots.Plot {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return nil
	}
	parts := strings.Split(arg, ";")
	if len(parts) == 2 {
		if id, ok := plots.ParsePlotId(arg); ok {
			if area := ps.currentArea(c); area != nil {
				return area.GetOrCreatePlot(id)
			}
		}
	}
	if len(parts) == 3 {
		if area := ps.PlotAreaByName(parts[0]); area != nil {
			if id, ok := plots.ParsePlotId(parts[1] + ";" + parts[2]); ok {
				return area.GetOrCreatePlot(id)
			}
		}
	}
	if len(parts) == 4 {
		if area := ps.PlotAreaByName(parts[0] + ";" + parts[1]); area != nil {
			if id, ok := plots.ParsePlotId(parts[2] + ";" + parts[3]); ok {
				return area.GetOrCreatePlot(id)
			}
		}
	}
	// Comma format "x,y"
	if strings.Contains(arg, ",") {
		if id, ok := plots.ParsePlotId(arg); ok {
			if area := ps.currentArea(c); area != nil {
				return area.GetOrCreatePlot(id)
			}
		}
	}
	// Alias lookup
	for _, a := range ps.PlotAreas() {
		for _, p := range a.BasePlots() {
			if p.Settings.Alias != "" && strings.EqualFold(p.Settings.Alias, arg) {
				return p
			}
		}
	}
	return nil
}

func (ps *Plugin) allowedPlots(p plugin.Player) int {
	maxPlots := 127
	if ps.Settings != nil && ps.Settings.Limit.MaxPlots > 0 {
		maxPlots = ps.Settings.Limit.MaxPlots
	}
	n := HasPermRange(p, "plots.plot", maxPlots)
	if n == 0 && HasPerm(p, "plots.claim") {
		return 1
	}
	return n
}

func (ps *Plugin) playerPlotCount(u uuid.UUID, worldName string) int {
	if ps.Settings == nil || ps.Settings.Limit.Global {
		return len(ps.PlotsByOwner(u))
	}
	count := 0
	for _, a := range ps.PlotAreasInWorld(worldName) {
		count += len(a.PlotsByOwner(u))
	}
	return count
}

func (ps *Plugin) grantedPlots(u uuid.UUID) int {
	if ps.DB == nil {
		return 0
	}
	meta, err := ps.DB.GetPlayerMeta(u)
	if err != nil {
		return 0
	}
	b := meta["grantedPlots"]
	if len(b) < 4 {
		return 0
	}
	return int(int32(binary.BigEndian.Uint32(b[:4])))
}

func (ps *Plugin) setGrantedPlots(u uuid.UUID, n int) {
	if ps.DB == nil {
		return
	}
	if n <= 0 {
		_ = ps.DB.RemovePlayerMeta(u, "grantedPlots")
		return
	}
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(int32(n)))
	_ = ps.DB.SetPlayerMeta(u, "grantedPlots", buf)
}

func (ps *Plugin) teleportToPlot(p plugin.Player, plot *plots.Plot, cause string) bool {
	if p == nil || plot == nil || plot.Area == nil {
		return false
	}
	w, ok := ps.WrapWorld(plot.Area.WorldName)
	if !ok && plot.Area.Type == plots.AreaSingle {
		var err error
		w, err = ps.ensureSingleWorld(plot.Area)
		ok = err == nil
	}
	if !ok {
		return false
	}
	base := plot.BasePlot()
	isMember := base.IsAdded(p.UUID()) || HasPerm(p, "plots.admin")
	loc := generator.SafePlotHome(w, base.Area, base, isMember)
	ev := &event.PlayerTeleportToPlotEvent{Player: p, Plot: base, Cause: cause}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		return false
	}
	delay := 0
	if ps.Settings != nil && ps.Settings.Teleport.Delay > 0 && !HasPerm(p, "plots.teleport.delay.bypass") {
		delay = ps.Settings.Teleport.Delay
	}
	if delay > 0 {
		startBlock := p.Location().Pos.Block()
		ps.sendCaption(p, "teleport.teleport_in_seconds", "amount", strconv.Itoa(delay))
		ps.Server.Scheduler().RunLater(delay*20, func() {
			if p.Location().Pos.Block() != startBlock {
				ps.sendCaption(p, "teleport.teleport_failed")
				return
			}
			p.Teleport(loc, plugin.TeleportCommand)
			ps.sendCaption(p, "teleport.teleported_to_plot")
		})
		return true
	}
	p.Teleport(loc, plugin.TeleportCommand)
	return true
}

func (ps *Plugin) playersInPlot(plot *plots.Plot) []plugin.Player {
	if plot == nil || ps.Server == nil {
		return nil
	}
	base := plot.BasePlot()
	var out []plugin.Player
	for _, p := range ps.Server.Players() {
		cur := ps.PlotAt(p.Location())
		if cur != nil && cur.BasePlot() == base {
			out = append(out, p)
		}
	}
	return out
}

func (ps *Plugin) kickFromPlot(target plugin.Player, plot *plots.Plot) {
	if target == nil || plot == nil || plot.Area == nil {
		return
	}
	base := plot.BasePlot()
	if cur := ps.PlotAt(target.Location()); cur == nil || cur.BasePlot() != base {
		return
	}
	if HasPerm(target, "plots.admin.entry.denied") {
		return
	}
	// Prefer world spawn if outside the plot, else overworld spawn.
	var dst plugin.Location
	if w, ok := ps.WrapWorld(base.Area.WorldName); ok {
		cand := w.SpawnPoint()
		if cur := ps.PlotAt(cand); cur == nil || cur.BasePlot() != base {
			dst = cand
		}
	}
	if dst.World == nil {
		if ow, ok := ps.Server.World(plugin.MinecraftKey("overworld")); ok {
			dst = ow.SpawnPoint()
		}
	}
	if dst.World != nil {
		target.Teleport(dst, plugin.TeleportPlugin)
	}
}

func (ps *Plugin) onlinePlayerNames() []string {
	if ps.Server == nil {
		return nil
	}
	psList := ps.Server.Players()
	out := make([]string, 0, len(psList))
	for _, p := range psList {
		out = append(out, p.Name())
	}
	sort.Strings(out)
	return out
}

func (ps *Plugin) allAreaNames() []string {
	areas := ps.PlotAreas()
	out := make([]string, 0, len(areas))
	for _, a := range areas {
		out = append(out, a.FullID())
	}
	sort.Strings(out)
	return out
}

func facingDirection(yaw float32) plots.Direction {
	norm := math.Mod(float64(yaw), 360)
	if norm < 0 {
		norm += 360
	}
	switch {
	case norm >= 315 || norm < 45:
		return plots.DirSouth
	case norm >= 45 && norm < 135:
		return plots.DirWest
	case norm >= 135 && norm < 225:
		return plots.DirNorth
	default:
		return plots.DirEast
	}
}

func lastArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[len(args)-1]
}

func filterPrefix(items []string, prefix string) []string {
	low := strings.ToLower(prefix)
	var out []string
	for _, it := range items {
		if strings.HasPrefix(strings.ToLower(it), low) {
			out = append(out, it)
		}
	}
	return out
}

func closestMatch(input string, candidates []string) string {
	low := strings.ToLower(input)
	best := ""
	bestDist := len(low)/2 + 3
	for _, cand := range candidates {
		d := levenshtein(low, strings.ToLower(cand))
		if d < bestDist {
			bestDist = d
			best = cand
		}
	}
	return best
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := range lb + 1 {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func formatTimeAgo(ms int64) string {
	if ms <= 0 {
		return "unknown"
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02 15:04")
}

func (ps *Plugin) formatUUIDList(set map[uuid.UUID]struct{}) string {
	if len(set) == 0 {
		return "None"
	}
	names := make([]string, 0, len(set))
	for u := range set {
		names = append(names, ps.PlayerName(u))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func formatEcoAmount(ps *Plugin, amount float64) string {
	if eco, ok := ps.Economy(); ok {
		return eco.Format(amount)
	}
	return fmt.Sprintf("%.2f", amount)
}
