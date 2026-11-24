// Package index ships the prebuilt SQLite search index inside the binary.
// The database (built from the pixel-art-16x16-1.4m dataset with
// internal/indexer) is embedded gzip-compressed and extracted to the user
// cache directory on first run, so the app never rebuilds an index.
//
// To ship a new index: rebuild with the indexer tool, then regenerate
// internal/index/icons.db.gz from the produced icons.db.
package index

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed icons.db.gz
var raw []byte

// DBPath returns the extracted database location under the user cache dir.
func DBPath() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "pixelart", "icons.db")
	}
	return "icons.db"
}

// Ensure extracts the embedded database to the cache dir when missing or
// incomplete, and returns its path. Extraction is atomic: a marker file is
// only written after the full database is in place.
func Ensure() (string, error) {
	p := DBPath()
	if marker, err := os.Stat(p + ".ok"); err == nil && !marker.IsDir() {
		if db, err := os.Stat(p); err == nil && db.Size() > 0 {
			return p, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", fmt.Errorf("创建缓存目录失败: %w", err)
	}
	tmp := p + ".tmp"
	if err := extract(tmp); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		return "", fmt.Errorf("写入索引失败: %w", err)
	}
	if err := os.WriteFile(p+".ok", []byte("ok"), 0o644); err != nil {
		return "", fmt.Errorf("写入索引标记失败: %w", err)
	}
	return p, nil
}

// extract gunzips the embedded database into dst.
func extract(dst string) error {
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("解压索引失败: %w", err)
	}
	defer zr.Close()
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("解压索引失败: %w", err)
	}
	if _, err := io.Copy(out, zr); err != nil {
		out.Close()
		return fmt.Errorf("解压索引失败: %w", err)
	}
	return out.Close()
}
