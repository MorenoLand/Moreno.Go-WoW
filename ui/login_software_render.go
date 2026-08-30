package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	xdraw "golang.org/x/image/draw"
)

// SoftwareRenderLogin runs the login render pipeline used in tests and returns the resulting RGBA.
func SoftwareRenderLogin(glue, frame, assets string) (*image.RGBA, error) {
	root, err := os.MkdirTemp("", "wow-ui-root-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(root)

	stageTree := func(rel, source string) error {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(source, e.Name()))
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644); err != nil {
				return err
			}
		}
		return nil
	}
	if err := stageTree("Interface/GlueXML", glue); err != nil {
		return nil, err
	}
	if err := stageTree("Interface/FrameXML", frame); err != nil {
		return nil, err
	}

	const screenWidth, screenHeight = 960, 640
	rt := NewRuntime(hostScreen{w: screenWidth, h: screenHeight})
	defer rt.Close()
	loader := NewLoader(root, rt)
	if err := loader.LoadTOC(`Interface\GlueXML\GlueXML.toc`, nil); err != nil {
		return nil, fmt.Errorf("LoadTOC: %v", err)
	}

	if errs := rt.ScriptErrors(); len(errs) != 0 {
		return nil, fmt.Errorf("%d script errors before rendering", len(errs))
	}

	rt.Execute("GlueParent_OnEvent('SET_GLUE_SCREEN', 'login')", "@render.lua")

	glueParent := rt.widgets["GlueParent"]
	if glueParent == nil {
		return nil, fmt.Errorf("GlueParent missing")
	}
	login := rt.widgets["AccountLogin"]
	if login == nil {
		return nil, fmt.Errorf("AccountLogin missing")
	}

	fontData, err := os.ReadFile(filepath.Join(assets, "Fonts", "FRIZQT__.TTF"))
	if err != nil {
		return nil, fmt.Errorf("font: %v", err)
	}
	fontObj, err := opentype.Parse(fontData)
	if err != nil {
		return nil, fmt.Errorf("font parse: %v", err)
	}
	face, err := opentype.NewFace(fontObj, &opentype.FaceOptions{Size: 16, DPI: 72})
	if err != nil {
		return nil, fmt.Errorf("font face: %v", err)
	}
	defer face.Close()

	canvas := image.NewRGBA(image.Rect(0, 0, screenWidth, screenHeight))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 8, G: 10, B: 14, A: 255}}, image.Point{}, draw.Src)

	assetLoader := NewLoader(assets, rt)
	cache := map[string]image.Image{}

	screen := Rect{X0: 0, Y0: 0, X1: screenWidth, Y1: screenHeight}

	var paint func(w *widget, parent Rect)
	paint = func(w *widget, parent Rect) {
		rect := ResolveRect(w, parent)
		switch w.kind {
		case kindTexture:
			if w.textureFile != "" {
				img, ok := cache[w.textureFile]
				if !ok {
					data, err := assetLoader.ReadAsset(w.textureFile)
					if err == nil {
						decoded, derr := DecodeBLP(data)
						if derr == nil {
							img = decoded
						}
					}
					cache[w.textureFile] = img
				}
				if img != nil {
					tc := [4]float64{w.texCoordL, w.texCoordR, w.texCoordT, w.texCoordB}
					if tc[0] == 0 && tc[1] == 0 && tc[2] == 0 && tc[3] == 0 {
						tc = [4]float64{0, 1, 0, 1}
					}
					drawSub(canvas, img, rect, screenHeight, tc)
				}
			}
		case kindFontString:
			if text := w.text; text != "" {
				var c color.Color = color.RGBA{R: 255, G: 255, B: 255, A: 255}
				if !w.textColor.isZero() {
					c = color.RGBA{
						R: uint8(w.textColor.r * 255),
						G: uint8(w.textColor.g * 255),
						B: uint8(w.textColor.b * 255),
						A: uint8(w.textColor.a * 255),
					}
				}
				drawText(canvas, face, text, rect, screenHeight, c)
			}
		}
		if w.backdrop != nil && !w.backdrop.bgColor.isZero() {
			c := color.RGBA{
				R: uint8(w.backdrop.bgColor.r * 255),
				G: uint8(w.backdrop.bgColor.g * 255),
				B: uint8(w.backdrop.bgColor.b * 255),
				A: uint8(w.backdrop.bgColor.a * 255),
			}
			dst := ScreenRect(rect, screenHeight)
			draw.Draw(canvas, dst, &image.Uniform{C: c}, image.Point{}, draw.Over)
		}
		
		if w.normalTexture != nil {
			paint(w.normalTexture, rect)
		}
		for _, child := range w.children {
			if child.shown {
				paint(child, rect)
			}
		}
	}
	for _, child := range glueParent.children {
		if child.shown {
			paint(child, screen)
		}
	}

	return canvas, nil
}


type hostScreen struct {
	w, h float64
}

func (h hostScreen) ScreenSize() (float64, float64) { return h.w, h.h }
func (h hostScreen) PlaySound(string)               {}
func (h hostScreen) PlayMusic(string)               {}
func (h hostScreen) PlayAmbience(string)            {}
func (h hostScreen) StopMusic()                     {}
func (h hostScreen) StopAmbience()                  {}
func (h hostScreen) StopAllSFX()                    {}
func (h hostScreen) LaunchURL(string)               {}
func (h hostScreen) Quit(bool)                      {}
func (h hostScreen) ConsoleExec(string)             {}
func (h hostScreen) Screenshot()                    {}

func drawSub(canvas *image.RGBA, img image.Image, r Rect, screenHeight float64, tc [4]float64) {
	b := img.Bounds()
	l := b.Min.X + int(float64(b.Dx())*tc[0])
	rt := b.Min.X + int(float64(b.Dx())*tc[1])
	tp := b.Min.Y + int(float64(b.Dy())*tc[2])
	bm := b.Min.Y + int(float64(b.Dy())*tc[3])
	if rt <= l || bm <= tp {
		l, rt, tp, bm = b.Min.X, b.Max.X, b.Min.Y, b.Max.Y
	}
	src := image.NewRGBA(image.Rect(0, 0, rt-l, bm-tp))
	draw.Draw(src, src.Bounds(), img, image.Pt(l, tp), draw.Src)

	dst := ScreenRect(r, screenHeight)
	if dst.Dx() <= 0 || dst.Dy() <= 0 {
		return
	}
	xdraw.BiLinear.Scale(canvas, dst, src, src.Bounds(), xdraw.Over, nil)
}

func drawText(canvas *image.RGBA, face font.Face, text string, r Rect, screenHeight float64, c color.Color) {
	dst := ScreenRect(r, screenHeight)
	d := &font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(c),
		Face: face,
	}
	adv := d.MeasureString(text).Round()
	x := dst.Min.X
	y := dst.Min.Y

	if dst.Dx() <= 0 && dst.Dy() <= 0 {
		x -= adv / 2
		y += 5
	} else {
		if adv < dst.Dx() {
			x = dst.Min.X + (dst.Dx()-adv)/2
		}
		y = dst.Min.Y + (dst.Dy()+16)/2 - 3
	}

	d.Dot = fixed.P(x, y)
	d.DrawString(text)
}
