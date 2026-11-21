# Plots for golem

An unofficial port of [PlotSquared](https://github.com/IntellectualSites/PlotSquared) to
[golem](https://github.com/golem-mc/golem), the Minecraft: Java Edition server written in Go.
It is not made or endorsed by IntellectualSites.

## Install

```bash
golem-launcher plugin add github.com/golem-mc/worldedit/worldeditplugin
golem-launcher plugin add github.com/golem-mc/plots/plotsplugin
```

`worldedit` is required. Optional integrations:

```bash
golem-launcher plugin add github.com/golem-mc/vault/vaultplugin
```

## Migrating from Bukkit or Paper

Copy your existing `plugins/PlotSquared` directory to `plugins/plots`. Existing
`config/settings.yml`, `config/worlds.yml`, `config/storage.yml`, `storage.db`
(SQLite), `schematics/`, and `lang/` files load directly. If `plugins/PlotSquared`
is present on startup without `plugins/plots`, the plugin logs a one-line reminder.

## License

Like PlotSquared, this port is licensed under the GNU General Public License v3 ([LICENSE](LICENSE)).
