package npy

import (
	"encoding/binary"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// writeNpy writes a .npy file with a hand-built header.
func writeNpy(t *testing.T, path, descr string, shape []int64, littleEndianUint16 bool, data []byte) {
	t.Helper()
	dict := "{'descr': '" + descr + "', 'fortran_order': False, 'shape': (" 
	for i, s := range shape {
		if i > 0 {
			dict += ", "
		}
		dict += itoa(s)
	}
	dict += ", ), }"
	// Pad the header so the data starts at a 64-byte boundary.
	total := 10 + len(dict)
	pad := 64 - total%64
	if pad == 0 {
		pad = 64
	}
	dict += string(make([]byte, 0)) + repeatSpaces(pad-1) + "\n"

	var head []byte
	head = append(head, "\x93NUMPY"...)
	head = append(head, 1, 0)
	var len2 [2]byte
	binary.LittleEndian.PutUint16(len2[:], uint16(len(dict)))
	head = append(head, len2[:]...)
	head = append(head, dict...)

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.Write(head)
	f.Write(data)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func repeatSpaces(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

func TestOpenAndRow(t *testing.T) {
	dir := t.TempDir()

	// palette: 3 colours, RGB
	pal := []byte{0, 0, 0, 10, 20, 30, 40, 50, 60}
	writeNpy(t, filepath.Join(dir, "palette.npy"), "|u1", []int64{3, 3}, false, pal)

	// tokens: 2 rows of 4 elements (2x2 sprites), uint16
	// row 0: 0 (transparent), 1, 1, 2   row 1: 2, 2, 1, 0
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
	writeNpy(t, filepath.Join(dir, "tokens.npy"), "<u2", []int64{2, 4}, true, tok)

	ds, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ds.Close()
	if ds.Rows() != 2 || ds.width != 2 || ds.height != 2 {
		t.Fatalf("dataset = rows %d %dx%d", ds.Rows(), ds.width, ds.height)
	}

	img, err := ds.Row(0)
	if err != nil {
		t.Fatal(err)
	}
	want := []color.RGBA{
		{}, {10, 20, 30, 255}, {10, 20, 30, 255}, {40, 50, 60, 255},
	}
	for i, w := range want {
		got := img.RGBAAt(i%2, i/2)
		if got != w {
			t.Fatalf("pixel %d = %v, want %v", i, got, w)
		}
	}

	img1, err := ds.Row(1)
	if err != nil {
		t.Fatal(err)
	}
	if img1.RGBAAt(0, 0) != (color.RGBA{40, 50, 60, 255}) || img1.RGBAAt(3, 1) != (color.RGBA{}) {
		t.Fatal("row 1 pixels wrong")
	}

	if _, err := ds.Row(2); err == nil {
		t.Fatal("row 2 should be out of range")
	}

	// Cache hit returns the same image pointer.
	again, _ := ds.Row(0)
	if again != img {
		t.Fatal("second Row(0) should hit the cache")
	}
}

func TestLRUEviction(t *testing.T) {
	dir := t.TempDir()
	writeNpy(t, filepath.Join(dir, "palette.npy"), "|u1", []int64{2, 3}, false,
		[]byte{0, 0, 0, 255, 0, 0})
	rows := 8
	tok := make([]byte, 0, rows*4*2)
	for r := 0; r < rows; r++ {
		for e := 0; e < 4; e++ {
			var b [2]byte
			v := uint16(0)
			if e == r%4 {
				v = 1
			}
			binary.LittleEndian.PutUint16(b[:], v)
			tok = append(tok, b[:]...)
		}
	}
	writeNpy(t, filepath.Join(dir, "tokens.npy"), "<u2", []int64{int64(rows), 4}, true, tok)

	ds, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ds.Close()
	ds.cacheCap = 4

	first, _ := ds.Row(0)
	for i := 1; i < 8; i++ {
		ds.Row(int64(i))
	}
	if len(ds.cache) > 4 {
		t.Fatalf("cache holds %d > 4", len(ds.cache))
	}
	again, _ := ds.Row(0)
	if again == first {
		t.Fatal("row 0 should have been evicted and re-decoded")
	}
}
