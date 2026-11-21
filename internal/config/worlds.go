package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plots"
	"go.yaml.in/yaml/v4"
)

// ConfigurationVersion matches upstream LegacyConverter.CONFIGURATION_VERSION.
const ConfigurationVersion = "post_flattening"

// WorldsFile represents upstream config/worlds.yml.
type WorldsFile struct {
	Version string                  `yaml:"configuration_version"`
	Worlds  map[string]*WorldConfig `yaml:"worlds"`
}

// WorldConfig mirrors one world entry under `worlds.<worldName>` in worlds.yml.
type WorldConfig struct {
	Plot               PlotSection             `yaml:"plot"`
	Wall               WallSection             `yaml:"wall"`
	Road               RoadSection             `yaml:"road"`
	Home               HomeSection             `yaml:"home"`
	Schematic          SchematicSection        `yaml:"schematic"`
	Economy            EconomySection          `yaml:"economy"`
	Chat               WorldChatSection        `yaml:"chat"`
	World              WorldBoundsSection      `yaml:"world"`
	Event              WorldEventSection       `yaml:"event"`
	Generator          *GeneratorSection       `yaml:"generator,omitempty"`
	NaturalMobSpawning bool                    `yaml:"natural_mob_spawning"`
	MiscSpawnUnowned   bool                    `yaml:"misc_spawn_unowned"`
	MobSpawnerSpawning bool                    `yaml:"mob_spawner_spawning"`
	Flags              map[string]any          `yaml:"flags,omitempty"`
	Areas              map[string]*WorldConfig `yaml:"areas,omitempty"`
}

type PlotSection struct {
	Height       int    `yaml:"height"`
	Size         int    `yaml:"size"`
	Filling      string `yaml:"filling"`
	Floor        string `yaml:"floor"`
	Bedrock      bool   `yaml:"bedrock"`
	AutoMerge    bool   `yaml:"auto_merge"`
	CreateSigns  bool   `yaml:"create_signs"`
	SignMaterial string `yaml:"sign_material"`
	Biome        string `yaml:"biome"`
}

type WallSection struct {
	PlaceTopBlock bool   `yaml:"place_top_block"`
	Block         string `yaml:"block"`
	BlockClaimed  string `yaml:"block_claimed"`
	Filling       string `yaml:"filling"`
	Height        int    `yaml:"height"`
}

type RoadSection struct {
	Width   int            `yaml:"width"`
	Height  int            `yaml:"height"`
	Block   string         `yaml:"block"`
	OffsetX int            `yaml:"offset_x,omitempty"`
	OffsetZ int            `yaml:"offset_z,omitempty"`
	Flags   map[string]any `yaml:"flags,omitempty"`
}

type HomeSection struct {
	Default    string `yaml:"default"`
	Nonmembers string `yaml:"nonmembers"`
}

type SchematicSection struct {
	OnClaim        bool     `yaml:"on_claim"`
	File           string   `yaml:"file"`
	SpecifyOnClaim bool     `yaml:"specify_on_claim"`
	Schematics     []string `yaml:"schematics"`
}

type EconomySection struct {
	Use    bool              `yaml:"use"`
	Prices map[string]string `yaml:"prices"`
}

type WorldChatSection struct {
	Enabled bool `yaml:"enabled"`
	Forced  bool `yaml:"forced"`
}

type WorldBoundsSection struct {
	Border                bool   `yaml:"border"`
	BorderSize            int    `yaml:"border_size"`
	MaxHeight             int    `yaml:"max_height"`
	MinHeight             int    `yaml:"min_height"`
	MinGenHeight          int    `yaml:"min_gen_height"`
	MaxGenHeight          int    `yaml:"max_gen_height"`
	GameMode              string `yaml:"gamemode"`
	ComponentBelowBedrock bool   `yaml:"component_below_bedrock"`
}

type WorldEventSection struct {
	Spawn SpawnEventSection `yaml:"spawn"`
}

type SpawnEventSection struct {
	Egg      bool `yaml:"egg"`
	Custom   bool `yaml:"custom"`
	Breeding bool `yaml:"breeding"`
}

type GeneratorSection struct {
	Type    string `yaml:"type"`
	Terrain string `yaml:"terrain"`
	Plugin  string `yaml:"plugin,omitempty"`
}

// DefaultWorldConfig returns upstream default settings for a HybridPlotWorld.
func DefaultWorldConfig() *WorldConfig {
	return &WorldConfig{
		Plot: PlotSection{
			Height:       62,
			Size:         42,
			Filling:      "minecraft:stone",
			Floor:        "minecraft:grass_block",
			Bedrock:      true,
			AutoMerge:    false,
			CreateSigns:  true,
			SignMaterial: "oak_wall_sign",
			Biome:        "minecraft:forest",
		},
		Wall: WallSection{
			PlaceTopBlock: true,
			Block:         "minecraft:smooth_stone_slab",
			BlockClaimed:  "minecraft:sandstone_slab",
			Filling:       "minecraft:stone",
			Height:        62,
		},
		Road: RoadSection{
			Width:  7,
			Height: 62,
			Block:  "minecraft:quartz_block",
		},
		Home: HomeSection{
			Default:    "side",
			Nonmembers: "side",
		},
		Schematic: SchematicSection{
			File: "null",
		},
		Economy: EconomySection{
			Use: false,
			Prices: map[string]string{
				"claim": "100",
				"merge": "100",
				"sell":  "100",
			},
		},
		Chat: WorldChatSection{
			Enabled: true,
		},
		World: WorldBoundsSection{
			BorderSize:   1,
			MaxHeight:    256,
			MinHeight:    1,
			MinGenHeight: 0,
			MaxGenHeight: 256,
			GameMode:     "creative",
		},
		Event: WorldEventSection{
			Spawn: SpawnEventSection{
				Custom: true,
			},
		},
	}
}

// LoadWorlds loads config/worlds.yml, creating an empty one with configuration_version if missing.
func LoadWorlds(dataDir string) (*WorldsFile, error) {
	cfgDir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(cfgDir, "worlds.yml")
	wf := &WorldsFile{
		Version: ConfigurationVersion,
		Worlds:  make(map[string]*WorldConfig),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return wf, SaveWorlds(dataDir, wf)
		}
		return nil, err
	}
	if len(data) == 0 {
		return wf, nil
	}
	// Parse via raw map so partial WorldConfigs inherit defaults rather than zero values.
	var raw struct {
		Version string               `yaml:"configuration_version"`
		Worlds  map[string]yaml.Node `yaml:"worlds"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Version != "" {
		wf.Version = raw.Version
	}
	for name, node := range raw.Worlds {
		wc := DefaultWorldConfig()
		if err := node.Decode(wc); err != nil {
			return nil, fmt.Errorf("worlds.%s: %w", name, err)
		}
		wf.Worlds[name] = wc
	}
	return wf, nil
}

// SaveWorlds writes config/worlds.yml.
func SaveWorlds(dataDir string, wf *WorldsFile) error {
	cfgDir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return err
	}
	if wf.Version == "" {
		wf.Version = ConfigurationVersion
	}
	out, err := yaml.Marshal(wf)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cfgDir, "worlds.yml"), out, 0o644)
}

// ToPlotArea converts a WorldConfig into a live PlotArea.
func (wc *WorldConfig) ToPlotArea(worldName, id string, minID, maxID *plots.PlotId) *plots.PlotArea {
	a := plots.DefaultPlotArea(worldName, id)
	a.Min = minID
	a.Max = maxID
	if wc.Plot.Size > 0 {
		a.PlotWidth = wc.Plot.Size
	}
	if wc.Road.Width >= 0 {
		a.RoadWidth = wc.Road.Width
	}
	a.RoadOffsetX = wc.Road.OffsetX
	a.RoadOffsetZ = wc.Road.OffsetZ
	a.PlotHeight = wc.Plot.Height
	a.RoadHeight = wc.Road.Height
	a.WallHeight = wc.Wall.Height
	a.MinGenHeight = wc.World.MinGenHeight
	a.MaxGenHeight = wc.World.MaxGenHeight
	a.MinBuildHeight = wc.World.MinHeight
	a.MaxBuildHeight = wc.World.MaxHeight
	a.PlotBedrock = wc.Plot.Bedrock
	a.PlaceTopBlock = wc.Wall.PlaceTopBlock
	a.ComponentBelowBedrock = wc.World.ComponentBelowBedrock

	if b, err := plots.ParseBlockBucket(wc.Plot.Filling); err == nil {
		a.MainBlock = b
	}
	if b, err := plots.ParseBlockBucket(wc.Plot.Floor); err == nil {
		a.TopBlock = b
	}
	if b, err := plots.ParseBlockBucket(wc.Wall.Block); err == nil {
		a.WallBlock = b
	}
	if b, err := plots.ParseBlockBucket(wc.Wall.BlockClaimed); err == nil {
		a.ClaimedWallBlock = b
	}
	if b, err := plots.ParseBlockBucket(wc.Wall.Filling); err == nil {
		a.WallFilling = b
	}
	if b, err := plots.ParseBlockBucket(wc.Road.Block); err == nil {
		a.RoadBlock = b
	}

	biome := strings.ToLower(strings.TrimSpace(wc.Plot.Biome))
	if biome == "" {
		biome = "minecraft:forest"
	} else if !strings.Contains(biome, ":") {
		biome = "minecraft:" + biome
	}
	a.PlotBiome = biome

	switch strings.ToLower(wc.World.GameMode) {
	case "survival", "s", "0":
		a.GameMode = plugin.Survival
	case "adventure", "a", "2":
		a.GameMode = plugin.Adventure
	case "spectator", "3":
		a.GameMode = plugin.Spectator
	default:
		a.GameMode = plugin.Creative
	}

	a.AutoMerge = wc.Plot.AutoMerge
	a.AllowSigns = wc.Plot.CreateSigns
	if wc.Plot.SignMaterial != "" {
		a.SignMaterial = strings.ToLower(wc.Plot.SignMaterial)
	}
	a.MobSpawning = wc.NaturalMobSpawning
	a.MobSpawnerSpawning = wc.MobSpawnerSpawning
	a.MiscSpawnUnowned = wc.MiscSpawnUnowned
	a.SpawnEggs = wc.Event.Spawn.Egg
	a.SpawnCustom = wc.Event.Spawn.Custom
	a.SpawnBreeding = wc.Event.Spawn.Breeding
	a.PlotChat = wc.Chat.Enabled
	a.ForcingPlotChat = wc.Chat.Forced
	a.WorldBorder = wc.World.Border
	if wc.World.BorderSize > 0 {
		a.BorderSize = wc.World.BorderSize
	}

	a.DefaultHome, _ = plots.ParseBlockLoc(wc.Home.Default)
	a.NonmemberHome, _ = plots.ParseBlockLoc(wc.Home.Nonmembers)
	a.HomeAllowNonmember = strings.EqualFold(wc.Home.Default, wc.Home.Nonmembers)

	a.UseEconomy = wc.Economy.Use
	if wc.Economy.Prices != nil {
		if v := wc.Economy.Prices["claim"]; v != "" {
			a.PriceClaim = v
		}
		if v := wc.Economy.Prices["merge"]; v != "" {
			a.PriceMerge = v
		}
		if v := wc.Economy.Prices["sell"]; v != "" {
			a.PriceSell = v
		}
	}
	a.SchematicOnClaim = wc.Schematic.OnClaim
	a.SchematicFile = wc.Schematic.File
	a.SchematicClaimSpecify = wc.Schematic.SpecifyOnClaim
	for _, s := range wc.Schematic.Schematics {
		a.Schematics = append(a.Schematics, strings.ToLower(s))
	}

	if wc.Generator != nil {
		if t, ok := plots.ParsePlotAreaType(wc.Generator.Type); ok {
			a.Type = t
		}
		if tr, ok := plots.ParsePlotAreaTerrainType(wc.Generator.Terrain); ok {
			a.Terrain = tr
		}
	}

	applyYAMLFlags(a.Flags, wc.Flags)
	applyYAMLFlags(a.RoadFlags, wc.Road.Flags)
	a.HasRoadFlags = len(a.RoadFlags.LocalFlags()) > 0

	a.ComputeGeometry()
	return a
}

// WorldConfigFromArea creates a serializable WorldConfig from a PlotArea.
func WorldConfigFromArea(a *plots.PlotArea) *WorldConfig {
	wc := DefaultWorldConfig()
	wc.Plot.Size = a.PlotWidth
	wc.Plot.Height = a.PlotHeight
	wc.Plot.Filling = a.MainBlock.String()
	wc.Plot.Floor = a.TopBlock.String()
	wc.Plot.Bedrock = a.PlotBedrock
	wc.Plot.AutoMerge = a.AutoMerge
	wc.Plot.CreateSigns = a.AllowSigns
	wc.Plot.SignMaterial = a.SignMaterial
	wc.Plot.Biome = a.PlotBiome

	wc.Wall.PlaceTopBlock = a.PlaceTopBlock
	wc.Wall.Block = a.WallBlock.String()
	wc.Wall.BlockClaimed = a.ClaimedWallBlock.String()
	wc.Wall.Filling = a.WallFilling.String()
	wc.Wall.Height = a.WallHeight

	wc.Road.Width = a.RoadWidth
	wc.Road.Height = a.RoadHeight
	wc.Road.Block = a.RoadBlock.String()
	wc.Road.OffsetX = a.RoadOffsetX
	wc.Road.OffsetZ = a.RoadOffsetZ

	wc.World.Border = a.WorldBorder
	wc.World.BorderSize = a.BorderSize
	wc.World.MaxHeight = a.MaxBuildHeight
	wc.World.MinHeight = a.MinBuildHeight
	wc.World.MinGenHeight = a.MinGenHeight
	wc.World.MaxGenHeight = a.MaxGenHeight
	wc.World.ComponentBelowBedrock = a.ComponentBelowBedrock
	switch a.GameMode {
	case plugin.Survival:
		wc.World.GameMode = "survival"
	case plugin.Adventure:
		wc.World.GameMode = "adventure"
	case plugin.Spectator:
		wc.World.GameMode = "spectator"
	default:
		wc.World.GameMode = "creative"
	}

	wc.NaturalMobSpawning = a.MobSpawning
	wc.MobSpawnerSpawning = a.MobSpawnerSpawning
	wc.MiscSpawnUnowned = a.MiscSpawnUnowned
	wc.Event.Spawn.Egg = a.SpawnEggs
	wc.Event.Spawn.Custom = a.SpawnCustom
	wc.Event.Spawn.Breeding = a.SpawnBreeding
	wc.Chat.Enabled = a.PlotChat
	wc.Chat.Forced = a.ForcingPlotChat
	wc.Economy.Use = a.UseEconomy
	wc.Economy.Prices = map[string]string{
		"claim": a.PriceClaim,
		"merge": a.PriceMerge,
		"sell":  a.PriceSell,
	}
	if a.Type != plots.AreaNormal || a.Terrain != plots.TerrainNone {
		wc.Generator = &GeneratorSection{
			Type:    a.Type.String(),
			Terrain: a.Terrain.String(),
		}
	}
	return wc
}

// ParseAreaKey parses upstream's partial area map key "<id>-<minX>;<minY>-<maxX>;<maxY>".
func ParseAreaKey(key string) (id string, minID, maxID plots.PlotId, ok bool) {
	// Split on '-' not preceded by ';' (since plot IDs can be negative, e.g. area--1;-1-1;1).
	var parts []string
	start := 0
	for i := 0; i < len(key); i++ {
		if key[i] == '-' && i > 0 && key[i-1] != ';' && key[i-1] != '-' {
			parts = append(parts, key[start:i])
			start = i + 1
		}
	}
	parts = append(parts, key[start:])
	if len(parts) != 3 {
		return "", plots.PlotId{}, plots.PlotId{}, false
	}
	p1, ok1 := plots.ParsePlotId(parts[1])
	p2, ok2 := plots.ParsePlotId(parts[2])
	if !ok1 || !ok2 {
		return "", plots.PlotId{}, plots.PlotId{}, false
	}
	return parts[0], p1, p2, true
}

// FormatAreaKey formats a partial area map key for worlds.yml.
func FormatAreaKey(id string, minID, maxID plots.PlotId) string {
	return fmt.Sprintf("%s-%s-%s", id, minID.String(), maxID.String())
}

func applyYAMLFlags(fc *plots.FlagContainer, raw map[string]any) {
	if fc == nil || len(raw) == 0 {
		return
	}
	for k, v := range raw {
		if k == "default" || v == nil {
			continue
		}
		var str string
		switch tv := v.(type) {
		case string:
			str = tv
		case bool:
			str = strconv.FormatBool(tv)
		case int:
			str = strconv.Itoa(tv)
		case float64:
			str = strconv.FormatFloat(tv, 'f', -1, 64)
		case []any:
			parts := make([]string, 0, len(tv))
			for _, item := range tv {
				parts = append(parts, fmt.Sprint(item))
			}
			str = strings.Join(parts, ",")
		default:
			str = fmt.Sprint(tv)
		}
		_ = fc.ParseAndSet(k, str)
	}
}
