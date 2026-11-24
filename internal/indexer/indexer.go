// Package indexer builds the search index from the dataset's captions.json
// and objects.json into the store. Nothing else is read: sprites render
// straight from tokens.npy on demand.
package indexer

import (
	"fmt"
	"time"

	"pixelart/internal/store"
	"pixelart/internal/translate"
)

// Progress reports an indexing phase's advancement. Phase is a short human
// label (Chinese); Done/Total count rows; Note carries extra detail.
type Progress struct {
	Phase string
	Done  int
	Total int
	Note  string
}

const (
	phaseMeta  = "加载元数据"
	phaseIndex = "写入索引"
)

// DefaultMetaDir is the built-in dataset location.
const DefaultMetaDir = `C:\Users\Administrator\Downloads\pixel-art-16x16-1.4m`

// NeedsBuild reports whether the store is empty and should be indexed at
// startup.
func NeedsBuild(st *store.Store) bool { return st.TotalIcons() == 0 }

// Run builds the full index from metaDir's captions.json and objects.json.
// progress is called from this goroutine and must be thread-safe.
func Run(metaDir string, st *store.Store, progress func(Progress)) error {
	if metaDir == "" {
		return fmt.Errorf("数据集目录未配置")
	}
	progress(Progress{Phase: phaseMeta, Note: metaDir})
	meta, err := LoadMeta(metaDir, func(done, total int) {
		progress(Progress{Phase: phaseMeta, Done: done, Total: total})
	})
	if err != nil {
		return fmt.Errorf("加载元数据失败: %w", err)
	}

	total := meta.Len()
	batch := make([]store.Icon, 0, 5000)
	flush := func() error { return st.InsertBatch(batch) }
	for id := 0; id < total; id++ {
		caption, subject, _ := meta.Get(int64(id))
		if subject == "" {
			continue
		}
		batch = append(batch, store.Icon{
			ID:        int64(id),
			Subject:   subject,
			Caption:   caption,
			CaptionZh: translate.Tags(caption + " " + subject),
		})
		if len(batch) >= 5000 {
			if err := flush(); err != nil {
				return fmt.Errorf("写入索引失败: %w", err)
			}
			batch = batch[:0]
		}
		if (id+1)%10_000 == 0 {
			progress(Progress{Phase: phaseIndex, Done: id + 1, Total: total})
		}
	}
	if err := flush(); err != nil {
		return fmt.Errorf("写入索引失败: %w", err)
	}
	if err := st.RebuildSubjects(); err != nil {
		return fmt.Errorf("刷新分类失败: %w", err)
	}
	st.SetSetting("last_index", time.Now().Format(time.RFC3339))
	progress(Progress{Phase: "", Done: total, Total: total, Note: fmt.Sprintf("共 %d 个图标", total)})
	return nil
}

// DBPath returns the database location under the user config directory.
func DBPath() string {
	return dbPath()
}
