// Package backup manages automatic and manual per-plot schematic backups
// under plugins/plots/backups/<owner>/<world>_<x>_<y>/.
package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/config"
	"github.com/golem-mc/plots/internal/generator"
	westorage "github.com/golem-mc/worldedit/storage"
)

// Entry describes one saved backup file for a plot.
type Entry struct {
	Path      string
	Name      string
	CreatedAt time.Time
}

// Manager stores and restores per-plot schematic backups.
type Manager struct {
	dataDir  string
	settings *config.BackupSettings
}

// NewManager creates a backup manager rooted at dataDir.
func NewManager(dataDir string, settings *config.BackupSettings) *Manager {
	return &Manager{dataDir: dataDir, settings: settings}
}

func (m *Manager) plotDir(plot *plots.Plot) string {
	base := plot.BasePlot()
	owner := base.Owner.String()
	plotKey := fmt.Sprintf("%s_%d_%d", base.Area.WorldName, base.ID.X, base.ID.Y)
	return filepath.Join(m.dataDir, "backups", owner, plotKey)
}

// Save saves a schematic backup of plot and enforces BackupLimit pruning.
func (m *Manager) Save(w plugin.World, area *plots.PlotArea, plot *plots.Plot) (Entry, error) {
	if w == nil || area == nil || plot == nil || !plot.HasOwner() {
		return Entry{}, fmt.Errorf("plot cannot be backed up")
	}
	if plot.IsMerged() {
		return Entry{}, fmt.Errorf("merged plots cannot be backed up")
	}
	dir := m.plotDir(plot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Entry{}, err
	}
	now := time.Now()
	name := fmt.Sprintf("%d.schem", now.UnixMilli())
	path := filepath.Join(dir, name)
	if err := generator.SavePlotSchematic(w, area, plot, path); err != nil {
		return Entry{}, err
	}
	m.prune(dir)
	return Entry{Path: path, Name: name, CreatedAt: now}, nil
}

// List returns saved backups for plot sorted newest-first.
func (m *Manager) List(plot *plots.Plot) ([]Entry, error) {
	if plot == nil || !plot.HasOwner() {
		return nil, nil
	}
	dir := m.plotDir(plot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Entry, 0, len(entries))
	for _, de := range entries {
		if de.IsDir() || (!strings.HasSuffix(de.Name(), ".schem") && !strings.HasSuffix(de.Name(), ".schematic")) {
			continue
		}
		full := filepath.Join(dir, de.Name())
		ts := parseBackupTime(de.Name(), full)
		out = append(out, Entry{Path: full, Name: de.Name(), CreatedAt: ts})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// Restore loads backup index (1-based, newest-first) and pastes it into plot.
func (m *Manager) Restore(w plugin.World, area *plots.PlotArea, plot *plots.Plot, index int, pasteOnTop bool) error {
	list, err := m.List(plot)
	if err != nil {
		return err
	}
	if index < 1 || index > len(list) {
		return fmt.Errorf("invalid backup number %d (have %d)", index, len(list))
	}
	cb, err := westorage.ReadSchematicFile(list[index-1].Path, "")
	if err != nil {
		return err
	}
	generator.PastePlotSchematic(w, area, plot, cb, pasteOnTop)
	return nil
}

// DeleteAll removes all saved backups for plot.
func (m *Manager) DeleteAll(plot *plots.Plot) error {
	if plot == nil || !plot.HasOwner() {
		return nil
	}
	return os.RemoveAll(m.plotDir(plot))
}

func (m *Manager) prune(dir string) {
	limit := 3
	if m.settings != nil && m.settings.BackupLimit > 0 {
		limit = m.settings.BackupLimit
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= limit {
		return
	}
	var list []Entry
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		full := filepath.Join(dir, de.Name())
		list = append(list, Entry{Path: full, Name: de.Name(), CreatedAt: parseBackupTime(de.Name(), full)})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	for i := limit; i < len(list); i++ {
		_ = os.Remove(list[i].Path)
	}
}

func parseBackupTime(name, path string) time.Time {
	stem := strings.TrimSuffix(strings.TrimSuffix(name, ".schem"), ".schematic")
	if ms, err := strconv.ParseInt(stem, 10, 64); err == nil && ms > 0 {
		return time.UnixMilli(ms)
	}
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}
