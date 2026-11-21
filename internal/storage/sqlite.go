// Package storage implements PlotSquared's SQLite database using upstream's
// exact table schema so an existing Bukkit/Paper storage.db loads seamlessly.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golem-mc/plots"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection with PlotSquared's table prefix.
type DB struct {
	mu     sync.Mutex
	sql    *sql.DB
	prefix string
}

// Open opens (or creates) the SQLite database at dbPath and ensures all 13
// upstream tables exist.
func Open(dbPath, prefix string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	// SQLite performs best with a single writer connection in-process.
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	db := &DB{sql: conn, prefix: prefix}
	if err := db.createTables(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return db, nil
}

// Close closes the underlying SQLite connection.
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sql == nil {
		return nil
	}
	err := d.sql.Close()
	d.sql = nil
	return err
}

func (d *DB) t(name string) string {
	return "`" + d.prefix + name + "`"
}

// createTables creates the exact 13 tables used by upstream SQLManager.java (SQLite branch).
func (d *DB) createTables() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ` + d.t("plot") + ` (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plot_id_x INT(11) NOT NULL,
			plot_id_z INT(11) NOT NULL,
			owner VARCHAR(45) NOT NULL,
			world VARCHAR(45) NOT NULL,
			timestamp timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("plot_denied") + ` (
			plot_plot_id INT(11) NOT NULL,
			user_uuid VARCHAR(40) NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("plot_helpers") + ` (
			plot_plot_id INT(11) NOT NULL,
			user_uuid VARCHAR(40) NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("plot_trusted") + ` (
			plot_plot_id INT(11) NOT NULL,
			user_uuid VARCHAR(40) NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("plot_comments") + ` (
			world VARCHAR(40) NOT NULL,
			hashcode INT(11) NOT NULL,
			comment VARCHAR(40) NOT NULL,
			inbox VARCHAR(40) NOT NULL,
			timestamp INT(11) NOT NULL,
			sender VARCHAR(40) NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("plot_settings") + ` (
			plot_plot_id INT(11) NOT NULL,
			biome VARCHAR(45) DEFAULT 'FOREST',
			rain INT(1) DEFAULT 0,
			custom_time TINYINT(1) DEFAULT '0',
			time INT(11) DEFAULT '8000',
			deny_entry TINYINT(1) DEFAULT '0',
			alias VARCHAR(50) DEFAULT NULL,
			merged INT(11) DEFAULT NULL,
			position VARCHAR(50) NOT NULL DEFAULT 'DEFAULT',
			PRIMARY KEY (plot_plot_id)
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("plot_rating") + ` (
			plot_plot_id INT(11) NOT NULL,
			rating INT(2) NOT NULL,
			player VARCHAR(40) NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("cluster") + ` (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pos1_x INT(11) NOT NULL,
			pos1_z INT(11) NOT NULL,
			pos2_x INT(11) NOT NULL,
			pos2_z INT(11) NOT NULL,
			owner VARCHAR(40) NOT NULL,
			world VARCHAR(45) NOT NULL,
			timestamp timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("cluster_helpers") + ` (
			cluster_id INT(11) NOT NULL,
			user_uuid VARCHAR(40) NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("cluster_invited") + ` (
			cluster_id INT(11) NOT NULL,
			user_uuid VARCHAR(40) NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("cluster_settings") + ` (
			cluster_id INT(11) NOT NULL,
			biome VARCHAR(45) DEFAULT 'FOREST',
			rain INT(1) DEFAULT 0,
			custom_time TINYINT(1) DEFAULT '0',
			time INT(11) DEFAULT '8000',
			deny_entry TINYINT(1) DEFAULT '0',
			alias VARCHAR(50) DEFAULT NULL,
			merged INT(11) DEFAULT NULL,
			position VARCHAR(50) NOT NULL DEFAULT 'DEFAULT',
			PRIMARY KEY (cluster_id)
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("player_meta") + ` (
			meta_id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid VARCHAR(40) NOT NULL,
			key VARCHAR(32) NOT NULL,
			value blob NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ` + d.t("plot_flags") + ` (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plot_id INTEGER NOT NULL,
			flag VARCHAR(64),
			value VARCHAR(512),
			FOREIGN KEY (plot_id) REFERENCES ` + d.t("plot") + ` (id) ON DELETE CASCADE,
			UNIQUE (plot_id, flag)
		)`,
	}
	for _, s := range stmts {
		if _, err := d.sql.Exec(s); err != nil {
			return fmt.Errorf("create table: %w", err)
		}
	}
	return nil
}

// LoadPlots loads all plots grouped by area FullID (`world` column in `plot`).
func (d *DB) LoadPlots() (map[string]map[plots.PlotId]*plots.Plot, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	byArea := make(map[string]map[plots.PlotId]*plots.Plot)
	byID := make(map[int64]*plots.Plot)

	rows, err := d.sql.Query(`SELECT id, plot_id_x, plot_id_z, owner, world, timestamp FROM ` + d.t("plot"))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var (
			id      int64
			px, pz  int
			ownerS  string
			areaID  string
			rawTime any
		)
		if err := rows.Scan(&id, &px, &pz, &ownerS, &areaID, &rawTime); err != nil {
			_ = rows.Close()
			return nil, err
		}
		owner := parseStoredUUID(ownerS)
		p := plots.NewPlot(nil, plots.PlotId{X: px, Y: pz}, owner)
		p.DBID = id
		p.Timestamp = parseTimestampMillis(rawTime)

		m := byArea[areaID]
		if m == nil {
			m = make(map[plots.PlotId]*plots.Plot)
			byArea[areaID] = m
		}
		m[p.ID] = p
		byID[id] = p
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// Load ratings
	if err := d.loadRatingsLocked(byID); err != nil {
		return nil, err
	}
	// Load trusted (stored in `plot_helpers` in upstream SQL schema!)
	if err := d.loadTierLocked(byID, "plot_helpers", func(p *plots.Plot, u uuid.UUID) {
		p.Trusted[u] = struct{}{}
	}); err != nil {
		return nil, err
	}
	// Load members (stored in `plot_trusted` in upstream SQL schema!)
	if err := d.loadTierLocked(byID, "plot_trusted", func(p *plots.Plot, u uuid.UUID) {
		p.Members[u] = struct{}{}
	}); err != nil {
		return nil, err
	}
	// Load denied
	if err := d.loadTierLocked(byID, "plot_denied", func(p *plots.Plot, u uuid.UUID) {
		p.Denied[u] = struct{}{}
	}); err != nil {
		return nil, err
	}
	// Load flags
	if err := d.loadFlagsLocked(byID); err != nil {
		return nil, err
	}
	// Load settings (alias, merged, position)
	if err := d.loadSettingsLocked(byID); err != nil {
		return nil, err
	}
	// Load comments
	if err := d.loadCommentsLocked(byArea); err != nil {
		return nil, err
	}

	return byArea, nil
}

func (d *DB) loadRatingsLocked(byID map[int64]*plots.Plot) error {
	rows, err := d.sql.Query(`SELECT plot_plot_id, player, rating FROM ` + d.t("plot_rating"))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			pid     int64
			playerS string
			rating  int
		)
		if err := rows.Scan(&pid, &playerS, &rating); err != nil {
			return err
		}
		if p := byID[pid]; p != nil {
			if p.Settings.Ratings == nil {
				p.Settings.Ratings = make(map[uuid.UUID]int)
			}
			p.Settings.Ratings[parseStoredUUID(playerS)] = rating
		}
	}
	return rows.Err()
}

func (d *DB) loadTierLocked(byID map[int64]*plots.Plot, table string, add func(*plots.Plot, uuid.UUID)) error {
	rows, err := d.sql.Query(`SELECT plot_plot_id, user_uuid FROM ` + d.t(table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			pid   int64
			userS string
		)
		if err := rows.Scan(&pid, &userS); err != nil {
			return err
		}
		if p := byID[pid]; p != nil {
			add(p, parseStoredUUID(userS))
		}
	}
	return rows.Err()
}

func (d *DB) loadFlagsLocked(byID map[int64]*plots.Plot) error {
	rows, err := d.sql.Query(`SELECT plot_id, flag, value FROM ` + d.t("plot_flags"))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			pid        int64
			flag, valS sql.NullString
		)
		if err := rows.Scan(&pid, &flag, &valS); err != nil {
			return err
		}
		if p := byID[pid]; p != nil && flag.Valid {
			_ = p.Flags.ParseAndSet(flag.String, valS.String)
		}
	}
	return rows.Err()
}

func (d *DB) loadSettingsLocked(byID map[int64]*plots.Plot) error {
	rows, err := d.sql.Query(`SELECT plot_plot_id, alias, merged, position FROM ` + d.t("plot_settings"))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			pid    int64
			alias  sql.NullString
			merged sql.NullInt64
			pos    sql.NullString
		)
		if err := rows.Scan(&pid, &alias, &merged, &pos); err != nil {
			return err
		}
		p := byID[pid]
		if p == nil {
			continue
		}
		if alias.Valid {
			p.Settings.Alias = alias.String
		}
		if merged.Valid {
			p.Settings.Merged = plots.UnhashMerged(int(merged.Int64))
		}
		if pos.Valid {
			if bl, err := plots.ParseBlockLoc(pos.String); err == nil {
				p.Settings.Position = bl
			}
		}
	}
	return rows.Err()
}

func (d *DB) loadCommentsLocked(byArea map[string]map[plots.PlotId]*plots.Plot) error {
	rows, err := d.sql.Query(`SELECT world, hashcode, comment, inbox, timestamp, sender FROM ` + d.t("plot_comments"))
	if err != nil {
		return err
	}
	defer rows.Close()
	// Build lookup by (world, hashcode)
	byHash := make(map[string]map[int32]*plots.Plot, len(byArea))
	for areaID, m := range byArea {
		hm := make(map[int32]*plots.Plot, len(m))
		for id, p := range m {
			hm[id.HashCode()] = p
		}
		byHash[areaID] = hm
	}
	for rows.Next() {
		var (
			world, comment, inbox, sender string
			hashcode                      int32
			ts                            int64
		)
		if err := rows.Scan(&world, &hashcode, &comment, &inbox, &ts, &sender); err != nil {
			return err
		}
		if hm := byHash[world]; hm != nil {
			if p := hm[hashcode]; p != nil {
				p.Settings.Comments = append(p.Settings.Comments, plots.PlotComment{
					World:     world,
					ID:        p.ID,
					Comment:   comment,
					Inbox:     inbox,
					Timestamp: ts,
					Sender:    sender,
				})
			}
		}
	}
	return rows.Err()
}

// CreatePlot inserts a plot and its settings row, setting p.DBID.
func (d *DB) CreatePlot(p *plots.Plot) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if p.Area == nil {
		return fmt.Errorf("plot has nil area")
	}
	if p.Timestamp == 0 {
		p.Timestamp = time.Now().UnixMilli()
	}
	ts := time.UnixMilli(p.Timestamp).UTC().Format("2006-01-02 15:04:05")
	res, err := d.sql.Exec(
		`INSERT INTO `+d.t("plot")+` (plot_id_x, plot_id_z, owner, world, timestamp) VALUES (?, ?, ?, ?, ?)`,
		p.ID.X, p.ID.Y, p.Owner.String(), p.Area.FullID(), ts,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	p.DBID = id

	var alias any
	if p.Settings.Alias != "" {
		alias = p.Settings.Alias
	}
	_, err = d.sql.Exec(
		`INSERT OR REPLACE INTO `+d.t("plot_settings")+` (plot_plot_id, alias, merged, position) VALUES (?, ?, ?, ?)`,
		id, alias, plots.HashMerged(p.Settings.Merged), p.Settings.Position.String(),
	)
	return err
}

// DeletePlot removes a plot and all its dependent rows.
func (d *DB) DeletePlot(p *plots.Plot) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	id, err := d.resolvePlotIDLocked(p)
	if err != nil || id == 0 {
		return err
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, tbl := range []string{"plot_settings", "plot_denied", "plot_helpers", "plot_trusted", "plot_rating"} {
		if _, err := tx.Exec(`DELETE FROM `+d.t(tbl)+` WHERE plot_plot_id = ?`, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM `+d.t("plot_flags")+` WHERE plot_id = ?`, id); err != nil {
		return err
	}
	if p.Area != nil {
		if _, err := tx.Exec(`DELETE FROM `+d.t("plot_comments")+` WHERE world = ? AND hashcode = ?`, p.Area.FullID(), p.ID.HashCode()); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM `+d.t("plot")+` WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetOwner updates a plot's owner.
func (d *DB) SetOwner(p *plots.Plot, owner uuid.UUID) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if p.Area == nil {
		return nil
	}
	_, err := d.sql.Exec(
		`UPDATE `+d.t("plot")+` SET owner = ? WHERE plot_id_x = ? AND plot_id_z = ? AND world = ?`,
		owner.String(), p.ID.X, p.ID.Y, p.Area.FullID(),
	)
	return err
}

// SetMerged persists the plot's merged edges bitmask.
func (d *DB) SetMerged(p *plots.Plot) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	id, err := d.resolvePlotIDLocked(p)
	if err != nil || id == 0 {
		return err
	}
	_, err = d.sql.Exec(
		`INSERT INTO `+d.t("plot_settings")+` (plot_plot_id, merged) VALUES (?, ?)
		 ON CONFLICT(plot_plot_id) DO UPDATE SET merged = excluded.merged`,
		id, plots.HashMerged(p.Settings.Merged),
	)
	return err
}

// SetAlias persists a plot's alias.
func (d *DB) SetAlias(p *plots.Plot, alias string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	id, err := d.resolvePlotIDLocked(p)
	if err != nil || id == 0 {
		return err
	}
	var val any
	if alias != "" {
		val = alias
	}
	_, err = d.sql.Exec(
		`INSERT INTO `+d.t("plot_settings")+` (plot_plot_id, alias) VALUES (?, ?)
		 ON CONFLICT(plot_plot_id) DO UPDATE SET alias = excluded.alias`,
		id, val,
	)
	return err
}

// SetPosition persists a plot's home position.
func (d *DB) SetPosition(p *plots.Plot, pos plots.BlockLoc) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	id, err := d.resolvePlotIDLocked(p)
	if err != nil || id == 0 {
		return err
	}
	_, err = d.sql.Exec(
		`INSERT INTO `+d.t("plot_settings")+` (plot_plot_id, position) VALUES (?, ?)
		 ON CONFLICT(plot_plot_id) DO UPDATE SET position = excluded.position`,
		id, pos.String(),
	)
	return err
}

// SetTrusted adds or removes a trusted player (upstream table `plot_helpers`).
func (d *DB) SetTrusted(p *plots.Plot, u uuid.UUID, add bool) error {
	return d.updateTier(p, "plot_helpers", u, add)
}

// SetMember adds or removes a member player (upstream table `plot_trusted`).
func (d *DB) SetMember(p *plots.Plot, u uuid.UUID, add bool) error {
	return d.updateTier(p, "plot_trusted", u, add)
}

// SetDenied adds or removes a denied player (upstream table `plot_denied`).
func (d *DB) SetDenied(p *plots.Plot, u uuid.UUID, add bool) error {
	return d.updateTier(p, "plot_denied", u, add)
}

func (d *DB) updateTier(p *plots.Plot, table string, u uuid.UUID, add bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	id, err := d.resolvePlotIDLocked(p)
	if err != nil || id == 0 {
		return err
	}
	if add {
		_, err = d.sql.Exec(`INSERT INTO `+d.t(table)+` (plot_plot_id, user_uuid) VALUES (?, ?)`, id, u.String())
		return err
	}
	_, err = d.sql.Exec(`DELETE FROM `+d.t(table)+` WHERE plot_plot_id = ? AND user_uuid = ?`, id, u.String())
	return err
}

// SetFlag upserts a flag value in `plot_flags`.
func (d *DB) SetFlag(p *plots.Plot, flagName, serialized string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	id, err := d.resolvePlotIDLocked(p)
	if err != nil || id == 0 {
		return err
	}
	_, err = d.sql.Exec(
		`INSERT INTO `+d.t("plot_flags")+` (plot_id, flag, value) VALUES (?, ?, ?)
		 ON CONFLICT(plot_id, flag) DO UPDATE SET value = excluded.value`,
		id, strings.ToLower(flagName), serialized,
	)
	return err
}

// RemoveFlag deletes a flag row from `plot_flags`.
func (d *DB) RemoveFlag(p *plots.Plot, flagName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	id, err := d.resolvePlotIDLocked(p)
	if err != nil || id == 0 {
		return err
	}
	_, err = d.sql.Exec(`DELETE FROM `+d.t("plot_flags")+` WHERE plot_id = ? AND flag = ?`, id, strings.ToLower(flagName))
	return err
}

// SetRating inserts or replaces a player's rating on a plot.
func (d *DB) SetRating(p *plots.Plot, rater uuid.UUID, rating int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	id, err := d.resolvePlotIDLocked(p)
	if err != nil || id == 0 {
		return err
	}
	if _, err := d.sql.Exec(`DELETE FROM `+d.t("plot_rating")+` WHERE plot_plot_id = ? AND player = ?`, id, rater.String()); err != nil {
		return err
	}
	_, err = d.sql.Exec(`INSERT INTO `+d.t("plot_rating")+` (plot_plot_id, rating, player) VALUES (?, ?, ?)`, id, rating, rater.String())
	return err
}

// DeleteRatings deletes all ratings for a plot.
func (d *DB) DeleteRatings(p *plots.Plot) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	id, err := d.resolvePlotIDLocked(p)
	if err != nil || id == 0 {
		return err
	}
	_, err = d.sql.Exec(`DELETE FROM `+d.t("plot_rating")+` WHERE plot_plot_id = ?`, id)
	return err
}

// AddComment inserts a plot comment.
func (d *DB) AddComment(c plots.PlotComment) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.sql.Exec(
		`INSERT INTO `+d.t("plot_comments")+` (world, hashcode, comment, inbox, timestamp, sender) VALUES (?, ?, ?, ?, ?, ?)`,
		c.World, c.ID.HashCode(), c.Comment, c.Inbox, c.Timestamp, c.Sender,
	)
	return err
}

// RemoveComment deletes a specific plot comment.
func (d *DB) RemoveComment(c plots.PlotComment) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.sql.Exec(
		`DELETE FROM `+d.t("plot_comments")+` WHERE world = ? AND hashcode = ? AND comment = ? AND inbox = ? AND sender = ?`,
		c.World, c.ID.HashCode(), c.Comment, c.Inbox, c.Sender,
	)
	return err
}

// ClearInbox removes all comments in a plot's inbox.
func (d *DB) ClearInbox(world string, id plots.PlotId, inbox string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.sql.Exec(
		`DELETE FROM `+d.t("plot_comments")+` WHERE world = ? AND hashcode = ? AND inbox = ?`,
		world, id.HashCode(), inbox,
	)
	return err
}

// MovePlot updates a plot's coordinates and world in the database.
func (d *DB) MovePlot(p *plots.Plot, newArea string, newID plots.PlotId) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	id, err := d.resolvePlotIDLocked(p)
	if err != nil || id == 0 {
		return err
	}
	_, err = d.sql.Exec(
		`UPDATE `+d.t("plot")+` SET plot_id_x = ?, plot_id_z = ?, world = ? WHERE id = ?`,
		newID.X, newID.Y, newArea, id,
	)
	return err
}

// LoadClusters loads all clusters grouped by area FullID.
func (d *DB) LoadClusters() (map[string][]*plots.PlotCluster, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	byArea := make(map[string][]*plots.PlotCluster)
	byID := make(map[int64]*plots.PlotCluster)

	rows, err := d.sql.Query(`SELECT id, pos1_x, pos1_z, pos2_x, pos2_z, owner, world FROM ` + d.t("cluster"))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var (
			id             int64
			x1, z1, x2, z2 int
			ownerS, areaID string
		)
		if err := rows.Scan(&id, &x1, &z1, &x2, &z2, &ownerS, &areaID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		c := &plots.PlotCluster{
			DBID:    id,
			Pos1:    plots.PlotId{X: x1, Y: z1},
			Pos2:    plots.PlotId{X: x2, Y: z2},
			Owner:   parseStoredUUID(ownerS),
			Helpers: make(map[uuid.UUID]struct{}),
			Invited: make(map[uuid.UUID]struct{}),
		}
		byID[id] = c
		byArea[areaID] = append(byArea[areaID], c)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// Helpers
	if err := d.loadClusterUsersLocked(byID, "cluster_helpers", func(c *plots.PlotCluster, u uuid.UUID) {
		c.Helpers[u] = struct{}{}
	}); err != nil {
		return nil, err
	}
	// Invited
	if err := d.loadClusterUsersLocked(byID, "cluster_invited", func(c *plots.PlotCluster, u uuid.UUID) {
		c.Invited[u] = struct{}{}
	}); err != nil {
		return nil, err
	}
	// Settings
	srows, err := d.sql.Query(`SELECT cluster_id, alias, position FROM ` + d.t("cluster_settings"))
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var (
			cid        int64
			alias, pos sql.NullString
		)
		if err := srows.Scan(&cid, &alias, &pos); err != nil {
			return nil, err
		}
		if c := byID[cid]; c != nil {
			if alias.Valid {
				c.Name = alias.String
				c.Settings.Alias = alias.String
			}
			if pos.Valid {
				if bl, err := plots.ParseBlockLoc(pos.String); err == nil {
					c.Settings.Position = bl
				}
			}
		}
	}
	return byArea, srows.Err()
}

func (d *DB) loadClusterUsersLocked(byID map[int64]*plots.PlotCluster, table string, add func(*plots.PlotCluster, uuid.UUID)) error {
	rows, err := d.sql.Query(`SELECT cluster_id, user_uuid FROM ` + d.t(table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid   int64
			userS string
		)
		if err := rows.Scan(&cid, &userS); err != nil {
			return err
		}
		if c := byID[cid]; c != nil {
			add(c, parseStoredUUID(userS))
		}
	}
	return rows.Err()
}

// CreateCluster inserts a cluster and its cluster_settings row.
func (d *DB) CreateCluster(c *plots.PlotCluster) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c.Area == nil {
		return fmt.Errorf("cluster has nil area")
	}
	res, err := d.sql.Exec(
		`INSERT INTO `+d.t("cluster")+` (pos1_x, pos1_z, pos2_x, pos2_z, owner, world) VALUES (?, ?, ?, ?, ?, ?)`,
		c.Pos1.X, c.Pos1.Y, c.Pos2.X, c.Pos2.Y, c.Owner.String(), c.Area.FullID(),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.DBID = id
	_, err = d.sql.Exec(
		`INSERT OR REPLACE INTO `+d.t("cluster_settings")+` (cluster_id, alias, position) VALUES (?, ?, ?)`,
		id, c.Name, c.Settings.Position.String(),
	)
	return err
}

// DeleteCluster deletes a cluster and its helpers/invited/settings rows.
func (d *DB) DeleteCluster(c *plots.PlotCluster) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c.DBID == 0 {
		return nil
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, tbl := range []string{"cluster_helpers", "cluster_invited", "cluster_settings"} {
		if _, err := tx.Exec(`DELETE FROM `+d.t(tbl)+` WHERE cluster_id = ?`, c.DBID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM `+d.t("cluster")+` WHERE id = ?`, c.DBID); err != nil {
		return err
	}
	return tx.Commit()
}

// ResizeCluster updates a cluster's bounds.
func (d *DB) ResizeCluster(c *plots.PlotCluster, pos1, pos2 plots.PlotId) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c.DBID == 0 {
		return nil
	}
	_, err := d.sql.Exec(
		`UPDATE `+d.t("cluster")+` SET pos1_x = ?, pos1_z = ?, pos2_x = ?, pos2_z = ? WHERE id = ?`,
		pos1.X, pos1.Y, pos2.X, pos2.Y, c.DBID,
	)
	return err
}

// SetClusterUser adds or removes a user in cluster_helpers or cluster_invited.
func (d *DB) SetClusterUser(c *plots.PlotCluster, table string, u uuid.UUID, add bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c.DBID == 0 {
		return nil
	}
	if add {
		_, err := d.sql.Exec(`INSERT INTO `+d.t(table)+` (cluster_id, user_uuid) VALUES (?, ?)`, c.DBID, u.String())
		return err
	}
	_, err := d.sql.Exec(`DELETE FROM `+d.t(table)+` WHERE cluster_id = ? AND user_uuid = ?`, c.DBID, u.String())
	return err
}

// SetClusterPosition updates cluster_settings.position.
func (d *DB) SetClusterPosition(c *plots.PlotCluster, pos plots.BlockLoc) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c.DBID == 0 {
		return nil
	}
	_, err := d.sql.Exec(
		`UPDATE `+d.t("cluster_settings")+` SET position = ? WHERE cluster_id = ?`,
		pos.String(), c.DBID,
	)
	return err
}

// SetPlayerMeta stores persistent player metadata in `player_meta`.
func (d *DB) SetPlayerMeta(u uuid.UUID, key string, val []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.sql.Exec(`DELETE FROM `+d.t("player_meta")+` WHERE uuid = ? AND key = ?`, u.String(), key); err != nil {
		return err
	}
	_, err := d.sql.Exec(`INSERT INTO `+d.t("player_meta")+` (uuid, key, value) VALUES (?, ?, ?)`, u.String(), key, val)
	return err
}

// GetPlayerMeta loads all persistent metadata keys for u.
func (d *DB) GetPlayerMeta(u uuid.UUID) (map[string][]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rows, err := d.sql.Query(`SELECT key, value FROM `+d.t("player_meta")+` WHERE uuid = ?`, u.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]byte)
	for rows.Next() {
		var k string
		var v []byte
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// RemovePlayerMeta deletes a metadata key for u.
func (d *DB) RemovePlayerMeta(u uuid.UUID, key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.sql.Exec(`DELETE FROM `+d.t("player_meta")+` WHERE uuid = ? AND key = ?`, u.String(), key)
	return err
}

func (d *DB) resolvePlotIDLocked(p *plots.Plot) (int64, error) {
	if p.DBID > 0 {
		return p.DBID, nil
	}
	if p.Area == nil {
		return 0, nil
	}
	var id int64
	err := d.sql.QueryRow(
		`SELECT id FROM `+d.t("plot")+` WHERE plot_id_x = ? AND plot_id_z = ? AND world = ? ORDER BY timestamp ASC LIMIT 1`,
		p.ID.X, p.ID.Y, p.Area.FullID(),
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	p.DBID = id
	return id, nil
}

func parseStoredUUID(s string) uuid.UUID {
	s = strings.TrimSpace(s)
	if s == "" {
		return uuid.Nil
	}
	if u, err := uuid.Parse(s); err == nil {
		return u
	}
	// Java's UUID.fromString("1-1-3-3-7") accepts short hex segments; normalize if needed.
	parts := strings.Split(s, "-")
	if len(parts) == 5 {
		lens := [5]int{8, 4, 4, 4, 12}
		padded := make([]string, 5)
		for i, p := range parts {
			if len(p) < lens[i] {
				p = strings.Repeat("0", lens[i]-len(p)) + p
			}
			padded[i] = p
		}
		if u, err := uuid.Parse(strings.Join(padded, "-")); err == nil {
			return u
		}
	}
	return uuid.NewMD5(uuid.NameSpaceOID, []byte("OfflinePlayer:"+s))
}

func parseTimestampMillis(raw any) int64 {
	switch v := raw.(type) {
	case time.Time:
		return v.UnixMilli()
	case int64:
		if v < 1e11 {
			return v * 1000
		}
		return v
	case string:
		for _, layout := range []string{
			time.RFC3339,
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05",
		} {
			if t, err := time.Parse(layout, v); err == nil {
				return t.UnixMilli()
			}
		}
	}
	return time.Now().UnixMilli()
}
