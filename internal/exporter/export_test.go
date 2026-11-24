package exporter

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func solid(t *testing.T, size int, c color.RGBA) *image.RGBA {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestExportScaled(t *testing.T) {
	dir := t.TempDir()
	img := solid(t, 16, color.RGBA{R: 255, A: 255})

	out, err := ExportPNG(img, "star_0000001", 256, dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(out) != "star_0000001_256px.png" {
		t.Fatalf("exported name = %q", filepath.Base(out))
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != 256 || got.Bounds().Dy() != 256 {
		t.Fatalf("exported size = %v", got.Bounds())
	}
}

func TestExportNative(t *testing.T) {
	dir := t.TempDir()
	img := solid(t, 16, color.RGBA{G: 255, A: 255})

	out, err := ExportPNG(img, "icon_x", 0, dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(out) != "icon_x.png" {
		t.Fatalf("exported name = %q", filepath.Base(out))
	}
	data, _ := os.ReadFile(out)
	got, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != 16 {
		t.Fatalf("native export size = %v", got.Bounds())
	}
}

func TestEncodePNG(t *testing.T) {
	img := solid(t, 16, color.RGBA{B: 255, A: 255})
	data, err := EncodePNG(img)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
}
