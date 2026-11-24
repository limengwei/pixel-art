package indexer

import (
	"os"
	"path/filepath"
)

// dbPath computes the database location under the user config directory.
func dbPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "pixelart", "icons.db")
	}
	return "icons.db"
}
