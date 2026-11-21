package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/golem-mc/plotsquared"
	"github.com/google/uuid"
)

func TestSQLiteCRUDAndUpstreamSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "storage.db")
	db, err := Open(dbPath, "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	area := plotsquared.DefaultPlotArea("plotworld", "")
	owner := uuid.New()
	trusted := uuid.New()
	member := uuid.New()
	denied := plotsquared.EveryoneUUID

	p := plotsquared.NewPlot(area, plotsquared.PlotId{X: 1, Y: -2}, owner)
	p.Settings.Alias = "spawn"
	p.Settings.Merged = [4]bool{true, false, true, false}
	p.Settings.Position = plotsquared.BlockLoc{X: 5, Y: 65, Z: 5, Set: true}

	if err := db.CreatePlot(p); err != nil {
		t.Fatalf("CreatePlot: %v", err)
	}
	if p.DBID <= 0 {
		t.Fatalf("expected DBID > 0, got %d", p.DBID)
	}
	if err := db.SetTrusted(p, trusted, true); err != nil {
		t.Fatalf("SetTrusted: %v", err)
	}
	if err := db.SetMember(p, member, true); err != nil {
		t.Fatalf("SetMember: %v", err)
	}
	if err := db.SetDenied(p, denied, true); err != nil {
		t.Fatalf("SetDenied: %v", err)
	}
	if err := db.SetFlag(p, "pvp", "true"); err != nil {
		t.Fatalf("SetFlag: %v", err)
	}
	if err := db.SetFlag(p, "greeting", "Welcome!"); err != nil {
		t.Fatalf("SetFlag greeting: %v", err)
	}
	if err := db.SetRating(p, member, 9); err != nil {
		t.Fatalf("SetRating: %v", err)
	}
	if err := db.AddComment(plotsquared.PlotComment{
		World: area.FullID(), ID: p.ID, Comment: "Nice!", Inbox: "public", Timestamp: 12345, Sender: "Alice",
	}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	// Create cluster
	c := &plotsquared.PlotCluster{
		Area:    area,
		Pos1:    plotsquared.PlotId{X: 5, Y: 5},
		Pos2:    plotsquared.PlotId{X: 7, Y: 7},
		Owner:   owner,
		Name:    "market",
		Helpers: make(map[uuid.UUID]struct{}),
		Invited: make(map[uuid.UUID]struct{}),
	}
	if err := db.CreateCluster(c); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	if err := db.SetClusterUser(c, "cluster_helpers", trusted, true); err != nil {
		t.Fatalf("SetClusterUser: %v", err)
	}
	if err := db.SetPlayerMeta(owner, "grantedPlots", []byte{0, 0, 0, 3}); err != nil {
		t.Fatalf("SetPlayerMeta: %v", err)
	}
	_ = db.Close()

	// Verify upstream table layout directly via raw SQL:
	// trusted -> plot_helpers, member -> plot_trusted!
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("raw sql.Open: %v", err)
	}
	var helperUUID, trustedUUID string
	if err := raw.QueryRow(`SELECT user_uuid FROM plot_helpers WHERE plot_plot_id = ?`, p.DBID).Scan(&helperUUID); err != nil {
		t.Fatalf("query plot_helpers: %v", err)
	}
	if helperUUID != trusted.String() {
		t.Errorf("plot_helpers user_uuid = %s, want trusted %s", helperUUID, trusted)
	}
	if err := raw.QueryRow(`SELECT user_uuid FROM plot_trusted WHERE plot_plot_id = ?`, p.DBID).Scan(&trustedUUID); err != nil {
		t.Fatalf("query plot_trusted: %v", err)
	}
	if trustedUUID != member.String() {
		t.Errorf("plot_trusted user_uuid = %s, want member %s", trustedUUID, member)
	}
	_ = raw.Close()

	// Reopen and verify LoadPlots / LoadClusters / GetPlayerMeta
	db2, err := Open(dbPath, "")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = db2.Close() }()

	plots, err := db2.LoadPlots()
	if err != nil {
		t.Fatalf("LoadPlots: %v", err)
	}
	loaded := plots["plotworld"][plotsquared.PlotId{X: 1, Y: -2}]
	if loaded == nil {
		t.Fatalf("expected plot (1;-2) loaded")
	}
	if loaded.Owner != owner || loaded.Settings.Alias != "spawn" {
		t.Errorf("owner/alias mismatch: %+v", loaded)
	}
	if loaded.Settings.Merged != ([4]bool{true, false, true, false}) {
		t.Errorf("merged = %v", loaded.Settings.Merged)
	}
	if _, ok := loaded.Trusted[trusted]; !ok {
		t.Errorf("trusted missing after reload")
	}
	if _, ok := loaded.Members[member]; !ok {
		t.Errorf("member missing after reload")
	}
	if _, ok := loaded.Denied[plotsquared.EveryoneUUID]; !ok {
		t.Errorf("denied everyone missing after reload")
	}
	if !loaded.FlagBool("pvp") || loaded.FlagString("greeting") != "Welcome!" {
		t.Errorf("flags mismatch: pvp=%v greeting=%q", loaded.FlagBool("pvp"), loaded.FlagString("greeting"))
	}
	if loaded.Settings.Ratings[member] != 9 {
		t.Errorf("rating = %v, want 9", loaded.Settings.Ratings)
	}
	if len(loaded.Settings.Comments) != 1 || loaded.Settings.Comments[0].Comment != "Nice!" {
		t.Errorf("comments = %+v", loaded.Settings.Comments)
	}

	clusters, err := db2.LoadClusters()
	if err != nil {
		t.Fatalf("LoadClusters: %v", err)
	}
	if len(clusters["plotworld"]) != 1 || clusters["plotworld"][0].Name != "market" {
		t.Errorf("clusters = %+v", clusters)
	}
	meta, err := db2.GetPlayerMeta(owner)
	if err != nil || len(meta["grantedPlots"]) != 4 {
		t.Errorf("meta = %v err=%v", meta, err)
	}

	if err := db2.DeletePlot(loaded); err != nil {
		t.Fatalf("DeletePlot: %v", err)
	}
	plotsAfter, err := db2.LoadPlots()
	if err != nil {
		t.Fatalf("LoadPlots after delete: %v", err)
	}
	if len(plotsAfter["plotworld"]) != 0 {
		t.Errorf("expected 0 plots after delete, got %d", len(plotsAfter["plotworld"]))
	}
}
