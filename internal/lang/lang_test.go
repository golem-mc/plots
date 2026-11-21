package lang

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golem-mc/golem/plugin"
)

func TestCatalogAndMiniMessage(t *testing.T) {
	cat := NewCatalog()
	if cat.Raw("core.prefix") == "core.prefix" {
		t.Fatalf("expected embedded messages_en.json to contain core.prefix")
	}

	txt := cat.Format("move.move_success", "origin", "1;1", "target", "2;2")
	plain := txt.PlainString()
	if !strings.Contains(plain, "1;1") || !strings.Contains(plain, "2;2") {
		t.Errorf("formatted message missing placeholders: %q", plain)
	}

	// Click + hover + hex + bold
	styled := ParseMiniMessage("<#112233><bold><click:run_command:'/p home'><hover:show_text:'<green>Go'>Click</hover></click></bold></#112233>")
	if styled.PlainString() != "Click" {
		t.Errorf("PlainString = %q, want Click", styled.PlainString())
	}
	if styled.Style.Color != "#112233" || styled.Style.Bold == nil || !*styled.Style.Bold {
		t.Errorf("unexpected style: %+v", styled.Style)
	}
	if styled.Style.Click == nil || styled.Style.Click.Action != plugin.RunCommand || styled.Style.Click.Value != "/p home" {
		t.Errorf("unexpected click: %+v", styled.Style.Click)
	}
	if styled.Style.Hover == nil || styled.Style.Hover.Text == nil || styled.Style.Hover.Text.PlainString() != "Go" {
		t.Errorf("unexpected hover: %+v", styled.Style.Hover)
	}

	// Legacy & codes
	leg := ParseMiniMessage("&aHello &lWorld")
	if leg.PlainString() != "Hello World" {
		t.Errorf("legacy PlainString = %q", leg.PlainString())
	}

	// StripClickEvents
	stripped := StripClickEvents("<red>Hi <click:run_command:'/op me'>bad</click>", []string{"RUN_COMMAND"})
	if strings.Contains(stripped, "run_command") {
		t.Errorf("click event not stripped: %q", stripped)
	}

	// LoadFromDir writes messages_en.json and overrides custom entries
	dir := t.TempDir()
	if err := cat.LoadFromDir(dir, "en"); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	enPath := filepath.Join(dir, "lang", "messages_en.json")
	if _, err := os.Stat(enPath); err != nil {
		t.Fatalf("expected messages_en.json created: %v", err)
	}
}
