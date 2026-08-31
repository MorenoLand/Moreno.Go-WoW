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

	"github.com/g3n/engine/window"
	lua "github.com/yuin/gopher-lua"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type UIEngine struct {
	Rt           *Runtime
	FontObj      *opentype.Font
	FontObjSm    *opentype.Font
	AssetLoader  *Loader
	Cache        map[string]image.Image
	BgImagePath  string // Path to a static background image (JPEG/PNG)
	statusKey    string
	rememberMe   bool
	pressed      *widget
	hovered      *widget
	screen       Rect
	uiScale      float64
	screenWidth  int
	screenHeight int
	rects        map[*widget]Rect
	layoutActive map[*widget]bool
}

func LoadUIEngine(glue, frame, assets string, bgImagePath string) (*UIEngine, error) {
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

	rt := NewRuntime(&hostScreen{w: 960, h: 640})

	l := NewLoaderWithAssets(root, assets, rt)
	if err := l.LoadTOC("Interface/GlueXML/GlueXML.toc", nil); err != nil {
		rt.Close()
		return nil, err
	}
	engine, err := newUIEngine(rt, l, bgImagePath)
	if err != nil {
		rt.Close()
		return nil, err
	}
	return engine, nil
}

func LoadUIEngineFromMPQ(dataPath, locale, bgImagePath string) (*UIEngine, error) {
	rt := NewRuntime(&hostScreen{w: 960, h: 640})
	l, err := NewMPQLoader(dataPath, locale, rt)
	if err != nil {
		rt.Close()
		return nil, err
	}
	if err := l.LoadTOC("Interface/GlueXML/GlueXML.toc", nil); err != nil {
		_ = l.Close()
		rt.Close()
		return nil, err
	}
	engine, err := newUIEngine(rt, l, bgImagePath)
	if err != nil {
		_ = l.Close()
		rt.Close()
		return nil, err
	}
	return engine, nil
}

func newUIEngine(rt *Runtime, loader *Loader, bgImagePath string) (*UIEngine, error) {
	if dialog := rt.widgets["ChangedOptionsDialog"]; dialog != nil {
		dialog.shown = false
	}

	rt.Execute("GlueParent_OnEvent('SET_GLUE_SCREEN', 'login')", "@render.lua")

	fontData, err := loader.readAsset("Fonts\\FRIZQT__.TTF")
	if err != nil {
		return nil, fmt.Errorf("font: %v", err)
	}
	fontObj, err := opentype.Parse(fontData)
	if err != nil {
		return nil, fmt.Errorf("font parse: %v", err)
	}

	// Try to load MORPHEUS for title-style text, fall back to FRIZQT
	var fontObjSm *opentype.Font
	if morphData, err2 := loader.readAsset("Fonts\\MORPHEUS.ttf"); err2 == nil {
		if mo, err3 := opentype.Parse(morphData); err3 == nil {
			fontObjSm = mo
		}
	}
	if fontObjSm == nil {
		fontObjSm = fontObj
	}

	cache := map[string]image.Image{}

	return &UIEngine{
		Rt:          rt,
		FontObj:     fontObj,
		FontObjSm:   fontObjSm,
		AssetLoader: loader,
		Cache:       cache,
		BgImagePath: bgImagePath,
	}, nil
}

func (eng *UIEngine) Close() {
	if eng.Rt != nil {
		eng.Rt.Close()
		eng.Rt = nil
	}
	if eng.AssetLoader != nil {
		_ = eng.AssetLoader.Close()
		eng.AssetLoader = nil
	}
}

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
	if screenWidth < 1 {
		screenWidth = 1
	}
	if screenHeight < 1 {
		screenHeight = 1
	}
	canvas := image.NewRGBA(image.Rect(0, 0, screenWidth, screenHeight))

	uiScale := float64(screenHeight) / 768.0
	eng.uiScale = uiScale
	eng.screenWidth = screenWidth
	eng.screenHeight = screenHeight
	eng.screen = Rect{X0: 0, Y0: 0, X1: float64(screenWidth) / uiScale, Y1: 768}
	eng.rects = make(map[*widget]Rect)
	eng.layoutActive = make(map[*widget]bool)

	face, _ := opentype.NewFace(eng.FontObj, &opentype.FaceOptions{Size: 13 * uiScale, DPI: 96})
	defer face.Close()

	faceLg, _ := opentype.NewFace(eng.FontObj, &opentype.FaceOptions{Size: 16 * uiScale, DPI: 96})
	defer faceLg.Close()
	if root := eng.Rt.widgets["GlueParent"]; root != nil {
		eng.prepareText(root, face, faceLg)
	}

	virtualWidth := eng.screen.W()
	virtualHeight := eng.screen.H()

	// Update host screen dimensions dynamically
	if host, ok := eng.Rt.Host.(*hostScreen); ok {
		host.w = virtualWidth
		host.h = virtualHeight
	}

	// ─── Step 1: Render background from BLP sky textures ───────────────
	eng.renderBackground(canvas, screenWidth, screenHeight)

	screen := eng.screen

	var paint func(w *widget, parent Rect)
	paint = func(w *widget, parent Rect) {
		rect := eng.layoutRect(w, parent)

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
					drawSubMode(canvas, img, scaledRect, float64(screenHeight), tc, strings.EqualFold(w.alphaMode, "ADD"))
				}
			} else if !w.vertexColor.isZero() {
				eng.drawTextureColor(canvas, scaledRect, w.vertexColor)
			}

		case kindFontString:
			text := eng.resolveText(w.text)
			if w.parent != nil && (w.parent.kind == kindButton || w.parent.kind == kindCheckButton) && w.parent.buttonLabel == w && w.parent.text != "" {
				text = eng.resolveText(w.parent.text)
			}
			if text != "" {
				c := eng.fontColor(w)
				f := face
				if w.fontObject != "" && strings.Contains(strings.ToLower(w.fontObject), "large") {
					f = faceLg
				}
				drawTextAligned(canvas, f, text, scaledRect, float64(screenHeight), c, w.justifyH)
			}

		case kindEditBox:
			eng.drawEditBoxBg(canvas, scaledRect)
		}

		if w.kind == kindButton || w.kind == kindCheckButton {
			eng.paintButtonState(w, rect, paint)
		}

		for _, child := range w.children {
			if child.shown && !(w.kind == kindEditBox && w.text != "" && child.kind == kindFontString && strings.HasSuffix(strings.ToLower(child.name), "fill")) {
				paint(child, rect)
			}
		}

		if w.kind == kindButton || w.kind == kindCheckButton {
			if w.buttonLabel == nil {
				text := eng.resolveText(w.text)
				if text != "" {
					drawTextAligned(canvas, face, text, scaledRect, float64(screenHeight), eng.fontColor(w), "CENTER")
				}
			}
		}
		if w.kind == kindEditBox {
			eng.drawEditText(canvas, face, w, rect, float64(screenHeight))
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
	if eng.statusKey != "" {
		status := eng.resolveText(eng.statusKey)
		drawTextAligned(canvas, face, status, screenScaledRect(Rect{X0: 80, Y0: 96, X1: virtualWidth - 80, Y1: 128}, uiScale), float64(screenHeight), color.RGBA{R: 255, G: 100, B: 80, A: 255}, "CENTER")
	}

	return canvas
}

func (eng *UIEngine) prepareText(w *widget, face, faceLg font.Face) {
	if w.kind == kindFontString {
		text := eng.resolveText(w.text)
		if w.parent != nil && (w.parent.kind == kindButton || w.parent.kind == kindCheckButton) && w.parent.buttonLabel == w && w.parent.text != "" {
			text = eng.resolveText(w.parent.text)
		}
		fontFace := face
		if strings.Contains(strings.ToLower(w.fontObject), "large") || strings.Contains(strings.ToLower(w.fontObject), "huge") {
			fontFace = faceLg
		}
		lines := strings.Split(cleanText(text), "\n")
		if w.autoTextWidth {
			maxWidth := 0
			for _, line := range lines {
				if width := font.MeasureString(fontFace, line).Ceil(); width > maxWidth {
					maxWidth = width
				}
			}
			w.width = float64(maxWidth) / eng.uiScale
		}
		if w.autoTextHeight {
			w.height = float64(fontFace.Metrics().Height.Ceil()*len(lines)) / eng.uiScale
		}
	}
	for _, child := range w.children {
		eng.prepareText(child, face, faceLg)
	}
}

func (eng *UIEngine) layoutRect(w *widget, parent Rect) Rect {
	if rect, ok := eng.rects[w]; ok {
		return rect
	}
	if eng.layoutActive[w] {
		return ResolveRect(w, parent)
	}
	eng.layoutActive[w] = true
	defer delete(eng.layoutActive, w)
	parentRect := parent
	if w.parent != nil {
		parentRect = eng.layoutRect(w.parent, parent)
	}
	rect := resolveRect(w, parentRect, func(name string) (Rect, bool) {
		target := eng.Rt.widgets[name]
		if target == nil {
			return Rect{}, false
		}
		targetParent := eng.screen
		if target.parent != nil {
			targetParent = eng.layoutRect(target.parent, eng.screen)
		}
		return eng.layoutRect(target, targetParent), true
	})
	eng.rects[w] = rect
	return rect
}

func (eng *UIEngine) paintButtonState(w *widget, rect Rect, paint func(*widget, Rect)) {
	base := w.normalTexture
	if !w.enabled {
		if w.checked && w.disabledCheckedTexture != nil {
			base = w.disabledCheckedTexture
		} else if w.disabledTexture != nil {
			base = w.disabledTexture
		}
	} else if w.buttonState == "PUSHED" && w.pushedTexture != nil {
		base = w.pushedTexture
	}
	if base != nil && base.shown {
		paint(base, rect)
	}
	if w.enabled && w.checked && w.checkedTexture != nil && w.checkedTexture.shown {
		paint(w.checkedTexture, rect)
	}
	if w.enabled && w.highlighted && w.highlightTexture != nil && w.highlightTexture.shown {
		paint(w.highlightTexture, rect)
	}
}

func (eng *UIEngine) drawEditText(canvas *image.RGBA, face font.Face, w *widget, rect Rect, screenHeight float64) {
	if w.text == "" {
		return
	}
	text := w.text
	if w.password {
		text = strings.Repeat("*", len([]rune(text)))
	}
	left := w.textInsetL
	right := w.textInsetR
	top := w.textInsetT
	bottom := w.textInsetB
	if left == 0 {
		left = 12
	}
	if right == 0 {
		right = 5
	}
	if top == 0 && bottom == 0 {
		bottom = 4
	}
	textRect := Rect{X0: rect.X0 + left, Y0: rect.Y0 + bottom, X1: rect.X1 - right, Y1: rect.Y1 - top}
	screenTextRect := screenScaledRect(textRect, eng.uiScale)
	drawTextAligned(canvas, face, text, screenTextRect, screenHeight, color.RGBA{R: 235, G: 235, B: 235, A: 255}, "LEFT")
	if eng.Rt.focused == w {
		width := font.MeasureString(face, text).Ceil()
		dst := ScreenRect(screenTextRect, screenHeight)
		caretX := dst.Min.X + width + 1
		caret := image.Rect(caretX, dst.Min.Y+4, caretX+1, dst.Max.Y-4)
		draw.Draw(canvas, caret, &image.Uniform{C: color.RGBA{R: 255, G: 220, B: 80, A: 255}}, image.Point{}, draw.Over)
	}
}

func (eng *UIEngine) drawTextureColor(canvas *image.RGBA, r Rect, c rgba) {
	dst := ScreenRect(r, float64(canvas.Bounds().Dy()))
	if dst.Dx() <= 0 || dst.Dy() <= 0 || c.a <= 0 {
		return
	}
	alpha := c.a
	if alpha > 1 {
		alpha = 1
	}
	draw.Draw(canvas, dst, &image.Uniform{C: color.RGBA{R: uint8(c.r * 255), G: uint8(c.g * 255), B: uint8(c.b * 255), A: uint8(alpha * 255)}}, image.Point{}, draw.Over)
}

func screenScaledRect(r Rect, scale float64) Rect {
	return Rect{X0: r.X0 * scale, Y0: r.Y0 * scale, X1: r.X1 * scale, Y1: r.Y1 * scale}
}

func (eng *UIEngine) SetStatusKey(key string) {
	eng.statusKey = key
}

func (eng *UIEngine) SetInitialCredentials(account, password string, rememberMe bool) {
	eng.rememberMe = rememberMe
	eng.Rt.SetCVar("accountName", account)
	eng.Rt.Execute("GlueParent_OnEvent('SET_GLUE_SCREEN', 'login')", "@credentials.lua")
	if checkbox := eng.Rt.widgets["AccountLoginSaveAccountName"]; checkbox != nil {
		checkbox.checked = account != ""
	}
	if password != "" {
		if edit := eng.Rt.widgets["AccountLoginPasswordEdit"]; edit != nil {
			eng.Rt.setText(edit, password)
			eng.Rt.setFocus(edit)
		}
	}
}

func (eng *UIEngine) SetGlueState(state GlueState) {
	eng.Rt.Glue = state
	eng.statusKey = ""
	eng.Rt.Execute("GlueParent_OnEvent('SET_GLUE_SCREEN', 'charselect')", "@network.lua")
}

func (eng *UIEngine) HandleCursor(x, y float64) bool {
	target := eng.hitTest(x, y)
	if target == eng.hovered {
		return false
	}
	if eng.hovered != nil {
		eng.hovered.highlighted = false
	}
	eng.hovered = target
	if target != nil && (target.kind == kindButton || target.kind == kindCheckButton) {
		target.highlighted = true
	}
	return true
}

func (eng *UIEngine) HandleMouse(x, y float64, button window.MouseButton, down bool) bool {
	if button != window.MouseButtonLeft {
		return false
	}
	target := eng.hitTest(x, y)
	if down {
		eng.pressed = target
		if target == nil {
			eng.Rt.setFocus(nil)
			return false
		}
		if target.kind == kindEditBox {
			eng.Rt.setFocus(target)
		} else if target.kind == kindButton || target.kind == kindCheckButton {
			target.buttonState = "PUSHED"
		}
		return true
	}
	pressed := eng.pressed
	eng.pressed = nil
	if pressed == nil {
		return target != nil
	}
	if pressed.kind == kindButton || pressed.kind == kindCheckButton {
		pressed.buttonState = "NORMAL"
	}
	if pressed == target && (target.kind == kindButton || target.kind == kindCheckButton) {
		if target.kind == kindCheckButton {
			target.checked = !target.checked
		}
		eng.Rt.fire(target, "OnClick", []lua.LValue{target.luaValue(eng.Rt.L), lua.LString("LeftButton"), lua.LBool(false)})
		return true
	}
	return pressed == target
}

func (eng *UIEngine) HandleChar(char rune) bool {
	w := eng.Rt.focused
	if w == nil || w.kind != kindEditBox || char < 32 || char == 127 {
		return false
	}
	runes := []rune(w.text)
	if w.cursor < 0 || w.cursor > len(runes) {
		w.cursor = len(runes)
	}
	if w.maxLetters > 0 && len(runes) >= w.maxLetters {
		return true
	}
	runes = append(runes, 0)
	copy(runes[w.cursor+1:], runes[w.cursor:])
	runes[w.cursor] = char
	if w.maxBytes > 0 && len([]byte(string(runes))) > w.maxBytes {
		return true
	}
	position := w.cursor + 1
	eng.Rt.setText(w, string(runes))
	w.cursor = position
	return true
}

func (eng *UIEngine) HandleKey(key window.Key) bool {
	w := eng.Rt.focused
	if key == window.KeyEscape {
		target := eng.keyboardTarget()
		if target != nil && target != w && !eng.isLoginTarget(target) {
			eng.Rt.fire(target, "OnKeyDown", []lua.LValue{target.luaValue(eng.Rt.L), lua.LString("ESCAPE")})
			return true
		}
		if w != nil {
			eng.Rt.setFocus(nil)
			return true
		}
		if target != nil {
			eng.Rt.fire(target, "OnKeyDown", []lua.LValue{target.luaValue(eng.Rt.L), lua.LString("ESCAPE")})
			return true
		}
		return false
	}
	if w != nil && w.kind == kindEditBox {
		switch key {
		case window.KeyTab:
			eng.Rt.fire(w, "OnTabPressed", []lua.LValue{w.luaValue(eng.Rt.L)})
		case window.KeyEnter:
			eng.Rt.fire(w, "OnEnterPressed", []lua.LValue{w.luaValue(eng.Rt.L)})
		case window.KeyLeft:
			if w.cursor > 0 {
				w.cursor--
			}
		case window.KeyRight:
			if w.cursor < len([]rune(w.text)) {
				w.cursor++
			}
		case window.KeyHome:
			w.cursor = 0
		case window.KeyEnd:
			w.cursor = len([]rune(w.text))
		case window.KeyBackspace:
			if w.cursor > 0 {
				runes := []rune(w.text)
				runes = append(runes[:w.cursor-1], runes[w.cursor:]...)
				w.cursor--
				eng.Rt.setText(w, string(runes))
			}
		case window.KeyDelete:
			runes := []rune(w.text)
			if w.cursor < len(runes) {
				runes = append(runes[:w.cursor], runes[w.cursor+1:]...)
				eng.Rt.setText(w, string(runes))
			}
		default:
			return false
		}
		return true
	}
	target := eng.keyboardTarget()
	if target == nil {
		return false
	}
	name := keyName(key)
	if name == "" {
		return false
	}
	eng.Rt.fire(target, "OnKeyDown", []lua.LValue{target.luaValue(eng.Rt.L), lua.LString(name)})
	return true
}

func (eng *UIEngine) isLoginTarget(target *widget) bool {
	for current := target; current != nil; current = current.parent {
		if current.name == "AccountLogin" {
			return true
		}
	}
	return false
}

func (eng *UIEngine) HandleKeyUp(key window.Key) bool {
	target := eng.keyboardTarget()
	if target == nil {
		return false
	}
	name := keyName(key)
	if name == "" {
		return false
	}
	eng.Rt.fire(target, "OnKeyUp", []lua.LValue{target.luaValue(eng.Rt.L), lua.LString(name)})
	return true
}

func (eng *UIEngine) keyboardTarget() *widget {
	root := eng.Rt.widgets["GlueParent"]
	if root == nil {
		return nil
	}
	for index := len(root.children) - 1; index >= 0; index-- {
		if target := keyboardWidget(root.children[index]); target != nil {
			return target
		}
	}
	return nil
}

func keyboardWidget(w *widget) *widget {
	if !w.shown {
		return nil
	}
	for index := len(w.children) - 1; index >= 0; index-- {
		if target := keyboardWidget(w.children[index]); target != nil {
			return target
		}
	}
	if w.enableKeyboard {
		return w
	}
	return nil
}

func keyName(key window.Key) string {
	switch key {
	case window.KeyEscape:
		return "ESCAPE"
	case window.KeyEnter:
		return "ENTER"
	case window.KeySpace:
		return "SPACE"
	case window.KeyPrintScreen:
		return "PRINTSCREEN"
	default:
		return ""
	}
}

func (eng *UIEngine) hitTest(x, y float64) *widget {
	if eng.uiScale == 0 {
		return nil
	}
	point := struct{ x, y float64 }{x / eng.uiScale, (float64(eng.screenHeight) - y) / eng.uiScale}
	var visit func(*widget, Rect) *widget
	visit = func(w *widget, parent Rect) *widget {
		if !w.shown {
			return nil
		}
		rect := eng.layoutRect(w, parent)
		for i := len(w.children) - 1; i >= 0; i-- {
			if target := visit(w.children[i], rect); target != nil {
				return target
			}
		}
		if point.x < rect.X0 || point.x > rect.X1 || point.y < rect.Y0 || point.y > rect.Y1 {
			return nil
		}
		if w.kind == kindButton || w.kind == kindCheckButton || w.kind == kindEditBox || w.enableMouse {
			return w
		}
		return nil
	}
	root := eng.Rt.widgets["GlueParent"]
	if root == nil {
		return nil
	}
	for i := len(root.children) - 1; i >= 0; i-- {
		if target := visit(root.children[i], eng.screen); target != nil {
			return target
		}
	}
	return nil
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
			if bd.tile {
				eng.drawTiled(canvas, inner, bgImg, bd.tileSize)
			} else {
				xdraw.NearestNeighbor.Scale(canvas, inner, bgImg, bgImg.Bounds(), xdraw.Over, nil)
			}
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

func (eng *UIEngine) drawTiled(canvas *image.RGBA, dst image.Rectangle, source image.Image, tileSize float64) {
	tile := source
	if tileSize > 0 {
		size := int(tileSize)
		if size > 0 {
			tileImage := image.NewRGBA(image.Rect(0, 0, size, size))
			xdraw.NearestNeighbor.Scale(tileImage, tileImage.Bounds(), source, source.Bounds(), xdraw.Src, nil)
			tile = tileImage
		}
	}
	bounds := tile.Bounds()
	for y := dst.Min.Y; y < dst.Max.Y; y += bounds.Dy() {
		for x := dst.Min.X; x < dst.Max.X; x += bounds.Dx() {
			tileRect := image.Rect(x, y, x+bounds.Dx(), y+bounds.Dy())
			draw.Draw(canvas, tileRect.Intersect(dst), tile, bounds.Min, draw.Over)
		}
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
	drawSubMode(canvas, img, r, screenHeight, tc, false)
}

func drawSubMode(canvas *image.RGBA, img image.Image, r Rect, screenHeight float64, tc [4]float64, additive bool) {
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
	if !additive {
		xdraw.BiLinear.Scale(canvas, dst, src, src.Bounds(), xdraw.Over, nil)
		return
	}
	blend := image.NewRGBA(dst)
	xdraw.BiLinear.Scale(blend, blend.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	for y := dst.Min.Y; y < dst.Max.Y; y++ {
		for x := dst.Min.X; x < dst.Max.X; x++ {
			sr, sg, sb, sa := blend.At(x, y).RGBA()
			dr, dg, db, da := canvas.At(x, y).RGBA()
			canvas.SetRGBA(x, y, color.RGBA{R: addChannel(dr, sr), G: addChannel(dg, sg), B: addChannel(db, sb), A: maxChannel(da, sa)})
		}
	}
}

func addChannel(dst, src uint32) uint8 {
	value := int(dst>>8) + int(src>>8)
	if value > 255 {
		value = 255
	}
	return uint8(value)
}

func maxChannel(left, right uint32) uint8 {
	if right > left {
		return uint8(right >> 8)
	}
	return uint8(left >> 8)
}

func drawText(canvas *image.RGBA, face font.Face, text string, r Rect, screenHeight float64, c color.Color) {
	drawTextAligned(canvas, face, text, r, screenHeight, c, "LEFT")
}

func drawTextAligned(canvas *image.RGBA, face font.Face, text string, r Rect, screenHeight float64, c color.Color, justify string) {
	text = cleanText(text)
	dst := ScreenRect(r, screenHeight)
	if dst.Dx() <= 0 || dst.Dy() <= 0 {
		return
	}

	if justify == "" {
		justify = "CENTER"
	}
	ascent := face.Metrics().Ascent.Ceil()
	height := face.Metrics().Height.Ceil()
	lines := strings.Split(text, "\n")
	totalHeight := height * len(lines)
	startY := dst.Min.Y + ascent + (dst.Dy()-totalHeight)/2
	for index, line := range lines {
		width := font.MeasureString(face, line).Ceil()
		dotX := dst.Min.X + 4
		switch strings.ToUpper(justify) {
		case "CENTER":
			dotX = dst.Min.X + (dst.Dx()-width)/2
		case "RIGHT":
			dotX = dst.Max.X - width - 4
		}
		d := &font.Drawer{Dst: canvas, Src: image.NewUniform(c), Face: face, Dot: fixed.P(dotX, startY+index*height)}
		d.DrawString(line)
	}
}

func cleanText(text string) string {
	text = strings.ReplaceAll(text, "|n", "\n")
	text = strings.ReplaceAll(text, "|r", "")
	for {
		index := strings.Index(text, "|c")
		if index < 0 || len(text) < index+10 {
			break
		}
		text = text[:index] + text[index+10:]
	}
	return text
}
