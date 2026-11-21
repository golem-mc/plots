package plotsplugin

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/event"
	"github.com/golem-mc/plots/internal/expiry"
	"github.com/golem-mc/plots/internal/lang"
	"github.com/google/uuid"
)

var validInboxes = []string{"public", "owner", "report"}

func (ps *Plugin) registerSocialAndClusterCommands() {
	ps.addSubCommand(&subCommand{
		name:     "rate",
		aliases:  []string{"rt"},
		perm:     "plots.rate",
		usage:    "/plot rate <1-10 | next | purge>",
		desc:     "Rate the current plot",
		category: catInfo,
		required: reqPlayer,
		run:      ps.cmdRate,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix([]string{"1", "5", "10", "next", "purge"}, lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "like",
		perm:     "plots.like",
		usage:    "/plot like [next | purge]",
		desc:     "Like the current plot",
		category: catInfo,
		required: reqPlayer,
		run:      func(c *cmdCtx) { ps.cmdLikeOrDislike(c, true) },
	})

	ps.addSubCommand(&subCommand{
		name:     "dislike",
		perm:     "plots.dislike",
		usage:    "/plot dislike [next | purge]",
		desc:     "Dislike the current plot",
		category: catInfo,
		required: reqPlayer,
		run:      func(c *cmdCtx) { ps.cmdLikeOrDislike(c, false) },
	})

	ps.addSubCommand(&subCommand{
		name:     "comment",
		aliases:  []string{"msg"},
		perm:     "plots.comment",
		usage:    "/plot comment [X;Z] <public | owner | report> <message...>",
		desc:     "Leave a comment in a plot inbox",
		category: catChat,
		required: reqPlayer,
		run:      ps.cmdComment,
		tab: func(c *cmdCtx) []string {
			if len(c.args) <= 1 {
				return filterPrefix(validInboxes, lastArg(c.args))
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "inbox",
		perm:     "plots.inbox",
		usage:    "/plot inbox [public | owner | report] [delete <#> | clear | page]",
		desc:     "View or manage comments in a plot inbox",
		category: catChat,
		required: reqPlayer,
		run:      ps.cmdInbox,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				return filterPrefix(validInboxes, c.args[0])
			}
			if len(c.args) == 2 {
				return filterPrefix([]string{"delete", "clear", "1"}, c.args[1])
			}
			return nil
		},
	})

	ps.addSubCommand(&subCommand{
		name:     "done",
		aliases:  []string{"submit"},
		perm:     "plots.done",
		usage:    "/plot done",
		desc:     "Mark the current plot as done",
		category: catSettings,
		run:      ps.cmdDone,
	})

	ps.addSubCommand(&subCommand{
		name:     "continue",
		perm:     "plots.continue",
		usage:    "/plot continue",
		desc:     "Allow building on a plot marked as done",
		category: catSettings,
		required: reqPlayer,
		run:      ps.cmdContinue,
	})

	ps.addSubCommand(&subCommand{
		name:     "cluster",
		aliases:  []string{"clusters"},
		perm:     "plots.cluster",
		usage:    "/plot cluster <list | create | delete | resize | invite | kick | leave | helpers | info | tp | sethome>",
		desc:     "Manage plot clusters",
		category: catAdministration,
		required: reqPlayer,
		run:      ps.cmdCluster,
		tab: func(c *cmdCtx) []string {
			if len(c.args) == 1 {
				return filterPrefix([]string{"list", "create", "delete", "resize", "invite", "kick", "leave", "helpers", "info", "tp", "sethome"}, c.args[0])
			}
			return nil
		},
	})
}

func (ps *Plugin) cmdRate(c *cmdCtx) {
	p := c.player
	if len(c.args) == 1 {
		switch strings.ToLower(c.args[0]) {
		case "next":
			ps.teleportNextUnrated(p)
			return
		case "purge":
			ps.purgePlotRatings(c)
			return
		}
	}
	plot := ps.checkRateablePlot(p)
	if plot == nil {
		return
	}
	if len(c.args) < 1 {
		ps.sendCaption(p, "ratings.rating_not_valid")
		return
	}
	val, err := strconv.Atoi(c.args[0])
	if err != nil || val < 1 || val > 10 {
		ps.sendCaption(p, "ratings.rating_not_valid")
		return
	}
	ps.applyPlotRating(p, plot, val, "ratings.rating_applied")
}

func (ps *Plugin) cmdLikeOrDislike(c *cmdCtx, isLike bool) {
	p := c.player
	if len(c.args) == 1 {
		switch strings.ToLower(c.args[0]) {
		case "next":
			ps.teleportNextUnrated(p)
			return
		case "purge":
			ps.purgePlotRatings(c)
			return
		}
	}
	plot := ps.checkRateablePlot(p)
	if plot == nil {
		return
	}
	val := 1
	capKey := "ratings.rating_disliked"
	if isLike {
		val = 10
		capKey = "ratings.rating_liked"
	}
	ps.applyPlotRating(p, plot, val, capKey)
}

func (ps *Plugin) checkRateablePlot(p plugin.Player) *plots.Plot {
	plot := ps.PlotAt(p.Location())
	if plot == nil {
		ps.sendCaption(p, "errors.not_in_plot")
		return nil
	}
	base := plot.BasePlot()
	if !base.HasOwner() {
		ps.sendCaption(p, "ratings.rating_not_owned")
		return nil
	}
	if base.IsOwner(p.UUID()) {
		ps.sendCaption(p, "ratings.rating_not_your_own")
		return nil
	}
	if ps.Settings != nil && ps.Settings.Done.RequiredForRatings && base.FlagString("done") == "" {
		ps.sendCaption(p, "ratings.rating_not_done")
		return nil
	}
	return base
}

func (ps *Plugin) applyPlotRating(p plugin.Player, base *plots.Plot, val int, successKey string) {
	if base.Settings.Ratings == nil {
		base.Settings.Ratings = make(map[uuid.UUID]int)
	}
	if _, exists := base.Settings.Ratings[p.UUID()]; exists {
		ps.sendCaption(p, "ratings.rating_already_exists", "plot", base.ID.String())
		return
	}
	ev := &event.PlotRateEvent{Player: p, Plot: base, Rating: val}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		return
	}
	base.Settings.Ratings[p.UUID()] = ev.Rating
	if ps.DB != nil {
		_ = ps.DB.SetRating(base, p.UUID(), ev.Rating)
	}
	ps.sendCaption(p, successKey, "plot", base.ID.String())
}

func (ps *Plugin) purgePlotRatings(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	if !HasPerm(c.sender, "plots.admin.command.purge.ratings") {
		ps.sendNoPerm(c.sender, "plots.admin.command.purge.ratings")
		return
	}
	base := plot.BasePlot()
	base.Settings.Ratings = nil
	if ps.DB != nil {
		_ = ps.DB.DeleteRatings(base)
	}
	ps.sendCaption(c.sender, "ratings.ratings_purged")
}

func (ps *Plugin) teleportNextUnrated(p plugin.Player) {
	for _, a := range ps.PlotAreas() {
		for _, pl := range a.BasePlots() {
			if !pl.HasOwner() || pl.IsAdded(p.UUID()) {
				continue
			}
			if ps.Settings != nil && ps.Settings.Done.RequiredForRatings && pl.FlagString("done") == "" {
				continue
			}
			if pl.Settings.Ratings != nil {
				if _, rated := pl.Settings.Ratings[p.UUID()]; rated {
					continue
				}
			}
			ps.teleportToPlot(p, pl, "rate")
			ps.sendCaption(p, "tutorial.rate_this")
			return
		}
	}
	ps.sendCaption(p, "invalid.found_no_plots")
}

func isValidInbox(name string) bool {
	return name == "public" || name == "owner" || name == "report"
}

func (ps *Plugin) canAccessInbox(p plugin.Player, plot *plots.Plot, inbox, mode string) bool {
	perm := "plots.inbox." + mode + "." + inbox
	if HasPerm(p, perm) {
		return true
	}
	if plot != nil && plot.IsOwner(p.UUID()) && (inbox == "public" || inbox == "owner") {
		return true
	}
	return false
}

func (ps *Plugin) cmdComment(c *cmdCtx) {
	p := c.player
	if len(c.args) < 2 {
		ps.sendCaption(p, "comment.comment_syntax", "command", "/plot comment [X;Z]", "list", "public|owner|report")
		return
	}
	var plot *plots.Plot
	idx := 0
	if !isValidInbox(strings.ToLower(c.args[0])) {
		plot = ps.resolvePlotFromArg(c, c.args[0])
		if plot != nil {
			idx = 1
		}
	}
	if plot == nil {
		plot = ps.currentPlot(c)
	}
	if plot == nil || len(c.args) < idx+2 {
		ps.sendCaption(p, "comment.comment_syntax", "command", "/plot comment [X;Z]", "list", "public|owner|report")
		return
	}
	inbox := strings.ToLower(c.args[idx])
	if !isValidInbox(inbox) {
		ps.sendCaption(p, "comment.invalid_inbox", "list", "public, owner, report")
		return
	}
	if !ps.canAccessInbox(p, plot, inbox, "write") {
		ps.sendCaption(p, "comment.no_perm_inbox")
		return
	}
	base := plot.BasePlot()
	msg := strings.Join(c.args[idx+1:], " ")
	comment := plots.PlotComment{
		World:     base.Area.WorldName,
		ID:        base.ID,
		Comment:   msg,
		Inbox:     inbox,
		Timestamp: time.Now().UnixMilli(),
		Sender:    p.Name(),
	}
	base.Settings.Comments = append(base.Settings.Comments, comment)
	if ps.DB != nil {
		_ = ps.DB.AddComment(comment)
	}
	for _, pl := range ps.Server.Players() {
		if ps.stateFor(pl.UUID()).chatSpy && HasPerm(pl, "plots.admin.command.chatspy") {
			pl.SendMessage(lang.ParseMiniMessage("<gray>[Plot Spy] /plot comment " + strings.Join(c.args, " ") + "</gray>"))
		}
	}
	ps.sendCaption(p, "comment.comment_added")
}

func (ps *Plugin) cmdInbox(c *cmdCtx) {
	p := c.player
	plot := ps.currentPlot(c)
	if plot == nil {
		ps.sendCaption(p, "errors.not_in_plot")
		return
	}
	if !plot.HasOwner() {
		ps.sendCaption(p, "info.plot_unowned")
		return
	}
	base := plot.BasePlot()
	if len(c.args) == 0 {
		for _, inb := range validInboxes {
			if ps.canAccessInbox(p, base, inb, "read") {
				cnt := 0
				for _, cm := range base.Settings.Comments {
					if strings.EqualFold(cm.Inbox, inb) {
						cnt++
					}
				}
				ps.sendCaption(p, "comment.inbox_item", "value", fmt.Sprintf("%s (%d)", inb, cnt))
			}
		}
		return
	}
	inbox := strings.ToLower(c.args[0])
	if !isValidInbox(inbox) {
		ps.sendCaption(p, "comment.invalid_inbox", "list", "public, owner, report")
		return
	}
	if len(c.args) >= 2 {
		switch strings.ToLower(c.args[1]) {
		case "clear":
			if !ps.canAccessInbox(p, base, inbox, "modify") {
				ps.sendCaption(p, "comment.no_perm_inbox_modify")
				return
			}
			var kept []plots.PlotComment
			for _, cm := range base.Settings.Comments {
				if !strings.EqualFold(cm.Inbox, inbox) {
					kept = append(kept, cm)
				}
			}
			base.Settings.Comments = kept
			if ps.DB != nil {
				_ = ps.DB.ClearInbox(base.Area.WorldName, base.ID, inbox)
			}
			ps.sendCaption(p, "comment.comment_removed_success", "value", inbox)
			return
		case "delete":
			if !ps.canAccessInbox(p, base, inbox, "modify") {
				ps.sendCaption(p, "comment.no_perm_inbox_modify")
				return
			}
			if len(c.args) != 3 {
				ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot inbox <inbox> delete <#>")
				return
			}
			idx, err := strconv.Atoi(c.args[2])
			if err != nil || idx < 1 {
				ps.sendCaption(p, "comment.not_valid_inbox_index", "number", c.args[2])
				return
			}
			matchedIdx := 0
			removed := false
			var kept []plots.PlotComment
			for _, cm := range base.Settings.Comments {
				if strings.EqualFold(cm.Inbox, inbox) {
					matchedIdx++
					if matchedIdx == idx {
						if ps.DB != nil {
							_ = ps.DB.RemoveComment(cm)
						}
						removed = true
						continue
					}
				}
				kept = append(kept, cm)
			}
			if !removed {
				ps.sendCaption(p, "comment.not_valid_inbox_index", "number", c.args[2])
				return
			}
			base.Settings.Comments = kept
			ps.sendCaption(p, "comment.comment_removed_success", "value", c.args[2])
			return
		}
	}
	if !ps.canAccessInbox(p, base, inbox, "read") {
		ps.sendCaption(p, "comment.no_perm_inbox")
		return
	}
	var list []plots.PlotComment
	for _, cm := range base.Settings.Comments {
		if strings.EqualFold(cm.Inbox, inbox) {
			list = append(list, cm)
		}
	}
	if len(list) == 0 {
		ps.sendCaption(p, "comment.inbox_empty")
		return
	}
	page := 1
	if len(c.args) >= 2 {
		if n, err := strconv.Atoi(c.args[1]); err == nil && n >= 1 {
			page = n
		}
	}
	pageSize := 12
	maxPages := max((len(list)+pageSize-1)/pageSize, 1)
	if page > maxPages {
		page = maxPages
	}
	ps.sendCaption(p, "list.comment_list_header_paged",
		"amount", strconv.Itoa(len(list)),
		"cur", strconv.Itoa(page),
		"max", strconv.Itoa(maxPages),
		"word", inbox,
	)
	start := (page - 1) * pageSize
	end := min(start+pageSize, len(list))
	for i := start; i < end; i++ {
		cm := list[i]
		line := fmt.Sprintf("<gray>[<gold>%d</gold>] <gold>%s</gold>: <gray>%s</gray>", i+1, cm.Sender, cm.Comment)
		p.SendMessage(lang.ParseMiniMessage(line))
	}
}

func (ps *Plugin) cmdDone(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil || !plot.HasOwner() {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	base := plot.BasePlot()
	if c.player != nil && !base.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin.command.done") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	if base.FlagString("done") != "" {
		ps.sendCaption(c.sender, "done.done_already_done")
		return
	}
	ev := &event.PlotDoneEvent{Plot: base}
	ps.Server.Events().Fire(ev)
	if ev.Cancelled() {
		ps.sendCaption(c.sender, "events.event_denied", "value", "Done")
		return
	}
	if ps.Settings != nil && !HasPerm(c.sender, "plots.admin.command.done") {
		if req, ok := ps.Settings.AutoClear["done"]; ok && req.Threshold > 0 {
			if w, okW := ps.WrapWorld(base.Area.WorldName); okW {
				an := expiry.AnalyzePlot(w, base.Area, base)
				if an.Complexity(req.Calibration) < req.Threshold {
					ps.sendCaption(c.sender, "done.done_insufficient_complexity")
					return
				}
			}
		}
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	_, _ = ps.SetFlag(base, "done", ts)
	ps.sendCaption(c.sender, "done.done_success")
}

func (ps *Plugin) cmdContinue(c *cmdCtx) {
	plot := ps.currentPlot(c)
	if plot == nil || !plot.HasOwner() {
		ps.sendCaption(c.sender, "errors.not_in_plot")
		return
	}
	base := plot.BasePlot()
	if !base.IsOwner(c.player.UUID()) && !HasPerm(c.sender, "plots.admin.command.continue") {
		ps.sendCaption(c.sender, "permission.no_plot_perms")
		return
	}
	if base.FlagString("done") == "" {
		ps.sendCaption(c.sender, "done.done_not_done")
		return
	}
	_, _ = ps.RemoveFlag(base, "done")
	ps.sendCaption(c.sender, "done.done_removed")
}

func (ps *Plugin) cmdCluster(c *cmdCtx) {
	p := c.player
	if len(c.args) == 0 {
		ps.sendCaption(p, "cluster.cluster_available_args", "list", "list, create, delete, resize, invite, kick, leave, helpers, info, tp, sethome")
		return
	}
	area := ps.currentArea(c)
	if area == nil {
		ps.sendCaption(p, "errors.not_in_plot_world")
		return
	}
	sub := strings.ToLower(c.args[0])
	switch sub {
	case "l", "list":
		ps.cmdClusterList(p, area)
	case "c", "create":
		ps.cmdClusterCreate(p, area, c.args[1:])
	case "del", "delete", "disband":
		ps.cmdClusterDelete(p, area, c.args[1:])
	case "res", "resize":
		ps.cmdClusterResize(p, area, c.args[1:])
	case "inv", "invite", "add":
		ps.cmdClusterInviteOrKick(p, area, c.args[1:], true)
	case "k", "kick", "remove":
		ps.cmdClusterInviteOrKick(p, area, c.args[1:], false)
	case "leave":
		ps.cmdClusterLeave(p, area)
	case "helpers":
		ps.cmdClusterHelpers(p, area, c.args[1:])
	case "i", "info":
		ps.cmdClusterInfo(p, area, c.args[1:])
	case "tp", "v", "visit":
		ps.cmdClusterTP(p, area, c.args[1:])
	case "sethome":
		ps.cmdClusterSetHome(p, area)
	default:
		ps.sendCaption(p, "cluster.cluster_available_args", "list", "list, create, delete, resize, invite, kick, leave, helpers, info, tp, sethome")
	}
}

func (ps *Plugin) clusterAtPlayer(p plugin.Player, area *plots.PlotArea) *plots.PlotCluster {
	bp := p.Location().Pos.Block()
	id := area.PlotIdNearest(bp.X, bp.Z)
	return area.ClusterAt(id)
}

func (ps *Plugin) cmdClusterList(p plugin.Player, area *plots.PlotArea) {
	if !HasPerm(p, "plots.cluster.list") {
		ps.sendNoPerm(p, "plots.cluster.list")
		return
	}
	clusters := area.Clusters()
	ps.sendCaption(p, "cluster.cluster_list_heading", "amount", strconv.Itoa(len(clusters)))
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].Name < clusters[j].Name })
	for _, cl := range clusters {
		label := fmt.Sprintf("'%s' (%s -> %s)", cl.Name, cl.Pos1, cl.Pos2)
		switch {
		case cl.Owner == p.UUID():
			ps.sendCaption(p, "cluster.cluster_list_element_owner", "cluster", label)
		case func() bool { _, ok := cl.Helpers[p.UUID()]; return ok }():
			ps.sendCaption(p, "cluster.cluster_list_element_helpers", "cluster", label)
		case func() bool { _, ok := cl.Invited[p.UUID()]; return ok }():
			ps.sendCaption(p, "cluster.cluster_list_element_invited", "cluster", label)
		default:
			ps.sendCaption(p, "cluster.cluster_list_element", "cluster", label)
		}
	}
}

func (ps *Plugin) cmdClusterCreate(p plugin.Player, area *plots.PlotArea, args []string) {
	if !HasPerm(p, "plots.cluster.create") {
		ps.sendNoPerm(p, "plots.cluster.create")
		return
	}
	if len(args) != 3 {
		ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot cluster create <name> <id-bot> <id-top>")
		return
	}
	name := args[0]
	if area.ClusterNamed(name) != nil {
		ps.sendCaption(p, "alias.alias_is_taken", "alias", name)
		return
	}
	p1, ok1 := plots.ParsePlotId(args[1])
	p2, ok2 := plots.ParsePlotId(args[2])
	if !ok1 || !ok2 {
		ps.sendCaption(p, "invalid.not_valid_plot_id")
		return
	}
	pos1 := plots.PlotId{X: min(p1.X, p2.X), Y: min(p1.Y, p2.Y)}
	pos2 := plots.PlotId{X: max(p1.X, p2.X), Y: max(p1.Y, p2.Y)}
	if !area.ContainsPlotId(pos1) || !area.ContainsPlotId(pos2) {
		ps.sendCaption(p, "cluster.cluster_outside", "area", area.FullID())
		return
	}
	for _, ex := range area.Clusters() {
		if ex.Intersects(pos1, pos2) {
			ps.sendCaption(p, "cluster.cluster_intersection", "cluster", ex.Name)
			return
		}
	}
	cl := &plots.PlotCluster{
		Area:    area,
		Pos1:    pos1,
		Pos2:    pos2,
		Owner:   p.UUID(),
		Helpers: make(map[uuid.UUID]struct{}),
		Invited: make(map[uuid.UUID]struct{}),
		Name:    name,
	}
	cl.Settings.Alias = name
	area.AddCluster(cl)
	if ps.DB != nil {
		_ = ps.DB.CreateCluster(cl)
	}
	ps.sendCaption(p, "cluster.cluster_created", "name", name)
}

func (ps *Plugin) cmdClusterDelete(p plugin.Player, area *plots.PlotArea, args []string) {
	if !HasPerm(p, "plots.cluster.delete") && !HasPerm(p, "plots.cluster") {
		ps.sendNoPerm(p, "plots.cluster.delete")
		return
	}
	var cl *plots.PlotCluster
	if len(args) >= 1 {
		cl = area.ClusterNamed(args[0])
		if cl == nil {
			ps.sendCaption(p, "cluster.invalid_cluster_name", "cluster", args[0])
			return
		}
	} else {
		cl = ps.clusterAtPlayer(p, area)
		if cl == nil {
			ps.sendCaption(p, "errors.not_in_cluster")
			return
		}
	}
	if cl.Owner != p.UUID() && !HasPerm(p, "plots.cluster.delete.other") {
		ps.sendNoPerm(p, "plots.cluster.delete.other")
		return
	}
	area.RemoveCluster(cl)
	if ps.DB != nil {
		_ = ps.DB.DeleteCluster(cl)
	}
	ps.sendCaption(p, "cluster.cluster_deleted", "cluster", cl.Name)
}

func (ps *Plugin) cmdClusterResize(p plugin.Player, area *plots.PlotArea, args []string) {
	if !HasPerm(p, "plots.cluster.resize") {
		ps.sendNoPerm(p, "plots.cluster.resize")
		return
	}
	if len(args) != 2 {
		ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot cluster resize <id-bot> <id-top>")
		return
	}
	cl := ps.clusterAtPlayer(p, area)
	if cl == nil {
		ps.sendCaption(p, "errors.not_in_cluster")
		return
	}
	_, isHelper := cl.Helpers[p.UUID()]
	if cl.Owner != p.UUID() && !isHelper && !HasPerm(p, "plots.cluster.resize.other") {
		ps.sendNoPerm(p, "plots.cluster.resize.other")
		return
	}
	p1, ok1 := plots.ParsePlotId(args[0])
	p2, ok2 := plots.ParsePlotId(args[1])
	if !ok1 || !ok2 {
		ps.sendCaption(p, "invalid.not_valid_plot_id")
		return
	}
	pos1 := plots.PlotId{X: min(p1.X, p2.X), Y: min(p1.Y, p2.Y)}
	pos2 := plots.PlotId{X: max(p1.X, p2.X), Y: max(p1.Y, p2.Y)}
	for _, ex := range area.Clusters() {
		if ex != cl && ex.Intersects(pos1, pos2) {
			ps.sendCaption(p, "cluster.cluster_intersection", "cluster", ex.Name)
			return
		}
	}
	cl.Pos1, cl.Pos2 = pos1, pos2
	if ps.DB != nil {
		_ = ps.DB.ResizeCluster(cl, pos1, pos2)
	}
	ps.sendCaption(p, "cluster.cluster_resized")
}

func (ps *Plugin) cmdClusterInviteOrKick(p plugin.Player, area *plots.PlotArea, args []string, invite bool) {
	perm := "plots.cluster.kick"
	otherPerm := "plots.cluster.kick.other"
	if invite {
		perm = "plots.cluster.invite"
		otherPerm = "plots.cluster.invite.other"
	}
	if !HasPerm(p, perm) {
		ps.sendNoPerm(p, perm)
		return
	}
	if len(args) != 1 {
		ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot cluster <invite | kick> <player>")
		return
	}
	cl := ps.clusterAtPlayer(p, area)
	if cl == nil {
		ps.sendCaption(p, "errors.not_in_cluster")
		return
	}
	_, isHelper := cl.Helpers[p.UUID()]
	if cl.Owner != p.UUID() && !isHelper && !HasPerm(p, otherPerm) {
		ps.sendNoPerm(p, otherPerm)
		return
	}
	uid, ok := ps.ResolvePlayerUUID(args[0])
	if !ok {
		ps.sendCaption(p, "errors.invalid_player", "value", args[0])
		return
	}
	if invite {
		cl.Invited[uid] = struct{}{}
		if ps.DB != nil {
			_ = ps.DB.SetClusterUser(cl, "cluster_invited", uid, true)
		}
		ps.sendCaption(p, "cluster.cluster_added_user")
		if target, online := ps.Server.Player(uid); online {
			ps.sendCaption(target, "cluster.cluster_invited", "cluster", cl.Name)
		}
		return
	}
	if uid == cl.Owner {
		ps.sendCaption(p, "cluster.cannot_kick_player", "name", args[0])
		return
	}
	delete(cl.Invited, uid)
	delete(cl.Helpers, uid)
	if ps.DB != nil {
		_ = ps.DB.SetClusterUser(cl, "cluster_invited", uid, false)
		_ = ps.DB.SetClusterUser(cl, "cluster_helpers", uid, false)
	}
	ps.sendCaption(p, "cluster.cluster_kicked_user")
}

func (ps *Plugin) cmdClusterLeave(p plugin.Player, area *plots.PlotArea) {
	cl := ps.clusterAtPlayer(p, area)
	if cl == nil {
		ps.sendCaption(p, "errors.not_in_cluster")
		return
	}
	if cl.Owner == p.UUID() {
		ps.sendCaption(p, "cluster.cluster_cannot_leave")
		return
	}
	delete(cl.Invited, p.UUID())
	delete(cl.Helpers, p.UUID())
	if ps.DB != nil {
		_ = ps.DB.SetClusterUser(cl, "cluster_invited", p.UUID(), false)
		_ = ps.DB.SetClusterUser(cl, "cluster_helpers", p.UUID(), false)
	}
	ps.sendCaption(p, "cluster.cluster_removed", "cluster", cl.Name)
}

func (ps *Plugin) cmdClusterHelpers(p plugin.Player, area *plots.PlotArea, args []string) {
	if len(args) != 2 {
		ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot cluster helpers <add | remove> <player>")
		return
	}
	cl := ps.clusterAtPlayer(p, area)
	if cl == nil {
		ps.sendCaption(p, "errors.not_in_cluster")
		return
	}
	if cl.Owner != p.UUID() && !HasPerm(p, "plots.admin") {
		ps.sendCaption(p, "permission.no_plot_perms")
		return
	}
	uid, ok := ps.ResolvePlayerUUID(args[1])
	if !ok {
		ps.sendCaption(p, "errors.invalid_player", "value", args[1])
		return
	}
	switch strings.ToLower(args[0]) {
	case "add":
		cl.Helpers[uid] = struct{}{}
		if ps.DB != nil {
			_ = ps.DB.SetClusterUser(cl, "cluster_helpers", uid, true)
		}
		ps.sendCaption(p, "cluster.cluster_added_helper")
	case "remove":
		delete(cl.Helpers, uid)
		if ps.DB != nil {
			_ = ps.DB.SetClusterUser(cl, "cluster_helpers", uid, false)
		}
		ps.sendCaption(p, "cluster.cluster_removed_helper")
	default:
		ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot cluster helpers <add | remove> <player>")
	}
}

func (ps *Plugin) cmdClusterInfo(p plugin.Player, area *plots.PlotArea, args []string) {
	var cl *plots.PlotCluster
	if len(args) >= 1 {
		cl = area.ClusterNamed(args[0])
	} else {
		cl = ps.clusterAtPlayer(p, area)
	}
	if cl == nil {
		ps.sendCaption(p, "errors.not_in_cluster")
		return
	}
	rights := fmt.Sprintf("helpers=%s, invited=%s", ps.formatUUIDList(cl.Helpers), ps.formatUUIDList(cl.Invited))
	ps.sendCaption(p, "cluster.cluster_info",
		"id", fmt.Sprintf("%s;%s", cl.Pos1, cl.Pos2),
		"name", cl.Name,
		"owner", ps.PlayerName(cl.Owner),
		"size", strconv.Itoa(cl.AreaCount()),
		"rights", rights,
	)
}

func (ps *Plugin) cmdClusterTP(p plugin.Player, area *plots.PlotArea, args []string) {
	if !HasPerm(p, "plots.cluster.tp") {
		ps.sendNoPerm(p, "plots.cluster.tp")
		return
	}
	if len(args) != 1 {
		ps.sendCaption(p, "commandconfig.command_syntax", "value", "/plot cluster tp <name>")
		return
	}
	cl := area.ClusterNamed(args[0])
	if cl == nil {
		ps.sendCaption(p, "cluster.invalid_cluster_name", "cluster", args[0])
		return
	}
	if !cl.IsAdded(p.UUID()) && !HasPerm(p, "plots.cluster.tp.other") {
		ps.sendNoPerm(p, "plots.cluster.tp.other")
		return
	}
	plot := area.GetOrCreatePlot(cl.Pos1)
	ps.teleportToPlot(p, plot, "cluster_tp")
	ps.sendCaption(p, "cluster.cluster_teleporting")
}

func (ps *Plugin) cmdClusterSetHome(p plugin.Player, area *plots.PlotArea) {
	if !HasPerm(p, "plots.cluster.sethome") {
		ps.sendNoPerm(p, "plots.cluster.sethome")
		return
	}
	cl := ps.clusterAtPlayer(p, area)
	if cl == nil {
		ps.sendCaption(p, "errors.not_in_cluster")
		return
	}
	if cl.Owner != p.UUID() && !HasPerm(p, "plots.cluster.sethome.other") {
		ps.sendNoPerm(p, "plots.cluster.sethome.other")
		return
	}
	loc := p.Location()
	bp := loc.Pos.Block()
	b1 := area.PlotInteriorBounds(cl.Pos1)
	rel := plots.BlockLoc{X: bp.X - b1.Min.X, Y: bp.Y, Z: bp.Z - b1.Min.Z, Yaw: loc.Yaw, Pitch: loc.Pitch, Set: true}
	cl.Settings.Position = rel
	if ps.DB != nil {
		_ = ps.DB.SetClusterPosition(cl, rel)
	}
	ps.sendCaption(p, "position.position_set")
}
