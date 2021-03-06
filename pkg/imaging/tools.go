package imaging

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"sync"
)

// OpaqueBounds returns a bounding box for the given image
func OpaqueBounds(img image.Image, threshold uint8) image.Rectangle {
	src := newScanner(img)
	out := image.Rectangle{}
	first := true
	dst := image.NewNRGBA(image.Rect(0, 0, src.w, src.h))
	var mu sync.Mutex
	parallel(0, src.h, func(ys <-chan int) {
		for y := range ys {
			i := y * dst.Stride
			src.scan(0, y, src.w, y+1, dst.Pix[i:i+src.w*4])
			for x := 0; x < src.w; x++ {
				a := dst.Pix[i+3]
				i += 4
				if a > threshold {
					mu.Lock()
					if first {
						out.Min = image.Point{x, y}
						out.Max = out.Min
						first = false
					}
					mu.Unlock()
				}
				if a > threshold {
					mu.Lock()
					if !first {
						out.Min.X = int(math.Min(float64(x), float64(out.Min.X)))
						out.Min.Y = int(math.Min(float64(y), float64(out.Min.Y)))
						out.Max.X = int(math.Max(float64(x), float64(out.Max.X)))
						out.Max.Y = int(math.Max(float64(y), float64(out.Max.Y)))
					}
					mu.Unlock()
				}
			}
		}
	})
	return out
}

// OpaquePolygon returns a bounding polygon for the given image
func OpaquePolygon(img image.Image, n int, threshold uint8) (out []image.Point) {
	bounds := OpaqueBounds(img, threshold)
	src := newScanner(img)
	dst := image.NewNRGBA(image.Rect(0, 0, src.w, src.h))
	out = make([]image.Point, 2*n)
	var (
		pointsLeft  = out[:n]
		pointsRight = out[n : 2*n]
	)
	yStep := float64(bounds.Dy()-1) / float64(n-1)
	w := bounds.Dx()
	// Left, Right
	parallel(0, n, func(ks <-chan int) {
		for k := range ks {
			y := int(math.Floor(float64(bounds.Min.Y) + float64(k)*yStep))
			i := (y - bounds.Min.Y) * 4 * w
			src.scan(bounds.Min.X, y, bounds.Max.X, y+1, dst.Pix[i:i+w*4])
			foundLeft := false
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				i += 4
				a := dst.Pix[i+3]
				if a <= threshold {
					continue
				}
				pointsRight[k].X = x
				pointsRight[k].Y = y
				if foundLeft {
					continue
				}
				foundLeft = true
				pointsLeft[n-1-k].X = x
				pointsLeft[n-1-k].Y = y
			}
		}
	})
	// h := bounds.Dy()
	// xMinStep := 3.0
	return out
}

// OpaqueArea returns the opaque area (in pixels) of the image
func OpaqueArea(img image.Image, threshold uint8) int {
	var (
		src             = newScanner(img)
		dst             = image.NewNRGBA(image.Rect(0, 0, src.w, src.h))
		numPixelsOpaque = 0
	)
	parallel(0, src.h, func(ys <-chan int) {
		for y := range ys {
			i := y * 4
			src.scan(0, y, src.w, y+1, dst.Pix[i:i+src.w*4])
			for x := 0; x < src.w; x++ {
				i += 4
				if a := dst.Pix[i+3]; a > threshold {
					numPixelsOpaque++
				}
			}
		}
	})
	return numPixelsOpaque
}

// New creates a new image with the specified width and height, and fills it with the specified color.
func New(width, height int, fillColor color.Color) *image.NRGBA {
	if width <= 0 || height <= 0 {
		return &image.NRGBA{}
	}

	c := color.NRGBAModel.Convert(fillColor).(color.NRGBA)
	if (c == color.NRGBA{0, 0, 0, 0}) {
		return image.NewNRGBA(image.Rect(0, 0, width, height))
	}

	return &image.NRGBA{
		Pix:    bytes.Repeat([]byte{c.R, c.G, c.B, c.A}, width*height),
		Stride: 4 * width,
		Rect:   image.Rect(0, 0, width, height),
	}
}

// Clone returns a copy of the given image.
func Clone(img image.Image) *image.NRGBA {
	src := newScanner(img)
	dst := image.NewNRGBA(image.Rect(0, 0, src.w, src.h))
	size := src.w * 4
	parallel(0, src.h, func(ys <-chan int) {
		for y := range ys {
			i := y * dst.Stride
			src.scan(0, y, src.w, y+1, dst.Pix[i:i+size])
		}
	})
	return dst
}

// Anchor is the anchor point for image alignment.
type Anchor int

// Anchor point positions.
const (
	Center Anchor = iota
	TopLeft
	Top
	TopRight
	Left
	Right
	BottomLeft
	Bottom
	BottomRight
)

func AnchorPoint(i image.Image, anchor Anchor) image.Point {
	return anchorPt(i.Bounds(), 0, 0, anchor)
}

func anchorPt(b image.Rectangle, w, h int, anchor Anchor) image.Point {
	var x, y int
	switch anchor {
	case TopLeft:
		x = b.Min.X
		y = b.Min.Y
	case Top:
		x = b.Min.X + (b.Dx()-w)/2
		y = b.Min.Y
	case TopRight:
		x = b.Max.X - w
		y = b.Min.Y
	case Left:
		x = b.Min.X
		y = b.Min.Y + (b.Dy()-h)/2
	case Right:
		x = b.Max.X - w
		y = b.Min.Y + (b.Dy()-h)/2
	case BottomLeft:
		x = b.Min.X
		y = b.Max.Y - h
	case Bottom:
		x = b.Min.X + (b.Dx()-w)/2
		y = b.Max.Y - h
	case BottomRight:
		x = b.Max.X - w
		y = b.Max.Y - h
	default:
		x = b.Min.X + (b.Dx()-w)/2
		y = b.Min.Y + (b.Dy()-h)/2
	}
	return image.Pt(x, y)
}

// Crop cuts out a rectangular region with the specified bounds
// from the image and returns the cropped image.
func Crop(img image.Image, rect image.Rectangle) *image.NRGBA {
	r := rect.Intersect(img.Bounds()).Sub(img.Bounds().Min)
	if r.Empty() {
		return &image.NRGBA{}
	}
	src := newScanner(img)
	dst := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	rowSize := r.Dx() * 4
	parallel(r.Min.Y, r.Max.Y, func(ys <-chan int) {
		for y := range ys {
			i := (y - r.Min.Y) * dst.Stride
			src.scan(r.Min.X, y, r.Max.X, y+1, dst.Pix[i:i+rowSize])
		}
	})
	return dst
}

// CropAnchor cuts out a rectangular region with the specified size
// from the image using the specified anchor point and returns the cropped image.
func CropAnchor(img image.Image, width, height int, anchor Anchor) *image.NRGBA {
	srcBounds := img.Bounds()
	pt := anchorPt(srcBounds, width, height, anchor)
	r := image.Rect(0, 0, width, height).Add(pt)
	b := srcBounds.Intersect(r)
	return Crop(img, b)
}

// CropCenter cuts out a rectangular region with the specified size
// from the center of the image and returns the cropped image.
func CropCenter(img image.Image, width, height int) *image.NRGBA {
	return CropAnchor(img, width, height, Center)
}

// Paste pastes the img image to the background image at the specified position and returns the combined image.
func Paste(background, img image.Image, pos image.Point) *image.NRGBA {
	dst := Clone(background)
	pos = pos.Sub(background.Bounds().Min)
	pasteRect := image.Rectangle{Min: pos, Max: pos.Add(img.Bounds().Size())}
	interRect := pasteRect.Intersect(dst.Bounds())
	if interRect.Empty() {
		return dst
	}
	src := newScanner(img)
	parallel(interRect.Min.Y, interRect.Max.Y, func(ys <-chan int) {
		for y := range ys {
			x1 := interRect.Min.X - pasteRect.Min.X
			x2 := interRect.Max.X - pasteRect.Min.X
			y1 := y - pasteRect.Min.Y
			y2 := y1 + 1
			i1 := y*dst.Stride + interRect.Min.X*4
			i2 := i1 + interRect.Dx()*4
			src.scan(x1, y1, x2, y2, dst.Pix[i1:i2])
		}
	})
	return dst
}

func OverlayOnCanvas(w, h int, bgColor color.Color, images []struct {
	Image image.Image
	Point image.Point
}) *image.NRGBA {
	dst := New(w, h, bgColor)
	for imgIndex := range images {
		img := images[imgIndex].Image
		pos := images[imgIndex].Point.Sub(dst.Bounds().Min)
		pasteRect := image.Rectangle{Min: pos, Max: pos.Add(img.Bounds().Size())}
		interRect := pasteRect.Intersect(dst.Bounds())
		if interRect.Empty() {
			continue
		}
		src := newScanner(img)
		parallel(interRect.Min.Y, interRect.Max.Y, func(ys <-chan int) {
			scanLine := make([]uint8, interRect.Dx()*4)
			for y := range ys {
				x1 := interRect.Min.X - pasteRect.Min.X
				x2 := interRect.Max.X - pasteRect.Min.X
				y1 := y - pasteRect.Min.Y
				y2 := y1 + 1
				src.scan(x1, y1, x2, y2, scanLine)
				i := y*dst.Stride + interRect.Min.X*4
				j := 0
				for x := interRect.Min.X; x < interRect.Max.X; x++ {
					r1 := float64(dst.Pix[i+0])
					g1 := float64(dst.Pix[i+1])
					b1 := float64(dst.Pix[i+2])
					a1 := float64(dst.Pix[i+3])

					r2 := float64(scanLine[j+0])
					g2 := float64(scanLine[j+1])
					b2 := float64(scanLine[j+2])
					a2 := float64(scanLine[j+3])

					coef2 := a2 / 255
					coef1 := (1 - coef2) * a1 / 255
					coefSum := coef1 + coef2
					coef1 /= coefSum
					coef2 /= coefSum

					dst.Pix[i+0] = uint8(r1*coef1 + r2*coef2)
					dst.Pix[i+1] = uint8(g1*coef1 + g2*coef2)
					dst.Pix[i+2] = uint8(b1*coef1 + b2*coef2)
					dst.Pix[i+3] = uint8(math.Min(a1+a2*(255-a1)/255, 255))

					i += 4
					j += 4
				}
			}
		})
	}
	return dst
}

// PasteCenter pastes the img image to the center of the background image and returns the combined image.
func PasteCenter(background, img image.Image) *image.NRGBA {
	bgBounds := background.Bounds()
	bgW := bgBounds.Dx()
	bgH := bgBounds.Dy()
	bgMinX := bgBounds.Min.X
	bgMinY := bgBounds.Min.Y

	centerX := bgMinX + bgW/2
	centerY := bgMinY + bgH/2

	x0 := centerX - img.Bounds().Dx()/2
	y0 := centerY - img.Bounds().Dy()/2

	return Paste(background, img, image.Pt(x0, y0))
}

// Overlay draws the img image over the background image at given position
// and returns the combined image. Opacity parameter is the opacity of the img
// image layer, used to compose the images, it must be from 0.0 to 1.0.
//
// Usage examples:
//
//	// Draw spriteImage over backgroundImage at the given position (x=50, y=50).
//	dstImage := imaging.Overlay(backgroundImage, spriteImage, image.Pt(50, 50), 1.0)
//
//	// Blend two opaque images of the same size.
//	dstImage := imaging.Overlay(imageOne, imageTwo, image.Pt(0, 0), 0.5)
//
func Overlay(background, img image.Image, pos image.Point, opacity float64) *image.NRGBA {
	opacity = math.Min(math.Max(opacity, 0.0), 1.0) // Ensure 0.0 <= opacity <= 1.0.
	dst := Clone(background)
	pos = pos.Sub(background.Bounds().Min)
	pasteRect := image.Rectangle{Min: pos, Max: pos.Add(img.Bounds().Size())}
	interRect := pasteRect.Intersect(dst.Bounds())
	if interRect.Empty() {
		return dst
	}
	src := newScanner(img)
	parallel(interRect.Min.Y, interRect.Max.Y, func(ys <-chan int) {
		scanLine := make([]uint8, interRect.Dx()*4)
		for y := range ys {
			x1 := interRect.Min.X - pasteRect.Min.X
			x2 := interRect.Max.X - pasteRect.Min.X
			y1 := y - pasteRect.Min.Y
			y2 := y1 + 1
			src.scan(x1, y1, x2, y2, scanLine)
			i := y*dst.Stride + interRect.Min.X*4
			j := 0
			for x := interRect.Min.X; x < interRect.Max.X; x++ {
				r1 := float64(dst.Pix[i+0])
				g1 := float64(dst.Pix[i+1])
				b1 := float64(dst.Pix[i+2])
				a1 := float64(dst.Pix[i+3])

				r2 := float64(scanLine[j+0])
				g2 := float64(scanLine[j+1])
				b2 := float64(scanLine[j+2])
				a2 := float64(scanLine[j+3])

				coef2 := opacity * a2 / 255
				coef1 := (1 - coef2) * a1 / 255
				coefSum := coef1 + coef2
				coef1 /= coefSum
				coef2 /= coefSum

				dst.Pix[i+0] = uint8(r1*coef1 + r2*coef2)
				dst.Pix[i+1] = uint8(g1*coef1 + g2*coef2)
				dst.Pix[i+2] = uint8(b1*coef1 + b2*coef2)
				dst.Pix[i+3] = uint8(math.Min(a1+a2*opacity*(255-a1)/255, 255))

				i += 4
				j += 4
			}
		}
	})
	return dst
}

type OverlayOp func(r1, g1, b1, a1, r2, g2, b2, a2 float64) (r, g, b, a float64)

func OpBlend(opacity float64) OverlayOp {
	return func(r1, g1, b1, a1, r2, g2, b2, a2 float64) (r, g, b, a float64) {
		coef2 := opacity * a2 / 255
		coef1 := (1 - coef2) * a1 / 255
		coefSum := coef1 + coef2
		coef1 /= coefSum
		coef2 /= coefSum

		r = r1*coef1 + r2*coef2
		g = g1*coef1 + g2*coef2
		b = b1*coef1 + b2*coef2
		a = math.Min(a1+a2*opacity*(255-a1)/255, 255)
		return
	}
}

func OpPlus(r1, g1, b1, a1, r2, g2, b2, a2 float64) (r, g, b, a float64) {
	r = math.Min(r1+r2, 255)
	g = math.Min(g1+g2, 255)
	b = math.Min(b1+b2, 255)
	a = math.Min(a1+a2*((255-a1)/255), 255)
	return
}

func OpMax(r1, g1, b1, a1, r2, g2, b2, a2 float64) (r, g, b, a float64) {
	r = math.Max(r1, r2)
	g = math.Max(g1, g2)
	b = math.Max(b1, b2)
	a = math.Min(a1+a2*((255-a1)/255), 255)
	return
}

func OpReplace(r1, g1, b1, a1, r2, g2, b2, a2 float64) (r, g, b, a float64) {
	r = r2
	g = g2
	b = b2
	a = a2
	return
}

func OpReplaceAlpha(r1, g1, b1, a1, r2, g2, b2, a2 float64) (r, g, b, a float64) {
	r = r1
	g = g1
	b = b1
	a = a2
	return
}

func OpMinAlpha(r1, g1, b1, a1, r2, g2, b2, a2 float64) (r, g, b, a float64) {
	r = r1
	g = g1
	b = b1
	a = math.Min(a1, a2)
	return
}

func OpMaxAlpha(r1, g1, b1, a1, r2, g2, b2, a2 float64) (r, g, b, a float64) {
	r = 255.0
	g = 255.0
	b = 255.0
	a = math.Max(a1, a2)
	return
}

func OpIgnore(r1, g1, b1, a1, r2, g2, b2, a2 float64) (r, g, b, a float64) {
	r = r1
	g = g1
	b = b1
	a = a1
	return
}

func OpLighten(r1, g1, b1, a1, r2, g2, b2, a2 float64) (r, g, b, a float64) {
	max := math.Max(r1, math.Max(g1, b1))
	min := math.Min(r1, math.Min(g1, b1))
	l1 := (max + min) / 2
	max = math.Max(r2, math.Max(g2, b2))
	min = math.Min(r2, math.Min(g2, b2))
	l2 := (max + min) / 2
	if l2 < l1 {
		r = r1
		g = g1
		b = b1
	} else {
		r = r2
		g = g2
		b = b2
	}
	a = math.Min(a1+a2*((255-a1)/255), 255)
	return
}

func OverlayWithOp(background, img image.Image, pos image.Point, op OverlayOp) *image.NRGBA {
	dst := Clone(background)
	pos = pos.Sub(background.Bounds().Min)
	pasteRect := image.Rectangle{Min: pos, Max: pos.Add(img.Bounds().Size())}
	interRect := pasteRect.Intersect(dst.Bounds())
	if interRect.Empty() {
		return dst
	}
	src := newScanner(img)
	parallel(interRect.Min.Y, interRect.Max.Y, func(ys <-chan int) {
		scanLine := make([]uint8, interRect.Dx()*4)
		var r, g, b, a float64
		for y := range ys {
			x1 := interRect.Min.X - pasteRect.Min.X
			x2 := interRect.Max.X - pasteRect.Min.X
			y1 := y - pasteRect.Min.Y
			y2 := y1 + 1
			src.scan(x1, y1, x2, y2, scanLine)
			i := y*dst.Stride + interRect.Min.X*4
			j := 0
			for x := interRect.Min.X; x < interRect.Max.X; x++ {
				r1 := float64(dst.Pix[i+0])
				g1 := float64(dst.Pix[i+1])
				b1 := float64(dst.Pix[i+2])
				a1 := float64(dst.Pix[i+3])

				r2 := float64(scanLine[j+0])
				g2 := float64(scanLine[j+1])
				b2 := float64(scanLine[j+2])
				a2 := float64(scanLine[j+3])

				r, g, b, a = op(r1, g1, b1, a1, r2, g2, b2, a2)
				dst.Pix[i+0] = uint8(r)
				dst.Pix[i+1] = uint8(g)
				dst.Pix[i+2] = uint8(b)
				dst.Pix[i+3] = uint8(a)

				i += 4
				j += 4
			}
		}
	})
	return dst
}

// OverlayCenter overlays the img image to the center of the background image and
// returns the combined image. Opacity parameter is the opacity of the img
// image layer, used to compose the images, it must be from 0.0 to 1.0.
func OverlayCenter(background, img image.Image, opacity float64) *image.NRGBA {
	bgBounds := background.Bounds()
	bgW := bgBounds.Dx()
	bgH := bgBounds.Dy()
	bgMinX := bgBounds.Min.X
	bgMinY := bgBounds.Min.Y

	centerX := bgMinX + bgW/2
	centerY := bgMinY + bgH/2

	x0 := centerX - img.Bounds().Dx()/2
	y0 := centerY - img.Bounds().Dy()/2

	return Overlay(background, img, image.Point{x0, y0}, opacity)
}
