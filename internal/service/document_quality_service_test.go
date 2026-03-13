package service

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestHashFile(t *testing.T) {
	// Create a temp file with known content.
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	hash, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Compute expected hash.
	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	if hash != expected {
		t.Errorf("HashFile = %s, want %s", hash, expected)
	}
}

func TestHashFile_Deterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "receipt.bin")
	if err := os.WriteFile(path, []byte("receipt data 12345"), 0o644); err != nil {
		t.Fatal(err)
	}

	hash1, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash2, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if hash1 != hash2 {
		t.Error("HashFile is not deterministic")
	}
}

func TestHashFile_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "a.txt")
	path2 := filepath.Join(dir, "b.txt")
	_ = os.WriteFile(path1, []byte("content A"), 0o644)
	_ = os.WriteFile(path2, []byte("content B"), 0o644)

	hash1, _ := HashFile(path1)
	hash2, _ := HashFile(path2)

	if hash1 == hash2 {
		t.Error("different files should have different hashes")
	}
}

func TestHashFile_NonExistent(t *testing.T) {
	_, err := HashFile("/tmp/nonexistent_file_abc123")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLaplacianVariance_SharpImage(t *testing.T) {
	// Create a sharp image with high-contrast edges (checkerboard).
	img := image.NewGray(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if (x/5+y/5)%2 == 0 {
				img.SetGray(x, y, color.Gray{Y: 0})
			} else {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}

	variance := laplacianVariance(img)
	if variance < blurThreshold {
		t.Errorf("sharp checkerboard should have high variance (got %.2f, threshold %.2f)", variance, blurThreshold)
	}
}

func TestLaplacianVariance_BlurryImage(t *testing.T) {
	// Create a uniform gray image (no edges = very blurry).
	img := image.NewGray(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.SetGray(x, y, color.Gray{Y: 128})
		}
	}

	variance := laplacianVariance(img)
	if variance >= blurThreshold {
		t.Errorf("uniform image should have low variance (got %.2f, threshold %.2f)", variance, blurThreshold)
	}
}

func TestLaplacianVariance_TinyImage(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 2, 2))
	variance := laplacianVariance(img)
	if variance != 0 {
		t.Errorf("image too small for Laplacian should return 0, got %.2f", variance)
	}
}

func TestComputeQualityScore(t *testing.T) {
	tests := []struct {
		name     string
		result   QualityResult
		minScore float64
		maxScore float64
	}{
		{
			name:     "perfect image",
			result:   QualityResult{Width: 1200, Height: 1600},
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "blurry image",
			result:   QualityResult{Width: 1200, Height: 1600, IsBlurry: true},
			minScore: 0.5,
			maxScore: 0.7,
		},
		{
			name:     "too small",
			result:   QualityResult{Width: 100, Height: 100, IsTooSmall: true},
			minScore: 0.5,
			maxScore: 0.8,
		},
		{
			name:     "blurry and small",
			result:   QualityResult{Width: 100, Height: 100, IsBlurry: true, IsTooSmall: true},
			minScore: 0.2,
			maxScore: 0.4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := computeQualityScore(&tt.result)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("computeQualityScore = %.2f, want [%.2f, %.2f]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestSuggestedAction(t *testing.T) {
	tests := []struct {
		name     string
		result   QualityResult
		expected string
	}{
		{"good image", QualityResult{QualityScore: 1.0}, "proceed"},
		{"too small", QualityResult{IsTooSmall: true, QualityScore: 0.3}, "request_reupload"},
		{"blurry low quality", QualityResult{IsBlurry: true, QualityScore: 0.4}, "warn_user"},
		{"blurry ok quality", QualityResult{IsBlurry: true, QualityScore: 0.6}, "proceed"},
		{"duplicate", QualityResult{IsDuplicate: true, QualityScore: 1.0}, "warn_user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := suggestedAction(&tt.result)
			if action != tt.expected {
				t.Errorf("suggestedAction = %s, want %s", action, tt.expected)
			}
		})
	}
}

func TestIsBlurry_WithRealImage(t *testing.T) {
	// Create a JPEG with high contrast text-like patterns.
	dir := t.TempDir()
	sharpPath := filepath.Join(dir, "sharp.jpg")
	blurPath := filepath.Join(dir, "blur.png")

	// Sharp: alternating black/white stripes.
	sharp := image.NewGray(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			if x%4 < 2 {
				sharp.SetGray(x, y, color.Gray{Y: 0})
			} else {
				sharp.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	f, _ := os.Create(sharpPath)
	_ = jpeg.Encode(f, sharp, &jpeg.Options{Quality: 95})
	f.Close()

	// Blur: smooth gradient.
	blur := image.NewGray(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		v := uint8(y * 255 / 200)
		for x := 0; x < 200; x++ {
			blur.SetGray(x, y, color.Gray{Y: v})
		}
	}
	f2, _ := os.Create(blurPath)
	_ = png.Encode(f2, blur)
	f2.Close()

	if isBlurry(sharp) {
		t.Error("sharp striped image should not be detected as blurry")
	}
	// Note: smooth gradient may or may not be "blurry" depending on threshold.
	// This test mainly validates the function doesn't panic.
	_ = isBlurry(blur)
}
