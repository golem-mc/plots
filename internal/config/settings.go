// Package config manages PlotSquared's YAML configuration files under
// plugins/plots/config/ (settings.yml, storage.yml, worlds.yml) and
// embedded plot world templates.
package config

import (
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v4"
)

// Settings mirrors upstream PlotSquared's config/settings.yml.
type Settings struct {
	Platform              string                   `yaml:"platform"`
	Debug                 bool                     `yaml:"debug"`
	HighFrequencyListener bool                     `yaml:"high-frequency-listener"`
	AutoClear             map[string]AutoClearTask `yaml:"auto_clear,omitempty"`
	ChunkProcessor        ChunkProcessorSettings   `yaml:"chunk_processor"`
	UUID                  UUIDSettings             `yaml:"uuid"`
	General               GeneralSettings          `yaml:"general"`
	Schematics            SchematicsSettings       `yaml:"schematics"`
	Paths                 PathsSettings            `yaml:"paths"`
	Timeformat            TimeformatSettings       `yaml:"timeformat"`
	Done                  DoneSettings             `yaml:"done"`
	Chat                  ChatSettings             `yaml:"chat"`
	Limit                 LimitSettings            `yaml:"limit"`
	Backup                BackupSettings           `yaml:"backup"`
	Confirmation          ConfirmationSettings     `yaml:"confirmation"`
	Teleport              TeleportSettings         `yaml:"teleport"`
	Redstone              RedstoneSettings         `yaml:"redstone"`
	Claim                 ClaimSettings            `yaml:"claim"`
	Ratings               RatingsSettings          `yaml:"ratings"`
	PaperComponents       PaperComponentsSettings  `yaml:"paper_components"`
	Queue                 QueueSettings            `yaml:"queue"`
	TabCompletions        TabCompletionsSettings   `yaml:"tab_completions"`
	Titles                TitlesSettings           `yaml:"titles"`
	Flags                 FlagsSettings            `yaml:"flags"`
	EnabledComponents     EnabledComponents        `yaml:"enabled_components"`
}

type AutoClearTask struct {
	Threshold              int                  `yaml:"threshold"`
	RequiredPlots          int                  `yaml:"required-plots"`
	Confirmation           bool                 `yaml:"confirmation"`
	Days                   int                  `yaml:"days"`
	SkipAccountAgeDays     int                  `yaml:"skip-account-age-days"`
	DeleteIfOwnerIsUnknown bool                 `yaml:"delete-if-owner-is-unknown"`
	Worlds                 []string             `yaml:"worlds"`
	Calibration            AutoClearCalibration `yaml:"calibration"`
}

type AutoClearCalibration struct {
	Variety   int `yaml:"variety"`
	VarietySD int `yaml:"variety-sd"`
	Changes   int `yaml:"changes"`
	ChangesSD int `yaml:"changes-sd"`
	Faces     int `yaml:"faces"`
	FacesSD   int `yaml:"faces-sd"`
	DataSD    int `yaml:"data-sd"`
	Air       int `yaml:"air"`
	AirSD     int `yaml:"air-sd"`
	Data      int `yaml:"data"`
}

type ChunkProcessorSettings struct {
	AutoTrim       bool `yaml:"auto-trim"`
	MaxTiles       int  `yaml:"max-tiles"`
	MaxEntities    int  `yaml:"max-entities"`
	DisablePhysics bool `yaml:"disable-physics"`
}

type UUIDSettings struct {
	Offline        bool `yaml:"offline"`
	ForceLowercase bool `yaml:"force-lowercase"`
}

type GeneralSettings struct {
	Scientific           bool     `yaml:"scientific"`
	MergeReplaceWall     bool     `yaml:"merge-replace-wall"`
	AlwaysShowExplosions bool     `yaml:"always-show-explosions"`
	InvalidBlocks        []string `yaml:"invalid-blocks"`
}

type SchematicsSettings struct {
	PasteOnTop             bool `yaml:"paste-on-top"`
	PasteRoadOnTop         bool `yaml:"paste-road-on-top"`
	PasteMismatches        bool `yaml:"paste-mismatches"`
	UseWallInRoadSchemHigh bool `yaml:"use-wall-in-road-schem-height"`
}

type PathsSettings struct {
	Schematics                   string `yaml:"schematics"`
	Templates                    string `yaml:"templates"`
	UseSchematicsPathForGenSchem bool   `yaml:"use-schematics-path-for-gen-schematics"`
}

type TimeformatSettings struct {
	DateFormat string `yaml:"date-format"`
	TimeZone   string `yaml:"time-zone"`
}

type DoneSettings struct {
	RequiredForDownload bool `yaml:"required-for-download"`
	RequiredForRatings  bool `yaml:"required-for-ratings"`
	RestrictBuilding    bool `yaml:"restrict-building"`
	CountsTowardsLimit  bool `yaml:"counts-towards-limit"`
}

type ChatSettings struct {
	LogPlotchatToConsole    bool     `yaml:"log-plotchat-to-console"`
	NotificationAsActionbar bool     `yaml:"notification-as-actionbar"`
	ClickEventActionsRemove []string `yaml:"click-event-actions-to-remove"`
}

type LimitSettings struct {
	Global   bool `yaml:"global"`
	MaxPlots int  `yaml:"max-plots"`
}

type BackupSettings struct {
	AutomaticBackups bool `yaml:"automatic-backups"`
	BackupLimit      int  `yaml:"backup-limit"`
	DeleteOnUnclaim  bool `yaml:"delete-on-unclaim"`
}

type ConfirmationSettings struct {
	TimeoutSeconds int `yaml:"confirmation-timeout-seconds"`
}

type TeleportSettings struct {
	OnDeath           bool `yaml:"on-death"`
	OnLogin           bool `yaml:"on-login"`
	OnClaim           bool `yaml:"on-claim"`
	OnAuto            bool `yaml:"on-auto"`
	Delay             int  `yaml:"delay"`
	OnClear           bool `yaml:"on-clear"`
	OnDelete          bool `yaml:"on-delete"`
	PerWorldVisit     bool `yaml:"per-world-visit"`
	VisitMergedOwners bool `yaml:"visit-merged-owners"`
	SizedBased        bool `yaml:"sized-based"`
}

type RedstoneSettings struct {
	DisableUnoccupied        bool `yaml:"disable-unoccupied"`
	DisableOffline           bool `yaml:"disable-offline"`
	DetectInvalidEdgePistons bool `yaml:"detect-invalid-edge-pistons"`
}

type ClaimSettings struct {
	MaxAutoArea int `yaml:"max-auto-area"`
}

type RatingsSettings struct {
	UseLikes   bool     `yaml:"use-likes"`
	Categories []string `yaml:"categories"`
}

type PaperComponentsSettings struct {
	PaperListeners   bool `yaml:"paper-listeners"`
	EntityPathing    bool `yaml:"entity-pathing"`
	EntityMovement   bool `yaml:"entity-movement"`
	CancelChunkSpawn bool `yaml:"cancel-chunk-spawn"`
	TileEntityCheck  bool `yaml:"tile-entity-check"`
}

type QueueSettings struct {
	MaxIterationTime int  `yaml:"max-iteration-time"`
	InitialBatchSize int  `yaml:"initial-batch-size"`
	NotifyProgress   bool `yaml:"notify-progress"`
	NotifyInterval   int  `yaml:"notify-interval"`
	NotifyWait       int  `yaml:"notify-wait"`
	UpdateEdges      bool `yaml:"update-edges"`
}

type TabCompletionsSettings struct {
	CacheExpiration int `yaml:"cache-expiration"`
}

type TitlesSettings struct {
	DisplayTitles            bool `yaml:"display-titles"`
	FadeIn                   int  `yaml:"titles-fade-in"`
	Stay                     int  `yaml:"titles-stay"`
	FadeOut                  int  `yaml:"titles-fade-out"`
	TitlesAsActionbar        bool `yaml:"titles-as-actionbar"`
	DisplayDefaultServerPlot bool `yaml:"display-default-on-server-plot"`
}

type FlagsSettings struct {
	InstabreakConsiderTool bool `yaml:"instabreak-consider-tool"`
}

type EnabledComponents struct {
	Database                   bool     `yaml:"database"`
	Events                     bool     `yaml:"events"`
	Commands                   bool     `yaml:"commands"`
	PersistentMeta             bool     `yaml:"persistent-meta"`
	RatingCache                bool     `yaml:"rating-cache"`
	WorldEditRestrictions      bool     `yaml:"worldedit-restrictions"`
	Economy                    bool     `yaml:"economy"`
	PlotExpiry                 bool     `yaml:"plot-expiry"`
	ChunkProcessor             bool     `yaml:"chunk-processor"`
	KillRoadMobs               bool     `yaml:"kill-road-mobs"`
	KillOwnedRoadMobs          bool     `yaml:"kill-owned-road-mobs"`
	KillNamedRoadMobs          bool     `yaml:"kill-named-road-mobs"`
	KillRoadItems              bool     `yaml:"kill-road-items"`
	KillRoadVehicles           bool     `yaml:"kill-road-vehicles"`
	CommentNotifier            bool     `yaml:"comment-notifier"`
	Worlds                     bool     `yaml:"worlds"`
	DatabasePurger             bool     `yaml:"database-purger"`
	BanDeleter                 bool     `yaml:"ban-deleter"`
	PersistentRoadRegen        bool     `yaml:"persistent-road-regen"`
	ComponentPresets           bool     `yaml:"component-presets"`
	PerUserLocale              bool     `yaml:"per-user-locale"`
	DefaultLocale              string   `yaml:"default-locale"`
	ExtendedUsernameCompletion bool     `yaml:"extended-username-completion"`
	TabCompletedAliases        []string `yaml:"tab-completed-aliases"`
	DisableBeaconOverflow      bool     `yaml:"disable-beacon-effect-overflow"`
}

// DefaultSettings returns a Settings struct populated with upstream defaults.
func DefaultSettings() *Settings {
	return &Settings{
		Platform:              "golem",
		Debug:                 false,
		HighFrequencyListener: true,
		AutoClear: map[string]AutoClearTask{
			"task1": {
				Threshold:          -1,
				RequiredPlots:      -1,
				Confirmation:       true,
				Days:               90,
				SkipAccountAgeDays: -1,
				Worlds:             []string{"*"},
				Calibration:        AutoClearCalibration{ChangesSD: 1},
			},
		},
		ChunkProcessor: ChunkProcessorSettings{
			MaxTiles:    4096,
			MaxEntities: 512,
		},
		General: GeneralSettings{
			MergeReplaceWall: true,
			InvalidBlocks: []string{
				"anvil", "barrier", "beacon", "brewing_stand", "bubble_column", "cake",
				"chest", "cobweb", "comparator", "damaged_anvil", "daylight_detector",
				"dragon_egg", "enchanting_table", "end_gateway", "end_portal", "end_rod",
				"ender_chest", "flower_pot", "lever", "redstone_wire", "repeater",
				"redstone_torch", "torch", "trapped_chest", "tripwire", "tripwire_hook",
				"turtle_egg", "bell", "oak_sign", "oak_wall_sign", "oak_door",
				"oak_button", "oak_pressure_plate", "oak_sapling", "oak_trapdoor",
			},
		},
		Schematics: SchematicsSettings{
			PasteOnTop:             true,
			PasteRoadOnTop:         true,
			PasteMismatches:        true,
			UseWallInRoadSchemHigh: true,
		},
		Paths: PathsSettings{
			Schematics: "schematics",
			Templates:  "templates",
		},
		Timeformat: TimeformatSettings{
			DateFormat: "2006-01-02 15:04:05 MST",
			TimeZone:   "GMT",
		},
		Done: DoneSettings{
			CountsTowardsLimit: true,
		},
		Chat: ChatSettings{
			LogPlotchatToConsole:    true,
			ClickEventActionsRemove: []string{"OPEN_URL", "OPEN_FILE", "RUN_COMMAND", "SUGGEST_COMMAND", "CHANGE_PAGE", "COPY_TO_CLIPBOARD"},
		},
		Limit: LimitSettings{
			MaxPlots: 127,
		},
		Backup: BackupSettings{
			AutomaticBackups: true,
			BackupLimit:      3,
			DeleteOnUnclaim:  true,
		},
		Confirmation: ConfirmationSettings{
			TimeoutSeconds: 20,
		},
		Teleport: TeleportSettings{
			OnClaim:           true,
			OnAuto:            true,
			VisitMergedOwners: true,
			SizedBased:        true,
		},
		Claim: ClaimSettings{
			MaxAutoArea: 4,
		},
		PaperComponents: PaperComponentsSettings{
			PaperListeners:   true,
			EntityPathing:    true,
			CancelChunkSpawn: true,
			TileEntityCheck:  true,
		},
		Queue: QueueSettings{
			MaxIterationTime: 30,
			InitialBatchSize: 5,
			NotifyProgress:   true,
			NotifyInterval:   5000,
			NotifyWait:       5000,
			UpdateEdges:      true,
		},
		TabCompletions: TabCompletionsSettings{
			CacheExpiration: 15,
		},
		Titles: TitlesSettings{
			DisplayTitles: true,
			FadeIn:        10,
			Stay:          50,
			FadeOut:       20,
		},
		EnabledComponents: EnabledComponents{
			Database:                   true,
			Events:                     true,
			Commands:                   true,
			PersistentMeta:             true,
			RatingCache:                true,
			WorldEditRestrictions:      true,
			CommentNotifier:            true,
			PersistentRoadRegen:        true,
			ComponentPresets:           true,
			DefaultLocale:              "en",
			ExtendedUsernameCompletion: true,
			TabCompletedAliases:        []string{"plot", "plots", "p", "plotsquared", "plot2", "p2", "ps", "2", "plotme"},
			DisableBeaconOverflow:      true,
		},
	}
}

// LoadSettings reads config/settings.yml, writing defaults if the file does not exist.
func LoadSettings(dataDir string) (*Settings, error) {
	cfgDir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(cfgDir, "settings.yml")
	s := DefaultSettings()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, SaveSettings(dataDir, s)
		}
		return nil, err
	}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, s); err != nil {
			return nil, err
		}
	}
	s.Platform = "golem"
	return s, nil
}

// SaveSettings writes config/settings.yml.
func SaveSettings(dataDir string, s *Settings) error {
	cfgDir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return err
	}
	out, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	header := "# PlotSquared configuration (settings.yml)\n"
	return os.WriteFile(filepath.Join(cfgDir, "settings.yml"), append([]byte(header), out...), 0o644)
}
