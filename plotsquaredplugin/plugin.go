// Package plotsquaredplugin is the PlotSquared plugin for golem, registered as
// "plotsquared". It provides the plotsquared.API service, implements
// plugin.GeneratorProvider for plot worlds, manages plots and plot areas,
// enforces protection rules, and registers /plot commands.
//
// Other plugins that want only the API import github.com/golem-mc/plotsquared.
package plotsquaredplugin

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plotsquared"
	"github.com/golem-mc/plotsquared/event"
	"github.com/golem-mc/plotsquared/internal/config"
	"github.com/golem-mc/plotsquared/internal/generator"
	"github.com/golem-mc/plotsquared/internal/lang"
	"github.com/golem-mc/plotsquared/internal/storage"
	"github.com/google/uuid"

	// PlotSquared depends on WorldEdit for block parsing, schematics, and region hooks.
	_ "github.com/golem-mc/worldedit"
)

func init() {
	plugin.Register("plotsquared", &PlotSquared{},
		plugin.Depends("worldedit"),
		plugin.SoftDepends("vault", "luckperms"),
		plugin.LoadBefore("multiverse-core"),
	)
}

// PlotSquared is the live golem plugin instance.
type PlotSquared struct {
	Server   plugin.Server
	Log      *slog.Logger
	Settings *config.Settings
	Storage  *config.StorageConfig
	Worlds   *config.WorldsFile
	Lang     *lang.Catalog
	DB       *storage.DB

	mu        sync.RWMutex
	areas     map[string][]*plotsquared.PlotArea // keyed by lowercase worldName
	gens      map[string]*generator.HybridGenerator
	plotsTmp  map[string]map[plotsquared.PlotId]*plotsquared.Plot
	clustTmp  map[string][]*plotsquared.PlotCluster
	levelOnce sync.Once
	levelName string
}

var current struct {
	sync.Mutex
	ps *PlotSquared
}

// Get returns the currently enabled PlotSquared plugin instance, for tests.
func Get() (*PlotSquared, bool) {
	current.Lock()
	defer current.Unlock()
	return current.ps, current.ps != nil
}

// Load runs before worlds load (plugin.Loader), loading configs early so Generator()
// can resolve world settings from worlds.yml immediately.
func (ps *PlotSquared) Load(s plugin.Server) error {
	ps.Server = s
	ps.Log = s.Logger()
	ps.areas = make(map[string][]*plotsquared.PlotArea)
	ps.gens = make(map[string]*generator.HybridGenerator)
	return ps.loadConfigs()
}

// Enable initializes PlotSquared: config, lang, SQLite DB, plot areas, worlds,
// permissions, WorldEdit restrictions, and API service.
func (ps *PlotSquared) Enable(s plugin.Server) error {
	ps.Server = s
	ps.Log = s.Logger()
	if ps.areas == nil {
		ps.areas = make(map[string][]*plotsquared.PlotArea)
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

	// Register WorldEdit region restriction manager
	ps.registerWorldEditHook()

	// Expose public plotsquared.API service
	plugin.Provide[plotsquared.API](s, &apiService{ps: ps}, 0)

	current.Lock()
	current.ps = ps
	current.Unlock()

	ps.Log.Info("PlotSquared enabled", "areas", len(ps.PlotAreas()))
	return nil
}

// Disable cleanly closes the SQLite database and clears runtime state.
func (ps *PlotSquared) Disable() {
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

// Generator implements plugin.GeneratorProvider, resolving "PlotSquared[:spec]"
// for world managers (e.g. Multiverse) or Server.Generator.
func (ps *PlotSquared) Generator(world plugin.Key, spec string) (plugin.Generator, error) {
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

func (ps *PlotSquared) loadConfigs() error {
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

func (ps *PlotSquared) initConfiguredWorlds() {
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
			if len(areas) > 0 && areas[0].Type == plotsquared.AreaNormal {
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

func (ps *PlotSquared) registerAreasFromWorldConfig(worldName string, wc *config.WorldConfig) {
	low := strings.ToLower(worldName)
	var areas []*plotsquared.PlotArea
	if len(wc.Areas) > 0 {
		for areaKey, subWC := range wc.Areas {
			id, minID, maxID, ok := config.ParseAreaKey(areaKey)
			if !ok {
				ps.Log.Warn("invalid partial area key in worlds.yml", "world", worldName, "key", areaKey)
				continue
			}
			m1, m2 := minID, maxID
			a := subWC.ToPlotArea(worldName, id, &m1, &m2)
			a.Type = plotsquared.AreaPartial
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

func (ps *PlotSquared) attachLoadedData(a *plotsquared.PlotArea) {
	fullID := a.FullID()
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

func (ps *PlotSquared) ensureWorldArea(worldName, _ string) *plotsquared.PlotArea {
	low := strings.ToLower(worldName)
	ps.mu.RLock()
	existing := ps.areas[low]
	ps.mu.RUnlock()
	if len(existing) > 0 {
		return existing[0]
	}

	var area *plotsquared.PlotArea
	if ps.Worlds != nil {
		if wc, ok := ps.Worlds.Worlds[worldName]; ok && wc != nil {
			area = wc.ToPlotArea(worldName, "", nil, nil)
		}
	}
	if area == nil {
		area = plotsquared.DefaultPlotArea(worldName, "")
	}
	ps.mu.Lock()
	ps.areas[low] = []*plotsquared.PlotArea{area}
	ps.mu.Unlock()
	ps.attachLoadedData(area)
	return area
}

func (ps *PlotSquared) generatorForArea(area *plotsquared.PlotArea) *generator.HybridGenerator {
	key := strings.ToLower(area.FullID())
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

func (ps *PlotSquared) onWorldLoad(w plugin.World) {
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

func (ps *PlotSquared) onWorldUnload(_ plugin.World) {
	// Keep area definitions loaded in memory so offline queries and reloads stay consistent.
}

func (ps *PlotSquared) onNewChunk(chunk plugin.Chunk) {
	w := chunk.World()
	if w == nil {
		return
	}
	areas := ps.PlotAreasInWorld(ps.WorldName(w))
	for _, a := range areas {
		if a.Type == plotsquared.AreaAugmented || a.Type == plotsquared.AreaPartial {
			gen := ps.generatorForArea(a)
			gen.AugmentChunk(w, chunk)
		}
	}
}

// LevelName returns the server's overworld level name (e.g. "world").
func (ps *PlotSquared) LevelName() string {
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
func (ps *PlotSquared) WorldNameForKey(key plugin.Key) string {
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
func (ps *PlotSquared) WorldKeyForName(name string) plugin.Key {
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

// WorldName implements plotsquared.API.
func (ps *PlotSquared) WorldName(w plugin.World) string {
	if w == nil {
		return ""
	}
	return ps.WorldNameForKey(w.Key())
}

// WrapWorld implements plotsquared.API.
func (ps *PlotSquared) WrapWorld(worldName string) (plugin.World, bool) {
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

// PlotAreas implements plotsquared.API.
func (ps *PlotSquared) PlotAreas() []*plotsquared.PlotArea {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	var out []*plotsquared.PlotArea
	for _, list := range ps.areas {
		out = append(out, list...)
	}
	return out
}

// PlotAreasInWorld implements plotsquared.API.
func (ps *PlotSquared) PlotAreasInWorld(worldName string) []*plotsquared.PlotArea {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	list := ps.areas[strings.ToLower(worldName)]
	out := make([]*plotsquared.PlotArea, len(list))
	copy(out, list)
	return out
}

// PlotAreaAt implements plotsquared.API.
func (ps *PlotSquared) PlotAreaAt(loc plugin.Location) *plotsquared.PlotArea {
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
func (ps *PlotSquared) PlotAreaAtBlock(w plugin.World, pos plugin.BlockPos) *plotsquared.PlotArea {
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

// PlotAt implements plotsquared.API.
func (ps *PlotSquared) PlotAt(loc plugin.Location) *plotsquared.Plot {
	if loc.World == nil {
		return nil
	}
	return ps.PlotAtBlock(loc.World, loc.Pos.Block())
}

// PlotAtBlock returns the Plot at (w, pos) (including merged roads), or nil.
func (ps *PlotSquared) PlotAtBlock(w plugin.World, pos plugin.BlockPos) *plotsquared.Plot {
	area := ps.PlotAreaAtBlock(w, pos)
	if area == nil {
		return nil
	}
	return area.PlotAt(pos.X, pos.Z)
}

// PlotsByOwner implements plotsquared.API.
func (ps *PlotSquared) PlotsByOwner(owner uuid.UUID) []*plotsquared.Plot {
	var out []*plotsquared.Plot
	for _, a := range ps.PlotAreas() {
		out = append(out, a.PlotsByOwner(owner)...)
	}
	return out
}

// PlotAreaByName finds a PlotArea by world name or "world;id" or partial area id.
func (ps *PlotSquared) PlotAreaByName(name string) *plotsquared.PlotArea {
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

type apiService struct{ ps *PlotSquared }

func (a *apiService) PlotAreas() []*plotsquared.PlotArea { return a.ps.PlotAreas() }
func (a *apiService) PlotAreasInWorld(worldName string) []*plotsquared.PlotArea {
	return a.ps.PlotAreasInWorld(worldName)
}
func (a *apiService) PlotAreaAt(loc plugin.Location) *plotsquared.PlotArea {
	return a.ps.PlotAreaAt(loc)
}
func (a *apiService) PlotAt(loc plugin.Location) *plotsquared.Plot { return a.ps.PlotAt(loc) }
func (a *apiService) PlotsByOwner(owner uuid.UUID) []*plotsquared.Plot {
	return a.ps.PlotsByOwner(owner)
}
func (a *apiService) WorldName(w plugin.World) string { return a.ps.WorldName(w) }
func (a *apiService) WrapWorld(worldName string) (plugin.World, bool) {
	return a.ps.WrapWorld(worldName)
}
