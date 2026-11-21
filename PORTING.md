# Porting PlotSquared to golem

This repository (`github.com/golem-mc/plots`) ports
[PlotSquared](https://github.com/IntellectualSites/PlotSquared) (Core + Bukkit)
to [golem](https://github.com/golem-mc/golem) under the plugin name `plots`.

## Layout

| Path | What | Java counterpart |
|---|---|---|
| `/` (package `plots`) | Public API: `PlotId`, `Plot`, `PlotArea`, `PlotCluster`, `BlockBucket`, flag registry and `API` service | `plot/`, `plot/flag/`, `location/` |
| `event/` | Plot plugin events registered on golem's event bus | `events/` |
| `internal/lang/` | Embedded `messages_en.json`, locale loader and MiniMessage-to-`plugin.Text` parser | `configuration/caption/`, `resources/lang/messages_en.json` |
| `internal/config/` | `config/settings.yml`, `config/storage.yml`, `config/worlds.yml` and bundled `.template` archives | `Settings`, `Storage`, `PlotArea` YAML serialization |
| `internal/storage/` | Pure-Go SQLite (`modernc.org/sqlite`) storage using upstream's exact 13-table schema | `database/SQLManager`, `database/SQLite` |
| `internal/generator/` | `HybridGenerator` (`plugin.Generator`), road/plot schematic overlays, plot terrain ops (clear, components, merge/unlink roads, copy/swap, road schematics) | `generator/HybridGen`, `HybridPlotWorld`, `ClassicPlotManager`, `HybridPlotManager` |
| `plotsplugin/` | The `plots` plugin: lifecycle, `GeneratorProvider`, permissions, plot manager, WorldEdit region restrictions, protection/movement/entity listeners, Vault economy, `/plot` commands & tab completion | `BukkitPlatform`, `PlotSquared`, `WESubscriber`, `listener/*`, `PlotListener`, `command/*` |

## Decisions

- **Plugin rename to `plots`**: The Go module is `github.com/golem-mc/plots`, the
  root API package is `plots`, the plugin package is `plotsplugin`, and the
  registered golem plugin name is `"plots"` (data directory `plugins/plots`).
  Upstream command names and aliases (`/plot`, `/p`, `/ps`, `/plotsquared`),
  permission nodes (`plots.*`), and file formats (`config/*.yml`, `storage.db`)
  are preserved unchanged. On startup, if `plugins/PlotSquared` exists and
  `plugins/plots` does not, a one-line migration hint is logged.
- **API vs plugin split**: Like `golem-mc/vault` and `golem-mc/worldguard`, the root package
  `github.com/golem-mc/plots` exports the public API and flag registry without
  registering a plugin; the plugin itself lives in `plotsplugin/`.
- **SQLite compatibility**: `internal/storage` creates and reads upstream's exact 13 SQLite
  tables (`plot`, `plot_denied`, `plot_helpers`, `plot_trusted`, `plot_comments`,
  `plot_settings`, `plot_rating`, `cluster`, `cluster_helpers`, `cluster_invited`,
  `cluster_settings`, `player_meta`, `plot_flags`), preserving upstream's historical
  mapping (`plot_helpers` stores trusted players, `plot_trusted` stores members),
  `HashUtil` merge bitmask (`N=8, E=4, S=2, W=1`), `PlotId.hashCode()` for comments, and
  `DBFunc.EVERYONE` (`00000001-0001-0003-0003-000000000007`).
- **MySQL omitted**: Only SQLite is supported (pure-Go `modernc.org/sqlite`). If
  `storage.yml` has `mysql.use: true`, a warning is logged and SQLite is used instead.
- **Web schematic services omitted**: Upstream's Arkitektonika / legacy web upload/download
  services are not ported; schematics are saved and loaded locally via `golem-mc/worldedit`.
- **Unsupported plugin hooks omitted**: Hooks for plugins without golem ports
  (PlaceholderAPI, MVdWPlaceholderAPI, BungeePerms, Towny, HolographicDisplays, FAWE-specific
  internal hooks) are omitted; `golem-mc/worldedit` and `golem-mc/vault` are used directly.
- **MiniMessage rendering**: Upstream's `messages_en.json` is embedded directly and written
  to `plugins/plots/lang/messages_en.json` on first run; a lightweight MiniMessage
  parser renders colors, `#rrggbb`, decorations, `<click:...>` and `<hover:show_text:...>`
  directly to `plugin.Text`.
