package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// SearchResult is one page of icons plus the total match count.
type SearchResult struct {
	Icons []Icon
	Total int
}

// Search returns a page of icons matching the FTS query ("" browses all),
// optionally filtered to one subject, ordered by FTS rank (or id when
// browsing).
func (s *Store) Search(query, subject string, page, pageSize int) SearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query, subject = strings.TrimSpace(query), strings.TrimSpace(subject)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 200
	}
	offset := (page - 1) * pageSize

	var (
		rows  *sql.Rows
		total int
		err   error
	)
	if query != "" {
		fts := buildFTSQuery(query)
		if subject != "" {
			s.db.QueryRow(`SELECT COUNT(*) FROM icons_fts f JOIN icons i ON i.rowid = f.rowid
				WHERE icons_fts MATCH ? AND i.subject = ?`, fts, subject).Scan(&total)
			rows, err = s.db.Query(`SELECT i.id, i.subject, i.caption, i.caption_zh
				FROM icons_fts f JOIN icons i ON i.rowid = f.rowid
				WHERE icons_fts MATCH ? AND i.subject = ?
				ORDER BY rank LIMIT ? OFFSET ?`, fts, subject, pageSize, offset)
		} else {
			s.db.QueryRow(`SELECT COUNT(*) FROM icons_fts WHERE icons_fts MATCH ?`, fts).Scan(&total)
			rows, err = s.db.Query(`SELECT i.id, i.subject, i.caption, i.caption_zh
				FROM icons_fts f JOIN icons i ON i.rowid = f.rowid
				WHERE icons_fts MATCH ?
				ORDER BY rank LIMIT ? OFFSET ?`, fts, pageSize, offset)
		}
	} else {
		if subject != "" {
			s.db.QueryRow(`SELECT COUNT(*) FROM icons WHERE subject = ?`, subject).Scan(&total)
			rows, err = s.db.Query(`SELECT id, subject, caption, caption_zh
				FROM icons WHERE subject = ? ORDER BY id LIMIT ? OFFSET ?`, subject, pageSize, offset)
		} else {
			s.db.QueryRow(`SELECT COUNT(*) FROM icons`).Scan(&total)
			rows, err = s.db.Query(`SELECT id, subject, caption, caption_zh
				FROM icons ORDER BY id LIMIT ? OFFSET ?`, pageSize, offset)
		}
	}
	if err != nil {
		return SearchResult{Icons: []Icon{}, Total: total}
	}
	return SearchResult{Icons: scanIcons(rows), Total: total}
}

// GetIcon returns one icon by its id.
func (s *Store) GetIcon(id int64) (Icon, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ic Icon
	err := s.db.QueryRow(`SELECT id, subject, caption, caption_zh FROM icons WHERE id = ?`, id).
		Scan(&ic.ID, &ic.Subject, &ic.Caption, &ic.CaptionZh)
	if err != nil {
		return Icon{}, false
	}
	return ic, true
}

// TotalIcons counts indexed icons.
func (s *Store) TotalIcons() int {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM icons`).Scan(&n)
	return n
}

// Subjects lists all subjects with their icon counts, largest first.
func (s *Store) Subjects() []Subject {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT subject, cnt FROM subjects ORDER BY cnt DESC, subject`)
	if err != nil {
		return []Subject{}
	}
	defer rows.Close()
	out := []Subject{}
	for rows.Next() {
		var su Subject
		if rows.Scan(&su.Name, &su.Count) == nil {
			out = append(out, su)
		}
	}
	return out
}

// RebuildSubjects recomputes the subjects table from the icons table.
func (s *Store) RebuildSubjects() error {
	_, err := s.db.Exec(`DELETE FROM subjects;
		INSERT INTO subjects(subject, cnt) SELECT subject, count(*) FROM icons GROUP BY subject`)
	return err
}

// InsertBatch inserts icons in one transaction (idempotent by id).
func (s *Store) InsertBatch(icons []Icon) error {
	if len(icons) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO icons(id, subject, caption, caption_zh)
		VALUES(?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	for _, ic := range icons {
		if _, err := stmt.Exec(ic.ID, ic.Subject, ic.Caption, ic.CaptionZh); err != nil {
			stmt.Close()
			tx.Rollback()
			return fmt.Errorf("insert id %d: %w", ic.ID, err)
		}
	}
	stmt.Close()
	return tx.Commit()
}

// buildFTSQuery turns a user query into an FTS5 AND-prefix match, quoting
// words so punctuation is literal.
func buildFTSQuery(query string) string {
	words := strings.Fields(query)
	for i, w := range words {
		words[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"*`
	}
	return strings.Join(words, " ")
}

func scanIcons(rows *sql.Rows) []Icon {
	defer rows.Close()
	icons := []Icon{}
	for rows.Next() {
		var ic Icon
		if rows.Scan(&ic.ID, &ic.Subject, &ic.Caption, &ic.CaptionZh) == nil {
			icons = append(icons, ic)
		}
	}
	return icons
}
