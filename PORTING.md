# Porting PlotSquared to golem

This repository ports [PlotSquared](https://github.com/IntellectualSites/PlotSquared)
(Core + Bukkit) to [golem](https://github.com/golem-mc/golem).

## Layout

| Path | What | Java counterpart |
|---|---|---|
| `/` (package `plotsquared`) | Public API: `PlotId`, `Plot`, `PlotArea`, `PlotCluster`, `BlockBucket`, flag registry and `API` service | `plot/`, `plot/flag/`, `location/` |
| `event/` | PlotSquared plugin events registered on golem's event bus | `events/` |
| `internal/lang/` | Embedded `messages_en.json`, locale loader and MiniMessage-to-`plugin.Text` parser | `configuration/caption/`, `resources/lang/messages_en.json` |
| `internal/config/` | `config/settings.yml`, `config/storage.yml`, `config/worlds.yml` and bundled `.template` archives | `Settings`, `Storage`, `PlotArea` YAML serialization |
| `internal/storage/` | Pure-Go SQLite (`modernc.org/sqlite`) storage using upstream's exact 13-table schema | `database/SQLManager`, `database/SQLite` |
| `internal/generator/` | `HybridGenerator` (`plugin.Generator`), road/plot schematic overlays, plot terrain ops (clear, components, merge/unlink roads, copy/swap, road schematics) | `generator/HybridGen`, `HybridPlotWorld`, `ClassicPlotManager`, `HybridPlotManager` |
| `plotsquaredplugin/` | The `plotsquared` plugin: lifecycle, `GeneratorProvider`, permissions, plot manager, WorldEdit region restrictions, protection/movement/entity listeners, Vault economy | `BukkitPlatform`, `PlotSquared`, `WESubscriber`, `listener/*`, `PlotListener` |

## Decisions

- **API vs plugin split**: Like `golem-mc/vault` and `golem-mc/worldguard`, the root package
  `github.com/golem-mc/plotsquared` exports the public API and flag registry without
  registering a plugin; the plugin itself will live in `plotsquaredplugin/`.
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
  to `plugins/plotsquared/lang/messages_en.json` on first run; a lightweight MiniMessage
  parser renders colors, `#rrggbb`, decorations, `<click:...>` and `<hover:show_text:...>`
  directly to `plugin.Text`.
