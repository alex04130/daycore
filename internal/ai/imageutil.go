package ai

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/jpeg"
	"image/png"

	xdraw "golang.org/x/image/draw"
)

// decodeImage decodes a base64-encoded image (PNG/JPEG/GIF) into an image.Image.
func decodeImage(b64 string) (image.Image, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}

// cropAndScale crops a normalized region [x,y,w,h] (each 0..1) of img and scales
// it by `scale` (clamped to [1,4]) using a high-quality kernel. This is the
// "zoom in" the model requests via the zoom_image tool to read small details.
func cropAndScale(img image.Image, nx, ny, nw, nh, scale float64) image.Image {
	b := img.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())

	nx = clamp01(nx)
	ny = clamp01(ny)
	if nw <= 0 || nw > 1 {
		nw = 1
	}
	if nh <= 0 || nh > 1 {
		nh = 1
	}
	x0 := b.Min.X + int(nx*w)
	y0 := b.Min.Y + int(ny*h)
	x1 := x0 + int(nw*w)
	y1 := y0 + int(nh*h)
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	if x1 <= x0 || y1 <= y0 {
		return img
	}
	rect := image.Rect(x0, y0, x1, y1)
	cropped := subImage(img, rect)

	if scale < 1 {
		scale = 1
	}
	if scale > 4 {
		scale = 4
	}
	dw := int(float64(rect.Dx()) * scale)
	dh := int(float64(rect.Dy()) * scale)
	if dw < 1 || dh < 1 {
		return cropped
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), xdraw.Over, nil)
	return dst
}

// subImage returns the rect of img, using the cheap SubImage path when available.
func subImage(img image.Image, rect image.Rectangle) image.Image {
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := img.(subImager); ok {
		return si.SubImage(rect)
	}
	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	xdraw.Copy(dst, image.Point{}, img, rect, xdraw.Src, nil)
	return dst
}

// encodePNG re-encodes an image to base64 PNG (lossless, good for text/OCR).
func encodePNG(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// keep jpeg import referenced (registers the JPEG decoder via init).
var _ = jpeg.DefaultQuality
