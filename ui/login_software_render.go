package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // register JPEG decoder for image.Decode
	"log"
	"math"
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
	Rt              *Runtime
	FontObj         *opentype.Font
	FontObjSm       *opentype.Font
	AssetLoader     *Loader
	Cache           map[string]image.Image
	BgImagePath     string // Path to a static background image (JPEG/PNG)
	statusKey       string
	rememberMe      bool
	pressed         *widget
	hovered         *widget
	screen          Rect
	uiScale         float64
	screenWidth     int
	screenHeight    int
	rects           map[*widget]Rect
	layoutActive    map[*widget]bool
	textFaces       map[string]font.Face
	movieFile       string
	movieImage      image.Image
	movie           *moviePlayback
	sceneBackground bool
	debugPanel      debugPanelState
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
	if data, err := loader.ReadFile("Interface\\FrameXML\\Sound.lua"); err == nil {
		rt.doFileBody(string(data), "Interface\\FrameXML\\Sound.lua")
	}

	rt.Execute("GlueParent_OnEvent('FRAMES_LOADED')", "@render.lua")
	rt.FireEvent("FRAMES_LOADED")
	rt.Execute("GlueParent_OnEvent('SET_GLUE_SCREEN', 'login')", "@render.lua")
	rt.FireEvent("SET_GLUE_SCREEN", lua.LString("login"))

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

func (eng *UIEngine) Update(elapsed float64) bool {
	if elapsed <= 0 {
		return false
	}
	var update func(*widget)
	update = func(w *widget) {
		if !w.shown {
			return
		}
		eng.Rt.fire(w, "OnUpdate", []lua.LValue{w.luaValue(eng.Rt.L), lua.LNumber(elapsed)})
		for _, child := range w.children {
			update(child)
		}
	}
	if root := eng.Rt.widgets["GlueParent"]; root != nil {
		for _, child := range root.children {
			update(child)
		}
	}
	return eng.updateMovie(elapsed)
}

func (eng *UIEngine) drawMovieFrame(canvas *image.RGBA, dst image.Rectangle, frame image.Image) {
	if dst.Dx() <= 0 || dst.Dy() <= 0 || frame.Bounds().Dx() <= 0 || frame.Bounds().Dy() <= 0 {
		return
	}
	scale := math.Min(float64(dst.Dx())/float64(frame.Bounds().Dx()), float64(dst.Dy())/float64(frame.Bounds().Dy()))
	width := int(float64(frame.Bounds().Dx()) * scale)
	height := int(float64(frame.Bounds().Dy()) * scale)
	left := dst.Min.X + (dst.Dx()-width)/2
	top := dst.Min.Y + (dst.Dy()-height)/2
	xdraw.BiLinear.Scale(canvas, image.Rect(left, top, left+width, top+height), frame, frame.Bounds(), xdraw.Src, nil)
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
	eng.textFaces = make(map[string]font.Face)
	defer func() {
		for _, textFace := range eng.textFaces {
			textFace.Close()
		}
	}()
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
				f := eng.faceFor(w, face, faceLg)
				if !w.autoTextWidth {
					text = strings.Join(wrapText(text, f, int(scaledRect.W())), "\n")
				}
				if w.maxLines > 0 {
					lines := strings.Split(text, "\n")
					if len(lines) > w.maxLines {
						text = strings.Join(lines[:w.maxLines], "\n")
					}
				}
				eng.drawTextAlignedWidget(canvas, f, text, scaledRect, float64(screenHeight), c, w)
			}

		case kindMovieFrame:
			if w.movieActive {
				eng.ensureMovie(w.movieFile, float64(w.movieVolume)/255)
				dst := ScreenRect(rect, float64(canvas.Bounds().Dy()))
				draw.Draw(canvas, dst, &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
				if eng.movieImage != nil {
					eng.drawMovieFrame(canvas, dst, eng.movieImage)
				}
			}
		}

		if w.kind == kindButton || w.kind == kindCheckButton {
			for _, child := range w.children {
				if child.shown && child.layerLevel < layerArtwork {
					paint(child, rect)
				}
			}
			eng.paintButtonState(w, rect, paint)
			for _, child := range w.children {
				if child.shown && child.layerLevel >= layerArtwork {
					paint(child, rect)
				}
			}
		} else {
			for _, child := range w.children {
				if child.shown && !(w.kind == kindEditBox && w.text != "" && child.kind == kindFontString && strings.HasSuffix(strings.ToLower(child.name), "fill")) {
					paint(child, rect)
				}
			}
		}

		if w.kind == kindButton || w.kind == kindCheckButton {
			if w.buttonLabel == nil {
				text := eng.resolveText(w.text)
				if text != "" {
					eng.drawTextAlignedWidget(canvas, face, text, scaledRect, float64(screenHeight), eng.fontColor(w), w)
				}
			}
		}
		if w.kind == kindEditBox {
			eng.drawEditText(canvas, face, faceLg, w, rect, float64(screenHeight))
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
	if eng.sceneBackground {
		for index := 3; index < len(canvas.Pix); index += 4 {
			if canvas.Pix[index] == 0 {
				canvas.Pix[index] = 1
			}
		}
	}
	if eng.debugPanel.visible {
		eng.drawDebugPanel(canvas, face, faceLg)
	}

	return canvas
}

func (eng *UIEngine) prepareText(w *widget, face, faceLg font.Face) {
	if w.kind == kindFontString {
		text := eng.resolveText(w.text)
		if w.parent != nil && (w.parent.kind == kindButton || w.parent.kind == kindCheckButton) && w.parent.buttonLabel == w && w.parent.text != "" {
			text = eng.resolveText(w.parent.text)
		}
		fontFace := eng.faceFor(w, face, faceLg)
		lines := strings.Split(cleanText(text), "\n")
		if w.autoTextWidth {
			maxWidth := 0
			for _, line := range lines {
				if width := font.MeasureString(fontFace, line).Ceil(); width > maxWidth {
					maxWidth = width
				}
			}
			w.textWidth = float64(maxWidth) / eng.uiScale
			w.width = float64(maxWidth) / eng.uiScale
		} else {
			maxWidth := 0
			for _, line := range lines {
				if width := font.MeasureString(fontFace, line).Ceil(); width > maxWidth {
					maxWidth = width
				}
			}
			w.textWidth = float64(maxWidth) / eng.uiScale
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
		w.renderRect = rect
		w.hasRenderRect = true
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
	w.renderRect = rect
	w.hasRenderRect = true
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

func (eng *UIEngine) editTextWidget(w *widget) *widget {
	for _, child := range w.children {
		if child.kind == kindFontString {
			return child
		}
	}
	return w
}

func editDisplayText(w *widget) string {
	if w.password {
		return strings.Repeat("*", len([]rune(w.text)))
	}
	return w.text
}

func (eng *UIEngine) editTextRect(rect Rect, w *widget) Rect {
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
	return Rect{X0: rect.X0 + left, Y0: rect.Y0 + bottom, X1: rect.X1 - right, Y1: rect.Y1 - top}
}

func (eng *UIEngine) editFace(w *widget) (font.Face, func()) {
	textWidget := eng.editTextWidget(w)
	style := eng.fontStyle(textWidget)
	fontObj := eng.FontObj
	size := 13.0
	if style != nil {
		if style.Height > 0 {
			size = style.Height
		}
		if strings.Contains(strings.ToLower(style.FontFile), "morpheus") {
			fontObj = eng.FontObjSm
		}
	}
	if fontObj == nil {
		return nil, func() {}
	}
	face, err := opentype.NewFace(fontObj, &opentype.FaceOptions{Size: size * eng.uiScale, DPI: 96})
	if err != nil {
		return nil, func() {}
	}
	return face, func() { face.Close() }
}

func (eng *UIEngine) setEditCursor(w *widget, x float64) {
	if w == nil || w.kind != kindEditBox {
		return
	}
	rect, ok := eng.rects[w]
	if !ok {
		parent := eng.screen
		if w.parent != nil {
			parent = eng.layoutRect(w.parent, eng.screen)
		}
		rect = eng.layoutRect(w, parent)
	}
	textRect := eng.editTextRect(rect, w)
	scale := eng.uiScale
	if scale <= 0 {
		scale = 1
	}
	position := (x - textRect.X0*scale) / scale
	display := []rune(editDisplayText(w))
	face, release := eng.editFace(w)
	defer release()
	if face == nil {
		moveEditCursor(w, int(math.Round(position/8)), false)
		return
	}
	for index := 0; index < len(display); index++ {
		left := float64(font.MeasureString(face, string(display[:index])).Ceil())
		right := float64(font.MeasureString(face, string(display[:index+1])).Ceil())
		if position < (left+right)/2 {
			moveEditCursor(w, index, false)
			return
		}
	}
	moveEditCursor(w, len(display), false)
}

func (eng *UIEngine) drawEditText(canvas *image.RGBA, face, faceLg font.Face, w *widget, rect Rect, screenHeight float64) {
	text := editDisplayText(w)
	textWidget := eng.editTextWidget(w)
	textFace := eng.faceFor(textWidget, face, faceLg)
	textRect := eng.editTextRect(rect, w)
	screenTextRect := screenScaledRect(textRect, eng.uiScale)
	textColor := color.RGBA{R: 235, G: 235, B: 235, A: 255}
	if !textWidget.textColor.isZero() {
		textColor = color.RGBA{R: uint8(textWidget.textColor.r * 255), G: uint8(textWidget.textColor.g * 255), B: uint8(textWidget.textColor.b * 255), A: uint8(textWidget.textColor.a * 255)}
	}
	start, end := editSelection(w)
	if eng.Rt.focused == w && start != end {
		startWidth := font.MeasureString(textFace, string([]rune(text)[:start])).Ceil()
		endWidth := font.MeasureString(textFace, string([]rune(text)[:end])).Ceil()
		dst := ScreenRect(screenTextRect, screenHeight)
		selection := image.Rect(dst.Min.X+startWidth, dst.Min.Y+2, dst.Min.X+endWidth, dst.Max.Y-2)
		draw.Draw(canvas, selection, &image.Uniform{C: color.RGBA{R: 35, G: 100, B: 180, A: 180}}, image.Point{}, draw.Over)
	}
	if text != "" {
		eng.drawTextAlignedWidget(canvas, textFace, text, screenTextRect, screenHeight, textColor, textWidget)
	}
	if eng.Rt.focused == w {
		width := font.MeasureString(textFace, string([]rune(text)[:clampCursor(w.cursor, len([]rune(text)))])).Ceil()
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

func (eng *UIEngine) SetSceneBackground(enabled bool) { eng.sceneBackground = enabled }

func (eng *UIEngine) CurrentModelPath() string {
	if frame := eng.Rt.widgets["CharacterSelect"]; frame != nil && frame.shown {
		if path, ok := eng.Rt.GetCVar("charSelectBackground"); ok && path != "" {
			return path
		}
	}
	if frame := eng.Rt.widgets["CharacterCreate"]; frame != nil && frame.shown {
		if path, ok := eng.Rt.GetCVar("charCustomizeBackground"); ok && path != "" {
			return path
		}
	}
	if frame := eng.Rt.widgets["AccountLogin"]; frame != nil && frame.shown && frame.modelFile != "" {
		return frame.modelFile
	}
	if value, ok := eng.Rt.GetCVar("currentGlueScreen"); ok {
		switch strings.ToLower(value) {
		case "charselect":
			if path, ok := eng.Rt.GetCVar("charSelectBackground"); ok && path != "" {
				return path
			}
		case "charcreate":
			if path, ok := eng.Rt.GetCVar("charCustomizeBackground"); ok && path != "" {
				return path
			}
		}
	}
	var path string
	var visit func(*widget)
	visit = func(w *widget) {
		if path != "" || !w.shown {
			return
		}
		if (w.kind == kindModel || w.kind == kindModelFFX) && w.modelFile != "" {
			path = w.modelFile
			return
		}
		for _, child := range w.children {
			visit(child)
		}
	}
	if root := eng.Rt.widgets["GlueParent"]; root != nil {
		for _, child := range root.children {
			visit(child)
		}
	}
	return path
}

func (eng *UIEngine) SetInitialCredentials(account, password string, rememberMe bool) {
	eng.rememberMe = rememberMe
	eng.Rt.SetCVar("accountName", account)
	eng.Rt.SetCVar("currentGlueScreen", "login")
	eng.Rt.Execute("for _, name in ipairs({'VideoOptionsFrame', 'AudioOptionsFrame', 'OptionsSelectFrame', 'CinematicsFrame', 'MovieFrame', 'RealmList', 'AddonList', 'GlueDialog'}) do local frame = _G[name]; if frame then frame:Hide() end end", "@credentials.lua")
	eng.Rt.Execute("GlueParent_OnEvent('SET_GLUE_SCREEN', 'login')", "@credentials.lua")
	eng.Rt.Execute("SetGlueScreen('login')", "@credentials.lua")
	eng.Rt.FireEvent("SET_GLUE_SCREEN", lua.LString("login"))
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
	if len(state.AddOns) == 0 {
		state.AddOns = eng.Rt.Glue.AddOns
	}
	eng.Rt.Glue = state
	eng.statusKey = ""
	eng.Rt.SetCVar("currentGlueScreen", "charselect")
	eng.Rt.Execute("for _, name in ipairs({'VideoOptionsFrame', 'AudioOptionsFrame', 'OptionsSelectFrame', 'CinematicsFrame', 'MovieFrame', 'RealmList', 'AddonList', 'GlueDialog'}) do local frame = _G[name]; if frame then frame:Hide() end end", "@network.lua")
	eng.Rt.Execute("GlueParent_OnEvent('SET_GLUE_SCREEN', 'charselect')", "@network.lua")
	eng.Rt.Execute("SetGlueScreen('charselect')", "@network.lua")
	eng.Rt.FireEvent("SET_GLUE_SCREEN", lua.LString("charselect"))
	eng.Rt.FireEvent("CHARACTER_LIST_UPDATE")
}

func (eng *UIEngine) HandleCursor(x, y float64) bool {
	if eng.debugPanel.dragging {
		eng.debugPanel.move(x, y, eng)
		if eng.hovered != nil {
			eng.hovered.highlighted = false
			eng.hovered = nil
		}
		return true
	}
	if eng.debugPanel.contains(x, y, eng) {
		if eng.hovered != nil {
			eng.hovered.highlighted = false
			eng.hovered = nil
			return true
		}
		return false
	}
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
	if eng.debugPanel.handleMouse(x, y, down, eng) {
		return true
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
			eng.setEditCursor(target, x)
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
	start, end := editSelection(w)
	if w.maxLetters > 0 && len(runes)-(end-start)+1 > w.maxLetters {
		return true
	}
	updated := make([]rune, 0, len(runes)-(end-start)+1)
	updated = append(updated, runes[:start]...)
	updated = append(updated, char)
	updated = append(updated, runes[end:]...)
	if w.maxBytes > 0 && len([]byte(string(updated))) > w.maxBytes {
		return true
	}
	position := start + 1
	eng.Rt.setText(w, string(updated))
	w.cursor = position
	w.selectionStart = position
	w.selectionEnd = position
	w.selectionAnchor = position
	return true
}

func clampCursor(value, length int) int {
	if value < 0 {
		return 0
	}
	if value > length {
		return length
	}
	return value
}

func editSelection(w *widget) (int, int) {
	length := len([]rune(w.text))
	start := clampCursor(w.selectionStart, length)
	end := clampCursor(w.selectionEnd, length)
	if start > end {
		start, end = end, start
	}
	return start, end
}

func moveEditCursor(w *widget, position int, extend bool) {
	position = clampCursor(position, len([]rune(w.text)))
	if !extend {
		w.cursor = position
		w.selectionStart = position
		w.selectionEnd = position
		w.selectionAnchor = position
		return
	}
	if w.selectionStart == w.selectionEnd {
		w.selectionAnchor = w.cursor
		w.selectionStart = w.cursor
	}
	w.cursor = position
	w.selectionEnd = position
}

func collapseEditSelection(w *widget, left bool) bool {
	start, end := editSelection(w)
	if start == end {
		return false
	}
	if left {
		w.cursor = start
	} else {
		w.cursor = end
	}
	w.selectionStart = w.cursor
	w.selectionEnd = w.cursor
	w.selectionAnchor = w.cursor
	return true
}

func deleteEditSelection(eng *UIEngine, w *widget) bool {
	start, end := editSelection(w)
	if start == end {
		return false
	}
	runes := []rune(w.text)
	runes = append(runes[:start], runes[end:]...)
	eng.Rt.setText(w, string(runes))
	w.cursor = start
	w.selectionStart = start
	w.selectionEnd = start
	w.selectionAnchor = start
	return true
}

func (eng *UIEngine) HandleKey(key window.Key) bool { return eng.handleKey(key, false) }

func (eng *UIEngine) handleKey(key window.Key, extendSelection bool) bool {
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
			if !extendSelection && collapseEditSelection(w, true) {
				break
			}
			moveEditCursor(w, w.cursor-1, extendSelection)
		case window.KeyRight:
			if !extendSelection && collapseEditSelection(w, false) {
				break
			}
			moveEditCursor(w, w.cursor+1, extendSelection)
		case window.KeyHome:
			moveEditCursor(w, 0, extendSelection)
		case window.KeyEnd:
			moveEditCursor(w, len([]rune(w.text)), extendSelection)
		case window.KeyBackspace:
			if deleteEditSelection(eng, w) {
				break
			}
			if w.cursor > 0 {
				runes := []rune(w.text)
				runes = append(runes[:w.cursor-1], runes[w.cursor:]...)
				w.cursor--
				eng.Rt.setText(w, string(runes))
				w.selectionStart = w.cursor
				w.selectionEnd = w.cursor
				w.selectionAnchor = w.cursor
			}
		case window.KeyDelete:
			if deleteEditSelection(eng, w) {
				break
			}
			runes := []rune(w.text)
			if w.cursor < len(runes) {
				runes = append(runes[:w.cursor], runes[w.cursor+1:]...)
				eng.Rt.setText(w, string(runes))
				w.selectionStart = w.cursor
				w.selectionEnd = w.cursor
				w.selectionAnchor = w.cursor
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

func (eng *UIEngine) HandleKeyWithMods(key window.Key, mods window.ModifierKey) bool {
	if key == window.KeyF2 {
		eng.ToggleDebugPanel()
		return true
	}
	if w := eng.Rt.focused; w != nil && w.kind == kindEditBox {
		if mods&window.ModControl != 0 && key == window.KeyA {
			w.selectionStart = 0
			w.selectionEnd = len([]rune(w.text))
			w.selectionAnchor = 0
			w.cursor = w.selectionEnd
			return true
		}
		if mods&window.ModShift != 0 {
			return eng.handleKey(key, true)
		}
	}
	if eng.Rt.focused == nil && mods&window.ModControl != 0 {
		switch key {
		case window.KeyM:
			eng.Rt.Execute("Sound_ToggleMusic()", "@bindings.lua")
			return true
		case window.KeyS:
			eng.Rt.Execute("Sound_ToggleSound()", "@bindings.lua")
			return true
		}
	}
	return eng.HandleKey(key)
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
	if target.name == "MovieFrame" && !target.shown {
		eng.Rt.Execute("SetGlueScreen('login')", "@movie.lua")
		eng.Rt.FireEvent("SET_GLUE_SCREEN", lua.LString("login"))
	}
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
	if eng.sceneBackground {
		return
	}
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
		if !bd.bgColor.isZero() {
			bgImg = eng.tintBackdropImage(bd.bgFile, bgImg, bd.bgColor)
		}
		// Tile the background inside insets
		inL := int(bd.insetL * eng.uiScale)
		inR := int(bd.insetR * eng.uiScale)
		inT := int(bd.insetT * eng.uiScale)
		inB := int(bd.insetB * eng.uiScale)
		inner := image.Rect(dst.Min.X+inL, dst.Min.Y+inT, dst.Max.X-inR, dst.Max.Y-inB)
		if inner.Dx() > 0 && inner.Dy() > 0 {
			if !bd.bgColor.isZero() {
				bgCol := color.RGBA{R: uint8(bd.bgColor.r * 255), G: uint8(bd.bgColor.g * 255), B: uint8(bd.bgColor.b * 255), A: uint8(bd.bgColor.a * 255)}
				draw.Draw(canvas, inner, &image.Uniform{C: bgCol}, image.Point{}, draw.Over)
			}
			if bd.tile {
				eng.drawTiled(canvas, inner, bgImg, bd.tileSize)
			} else {
				xdraw.NearestNeighbor.Scale(canvas, inner, bgImg, bgImg.Bounds(), xdraw.Over, nil)
			}
		}
	} else if bd.bgFile != "" {
		// Fallback: solid very dark box
		draw.Draw(canvas, dst, &image.Uniform{C: color.RGBA{R: 8, G: 15, B: 25, A: 200}}, image.Point{}, draw.Over)
	}

	// Draw border
	edgeImg := eng.loadBLP(bd.edgeFile)
	if edgeImg != nil {
		if !bd.edgeColor.isZero() {
			edgeImg = eng.tintBackdropImage(bd.edgeFile, edgeImg, bd.edgeColor)
		}
		drawBackdropEdge(canvas, dst, edgeImg, bd.edgeSize*eng.uiScale)
	} else if bd.edgeFile != "" {
		drawBorder(canvas, dst, color.RGBA{R: 80, G: 120, B: 150, A: 200}, 1)
	}
}

func (eng *UIEngine) tintBackdropImage(path string, source image.Image, tint rgba) image.Image {
	key := fmt.Sprintf("backdrop:%s:%.4f:%.4f:%.4f:%.4f", path, tint.r, tint.g, tint.b, tint.a)
	if imageData, ok := eng.Cache[key]; ok {
		return imageData
	}
	imageData := tintImage(source, tint)
	eng.Cache[key] = imageData
	return imageData
}

func tintImage(source image.Image, tint rgba) image.Image {
	bounds := source.Bounds()
	result := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	red := clampImageChannel(tint.r)
	green := clampImageChannel(tint.g)
	blue := clampImageChannel(tint.b)
	alpha := clampImageChannel(tint.a)
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			pixel := color.NRGBAModel.Convert(source.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			result.SetNRGBA(x, y, color.NRGBA{R: uint8(float64(pixel.R) * red), G: uint8(float64(pixel.G) * green), B: uint8(float64(pixel.B) * blue), A: uint8(float64(pixel.A) * alpha)})
		}
	}
	return result
}

func clampImageChannel(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func drawBackdropEdge(canvas *image.RGBA, dst image.Rectangle, source image.Image, edgeSize float64) {
	b := source.Bounds()
	if b.Dx() < 8 || b.Dy() < 1 || dst.Dx() < 2 || dst.Dy() < 2 {
		return
	}
	tileWidth := b.Dx() / 8
	tileHeight := b.Dy()
	if tileWidth < 1 || tileHeight < 1 {
		return
	}
	edge := int(edgeSize)
	if edge <= 0 {
		edge = tileHeight
	}
	if edge > dst.Dx()/2 {
		edge = dst.Dx() / 2
	}
	if edge > dst.Dy()/2 {
		edge = dst.Dy() / 2
	}
	if edge < 1 {
		return
	}
	drawPart := func(target image.Rectangle, index int, transpose bool, fraction float64) {
		if target.Dx() <= 0 || target.Dy() <= 0 {
			return
		}
		fraction = math.Max(0, math.Min(1, fraction))
		mapped := image.NewNRGBA(image.Rect(0, 0, target.Dx(), target.Dy()))
		for y := 0; y < target.Dy(); y++ {
			for x := 0; x < target.Dx(); x++ {
				var sourceX, sourceY int
				if transpose {
					sourceX = int(float64(y) / float64(target.Dy()) * float64(tileWidth))
					sourceY = int(float64(x) / float64(target.Dx()) * float64(tileHeight) * fraction)
				} else {
					sourceX = int(float64(x) / float64(target.Dx()) * float64(tileWidth))
					sourceY = int(float64(y) / float64(target.Dy()) * float64(tileHeight) * fraction)
				}
				if sourceX >= tileWidth {
					sourceX = tileWidth - 1
				}
				if sourceY >= tileHeight {
					sourceY = tileHeight - 1
				}
				mapped.Set(x, y, source.At(b.Min.X+index*tileWidth+sourceX, b.Min.Y+sourceY))
			}
		}
		draw.Draw(canvas, target, mapped, image.Point{}, draw.Over)
	}
	run := func(index int, target image.Rectangle, vertical bool, transpose bool) {
		span := target.Dx()
		if vertical {
			span = target.Dy()
		}
		if span <= 0 {
			return
		}
		step := edge
		if span/step > 64 {
			step = int(math.Ceil(float64(span) / 64))
		}
		for at := 0; at < span; at += step {
			length := step
			if remaining := span - at; remaining < length {
				length = remaining
			}
			fraction := float64(length) / float64(step)
			if vertical {
				drawPart(image.Rect(target.Min.X, target.Min.Y+at, target.Max.X, target.Min.Y+at+length), index, transpose, fraction)
			} else {
				drawPart(image.Rect(target.Min.X+at, target.Min.Y, target.Min.X+at+length, target.Max.Y), index, transpose, fraction)
			}
		}
	}
	tx0, tx1 := dst.Min.X, dst.Max.X
	ty0, ty1 := dst.Min.Y, dst.Max.Y
	run(0, image.Rect(tx0, ty0+edge, tx0+edge, ty1-edge), true, false)
	run(1, image.Rect(tx1-edge, ty0+edge, tx1, ty1-edge), true, false)
	run(2, image.Rect(tx0+edge, ty0, tx1-edge, ty0+edge), false, true)
	run(3, image.Rect(tx0+edge, ty1-edge, tx1-edge, ty1), false, true)
	drawPart(image.Rect(tx0, ty0, tx0+edge, ty0+edge), 4, false, 1)
	drawPart(image.Rect(tx1-edge, ty0, tx1, ty0+edge), 5, false, 1)
	drawPart(image.Rect(tx0, ty1-edge, tx0+edge, ty1), 6, false, 1)
	drawPart(image.Rect(tx1-edge, ty1-edge, tx1, ty1), 7, false, 1)
}

func (eng *UIEngine) drawTiled(canvas *image.RGBA, dst image.Rectangle, source image.Image, tileSize float64) {
	tile := source
	if tileSize > 0 {
		size := int(tileSize * eng.uiScale)
		if size > 0 {
			tileImage := image.NewRGBA(image.Rect(0, 0, size, size))
			xdraw.NearestNeighbor.Scale(tileImage, tileImage.Bounds(), source, source.Bounds(), xdraw.Src, nil)
			tile = tileImage
		}
	}
	bounds := tile.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return
	}
	for y := dst.Min.Y; y < dst.Max.Y; y += bounds.Dy() {
		for x := dst.Min.X; x < dst.Max.X; x += bounds.Dx() {
			tileRect := image.Rect(x, y, x+bounds.Dx(), y+bounds.Dy())
			draw.Draw(canvas, tileRect.Intersect(dst), tile, bounds.Min, draw.Over)
		}
	}
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
	if style := eng.fontStyle(w); style != nil && !style.Color.isZero() {
		return color.RGBA{R: uint8(style.Color.r * 255), G: uint8(style.Color.g * 255), B: uint8(style.Color.b * 255), A: 255}
	}
	// Default WoW glue label is yellow-gold
	return color.RGBA{R: 255, G: 210, B: 0, A: 255}
}

func (eng *UIEngine) fontStyle(w *widget) *Font {
	name := w.fontObject
	if w.parent != nil && (w.parent.kind == kindButton || w.parent.kind == kindCheckButton) && w.parent.buttonLabel == w {
		switch {
		case !w.parent.enabled && w.parent.disabledFont != "":
			name = w.parent.disabledFont
		case w.parent.highlighted && w.parent.highlightFont != "":
			name = w.parent.highlightFont
		case w.parent.normalFont != "":
			name = w.parent.normalFont
		}
	}
	return eng.Rt.fonts[name]
}

func (eng *UIEngine) faceFor(w *widget, fallback, fallbackLarge font.Face) font.Face {
	style := eng.fontStyle(w)
	size := 13.0
	fontObj := eng.FontObj
	fontKey := "FRIZQT__.TTF"
	if style != nil {
		if style.Height > 0 {
			size = style.Height
		}
		if style.FontFile != "" {
			fontKey = style.FontFile
		}
		if strings.Contains(strings.ToLower(style.FontFile), "morpheus") {
			fontObj = eng.FontObjSm
		}
	} else if strings.Contains(strings.ToLower(w.fontObject), "large") || strings.Contains(strings.ToLower(w.fontObject), "huge") {
		return fallbackLarge
	}
	if size == 13 && style == nil {
		return fallback
	}
	key := fmt.Sprintf("%s|%.3f", fontKey, size*eng.uiScale)
	if textFace, ok := eng.textFaces[key]; ok {
		return textFace
	}
	textFace, err := opentype.NewFace(fontObj, &opentype.FaceOptions{Size: size * eng.uiScale, DPI: 96})
	if err != nil {
		return fallback
	}
	eng.textFaces[key] = textFace
	return textFace
}

func (eng *UIEngine) textJustify(w *widget) string {
	if w.justifyH != "" {
		return w.justifyH
	}
	if style := eng.fontStyle(w); style != nil && style.JustifyH != "" {
		return style.JustifyH
	}
	return "CENTER"
}

func (eng *UIEngine) textVerticalJustify(w *widget) string {
	if w.justifyV != "" {
		return w.justifyV
	}
	if style := eng.fontStyle(w); style != nil && style.JustifyV != "" {
		return style.JustifyV
	}
	return "CENTER"
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
			if sr == 0 && sg == 0 && sb == 0 {
				continue
			}
			dr, dg, db, da := canvas.At(x, y).RGBA()
			canvas.SetRGBA(x, y, color.RGBA{R: addChannel(dr, scaleChannel(sr, sa)), G: addChannel(dg, scaleChannel(sg, sa)), B: addChannel(db, scaleChannel(sb, sa)), A: maxChannel(da, sa)})
		}
	}
}

func scaleChannel(value, alpha uint32) uint32 {
	return uint32((uint64(value) * uint64(alpha)) / 0xffff)
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
	dst := ScreenRect(r, screenHeight)
	text = strings.Join(wrapText(text, face, dst.Dx()), "\n")
	drawTextAlignedV(canvas, face, text, r, screenHeight, c, justify, "CENTER")
}

func drawTextAlignedV(canvas *image.RGBA, face font.Face, text string, r Rect, screenHeight float64, c color.Color, justify, vertical string) {
	drawTextAlignedVStyle(canvas, face, text, r, screenHeight, c, justify, vertical, nil, 1)
}

func (eng *UIEngine) drawTextAlignedWidget(canvas *image.RGBA, face font.Face, text string, r Rect, screenHeight float64, c color.Color, w *widget) {
	style := eng.fontStyle(w)
	if style == nil && !w.shadowColorSet && !w.shadowOffsetSet {
		drawTextAlignedV(canvas, face, text, r, screenHeight, c, eng.textJustify(w), eng.textVerticalJustify(w))
		return
	}
	shadow := &Font{}
	if style != nil {
		*shadow = *style
	}
	if w.shadowColorSet {
		shadow.Shadow = true
		shadow.ShadowColor = w.shadowColor
	}
	if w.shadowOffsetSet {
		shadow.Shadow = true
		shadow.ShadowOffsetX = w.shadowOffsetX
		shadow.ShadowOffsetY = w.shadowOffsetY
	}
	if shadow.ShadowColor.isZero() {
		shadow.ShadowColor = rgba{r: 0, g: 0, b: 0, a: 1}
	}
	drawTextAlignedVStyle(canvas, face, text, r, screenHeight, c, eng.textJustify(w), eng.textVerticalJustify(w), shadow, eng.uiScale)
}

func drawTextAlignedVStyle(canvas *image.RGBA, face font.Face, text string, r Rect, screenHeight float64, c color.Color, justify, vertical string, shadow *Font, shadowScale float64) {
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
	switch strings.ToUpper(vertical) {
	case "TOP":
		startY = dst.Min.Y + ascent
	case "BOTTOM":
		startY = dst.Max.Y - totalHeight + ascent
	}
	for index, line := range lines {
		width := font.MeasureString(face, line).Ceil()
		dotX := dst.Min.X + 4
		switch strings.ToUpper(justify) {
		case "CENTER":
			dotX = dst.Min.X + (dst.Dx()-width)/2
		case "RIGHT":
			dotX = dst.Max.X - width - 4
		}
		if shadow != nil && shadow.Shadow {
			shadowColor := color.RGBA{R: uint8(shadow.ShadowColor.r * 255), G: uint8(shadow.ShadowColor.g * 255), B: uint8(shadow.ShadowColor.b * 255), A: uint8(shadow.ShadowColor.a * 255)}
			shadowX := int(math.Round(shadow.ShadowOffsetX * shadowScale))
			shadowY := int(math.Round(-shadow.ShadowOffsetY * shadowScale))
			d := &font.Drawer{Dst: canvas, Src: image.NewUniform(shadowColor), Face: face, Dot: fixed.P(dotX+shadowX, startY+index*height+shadowY)}
			d.DrawString(line)
		}
		d := &font.Drawer{Dst: canvas, Src: image.NewUniform(c), Face: face, Dot: fixed.P(dotX, startY+index*height)}
		d.DrawString(line)
	}
}

func wrapText(text string, face font.Face, maxWidth int) []string {
	paragraphs := strings.Split(text, "\n")
	if maxWidth <= 0 {
		return paragraphs
	}
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		words := strings.Fields(paragraph)
		line := ""
		for _, word := range words {
			candidate := word
			if line != "" {
				candidate = line + " " + word
			}
			if font.MeasureString(face, candidate).Ceil() <= maxWidth || line == "" && font.MeasureString(face, candidate).Ceil() <= maxWidth {
				line = candidate
				continue
			}
			if line != "" {
				lines = append(lines, line)
			}
			line = word
			for font.MeasureString(face, line).Ceil() > maxWidth {
				runes := []rune(line)
				if len(runes) <= 1 {
					break
				}
				cut := len(runes) - 1
				for cut > 1 && font.MeasureString(face, string(runes[:cut])).Ceil() > maxWidth {
					cut--
				}
				lines = append(lines, string(runes[:cut]))
				line = string(runes[cut:])
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
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
