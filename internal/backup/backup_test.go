package backup

import (
	"testing"
	"time"

	"github.com/golem-mc/golem/plugin"
	"github.com/golem-mc/golem/plugintest"
	"github.com/golem-mc/golem/registry"
	"github.com/golem-mc/plots"
	"github.com/golem-mc/plots/internal/config"
	"github.com/golem-mc/plots/internal/generator"
	"github.com/google/uuid"
)

func TestBackupSaveListRestorePruneAndDelete(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := plugintest.Start(t)
	host := srv.Host()

	area := plots.DefaultPlotArea("backupworld", "")
	area.PlotWidth = 6
	area.RoadWidth = 5
	area.PlotHeight = 10
	area.RoadHeight = 10
	area.WallHeight = 10
	area.MinGenHeight = 0
	area.MaxGenHeight = 24
	area.MinBuildHeight = 1
	area.MaxBuildHeight = 24
	area.ComputeGeometry()

	gen := generator.NewHybridGenerator(area, "", nil)
	w, err := host.CreateWorld(plugin.WorldOptions{
		Key:       plugin.MinecraftKey("backupworld"),
		Dimension: plugin.MinecraftKey("overworld"),
		Generator: gen,
	})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}
	w.AddTicket(plugin.ChunkPos{X: 0, Z: 0}, 2)
	loaded := 0
	w.LoadChunk(plugin.ChunkPos{X: 0, Z: 0}, true, func(plugin.Chunk) { loaded++ })
	srv.RunTicks(5)
	if loaded != 1 {
		t.Fatalf("expected chunk (0,0) loaded")
	}

	owner := uuid.New()
	p := plots.NewPlot(area, plots.PlotId{X: 1, Y: 1}, owner)
	area.SetPlot(p)
	b := area.PlotInteriorBounds(p.ID)
	marker := plugin.BlockPos{X: b.Min.X + 2, Y: 11, Z: b.Min.Z + 2}

	dataDir := t.TempDir()
	mgr := NewManager(dataDir, &config.BackupSettings{
		AutomaticBackups: true,
		BackupLimit:      2,
		DeleteOnUnclaim:  true,
	})

	// Empty list before any backups
	if list, err := mgr.List(p); err != nil || len(list) != 0 {
		t.Fatalf("initial List = (%v, %v), want empty", list, err)
	}

	// Error cases: unowned plot and merged plot cannot be backed up
	unowned := plots.NewPlot(area, plots.PlotId{X: 2, Y: 1}, uuid.Nil)
	if _, err := mgr.Save(w, area, unowned); err == nil {
		t.Errorf("expected error backing up unowned plot")
	}
	p.Settings.Merged[plots.DirEast] = true
	if _, err := mgr.Save(w, area, p); err == nil {
		t.Errorf("expected error backing up merged plot")
	}
	p.Settings.Merged[plots.DirEast] = false

	// Save 3 distinct backups spaced by >=5ms so timestamps/filenames are strictly ordered.
	// BackupLimit=2 should prune the oldest (gold), leaving #1=emerald (newest) and #2=diamond.
	gold := plugin.DefaultState(registry.BlockGoldBlock)
	diamond := plugin.DefaultState(registry.BlockDiamondBlock)
	emerald := plugin.DefaultState(registry.BlockEmeraldBlock)
	air := plugin.DefaultState(registry.BlockAir)

	w.SetBlockState(marker, gold, plugin.UpdateClients)
	if _, err := mgr.Save(w, area, p); err != nil {
		t.Fatalf("Save #1 (gold): %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	w.SetBlockState(marker, diamond, plugin.UpdateClients)
	if _, err := mgr.Save(w, area, p); err != nil {
		t.Fatalf("Save #2 (diamond): %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	w.SetBlockState(marker, emerald, plugin.UpdateClients)
	if _, err := mgr.Save(w, area, p); err != nil {
		t.Fatalf("Save #3 (emerald): %v", err)
	}

	list, err := mgr.List(p)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected BackupLimit=2 backups after pruning, got %d (%+v)", len(list), list)
	}
	if !list[0].CreatedAt.After(list[1].CreatedAt) {
		t.Errorf("expected newest-first order: %v vs %v", list[0].CreatedAt, list[1].CreatedAt)
	}

	// Clear marker and restore backup #2 (diamond), then #1 (emerald)
	w.SetBlockState(marker, air, plugin.UpdateClients)
	if err := mgr.Restore(w, area, p, 2, false); err != nil {
		t.Fatalf("Restore #2: %v", err)
	}
	if got := w.BlockState(marker); got != diamond {
		t.Errorf("after Restore #2 got %v, want diamond_block", got)
	}

	if err := mgr.Restore(w, area, p, 1, false); err != nil {
		t.Fatalf("Restore #1: %v", err)
	}
	if got := w.BlockState(marker); got != emerald {
		t.Errorf("after Restore #1 got %v, want emerald_block", got)
	}

	// Out-of-range index returns error
	if err := mgr.Restore(w, area, p, 0, false); err == nil {
		t.Errorf("expected error for index 0")
	}
	if err := mgr.Restore(w, area, p, 3, false); err == nil {
		t.Errorf("expected error for index 3")
	}

	// DeleteAll clears all saved backups
	if err := mgr.DeleteAll(p); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}
	if list, err := mgr.List(p); err != nil || len(list) != 0 {
		t.Errorf("after DeleteAll List = (%v, %v), want empty", list, err)
	}
}
