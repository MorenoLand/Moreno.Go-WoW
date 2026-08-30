package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	xdraw "golang.org/x/image/draw"
)

type UIEngine struct {
	Rt          *Runtime
	FontObj     *opentype.Font
	AssetLoader *Loader
	Cache       map[string]image.Image
}

func LoadUIEngine(glue, frame, assets string) (*UIEngine, error) {
	root, err := os.MkdirTemp("", "wow-ui-root-*")
	if err != nil {
		return nil, err
	}

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
	
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if !d.IsDir() && filepath.Base(path) == "GlueStrings.lua" {
			log.Printf("STAGED FILE: %s", path)
		}
		return nil
	})

	rt := NewRuntime(&hostScreen{})

	l := NewLoader(root, rt)
	if err := l.LoadTOC("Interface/GlueXML/GlueXML.toc", nil); err != nil {
		return nil, err
	}

	rt.Execute("GlueParent_OnEvent('SET_GLUE_SCREEN', 'login')", "@render.lua")

	fontData, err := os.ReadFile(filepath.Join(assets, "Fonts", "FRIZQT__.TTF"))
	if err != nil {
		return nil, fmt.Errorf("font: %v", err)
	}
	fontObj, err := opentype.Parse(fontData)
	if err != nil {
		return nil, fmt.Errorf("font parse: %v", err)
	}
	assetLoader := NewLoader(assets, rt)
	cache := map[string]image.Image{}

	return &UIEngine{
		Rt:          rt,
		FontObj:     fontObj,
		AssetLoader: assetLoader,
		Cache:       cache,
	}, nil
}

func (eng *UIEngine) Close() {
}

func (eng *UIEngine) Render(screenWidth, screenHeight int) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, screenWidth, screenHeight))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 8, G: 10, B: 14, A: 255}}, image.Point{}, draw.Src)

	uiScale := float64(screenHeight) / 768.0
	
	face, _ := opentype.NewFace(eng.FontObj, &opentype.FaceOptions{Size: 16 * uiScale, DPI: 72})
	defer face.Close()

	virtualWidth := float64(screenWidth) / uiScale
	virtualHeight := 768.0

	// Update host screen dimensions dynamically before layout evaluation
	if host, ok := eng.Rt.Host.(*hostScreen); ok {
		host.w = virtualWidth
		host.h = virtualHeight
	}

	screen := Rect{X0: 0, Y0: 0, X1: virtualWidth, Y1: virtualHeight}

	var paint func(w *widget, parent Rect)
	paint = func(w *widget, parent Rect) {
		rect := ResolveRect(w, parent)

		// Scale UI coordinates to the window resolution based on a 768p reference height.
		scaledRect := Rect{
			X0: rect.X0 * uiScale,
			Y0: rect.Y0 * uiScale,
			X1: rect.X1 * uiScale,
			Y1: rect.Y1 * uiScale,
		}

		switch w.kind {
		case kindTexture:
			if w.textureFile != "" {
				tc := [4]float64{w.texCoordL, w.texCoordR, w.texCoordT, w.texCoordB}
				img, ok := eng.Cache[w.textureFile]
				if !ok {
					data, err := eng.AssetLoader.ReadAsset(w.textureFile)
					if err == nil {
						decoded, derr := DecodeBLP(data)
						if derr == nil {
							img = decoded
						} else {
							log.Printf("Decode error for %s: %v", w.textureFile, derr)
						}
					} else {
						log.Printf("Read error for %s: %v", w.textureFile, err)
					}
					eng.Cache[w.textureFile] = img
				}
				if img != nil {
					if tc[0] == 0 && tc[1] == 0 && tc[2] == 0 && tc[3] == 0 {
						tc = [4]float64{0, 1, 0, 1}
					}
					drawSub(canvas, img, scaledRect, float64(screenHeight), tc)
				}
			}
		case kindFontString, kindButton:
			if text := w.text; text != "" {
				var c color.Color = color.RGBA{R: 255, G: 255, B: 255, A: 255}
				if !w.textColor.isZero() {
					c = color.RGBA{
						R: uint8(w.textColor.r * 255),
						G: uint8(w.textColor.g * 255),
						B: uint8(w.textColor.b * 255),
						A: uint8(w.textColor.a * 255),
					}
				} else if w.kind == kindButton {
					c = color.RGBA{R: 255, G: 200, B: 0, A: 255}
				}
				drawText(canvas, face, text, scaledRect, float64(screenHeight), c, w.kind == kindButton)
			}
		}

		if w.backdrop != nil && w.backdrop.bgFile != "" {
			img, ok := eng.Cache[w.backdrop.bgFile]
			if !ok {
				data, err := eng.AssetLoader.ReadAsset(w.backdrop.bgFile)
				if err == nil {
					decoded, derr := DecodeBLP(data)
					if derr == nil {
						img = decoded
					}
				}
				eng.Cache[w.backdrop.bgFile] = img
			}
			if img != nil {
				drawSub(canvas, img, scaledRect, float64(screenHeight), [4]float64{0, 1, 0, 1})
			}
		}

		if w.normalTexture != nil && w.normalTexture.shown {
			paint(w.normalTexture, rect)
		}
		if w.pushedTexture != nil && w.pushedTexture.shown {
			paint(w.pushedTexture, rect)
		}
		if w.highlightTexture != nil && w.highlightTexture.shown {
			paint(w.highlightTexture, rect)
		}

		for _, child := range w.children {
			if child.shown {
				paint(child, rect)
			}
		}
	}

	glueParent := eng.Rt.widgets["GlueParent"]
	if glueParent != nil {
		for _, child := range glueParent.children {
			if child.shown {
				paint(child, screen)
			}
		}
	}

	return canvas
}


type hostScreen struct {
	w, h float64
}

func (h *hostScreen) ScreenSize() (float64, float64) { return h.w, h.h }
func (h *hostScreen) PlaySound(string)               {}
func (h *hostScreen) PlayMusic(string)               {}
func (h *hostScreen) PlayAmbience(string)            {}
func (h *hostScreen) StopMusic()                     {}
func (h *hostScreen) StopAmbience()                  {}
func (h *hostScreen) StopAllSFX()                    {}
func (h *hostScreen) LaunchURL(string)               {}
func (h *hostScreen) Quit(bool)                      {}
func (h *hostScreen) ConsoleExec(string)             {}
func (h *hostScreen) Screenshot()                    {}


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

func drawText(canvas *image.RGBA, face font.Face, text string, r Rect, screenHeight float64, c color.Color, center bool) {
	dst := ScreenRect(r, screenHeight)
	if dst.Dx() <= 0 || dst.Dy() <= 0 {
		return
	}
	
	dotX := dst.Min.X
	if center {
		width := font.MeasureString(face, text).Ceil()
		dotX = dst.Min.X + (dst.Dx() - width) / 2
	}
	
	d := &font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(dotX, dst.Min.Y+face.Metrics().Ascent.Ceil()+(dst.Dy()-face.Metrics().Height.Ceil())/2),
	}
	d.DrawString(text)
}
