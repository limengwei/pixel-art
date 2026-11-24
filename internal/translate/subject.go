package translate

import (
	"embed"
	"encoding/json"
	"io/fs"
	"strings"
	"sync"
)

// zhFS holds the per-part subject translation files (part01.json…).
//
//go:embed zh
var zhFS embed.FS

var (
	subjectOnce sync.Once
	subjectZh   map[string]string
)

// subjectMap lazily merges every zh/*.json part into one lookup table.
func subjectMap() map[string]string {
	subjectOnce.Do(func() {
		subjectZh = make(map[string]string, 7000)
		entries, err := fs.ReadDir(zhFS, "zh")
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			raw, err := zhFS.ReadFile("zh/" + e.Name())
			if err != nil {
				continue
			}
			var m map[string]string
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			for k, v := range m {
				subjectZh[k] = v
			}
		}
	})
	return subjectZh
}

// Subject returns the Chinese display name for an English icon subject
// (e.g. "butterfly" -> "蝴蝶"), or "" when there is no translation.
func Subject(name string) string {
	v, ok := subjectMap()[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return ""
	}
	return v
}
