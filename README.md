# Plots for golem

An unofficial port of [PlotSquared](https://github.com/IntellectualSites/PlotSquared) to
[golem](https://github.com/golem-mc/golem), the Minecraft: Java Edition server written in Go.
It provides plot world generation, claiming, merging, clusters, protection flags, ratings,
comments, schematics, backups, Vault economy, and WorldEdit region restrictions.
It is not made or endorsed by IntellectualSites.

## Install

```bash
golem-launcher plugin add github.com/golem-mc/worldedit/worldeditplugin
golem-launcher plugin add github.com/golem-mc/plots/plotsplugin
```

`worldedit` is required. Optional integrations:

```bash
golem-launcher plugin add github.com/golem-mc/vault/vaultplugin   # plot claim/merge/buy/sell economy
golem-launcher plugin add github.com/golem-mc/perms/permsplugin   # fine-grained permissions
```

## Getting Started

1. Create a plot world as an operator or from the server console:
   ```
   /plot setup plots
   ```
   You can pass size and road overrides inline, e.g. `/plot setup plots plot.size=32 road.width=5 plot.height=62`.
2. Teleport to the new plot world:
   ```
   /plot area tp plots
   ```
3. Claim the nearest free plot (or stand inside an unclaimed plot and run `/plot claim`):
   ```
   /plot auto
   ```
4. Trust a friend and customize your plot:
   ```
   /plot trust Steve
   /plot flag set pvp true
   /plot set floor smooth_quartz
   ```

## Commands

All commands work under `/plot` and its aliases: `/p`, `/ps`, `/plots`, `/plotsquared`, `/p2`, `/2`, `/plotme`.
Prefix any subcommand with `<X;Z>` or `<world;X;Z>` to target a specific plot (e.g. `/p 1;1 info`), or `-f` to skip confirmation when permitted.

### Claiming & Plot Management

| Command | Aliases | Description | Permission |
|---|---|---|---|
| `/plot claim [schematic]` | `c` | Claim the plot you are standing in | `plots.claim` |
| `/plot auto [length,width] [schematic]` | `a` | Claim the nearest free plot or `NxM` mega-plot | `plots.auto` (`plots.auto.mega` for multi-plot) |
| `/plot delete` | `del`, `dispose`, `unclaim` | Delete and unclaim the current plot | `plots.delete` |
| `/plot clear` | `reset` | Reset the current plot to default terrain | `plots.clear` |
| `/plot setowner <player \| none>` | `owner`, `so`, `seto` | Transfer or clear plot ownership | `plots.admin.command.setowner` |
| `/plot buy [plot]` | — | Purchase a plot listed with the `price` flag | `plots.buy` |
| `/plot grant <check \| add> <player>` | — | Check or add bonus plot grants for a player | `plots.grant`, `plots.grant.check`, `plots.grant.add` |
| `/plot copy <X;Z>` | `copypaste` | Copy the current plot to an unowned target plot | `plots.copy` |
| `/plot move <X;Z>` | — | Move the current plot to an unowned target plot | `plots.move` |
| `/plot swap <X;Z>` | `switch` | Swap blocks and ownership with another plot | `plots.swap` |
| `/plot leave` | — | Remove yourself from a plot you are added to | `plots.leave` |

### Navigation & Teleportation

| Command | Aliases | Description | Permission |
|---|---|---|---|
| `/plot home [page \| alias \| area;x;y]` | `h` | Teleport to one of your plots | `plots.home` |
| `/plot visit <player \| alias \| plot> [area] [#]` | `v`, `tp`, `teleport`, `goto`, `warp` | Teleport to another player's plot | `plots.visit` |
| `/plot middle` | `center`, `centre` | Teleport to the center of the current plot | `plots.middle` |
| `/plot sethome [none]` | `sh`, `seth` | Set or reset the plot's home location | `plots.set.home` |
| `/plot kick <player \| *>` | `k` | Kick player(s) from the current plot | `plots.kick` |
| `/plot target <<X;Z> \| nearest>` | — | Point your compass at a plot | `plots.target` |

### Members & Merging

| Command | Aliases | Description | Permission |
|---|---|---|---|
| `/plot trust <player \| *>` | `t` | Allow a player to build even while you are offline | `plots.trust` |
| `/plot add <player \| *>` | — | Allow a player to build while you are online | `plots.add` |
| `/plot deny <player \| *>` | `d`, `ban` | Block a player from entering the plot and kick them | `plots.deny` |
| `/plot remove <player \| *>` | `r`, `untrust`, `ut`, `undeny`, `unban`, `ud`, `pardon` | Remove a trusted, added, or denied player | `plots.remove` |
| `/plot merge <all \| n \| e \| s \| w> [removeroads]` | `m` | Merge adjacent plots | `plots.merge` |
| `/plot unlink [createroads]` | `u`, `unmerge` | Unlink a merged plot group | `plots.unlink` |

### Appearance, Flags, Info & Chat

| Command | Aliases | Description | Permission |
|---|---|---|---|
| `/plot info [id] [category] [-f]` | `i` | Display plot info or a specific field | `plots.info` |
| `/plot list <mine \| shared \| world \| area \| all \| top \| forsale \| unowned \| done \| expired \| fuzzy <term> \| player> [#]` | `l`, `find`, `search` | List plots matching a filter | `plots.list`, `plots.list.<type>` |
| `/plot near` | `n` | List players inside the current plot | `plots.use` |
| `/plot caps` | — | Show current entity counts vs. plot cap flags | `plots.use` |
| `/plot set <floor \| wall \| border \| all \| air \| outline \| middle \| main \| biome \| alias \| home \| flag \| desc> <value>` | `s` | Change a plot component or property | `plots.set`, `plots.set.<component>` |
| `/plot biome [biome]` | `setbiome`, `sb`, `b` | Change the plot's biome | `plots.set.biome` |
| `/plot alias <set \| remove> <alias>` | `setalias`, `sa`, `name`, `rename` | Set or remove a plot alias | `plots.alias` |
| `/plot desc [description]` | `setdescription`, `setdesc`, `description` | Set or clear the plot description | `plots.set.desc` |
| `/plot flag <set \| add \| remove \| list \| info> [flag] [value]` | `f` | Manage plot protection and behavior flags | `plots.flag`, `plots.set.flag.<flag>.<value>` |
| `/plot music [disc \| none]` | — | Open the plot music disc menu or set a disc | `plots.music` |
| `/plot toggle <chat \| chatspy \| clear-confirmation \| time \| titles \| worldedit \| debug>` | `attribute` | Toggle personal plot settings | `plots.toggle`, `plots.toggle.<setting>` |
| `/plot chat [message]` | — | Toggle plot chat or send a message to plot chat | `plots.chat` |
| `/plot confirm` | — | Confirm a pending destructive command | `plots.confirm` |
| `/plot help [category \| page \| command]` | `?` | Show paginated command help | `plots.use` |

### Ratings, Comments & Clusters

| Command | Aliases | Description | Permission |
|---|---|---|---|
| `/plot rate <1-10 \| next \| purge>` | `rt` | Rate a plot from 1–10 or jump to the next unrated plot | `plots.rate` |
| `/plot like [next \| purge]` | — | Like the current plot | `plots.like` |
| `/plot dislike [next \| purge]` | — | Dislike the current plot | `plots.dislike` |
| `/plot comment [X;Z] <public \| owner \| report> <message>` | `msg` | Post a comment to a plot inbox | `plots.comment`, `plots.inbox.write.<inbox>` |
| `/plot inbox [inbox] [delete <#> \| clear \| page]` | — | Read or manage plot inbox comments | `plots.inbox`, `plots.inbox.read.<inbox>` |
| `/plot done` | `submit` | Mark a plot as completed | `plots.done` |
| `/plot continue` | — | Re-open a completed plot for building | `plots.continue` |
| `/plot cluster <list \| create \| delete \| resize \| invite \| kick \| leave \| helpers \| info \| tp \| sethome>` | `clusters` | Manage multi-plot clusters | `plots.cluster`, `plots.cluster.<action>` |

### Schematics & Backups

| Command | Aliases | Description | Permission |
|---|---|---|---|
| `/plot schematic <save [name] \| saveall <area> \| paste <name> \| list>` | `schem` | Save, paste, or list `.schem` schematics | `plots.schematic`, `plots.schematic.<action>` |
| `/plot load <schematic>` | `restore` | Paste a saved schematic into your plot | `plots.load` |
| `/plot backup <save \| list \| load <#>>` | — | Save, list, or restore per-plot backups | `plots.backup`, `plots.backup.<action>` |
| `/plot createroadschematic` | `crs` | Capture custom road schematics from the current plot | `plots.createroadschematic` |
| `/plot debugroadregen <plot \| region>` | — | Regenerate roads around a plot or nearby chunks | `plots.debugroadregen` |
| `/plot regenallroads <world>` | `rgar` | Regenerate roads across a plot world | `plots.regenallroads` |

### Administration & Maintenance

| Command | Aliases | Description | Permission |
|---|---|---|---|
| `/plot area <create <world[:id]> [key=val...] \| info [area] \| list \| tp <area> \| regen>` | `world` | Create and manage plot worlds and partial/augmented areas | `plots.area`, `plots.area.<action>` |
| `/plot setup <world> [key=val...]` | `create` | Create a new plot world | `plots.admin.command.setup` |
| `/plot template <import <world> <template> \| export <world> \| list>` | — | Import or export `.template` plot world archives (`skyblock`, `town`, `bridge` bundled) | `plots.admin`, `plots.template.<action>` |
| `/plot purge <filter:value...>` | — | Bulk-delete plots by `world`, `area`, `id`, `owner`, `shared`, `unknown`, `clear` | `plots.admin` |
| `/plot condense <area> <start <radius> \| stop \| info>` | — | Move outer plots inward toward `(0;0)` | `plots.admin` |
| `/plot trim <world>` | — | Report owned plots in a world | `plots.admin` |
| `/plot reload` | `rl` | Reload configs, messages, and road schematics | `plots.admin.command.reload` |
| `/plot plugin` | `version` | Show plugin version and plot counts | `plots.use` |
| `/plot debug` | — | Show areas, plot counts, database, economy, and expiry status | `plots.admin` |

## Permissions

Permission nodes match upstream PlotSquared (`plots.*`):

- `plots.use` (default: everyone) — access to `/plot` and `/plot help`.
- `plots.permpack.basic` (default: false) — grants everyday player permissions (claim, auto, home, visit, clear, delete, trust/add/deny/remove, merge up to 4, flags, alias, desc, done, chat, toggle).
- `plots.permpack.basicinbox` (default: false) — grants `public`, `owner`, and `report` inbox read/write access.
- `plots.plot.<n>` — maximum plots a player may claim (e.g. `plots.plot.4`, or `plots.plot.*`).
- `plots.merge.<n>` — maximum merged plot group size (e.g. `plots.merge.4`).
- `plots.set.flag.<flag>.<value>` — permission to set a specific flag value (or `plots.set.flag.*`).
- `plots.admin` (default: op) — grants all `plots.admin.*` bypass and management permissions.
- `plots.worldedit.bypass` (default: op) — bypass WorldEdit plot region restrictions (toggleable with `/plot toggle worldedit`).

## Configuration

All configuration files live under `plugins/plots/`:

- `plugins/plots/config/settings.yml` — global settings:
  - `limit.global` (`true`), `limit.max-plots` (`127`)
  - `teleport.on-claim` (`true`), `teleport.on-auto` (`true`), `teleport.delay` (`0`)
  - `backup.automatic-backups` (`true`), `backup.backup-limit` (`3`), `backup.delete-on-unclaim` (`true`)
  - `confirmation.confirmation-timeout-seconds` (`20`)
  - `done.restrict-building` (`true`), `done.required-for-ratings` (`false`)
  - `enabled_components.worldedit-restrictions` (`true`), `economy` (`true`), `plot-expiry` (`false`)
  - `auto_clear` — plot expiry schedules (`days`, `threshold`, `worlds`, `calibration`)
- `plugins/plots/config/worlds.yml` — per-world and partial-area geometry (`plot.size: 42`, `road.width: 7`, `plot.height: 62`, `plot.floor: minecraft:grass_block`, `plot.filling: minecraft:stone`, `road.block: minecraft:quartz_block`, `wall.block: minecraft:smooth_stone_slab`, `wall.block_claimed: minecraft:sandstone_slab`, `economy.use`, `economy.prices`, `flags`, `road.flags`, `areas`).
- `plugins/plots/config/storage.yml` — SQLite database file (`sqlite.db: storage`, optional table `prefix`).
- `plugins/plots/lang/messages_en.json` — editable MiniMessage translations (written on first run).

## Migrating from Bukkit or Paper

1. Copy your existing `plugins/PlotSquared` directory to `plugins/plots`.
2. Existing `config/settings.yml`, `config/worlds.yml`, `config/storage.yml`, SQLite `storage.db` (same 13-table schema), `schematics/`, `templates/`, and `lang/` files load directly.
3. If `plugins/PlotSquared` exists on startup without `plugins/plots`, the plugin logs a one-line reminder.
4. When using a world manager such as Multiverse, specify `plots` (or `plots:s=42,g=7,h=62`) as the world generator.

## For Plugin Developers

Other golem plugins can query plots and listen to plot events by importing the API module without registering the plugin:

```bash
go get github.com/golem-mc/plots
```

```go
package example

import (
	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/event"
)

func setup(s plugin.Server) {
	// Look up the live plots API service
	if api, ok := plots.Lookup(s); ok {
		for _, area := range api.PlotAreas() {
			s.Logger().Info("loaded plot area", "area", area.FullID())
		}
	}

	// Listen to plot events on golem's event bus
	plugin.Listen(s, func(e *event.PlayerClaimPlotEvent) {
		if e.Plot.ID.X == 0 && e.Plot.ID.Y == 0 {
			e.SetCancelled(true)
		}
	})
}
```

## Differences from PlotSquared

- Registered under the plugin name `plots` with data directory `plugins/plots` (commands `/plot`, `/p`, `/ps`, `/plotsquared`, `plots.*` permissions, and file formats are unchanged).
- Storage uses pure-Go SQLite only; MySQL is not supported (if `mysql.use: true` is set in `storage.yml`, a warning is logged and SQLite is used).
- Web schematic upload and download services (Arkitektonika / legacy web interface) are omitted; `/plot schematic save` and `/plot schematic paste` operate on local `.schem` files via `golem-mc/worldedit`.
- Hooks for Bukkit plugins without golem ports (PlaceholderAPI, MVdWPlaceholderAPI, HolographicDisplays, Towny) are omitted; `golem-mc/worldedit` and `golem-mc/vault` are integrated directly.

## License

Like PlotSquared, this port is licensed under the GNU General Public License v3 ([LICENSE](LICENSE)).
