package plots

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	flagRegistryMu sync.RWMutex
	flagRegistry   = make(map[string]*FlagDef)

	// GlobalFlags is the root FlagContainer holding default values for all registered flags.
	GlobalFlags = NewFlagContainer(nil)
)

// RegisterFlag registers a flag definition in the global registry and sets its default on GlobalFlags.
func RegisterFlag(def *FlagDef) {
	key := strings.ToLower(def.Name)
	def.Name = key
	flagRegistryMu.Lock()
	flagRegistry[key] = def
	flagRegistryMu.Unlock()
	GlobalFlags.Set(key, def.Default)
}

// LookupFlag looks up a registered flag by name (case-insensitive).
func LookupFlag(name string) *FlagDef {
	flagRegistryMu.RLock()
	defer flagRegistryMu.RUnlock()
	return flagRegistry[strings.ToLower(name)]
}

// AllFlags returns all registered FlagDefs sorted by name.
func AllFlags() []*FlagDef {
	flagRegistryMu.RLock()
	out := make([]*FlagDef, 0, len(flagRegistry))
	for _, d := range flagRegistry {
		out = append(out, d)
	}
	flagRegistryMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func init() {
	registerBuiltinFlags()
}

func registerBuiltinFlags() {
	registerBoolFlags()
	registerEnumFlags()
	registerValueFlags()
}

func registerBoolFlags() {
	// Block type list flags
	for _, b := range []struct {
		name, desc string
	}{
		{"break", "flags.flag_description_break"},
		{"use", "flags.flag_description_use"},
		{"place", "flags.flag_description_place"},
	} {
		RegisterFlag(&FlagDef{
			Name:        b.name,
			Kind:        FlagBlockList,
			Default:     []string(nil),
			Category:    "flags.flag_category_block_list",
			Description: b.desc,
			Example:     "stone,oak_planks",
			ValuedPerm:  true,
		})
	}

	// Boolean flags (exact upstream names and defaults from GlobalFlagContainer.java)
	boolFlags := []struct {
		name string
		def  bool
		desc string
	}{
		{"animal-attack", false, "flags.flag_description_animal_attack"},
		{"animal-interact", false, "flags.flag_description_animal_interact"},
		{"beacon-effects", true, "flags.flag_description_beacon_effects"},
		{"block-burn", false, "flags.flag_description_block_burn"},
		{"block-ignition", true, "flags.flag_description_block_ignition"},
		{"chat", true, "flags.flag_description_chat"},
		{"concrete-harden", true, "flags.flag_description_concrete_harden"},
		{"copper-oxide", false, "flags.flag_description_copper_oxide"},
		{"coral-dry", false, "flags.flag_description_coral_dry"},
		{"crop-grow", true, "flags.flag_description_crop_grow"},
		{"deny-exit", false, "flags.flag_description_deny_exit"},
		{"deny-portals", false, "flags.flag_description_deny_portals"},
		{"deny-portal-travel", false, "flags.flag_description_deny_portal_travel"},
		{"device-interact", false, "flags.flag_description_device_interact"},
		{"disable-physics", false, "flags.flag_description_disable_physics"},
		{"drop-protection", false, "flags.flag_description_drop_protection"},
		{"edit-sign", false, "flags.flag_description_edit_sign"},
		{"entity-change-block", false, "flags.flag_description_entity_change_block"},
		{"explosion", false, "flags.flag_description_explosion"},
		{"fishing", false, "flags.flag_description_fishing"},
		{"forcefield", false, "flags.flag_description_forcefield"},
		{"grass-grow", true, "flags.flag_description_grass_grow"},
		{"hanging-break", false, "flags.flag_description_hanging_break"},
		{"hanging-place", false, "flags.flag_description_hanging_place"},
		{"hide-info", false, "flags.flag_description_hide_info"},
		{"hostile-attack", false, "flags.flag_description_hostile_attack"},
		{"hostile-interact", false, "flags.flag_description_hostile_interact"},
		{"ice-form", false, "flags.flag_description_ice_form"},
		{"ice-melt", false, "flags.flag_description_ice_melt"},
		{"instabreak", false, "flags.flag_description_instabreak"},
		{"interaction-interact", false, "flags.flag_description_interaction_interact"},
		{"invincible", false, "flags.flag_description_invincible"},
		{"item-drop", true, "flags.flag_description_item_drop"},
		{"keep-inventory", false, "flags.flag_description_keep_inventory"},
		{"kelp-grow", true, "flags.flag_description_kelp_grow"},
		{"leaf-decay", true, "flags.flag_description_leaf_decay"},
		{"lectern-read-book", false, "flags.flag_description_lectern_read_book"},
		{"misc-break", false, "flags.flag_description_misc_break"},
		{"misc-interact", false, "flags.flag_description_misc_interact"},
		{"misc-place", false, "flags.flag_description_misc_place"},
		{"mob-break", false, "flags.flag_description_mob_break"},
		{"mob-place", false, "flags.flag_description_mob_place"},
		{"mycel-grow", true, "flags.flag_description_mycel_grow"},
		{"notify-enter", false, "flags.flag_description_notify_enter"},
		{"notify-leave", false, "flags.flag_description_notify_leave"},
		{"no-worldedit", false, "flags.flag_description_no_worldedit"},
		{"player-interact", false, "flags.flag_description_player_interact"},
		{"prevent-creative-copy", false, "flags.flag_description_prevent_creative_copy"},
		{"projectile-change-block", false, "flags.flag_description_projectile_change_block"},
		{"projectiles", false, "flags.flag_description_projectiles"},
		{"pve", false, "flags.flag_description_pve"},
		{"pvp", false, "flags.flag_description_pvp"},
		{"redstone", true, "flags.flag_description_redstone"},
		{"sculk-sensor-interact", false, "flags.flag_description_sculk_sensor_interact"},
		{"server-plot", false, "flags.flag_description_server_plot"},
		{"snow-form", false, "flags.flag_description_snow_form"},
		{"snow-melt", true, "flags.flag_description_snow_melt"},
		{"soil-dry", false, "flags.flag_description_soil_dry"},
		{"tamed-attack", false, "flags.flag_description_tamed_attack"},
		{"tamed-interact", false, "flags.flag_description_tamed_interact"},
		{"tile-drop", true, "flags.flag_description_tile_drop"},
		{"untrusted-visit", true, "flags.flag_description_untrusted_visit"},
		{"vehicle-break", false, "flags.flag_description_vehicle_break"},
		{"vehicle-place", false, "flags.flag_description_vehicle_place"},
		{"vehicle-use", false, "flags.flag_description_vehicle_use"},
		{"villager-interact", false, "flags.flag_description_villager_interact"},
		{"vine-grow", true, "flags.flag_description_vine_grow"},
		{"weaving-death-place", false, "flags.flag_description_weaving_death_place"},
		{"wind-charge", false, "flags.flag_description_wind_charge"},
	}
	for _, bf := range boolFlags {
		RegisterFlag(&FlagDef{
			Name:        bf.name,
			Kind:        FlagBool,
			Default:     bf.def,
			Category:    "flags.flag_category_boolean",
			Description: bf.desc,
			Example:     "true",
			ValuedPerm:  true,
			TabComplete: []string{"true", "false"},
		})
	}
}

func registerEnumFlags() {
	// Price (DoubleFlag)
	RegisterFlag(&FlagDef{
		Name:        "price",
		Kind:        FlagFloat,
		Default:     0.0,
		Category:    "flags.flag_category_decimal",
		Description: "flags.flag_description_price",
		Example:     "100.0",
		ValuedPerm:  true,
		ParseFn: func(input string) (any, error) {
			v, err := strconv.ParseFloat(strings.TrimSpace(input), 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
				return nil, fmt.Errorf("price must be > 0, got %q", input)
			}
			return v, nil
		},
	})

	// Enum flags
	RegisterFlag(&FlagDef{
		Name:        "deny-teleport",
		Kind:        FlagEnum,
		Default:     "none",
		Category:    "flags.flag_category_enum",
		Description: "flags.flag_description_deny_teleport",
		Example:     "nonmembers",
		ValuedPerm:  true,
		EnumValues:  []string{"none", "members", "nonmembers", "trusted", "nontrusted", "nonowners"},
		TabComplete: []string{"members", "nonmembers", "trusted", "nontrusted", "nonowners"},
		ParseFn: func(input string) (any, error) {
			low := strings.ToLower(strings.TrimSpace(input))
			valid := []string{"none", "members", "nonmembers", "trusted", "nontrusted", "nonowners"}
			if slices.Contains(valid, low) {
				return low, nil
			}
			return nil, fmt.Errorf("expected one of members, nonmembers, trusted, nontrusted, nonowners")
		},
	})

	RegisterFlag(&FlagDef{
		Name:        "fly",
		Kind:        FlagEnum,
		Default:     "default",
		Category:    "flags.flag_category_boolean",
		Description: "flags.flag_description_flight",
		Example:     "true",
		ValuedPerm:  true,
		EnumValues:  []string{"enabled", "disabled", "default"},
		TabComplete: []string{"true", "false", "default"},
		ParseFn: func(input string) (any, error) {
			switch strings.ToLower(strings.TrimSpace(input)) {
			case "true", "enabled", "allow":
				return "enabled", nil
			case "false", "disabled", "disallow":
				return "disabled", nil
			default:
				return "default", nil
			}
		},
	})

	RegisterFlag(&FlagDef{
		Name:        "liquid-flow",
		Kind:        FlagEnum,
		Default:     "default",
		Category:    "flags.flag_category_boolean",
		Description: "flags.flag_description_liquid_flow",
		Example:     "true",
		ValuedPerm:  true,
		EnumValues:  []string{"enabled", "disabled", "default"},
		TabComplete: []string{"true", "false", "default"},
		ParseFn: func(input string) (any, error) {
			switch strings.ToLower(strings.TrimSpace(input)) {
			case "true", "enabled", "allow":
				return "enabled", nil
			case "false", "disabled", "disallow":
				return "disabled", nil
			default:
				return "default", nil
			}
		},
	})

	RegisterFlag(&FlagDef{
		Name:        "titles",
		Kind:        FlagEnum,
		Default:     "none",
		Category:    "flags.flag_category_enum",
		Description: "flags.flag_description_titles",
		Example:     "true",
		ValuedPerm:  true,
		EnumValues:  []string{"none", "true", "false"},
		TabComplete: []string{"none", "true", "false"},
	})

	RegisterFlag(&FlagDef{
		Name:        "weather",
		Kind:        FlagEnum,
		Default:     string(WeatherOff),
		Category:    "flags.flag_category_weather",
		Description: "flags.flag_description_weather",
		Example:     "rain",
		ValuedPerm:  true,
		EnumValues:  []string{"rain", "clear", "world", "off"},
		TabComplete: []string{"clear", "rain", "reset"},
		ParseFn: func(input string) (any, error) {
			switch strings.ToLower(strings.TrimSpace(input)) {
			case "rain", "downfall":
				return string(WeatherRain), nil
			case "clear", "sun":
				return string(WeatherClear), nil
			case "reset", "world":
				return string(WeatherWorld), nil
			default:
				return string(WeatherOff), nil
			}
		},
	})

	parseGamemode := func(input string) (any, error) {
		switch strings.ToLower(strings.TrimSpace(input)) {
		case "creative", "c", "1":
			return "creative", nil
		case "adventure", "a", "2":
			return "adventure", nil
		case "spectator", "sp", "3":
			return "spectator", nil
		case "survival", "s", "0":
			return "survival", nil
		default:
			return "default", nil
		}
	}
	for _, gm := range []struct{ name, desc string }{
		{"gamemode", "flags.flag_description_gamemode"},
		{"guest-gamemode", "flags.flag_description_guest_gamemode"},
	} {
		RegisterFlag(&FlagDef{
			Name:        gm.name,
			Kind:        FlagEnum,
			Default:     "default",
			Category:    "flags.flag_category_gamemode",
			Description: gm.desc,
			Example:     "survival",
			ValuedPerm:  true,
			EnumValues:  []string{"default", "survival", "creative", "adventure", "spectator"},
			TabComplete: []string{"survival", "creative", "adventure", "spectator"},
			ParseFn:     parseGamemode,
		})
	}

	// Internal flags
	RegisterFlag(&FlagDef{
		Name:        "analysis",
		Kind:        FlagIntList,
		Default:     []int(nil),
		Category:    "flags.flag_category_integers",
		Description: "flags.flag_description_analysis",
		Internal:    true,
	})
	RegisterFlag(&FlagDef{
		Name:        "done",
		Kind:        FlagString,
		Default:     "",
		Category:    "flags.flag_category_string",
		Description: "flags.flag_description_done",
		Internal:    true,
	})
}

func registerValueFlags() {
	// Entity cap integer flags (default math.MaxInt32 = unlimited, matching upstream)
	capFlags := []struct{ name, desc string }{
		{"animal-cap", "flags.flag_description_animal_cap"},
		{"entity-cap", "flags.flag_description_entity_cap"},
		{"hostile-cap", "flags.flag_description_hostile_cap"},
		{"misc-cap", "flags.flag_description_misc_cap"},
		{"mob-cap", "flags.flag_description_mob_cap"},
		{"vehicle-cap", "flags.flag_description_vehicle_cap"},
	}
	for _, cf := range capFlags {
		RegisterFlag(&FlagDef{
			Name:        cf.name,
			Kind:        FlagInt,
			Default:     math.MaxInt32,
			Category:    "flags.flag_category_integers",
			Description: cf.desc,
			Example:     "10",
			ValuedPerm:  true,
		})
	}

	// Time flag
	RegisterFlag(&FlagDef{
		Name:        "time",
		Kind:        FlagLong,
		Default:     int64(math.MinInt64),
		Category:    "flags.flag_category_integers",
		Description: "flags.flag_description_time",
		Example:     "6000",
		ValuedPerm:  true,
	})

	// Blocked commands
	RegisterFlag(&FlagDef{
		Name:        "blocked-cmds",
		Kind:        FlagStringList,
		Default:     []string(nil),
		Category:    "flags.flag_category_string_list",
		Description: "flags.flag_description_blocked_cmds",
		Example:     "sethome,tpa",
		ValuedPerm:  true,
	})

	// Keep flag
	RegisterFlag(&FlagDef{
		Name:        "keep",
		Kind:        FlagKeep,
		Default:     KeepValue{},
		Category:    "flags.flag_category_mixed",
		Description: "flags.flag_description_keep",
		Example:     "3w 4d 2h",
		ValuedPerm:  true,
		ParseFn: func(input string) (any, error) {
			in := strings.TrimSpace(input)
			if v, err := strconv.ParseInt(in, 10, 64); err == nil {
				if v < 0 {
					return nil, fmt.Errorf("invalid keep timestamp %q", input)
				}
				return KeepValue{UntilMs: v}, nil
			}
			switch strings.ToLower(in) {
			case "true":
				return KeepValue{Permanent: true}, nil
			case "false":
				return KeepValue{}, nil
			default:
				secs, err := ParseDurationSeconds(in)
				if err != nil {
					return nil, err
				}
				return KeepValue{UntilMs: time.Now().UnixMilli() + secs*1000}, nil
			}
		},
		FormatFn: func(val any) string {
			kv, _ := val.(KeepValue)
			return kv.String()
		},
		MergeFn: func(a, b any) any {
			ka, _ := a.(KeepValue)
			kb, _ := b.(KeepValue)
			if ka.Permanent || kb.Permanent {
				return KeepValue{Permanent: true}
			}
			if kb.UntilMs > ka.UntilMs {
				return kb
			}
			return ka
		},
	})

	// Music flag
	RegisterFlag(&FlagDef{
		Name:        "music",
		Kind:        FlagMusic,
		Default:     "minecraft:air",
		Category:    "flags.flag_category_music",
		Description: "flags.flag_description_music",
		Example:     "ward",
		ValuedPerm:  true,
		ParseFn: func(input string) (any, error) {
			in := strings.ToLower(strings.TrimSpace(input))
			if in == "" || in == "air" || in == "minecraft:air" || in == "none" || in == "off" {
				return "minecraft:air", nil
			}
			in = strings.TrimPrefix(in, "minecraft:")
			if !strings.HasPrefix(in, "music_disc_") {
				in = "music_disc_" + in
			}
			return "minecraft:" + in, nil
		},
	})

	// String flags
	for _, sf := range []struct{ name, desc, ex string }{
		{"description", "flags.flag_description_description", "My awesome plot"},
		{"greeting", "flags.flag_description_greeting", "Welcome to my plot!"},
		{"farewell", "flags.flag_description_farewell", "Goodbye!"},
	} {
		RegisterFlag(&FlagDef{
			Name:        sf.name,
			Kind:        FlagString,
			Default:     "",
			Category:    "flags.flag_category_string",
			Description: sf.desc,
			Example:     sf.ex,
			ValuedPerm:  false,
		})
	}

	// Plot title flag
	RegisterFlag(&FlagDef{
		Name:        "plot-title",
		Kind:        FlagPlotTitle,
		Default:     PlotTitleConfigured,
		Category:    "flags.flag_category_string",
		Description: "flags.flag_description_title",
		Example:     `"A Title" "The subtitle"`,
		ValuedPerm:  false,
		ParseFn: func(input string) (any, error) {
			in := strings.TrimSpace(input)
			if in == "CONFIGURED" {
				return PlotTitleConfigured, nil
			}
			if !strings.Contains(in, `"`) {
				return PlotTitle{Title: in}, nil
			}
			parts := splitQuoted(in)
			if len(parts) == 0 || len(parts) > 2 {
				return nil, fmt.Errorf("expected '\"Title\" \"Subtitle\"', got %q", input)
			}
			if len(parts) == 1 {
				return PlotTitle{Title: parts[0]}, nil
			}
			return PlotTitle{Title: parts[0], Subtitle: parts[1]}, nil
		},
		FormatFn: func(val any) string {
			pt, ok := val.(PlotTitle)
			if !ok || pt.Configured {
				return "CONFIGURED"
			}
			return fmt.Sprintf("%q %q", pt.Title, pt.Subtitle)
		},
	})

	// Timed interval flags (feed & heal)
	for _, tf := range []struct{ name, desc string }{
		{"feed", "flags.flag_description_feed"},
		{"heal", "flags.flag_description_heal"},
	} {
		RegisterFlag(&FlagDef{
			Name:        tf.name,
			Kind:        FlagTimedInt,
			Default:     TimedInt{},
			Category:    "flags.flag_category_intervals",
			Description: tf.desc,
			Example:     "10 2",
			ValuedPerm:  true,
		})
	}
}

func splitQuoted(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	hasToken := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			if inQuote {
				out = append(out, cur.String())
				cur.Reset()
				inQuote = false
				hasToken = false
			} else {
				inQuote = true
				hasToken = true
			}
			continue
		}
		if !inQuote && (ch == ' ' || ch == '\t') {
			if hasToken {
				out = append(out, cur.String())
				cur.Reset()
				hasToken = false
			}
			continue
		}
		cur.WriteByte(ch)
		hasToken = true
	}
	if hasToken {
		out = append(out, cur.String())
	}
	return out
}
