package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/golem-mc/plotsquared"
)

func TestConfigRoundTripAndTemplates(t *testing.T) {
	dir := t.TempDir()

	s, err := LoadSettings(dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.Limit.MaxPlots != 127 || !s.EnabledComponents.Database {
		t.Errorf("unexpected default settings: %+v", s)
	}
	s.EnabledComponents.Economy = true
	if err := SaveSettings(dir, s); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	s2, err := LoadSettings(dir)
	if err != nil || !s2.EnabledComponents.Economy {
		t.Fatalf("reload settings: err=%v eco=%v", err, s2.EnabledComponents.Economy)
	}

	sc, err := LoadStorageConfig(dir)
	if err != nil {
		t.Fatalf("LoadStorageConfig: %v", err)
	}
	if !sc.SQLite.Use || sc.SQLite.DB != "storage" {
		t.Errorf("unexpected storage defaults: %+v", sc)
	}

	wf, err := LoadWorlds(dir)
	if err != nil {
		t.Fatalf("LoadWorlds: %v", err)
	}
	wc := DefaultWorldConfig()
	wc.Plot.Size = 32
	wc.Road.Width = 5
	wc.Flags = map[string]any{"pvp": true, "greeting": "Hi"}
	wf.Worlds["plotworld"] = wc
	if err := SaveWorlds(dir, wf); err != nil {
		t.Fatalf("SaveWorlds: %v", err)
	}
	wf2, err := LoadWorlds(dir)
	if err != nil {
		t.Fatalf("reload Worlds: %v", err)
	}
	area := wf2.Worlds["plotworld"].ToPlotArea("plotworld", "", nil, nil)
	if area.PlotWidth != 32 || area.RoadWidth != 5 || area.Size != 37 {
		t.Errorf("unexpected area geometry: %+v", area)
	}
	if v, _ := area.Flags.GetValue("pvp").(bool); !v {
		t.Errorf("expected area flag pvp=true")
	}

	id, minID, maxID, ok := ParseAreaKey("town--2;-3-4;5")
	if !ok || id != "town" || minID != (plotsquared.PlotId{X: -2, Y: -3}) || maxID != (plotsquared.PlotId{X: 4, Y: 5}) {
		t.Errorf("ParseAreaKey failed: %q %v %v %v", id, minID, maxID, ok)
	}

	if err := CopyTemplates(dir, "templates"); err != nil {
		t.Fatalf("CopyTemplates: %v", err)
	}
	for _, name := range []string{"skyblock.template", "town.template", "bridge.template"} {
		if _, err := os.Stat(filepath.Join(dir, "templates", name)); err != nil {
			t.Errorf("missing template %s: %v", name, err)
		}
	}
}
