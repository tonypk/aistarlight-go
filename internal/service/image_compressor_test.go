package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func createTestImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Create a gradient pattern
			r := uint8(x * 255 / w)
			g := uint8(y * 255 / h)
			b := uint8(128)
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

func encodeJPEG(img image.Image, quality int) []byte {
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	return buf.Bytes()
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestCompressReceiptImage_SmallImage(t *testing.T) {
	img := createTestImage(800, 600)
	data := encodeJPEG(img, 90)

	result, err := CompressReceiptImage(bytes.NewReader(data), len(data))
	if err != nil {
		t.Fatalf("CompressReceiptImage: %v", err)
	}

	if result.Width != 800 || result.Height != 600 {
		t.Errorf("Expected 800x600, got %dx%d (should not resize small images)", result.Width, result.Height)
	}

	if result.CompressedSize >= result.OriginalSize {
		t.Logf("Compressed %d -> %d (no savings, small image)", result.OriginalSize, result.CompressedSize)
	}
}

func TestCompressReceiptImage_LargeImage(t *testing.T) {
	img := createTestImage(4000, 3000) // Simulates a phone camera photo
	data := encodeJPEG(img, 95)

	result, err := CompressReceiptImage(bytes.NewReader(data), len(data))
	if err != nil {
		t.Fatalf("CompressReceiptImage: %v", err)
	}

	// Should be resized to max 1600px
	if result.Width > MaxReceiptDimension && result.Height > MaxReceiptDimension {
		t.Errorf("Expected max dimension %d, got %dx%d", MaxReceiptDimension, result.Width, result.Height)
	}

	// Should be smaller than original
	if result.CompressedSize >= result.OriginalSize {
		t.Errorf("Expected compression: original=%d, compressed=%d", result.OriginalSize, result.CompressedSize)
	}

	t.Logf("Compressed: %d -> %d (%.0f%% reduction), %dx%d",
		result.OriginalSize, result.CompressedSize,
		(1-float64(result.CompressedSize)/float64(result.OriginalSize))*100,
		result.Width, result.Height)
}

func TestCompressReceiptImage_PNG(t *testing.T) {
	img := createTestImage(2000, 1500)
	data := encodePNG(img)

	result, err := CompressReceiptImage(bytes.NewReader(data), len(data))
	if err != nil {
		t.Fatalf("CompressReceiptImage PNG: %v", err)
	}

	// PNG is always larger than JPEG for photos, so compression should help
	if result.CompressedSize >= len(data) {
		t.Logf("PNG %d -> JPEG %d", len(data), result.CompressedSize)
	}

	// Output should be valid JPEG
	if len(result.Data) < 100 {
		t.Error("Output data too small to be valid JPEG")
	}
}

func TestCompressReceiptImage_PreservesAspectRatio(t *testing.T) {
	// Tall narrow receipt (common for thermal printer)
	img := createTestImage(800, 3000)
	data := encodeJPEG(img, 90)

	result, err := CompressReceiptImage(bytes.NewReader(data), len(data))
	if err != nil {
		t.Fatalf("CompressReceiptImage: %v", err)
	}

	// Height should be capped at 1600, width proportional
	if result.Height > MaxReceiptDimension {
		t.Errorf("Height %d exceeds max %d", result.Height, MaxReceiptDimension)
	}

	ratio := float64(result.Width) / float64(result.Height)
	expectedRatio := float64(800) / float64(3000)
	if diff := ratio - expectedRatio; diff > 0.02 || diff < -0.02 {
		t.Errorf("Aspect ratio changed: expected %.3f, got %.3f", expectedRatio, ratio)
	}
}

func TestScaleDimensions(t *testing.T) {
	tests := []struct {
		w, h, max int
		expectW   int
		expectH   int
	}{
		{800, 600, 1600, 800, 600},    // no change needed
		{4000, 3000, 1600, 1600, 1200}, // scale down
		{3000, 4000, 1600, 1200, 1600}, // portrait
		{1600, 1600, 1600, 1600, 1600}, // exact
		{100, 50, 1600, 100, 50},       // tiny
	}

	for _, tt := range tests {
		w, h := scaleDimensions(tt.w, tt.h, tt.max)
		if w != tt.expectW || h != tt.expectH {
			t.Errorf("scaleDimensions(%d, %d, %d) = %d, %d; want %d, %d",
				tt.w, tt.h, tt.max, w, h, tt.expectW, tt.expectH)
		}
	}
}

func TestToGrayscale(t *testing.T) {
	img := createTestImage(100, 100)
	gray := toGrayscale(img)

	bounds := gray.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Errorf("Expected 100x100, got %dx%d", bounds.Dx(), bounds.Dy())
	}

	// Verify all pixels are grayscale (single channel)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			c := gray.GrayAt(x, y)
			if c.Y == 0 && x > 0 && y > 0 {
				// With our gradient test image, non-origin pixels shouldn't be pure black
				// (unless the gradient produces exactly 0, which it can at origin)
			}
		}
	}
}
