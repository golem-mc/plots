// Package lang loads PlotSquared's messages_<locale>.json captions and renders
// MiniMessage strings to golem plugin.Text components.
package lang

import (
	_ "embed"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/golem-mc/golem/plugin"
)

//go:embed messages_en.json
var defaultMessagesJSON []byte

// Catalog holds loaded message captions with fallback to embedded English defaults.
type Catalog struct {
	mu       sync.RWMutex
	messages map[string]string
	defaults map[string]string
}

// NewCatalog creates a Catalog loaded with embedded messages_en.json.
func NewCatalog() *Catalog {
	defs := make(map[string]string)
	_ = json.Unmarshal(defaultMessagesJSON, &defs)
	msgs := make(map[string]string, len(defs))
	maps.Copy(msgs, defs)
	return &Catalog{
		messages: msgs,
		defaults: defs,
	}
}

// LoadFromDir ensures lang/messages_en.json exists in dataDir and loads messages_<locale>.json.
func (c *Catalog) LoadFromDir(dataDir, locale string) error {
	langDir := filepath.Join(dataDir, "lang")
	if err := os.MkdirAll(langDir, 0o755); err != nil {
		return err
	}
	enPath := filepath.Join(langDir, "messages_en.json")
	if _, err := os.Stat(enPath); os.IsNotExist(err) {
		if err := os.WriteFile(enPath, defaultMessagesJSON, 0o644); err != nil {
			return err
		}
	}
	if locale == "" {
		locale = "en"
	}
	target := filepath.Join(langDir, "messages_"+locale+".json")
	data, err := os.ReadFile(target)
	if err != nil {
		data, err = os.ReadFile(enPath)
		if err != nil {
			return err
		}
	}
	loaded := make(map[string]string)
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}
	c.mu.Lock()
	c.messages = make(map[string]string, len(c.defaults))
	maps.Copy(c.messages, c.defaults)
	maps.Copy(c.messages, loaded)
	c.mu.Unlock()
	return nil
}

// Raw returns the raw MiniMessage template for key (falling back to key itself if unknown).
func (c *Catalog) Raw(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if v, ok := c.messages[key]; ok {
		return v
	}
	if v, ok := c.defaults[key]; ok {
		return v
	}
	return key
}

// Prefix returns the raw MiniMessage string for core.prefix.
func (c *Catalog) Prefix() string {
	return c.Raw("core.prefix")
}

// Format renders caption key with alternating placeholder/value pairs (e.g. "plot", "1;1").
// Both `<placeholder>` and `<prefix>` are expanded before MiniMessage parsing.
func (c *Catalog) Format(key string, kv ...string) plugin.Text {
	raw := c.Raw(key)
	if strings.Contains(raw, "<prefix>") {
		raw = strings.ReplaceAll(raw, "<prefix>", c.Prefix())
	}
	for i := 0; i+1 < len(kv); i += 2 {
		placeholder := "<" + kv[i] + ">"
		raw = strings.ReplaceAll(raw, placeholder, kv[i+1])
	}
	return ParseMiniMessage(raw)
}

// FormatRaw expands <prefix> and placeholders in a raw MiniMessage string.
func (c *Catalog) FormatRaw(raw string, kv ...string) plugin.Text {
	if strings.Contains(raw, "<prefix>") {
		raw = strings.ReplaceAll(raw, "<prefix>", c.Prefix())
	}
	for i := 0; i+1 < len(kv); i += 2 {
		raw = strings.ReplaceAll(raw, "<"+kv[i]+">", kv[i+1])
	}
	return ParseMiniMessage(raw)
}

var namedColors = map[string]string{
	"black":        "black",
	"dark_blue":    "dark_blue",
	"dark_green":   "dark_green",
	"dark_aqua":    "dark_aqua",
	"dark_red":     "dark_red",
	"dark_purple":  "dark_purple",
	"gold":         "gold",
	"gray":         "gray",
	"grey":         "gray",
	"dark_gray":    "dark_gray",
	"dark_grey":    "dark_gray",
	"blue":         "blue",
	"green":        "green",
	"aqua":         "aqua",
	"red":          "red",
	"light_purple": "light_purple",
	"yellow":       "yellow",
	"white":        "white",
}

// ParseMiniMessage converts a MiniMessage string (with color tags, hex colors,
// decorations, click/hover tags, and legacy &/§ codes) into a plugin.Text tree.
func ParseMiniMessage(s string) plugin.Text {
	if s == "" {
		return plugin.Text{}
	}
	// Expand legacy & color codes if present outside tags
	s = convertLegacyCodes(s)

	var styleStack []plugin.Style
	curStyle := plugin.Style{}
	var parts []plugin.Text
	var buf strings.Builder

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		parts = append(parts, plugin.Text{
			Literal: buf.String(),
			Style:   curStyle,
		})
		buf.Reset()
	}

	for i := 0; i < len(s); {
		if s[i] == '<' {
			end := findTagEnd(s, i+1)
			if end > i {
				tagBody := s[i+1 : end]
				if newStyle, pushed, handled := applyTag(curStyle, &styleStack, tagBody); handled {
					flush()
					if pushed {
						styleStack = append(styleStack, curStyle)
					}
					curStyle = newStyle
					i = end + 1
					continue
				}
			}
		}
		buf.WriteByte(s[i])
		i++
	}
	flush()

	if len(parts) == 0 {
		return plugin.Text{}
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return plugin.Text{Children: parts}
}

func findTagEnd(s string, start int) int {
	inSingle := false
	inDouble := false
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '>':
			if !inSingle && !inDouble {
				return i
			}
		}
	}
	return -1
}

func applyTag(cur plugin.Style, stack *[]plugin.Style, tag string) (plugin.Style, bool, bool) {
	if tag == "" {
		return cur, false, false
	}
	low := strings.ToLower(tag)
	if strings.HasPrefix(low, "/") {
		// Closing tag: pop one style from stack if available.
		name := low[1:]
		if isKnownTagName(name) {
			if n := len(*stack); n > 0 {
				prev := (*stack)[n-1]
				*stack = (*stack)[:n-1]
				return prev, false, true
			}
			return plugin.Style{}, false, true
		}
		return cur, false, false
	}
	if low == "reset" || low == "r" {
		*stack = (*stack)[:0]
		return plugin.Style{}, false, true
	}
	if low == "newline" || low == "br" {
		return cur, false, false
	}
	if c, ok := namedColors[low]; ok {
		next := cur
		next.Color = c
		return next, true, true
	}
	if strings.HasPrefix(low, "#") && len(low) == 7 && isHex(low[1:]) {
		next := cur
		next.Color = low
		return next, true, true
	}
	if strings.HasPrefix(low, "color:") || strings.HasPrefix(low, "colour:") || strings.HasPrefix(low, "c:") {
		val := strings.Trim(low[strings.IndexByte(low, ':')+1:], "'\"")
		if c, ok := namedColors[val]; ok {
			next := cur
			next.Color = c
			return next, true, true
		}
		if strings.HasPrefix(val, "#") && len(val) == 7 && isHex(val[1:]) {
			next := cur
			next.Color = val
			return next, true, true
		}
	}
	switch low {
	case "bold", "b":
		next := cur
		next.Bold = new(true)
		return next, true, true
	case "!bold", "!b":
		next := cur
		next.Bold = new(false)
		return next, true, true
	case "italic", "i", "em":
		next := cur
		next.Italic = new(true)
		return next, true, true
	case "!italic", "!i", "!em":
		next := cur
		next.Italic = new(false)
		return next, true, true
	case "underlined", "u":
		next := cur
		next.Underlined = new(true)
		return next, true, true
	case "!underlined", "!u":
		next := cur
		next.Underlined = new(false)
		return next, true, true
	case "strikethrough", "st":
		next := cur
		next.Strikethrough = new(true)
		return next, true, true
	case "!strikethrough", "!st":
		next := cur
		next.Strikethrough = new(false)
		return next, true, true
	case "obfuscated", "obf":
		next := cur
		next.Obfuscated = new(true)
		return next, true, true
	case "!obfuscated", "!obf":
		next := cur
		next.Obfuscated = new(false)
		return next, true, true
	}
	if strings.HasPrefix(low, "click:") {
		if click, ok := parseClickTag(tag[6:]); ok {
			next := cur
			next.Click = click
			return next, true, true
		}
	}
	if strings.HasPrefix(low, "hover:show_text:") {
		val := unquoteTagArg(tag[16:])
		ht := ParseMiniMessage(val)
		next := cur
		next.Hover = &plugin.Hover{Text: &ht}
		return next, true, true
	}
	return cur, false, false
}

func isKnownTagName(name string) bool {
	if _, ok := namedColors[name]; ok {
		return true
	}
	if strings.HasPrefix(name, "#") && len(name) == 7 && isHex(name[1:]) {
		return true
	}
	switch name {
	case "color", "colour", "c", "bold", "b", "italic", "i", "em",
		"underlined", "u", "strikethrough", "st", "obfuscated", "obf",
		"click", "hover":
		return true
	}
	return false
}

func parseClickTag(rest string) (*plugin.Click, bool) {
	before, after, ok := strings.Cut(rest, ":")
	if !ok {
		return nil, false
	}
	actionStr := strings.ToLower(strings.TrimSpace(before))
	val := unquoteTagArg(after)
	var action plugin.ClickAction
	switch actionStr {
	case "open_url":
		action = plugin.OpenURL
	case "run_command":
		action = plugin.RunCommand
	case "suggest_command":
		action = plugin.SuggestCommand
	case "change_page":
		action = plugin.ChangePage
	case "copy_to_clipboard":
		action = plugin.CopyToClipboard
	default:
		return nil, false
	}
	return &plugin.Click{Action: action, Value: val}, true
}

func unquoteTagArg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && ((s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"')) {
		return s[1 : len(s)-1]
	}
	return s
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		isDigit := c >= '0' && c <= '9'
		isLower := c >= 'a' && c <= 'f'
		isUpper := c >= 'A' && c <= 'F'
		if !isDigit && !isLower && !isUpper {
			return false
		}
	}
	return true
}

var legacyColorTags = map[byte]string{
	'0': "<black>",
	'1': "<dark_blue>",
	'2': "<dark_green>",
	'3': "<dark_aqua>",
	'4': "<dark_red>",
	'5': "<dark_purple>",
	'6': "<gold>",
	'7': "<gray>",
	'8': "<dark_gray>",
	'9': "<blue>",
	'a': "<green>",
	'b': "<aqua>",
	'c': "<red>",
	'd': "<light_purple>",
	'e': "<yellow>",
	'f': "<white>",
	'k': "<obfuscated>",
	'l': "<bold>",
	'm': "<strikethrough>",
	'n': "<underlined>",
	'o': "<italic>",
	'r': "<reset>",
}

// convertLegacyCodes turns &a / §a into MiniMessage tags so user flag text
// (e.g. greeting/farewell/plot-title) works with both MiniMessage and & codes.
func convertLegacyCodes(s string) string {
	if !strings.ContainsAny(s, "&§") {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s) + 16)
	for i := 0; i < len(s); {
		if (s[i] == '&' && i+1 < len(s)) || (s[i] == 0xc2 && i+2 < len(s) && s[i+1] == 0xa7) {
			codeIdx := i + 1
			if s[i] == 0xc2 {
				codeIdx = i + 2
			}
			c := s[codeIdx]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if tag, ok := legacyColorTags[c]; ok {
				sb.WriteString(tag)
				i = codeIdx + 1
				continue
			}
		}
		sb.WriteByte(s[i])
		i++
	}
	return sb.String()
}

// StripClickEvents removes dangerous click event tags (e.g. run_command) from user flag input.
func StripClickEvents(s string, blockedActions []string) string {
	if !strings.Contains(strings.ToLower(s), "<click:") {
		return s
	}
	blocked := make(map[string]bool, len(blockedActions))
	for _, a := range blockedActions {
		blocked[strings.ToLower(strings.TrimSpace(a))] = true
	}
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '<' {
			end := findTagEnd(s, i+1)
			if end > i {
				body := s[i+1 : end]
				low := strings.ToLower(body)
				if strings.HasPrefix(low, "click:") {
					rest := low[6:]
					if idx := strings.IndexByte(rest, ':'); idx > 0 {
						act := rest[:idx]
						if len(blocked) == 0 || blocked[act] {
							i = end + 1
							continue
						}
					}
				}
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}
