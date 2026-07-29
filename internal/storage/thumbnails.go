package storage

import (
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"
)

func (s *Storage) GenerateThumbnail(srcPath, id string) error {
	dstPath := s.ThumbnailPath(id)
	if _, err := os.Stat(dstPath); err == nil {
		return nil
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}

	defer f.Close() //nolint:errcheck

	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	thumb := scaleImage(src, 140)
	out, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create thumbnail: %w", err)
	}

	defer out.Close() //nolint:errcheck

	if err = jpeg.Encode(out, thumb, &jpeg.Options{Quality: 75}); err != nil {
		return fmt.Errorf("encode thumbnail: %w", err)
	}

	return nil
}

func scaleImage(src image.Image, maxWidth int) image.Image {
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w <= maxWidth {
		return src
	}

	newH := h * maxWidth / w
	dst := image.NewRGBA(image.Rect(0, 0, maxWidth, newH))
	for y := range newH {
		for x := range maxWidth {
			srcX := x * w / maxWidth
			srcY := y * h / newH
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	return dst
}
