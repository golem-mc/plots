package plots

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FlagKind categorizes how a flag's value is parsed, merged, and formatted.
type FlagKind uint8

const (
	FlagBool  FlagKind = iota
	FlagInt            // non-negative integer (-1 / unlimited default for caps)
	FlagLong           // e.g. time flag
	FlagFloat          // e.g. price flag
	FlagString
	FlagStringList // e.g. blocked-cmds
	FlagBlockList  // e.g. break, place, use
	FlagIntList    // e.g. analysis (internal)
	FlagEnum       // e.g. fly, liquid-flow, titles, weather, deny-teleport, gamemode
	FlagTimedInt   // e.g. feed, heal ("<interval> <amount>")
	FlagKeep       // bool or Unix millis timestamp or duration string
	FlagPlotTitle  // `"Title" "Subtitle"` or `CONFIGURED`
	FlagMusic      // music disc item id or air
)

// TimedInt is the value of interval flags such as feed and heal.
type TimedInt struct {
	Interval int
	Value    int
}

func (t TimedInt) String() string {
	return fmt.Sprintf("%d %d", t.Interval, t.Value)
}

// KeepValue is the value of the keep flag (permanent bool or expiration Unix millis).
type KeepValue struct {
	Permanent bool
	UntilMs   int64
}

func (k KeepValue) String() string {
	if k.Permanent {
		return "true"
	}
	if k.UntilMs > 0 {
		return strconv.FormatInt(k.UntilMs, 10)
	}
	return "false"
}

// IsKept reports whether the plot is currently protected from expiry.
func (k KeepValue) IsKept(now time.Time) bool {
	if k.Permanent {
		return true
	}
	return k.UntilMs > 0 && now.UnixMilli() < k.UntilMs
}

// FlagDef defines a PlotSquared flag: its name, type, default value, parser, and metadata.
type FlagDef struct {
	Name        string
	Kind        FlagKind
	Default     any
	Category    string // caption key e.g. "flags.flag_category_boolean"
	Description string // caption key e.g. "flags.flag_description_pvp"
	Example     string
	Internal    bool
	ValuedPerm  bool     // whether permission checks plots.set.flag.<name>.<value>
	EnumValues  []string // valid values for FlagEnum
	TabComplete []string
	ParseFn     func(input string) (any, error)
	FormatFn    func(val any) string
	MergeFn     func(a, b any) any
}

// Parse parses input according to d's rules.
func (d *FlagDef) Parse(input string) (any, error) {
	if d.ParseFn != nil {
		return d.ParseFn(input)
	}
	in := strings.TrimSpace(input)
	switch d.Kind {
	case FlagBool:
		switch strings.ToLower(in) {
		case "true", "yes", "on", "allow", "1":
			return true, nil
		case "false", "no", "off", "deny", "0":
			return false, nil
		default:
			return nil, fmt.Errorf("expected boolean (true/false), got %q", input)
		}
	case FlagInt:
		v, err := strconv.Atoi(in)
		if err != nil || v < 0 {
			return nil, fmt.Errorf("expected non-negative integer, got %q", input)
		}
		return v, nil
	case FlagLong:
		v, err := strconv.ParseInt(in, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("expected integer, got %q", input)
		}
		return v, nil
	case FlagFloat:
		v, err := strconv.ParseFloat(in, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return nil, fmt.Errorf("expected positive number, got %q", input)
		}
		return v, nil
	case FlagString:
		return in, nil
	case FlagStringList:
		if in == "" {
			return []string(nil), nil
		}
		raw := strings.Split(in, ",")
		out := make([]string, 0, len(raw))
		for _, r := range raw {
			r = strings.TrimSpace(r)
			if r != "" {
				out = append(out, r)
			}
		}
		return out, nil
	case FlagBlockList:
		if in == "" {
			return []string(nil), nil
		}
		raw := strings.Split(in, ",")
		out := make([]string, 0, len(raw))
		for _, r := range raw {
			r = strings.ToLower(strings.TrimSpace(r))
			if r == "" {
				continue
			}
			if !strings.HasPrefix(r, "#") && !strings.Contains(r, ":") {
				r = "minecraft:" + r
			}
			out = append(out, r)
		}
		return out, nil
	case FlagIntList:
		if in == "" {
			return []int(nil), nil
		}
		raw := strings.Split(in, ",")
		out := make([]int, 0, len(raw))
		for _, r := range raw {
			v, err := strconv.Atoi(strings.TrimSpace(r))
			if err != nil {
				return nil, fmt.Errorf("expected integer list, got %q", input)
			}
			out = append(out, v)
		}
		return out, nil
	case FlagTimedInt:
		parts := strings.Fields(in)
		if len(parts) == 0 {
			return nil, fmt.Errorf("expected '<interval> [amount]', got %q", input)
		}
		interval, err := strconv.Atoi(parts[0])
		if err != nil || interval < 1 {
			return nil, fmt.Errorf("expected interval >= 1, got %q", parts[0])
		}
		amount := 1
		if len(parts) >= 2 {
			amount, err = strconv.Atoi(parts[1])
			if err != nil || amount < 1 {
				return nil, fmt.Errorf("expected amount >= 1, got %q", parts[1])
			}
		}
		return TimedInt{Interval: interval, Value: amount}, nil
	case FlagEnum:
		low := strings.ToLower(in)
		if slices.Contains(d.EnumValues, low) {
			return low, nil
		}
		return nil, fmt.Errorf("expected one of %s, got %q", strings.Join(d.EnumValues, ", "), input)
	default:
		return in, nil
	}
}

// Format serializes val to its database/display string form.
func (d *FlagDef) Format(val any) string {
	if d.FormatFn != nil {
		return d.FormatFn(val)
	}
	switch v := val.(type) {
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return v
	case []string:
		return strings.Join(v, ",")
	case []int:
		parts := make([]string, len(v))
		for i, n := range v {
			parts[i] = strconv.Itoa(n)
		}
		return strings.Join(parts, ",")
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}

// Merge combines two values of this flag when plots are merged or values are added.
func (d *FlagDef) Merge(a, b any) any {
	if d.MergeFn != nil {
		return d.MergeFn(a, b)
	}
	switch d.Kind {
	case FlagBool:
		ab, _ := a.(bool)
		bb, _ := b.(bool)
		return ab || bb
	case FlagInt:
		ai, _ := a.(int)
		bi, _ := b.(int)
		if ai == math.MaxInt32 || bi == math.MaxInt32 {
			return math.MaxInt32
		}
		return ai + bi
	case FlagStringList, FlagBlockList:
		as, _ := a.([]string)
		bs, _ := b.([]string)
		out := slices.Clone(as)
		for _, item := range bs {
			if !slices.Contains(out, item) {
				out = append(out, item)
			}
		}
		return out
	case FlagTimedInt:
		at, _ := a.(TimedInt)
		bt, _ := b.(TimedInt)
		return TimedInt{Interval: at.Interval + bt.Interval, Value: at.Value + bt.Value}
	default:
		return b
	}
}

// FlagContainer holds flag values with optional parent fallback (plot -> area -> global).
type FlagContainer struct {
	mu      sync.RWMutex
	parent  *FlagContainer
	values  map[string]any
	unknown map[string]string
}

// NewFlagContainer creates an empty container backed by parent.
func NewFlagContainer(parent *FlagContainer) *FlagContainer {
	return &FlagContainer{
		parent:  parent,
		values:  make(map[string]any),
		unknown: make(map[string]string),
	}
}

// SetParent updates the fallback parent container.
func (c *FlagContainer) SetParent(parent *FlagContainer) {
	c.mu.Lock()
	c.parent = parent
	c.mu.Unlock()
}

// Set stores a parsed flag value under its lowercase name.
func (c *FlagContainer) Set(name string, val any) {
	key := strings.ToLower(name)
	c.mu.Lock()
	c.values[key] = val
	c.mu.Unlock()
}

// Remove removes a locally set flag from this container.
func (c *FlagContainer) Remove(name string) bool {
	key := strings.ToLower(name)
	c.mu.Lock()
	_, ok := c.values[key]
	delete(c.values, key)
	delete(c.unknown, key)
	c.mu.Unlock()
	return ok
}

// Clear removes all locally set flags.
func (c *FlagContainer) Clear() {
	c.mu.Lock()
	clear(c.values)
	clear(c.unknown)
	c.mu.Unlock()
}

// SetUnknown preserves an unrecognized flag loaded from SQLite or worlds.yml.
func (c *FlagContainer) SetUnknown(name, raw string) {
	c.mu.Lock()
	c.unknown[strings.ToLower(name)] = raw
	c.mu.Unlock()
}

// HasLocal reports whether name is explicitly set on this container (ignoring parent).
func (c *FlagContainer) HasLocal(name string) bool {
	c.mu.RLock()
	_, ok := c.values[strings.ToLower(name)]
	c.mu.RUnlock()
	return ok
}

// GetLocal returns the value explicitly set on this container, if any.
func (c *FlagContainer) GetLocal(name string) (any, bool) {
	c.mu.RLock()
	v, ok := c.values[strings.ToLower(name)]
	c.mu.RUnlock()
	return v, ok
}

// GetValue returns the effective value of name, walking up parent containers
// and finally falling back to the global flag definition's Default.
func (c *FlagContainer) GetValue(name string) any {
	key := strings.ToLower(name)
	for cur := c; cur != nil; {
		cur.mu.RLock()
		v, ok := cur.values[key]
		p := cur.parent
		cur.mu.RUnlock()
		if ok {
			return v
		}
		cur = p
	}
	if def := LookupFlag(key); def != nil {
		return def.Default
	}
	return nil
}

// LocalFlags returns a snapshot map of all explicitly set flags on this container.
func (c *FlagContainer) LocalFlags() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]any, len(c.values))
	maps.Copy(out, c.values)
	return out
}

// UnknownFlags returns a snapshot of unrecognized flags preserved by this container.
func (c *FlagContainer) UnknownFlags() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]string, len(c.unknown))
	maps.Copy(out, c.unknown)
	return out
}

// ParseAndSet parses raw according to the registered FlagDef and stores it (or stores it as unknown).
func (c *FlagContainer) ParseAndSet(name, raw string) error {
	def := LookupFlag(name)
	if def == nil {
		c.SetUnknown(name, raw)
		return nil
	}
	val, err := def.Parse(raw)
	if err != nil {
		return err
	}
	c.Set(def.Name, val)
	return nil
}

// ParseDurationSeconds parses PlotSquared's TimeUtil duration strings like "3w 4d 2h 30m 10s".
func ParseDurationSeconds(s string) (int64, error) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(s)))
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty duration")
	}
	var total int64
	for _, f := range fields {
		if len(f) < 2 {
			return 0, fmt.Errorf("invalid duration token %q", f)
		}
		unit := f[len(f)-1]
		num, err := strconv.ParseInt(f[:len(f)-1], 10, 64)
		if err != nil || num < 0 {
			return 0, fmt.Errorf("invalid duration token %q", f)
		}
		switch unit {
		case 'y':
			total += num * 31536000
		case 'w':
			total += num * 604800
		case 'd':
			total += num * 86400
		case 'h':
			total += num * 3600
		case 'm':
			total += num * 60
		case 's':
			total += num
		default:
			return 0, fmt.Errorf("unknown duration unit %q", string(unit))
		}
	}
	return total, nil
}
