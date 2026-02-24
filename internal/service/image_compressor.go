package service

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"io"
	"math"

	_ "golang.org/x/image/bmp"  // register BMP decoder
	_ "golang.org/x/image/tiff" // register TIFF decoder
	_ "golang.org/x/image/webp" // register WebP decoder
)

const (
	// MaxReceiptDimension is the maximum width/height for a receipt image.
	MaxReceiptDimension = 1600
	// ReceiptJPEGQuality is the JPEG output quality (1-100).
	ReceiptJPEGQuality = 60
	// MaxReceiptFileSize is the target max compressed size (500KB).
	MaxReceiptFileSize = 500 * 1024
)

// CompressResult holds the compression output.
type CompressImageResult struct {
	Data           []byte
	Width          int
	Height         int
	OriginalSize   int
	CompressedSize int
}

// CompressReceiptImage reads an image, resizes it, converts to grayscale, and outputs JPEG.
// This is a server-side safety net — browser should already compress before uploading.
func CompressReceiptImage(r io.Reader, originalSize int) (*CompressImageResult, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()

	// Calculate new dimensions
	newW, newH := scaleDimensions(origW, origH, MaxReceiptDimension)

	// Resize using nearest-neighbor (fast, good enough for receipts)
	resized := resizeNearestNeighbor(img, newW, newH)

	// Convert to grayscale
	gray := toGrayscale(resized)

	// Encode as JPEG
	quality := ReceiptJPEGQuality
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, gray, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode JPEG: %w", err)
	}

	// If still too large, reduce quality progressively
	for buf.Len() > MaxReceiptFileSize && quality > 30 {
		quality -= 5
		buf.Reset()
		if err := jpeg.Encode(&buf, gray, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("encode JPEG at quality %d: %w", quality, err)
		}
	}

	// If STILL too large, resize further
	if buf.Len() > MaxReceiptFileSize && newW > 800 {
		smallW, smallH := scaleDimensions(origW, origH, 800)
		smallImg := resizeNearestNeighbor(img, smallW, smallH)
		smallGray := toGrayscale(smallImg)
		buf.Reset()
		if err := jpeg.Encode(&buf, smallGray, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("encode smaller JPEG: %w", err)
		}
		newW, newH = smallW, smallH
	}

	return &CompressImageResult{
		Data:           buf.Bytes(),
		Width:          newW,
		Height:         newH,
		OriginalSize:   originalSize,
		CompressedSize: buf.Len(),
	}, nil
}

func scaleDimensions(w, h, maxDim int) (int, int) {
	if w <= maxDim && h <= maxDim {
		return w, h
	}
	ratio := math.Min(float64(maxDim)/float64(w), float64(maxDim)/float64(h))
	return int(math.Round(float64(w) * ratio)), int(math.Round(float64(h) * ratio))
}

// resizeNearestNeighbor does fast nearest-neighbor scaling.
// For receipt OCR, this is perfectly adequate and avoids external dependencies.
func resizeNearestNeighbor(src image.Image, newW, newH int) *image.RGBA {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))

	for y := 0; y < newH; y++ {
		srcY := bounds.Min.Y + y*srcH/newH
		for x := 0; x < newW; x++ {
			srcX := bounds.Min.X + x*srcW/newW
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst
}

// toGrayscale converts an image to grayscale.
func toGrayscale(src image.Image) *image.Gray {
	bounds := src.Bounds()
	gray := image.NewGray(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			// Luminosity: 0.299R + 0.587G + 0.114B
			lum := (299*r + 587*g + 114*b) / 1000
			gray.SetGray(x, y, color.Gray{Y: uint8(lum >> 8)})
		}
	}

	return gray
}
