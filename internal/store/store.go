package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct{ db *sql.DB }

// Backup is a single tracked backup job. Status is one of: completed,
// running, failed, scheduled, paused. Schedule is a free-form string
// (cron expression, "daily at 2am", etc.) and is informational only.
type Backup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	SizeBytes   int    `json:"size_bytes"`
	Status      string `json:"status"`
	Schedule    string `json:"schedule"`
	LastRunAt   string `json:"last_run_at"`
	CreatedAt   string `json:"created_at"`
}

func Open(d string) (*DB, error) {
	if err := os.MkdirAll(d, 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(d, "saddlebag.db")+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.Exec(`CREATE TABLE IF NOT EXISTS backups(
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		source TEXT DEFAULT '',
		destination TEXT DEFAULT '',
		size_bytes INTEGER DEFAULT 0,
		status TEXT DEFAULT 'completed',
		schedule TEXT DEFAULT '',
		last_run_at TEXT DEFAULT '',
		created_at TEXT DEFAULT(datetime('now'))
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_backups_status ON backups(status)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_backups_last_run ON backups(last_run_at)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS extras(
		resource TEXT NOT NULL,
		record_id TEXT NOT NULL,
		data TEXT NOT NULL DEFAULT '{}',
		PRIMARY KEY(resource, record_id)
	)`)
	return &DB{db: db}, nil
}

func (d *DB) Close() error { return d.db.Close() }

func genID() string { return fmt.Sprintf("%d", time.Now().UnixNano()) }
func now() string   { return time.Now().UTC().Format(time.RFC3339) }

func (d *DB) Create(e *Backup) error {
	e.ID = genID()
	e.CreatedAt = now()
	if e.Status == "" {
		e.Status = "scheduled"
	}
	_, err := d.db.Exec(
		`INSERT INTO backups(id, name, source, destination, size_bytes, status, schedule, last_run_at, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Name, e.Source, e.Destination, e.SizeBytes, e.Status, e.Schedule, e.LastRunAt, e.CreatedAt,
	)
	return err
}

func (d *DB) Get(id string) *Backup {
	var e Backup
	err := d.db.QueryRow(
		`SELECT id, name, source, destination, size_bytes, status, schedule, last_run_at, created_at
		 FROM backups WHERE id=?`,
		id,
	).Scan(&e.ID, &e.Name, &e.Source, &e.Destination, &e.SizeBytes, &e.Status, &e.Schedule, &e.LastRunAt, &e.CreatedAt)
	if err != nil {
		return nil
	}
	return &e
}

func (d *DB) List() []Backup {
	rows, _ := d.db.Query(
		`SELECT id, name, source, destination, size_bytes, status, schedule, last_run_at, created_at
		 FROM backups ORDER BY name ASC`,
	)
	if rows == nil {
		return nil
	}
	defer rows.Close()
	var o []Backup
	for rows.Next() {
		var e Backup
		rows.Scan(&e.ID, &e.Name, &e.Source, &e.Destination, &e.SizeBytes, &e.Status, &e.Schedule, &e.LastRunAt, &e.CreatedAt)
		o = append(o, e)
	}
	return o
}

func (d *DB) Update(e *Backup) error {
	_, err := d.db.Exec(
		`UPDATE backups SET name=?, source=?, destination=?, size_bytes=?, status=?, schedule=?, last_run_at=?
		 WHERE id=?`,
		e.Name, e.Source, e.Destination, e.SizeBytes, e.Status, e.Schedule, e.LastRunAt, e.ID,
	)
	return err
}

// MarkRun stamps the backup as having just completed (or failed) a run.
// Updates status, size_bytes, and last_run_at in a single statement
// without touching name, source, destination, or schedule.
func (d *DB) MarkRun(id, status string, sizeBytes int) error {
	_, err := d.db.Exec(
		`UPDATE backups SET status=?, size_bytes=?, last_run_at=? WHERE id=?`,
		status, sizeBytes, now(), id,
	)
	return err
}

func (d *DB) Delete(id string) error {
	_, err := d.db.Exec(`DELETE FROM backups WHERE id=?`, id)
	return err
}

func (d *DB) Count() int {
	var n int
	d.db.QueryRow(`SELECT COUNT(*) FROM backups`).Scan(&n)
	return n
}

func (d *DB) Search(q string, filters map[string]string) []Backup {
	where := "1=1"
	args := []any{}
	if q != "" {
		where += " AND (name LIKE ? OR source LIKE ? OR destination LIKE ?)"
		s := "%" + q + "%"
		args = append(args, s, s, s)
	}
	if v, ok := filters["status"]; ok && v != "" {
		where += " AND status=?"
		args = append(args, v)
	}
	rows, _ := d.db.Query(
		`SELECT id, name, source, destination, size_bytes, status, schedule, last_run_at, created_at
		 FROM backups WHERE `+where+`
		 ORDER BY name ASC`,
		args...,
	)
	if rows == nil {
		return nil
	}
	defer rows.Close()
	var o []Backup
	for rows.Next() {
		var e Backup
		rows.Scan(&e.ID, &e.Name, &e.Source, &e.Destination, &e.SizeBytes, &e.Status, &e.Schedule, &e.LastRunAt, &e.CreatedAt)
		o = append(o, e)
	}
	return o
}

// Stats returns total backups, total bytes across all completed backups,
// counts by status, and the count of backups that have failed (which is
// the most actionable metric for an admin).
func (d *DB) Stats() map[string]any {
	m := map[string]any{
		"total":       d.Count(),
		"total_bytes": 0,
		"failed":      0,
		"by_status":   map[string]int{},
	}

	var totalBytes int64
	d.db.QueryRow(`SELECT COALESCE(SUM(size_bytes), 0) FROM backups WHERE status='completed'`).Scan(&totalBytes)
	m["total_bytes"] = totalBytes

	var failed int
	d.db.QueryRow(`SELECT COUNT(*) FROM backups WHERE status='failed'`).Scan(&failed)
	m["failed"] = failed

	if rows, _ := d.db.Query(`SELECT status, COUNT(*) FROM backups GROUP BY status`); rows != nil {
		defer rows.Close()
		by := map[string]int{}
		for rows.Next() {
			var s string
			var c int
			rows.Scan(&s, &c)
			by[s] = c
		}
		m["by_status"] = by
	}

	return m
}

// ─── Extras ───────────────────────────────────────────────────────

func (d *DB) GetExtras(resource, recordID string) string {
	var data string
	err := d.db.QueryRow(
		`SELECT data FROM extras WHERE resource=? AND record_id=?`,
		resource, recordID,
	).Scan(&data)
	if err != nil || data == "" {
		return "{}"
	}
	return data
}

func (d *DB) SetExtras(resource, recordID, data string) error {
	if data == "" {
		data = "{}"
	}
	_, err := d.db.Exec(
		`INSERT INTO extras(resource, record_id, data) VALUES(?, ?, ?)
		 ON CONFLICT(resource, record_id) DO UPDATE SET data=excluded.data`,
		resource, recordID, data,
	)
	return err
}

func (d *DB) DeleteExtras(resource, recordID string) error {
	_, err := d.db.Exec(
		`DELETE FROM extras WHERE resource=? AND record_id=?`,
		resource, recordID,
	)
	return err
}

func (d *DB) AllExtras(resource string) map[string]string {
	out := make(map[string]string)
	rows, _ := d.db.Query(
		`SELECT record_id, data FROM extras WHERE resource=?`,
		resource,
	)
	if rows == nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, data string
		rows.Scan(&id, &data)
		out[id] = data
	}
	return out
}
