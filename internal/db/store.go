package db

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/PatrickSteil/mobidata-poller/internal/model"
)

// Schema design
// ─────────────────────────────────────────────────────────────────────────────
// We use two tables that are kept in sync inside every write transaction:
//
//  vehicles        — canonical data store, PRIMARY KEY (id, operator)
//                    rowid is an auto-assigned integer used as the foreign key.
//
//  vehicles_geo    — SQLite R*Tree virtual table.
//                    Column layout: id INTEGER, minLat, maxLat, minLon, maxLon
//                    For a point feature minLat==maxLat and minLon==maxLon.
//                    The R*Tree id column shadows vehicles.rowid so we can
//                    JOIN back to the main table without a separate lookup.
//
// Spatial query flow:
//   1. Convert the search radius to a degree bounding box.
//   2. Let the R*Tree do the fast MBRI (bounding-box rectangle intersection).
//   3. JOIN to vehicles to apply optional type filter.
//   4. Exact Haversine check in Go to reject the ~22% false positives that
//      lie in the corners of the bounding box but outside the true circle.

const schema = `
CREATE TABLE IF NOT EXISTS vehicles (
	rowid        INTEGER PRIMARY KEY,  -- explicit alias for SQLite rowid
	id           TEXT    NOT NULL,
	operator     TEXT    NOT NULL,
	lat          REAL    NOT NULL,
	lon          REAL    NOT NULL,
	type         TEXT    NOT NULL,
	is_reserved  INTEGER NOT NULL DEFAULT 0,
	is_disabled  INTEGER NOT NULL DEFAULT 0,
	last_updated INTEGER NOT NULL,
	seen_at      INTEGER NOT NULL,
	UNIQUE (id, operator)
);

-- R*Tree spatial index.
-- Column 1 is the integer primary key that mirrors vehicles.rowid.
-- Columns 2-5 are the bounding box; for point data min==max.
CREATE VIRTUAL TABLE IF NOT EXISTS vehicles_geo USING rtree(
	id,           -- INTEGER, mirrors vehicles.rowid
	min_lat, max_lat,
	min_lon, max_lon
);

CREATE INDEX IF NOT EXISTS idx_vehicles_type ON vehicles(type);
`

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and ensures the schema
// exists.  The R*Tree module is compiled into go-sqlite3 by default.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1) // single writer; WAL allows concurrent readers
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate db: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// Reset drops and recreates all tables, giving a clean slate.
// Call this at the start of a poll cycle to hot-swap the database instead of
// upsert-diffing stale entries that may no longer exist in the live feed.
func (s *Store) Reset() error {
	_, err := s.db.Exec(`
		DROP TABLE IF EXISTS vehicles_geo;
		DROP TABLE IF EXISTS vehicles;
	`)
	if err != nil {
		return fmt.Errorf("reset db: %w", err)
	}
	_, err = s.db.Exec(schema)
	return err
}

// ── Writes ────────────────────────────────────────────────────────────────────

// UpsertVehicles inserts or replaces a batch of vehicles, keeping the R*Tree
// index in sync within the same transaction.
func (s *Store) UpsertVehicles(vehicles []model.Vehicle) error {
	if len(vehicles) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Upsert into the main table; we need the rowid back for the R*Tree entry.
	// INSERT OR REPLACE would delete+reinsert (new rowid), so we use
	// INSERT … ON CONFLICT DO UPDATE and then retrieve last_insert_rowid()
	// which returns the rowid of the affected row for both INSERT and UPDATE.
	mainStmt, err := tx.Prepare(`
		INSERT INTO vehicles (id, operator, lat, lon, type, is_reserved, is_disabled, last_updated, seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id, operator) DO UPDATE SET
			lat          = excluded.lat,
			lon          = excluded.lon,
			type         = excluded.type,
			is_reserved  = excluded.is_reserved,
			is_disabled  = excluded.is_disabled,
			last_updated = excluded.last_updated,
			seen_at      = excluded.seen_at
		RETURNING rowid
	`)
	if err != nil {
		return err
	}
	defer mainStmt.Close()

	// R*Tree virtual tables do not support ON CONFLICT / UPSERT.
	// The correct pattern is DELETE the old entry by rowid, then INSERT the
	// new one. Both are O(log n) and run in the same transaction so the index
	// is never inconsistent.
	geoDelStmt, err := tx.Prepare(`DELETE FROM vehicles_geo WHERE id = ?`)
	if err != nil {
		return err
	}
	defer geoDelStmt.Close()

	geoInsStmt, err := tx.Prepare(`
		INSERT INTO vehicles_geo (id, min_lat, max_lat, min_lon, max_lon)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer geoInsStmt.Close()

	for _, v := range vehicles {
		row := mainStmt.QueryRow(
			v.ID, v.Operator, v.Lat, v.Lon, string(v.Type),
			boolInt(v.IsReserved), boolInt(v.IsDisabled), v.LastUpdated, v.SeenAt,
		)
		var rowid int64
		if err = row.Scan(&rowid); err != nil {
			return fmt.Errorf("upsert vehicle %s: %w", v.ID, err)
		}
		// DELETE is a no-op on the first insert for a new vehicle.
		if _, err = geoDelStmt.Exec(rowid); err != nil {
			return fmt.Errorf("delete geo %s: %w", v.ID, err)
		}
		if _, err = geoInsStmt.Exec(rowid, v.Lat, v.Lat, v.Lon, v.Lon); err != nil {
			return fmt.Errorf("insert geo %s: %w", v.ID, err)
		}
	}

	return tx.Commit()
}

// ── Reads ─────────────────────────────────────────────────────────────────────

// QueryParams holds parameters for a geo+type query.
type QueryParams struct {
	Lat    float64
	Lon    float64
	Radius float64             // metres
	Types  []model.VehicleType // empty = all types
}

// Query returns vehicles within Radius metres of (Lat, Lon), optionally
// filtered by vehicle type.
//
// The R*Tree narrows candidates to a bounding box; we then apply an exact
// Haversine check in Go to get a true circle result.
func (s *Store) Query(p QueryParams) ([]model.Vehicle, error) {
	// Convert radius to degree deltas for the R*Tree bounding box.
	latDelta := p.Radius / 111_320.0
	lonDelta := p.Radius / (111_320.0 * math.Cos(p.Lat*math.Pi/180))

	minLat := p.Lat - latDelta
	maxLat := p.Lat + latDelta
	minLon := p.Lon - lonDelta
	maxLon := p.Lon + lonDelta

	// Build optional IN clause for type filter.
	var typeClause string
	args := []any{minLat, maxLat, minLon, maxLon}
	if len(p.Types) > 0 {
		ph := make([]string, len(p.Types))
		for i, t := range p.Types {
			ph[i] = "?"
			args = append(args, string(t))
		}
		typeClause = fmt.Sprintf("AND v.type IN (%s)", strings.Join(ph, ","))
	}

	// The R*Tree WHERE clause performs the fast spatial pre-filter.
	// The JOIN to vehicles fetches the full row and applies the type filter.
	q := fmt.Sprintf(`
		SELECT v.id, v.operator, v.lat, v.lon, v.type,
		       v.is_reserved, v.is_disabled, v.last_updated, v.seen_at
		FROM   vehicles_geo g
		JOIN   vehicles      v ON v.rowid = g.id
		WHERE  g.min_lat >= ? AND g.max_lat <= ?
		  AND  g.min_lon >= ? AND g.max_lon <= ?
		  %s
	`, typeClause)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type candidate struct {
		v    model.Vehicle
		dist float64
	}
	var candidates []candidate
	for rows.Next() {
		var v model.Vehicle
		var vtype string
		var isRes, isDis int
		if err := rows.Scan(
			&v.ID, &v.Operator, &v.Lat, &v.Lon, &vtype,
			&isRes, &isDis, &v.LastUpdated, &v.SeenAt,
		); err != nil {
			return nil, err
		}
		v.Type = model.VehicleType(vtype)
		v.IsReserved = isRes == 1
		v.IsDisabled = isDis == 1

		// Exact Haversine check — rejects the ~22% false positives in the
		// bounding box corners that fall outside the true search circle.
		if d := haversine(p.Lat, p.Lon, v.Lat, v.Lon); d <= p.Radius {
			candidates = append(candidates, candidate{v, d})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort nearest-first.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].dist < candidates[j].dist
	})

	results := make([]model.Vehicle, len(candidates))
	for i, c := range candidates {
		results[i] = c.v
	}
	return results, nil
}

// Stats returns vehicle counts grouped by type.
func (s *Store) Stats() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT type, COUNT(*) FROM vehicles GROUP BY type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]int{}
	for rows.Next() {
		var t string
		var c int
		if err := rows.Scan(&t, &c); err != nil {
			return nil, err
		}
		m[t] = c
	}
	return m, rows.Err()
}

// ── helpers ───────────────────────────────────────────────────────────────────

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// haversine returns the great-circle distance in metres between two points.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6_371_000 // Earth radius in metres
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
