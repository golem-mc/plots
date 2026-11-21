// Package plotsplugin is the Plots plugin (ported from PlotSquared) for golem,
// registered as "plots". It provides the plots.API service, implements
// plugin.GeneratorProvider for plot worlds, manages plots and plot areas,
// enforces protection rules, and registers /plot commands.
//
// Other plugins that want only the API import github.com/golem-mc/plots.
package plotsplugin

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/event"
	"github.com/golem-mc/plots/internal/backup"
	"github.com/golem-mc/plots/internal/config"
	"github.com/golem-mc/plots/internal/expiry"
	"github.com/golem-mc/plots/internal/generator"
	"github.com/golem-mc/plots/internal/lang"
	"github.com/golem-mc/plots/internal/storage"
	"github.com/google/uuid"

	// Plots depends on WorldEdit for block parsing, schematics, and region hooks.
	_ "github.com/golem-mc/worldedit"
)

func init() {
	plugin.Register("plots", &Plugin{},
		plugin.Depends("worldedit"),
		plugin.SoftDepends("vault", "luckperms"),
		plugin.LoadBefore("multiverse-core"),
	)
}

// Plugin is the live golem plugin instance.
type Plugin struct {
	Server   plugin.Server
	Log      *slog.Logger
	Settings *config.Settings
	Storage  *config.StorageConfig
	Worlds   *config.WorldsFile
	Lang     *lang.Catalog
	DB       *storage.DB
	Backups  *backup.Manager
	Expiry   *expiry.Manager
	clockNow func() time.Time

	mu              sync.RWMutex
	areas           map[string][]*plots.PlotArea // keyed by lowercase worldName
	gens            map[string]*generator.HybridGenerator
	plotsTmp        map[string]map[plots.PlotId]*plots.Plot
	clustTmp        map[string][]*plots.PlotCluster
	statesMu        sync.Mutex
	states          map[uuid.UUID]*playerState
	cmds            map[string]*subCommand
	subCmds         []*subCommand
	confirmMu       sync.Mutex
	pendingConfirms map[uuid.UUID]*pendingConfirm
	levelOnce       sync.Once
	levelName       string
	legacyOnce      sync.Once

	dbgMu         sync.Mutex
	unsafeAllowed map[uuid.UUID]struct{}
	expiryTask    plugin.Task
	rgarTask      plugin.Task
}

var current struct {
	sync.Mutex
	ps *Plugin
}

func (ps *Plugin) now() time.Time {
	if ps.clockNow != nil {
		return ps.clockNow()
	}
	return time.Now()
}

// Get returns the currently enabled Plots plugin instance, for tests.
func Get() (*Plugin, bool) {
	current.Lock()
	defer current.Unlock()
	return current.ps, current.ps != nil
}

// CheckLegacyDataDir logs a one-line migration hint when plugins/PlotSquared
// exists on disk and plugins/plots does not.
func CheckLegacyDataDir(pluginsDir string, log *slog.Logger) bool {
	if _, err := os.Stat(filepath.Join(pluginsDir, "plots")); err == nil {
		return false
	}
	info, err := os.Stat(filepath.Join(pluginsDir, "PlotSquared"))
	if err != nil || !info.IsDir() {
		return false
	}
	if log != nil {
		log.Info("found plugins/PlotSquared without plugins/plots; copy plugins/PlotSquared to plugins/plots to migrate your existing PlotSquared data")
	}
	return true
}

func (ps *Plugin) checkLegacyDataDir() {
	ps.legacyOnce.Do(func() {
		CheckLegacyDataDir("plugins", ps.Log)
	})
}

// Load runs before worlds load (plugin.Loader), loading configs early so Generator()
// can resolve world settings from worlds.yml immediately.
func (ps *Plugin) Load(s plugin.Server) error {
	ps.Server = s
	ps.Log = s.Logger()
	ps.checkLegacyDataDir()
	ps.areas = make(map[string][]*plots.PlotArea)
	ps.gens = make(map[string]*generator.HybridGenerator)
	return ps.loadConfigs()
}

// Enable initializes Plots: config, lang, SQLite DB, plot areas, worlds,
// permissions, WorldEdit restrictions, and API service.
func (ps *Plugin) Enable(s plugin.Server) error {
	ps.Server = s
	ps.Log = s.Logger()
	ps.checkLegacyDataDir()
	if ps.areas == nil {
		ps.areas = make(map[string][]*plots.PlotArea)
	}
	if ps.gens == nil {
		ps.gens = make(map[string]*generator.HybridGenerator)
	}

	if ps.Settings == nil {
		if err := ps.loadConfigs(); err != nil {
			return err
		}
	}

	// Copy bundled templates
	if err := config.CopyTemplates(s.DataDir(), ps.Settings.Paths.Templates); err != nil {
		ps.Log.Warn("failed to copy bundled templates", "err", err)
	}

	// Load localization
	ps.Lang = lang.NewCatalog()
	if err := ps.Lang.LoadFromDir(s.DataDir(), ps.Settings.EnabledComponents.DefaultLocale); err != nil {
		ps.Log.Warn("failed to load lang file, using embedded messages_en.json", "err", err)
	}

	// Define PlotSquared plugin events on golem's event bus
	event.DefineAll(s)

	// Register permissions
	ps.registerPermissions()

	// Open SQLite database
	if ps.Settings.EnabledComponents.Database {
		if ps.Storage.MySQL.Use {
			ps.Log.Warn("MySQL storage is not supported on golem; using SQLite instead")
		}
		dbName := ps.Storage.SQLite.DB
		if dbName == "" {
			dbName = "storage"
		}
		if !strings.HasSuffix(strings.ToLower(dbName), ".db") {
			dbName += ".db"
		}
		dbPath := filepath.Join(s.DataDir(), dbName)
		db, err := storage.Open(dbPath, ps.Storage.Prefix)
		if err != nil {
			return fmt.Errorf("open sqlite database: %w", err)
		}
		ps.DB = db

		plotsTmp, err := db.LoadPlots()
		if err != nil {
			ps.Log.Error("failed loading plots from sqlite", "err", err)
		} else {
			ps.plotsTmp = plotsTmp
		}
		clustTmp, err := db.LoadClusters()
		if err != nil {
			ps.Log.Error("failed loading clusters from sqlite", "err", err)
		} else {
			ps.clustTmp = clustTmp
		}
	}

	// Load configured plot areas and ensure custom plot worlds are loaded
	ps.initConfiguredWorlds()

	ps.registerSingleOwned()
	ps.registerSingleListeners()

	// Listen for world load/unload and augmented/partial chunk generation
	plugin.Listen(s, func(e *plugin.WorldLoadEvent) {
		ps.onWorldLoad(e.World)
	})
	plugin.Listen(s, func(e *plugin.WorldUnloadEvent) {
		ps.onWorldUnload(e.World)
	})
	plugin.Listen(s, func(e *plugin.ChunkLoadEvent) {
		if e.New {
			ps.onNewChunk(e.Chunk)
		}
	})

	plugin.Listen(s, func(e *plugin.ChunkUnloadEvent) {
		ps.onChunkUnload(e.Chunk)
	})

	// Initialize backup and expiry managers
	ps.Backups = backup.NewManager(s.DataDir(), &ps.Settings.Backup)
	ps.Expiry = expiry.NewManager()
	ps.startExpiryTask()

	// Register protection & movement listeners
	ps.registerProtection()

	// Register /plot commands and tab completion
	ps.registerCommands()

	// Register WorldEdit region restriction manager
	ps.registerWorldEditHook()

	// Expose public plots.API service
	plugin.Provide[plots.API](s, &apiService{ps: ps}, 0)

	current.Lock()
	current.ps = ps
	current.Unlock()

	ps.Log.Info("Plots enabled", "areas", len(ps.PlotAreas()))
	return nil
}

// Disable cleanly closes the SQLite database and clears runtime state.
func (ps *Plugin) Disable() {
	if ps.DB != nil {
		if err := ps.DB.Close(); err != nil {
			ps.Log.Error("error closing sqlite db", "err", err)
		}
		ps.DB = nil
	}
	current.Lock()
	if current.ps == ps {
		current.ps = nil
	}
	current.Unlock()
}

// Generator implements plugin.GeneratorProvider, resolving "plots[:spec]"
// for world managers (e.g. Multiverse) or Server.Generator.
func (ps *Plugin) Generator(world plugin.Key, spec string) (plugin.Generator, error) {
	worldName := ps.WorldNameForKey(world)
	area := ps.ensureWorldArea(worldName, spec)
	if spec != "" {
		if err := generator.ApplyGeneratorSpec(area, spec); err != nil {
			return nil, err
		}
		if ps.Worlds != nil {
			ps.Worlds.Worlds[worldName] = config.WorldConfigFromArea(area)
			_ = config.SaveWorlds(ps.Server.DataDir(), ps.Worlds)
		}
	}
	gen := ps.generatorForArea(area)
	gen.Reload(ps.Server.DataDir(), ps.Settings)
	return gen, nil
}

func (ps *Plugin) loadConfigs() error {
	dataDir := ps.Server.DataDir()
	settings, err := config.LoadSettings(dataDir)
	if err != nil {
		return fmt.Errorf("load settings.yml: %w", err)
	}
	ps.Settings = settings

	stor, err := config.LoadStorageConfig(dataDir)
	if err != nil {
		return fmt.Errorf("load storage.yml: %w", err)
	}
	ps.Storage = stor

	worlds, err := config.LoadWorlds(dataDir)
	if err != nil {
		return fmt.Errorf("load worlds.yml: %w", err)
	}
	ps.Worlds = worlds
	return nil
}

func (ps *Plugin) initConfiguredWorlds() {
	if ps.Worlds == nil {
		return
	}
	for worldName, wc := range ps.Worlds.Worlds {
		if wc == nil {
			continue
		}
		ps.registerAreasFromWorldConfig(worldName, wc)

		// If this is a custom plot world (not yet loaded), load it with our HybridGenerator.
		if _, loaded := ps.WrapWorld(worldName); !loaded {
			areas := ps.PlotAreasInWorld(worldName)
			if len(areas) > 0 && areas[0].Type == plots.AreaNormal {
				gen := ps.generatorForArea(areas[0])
				key := ps.WorldKeyForName(worldName)
				if _, err := ps.Server.CreateWorld(plugin.WorldOptions{
					Key:       key,
					Dimension: plugin.MinecraftKey("overworld"),
					Generator: gen,
				}); err != nil {
					ps.Log.Warn("could not load configured plot world", "world", worldName, "err", err)
				}
			}
		}
	}
}

func (ps *Plugin) registerAreasFromWorldConfig(worldName string, wc *config.WorldConfig) {
	low := strings.ToLower(worldName)
	var areas []*plots.PlotArea
	if len(wc.Areas) > 0 {
		for areaKey, subWC := range wc.Areas {
			id, minID, maxID, ok := config.ParseAreaKey(areaKey)
			if !ok {
				ps.Log.Warn("invalid partial area key in worlds.yml", "world", worldName, "key", areaKey)
				continue
			}
			m1, m2 := minID, maxID
			a := subWC.ToPlotArea(worldName, id, &m1, &m2)
			a.Type = plots.AreaPartial
			areas = append(areas, a)
		}
	} else {
		areas = append(areas, wc.ToPlotArea(worldName, "", nil, nil))
	}

	ps.mu.Lock()
	ps.areas[low] = areas
	ps.mu.Unlock()

	for _, a := range areas {
		ps.attachLoadedData(a)
		ps.generatorForArea(a)
	}
}

func (ps *Plugin) attachLoadedData(a *plots.PlotArea) {
	fullID := a.FullID()
	if a.Type == plots.AreaSingle {
		// Only the instance of one plot holds that plot's data; the "*" template none.
		if a.SingleID != nil {
			if p, ok := ps.plotsTmp[fullID][*a.SingleID]; ok {
				a.SetPlot(p)
			}
		}
		return
	}
	if ps.plotsTmp != nil {
		if m, ok := ps.plotsTmp[fullID]; ok {
			for _, p := range m {
				a.SetPlot(p)
			}
		}
	}
	if ps.clustTmp != nil {
		if cs, ok := ps.clustTmp[fullID]; ok {
			for _, c := range cs {
				a.AddCluster(c)
			}
		}
	}
}

func (ps *Plugin) ensureWorldArea(worldName, _ string) *plots.PlotArea {
	low := strings.ToLower(worldName)
	ps.mu.RLock()
	existing := ps.areas[low]
	ps.mu.RUnlock()
	if len(existing) > 0 {
		return existing[0]
	}

	var area *plots.PlotArea
	if ps.Worlds != nil {
		if wc, ok := ps.Worlds.Worlds[worldName]; ok && wc != nil {
			area = wc.ToPlotArea(worldName, "", nil, nil)
		}
	}
	if area == nil {
		area = plots.DefaultPlotArea(worldName, "")
	}
	ps.mu.Lock()
	ps.areas[low] = []*plots.PlotArea{area}
	ps.mu.Unlock()
	ps.attachLoadedData(area)
	return area
}

func (ps *Plugin) generatorForArea(area *plots.PlotArea) *generator.HybridGenerator {
	key := strings.ToLower(area.FullID())
	if area.Type == plots.AreaSingle && area.SingleID != nil {
		key = strings.ToLower(area.WorldName)
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if g, ok := ps.gens[key]; ok {
		return g
	}
	var dataDir string
	if ps.Server != nil {
		dataDir = ps.Server.DataDir()
	}
	g := generator.NewHybridGenerator(area, dataDir, ps.Settings)
	ps.gens[key] = g
	return g
}

func (ps *Plugin) onWorldLoad(w plugin.World) {
	name := ps.WorldName(w)
	if ps.Worlds == nil {
		return
	}
	if wc, ok := ps.Worlds.Worlds[name]; ok && wc != nil {
		if len(ps.PlotAreasInWorld(name)) == 0 {
			ps.registerAreasFromWorldConfig(name, wc)
		}
	}
}

func (ps *Plugin) onWorldUnload(_ plugin.World) {
	// Keep area definitions loaded in memory so offline queries and reloads stay consistent.
}

func (ps *Plugin) onNewChunk(chunk plugin.Chunk) {
	w := chunk.World()
	if w == nil {
		return
	}
	areas := ps.PlotAreasInWorld(ps.WorldName(w))
	for _, a := range areas {
		if a.Type == plots.AreaAugmented || a.Type == plots.AreaPartial {
			gen := ps.generatorForArea(a)
			gen.AugmentChunk(w, chunk)
		}
	}
}

// onChunkUnload mirrors PlotSquared's ChunkListener.onChunkUnload: in plot
// worlds it removes the road entities and, when the chunk held more than
// chunk_processor.max-entities, the entities over the cap. Players are kept.
func (ps *Plugin) onChunkUnload(chunk plugin.Chunk) {
	w := chunk.World()
	if w == nil || len(ps.PlotAreasInWorld(ps.WorldName(w))) == 0 {
		return
	}
	entities := chunk.Entities()
	max := ps.Settings.ChunkProcessor.MaxEntities
	kept := 0
	for _, ent := range entities {
		if ent.Type() == registry.EntityTypePlayer {
			continue
		}
		if area := ps.PlotAreaAt(ent.Location()); area != nil && area.PlotAt(ent.Location().Pos.Block().X, ent.Location().Pos.Block().Z) == nil {
			ent.Remove()
			continue
		}
		kept++
		if kept > max {
			ent.Remove()
		}
	}
}

// LevelName returns the server's overworld level name (e.g. "world").
func (ps *Plugin) LevelName() string {
	ps.levelOnce.Do(func() {
		if ps.Server != nil {
			ps.levelName = filepath.Base(ps.Server.WorldFolder(plugin.MinecraftKey("overworld")))
		}
		if ps.levelName == "" || ps.levelName == "." || ps.levelName == string(filepath.Separator) {
			ps.levelName = "world"
		}
	})
	return ps.levelName
}

// WorldNameForKey maps a golem dimension key to Bukkit's world name convention.
func (ps *Plugin) WorldNameForKey(key plugin.Key) string {
	if id, ok := singleIDFromKey(key); ok {
		return plots.SingleWorldName(id)
	}
	if key.Namespace != "minecraft" && key.Namespace != "" {
		return key.Namespace + "_" + key.Path
	}
	switch key.Path {
	case "overworld":
		return ps.LevelName()
	case "the_nether":
		return ps.LevelName() + "_nether"
	case "the_end":
		return ps.LevelName() + "_the_end"
	default:
		return key.Path
	}
}

// WorldKeyForName maps a Bukkit-style world name to a golem dimension key.
func (ps *Plugin) WorldKeyForName(name string) plugin.Key {
	if id, ok := plots.ParseSingleWorldName(name); ok {
		return singleKey(id)
	}
	lvl := ps.LevelName()
	switch {
	case strings.EqualFold(name, lvl):
		return plugin.MinecraftKey("overworld")
	case strings.EqualFold(name, lvl+"_nether"):
		return plugin.MinecraftKey("the_nether")
	case strings.EqualFold(name, lvl+"_the_end"):
		return plugin.MinecraftKey("the_end")
	}
	if ns, path, ok := strings.Cut(name, ":"); ok {
		return plugin.Key{Namespace: strings.ToLower(ns), Path: strings.ToLower(path)}
	}
	return plugin.MinecraftKey(strings.ToLower(name))
}

// WorldName implements plots.API.
func (ps *Plugin) WorldName(w plugin.World) string {
	if w == nil {
		return ""
	}
	return ps.WorldNameForKey(w.Key())
}

// WrapWorld implements plots.API.
func (ps *Plugin) WrapWorld(worldName string) (plugin.World, bool) {
	if ps.Server == nil {
		return nil, false
	}
	for _, w := range ps.Server.Worlds() {
		if strings.EqualFold(ps.WorldName(w), worldName) {
			return w, true
		}
	}
	return nil, false
}

// PlotAreas implements plots.API.
func (ps *Plugin) PlotAreas() []*plots.PlotArea {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	var out []*plots.PlotArea
	for _, list := range ps.areas {
		out = append(out, list...)
	}
	return out
}

// PlotAreasInWorld implements plots.API.
func (ps *Plugin) PlotAreasInWorld(worldName string) []*plots.PlotArea {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	list := ps.areas[strings.ToLower(worldName)]
	out := make([]*plots.PlotArea, len(list))
	copy(out, list)
	return out
}

// PlotAreaAt implements plots.API.
func (ps *Plugin) PlotAreaAt(loc plugin.Location) *plots.PlotArea {
	if loc.World == nil {
		return nil
	}
	bp := loc.Pos.Block()
	for _, a := range ps.PlotAreasInWorld(ps.WorldName(loc.World)) {
		if a.ContainsXZ(bp.X, bp.Z) {
			return a
		}
	}
	return nil
}

// PlotAreaAtBlock returns the PlotArea covering (w, pos).
func (ps *Plugin) PlotAreaAtBlock(w plugin.World, pos plugin.BlockPos) *plots.PlotArea {
	if w == nil {
		return nil
	}
	for _, a := range ps.PlotAreasInWorld(ps.WorldName(w)) {
		if a.ContainsXZ(pos.X, pos.Z) {
			return a
		}
	}
	return nil
}

// PlotAt implements plots.API.
func (ps *Plugin) PlotAt(loc plugin.Location) *plots.Plot {
	if loc.World == nil {
		return nil
	}
	return ps.PlotAtBlock(loc.World, loc.Pos.Block())
}

// PlotAtBlock returns the Plot at (w, pos) (including merged roads), or nil.
func (ps *Plugin) PlotAtBlock(w plugin.World, pos plugin.BlockPos) *plots.Plot {
	area := ps.PlotAreaAtBlock(w, pos)
	if area == nil {
		return nil
	}
	return area.PlotAt(pos.X, pos.Z)
}

// PlotsByOwner implements plots.API.
func (ps *Plugin) PlotsByOwner(owner uuid.UUID) []*plots.Plot {
	var out []*plots.Plot
	for _, a := range ps.PlotAreas() {
		out = append(out, a.PlotsByOwner(owner)...)
	}
	return out
}

// PlotAreaByName finds a PlotArea by world name or "world;id" or partial area id.
func (ps *Plugin) PlotAreaByName(name string) *plots.PlotArea {
	if name == "" {
		return nil
	}
	for _, a := range ps.PlotAreas() {
		if strings.EqualFold(a.FullID(), name) || strings.EqualFold(a.WorldName, name) || (a.ID != "" && strings.EqualFold(a.ID, name)) {
			return a
		}
	}
	return nil
}

type apiService struct{ ps *Plugin }

func (a *apiService) PlotAreas() []*plots.PlotArea { return a.ps.PlotAreas() }
func (a *apiService) PlotAreasInWorld(worldName string) []*plots.PlotArea {
	return a.ps.PlotAreasInWorld(worldName)
}
func (a *apiService) PlotAreaAt(loc plugin.Location) *plots.PlotArea {
	return a.ps.PlotAreaAt(loc)
}
func (a *apiService) PlotAt(loc plugin.Location) *plots.Plot { return a.ps.PlotAt(loc) }
func (a *apiService) PlotsByOwner(owner uuid.UUID) []*plots.Plot {
	return a.ps.PlotsByOwner(owner)
}
func (a *apiService) WorldName(w plugin.World) string { return a.ps.WorldName(w) }
func (a *apiService) WrapWorld(worldName string) (plugin.World, bool) {
	return a.ps.WrapWorld(worldName)
}
