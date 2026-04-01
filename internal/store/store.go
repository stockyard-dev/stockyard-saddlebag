package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn    *sql.DB
	dataDir string
}

func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	os.MkdirAll(filepath.Join(dataDir, "files"), 0755)
	conn, err := sql.Open("sqlite", filepath.Join(dataDir, "saddlebag.db"))
	if err != nil {
		return nil, err
	}
	conn.Exec("PRAGMA journal_mode=WAL")
	conn.Exec("PRAGMA busy_timeout=5000")
	conn.SetMaxOpenConns(4)
	db := &DB{conn: conn, dataDir: dataDir}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error { return db.conn.Close() }
func (db *DB) FilesDir() string { return filepath.Join(db.dataDir, "files") }

func (db *DB) migrate() error {
	_, err := db.conn.Exec(`
CREATE TABLE IF NOT EXISTS files (
    id TEXT PRIMARY KEY,
    filename TEXT NOT NULL,
    size_bytes INTEGER DEFAULT 0,
    content_type TEXT DEFAULT 'application/octet-stream',
    downloads INTEGER DEFAULT 0,
    max_downloads INTEGER DEFAULT 0,
    password TEXT DEFAULT '',
    expires_at TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);
`)
	return err
}

type File struct {
	ID           string `json:"id"`
	Filename     string `json:"filename"`
	SizeBytes    int64  `json:"size_bytes"`
	ContentType  string `json:"content_type"`
	Downloads    int    `json:"downloads"`
	MaxDownloads int    `json:"max_downloads"`
	HasPassword  bool   `json:"has_password"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	CreatedAt    string `json:"created_at"`
}

func (db *DB) CreateFile(filename, contentType string, sizeBytes int64, maxDownloads int, password, expiresAt string) (*File, error) {
	id := genID(10)
	now := time.Now().UTC().Format(time.RFC3339)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := db.conn.Exec("INSERT INTO files (id,filename,size_bytes,content_type,max_downloads,password,expires_at,created_at) VALUES (?,?,?,?,?,?,?,?)",
		id, filename, sizeBytes, contentType, maxDownloads, password, expiresAt, now)
	if err != nil {
		return nil, err
	}
	return &File{ID: id, Filename: filename, SizeBytes: sizeBytes, ContentType: contentType,
		MaxDownloads: maxDownloads, HasPassword: password != "", ExpiresAt: expiresAt, CreatedAt: now}, nil
}

func (db *DB) GetFile(id string) (*File, error) {
	var f File
	var pw string
	err := db.conn.QueryRow("SELECT id,filename,size_bytes,content_type,downloads,max_downloads,password,expires_at,created_at FROM files WHERE id=?", id).
		Scan(&f.ID, &f.Filename, &f.SizeBytes, &f.ContentType, &f.Downloads, &f.MaxDownloads, &pw, &f.ExpiresAt, &f.CreatedAt)
	f.HasPassword = pw != ""
	return &f, err
}

func (db *DB) GetFilePassword(id string) string {
	var pw string
	db.conn.QueryRow("SELECT password FROM files WHERE id=?", id).Scan(&pw)
	return pw
}

func (db *DB) IncrementDownloads(id string) {
	db.conn.Exec("UPDATE files SET downloads=downloads+1 WHERE id=?", id)
}

func (db *DB) IsExpired(f *File) bool {
	if f.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, f.ExpiresAt)
		if err == nil && time.Now().After(t) {
			return true
		}
	}
	if f.MaxDownloads > 0 && f.Downloads >= f.MaxDownloads {
		return true
	}
	return false
}

func (db *DB) ListFiles() ([]File, error) {
	rows, err := db.conn.Query("SELECT id,filename,size_bytes,content_type,downloads,max_downloads,password,expires_at,created_at FROM files ORDER BY created_at DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []File
	for rows.Next() {
		var f File
		var pw string
		rows.Scan(&f.ID, &f.Filename, &f.SizeBytes, &f.ContentType, &f.Downloads, &f.MaxDownloads, &pw, &f.ExpiresAt, &f.CreatedAt)
		f.HasPassword = pw != ""
		out = append(out, f)
	}
	return out, rows.Err()
}

func (db *DB) DeleteFile(id string) error {
	os.Remove(filepath.Join(db.FilesDir(), id))
	_, err := db.conn.Exec("DELETE FROM files WHERE id=?", id)
	return err
}

func (db *DB) TotalFiles() int {
	var c int
	db.conn.QueryRow("SELECT COUNT(*) FROM files").Scan(&c)
	return c
}

func (db *DB) TotalSize() int64 {
	var s int64
	db.conn.QueryRow("SELECT COALESCE(SUM(size_bytes),0) FROM files").Scan(&s)
	return s
}

func (db *DB) Stats() map[string]any {
	var files int
	var size int64
	db.conn.QueryRow("SELECT COUNT(*) FROM files").Scan(&files)
	db.conn.QueryRow("SELECT COALESCE(SUM(size_bytes),0) FROM files").Scan(&size)
	var downloads int
	db.conn.QueryRow("SELECT COALESCE(SUM(downloads),0) FROM files").Scan(&downloads)
	return map[string]any{"files": files, "total_bytes": size, "total_downloads": downloads}
}

func (db *DB) Cleanup() (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := db.conn.Query("SELECT id FROM files WHERE expires_at != '' AND expires_at < ?", now)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var count int64
	for rows.Next() {
		var id string
		rows.Scan(&id)
		os.Remove(filepath.Join(db.FilesDir(), id))
		db.conn.Exec("DELETE FROM files WHERE id=?", id)
		count++
	}
	return count, nil
}

func genID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
