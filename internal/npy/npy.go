// Package npy reads the Pixel Art Atlas dataset directly: tokens.npy (one
// palette-indexed 16x16 sprite per row) and palette.npy (its colours).
// Rows are decoded on demand with ReadAt, so nothing large is ever held in
// memory but the small palette and a bounded cache of decoded sprites.
package npy

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
)

// header is a parsed .npy header.
type header struct {
	dtype    string // element type as in the file, e.g. "<u2", "|u1"
	itemSize int
	shape    []int64
	dataAt   int64 // byte offset of the data
}

var descrRe = regexp.MustCompile(`'descr':\s*'([^']+)'`)
var shapeRe = regexp.MustCompile(`'shape':\s*\(([^)]*)\)`)

// parseHeader reads and parses a .npy header from r.
func parseHeader(r io.Reader) (header, error) {
	br := bufio.NewReader(r)
	magic := make([]byte, 6)
	if _, err := io.ReadFull(br, magic); err != nil {
		return header{}, fmt.Errorf("npy: read magic: %w", err)
	}
	if string(magic) != "\x93NUMPY" {
		return header{}, fmt.Errorf("npy: bad magic %q", magic)
	}
	ver := make([]byte, 2)
	if _, err := io.ReadFull(br, ver); err != nil {
		return header{}, fmt.Errorf("npy: read version: %w", err)
	}
	var headLen int64
	switch ver[0] {
	case 1:
		l := make([]byte, 2)
		if _, err := io.ReadFull(br, l); err != nil {
			return header{}, fmt.Errorf("npy: read header length: %w", err)
		}
		headLen = int64(binary.LittleEndian.Uint16(l))
	case 2:
		l := make([]byte, 4)
		if _, err := io.ReadFull(br, l); err != nil {
			return header{}, fmt.Errorf("npy: read header length: %w", err)
		}
		headLen = int64(binary.LittleEndian.Uint32(l))
	default:
		return header{}, fmt.Errorf("npy: unsupported version %d", ver[0])
	}
	raw := make([]byte, headLen)
	if _, err := io.ReadFull(br, raw); err != nil {
		return header{}, fmt.Errorf("npy: read header: %w", err)
	}
	h := header{dataAt: 10 + headLen}

	if m := descrRe.FindSubmatch(raw); m != nil {
		h.dtype = string(m[1])
	} else {
		return h, fmt.Errorf("npy: no descr in header")
	}
	switch h.dtype {
	case "|u1", "<u1", ">u1", "b1":
		h.itemSize = 1
	case "<u2", ">u2":
		h.itemSize = 2
	default:
		return h, fmt.Errorf("npy: unsupported dtype %q", h.dtype)
	}

	if m := shapeRe.FindSubmatch(raw); m != nil {
		for _, f := range splitCommas(string(m[1])) {
			if f == "" {
				continue
			}
			n, err := strconv.ParseInt(f, 10, 64)
			if err != nil {
				return h, fmt.Errorf("npy: bad shape: %w", err)
			}
			h.shape = append(h.shape, n)
		}
	} else {
		return h, fmt.Errorf("npy: no shape in header")
	}
	return h, nil
}

// splitCommas splits "1409404, 256" into its numbers.
func splitCommas(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			field := trimSpace(s[start:i])
			out = append(out, field)
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}

// Dataset is an open tokens.npy + palette.npy pair.
type Dataset struct {
	f        *os.File
	palette  []color.RGBA // index 0 is transparent
	rows     int64
	perRow   int64 // elements per row (256 = 16x16)
	stride   int64 // bytes per row
	dataAt   int64
	u16      bool
	width    int
	height   int
	cache    map[int64]*image.RGBA
	order    []int64 // LRU: oldest first
	cacheCap int
	mu       sync.Mutex
}

// Open opens the dataset rooted at dir, which holds tokens.npy and
// palette.npy.
func Open(dir string) (*Dataset, error) {
	if dir == "" {
		return nil, fmt.Errorf("npy: dataset dir empty")
	}
	tokPath := filepath.Join(dir, "tokens.npy")
	palPath := filepath.Join(dir, "palette.npy")

	f, err := os.Open(tokPath)
	if err != nil {
		return nil, fmt.Errorf("npy: open tokens: %w", err)
	}
	th, err := parseHeader(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("npy: %s: %w", tokPath, err)
	}
	if len(th.shape) != 2 {
		f.Close()
		return nil, fmt.Errorf("npy: tokens shape %v is not 2D", th.shape)
	}

	pf, err := os.Open(palPath)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("npy: open palette: %w", err)
	}
	defer pf.Close()
	ph, err := parseHeader(pf)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("npy: %s: %w", palPath, err)
	}
	if len(ph.shape) != 2 || (ph.shape[1] != 3 && ph.shape[1] != 4) {
		f.Close()
		return nil, fmt.Errorf("npy: palette shape %v is not (n, 3) or (n, 4)", ph.shape)
	}
	channels := int(ph.shape[1])
	raw, err := io.ReadAll(io.NewSectionReader(pf, ph.dataAt, ph.shape[0]*int64(channels)))
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("npy: read palette: %w", err)
	}
	palette := make([]color.RGBA, ph.shape[0])
	for i := range palette {
		o := i * channels
		if o >= len(raw) {
			break
		}
		palette[i] = color.RGBA{R: raw[o], G: raw[o+1], B: raw[o+2], A: 255}
	}
	palette[0] = color.RGBA{} // index zero is transparent

	perRow := th.shape[1]
	side := int(math.Sqrt(float64(perRow)))
	if int64(side*side) != perRow || side == 0 {
		f.Close()
		return nil, fmt.Errorf("npy: %d elements per row is not a square", perRow)
	}
	return &Dataset{
		f: f, palette: palette, rows: th.shape[0], perRow: perRow,
		stride: perRow * int64(th.itemSize), dataAt: th.dataAt,
		u16: th.itemSize == 2, width: side, height: side,
		cache: make(map[int64]*image.RGBA, 1024), cacheCap: 1024,
	}, nil
}

// Close releases the tokens file.
func (d *Dataset) Close() error { return d.f.Close() }

// Rows returns how many sprites the dataset holds.
func (d *Dataset) Rows() int64 { return d.rows }

// Row decodes sprite id as an RGBA image (16x16), reading only that row
// from disk. Results are cached (LRU) for reuse while scrolling.
func (d *Dataset) Row(id int64) (*image.RGBA, error) {
	if id < 0 || id >= d.rows {
		return nil, fmt.Errorf("npy: row %d out of range 0..%d", id, d.rows-1)
	}
	d.mu.Lock()
	if img, ok := d.cache[id]; ok {
		d.touch(id)
		d.mu.Unlock()
		return img, nil
	}
	d.mu.Unlock()

	buf := make([]byte, d.stride)
	if _, err := d.f.ReadAt(buf, d.dataAt+id*d.stride); err != nil {
		return nil, fmt.Errorf("npy: read row %d: %w", id, err)
	}
	img := image.NewRGBA(image.Rect(0, 0, d.width, d.height))
	for i := int64(0); i < d.perRow; i++ {
		var idx int64
		if d.u16 {
			idx = int64(binary.LittleEndian.Uint16(buf[i*2:]))
		} else {
			idx = int64(buf[i])
		}
		if idx == 0 || idx >= int64(len(d.palette)) {
			continue // transparent
		}
		img.SetRGBA(int(i)%d.width, int(i)/d.width, d.palette[idx])
	}

	d.mu.Lock()
	d.cache[id] = img
	d.order = append(d.order, id)
	for int64(len(d.order)) > int64(d.cacheCap) {
		oldest := d.order[0]
		d.order = d.order[1:]
		delete(d.cache, oldest)
	}
	d.mu.Unlock()
	return img, nil
}

// touch moves id to the fresh end of the LRU order.
func (d *Dataset) touch(id int64) {
	for i, v := range d.order {
		if v == id {
			d.order = append(d.order[:i], d.order[i+1:]...)
			d.order = append(d.order, id)
			return
		}
	}
}
