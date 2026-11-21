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
| `internal/backup/` | Per-plot schematic backups with automatic backup hooks and backup-limit pruning | `backup/SimpleBackupManager`, `PlayerBackupProfile` |
| `internal/expiry/` | Plot complexity analysis and automatic plot expiry evaluation | `plot/expiration/ExpireManager`, `PlotAnalysis` |
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

## Self-Audit

### Core Subsystems

| Subsystem | Implementation | Test Coverage |
|---|---|---|
| Plot model, geometry, flags, permissions, pricing | `plots.go`, `area.go`, `flag.go`, `cluster.go` | `plots_test.go` (`TestPlotIdSpiralAndParsing`, `TestPlotAreaGeometryAndMergedPlotAt`, `TestFlagRegistryAndInheritance`, `TestBlockBucketAndPriceEval`) |
| Configuration & templates | `internal/config/` | `internal/config/config_test.go` (`TestSettingsAndStorageRoundTrip`, `TestWorldsConfigAndPartialAreas`, `TestBundledTemplatesCopy`) |
| Localization & MiniMessage | `internal/lang/` | `internal/lang/lang_test.go` (`TestCatalogLoadAndFormat`, `TestMiniMessageParsing`) |
| SQLite storage (upstream 13-table schema) | `internal/storage/sqlite.go` | `internal/storage/sqlite_test.go` (`TestSQLiteCRUDAndUpstreamSchema`, `TestLoadUpstreamPlotSquaredSQLiteDB`) |
| Generator (`HybridPlotWorld` / `HybridGen`) & world ops | `internal/generator/` | `internal/generator/hybrid_test.go` (`TestHybridGenDefaultWorld`, `TestHybridGenNonDefaultConfigs`, `TestHybridGenRoadAndPlotSchematics`, `TestApplyGeneratorSpec`), `internal/generator/ops_test.go` (`TestGeneratorWorldOps`) |
| Backups | `internal/backup/backup.go` | `internal/backup/backup_test.go` (`TestBackupSaveListRestorePruneAndDelete`), `plotsplugin/tools_test.go` (`TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry`) |
| Plot expiry & complexity analysis | `internal/expiry/expiry.go` | `internal/expiry/expiry_test.go` (`TestAnalyzePlotAndComplexity`, `TestExpiryRulesAndExpiredPlots`), `plotsplugin/tools_test.go` (`TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry`) |
| Lifecycle, `GeneratorProvider`, `plots.API`, legacy dir hint, WorldEdit hook | `plotsplugin/plugin.go`, `plotsplugin/worldedit.go` | `plotsplugin/plugin_test.go` (`TestGeneratorLifecycleAndPlotOps`, `TestRoadSchematicGeneration`, `TestLegacyPlotSquaredDataDirHint`) |

### Upstream Commands (`PlotSquared/Core/.../core/command/*.java`)

Framework/support classes (`Argument`, `CmdConfirm`, `CmdInstance`, `Command`, `CommandCaller`, `CommandCategory`, `CommandDeclaration`, `MainCommand`, `RequiredType`, `SetCommand`, `SubCommand`) are implemented in `plotsplugin/commands.go` and covered across all command tests below (including prefix `<x;z>`, `-f`, confirmation, and `TabComplete`).

| Upstream Command Class | Implementation | Covered By Test / Issue |
|---|---|---|
| `Add.java` | `plotsplugin/cmd_members_tp.go` (`/plot add`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestProtectionMatrixRolesAndLocations` |
| `Alias.java` | `plotsplugin/cmd_info_flags.go` (`/plot alias`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestCommandsBuySetOwnerMoveSwapAutoMegaAndKick` |
| `Area.java` | `plotsplugin/cmd_admin.go` (`/plot area`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Auto.java` | `plotsplugin/cmd_claiming.go` (`/plot auto`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestConfirmMergeUnlinkClearDeleteGrantAndCopySwap`, `TestCommandsBuySetOwnerMoveSwapAutoMegaAndKick` |
| `Backup.java` | `plotsplugin/cmd_schematics_backups.go` (`/plot backup`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry`, `TestBackupSaveListRestorePruneAndDelete` |
| `Biome.java` | `plotsplugin/cmd_info_flags.go` (`/plot biome`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Buy.java` | `plotsplugin/cmd_claiming.go` (`/plot buy`) | `TestCommandsBuySetOwnerMoveSwapAutoMegaAndKick`, `TestVaultEconomyBuyPlot` |
| `Caps.java` | `plotsplugin/cmd_info_flags.go` (`/plot caps`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport` |
| `Claim.java` | `plotsplugin/cmd_claiming.go` (`/plot claim`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport` |
| `Clear.java` | `plotsplugin/cmd_claiming.go` (`/plot clear`) | `TestConfirmMergeUnlinkClearDeleteGrantAndCopySwap`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Cluster.java` | `plotsplugin/cmd_social.go` (`/plot cluster`) | `TestSocialRatingsCommentsDoneAndClusters`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Comment.java` | `plotsplugin/cmd_social.go` (`/plot comment`) | `TestSocialRatingsCommentsDoneAndClusters`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Condense.java` | `plotsplugin/cmd_admin.go` (`/plot condense`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry` |
| `Confirm.java` | `plotsplugin/commands.go` (`/plot confirm`) | `TestConfirmMergeUnlinkClearDeleteGrantAndCopySwap` |
| `Continue.java` | `plotsplugin/cmd_social.go` (`/plot continue`) | `TestSocialRatingsCommentsDoneAndClusters` |
| `Copy.java` | `plotsplugin/cmd_claiming.go` (`/plot copy`) | `TestConfirmMergeUnlinkClearDeleteGrantAndCopySwap` |
| `CreateRoadSchematic.java` | `plotsplugin/cmd_schematics_backups.go` (`/plot createroadschematic`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry`, `TestGeneratorWorldOps` |
| `DatabaseCommand.java` | `plotsplugin/cmd_admin.go` (`/plot database`, SQLite status; MySQL omitted) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry` |
| `Debug.java` | `plotsplugin/cmd_admin.go` (`/plot debug`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry` |
| `DebugAllowUnsafe.java` | `plotsplugin/cmd_debug.go` (`/plot debugallowunsafe`) | `TestDebugAllowUnsafeExecLoadAndSaveTests` |
| `DebugExec.java` | `plotsplugin/cmd_debug.go` (`/plot debugexec`) | `TestDebugAllowUnsafeExecLoadAndSaveTests` |
| `DebugImportWorlds.java` | Depends on `SinglePlotArea` | Tracked in golem-mc/plots#12 |
| `DebugLoadTest.java` | `plotsplugin/cmd_debug.go` (`/plot debugloadtest`) | `TestDebugAllowUnsafeExecLoadAndSaveTests` |
| `DebugPaste.java` | Deliberately omitted (external web paste service) | See Decisions above |
| `DebugRoadRegen.java` | `plotsplugin/cmd_schematics_backups.go` (`/plot debugroadregen`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry` |
| `DebugSaveTest.java` | `plotsplugin/cmd_debug.go` (`/plot debugsavetest`) | `TestDebugAllowUnsafeExecLoadAndSaveTests` |
| `Delete.java` | `plotsplugin/cmd_claiming.go` (`/plot delete`) | `TestConfirmMergeUnlinkClearDeleteGrantAndCopySwap` |
| `Deny.java` | `plotsplugin/cmd_members_tp.go` (`/plot deny`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestProtectionMatrixRolesAndLocations` |
| `Desc.java` | `plotsplugin/cmd_info_flags.go` (`/plot desc`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Dislike.java` | `plotsplugin/cmd_social.go` (`/plot dislike`) | `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Done.java` | `plotsplugin/cmd_social.go` (`/plot done`) | `TestSocialRatingsCommentsDoneAndClusters`, `TestProtectionMatrixRolesAndLocations` |
| `Download.java` | `plotsplugin/cmd_schematics_backups.go` (`/plot download`, explains web upload omitted) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry` |
| `FlagCommand.java` | `plotsplugin/cmd_info_flags.go` (`/plot flag`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestCommandsBuySetOwnerMoveSwapAutoMegaAndKick` |
| `Grant.java` | `plotsplugin/cmd_claiming.go` (`/plot grant`) | `TestConfirmMergeUnlinkClearDeleteGrantAndCopySwap` |
| `Help.java` | `plotsplugin/commands.go` (`/plot help`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport` |
| `HomeCommand.java` | `plotsplugin/cmd_members_tp.go` (`/plot home`) | `TestCommandsBuySetOwnerMoveSwapAutoMegaAndKick` |
| `Inbox.java` | `plotsplugin/cmd_social.go` (`/plot inbox`) | `TestSocialRatingsCommentsDoneAndClusters`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Info.java` | `plotsplugin/cmd_info_flags.go` (`/plot info`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport` |
| `Kick.java` | `plotsplugin/cmd_members_tp.go` (`/plot kick`) | `TestCommandsBuySetOwnerMoveSwapAutoMegaAndKick` |
| `Leave.java` | `plotsplugin/cmd_claiming.go` (`/plot leave`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport` |
| `Like.java` | `plotsplugin/cmd_social.go` (`/plot like`) | `TestSocialRatingsCommentsDoneAndClusters` |
| `ListCmd.java` | `plotsplugin/cmd_info_flags.go` (`/plot list`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry` |
| `Load.java` | `plotsplugin/cmd_schematics_backups.go` (`/plot load`) | `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Merge.java` | `plotsplugin/cmd_members_tp.go` (`/plot merge`) | `TestConfirmMergeUnlinkClearDeleteGrantAndCopySwap` |
| `Middle.java` | `plotsplugin/cmd_members_tp.go` (`/plot middle`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport` |
| `Move.java` | `plotsplugin/cmd_claiming.go` (`/plot move`) | `TestCommandsBuySetOwnerMoveSwapAutoMegaAndKick` |
| `Music.java` | `plotsplugin/cmd_info_flags.go` (`/plot music` sets flag; audio playback tracked) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`; audio playback in golem-mc/plots#14 |
| `Near.java` | `plotsplugin/cmd_info_flags.go` (`/plot near`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport` |
| `Owner.java` | `plotsplugin/cmd_claiming.go` (`/plot setowner`) | `TestCommandsBuySetOwnerMoveSwapAutoMegaAndKick` |
| `PluginCmd.java` | `plotsplugin/cmd_admin.go` (`/plot plugin`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry` |
| `Purge.java` | `plotsplugin/cmd_admin.go` (`/plot purge`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry` |
| `Rate.java` | `plotsplugin/cmd_social.go` (`/plot rate`) | `TestSocialRatingsCommentsDoneAndClusters`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `RegenAllRoads.java` | `plotsplugin/cmd_schematics_backups.go` (`/plot regenallroads`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry` |
| `Reload.java` | `plotsplugin/cmd_admin.go` (`/plot reload`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry` |
| `Remove.java` | `plotsplugin/cmd_members_tp.go` (`/plot remove`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport` |
| `SchematicCmd.java` | `plotsplugin/cmd_schematics_backups.go` (`/plot schematic`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Set.java` | `plotsplugin/cmd_info_flags.go` (`/plot set`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `SetHome.java` | `plotsplugin/cmd_members_tp.go` (`/plot sethome`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestCommandsBuySetOwnerMoveSwapAutoMegaAndKick` |
| `Setup.java` | `plotsplugin/cmd_admin.go` (`/plot setup`) | `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Swap.java` | `plotsplugin/cmd_claiming.go` (`/plot swap`) | `TestCommandsBuySetOwnerMoveSwapAutoMegaAndKick` |
| `Target.java` | `plotsplugin/cmd_members_tp.go` (`/plot target`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport` |
| `Template.java` | `plotsplugin/cmd_admin.go` (`/plot template`) | `TestSchematicsBackupsTemplatesPurgeCondenseAndExpiry`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Toggle.java` | `plotsplugin/cmd_info_flags.go` (`/plot toggle`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin`, `TestProtectionEnvironmentFlagsAndPlotChat` |
| `Trim.java` | `plotsplugin/cmd_admin.go` (`/plot trim`) | `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin` |
| `Trust.java` | `plotsplugin/cmd_members_tp.go` (`/plot trust`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport`, `TestProtectionMatrixRolesAndLocations` |
| `Unlink.java` | `plotsplugin/cmd_members_tp.go` (`/plot unlink`) | `TestConfirmMergeUnlinkClearDeleteGrantAndCopySwap` |
| `Visit.java` | `plotsplugin/cmd_members_tp.go` (`/plot visit`) | `TestCorePlotCommandsClaimTrustFlagsAndTeleport` |

### Upstream Listeners (`PlotSquared/Bukkit/.../bukkit/listener/*.java` & `Core/.../core/listener/*.java`)

| Upstream Listener Class | Implementation | Covered By Test / Issue |
|---|---|---|
| `BlockEventListener.java` | `plotsplugin/protection_blocks.go` (`BlockPlaceEvent`, `BlockBreakEvent`, `BlockDamageEvent`, `SignChangeEvent`, `BlockBurnEvent`, `BlockIgniteEvent`, `BlockSpreadEvent`, `BlockFormEvent`, `BlockFadeEvent`, `MoistureChangeEvent`, `BlockGrowEvent`, `LeavesDecayEvent`, `StructureGrowEvent`, `BlockFromToEvent`, `BlockPistonEvent`, `BlockDispenseEvent`, `BlockExplodeEvent`, `SpongeAbsorbEvent`, `TNTPrimeEvent`, `PortalCreateEvent`, `CauldronLevelChangeEvent`) | `TestBlockAndInteractProtection`, `TestProtectionMatrixRolesAndLocations`, `TestProtectionEnvironmentFlagsAndPlotChat` |
| `BlockEventListener117.java` | `plotsplugin/protection_blocks.go` (`BlockFertilizeEvent`, `BlockFormEvent`) & `plotsplugin/protection_players.go` (sculk sensor physical trigger) | `TestProtectionEnvironmentFlagsAndPlotChat` |
| `HighFreqBlockEventListener.java` | `plotsplugin/protection_blocks.go` (`BlockPhysicsEvent`, `BlockRedstoneEvent`) | `TestProtectionEnvironmentFlagsAndPlotChat` |
| `PlayerEventListener.java` | `plotsplugin/protection_players.go` (`PlayerInteractEvent`, `PlayerBucketEvent`, `PlayerInteractEntityEvent`, `PlayerArmorStandManipulateEvent`, `PlayerTakeLecternBookEvent`, `PlayerAttackEvent`, `PlayerDropItemEvent`, `EntityPickupItemEvent`, `PlayerFishEvent`, `PlayerLeashEntityEvent`, `PlayerChatEvent`, `PlayerCommandEvent`) & `plotsplugin/movement.go` | `TestBlockAndInteractProtection`, `TestMovementFlagsPvPChatAndBlockedCmds`, `TestProtectionMatrixRolesAndLocations`, `TestProtectionEnvironmentFlagsAndPlotChat` |
| `PlayerEventListener1201.java` | Covered via `PlayerInteractEvent` (right-click) and `SignChangeEvent` in `plotsplugin/protection_players.go` & `protection_blocks.go` | `TestProtectionMatrixRolesAndLocations` |
| `EntityEventListener.java` | `plotsplugin/protection_entities.go` (`EntityDamageEvent`, `EntityCombustEvent`, `EntityKnockbackEvent`, `EntityExplodeEvent`, `EntityChangeBlockEvent`, `HangingPlaceEvent`, `HangingBreakEvent`, `VehicleEvent`, `VehicleMoveEvent`) | `TestBlockAndInteractProtection`, `TestMovementFlagsPvPChatAndBlockedCmds`, `TestProtectionMatrixRolesAndLocations` |
| `EntitySpawnListener.java` | `plotsplugin/protection_entities.go` (`EntitySpawnEvent` with mob/animal/misc caps & area spawn rules) | `TestProtectionEnvironmentFlagsAndPlotChat` |
| `ProjectileEventListener.java` | `plotsplugin/protection_entities.go` (`ProjectileLaunchEvent`, `ProjectileHitEvent`, `PotionSplashEvent`, `AreaEffectCloudApplyEvent`) | `TestProtectionEnvironmentFlagsAndPlotChat` |
| `ProjectileEventListener1211.java` | Handled by `ProjectileHitEvent` in `plotsplugin/protection_entities.go` | `TestProtectionEnvironmentFlagsAndPlotChat` |
| `SpigotListener.java` | `plotsplugin/protection_entities.go` (`EntityPotionEffectEvent`) | `TestProtectionEnvironmentFlagsAndPlotChat` |
| `ForceFieldListener.java` | `plotsplugin/movement.go` (`forcefield` flag velocity knockback on `PlayerMoveEvent`) | `TestProtectionEnvironmentFlagsAndPlotChat` |
| `PlotListener.java` (Core) | `plotsplugin/movement.go` (plot entry/exit flags: `greeting`, `farewell`, `titles`, `fly`, `gamemode`, `time`, `weather`, `deny-exit`, `entry`, `PlayerEnterPlotEvent`, `PlayerLeavePlotEvent`) | `TestMovementFlagsPvPChatAndBlockedCmds`, `TestProtectionMatrixRolesAndLocations`; music audio playback in golem-mc/plots#14 |
| `WorldEvents.java` | `plotsplugin/plugin.go` (`WorldLoadEvent`, `WorldUnloadEvent`) | `TestGeneratorLifecycleAndPlotOps` |
| `ServerListener.java` | `plotsplugin/plugin.go` (`Load` / `Enable` lifecycle) | `TestGeneratorLifecycleAndPlotOps` |
| `ChunkListener.java` | `plotsplugin/plugin.go` (`ChunkLoadEvent` for augmented/partial chunk generation), `protection_blocks.go` (`BlockPhysicsEvent`), `protection_entities.go` (`EntitySpawnEvent`); `ChunkUnloadEvent` cleanup tracked | `TestGeneratorLifecycleAndPlotOps`; unload cleanup in golem-mc/plots#13 |
| `PaperListener.java` | `plotsplugin/protection_entities.go` (`EntityMoveEvent`, `EntitySpawnEvent`, `ProjectileLaunchEvent`), `protection_blocks.go` (`BlockPlaceEvent`), `commands.go` (`TabComplete`); Paper-only pre-pathfind events not applicable on golem | `TestCommandsSetComponentsToggleDislikeInboxClusterAndAdmin`, `TestProtectionMatrixRolesAndLocations` |
| `SingleWorldListener.java` | Not ported yet (`SinglePlotArea` mode) | Tracked in golem-mc/plots#12 |
| `WESubscriber.java`, `WEExtent.java`, `ProcessedWEExtent.java`, `ExtentWrapper.java` (Core) | `plotsplugin/worldedit.go` (`worldedit.RegisterRegionManager` + `/plot toggle worldedit` bypass) | `TestGeneratorLifecycleAndPlotOps` |
| `PlayerBlockEventType.java` (Core) | Inlined in `plotsplugin/protection_blocks.go` & `protection_players.go` | `TestProtectionMatrixRolesAndLocations` |
