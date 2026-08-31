package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // register JPEG decoder for image.Decode
	"log"
	"os"
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	xdraw "golang.org/x/image/draw"
)

type UIEngine struct {
	Rt          *Runtime
	FontObj     *opentype.Font
	FontObjSm   *opentype.Font
	AssetLoader *Loader
	Cache       map[string]image.Image
	BgImagePath string // Path to a static background image (JPEG/PNG)
}

func LoadUIEngine(glue, frame, assets string, bgImagePath string) (*UIEngine, error) {
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

	// Try to load MORPHEUS for title-style text, fall back to FRIZQT
	var fontObjSm *opentype.Font
	if morphData, err2 := os.ReadFile(filepath.Join(assets, "Fonts", "MORPHEUS.ttf")); err2 == nil {
		if mo, err3 := opentype.Parse(morphData); err3 == nil {
			fontObjSm = mo
		}
	}
	if fontObjSm == nil {
		fontObjSm = fontObj
	}

	assetLoader := NewLoader(assets, rt)
	cache := map[string]image.Image{}

	return &UIEngine{
		Rt:          rt,
		FontObj:     fontObj,
		FontObjSm:   fontObjSm,
		AssetLoader: assetLoader,
		Cache:       cache,
		BgImagePath: bgImagePath,
	}, nil
}

func (eng *UIEngine) Close() {}

// resolveText looks up a string that may be a Lua global (e.g. "ACCOUNT_NAME")
// and returns the localized value, or the original string if not found.
func (eng *UIEngine) resolveText(s string) string {
	if s == "" {
		return s
	}
	// Try to look it up as a Lua global
	v := eng.Rt.L.GetGlobal(s)
	if v != nil && v.Type() == lua.LTString {
		return string(v.(lua.LString))
	}
	return s
}

// loadBLP loads an asset as an image, caching the result.
func (eng *UIEngine) loadBLP(path string) image.Image {
	if path == "" {
		return nil
	}
	if img, ok := eng.Cache[path]; ok {
		return img
	}
	data, err := eng.AssetLoader.ReadAsset(path)
	if err != nil {
		eng.Cache[path] = nil
		return nil
	}
	img, err := DecodeBLP(data)
	if err != nil {
		log.Printf("DecodeBLP %s: %v", path, err)
		eng.Cache[path] = nil
		return nil
	}
	eng.Cache[path] = img
	return img
}

func (eng *UIEngine) Render(screenWidth, screenHeight int) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, screenWidth, screenHeight))

	uiScale := float64(screenHeight) / 768.0

	face, _ := opentype.NewFace(eng.FontObj, &opentype.FaceOptions{Size: 13 * uiScale, DPI: 96})
	defer face.Close()

	faceLg, _ := opentype.NewFace(eng.FontObj, &opentype.FaceOptions{Size: 16 * uiScale, DPI: 96})
	defer faceLg.Close()

	virtualWidth := float64(screenWidth) / uiScale
	virtualHeight := 768.0

	// Update host screen dimensions dynamically
	if host, ok := eng.Rt.Host.(*hostScreen); ok {
		host.w = virtualWidth
		host.h = virtualHeight
	}

	// ─── Step 1: Render background from BLP sky textures ───────────────
	eng.renderBackground(canvas, screenWidth, screenHeight)

	screen := Rect{X0: 0, Y0: 0, X1: virtualWidth, Y1: virtualHeight}

	var paint func(w *widget, parent Rect)
	paint = func(w *widget, parent Rect) {
		rect := ResolveRect(w, parent)

		scaledRect := Rect{
			X0: rect.X0 * uiScale,
			Y0: rect.Y0 * uiScale,
			X1: rect.X1 * uiScale,
			Y1: rect.Y1 * uiScale,
		}

		// ─── Backdrop ────────────────────────────────────────────────────
		if w.backdrop != nil && (w.backdrop.bgFile != "" || w.backdrop.edgeFile != "") {
			eng.drawBackdrop(canvas, w.backdrop, scaledRect)
		}

		switch w.kind {
		case kindTexture:
			if w.textureFile != "" {
				tc := [4]float64{w.texCoordL, w.texCoordR, w.texCoordT, w.texCoordB}
				img := eng.loadBLP(w.textureFile)
				if img != nil {
					if tc[0] == 0 && tc[1] == 0 && tc[2] == 0 && tc[3] == 0 {
						tc = [4]float64{0, 1, 0, 1}
					}
					drawSub(canvas, img, scaledRect, float64(screenHeight), tc)
				}
			}

		case kindFontString:
			text := eng.resolveText(w.text)
			if text != "" {
				c := eng.fontColor(w)
				f := face
				if w.fontObject != "" && strings.Contains(strings.ToLower(w.fontObject), "large") {
					f = faceLg
				}
				drawText(canvas, f, text, scaledRect, float64(screenHeight), c, false)
			}

		case kindEditBox:
			// EditBox: draw dark fill + border
			eng.drawEditBoxBg(canvas, scaledRect)

		case kindButton, kindCheckButton:
			// Draw normal texture first (handled via normalTexture child)
			// Then draw label text centered
			text := eng.resolveText(w.text)
			if text != "" {
				c := color.RGBA{R: 255, G: 210, B: 0, A: 255} // WoW gold
				if !w.textColor.isZero() {
					c = color.RGBA{
						R: uint8(w.textColor.r * 255),
						G: uint8(w.textColor.g * 255),
						B: uint8(w.textColor.b * 255),
						A: 255,
					}
				}
				drawText(canvas, face, text, scaledRect, float64(screenHeight), c, true)
			}
		}

		// Button textures
		if w.normalTexture != nil && w.normalTexture.shown {
			paint(w.normalTexture, rect)
		}

		// Children (layers, frames, etc.)
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

// renderBackground draws the WotLK Northrend background.
// Since the background comes from a 3D animated M2 model which we can't render
// in software yet, we use the reference screenshot (Wow.jpg / Wow.png) from the
// config or a fallback gradient.
func (eng *UIEngine) renderBackground(canvas *image.RGBA, w, h int) {
	// Try loading a reference background image from the game data directory.
	// The user has Wow.jpg on the desktop which is the actual game rendering.
	bgPaths := []string{}
	if eng.BgImagePath != "" {
		bgPaths = append(bgPaths, eng.BgImagePath)
	}
	// Also try Wow.jpg on the desktop as a convenience default
	bgPaths = append(bgPaths,
		`C:\Users\null\Desktop\Wow.jpg`,
		`C:\Users\null\Desktop\Wow.png`,
	)

	for _, p := range bgPaths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		img, _, ierr := image.Decode(f)
		f.Close()
		if ierr != nil {
			continue
		}
		// Scale to fill canvas
		xdraw.BiLinear.Scale(canvas, canvas.Bounds(), img, img.Bounds(), xdraw.Src, nil)
		return
	}

	// Fallback: Northrend-style gradient
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h)
		// Top: dark midnight blue, bottom: slightly lighter teal
		r := uint8(5 + int(t*15))
		g := uint8(18 + int(t*45))
		b := uint8(40 + int(t*50))
		row := &image.Uniform{C: color.RGBA{R: r, G: g, B: b, A: 255}}
		draw.Draw(canvas, image.Rect(0, y, w, y+1), row, image.Point{}, draw.Src)
	}
}

// drawBackdrop renders an EditBox / Frame backdrop (dark tinted fill + teal border).
func (eng *UIEngine) drawBackdrop(canvas *image.RGBA, bd *backdrop, r Rect) {
	dst := ScreenRect(r, float64(canvas.Bounds().Dy()))
	if dst.Dx() <= 0 || dst.Dy() <= 0 {
		return
	}

	bgImg := eng.loadBLP(bd.bgFile)
	if bgImg != nil {
		// Tile the background inside insets
		inL := int(bd.insetL)
		inR := int(bd.insetR)
		inT := int(bd.insetT)
		inB := int(bd.insetB)
		inner := image.Rect(dst.Min.X+inL, dst.Min.Y+inT, dst.Max.X-inR, dst.Max.Y-inB)
		if inner.Dx() > 0 && inner.Dy() > 0 {
			// Apply backdrop color tint (RGBA from script)
			bgCol := color.RGBA{
				R: uint8(bd.bgColor.r * 255),
				G: uint8(bd.bgColor.g * 255),
				B: uint8(bd.bgColor.b * 255),
				A: uint8(bd.bgColor.a * 255),
			}
			if bgCol.A == 0 && (bgCol.R != 0 || bgCol.G != 0 || bgCol.B != 0) {
				bgCol.A = 255
			}
			if bgCol.A == 0 {
				// Default: very dark near-transparent navy
				bgCol = color.RGBA{R: 10, G: 20, B: 30, A: 180}
			}
			draw.Draw(canvas, inner, &image.Uniform{C: bgCol}, image.Point{}, draw.Over)
			// Tile the actual texture over it
			xdraw.NearestNeighbor.Scale(canvas, inner, bgImg, bgImg.Bounds(), xdraw.Over, nil)
		}
	} else {
		// Fallback: solid very dark box
		draw.Draw(canvas, dst, &image.Uniform{C: color.RGBA{R: 8, G: 15, B: 25, A: 200}}, image.Point{}, draw.Over)
	}

	// Draw border
	edgeImg := eng.loadBLP(bd.edgeFile)
	if edgeImg != nil {
		// Simple: draw a thin teal/grey outline
		borderColor := color.RGBA{R: uint8(bd.edgeColor.r * 255), G: uint8(bd.edgeColor.g * 255), B: uint8(bd.edgeColor.b * 255), A: 255}
		if borderColor.R == 0 && borderColor.G == 0 && borderColor.B == 0 {
			borderColor = color.RGBA{R: 100, G: 130, B: 160, A: 200}
		}
		drawBorder(canvas, dst, borderColor, 1)
	} else {
		drawBorder(canvas, dst, color.RGBA{R: 80, G: 120, B: 150, A: 200}, 1)
	}
}

// drawEditBoxBg draws the standard WoW EditBox dark-panel look.
func (eng *UIEngine) drawEditBoxBg(canvas *image.RGBA, r Rect) {
	dst := ScreenRect(r, float64(canvas.Bounds().Dy()))
	if dst.Dx() <= 0 || dst.Dy() <= 0 {
		return
	}
	// Dark fill
	draw.Draw(canvas, dst, &image.Uniform{C: color.RGBA{R: 10, G: 18, B: 28, A: 220}}, image.Point{}, draw.Over)
	// Teal-ish border matching the reference
	drawBorder(canvas, dst, color.RGBA{R: 60, G: 100, B: 140, A: 255}, 2)
}

// drawBorder draws a pixel-thick rect border.
func drawBorder(canvas *image.RGBA, r image.Rectangle, c color.Color, thickness int) {
	u := &image.Uniform{C: c}
	draw.Draw(canvas, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+thickness), u, image.Point{}, draw.Over)
	draw.Draw(canvas, image.Rect(r.Min.X, r.Max.Y-thickness, r.Max.X, r.Max.Y), u, image.Point{}, draw.Over)
	draw.Draw(canvas, image.Rect(r.Min.X, r.Min.Y, r.Min.X+thickness, r.Max.Y), u, image.Point{}, draw.Over)
	draw.Draw(canvas, image.Rect(r.Max.X-thickness, r.Min.Y, r.Max.X, r.Max.Y), u, image.Point{}, draw.Over)
}

// fontColor determines the text color for a widget.
func (eng *UIEngine) fontColor(w *widget) color.Color {
	if !w.textColor.isZero() {
		return color.RGBA{
			R: uint8(w.textColor.r * 255),
			G: uint8(w.textColor.g * 255),
			B: uint8(w.textColor.b * 255),
			A: 255,
		}
	}
	// Default WoW glue label is yellow-gold
	return color.RGBA{R: 255, G: 210, B: 0, A: 255}
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

	dotX := dst.Min.X + 4
	if center {
		width := font.MeasureString(face, text).Ceil()
		dotX = dst.Min.X + (dst.Dx()-width)/2
	}

	ascent := face.Metrics().Ascent.Ceil()
	height := face.Metrics().Height.Ceil()
	dotY := dst.Min.Y + ascent + (dst.Dy()-height)/2

	d := &font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.P(dotX, dotY),
	}
	d.DrawString(text)
}
