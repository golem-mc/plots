package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/golem-mc/plots"
	"github.com/google/uuid"
)

func TestSQLiteCRUDAndUpstreamSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "storage.db")
	db, err := Open(dbPath, "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	area := plots.DefaultPlotArea("plotworld", "")
	owner := uuid.New()
	trusted := uuid.New()
	member := uuid.New()
	denied := plots.EveryoneUUID

	p := plots.NewPlot(area, plots.PlotId{X: 1, Y: -2}, owner)
	p.Settings.Alias = "spawn"
	p.Settings.Merged = [4]bool{true, false, true, false}
	p.Settings.Position = plots.BlockLoc{X: 5, Y: 65, Z: 5, Set: true}

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
	if err := db.AddComment(plots.PlotComment{
		World: area.FullID(), ID: p.ID, Comment: "Nice!", Inbox: "public", Timestamp: 12345, Sender: "Alice",
	}); err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	// Create cluster
	c := &plots.PlotCluster{
		Area:    area,
		Pos1:    plots.PlotId{X: 5, Y: 5},
		Pos2:    plots.PlotId{X: 7, Y: 7},
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

	loadedPlots, err := db2.LoadPlots()
	if err != nil {
		t.Fatalf("LoadPlots: %v", err)
	}
	loaded := loadedPlots["plotworld"][plots.PlotId{X: 1, Y: -2}]
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
	if _, ok := loaded.Denied[plots.EveryoneUUID]; !ok {
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

// upstreamSQLiteDDL returns upstream PlotSquared's exact SQLite CREATE TABLE statements
// copied from PlotSquared/Core/src/main/java/com/plotsquared/core/database/SQLManager.java:1291-1348.
func upstreamSQLiteDDL(prefix string) []string {
	return []string{
		"CREATE TABLE IF NOT EXISTS `" + prefix + "plot` (" +
			"`id` INTEGER PRIMARY KEY AUTOINCREMENT, `plot_id_x` INT(11) NOT NULL," +
			"`plot_id_z` INT(11) NOT NULL, `owner` VARCHAR(45) NOT NULL," +
			"`world` VARCHAR(45) NOT NULL," +
			"`timestamp` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP)",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "plot_denied` (`plot_plot_id` INT(11) NOT NULL," +
			"`user_uuid` VARCHAR(40) NOT NULL)",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "plot_helpers` (`plot_plot_id` INT(11) NOT NULL," +
			"`user_uuid` VARCHAR(40) NOT NULL)",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "plot_trusted` (`plot_plot_id` INT(11) NOT NULL," +
			"`user_uuid` VARCHAR(40) NOT NULL)",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "plot_comments` (" +
			"`world` VARCHAR(40) NOT NULL, `hashcode` INT(11) NOT NULL," +
			"`comment` VARCHAR(40) NOT NULL," +
			"`inbox` VARCHAR(40) NOT NULL, `timestamp` INT(11) NOT NULL," +
			"`sender` VARCHAR(40) NOT NULL)",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "plot_settings` (" +
			"  `plot_plot_id` INT(11) NOT NULL," +
			"  `biome` VARCHAR(45) DEFAULT 'FOREST', `rain` INT(1) DEFAULT 0," +
			"  `custom_time` TINYINT(1) DEFAULT '0', `time` INT(11) DEFAULT '8000'," +
			"  `deny_entry` TINYINT(1) DEFAULT '0'," +
			"  `alias` VARCHAR(50) DEFAULT NULL, `merged` INT(11) DEFAULT NULL," +
			"  `position` VARCHAR(50) NOT NULL DEFAULT 'DEFAULT'," +
			"  PRIMARY KEY (`plot_plot_id`))",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "plot_rating` (`plot_plot_id` INT(11) NOT NULL, `rating` INT(2) NOT NULL, `player` VARCHAR(40) NOT NULL)",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "cluster` (" +
			"`id` INTEGER PRIMARY KEY AUTOINCREMENT, `pos1_x` INT(11) NOT NULL," +
			"`pos1_z` INT(11) NOT NULL, `pos2_x` INT(11) NOT NULL," +
			"`pos2_z` INT(11) NOT NULL, `owner` VARCHAR(40) NOT NULL," +
			"`world` VARCHAR(45) NOT NULL," +
			"`timestamp` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP)",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "cluster_helpers` (`cluster_id` INT(11) NOT NULL," +
			"`user_uuid` VARCHAR(40) NOT NULL)",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "cluster_invited` (`cluster_id` INT(11) NOT NULL," +
			"`user_uuid` VARCHAR(40) NOT NULL)",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "cluster_settings` (" +
			"  `cluster_id` INT(11) NOT NULL, `biome` VARCHAR(45) DEFAULT 'FOREST'," +
			"  `rain` INT(1) DEFAULT 0, `custom_time` TINYINT(1) DEFAULT '0'," +
			"  `time` INT(11) DEFAULT '8000', `deny_entry` TINYINT(1) DEFAULT '0'," +
			"  `alias` VARCHAR(50) DEFAULT NULL, `merged` INT(11) DEFAULT NULL," +
			"  `position` VARCHAR(50) NOT NULL DEFAULT 'DEFAULT'," +
			"  PRIMARY KEY (`cluster_id`))",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "player_meta` (" +
			" `meta_id` INTEGER PRIMARY KEY AUTOINCREMENT," +
			" `uuid` VARCHAR(40) NOT NULL, `key` VARCHAR(32) NOT NULL," +
			" `value` blob NOT NULL)",
		"CREATE TABLE IF NOT EXISTS `" + prefix + "plot_flags`(" +
			"`id` INTEGER PRIMARY KEY AUTOINCREMENT, `plot_id` INTEGER NOT NULL," +
			" `flag` VARCHAR(64), `value` VARCHAR(512)," +
			"FOREIGN KEY (plot_id) REFERENCES `" + prefix + "plot` (id) ON DELETE CASCADE, UNIQUE (plot_id, flag))",
	}
}

func TestLoadUpstreamPlotSquaredSQLiteDB(t *testing.T) {
	for _, prefix := range []string{"", "ps_"} {
		name := "no_prefix"
		if prefix != "" {
			name = "prefixed_" + prefix
		}
		t.Run(name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "storage.db")
			raw, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatalf("sql.Open: %v", err)
			}
			for _, ddl := range upstreamSQLiteDDL(prefix) {
				if _, err := raw.Exec(ddl); err != nil {
					t.Fatalf("exec upstream DDL: %v\nSQL: %s", err, ddl)
				}
			}

			alice := uuid.MustParse("11111111-2222-3333-4444-555555555555")
			bob := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
			charlie := uuid.MustParse("99999999-8888-7777-6666-555555555555")

			// Insert sample rows as upstream PlotSquared writes them:
			// Plot #1: (1;1) in "plots", owned by Alice, alias="central", custom home, trusted=Bob (plot_helpers),
			//          member=Charlie (plot_trusted), denied="1-1-3-3-7" (DBFunc.EVERYONE short UUID),
			//          flags (pvp=true, greeting, price=250.5, break=stone,dirt), rating from Bob, comment with PlotId.hashCode().
			// Plot #2 & #3: merged pair (2;1) [merged=4=East] and (3;1) [merged=1=West] in "plots".
			// Plot #4: partial-area plot (-3;4) in "world;spawn".
			p := prefix
			exec := func(q string, args ...any) {
				t.Helper()
				if _, err := raw.Exec(q, args...); err != nil {
					t.Fatalf("exec %q: %v", q, err)
				}
			}
			exec("INSERT INTO `"+p+"plot` (id, plot_id_x, plot_id_z, owner, world, timestamp) VALUES (1, 1, 1, ?, 'plots', '2025-02-10 12:30:00')", alice.String())
			exec("INSERT INTO `"+p+"plot` (id, plot_id_x, plot_id_z, owner, world, timestamp) VALUES (2, 2, 1, ?, 'plots', '2025-02-11 08:00:00')", alice.String())
			exec("INSERT INTO `"+p+"plot` (id, plot_id_x, plot_id_z, owner, world, timestamp) VALUES (3, 3, 1, ?, 'plots', '2025-02-11 08:00:00')", alice.String())
			exec("INSERT INTO `"+p+"plot` (id, plot_id_x, plot_id_z, owner, world, timestamp) VALUES (4, -3, 4, ?, 'world;spawn', '2025-02-12 09:15:00')", bob.String())

			// Upstream plot_settings: merged bitmask N=8, E=4, S=2, W=1 (HashUtil.hash)
			exec("INSERT INTO `" + p + "plot_settings` (plot_plot_id, alias, merged, position) VALUES (1, 'central', 0, '10,65,12,90.5,-15.0')")
			exec("INSERT INTO `" + p + "plot_settings` (plot_plot_id, alias, merged, position) VALUES (2, NULL, 4, 'DEFAULT')")
			exec("INSERT INTO `" + p + "plot_settings` (plot_plot_id, alias, merged, position) VALUES (3, NULL, 1, 'DEFAULT')")

			// Upstream plot_helpers = Trusted; plot_trusted = Members; plot_denied accepts short "1-1-3-3-7"
			exec("INSERT INTO `"+p+"plot_helpers` (plot_plot_id, user_uuid) VALUES (1, ?)", bob.String())
			exec("INSERT INTO `"+p+"plot_trusted` (plot_plot_id, user_uuid) VALUES (1, ?)", charlie.String())
			exec("INSERT INTO `" + p + "plot_denied` (plot_plot_id, user_uuid) VALUES (1, '1-1-3-3-7')")

			// Upstream plot_flags
			exec("INSERT INTO `" + p + "plot_flags` (plot_id, flag, value) VALUES (1, 'pvp', 'true')")
			exec("INSERT INTO `" + p + "plot_flags` (plot_id, flag, value) VALUES (1, 'greeting', 'Welcome to central!')")
			exec("INSERT INTO `" + p + "plot_flags` (plot_id, flag, value) VALUES (1, 'price', '250.5')")
			exec("INSERT INTO `" + p + "plot_flags` (plot_id, flag, value) VALUES (1, 'break', 'stone,dirt')")

			// Upstream plot_rating
			exec("INSERT INTO `"+p+"plot_rating` (plot_plot_id, rating, player) VALUES (1, 9, ?)", bob.String())

			// Upstream plot_comments: hashcode = (x << 16) | (y & 0xFFFF) (PlotId.java:47)
			hash11 := int32((1 << 16) | (1 & 0xFFFF))
			hashNeg34 := int32((-3 << 16) | (4 & 0xFFFF))
			exec("INSERT INTO `"+p+"plot_comments` (world, hashcode, comment, inbox, timestamp, sender) VALUES ('plots', ?, 'Awesome!', 'public', 1739200000, 'Bob')", hash11)
			exec("INSERT INTO `"+p+"plot_comments` (world, hashcode, comment, inbox, timestamp, sender) VALUES ('world;spawn', ?, 'Spawn note', 'owner', 1739200100, 'Alice')", hashNeg34)

			// Upstream cluster + helpers + invited + settings
			exec("INSERT INTO `"+p+"cluster` (id, pos1_x, pos1_z, pos2_x, pos2_z, owner, world) VALUES (1, 10, 10, 12, 12, ?, 'plots')", alice.String())
			exec("INSERT INTO `"+p+"cluster_helpers` (cluster_id, user_uuid) VALUES (1, ?)", bob.String())
			exec("INSERT INTO `"+p+"cluster_invited` (cluster_id, user_uuid) VALUES (1, ?)", charlie.String())
			exec("INSERT INTO `" + p + "cluster_settings` (cluster_id, alias, position) VALUES (1, 'plaza', '5,63,5')")

			// Upstream player_meta
			exec("INSERT INTO `"+p+"player_meta` (uuid, key, value) VALUES (?, 'grantedPlots', ?)", alice.String(), []byte{0, 0, 0, 5})

			if err := raw.Close(); err != nil {
				t.Fatalf("close raw db: %v", err)
			}

			// Open via our storage package and verify every single entity & field
			db, err := Open(dbPath, prefix)
			if err != nil {
				t.Fatalf("storage.Open: %v", err)
			}
			defer func() { _ = db.Close() }()

			loaded, err := db.LoadPlots()
			if err != nil {
				t.Fatalf("LoadPlots: %v", err)
			}

			p1 := loaded["plots"][plots.PlotId{X: 1, Y: 1}]
			if p1 == nil {
				t.Fatalf("expected plot (1;1) in 'plots', got keys: %v", loaded["plots"])
			}
			if p1.Owner != alice {
				t.Errorf("p1.Owner = %s, want %s", p1.Owner, alice)
			}
			if p1.Settings.Alias != "central" {
				t.Errorf("p1.Alias = %q, want central", p1.Settings.Alias)
			}
			if !p1.Settings.Position.Set || p1.Settings.Position.X != 10 || p1.Settings.Position.Y != 65 || p1.Settings.Position.Z != 12 {
				t.Errorf("p1.Position = %+v, want (10,65,12)", p1.Settings.Position)
			}
			if !p1.IsTrusted(bob) || p1.IsMember(bob) {
				t.Errorf("expected Bob trusted (from plot_helpers) and not member")
			}
			if !p1.IsMember(charlie) || p1.IsTrusted(charlie) {
				t.Errorf("expected Charlie member (from plot_trusted) and not trusted")
			}
			if !p1.IsDenied(plots.EveryoneUUID) {
				t.Errorf("expected EveryoneUUID denied from upstream short UUID '1-1-3-3-7'")
			}
			if !p1.FlagBool("pvp") || p1.FlagString("greeting") != "Welcome to central!" {
				t.Errorf("flags mismatch: pvp=%v greeting=%q", p1.FlagBool("pvp"), p1.FlagString("greeting"))
			}
			if price, _ := p1.FlagValue("price").(float64); price != 250.5 {
				t.Errorf("price flag = %v, want 250.5", price)
			}
			if brk, _ := p1.FlagValue("break").([]string); len(brk) != 2 {
				t.Errorf("break flag = %v, want 2 blocks", brk)
			}
			if p1.Settings.Ratings[bob] != 9 {
				t.Errorf("ratings = %v, want Bob=9", p1.Settings.Ratings)
			}
			if len(p1.Settings.Comments) != 1 || p1.Settings.Comments[0].Comment != "Awesome!" || p1.Settings.Comments[0].Sender != "Bob" {
				t.Errorf("p1 comments = %+v", p1.Settings.Comments)
			}

			// Merged plots (2;1) and (3;1)
			p2 := loaded["plots"][plots.PlotId{X: 2, Y: 1}]
			p3 := loaded["plots"][plots.PlotId{X: 3, Y: 1}]
			if p2 == nil || p3 == nil {
				t.Fatalf("expected merged plots (2;1) and (3;1) loaded")
			}
			if !p2.IsMergedDir(plots.DirEast) || p2.IsMergedDir(plots.DirWest) {
				t.Errorf("p2 merged = %v, want East only", p2.Settings.Merged)
			}
			if !p3.IsMergedDir(plots.DirWest) || p3.IsMergedDir(plots.DirEast) {
				t.Errorf("p3 merged = %v, want West only", p3.Settings.Merged)
			}

			// Partial area plot (-3;4) in "world;spawn"
			p4 := loaded["world;spawn"][plots.PlotId{X: -3, Y: 4}]
			if p4 == nil || p4.Owner != bob {
				t.Fatalf("expected partial area plot (-3;4) owned by Bob, got %+v", p4)
			}
			if len(p4.Settings.Comments) != 1 || p4.Settings.Comments[0].Comment != "Spawn note" {
				t.Errorf("p4 comments = %+v", p4.Settings.Comments)
			}

			// Cluster
			clusters, err := db.LoadClusters()
			if err != nil {
				t.Fatalf("LoadClusters: %v", err)
			}
			if len(clusters["plots"]) != 1 {
				t.Fatalf("clusters[plots] = %+v, want 1", clusters["plots"])
			}
			cl := clusters["plots"][0]
			if cl.Name != "plaza" || cl.Owner != alice || cl.Pos1 != (plots.PlotId{X: 10, Y: 10}) || cl.Pos2 != (plots.PlotId{X: 12, Y: 12}) {
				t.Errorf("cluster mismatch: %+v", cl)
			}
			if _, ok := cl.Helpers[bob]; !ok {
				t.Errorf("expected Bob in cluster helpers")
			}
			if _, ok := cl.Invited[charlie]; !ok {
				t.Errorf("expected Charlie in cluster invited")
			}
			if !cl.Settings.Position.Set || cl.Settings.Position.X != 5 || cl.Settings.Position.Y != 63 || cl.Settings.Position.Z != 5 {
				t.Errorf("cluster position = %+v, want (5,63,5)", cl.Settings.Position)
			}

			// Player meta
			meta, err := db.GetPlayerMeta(alice)
			if err != nil {
				t.Fatalf("GetPlayerMeta: %v", err)
			}
			if got := meta["grantedPlots"]; len(got) != 4 || got[3] != 5 {
				t.Errorf("grantedPlots = %v, want [0 0 0 5]", got)
			}
		})
	}
}
