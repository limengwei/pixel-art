package mcp

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"pixelart/internal/npy"
	"pixelart/internal/store"
)

// writeNpy is a minimal .npy writer for tests.
func writeNpy(t *testing.T, path, descr string, shape []int64, data []byte) {
	t.Helper()
	dict := "{'descr': '" + descr + "', 'fortran_order': False, 'shape': (" + shapeStr(shape) + ", ), }"
	for len(dict)%64 != 63 { // pad so dict+\n aligns the data to 64
		dict += " "
	}
	dict += "\n"
	var head []byte
	head = append(head, "\x93NUMPY"...)
	head = append(head, 1, 0)
	var l [2]byte
	binary.LittleEndian.PutUint16(l[:], uint16(len(dict)))
	head = append(head, l[:]...)
	head = append(head, dict...)
	os.WriteFile(path, append(head, data...), 0o644)
}

func shapeStr(shape []int64) string {
	parts := make([]string, len(shape))
	for i, s := range shape {
		parts[i] = itoa64(s)
	}
	return strings.Join(parts, ", ")
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

// newTestEnv returns a store with two icons and a tiny dataset of 2x2
// sprites.
func newTestEnv(t *testing.T) (*store.Store, *npy.Dataset) {
	t.Helper()
	dir := t.TempDir()
	// palette: index 1 red, 2 blue.
	writeNpy(t, filepath.Join(dir, "palette.npy"), "|u1", []int64{3, 3},
		[]byte{0, 0, 0, 255, 0, 0, 0, 0, 255})
	// tokens: 2 rows of 4 uint16 elements.
	tok := make([]byte, 0, 16)
	for _, v := range []uint16{0, 1, 1, 2} {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], v)
		tok = append(tok, b[:]...)
	}
	for _, v := range []uint16{2, 2, 1, 0} {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], v)
		tok = append(tok, b[:]...)
	}
	writeNpy(t, filepath.Join(dir, "tokens.npy"), "<u2", []int64{2, 4}, tok)

	ds, err := npy.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ds.Close() })

	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.InsertBatch([]store.Icon{
		{ID: 0, Subject: "aa battery", Caption: "brand new aa battery", CaptionZh: "电池"},
		{ID: 1, Subject: "dress form", Caption: "pink dress form", CaptionZh: "裙子 模台"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.RebuildSubjects(); err != nil {
		t.Fatal(err)
	}
	return st, ds
}

func TestToolsOverInMemoryTransport(t *testing.T) {
	st, ds := newTestEnv(t)
	server := New(st, ds)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	go server.Run(ctx, serverTransport)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}

	call := func(name string, args map[string]any) string {
		t.Helper()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s returned error: %+v", name, res.Content)
		}
		if len(res.Content) == 0 {
			t.Fatalf("%s returned no content", name)
		}
		text, _ := res.Content[0].(*mcp.TextContent)
		return text.Text
	}

	// search_icons in English and Chinese.
	if text := call("search_icons", map[string]any{"query": "battery", "pageSize": 10}); !strings.Contains(text, "id=0") {
		t.Fatalf("search battery = %q", text)
	}
	if text := call("search_icons", map[string]any{"query": "电池"}); !strings.Contains(text, "id=0") {
		t.Fatalf("search 电池 = %q", text)
	}
	// get_icon.
	if text := call("get_icon", map[string]any{"id": 1}); !strings.Contains(text, "dress form") {
		t.Fatalf("get_icon = %q", text)
	}
	// export_icon renders from the dataset.
	out := call("export_icon", map[string]any{"id": 0, "size": 64, "directory": t.TempDir()})
	if !strings.Contains(out, ".png") {
		t.Fatalf("export_icon = %q", out)
	}
	data, err := os.ReadFile(strings.TrimSpace(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("exported file empty")
	}
	// The rendered sprite's red pixel round-trips through the export.
	// (2x2 upscaled to 64x64 nearest: top-right quadrant is red.)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 64 {
		t.Fatalf("export size = %v", img.Bounds())
	}
	r, g, b, _ := img.At(48, 16).RGBA()
	if r == 0 || g != 0 || b != 0 {
		t.Fatalf("pixel (48,16) = %d %d %d, want red", r, g, b)
	}
}
