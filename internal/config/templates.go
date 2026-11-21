package config

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed templates/*.template
var embeddedTemplates embed.FS

// CopyTemplates extracts upstream's bundled templates (skyblock.template,
// town.template, bridge.template) into <dataDir>/<templatesDir> if missing.
func CopyTemplates(dataDir, templatesDir string) error {
	if templatesDir == "" {
		templatesDir = "templates"
	}
	outDir := filepath.Join(dataDir, templatesDir)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	entries, err := fs.ReadDir(embeddedTemplates, "templates")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		dst := filepath.Join(outDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := embeddedTemplates.ReadFile("templates/" + e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
