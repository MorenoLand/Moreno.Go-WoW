package ui

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/opentype"
)

func TestLoginRender(t *testing.T) {
	glue := os.Getenv("WOW_TEST_GLUEXML")
	frame := os.Getenv("WOW_TEST_FRAMEXML")
	assets := os.Getenv("WOW_TEST_ASSETS")
	if glue == "" || frame == "" || assets == "" {
		t.Skip("WOW_TEST_GLUEXML/WOW_TEST_FRAMEXML/WOW_TEST_ASSETS not set; skipped")
	}

	root := t.TempDir()
	stageTree := func(rel, source string) {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(source, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	stageTree("Interface/GlueXML", glue)
	stageTree("Interface/FrameXML", frame)

	const screenWidth, screenHeight = 1280, 960
	rt := NewRuntime(hostScreen{w: screenWidth, h: screenHeight})
	defer rt.Close()
	loader := NewLoader(root, rt)
	t.Logf("Loading TOC...")
	if err := loader.LoadTOC(`Interface\GlueXML\GlueXML.toc`, nil); err != nil {
		t.Fatalf("LoadTOC: %v", err)
	}
	t.Logf("TOC loaded, checking script errors...")
	if errs := rt.ScriptErrors(); len(errs) != 0 {
		for _, e := range errs {
			t.Logf("script error: %v", e)
		}
		t.Fatalf("%d script errors before rendering", len(errs))
	}

	t.Logf("Setting current screen...")
	rt.Execute("GlueParent_OnEvent('SET_GLUE_SCREEN', 'login')", "@render.lua")
	t.Logf("SetCurrentScreen done")

	glueParent := rt.widgets["GlueParent"]
	if glueParent == nil {
		t.Fatal("GlueParent missing")
	}
	login := rt.widgets["AccountLogin"]
	if login == nil {
		t.Fatal("AccountLogin missing")
	}
	if !login.shown {
		t.Fatal("AccountLogin not shown after SetCurrentScreen('login')")
	}
	parentName := "<nil>"
	if login.parent != nil {
		parentName = login.parent.name
	}
	t.Logf("AccountLogin is shown, parent is %s, children count: %d", parentName, len(login.children))
	t.Logf("GlueParent children count: %d", len(glueParent.children))

	fontData, err := os.ReadFile(filepath.Join(assets, "Fonts", "FRIZQT__.TTF"))
	if err != nil {
		t.Fatalf("font: %v", err)
	}
	fontObj, err := opentype.Parse(fontData)
	if err != nil {
		t.Fatalf("font parse: %v", err)
	}
	face, err := opentype.NewFace(fontObj, &opentype.FaceOptions{Size: 16, DPI: 72})
	if err != nil {
		t.Fatalf("font face: %v", err)
	}
	defer face.Close()

	canvas := image.NewRGBA(image.Rect(0, 0, screenWidth, screenHeight))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 8, G: 10, B: 14, A: 255}}, image.Point{}, draw.Src)

	assetLoader := NewLoader(assets, rt)
	cache := map[string]image.Image{}

	screen := Rect{X0: 0, Y0: 0, X1: screenWidth, Y1: screenHeight}

	var paint func(w *widget, parent Rect, depth int)
	paint = func(w *widget, parent Rect, depth int) {
		rect := ResolveRect(w, parent)
		t.Logf("%*s %s: %s @ %v (size %gx%g)", depth*2, "", w.kind.objectType(), w.name, rect, w.width, w.height)
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
							t.Logf("%*s   Loaded texture %s (%dx%d)", depth*2, "", w.textureFile, img.Bounds().Dx(), img.Bounds().Dy())
						} else {
							t.Logf("%*s   blp %s: %v", depth*2, "", w.textureFile, derr)
						}
					} else {
						t.Logf("%*s   asset missing: %s (%v)", depth*2, "", w.textureFile, err)
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
		for _, child := range w.children {
			if child.shown {
				paint(child, rect, depth+1)
			}
		}
	}
	t.Logf("Painting tree...")
	for _, child := range glueParent.children {
		if child.shown {
			paint(child, screen, 0)
		}
	}
	t.Logf("Painting done")

	out := os.Getenv("WOW_RENDER_OUT")
	if out == "" {
		out = filepath.Join(assets, "..", "login-render.png")
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatalf("create %s: %v", out, err)
	}
	defer f.Close()
	if err := png.Encode(f, canvas); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	t.Logf("rendered %s", out)

	var colored int
	for x := 0; x < screenWidth; x += 16 {
		for y := 0; y < screenHeight; y += 16 {
			r, g, b, _ := canvas.At(x, y).RGBA()
			if r>>8 != 8 || g>>8 != 10 || b>>8 != 14 {
				colored++
			}
		}
	}
	if colored == 0 {
		t.Fatal("render produced only background pixels")
	}
}
