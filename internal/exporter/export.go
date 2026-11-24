// Package exporter writes sprites out as PNG files at requested sizes.
package exporter

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
)

// ExportPNG writes img to outDir at size×size pixels using nearest-neighbor
// sampling (pixel art stays crisp); size 0 keeps img's own size. name is
// the file name without extension. It returns the written absolute path.
func ExportPNG(img *image.RGBA, name string, size int, outDir string) (string, error) {
	if outDir == "" {
		outDir = os.TempDir()
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}
	name = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))

	var out image.Image = img
	if size > 0 && (img.Bounds().Dx() != size || img.Bounds().Dy() != size) {
		dst := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.NearestNeighbor.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
		out = dst
		name = fmt.Sprintf("%s_%dpx", name, size)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return "", fmt.Errorf("编码 PNG 失败: %w", err)
	}
	path := filepath.Join(outDir, name+".png")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("写出文件失败: %w", err)
	}
	return filepath.Abs(path)
}

// EncodePNG encodes img as PNG bytes, for the clipboard.
func EncodePNG(img *image.RGBA) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("编码 PNG 失败: %w", err)
	}
	return buf.Bytes(), nil
}

// PNG renders img at size (0 keeps its own size, nearest-neighbor scaling
// otherwise) and returns the PNG bytes with the file name it suggests,
// with a "_Npx" suffix when resized.
func PNG(img *image.RGBA, name string, size int) ([]byte, string, error) {
	name = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	var out image.Image = img
	if size > 0 && (img.Bounds().Dx() != size || img.Bounds().Dy() != size) {
		dst := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.NearestNeighbor.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
		out = dst
		name = fmt.Sprintf("%s_%dpx", name, size)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, "", fmt.Errorf("编码 PNG 失败: %w", err)
	}
	return buf.Bytes(), name + ".png", nil
}
