// Package store keeps the icon index in SQLite and answers searches.
package store

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// schemaVersion bumps when the layout changes; an older database is
// dropped and rebuilt automatically.
const schemaVersion = "2"

// Icon is one indexed sprite, identified by its dataset row id.
type Icon struct {
	ID        int64  // dataset row id (0..1409403)
	Subject   string // dataset subject, e.g. "aa battery"
	Caption   string // full English caption
	CaptionZh string // translated Chinese tags for search
}

// Subject is a category row: a subject and how many icons it has.
type Subject struct {
	Name  string
	Count int
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// Open creates or opens the database at path, applying pragmas and schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(4)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// migrate drops a database of another schema version and recreates it.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("create meta table: %w", err)
	}
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v)
	switch {
	case err == sql.ErrNoRows:
		// Fresh, or a v1 database (which predates schema_version): check
		// whether the old layout is there.
		var one int
		if err := s.db.QueryRow(`SELECT 1 FROM pragma_table_info('icons') WHERE name = 'filename'`).Scan(&one); err == nil {
			if _, err := s.db.Exec(`DROP TRIGGER IF EXISTS icons_ai; DROP TRIGGER IF EXISTS icons_ad; DROP TRIGGER IF EXISTS icons_au;
				DROP TABLE IF EXISTS icons_fts; DROP TABLE IF EXISTS icons; DROP TABLE IF EXISTS subjects;`); err != nil {
				return fmt.Errorf("drop v1 schema: %w", err)
			}
		}
	case err != nil:
		return fmt.Errorf("read schema version: %w", err)
	case v != schemaVersion:
		if _, err := s.db.Exec(`DROP TRIGGER IF EXISTS icons_ai; DROP TRIGGER IF EXISTS icons_ad; DROP TRIGGER IF EXISTS icons_au;
			DROP TABLE IF EXISTS icons_fts; DROP TABLE IF EXISTS icons; DROP TABLE IF EXISTS subjects;`); err != nil {
			return fmt.Errorf("drop old schema: %w", err)
		}
	}
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if _, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, schemaVersion); err != nil {
		return fmt.Errorf("stamp schema version: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// GetSetting reads a settings value, or def when absent.
func (s *Store) GetSetting(key, def string) string {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return def
	}
	return v
}

// SetSetting writes a settings value.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS icons (
	id          INTEGER PRIMARY KEY,
	subject     TEXT NOT NULL,
	caption     TEXT NOT NULL DEFAULT '',
	caption_zh  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_icons_subject ON icons(subject);

CREATE VIRTUAL TABLE IF NOT EXISTS icons_fts USING fts5(
	subject,
	caption,
	caption_zh,
	content='icons',
	content_rowid='rowid'
);

CREATE TRIGGER IF NOT EXISTS icons_ai AFTER INSERT ON icons BEGIN
	INSERT INTO icons_fts(rowid, subject, caption, caption_zh)
	VALUES (new.rowid, new.subject, new.caption, new.caption_zh);
END;
CREATE TRIGGER IF NOT EXISTS icons_ad AFTER DELETE ON icons BEGIN
	INSERT INTO icons_fts(icons_fts, rowid, subject, caption, caption_zh)
	VALUES ('delete', old.rowid, old.subject, old.caption, old.caption_zh);
END;
CREATE TRIGGER IF NOT EXISTS icons_au AFTER UPDATE ON icons BEGIN
	INSERT INTO icons_fts(icons_fts, rowid, subject, caption, caption_zh)
	VALUES ('delete', old.rowid, old.subject, old.caption, old.caption_zh);
	INSERT INTO icons_fts(rowid, subject, caption, caption_zh)
	VALUES (new.rowid, new.subject, new.caption, new.caption_zh);
END;

CREATE TABLE IF NOT EXISTS subjects (
	subject TEXT PRIMARY KEY,
	cnt     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL DEFAULT ''
);
`
