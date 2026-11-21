package plotsplugin

import (
	"fmt"
	"strings"

	"github.com/golem-mc/golem/plugin"
)

func (ps *Plugin) registerPermissions() {
	perms := ps.Server.Permissions()
	op := plugin.PermissionDefault{OpLevel: 2}
	everyone := plugin.PermissionDefault{OpLevel: 0}
	nobody := plugin.PermissionDefault{OpLevel: 5}

	perms.Register(plugin.Permission{Name: "plots.use", Default: everyone})

	opNodes := []string{
		"plots.projectile.unowned", "plots.projectile.other",
		"plots.toggle.titles", "plots.toggle.chat", "plots.toggle.time",
		"plots.merge.other", "plots.merge.keeproad.denied", "plots.trust.everyone",
		"plots.deny.everyone", "plots.area.create", "plots.area.info", "plots.area.list",
		"plots.area.regen", "plots.area.tp", "plots.auto.mega", "plots.continue",
		"plots.cluster", "plots.cluster.list", "plots.cluster.create", "plots.cluster.create.other",
		"plots.cluster.size", "plots.cluster.delete.other", "plots.cluster.resize",
		"plots.cluster.resize.other", "plots.cluster.resize.shrink", "plots.cluster.resize.expand",
		"plots.cluster.invite", "plots.cluster.invite.other", "plots.cluster.kick",
		"plots.cluster.kick.other", "plots.cluster.leave", "plots.cluster.tp", "plots.cluster.tp.other",
		"plots.cluster.sethome", "plots.cluster.sethome.other",
		"plots.set", "plots.set.flag", "plots.set.flag.other", "plots.set.biome",
		"plots.set.alias", "plots.set.home", "plots.set.main", "plots.set.floor",
		"plots.set.air", "plots.set.all", "plots.set.border", "plots.set.wall",
		"plots.set.outline", "plots.set.middle",
		"plots.flag.remove", "plots.flag.list",
		"plots.list.forsale", "plots.list.mine", "plots.list.shared", "plots.list.world",
		"plots.list.top", "plots.list.all", "plots.list.unowned", "plots.list.player",
		"plots.list.done", "plots.list.expired", "plots.list.fuzzy", "plots.list.area",
		"plots.target", "plots.comment", "plots.schematic.paste", "plots.schematic.list",
		"plots.schematic.save", "plots.visit.shared", "plots.visit.unowned", "plots.visit.owned",
		"plots.visit.other", "plots.visit.denied", "plots.home", "plots.alias.remove",
		"plots.backup", "plots.backup.save", "plots.backup.list", "plots.backup.load",
		"plots.rate", "plots.template", "plots.template.export", "plots.template.import",
		"plots.components", "plots.teleport.delay.bypass", "plots.admin.border.bypass",
		"plots.worldedit.bypass", "plots.gamemode.bypass", "plots.confirm.bypass",
		"plots.flag.notify-enter.bypass", "plots.flag.notify-leave.bypass",
	}
	for _, n := range opNodes {
		perms.Register(plugin.Permission{Name: n, Default: op})
	}

	adminChildren := map[string]bool{
		"plots.admin.command.continue": true, "plots.admin.command.delete": true,
		"plots.admin.command.deny": true, "plots.admin.command.done": true,
		"plots.admin.command.download": true, "plots.admin.command.kick": true,
		"plots.admin.command.load": true, "plots.admin.command.merge": true,
		"plots.admin.command.setowner": true, "plots.admin.command.remove": true,
		"plots.admin.command.save": true, "plots.admin.command.schematic.paste": true,
		"plots.admin.command.schematic.save": true, "plots.admin.command.unlink": true,
		"plots.admin.alias.set": true, "plots.admin.alias.remove": true,
		"plots.admin.chat.bypass": true, "plots.admin.backup.other": true,
		"plots.admin.unsafe": true, "plots.admin.debug.other": true,
		"plots.admin.area.sudo": true, "plots.admin.interact.blockedcommands": true,
		"plots.admin.exit.denied": true, "plots.admin.entry.denied": true,
		"plots.admin.visit.untrusted": true, "plots.admin.entry.forcefield": true,
		"plots.admin.command.chatspy": true, "plots.admin.caps.other": true,
		"plots.admin.music.other": true, "plots.admin.destroy.unowned": true,
		"plots.admin.destroy.groundlevel": true, "plots.admin.destroy.other": true,
		"plots.admin.destroy.road": true, "plots.admin.build.road": true,
		"plots.admin.build.unowned": true, "plots.admin.build.other": true,
		"plots.admin.interact.road": true, "plots.admin.interact.unowned": true,
		"plots.admin.interact.other": true, "plots.admin.build.heightlimit": true,
		"plots.admin.command.purge.ratings": true, "plots.admin.command.trust": true,
		"plots.admin.command.add": true, "plots.admin.info.force": true,
		"plots.admin.command.schematic": true, "plots.admin.command.clear": true,
		"plots.admin.flight": true, "plots.admin.command.blocked-cmds.shared": true,
		"plots.admin.command.blocked-cmds.road": true, "plots.admin.components.other": true,
		"plots.admin.vehicle.break.unowned": true, "plots.admin.pve.unowned": true,
		"plots.admin.border.bypass": true,
	}
	perms.Register(plugin.Permission{Name: "plots.admin", Default: op, Children: adminChildren})

	basicChildren := map[string]bool{
		"plots.use": true, "plots.info": true, "plots.claim": true, "plots.auto": true,
		"plots.home": true, "plots.clear": true, "plots.delete": true, "plots.music": true,
		"plots.list": true, "plots.list.mine": true, "plots.list.shared": true,
		"plots.list.world": true, "plots.list.all": true, "plots.list.forsale": true,
		"plots.list.unowned": true, "plots.set": true, "plots.visit": true,
		"plots.visit.owned": true, "plots.visit.shared": true, "plots.set.flag": true,
		"plots.flag.add": true, "plots.flag.remove": true, "plots.flag.list": true,
		"plots.flag.info": true, "plots.flag": true, "plots.buy": true, "plots.chat": true,
		"plots.confirm": true, "plots.toggle": true, "plots.toggle.titles": true,
		"plots.toggle.chat": true, "plots.toggle.time": true, "plots.set.biome": true,
		"plots.set.home": true, "plots.alias.set": true, "plots.alias.remove": true,
		"plots.set.description": true, "plots.description": true, "plots.alias": true,
		"plots.merge": true, "plots.merge.other": true, "plots.merge.4": true,
		"plots.unlink": true, "plots.denied": true, "plots.add": true, "plots.trust": true,
		"plots.deny": true, "plots.remove": true, "plots.untrust": true, "plots.undeny": true,
		"plots.kick": true, "plots.download": true, "plots.save": true, "plots.done": true,
		"plots.continue": true, "plots.middle": true,
	}
	perms.Register(plugin.Permission{Name: "plots.permpack.basic", Default: nobody, Children: basicChildren})

	basicInboxChildren := map[string]bool{
		"plots.inbox.read.public": true, "plots.inbox.modify.public": true, "plots.inbox.write.public": true,
		"plots.inbox.read.report": true, "plots.inbox.write.report": true,
		"plots.inbox.read.owner": true, "plots.inbox.modify.owner": true, "plots.inbox.write.owner": true,
		"plots.comment": true, "plots.inbox": true,
	}
	perms.Register(plugin.Permission{Name: "plots.permpack.basicinbox", Default: nobody, Children: basicInboxChildren})
}

// HasPerm checks whether sender has node, falling back through wildcard prefixes
// (e.g. plots.admin, plots.*) and operator level >= 2.
func HasPerm(sender plugin.CommandSender, node string) bool {
	if sender == nil {
		return true
	}
	if sender.HasPermission(node) {
		return true
	}
	if sender.PermissionSet(node) {
		return false
	}
	// Check wildcard parents like plots.set.flag.* or plots.*
	parts := strings.Split(node, ".")
	for i := len(parts) - 1; i >= 1; i-- {
		wc := strings.Join(parts[:i], ".") + ".*"
		if sender.HasPermission(wc) {
			return true
		}
	}
	if strings.HasPrefix(node, "plots.admin.") && sender.HasPermission("plots.admin") {
		return true
	}
	return sender.OpLevel() >= 2
}

// HasPermRange checks numbered permissions like "plots.plot.<n>" up to maxVal
// (returning maxVal for ops or wildcard "plots.plot.*").
func HasPermRange(sender plugin.CommandSender, stub string, maxVal int) int {
	if sender == nil || sender.OpLevel() >= 2 {
		return maxVal
	}
	if sender.HasPermission(stub+".*") || sender.HasPermission("plots.admin") {
		return maxVal
	}
	for i := maxVal; i > 0; i-- {
		if sender.HasPermission(fmt.Sprintf("%s.%d", stub, i)) {
			return i
		}
	}
	return 0
}
