// Package dataset manages the pixel-art-16x16-1.4m dataset — the only one
// this app supports. It resolves a fixed directory under the user cache
// dir, checks readiness, and can download + extract the Hugging Face zip
// into place on first launch.
package dataset

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Name is the dataset's on-disk directory name.
const Name = "pixel-art-16x16-1.4m"

// DownloadURL is the Hugging Face zip download link.
const DownloadURL = "https://huggingface.co/datasets/Angais/pixel-art-atlas-1.4m/resolve/main/pixel-art-16x16-1.4m.zip?download=true"

// Dir returns the dataset directory under the user cache dir.
func Dir() string {
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "pixelart", Name)
	}
	return Name
}

// DBPath returns the search index database location under the user cache
// dir. The database is built by the welcome page's deploy step.
func DBPath() string {
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "pixelart", "icons.db")
	}
	return "icons.db"
}

// Ready reports whether the dataset's tokens.npy is present.
func Ready() bool {
	fi, err := os.Stat(filepath.Join(Dir(), "tokens.npy"))
	return err == nil && !fi.IsDir() && fi.Size() > 0
}

// ExpectedIcons is the dataset's fixed sprite count; a complete index
// holds exactly this many icons.
const ExpectedIcons = 1_409_404

// expectedFiles are the essential dataset files with their exact sizes in
// the fixed pixel-art-16x16-1.4m release. A size mismatch means an
// interrupted download or extraction.
var expectedFiles = map[string]int64{
	"tokens.npy":    721_614_976,
	"palette.npy":   12_419,
	"captions.json": 62_447_413,
	"objects.json":  21_605_555,
	"split.npy":     1_409_532,
}

// ManifestName is the SHA-256 manifest written into the dataset dir after
// a successful deploy; startup verification hashes files against it.
const ManifestName = "manifest.json"

// Check validates the dataset files under Dir() in two layers: exact size
// first, then — when a deploy left a hash manifest — file content. It
// reports each problem found; ok is true when everything matches.
func Check() (ok bool, problems []string) {
	return checkDir(Dir())
}

func checkDir(dir string) (bool, []string) {
	var m map[string]string
	if b, err := os.ReadFile(filepath.Join(dir, ManifestName)); err == nil {
		var mm map[string]string
		if json.Unmarshal(b, &mm) == nil {
			m = mm
		}
	}
	var problems []string
	for name, want := range expectedFiles {
		path := filepath.Join(dir, name)
		fi, err := os.Stat(path)
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("%s 缺失", name))
		case fi.Size() != want:
			problems = append(problems, fmt.Sprintf("%s 大小不符（%d / %d 字节，可能下载或解压中断）",
				name, fi.Size(), want))
		default:
			// Size matches; verify content when a manifest exists.
			if wantHash, ok := m[name]; ok {
				if h, err := sha256File(path); err == nil && h != wantHash {
					problems = append(problems, fmt.Sprintf("%s 内容校验失败（哈希不符，可能磁盘损坏）", name))
				}
			}
		}
	}
	sort.Strings(problems) // stable order for display
	return len(problems) == 0, problems
}

// HasManifest reports whether a deploy (or a startup backfill) already
// wrote the hash manifest.
func HasManifest() bool {
	fi, err := os.Stat(filepath.Join(Dir(), ManifestName))
	return err == nil && !fi.IsDir()
}

// WriteManifest hashes every essential dataset file (SHA-256) and stores
// the manifest inside the dataset dir. Called after a deploy completes so
// later startups can verify content, not just size.
func WriteManifest() error {
	return writeManifest(Dir())
}

func writeManifest(dir string) error {
	m := make(map[string]string, len(expectedFiles))
	for name := range expectedFiles {
		h, err := sha256File(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		m[name] = h
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ManifestName), b, 0o644)
}

// sha256File hashes a file streamingly.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// PartialZipPath is where an interrupted download leaves its half zip.
func PartialZipPath() string {
	return filepath.Join(filepath.Dir(Dir()), Name+".zip")
}

// Progress reports download or extraction advancement. Total is in bytes
// for both phases (0 = unknown). Note carries extra detail.
type Progress struct {
	Phase string // "下载" or "解压"
	Done  int64
	Total int64
	Note  string
}

// Download fetches the dataset zip into the cache dir and extracts it in
// place, deleting the zip afterwards. progress is called from the
// downloading goroutine and must be thread-safe.
func Download(ctx context.Context, progress func(Progress)) error {
	base := filepath.Dir(Dir())
	if err := os.MkdirAll(base, 0o755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}
	zipPath := filepath.Join(base, Name+".zip")

	if err := download(ctx, zipPath, progress); err != nil {
		os.Remove(zipPath)
		return err
	}
	if err := extract(zipPath, progress); err != nil {
		os.Remove(zipPath)
		return err
	}
	os.Remove(zipPath)
	if !Ready() {
		return fmt.Errorf("解压完成但未找到 tokens.npy，请重试")
	}
	return nil
}

// download streams the zip to disk, reporting byte progress. The total is
// the Content-Length (prefetched with HEAD when the GET is chunked), so the
// progress bar always has a denominator.
func download(ctx context.Context, dst string, progress func(Progress)) error {
	total := contentLength(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载数据集失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载数据集失败: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	progress(Progress{Phase: "下载", Total: total, Note: dst})

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("写入下载文件失败: %w", err)
	}
	buf := make([]byte, 1<<20)
	var done int64
	var last int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return fmt.Errorf("写入下载文件失败: %w", werr)
			}
			done += int64(n)
			if done-last >= 4<<20 || rerr == io.EOF { // update every ~4MB
				progress(Progress{Phase: "下载", Done: done, Total: total})
				last = done
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return fmt.Errorf("下载数据集失败: %w", rerr)
		}
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("写入下载文件失败: %w", err)
	}
	progress(Progress{Phase: "下载", Done: done, Total: done, Note: "下载完成"})
	return nil
}

// contentLength prefetches the zip size with a HEAD request; 0 when the
// server does not tell.
func contentLength(ctx context.Context) int64 {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, DownloadURL, nil)
	if err != nil {
		return 0
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.ContentLength > 0 {
		return resp.ContentLength
	}
	return 0
}

// extract unpacks the zip beside dst so the dataset lands at Dir().
func extract(zipPath string, progress func(Progress)) error {
	return extractAt(filepath.Dir(Dir()), Dir(), zipPath, progress)
}

// extractAt unpacks the zip under base (entries keep their inner paths);
// when the zip has no top-level dataset folder the loose files are moved
// into dir. Progress is bytes extracted vs the sum of uncompressed sizes
// (fixed for this dataset), reported in ~16MB steps so a single huge file
// like tokens.npy still moves the bar.
func extractAt(base, dir, zipPath string, progress func(Progress)) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("打开压缩包失败: %w", err)
	}
	defer zr.Close()

	var total int64
	for _, f := range zr.File {
		total += int64(f.UncompressedSize64)
	}
	progress(Progress{Phase: "解压", Total: total})

	var done, last int64
	report := func() {
		if done-last >= 16<<20 || done == total {
			progress(Progress{Phase: "解压", Done: done, Total: total})
			last = done
		}
	}
	for _, f := range zr.File {
		name := filepath.FromSlash(f.Name)
		if !filepath.IsLocal(name) {
			continue // skip anything escaping the root
		}
		dst := filepath.Join(base, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("解压失败: %w", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("解压失败: %w", err)
		}
		if err := extractFile(f, dst, func(n int64) {
			done += n
			report()
		}); err != nil {
			return err
		}
	}
	// The zip may or may not carry the top-level dataset folder; if it
	// doesn't, the files landed directly in base — move them into dir.
	if _, err := os.Stat(filepath.Join(dir, "tokens.npy")); err != nil {
		if err := hoist(base, dir); err != nil {
			return err
		}
	}
	return nil
}

// extractFile copies one zip entry in chunks, reporting each chunk's bytes.
func extractFile(f *zip.File, dst string, onBytes func(int64)) error {
	src, err := f.Open()
	if err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}
	defer src.Close()
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}
	buf := make([]byte, 8<<20)
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return fmt.Errorf("解压失败: %w", werr)
			}
			onBytes(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return fmt.Errorf("解压失败: %w", rerr)
		}
	}
	return out.Close()
}

// hoist moves loose dataset files from base into dir (used when the zip
// has no top-level folder).
func hoist(base, dir string) error {
	entries, err := os.ReadDir(base)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".npy") && !strings.HasSuffix(e.Name(), ".json") &&
			!strings.HasSuffix(e.Name(), ".md") && !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		if err := os.Rename(filepath.Join(base, e.Name()), filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("解压失败: %w", err)
		}
	}
	return nil
}
