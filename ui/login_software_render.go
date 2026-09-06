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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/g3n/engine/window"
	lua "github.com/yuin/gopher-lua"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type UIEngine struct {
	Rt               *Runtime
	FontObj          *opentype.Font
	FontObjSm        *opentype.Font
	FontObjArial     *opentype.Font
	AssetLoader      *Loader
	Cache            map[string]image.Image
	BgImagePath      string // Path to a static background image (JPEG/PNG)
	statusKey        string
	statusText       string
	statusTTL        float64
	statusDialogType string
	rememberMe       bool
	pressed          *widget
	sliderDragging   *widget
	selecting        *widget
	hovered          *widget
	screen           Rect
	uiScale          float64
	screenWidth      int
	screenHeight     int
	rects            map[*widget]Rect
	layoutActive     map[*widget]bool
	textFaces        map[string]font.Face
	layerPool        []*image.RGBA
	layerDepth       int
	paintCanvas      *image.RGBA
	movieFile        string
	movieImage       image.Image
	movie            *moviePlayback
	sceneBackground  bool
	debugPanel       debugPanelState
	lastClick        *widget
	lastClickAt      time.Time
	loading          bool
	loadingPath      string
	loadingProgress  float64
	worldRoot        *widget
	worldUIReady     bool
	worldLoading     bool
	worldActive      bool
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
	// ChatFontNormal uses Fonts\\ARIALN.TTF in live Fonts.xml.
	var fontObjArial *opentype.Font
	if arialData, err2 := loader.readAsset("Fonts\\ARIALN.TTF"); err2 == nil {
		if ao, err3 := opentype.Parse(arialData); err3 == nil {
			fontObjArial = ao
		}
	}
	if fontObjArial == nil {
		fontObjArial = fontObj
	}

	cache := map[string]image.Image{}

	eng := &UIEngine{
		Rt:           rt,
		FontObj:      fontObj,
		FontObjSm:    fontObjSm,
		FontObjArial: fontObjArial,
		AssetLoader:  loader,
		Cache:        cache,
		textFaces:    make(map[string]font.Face),
		BgImagePath:  bgImagePath,
		uiScale:      1,
	}
	rt.measureText = eng.measureFontString
	rt.loadAddOn = eng.loadAddOn
	return eng, nil
}

func (eng *UIEngine) Close() {
	for _, textFace := range eng.textFaces {
		textFace.Close()
	}
	eng.textFaces = nil
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
	statusChanged := false
	if eng.statusTTL > 0 {
		eng.statusTTL -= elapsed
		if eng.statusTTL <= 0 {
			eng.closeStatusDialog()
			statusChanged = true
		}
	}
	if eng.Rt != nil && eng.Rt.tickLogout(elapsed) {
		statusChanged = true
	}
	var update func(*widget)
	update = func(w *widget) {
		if !w.shown {
			return
		}
		// Most Glue/FrameXML widgets have no OnUpdate script; skip building
		// Lua args and the handler lookup for those nodes each frame.
		if _, ok := w.scripts["OnUpdate"]; ok {
			eng.Rt.fire(w, "OnUpdate", []lua.LValue{w.luaValue(eng.Rt.L), lua.LNumber(elapsed)})
		}
		for _, child := range w.children {
			update(child)
		}
	}
	if root := eng.Rt.widgets["GlueParent"]; root != nil {
		for _, child := range root.children {
			update(child)
		}
	}
	if eng.worldRoot != nil {
		if chat := eng.Rt.widgets["ChatFrame1"]; chat != nil {
			update(chat)
		}
	}
	return statusChanged || eng.updateMovie(elapsed)
}

func (eng *UIEngine) drawMovieFrame(canvas *image.RGBA, dst image.Rectangle, frame image.Image) {
	if dst.Dx() <= 0 || dst.Dy() <= 0 || frame.Bounds().Dx() <= 0 || frame.Bounds().Dy() <= 0 {
		return
	}
	xdraw.BiLinear.Scale(canvas, dst, frame, frame.Bounds(), xdraw.Src, nil)
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
	eng.worldActive = false
	return eng.render(screenWidth, screenHeight, eng.Rt.widgets["GlueParent"], true)
}

func (eng *UIEngine) RenderWorld(screenWidth, screenHeight int) *image.RGBA {
	eng.worldActive = true
	eng.syncCombatLogButtons()
	return eng.render(screenWidth, screenHeight, eng.worldRoot, false)
}

func (eng *UIEngine) render(screenWidth, screenHeight int, root *widget, drawBackground bool) *image.RGBA {
	if screenWidth < 1 {
		screenWidth = 1
	}
	if screenHeight < 1 {
		screenHeight = 1
	}
	bounds := image.Rect(0, 0, screenWidth, screenHeight)
	if eng.paintCanvas == nil || !eng.paintCanvas.Bounds().Eq(bounds) {
		eng.paintCanvas = image.NewRGBA(bounds)
	} else {
		clear(eng.paintCanvas.Pix)
	}
	canvas := eng.paintCanvas
	if eng.rects == nil {
		eng.rects = make(map[*widget]Rect)
	} else {
		clear(eng.rects)
	}
	if eng.layoutActive == nil {
		eng.layoutActive = make(map[*widget]bool)
	} else {
		clear(eng.layoutActive)
	}

	uiScale := float64(screenHeight) / 768.0
	eng.uiScale = uiScale
	eng.Rt.SetCVar("uiScale", fmt.Sprintf("%.6f", uiScale))
	eng.screenWidth = screenWidth
	eng.screenHeight = screenHeight
	eng.layerDepth = 0
	eng.screen = Rect{X0: 0, Y0: 0, X1: float64(screenWidth) / uiScale, Y1: 768}

	face := eng.cachedFace("__base13", eng.FontObj, 13*uiScale)
	faceLg := eng.cachedFace("__base16", eng.FontObj, 16*uiScale)
	if root != nil {
		eng.prepareText(root, face, faceLg)
	}
	eng.updateScrollFrames()

	virtualWidth := eng.screen.W()
	virtualHeight := eng.screen.H()

	// Update host screen dimensions dynamically
	if host, ok := eng.Rt.Host.(*hostScreen); ok {
		host.w = virtualWidth
		host.h = virtualHeight
	}

	if eng.loading {
		eng.renderLoadingScreen(canvas)
		return canvas
	}
	// Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬ Step 1: Render background from BLP sky textures Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬
	if drawBackground {
		eng.renderBackground(canvas, screenWidth, screenHeight)
	}

	screen := eng.screen

	var paint func(*image.RGBA, *widget, Rect)
	var paintWidget func(*image.RGBA, *widget, Rect)
	paintChildren := func(target *image.RGBA, w *widget, rect Rect) {
		if w.kind == kindScrollFrame {
			children := eng.acquireLayer(target.Bounds())
			for _, child := range orderedChildren(w.children) {
				if child.shown && child == w.scrollChild {
					paint(children, child, rect)
				}
			}
			clip := ScreenRect(screenScaledRect(rect, eng.uiScale), float64(target.Bounds().Dy())).Intersect(target.Bounds())
			if !clip.Empty() {
				draw.Draw(target, clip, children, clip.Min, draw.Over)
			}
			eng.releaseLayer()
			for _, child := range orderedChildren(w.children) {
				if child.shown && child != w.scrollChild && !childDrawsBehindParent(child, w) {
					paint(target, child, rect)
				}
			}
			return
		}
		for _, child := range orderedChildren(w.children) {
			if child.shown && !childDrawsBehindParent(child, w) && !(w.kind == kindEditBox && w.text != "" && child.kind == kindFontString && strings.HasSuffix(strings.ToLower(child.name), "fill")) {
				if child.layerLevel == layerHighlight && !isHighlighted(w) {
					continue
				}
				paint(target, child, rect)
			}
		}
	}
	paintWidget = func(target *image.RGBA, w *widget, parent Rect) {
		rect := eng.layoutRect(w, parent)

		scaledRect := Rect{
			X0: rect.X0 * uiScale,
			Y0: rect.Y0 * uiScale,
			X1: rect.X1 * uiScale,
			Y1: rect.Y1 * uiScale,
		}

		// Child frames with a lower strata/level (e.g. OptionsFrame $parentBackdrop at
		// parent.frameLevel-1) draw behind this frame's backdrop and layers.
		for _, child := range orderedChildren(w.children) {
			if child.shown && childDrawsBehindParent(child, w) {
				paint(target, child, rect)
			}
		}

		// Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬ Backdrop Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬Ã¢â€â‚¬
		if w.backdrop != nil && (w.backdrop.bgFile != "" || w.backdrop.edgeFile != "") {
			eng.drawBackdrop(target, w.backdrop, scaledRect)
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
					if !w.vertexColor.isZero() {
						img = eng.tintTextureImage(w.textureFile, img, w.vertexColor)
					}
					if w.horizTile || w.vertTile {
						eng.drawTiledTexture(target, img, scaledRect, float64(screenHeight), tc, w.horizTile, w.vertTile, strings.EqualFold(w.blendMode, "ADD") || strings.EqualFold(w.alphaMode, "ADD"))
					} else {
						drawSubModeFilter(target, img, scaledRect, float64(screenHeight), tc, strings.EqualFold(w.blendMode, "ADD") || strings.EqualFold(w.alphaMode, "ADD"))
					}
				}
			} else if !w.vertexColor.isZero() {
				eng.drawTextureColor(target, scaledRect, w.vertexColor)
			}

		case kindStatusBar:
			fraction := 1.0
			if w.maxValue > w.minValue {
				fraction = (w.value - w.minValue) / (w.maxValue - w.minValue)
			}
			if fraction < 0 {
				fraction = 0
			}
			if fraction > 1 {
				fraction = 1
			}
			fill := scaledRect
			if strings.EqualFold(w.orientation, "VERTICAL") {
				fill.Y0 = scaledRect.Y1 - scaledRect.H()*fraction
			} else {
				fill.X1 = scaledRect.X0 + scaledRect.W()*fraction
			}
			if fill.W() > 0 && fill.H() > 0 && w.statusBarTexture != nil && w.statusBarTexture.textureFile != "" {
				if img := eng.loadBLP(w.statusBarTexture.textureFile); img != nil {
					tc := [4]float64{w.statusBarTexture.texCoordL, w.statusBarTexture.texCoordR, w.statusBarTexture.texCoordT, w.statusBarTexture.texCoordB}
					if tc[0] == 0 && tc[1] == 0 && tc[2] == 0 && tc[3] == 0 {
						tc = [4]float64{0, 1, 0, 1}
					}
					drawSubModeFilter(target, img, fill, float64(screenHeight), tc, strings.EqualFold(w.statusBarTexture.blendMode, "ADD") || strings.EqualFold(w.statusBarTexture.alphaMode, "ADD"))
				} else if !w.statusBarColor.isZero() {
					eng.drawTextureColor(target, fill, w.statusBarColor)
				}
			} else if fill.W() > 0 && fill.H() > 0 && !w.statusBarColor.isZero() {
				eng.drawTextureColor(target, fill, w.statusBarColor)
			}

		case kindFontString:
			// FrameXML places a FontString directly on ScrollingMessageFrame as
			// the font attribute holder; messages are drawn only via drawMessageLines.
			if w.parent != nil && w.parent.kind == kindScrollingMessageFrame {
				break
			}
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
				eng.drawTextAlignedWidget(target, f, text, scaledRect, float64(screenHeight), c, w)
			}

		case kindMovieFrame:
			if w.movieActive {
				eng.ensureMovie(w.movieFile, float64(w.movieVolume)/255)
				dst := ScreenRect(rect, float64(target.Bounds().Dy()))
				draw.Draw(target, dst, &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
				if eng.movieImage != nil {
					eng.drawMovieFrame(target, dst, eng.movieImage)
				}
			}
		}

		if w.kind == kindButton || w.kind == kindCheckButton {
			children := orderedChildren(w.children)
			for _, child := range children {
				if child.shown && !childDrawsBehindParent(child, w) && child.layerLevel < layerArtwork {
					paint(target, child, rect)
				}
			}
			eng.paintButtonState(w, rect, func(child *widget, childRect Rect) { paint(target, child, childRect) })
			for _, child := range children {
				if child.shown && !childDrawsBehindParent(child, w) && child.layerLevel >= layerArtwork {
					// HIGHLIGHT-layer regions only draw while the frame is hovered/locked.
					if child.layerLevel == layerHighlight && !isHighlighted(w) {
						continue
					}
					paint(target, child, rect)
				}
			}
		} else {
			paintChildren(target, w, rect)
		}
		if w.kind == kindSlider {
			eng.paintSliderThumb(w, rect, func(child *widget, childRect Rect) { paint(target, child, childRect) })
		}
		if w.kind == kindScrollingMessageFrame {
			eng.drawMessageLines(target, w, rect, face, faceLg, float64(screenHeight))
		}

		if w.kind == kindButton || w.kind == kindCheckButton {
			if w.buttonLabel == nil {
				text := eng.resolveText(w.text)
				if text != "" {
					eng.drawTextAlignedWidget(target, face, text, scaledRect, float64(screenHeight), eng.fontColor(w), w)
				}
			}
		}
		if w.kind == kindEditBox {
			eng.drawEditText(target, face, faceLg, w, rect, float64(screenHeight))
		}
	}
	painting := make(map[*widget]bool)
	paint = func(target *image.RGBA, w *widget, parent Rect) {
		if w == nil {
			return
		}
		if painting[w] {
			return
		}
		painting[w] = true
		defer delete(painting, w)
		alpha := w.alpha
		if alpha >= 1 {
			paintWidget(target, w, parent)
			return
		}
		if alpha <= 0 {
			return
		}
		layer := eng.acquireLayer(target.Bounds())
		paintWidget(layer, w, parent)
		for index := 3; index < len(layer.Pix); index += 4 {
			layer.Pix[index] = uint8(float64(layer.Pix[index]) * alpha)
		}
		draw.Draw(target, target.Bounds(), layer, layer.Bounds().Min, draw.Over)
		eng.releaseLayer()
	}

	if root != nil {
		rootRect := screen
		if root.name == "GlueParent" {
			rootRect = eng.glueParentRect()
		}
		for _, child := range orderedChildren(root.children) {
			if child.shown {
				paint(canvas, child, rootRect)
			}
		}
	}
	if drawBackground && eng.sceneBackground {
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

func (eng *UIEngine) updateScrollFrames() {
	if eng == nil || eng.Rt == nil {
		return
	}
	frames := make([]*widget, 0)
	for _, w := range eng.Rt.widgets {
		if w.kind == kindScrollFrame && w.scrollChild != nil {
			frames = append(frames, w)
		}
	}
	for _, frame := range frames {
		contentW, contentH := eng.scrollContentExtent(frame)
		if frame.scrollChild.width != contentW || frame.scrollChild.height != contentH {
			frame.scrollChild.width = contentW
			frame.scrollChild.height = contentH
			eng.rects = make(map[*widget]Rect)
			eng.layoutActive = make(map[*widget]bool)
		}
		frameRect := eng.layoutRect(frame, eng.screen)
		frame.verticalRange = math.Max(0, contentH-frameRect.H())
		frame.horizontalRange = math.Max(0, contentW-frameRect.W())
		if frame.verticalScroll > frame.verticalRange {
			frame.verticalScroll = frame.verticalRange
		}
		if frame.verticalScroll < 0 {
			frame.verticalScroll = 0
		}
		if scrollbar := eng.Rt.widgets[frame.name+"ScrollBar"]; scrollbar != nil {
			scrollbar.minValue = 0
			scrollbar.maxValue = frame.verticalRange
			scrollbar.value = frame.verticalScroll
			up := eng.Rt.widgets[scrollbar.name+"ScrollUpButton"]
			down := eng.Rt.widgets[scrollbar.name+"ScrollDownButton"]
			if up != nil {
				up.shown = true
				up.enabled = scrollbar.value > scrollbar.minValue
			}
			if down != nil {
				down.shown = true
				down.enabled = scrollbar.value < scrollbar.maxValue
			}
		}
	}
}

func (eng *UIEngine) acquireLayer(bounds image.Rectangle) *image.RGBA {
	if eng.layerPool == nil {
		eng.layerPool = make([]*image.RGBA, 0, 2)
	}
	if eng.layerDepth == len(eng.layerPool) {
		eng.layerPool = append(eng.layerPool, image.NewRGBA(bounds))
	} else if !eng.layerPool[eng.layerDepth].Bounds().Eq(bounds) {
		eng.layerPool[eng.layerDepth] = image.NewRGBA(bounds)
	}
	layer := eng.layerPool[eng.layerDepth]
	eng.layerDepth++
	draw.Draw(layer, bounds, &image.Uniform{C: color.Transparent}, image.Point{}, draw.Src)
	return layer
}

func (eng *UIEngine) releaseLayer() {
	if eng.layerDepth > 0 {
		eng.layerDepth--
	}
}

func (eng *UIEngine) scrollContentExtent(frame *widget) (float64, float64) {
	if frame == nil || frame.scrollChild == nil {
		return 0, 0
	}
	frameRect := eng.layoutRect(frame, eng.screen)
	child := frame.scrollChild
	childRect := eng.layoutRect(child, frameRect)
	minX, maxX := childRect.X0, childRect.X1
	minY, maxY := childRect.Y0, childRect.Y1
	visited := map[*widget]bool{child: true}
	var visit func(*widget)
	visit = func(parent *widget) {
		for _, current := range parent.children {
			if current == nil || visited[current] {
				continue
			}
			visited[current] = true
			rect := eng.layoutRect(current, childRect)
			minX = math.Min(minX, rect.X0)
			maxX = math.Max(maxX, rect.X1)
			minY = math.Min(minY, rect.Y0)
			maxY = math.Max(maxY, rect.Y1)
			visit(current)
		}
	}
	visit(child)
	contentW := math.Max(child.width, maxX-childRect.X0)
	contentW = math.Max(contentW, childRect.X1-minX)
	contentH := math.Max(child.height, childRect.Y1-minY)
	contentH = math.Max(contentH, maxY-childRect.Y0)
	return contentW, contentH
}

func (eng *UIEngine) paintSliderThumb(w *widget, rect Rect, paint func(*widget, Rect)) {
	if w == nil || w.thumbTexture == nil || !w.thumbTexture.shown || rect.H() <= 0 {
		return
	}
	min, max := w.minValue, w.maxValue
	fraction := 0.0
	if max > min {
		fraction = (w.value - min) / (max - min)
		if fraction < 0 {
			fraction = 0
		}
		if fraction > 1 {
			fraction = 1
		}
	}
	thumbWidth, thumbHeight := w.thumbTexture.width, w.thumbTexture.height
	if thumbWidth <= 0 {
		thumbWidth = 18
	}
	if thumbHeight <= 0 {
		thumbHeight = 24
	}
	centerX, centerY := (rect.X0+rect.X1)/2, 0.0
	if strings.EqualFold(w.orientation, "HORIZONTAL") {
		trackStart := rect.X0 + thumbWidth/2
		trackEnd := rect.X1 - thumbWidth/2
		travel := math.Max(0, trackEnd-trackStart)
		centerX = trackStart + travel*fraction
		centerY = (rect.Y0 + rect.Y1) / 2
	} else {
		trackStart := rect.Y0 + 18
		trackEnd := rect.Y1 - 18
		travel := math.Max(0, trackEnd-trackStart-thumbHeight)
		centerY = trackStart + thumbHeight/2 + travel*fraction
	}
	thumbRect := Rect{X0: centerX - thumbWidth/2, Y0: centerY - thumbHeight/2, X1: centerX + thumbWidth/2, Y1: centerY + thumbHeight/2}
	eng.rects[w.thumbTexture] = thumbRect
	w.thumbTexture.renderRect = thumbRect
	w.thumbTexture.hasRenderRect = true
	paint(w.thumbTexture, thumbRect)
}

func (eng *UIEngine) HandleScroll(delta float64) bool {
	if eng == nil || eng.Rt == nil || delta == 0 {
		return false
	}
	if eng.uiScale <= 0 {
		return false
	}
	point := struct{ x, y float64 }{eng.Rt.cursorX / eng.uiScale, (float64(eng.screenHeight) - eng.Rt.cursorY) / eng.uiScale}
	var target *widget
	targetArea := math.MaxFloat64
	for _, frame := range eng.Rt.widgets {
		if frame.kind != kindScrollFrame || !frame.shown {
			continue
		}
		parent := eng.screen
		if frame.parent != nil {
			parent = eng.layoutRect(frame.parent, eng.screen)
		}
		rect := eng.layoutRect(frame, parent)
		area := rect.W() * rect.H()
		if point.x >= rect.X0 && point.x <= rect.X1 && point.y >= rect.Y0 && point.y <= rect.Y1 && area < targetArea {
			target = frame
			targetArea = area
		}
	}
	if target == nil {
		return false
	}
	step := target.height / 2
	if scrollbar := eng.Rt.widgets[target.name+"ScrollBar"]; scrollbar != nil && scrollbar.height > 0 {
		step = scrollbar.height / 2
	}
	if step <= 0 {
		step = 1
	}
	value := target.verticalScroll - delta*step
	if value < 0 {
		value = 0
	}
	if value > target.verticalRange {
		value = target.verticalRange
	}
	if value == target.verticalScroll {
		return true
	}
	target.verticalScroll = value
	eng.Rt.fire(target, "OnVerticalScroll", []lua.LValue{target.luaValue(eng.Rt.L), lua.LNumber(value)})
	return true
}

func (eng *UIEngine) prepareText(w *widget, face, faceLg font.Face) {
	if w == nil {
		return
	}
	if w.kind == kindFontString {
		eng.measureFontStringWithFaces(w, face, faceLg)
	}
	for _, child := range w.children {
		eng.prepareText(child, face, faceLg)
	}
	// Three-piece tabs (ChatFrame*TabMiddle, etc.) are sized by
	// PanelTemplates_TabResize; do not override their authored width.
	if w.buttonLabel != nil && w.buttonLabel.textWidth > 0 && eng.Rt.lookup(w.name+"Middle") == nil && w.width < w.buttonLabel.textWidth+50 {
		w.width = w.buttonLabel.textWidth + 50
	}
}

// measureFontString updates auto-sized FontString metrics so Lua GetHeight /
// GetWidth match the original client immediately after SetText.
func (eng *UIEngine) measureFontString(w *widget) {
	if eng == nil || w == nil || w.kind != kindFontString {
		return
	}
	if eng.uiScale <= 0 {
		eng.uiScale = 1
	}
	face := eng.cachedFace("__base13", eng.FontObj, 13*eng.uiScale)
	faceLg := eng.cachedFace("__base16", eng.FontObj, 16*eng.uiScale)
	eng.measureFontStringWithFaces(w, face, faceLg)
}

func (eng *UIEngine) measureFontStringWithFaces(w *widget, face, faceLg font.Face) {
	text := eng.resolveText(w.text)
	if w.parent != nil && (w.parent.kind == kindButton || w.parent.kind == kindCheckButton) && w.parent.buttonLabel == w && w.parent.text != "" {
		text = eng.resolveText(w.parent.text)
	}
	fontFace := eng.faceFor(w, face, faceLg)
	if fontFace == nil {
		return
	}
	clean := cleanText(text)
	var lines []string
	if !w.autoTextWidth && w.width > 0 {
		lines = wrapText(clean, fontFace, int(w.width*eng.uiScale))
	} else {
		lines = strings.Split(clean, "\n")
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	maxWidth := 0
	for _, line := range lines {
		if width := font.MeasureString(fontFace, line).Ceil(); width > maxWidth {
			maxWidth = width
		}
	}
	w.textWidth = float64(maxWidth) / eng.uiScale
	if w.autoTextWidth && !w.explicitWidth {
		width := maxWidth
		if strings.EqualFold(eng.textJustify(w), "LEFT") || strings.EqualFold(eng.textJustify(w), "RIGHT") {
			width += int(math.Ceil(8 * eng.uiScale))
		}
		w.width = float64(width) / eng.uiScale
	}
	if w.autoTextHeight {
		w.height = float64(fontFace.Metrics().Height.Ceil()*len(lines)) / eng.uiScale
	}
}

func (eng *UIEngine) layoutRect(w *widget, parent Rect) Rect {
	if w != nil && w.name == "GlueParent" {
		rect := eng.glueParentRect()
		eng.rects[w] = rect
		w.renderRect = rect
		w.hasRenderRect = true
		return rect
	}
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
	if w.parent != nil && w.parent.kind == kindScrollFrame && len(w.points) == 0 {
		shift := w.parent.verticalScroll
		rect := scaleRect(Rect{X0: parentRect.X0, Y0: parentRect.Y1 - shift - w.height, X1: parentRect.X0 + w.width, Y1: parentRect.Y1 - shift}, w.scale)
		eng.rects[w] = rect
		w.renderRect = rect
		w.hasRenderRect = true
		return rect
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

func (eng *UIEngine) glueParentRect() Rect {
	if eng.screen.H() <= 0 || eng.screen.W()/eng.screen.H() <= 16.0/9.0 {
		return eng.screen
	}
	width := eng.screen.H() * 16.0 / 9.0
	margin := (eng.screen.W() - width) / 2
	return Rect{X0: margin, Y0: 0, X1: eng.screen.W() - margin, Y1: eng.screen.H()}
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
	if w.enabled && isHighlighted(w) && w.highlightTexture != nil && w.highlightTexture.shown {
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
	if !w.textInsetsSet {
		if left == 0 {
			left = 12
		}
		if right == 0 {
			right = 5
		}
		if top == 0 && bottom == 0 {
			bottom = 4
		}
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
	face, err := opentype.NewFace(fontObj, &opentype.FaceOptions{Size: size * eng.uiScale, DPI: 72})
	if err != nil {
		return nil, func() {}
	}
	return face, func() { face.Close() }
}

func (eng *UIEngine) editTextOrigin(face font.Face, text string, dst image.Rectangle, w *widget) int {
	width := font.MeasureString(face, text).Ceil()
	switch strings.ToUpper(eng.textJustify(w)) {
	case "RIGHT":
		return dst.Max.X - width - 4
	case "CENTER":
		return dst.Min.X + (dst.Dx()-width)/2
	default:
		return dst.Min.X + 4
	}
}

func (eng *UIEngine) setEditCursor(w *widget, x float64) { eng.setEditCursorAt(w, x, false) }

func (eng *UIEngine) setEditCursorAt(w *widget, x float64, extend bool) {
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
	display := []rune(editDisplayText(w))
	face, release := eng.editFace(w)
	defer release()
	if face == nil {
		moveEditCursor(w, int(math.Round((x-textRect.X0*scale)/scale/8)), extend)
		return
	}
	dst := ScreenRect(screenScaledRect(textRect, scale), float64(eng.screenHeight))
	origin := eng.editTextOrigin(face, string(display), dst, eng.editTextWidget(w))
	position := x - float64(origin)
	moveEditCursor(w, editCursorIndex(face, display, position), extend)
}

func editCursorIndex(face font.Face, text []rune, position float64) int {
	for index := 0; index < len(text); index++ {
		left := float64(font.MeasureString(face, string(text[:index])).Ceil())
		right := float64(font.MeasureString(face, string(text[:index+1])).Ceil())
		if position < (left+right)/2 {
			return index
		}
	}
	return len(text)
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
		dst := ScreenRect(screenTextRect, screenHeight)
		origin := eng.editTextOrigin(textFace, text, dst, textWidget)
		startWidth := font.MeasureString(textFace, string([]rune(text)[:start])).Ceil()
		endWidth := font.MeasureString(textFace, string([]rune(text)[:end])).Ceil()
		selection := image.Rect(origin+startWidth, dst.Min.Y+2, origin+endWidth, dst.Max.Y-2)
		draw.Draw(canvas, selection, &image.Uniform{C: color.RGBA{R: 35, G: 100, B: 180, A: 180}}, image.Point{}, draw.Over)
	}
	if text != "" {
		eng.drawTextAlignedWidget(canvas, textFace, text, screenTextRect, screenHeight, textColor, textWidget)
	}
	if eng.Rt.focused == w {
		dst := ScreenRect(screenTextRect, screenHeight)
		origin := eng.editTextOrigin(textFace, text, dst, textWidget)
		width := font.MeasureString(textFace, string([]rune(text)[:clampCursor(w.cursor, len([]rune(text)))])).Ceil()
		caretX := origin + width + 1
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
	if key == "" {
		eng.closeStatusDialog()
		return
	}
	wasOpen := eng.statusKey != "" || eng.statusText != ""
	eng.statusKey = key
	eng.statusText = ""
	eng.statusTTL = 0
	eng.updateStatusDialog(wasOpen, eng.resolveText(key))
}

func (eng *UIEngine) SetStatusText(text string) {
	if text == "" {
		eng.closeStatusDialog()
		return
	}
	wasOpen := eng.statusKey != "" || eng.statusText != ""
	eng.statusKey = ""
	eng.statusText = text
	eng.statusTTL = 3
	eng.updateStatusDialog(wasOpen, text)
}

func (eng *UIEngine) updateStatusDialog(wasOpen bool, text string) {
	if eng == nil || eng.Rt == nil || text == "" {
		return
	}
	dialogType := "OKAY"
	if eng.statusKey == "GAME_SERVER_LOGIN" {
		dialogType = "CANCEL"
	}
	if eng.statusDialogType != dialogType || !wasOpen {
		eng.Rt.FireEvent("OPEN_STATUS_DIALOG", lua.LString(dialogType), lua.LString(text))
	} else {
		eng.Rt.FireEvent("UPDATE_STATUS_DIALOG", lua.LString(text))
	}
	eng.statusDialogType = dialogType
}

func (eng *UIEngine) closeStatusDialog() {
	wasOpen := eng != nil && eng.statusDialogType != ""
	if wasOpen && eng.Rt != nil {
		eng.Rt.FireEvent("CLOSE_STATUS_DIALOG")
	}
	eng.statusKey = ""
	eng.statusText = ""
	eng.statusTTL = 0
	eng.statusDialogType = ""
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
	if scene := eng.Rt.widgets["LoginScene"]; scene != nil {
		visit(scene)
	}
	return path
}

func (eng *UIEngine) CreatePreviewState() (CreatePreviewState, bool) {
	if eng.Rt == nil {
		return CreatePreviewState{}, false
	}
	frame := eng.Rt.widgets["CharacterCreate"]
	if frame == nil || !frame.shown || len(createRaces) == 0 {
		return CreatePreviewState{}, false
	}
	raceIndex := clampCreateIndex(eng.Rt.selectedRace, len(createRaces))
	classIndex := eng.Rt.selectedClass
	if !validCreateClass(raceIndex, classIndex) {
		for candidate := range createClasses {
			if validCreateClass(raceIndex, candidate+1) {
				classIndex = candidate + 1
				break
			}
		}
	}
	gender := uint8(0)
	if eng.Rt.selectedSex == 3 {
		gender = 1
	}
	return CreatePreviewState{RaceID: createRaces[raceIndex-1].id, ClassID: uint8(classIndex), Gender: gender, Facing: eng.Rt.createFacing}, true
}

func (eng *UIEngine) CharacterSelectVisible() bool {
	return eng != nil && eng.Rt != nil && eng.Rt.widgets["CharacterSelect"] != nil && eng.Rt.widgets["CharacterSelect"].shown
}

func (eng *UIEngine) SceneCharacterFacing() float32 {
	if state, ok := eng.CreatePreviewState(); ok {
		return state.Facing
	}
	if eng.Rt == nil {
		return 0
	}
	return eng.Rt.Glue.CharacterFacing
}

func (eng *UIEngine) SetInitialCredentials(account, password string, rememberMe bool) {
	eng.closeStatusDialog()
	eng.SetWorldLoading(false)
	eng.ClearLoadingScreen()
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
	eng.SetWorldLoading(false)
	eng.ClearLoadingScreen()
	if len(state.AddOns) == 0 {
		state.AddOns = eng.Rt.Glue.AddOns
	}
	eng.Rt.Glue = state
	eng.closeStatusDialog()
	eng.Rt.SetCVar("currentGlueScreen", "charselect")
	eng.Rt.Execute("for _, name in ipairs({'VideoOptionsFrame', 'AudioOptionsFrame', 'OptionsSelectFrame', 'CinematicsFrame', 'MovieFrame', 'RealmList', 'AddonList', 'GlueDialog'}) do local frame = _G[name]; if frame then frame:Hide() end end", "@network.lua")
	eng.Rt.Execute("GlueParent_OnEvent('SET_GLUE_SCREEN', 'charselect')", "@network.lua")
	eng.Rt.Execute("SetGlueScreen('charselect')", "@network.lua")
	eng.Rt.FireEvent("SET_GLUE_SCREEN", lua.LString("charselect"))
	eng.Rt.FireEvent("CHARACTER_LIST_UPDATE")
}

func (eng *UIEngine) HandleCursor(x, y float64) bool {
	if eng.Rt != nil {
		eng.Rt.cursorX = x
		eng.Rt.cursorY = y
	}
	if eng.selecting != nil {
		eng.setEditCursorAt(eng.selecting, x, true)
		return true
	}
	if eng.sliderDragging != nil {
		eng.setSliderValueAt(eng.sliderDragging, x, y)
		return true
	}
	if eng.debugPanel.dragging {
		eng.debugPanel.move(x, y, eng)
		if eng.hovered != nil && !eng.hovered.highlightLocked {
			eng.hovered.highlighted = false
			eng.hovered = nil
		}
		return true
	}
	if eng.debugPanel.contains(x, y, eng) {
		if eng.hovered != nil && !eng.hovered.highlightLocked {
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
	if eng.hovered != nil && !eng.hovered.highlightLocked {
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
	if eng.Rt != nil {
		eng.Rt.cursorX = x
		eng.Rt.cursorY = y
	}
	if eng.debugPanel.handleMouse(x, y, down, eng) {
		return true
	}
	target := eng.hitTest(x, y)
	if down {
		eng.pressed = target
		if target == nil {
			if eng.Rt != nil {
				eng.Rt.setFocus(nil)
			}
			return false
		}
		if target.kind == kindEditBox {
			eng.Rt.setFocus(target)
			eng.setEditCursor(target, x)
			eng.selecting = target
		} else if target.kind == kindSlider {
			eng.sliderDragging = target
			eng.setSliderValueAt(target, x, y)
		} else if target.kind == kindButton || target.kind == kindCheckButton {
			target.buttonState = "PUSHED"
		}
		if eng.Rt != nil {
			eng.Rt.fire(target, "OnMouseDown", []lua.LValue{target.luaValue(eng.Rt.L), lua.LString("LeftButton")})
		}
		return true
	}
	if eng.sliderDragging != nil {
		slider := eng.sliderDragging
		eng.setSliderValueAt(slider, x, y)
		eng.sliderDragging = nil
		eng.pressed = nil
		if eng.Rt != nil {
			eng.Rt.fire(slider, "OnMouseUp", []lua.LValue{slider.luaValue(eng.Rt.L), lua.LString("LeftButton")})
		}
		return true
	}
	pressed := eng.pressed
	eng.pressed = nil
	eng.selecting = nil
	if pressed == nil {
		return target != nil
	}
	if pressed.kind == kindButton || pressed.kind == kindCheckButton {
		pressed.buttonState = "NORMAL"
	}
	if eng.Rt != nil {
		eng.Rt.fire(pressed, "OnMouseUp", []lua.LValue{pressed.luaValue(eng.Rt.L), lua.LString("LeftButton")})
	}
	if pressed == target && (target.kind == kindButton || target.kind == kindCheckButton) {
		if target.kind == kindCheckButton {
			target.checked = !target.checked
		}
		if eng.Rt != nil {
			eng.Rt.fire(target, "OnClick", []lua.LValue{target.luaValue(eng.Rt.L), lua.LString("LeftButton"), lua.LBool(false)})
		}
		if eng.lastClick == target && time.Since(eng.lastClickAt) <= 500*time.Millisecond {
			if eng.Rt != nil {
				eng.Rt.fire(target, "OnDoubleClick", []lua.LValue{target.luaValue(eng.Rt.L), lua.LString("LeftButton")})
			}
			eng.lastClick = nil
		} else if target.kind == kindButton || target.kind == kindCheckButton {
			eng.lastClick = target
			eng.lastClickAt = time.Now()
		}
		return true
	}
	return pressed == target
}

func (eng *UIEngine) setSliderValueAt(w *widget, x, y float64) {
	if w == nil || w.kind != kindSlider || eng.uiScale <= 0 {
		return
	}
	rect, ok := eng.rects[w]
	if !ok {
		rect = w.renderRect
	}
	if rect.W() <= 0 || rect.H() <= 0 {
		return
	}
	pointX := x / eng.uiScale
	pointY := (float64(eng.screenHeight) - y) / eng.uiScale
	thumbWidth, thumbHeight := 18.0, 24.0
	if w.thumbTexture != nil {
		if w.thumbTexture.width > 0 {
			thumbWidth = w.thumbTexture.width
		}
		if w.thumbTexture.height > 0 {
			thumbHeight = w.thumbTexture.height
		}
	}
	var fraction float64
	if strings.EqualFold(w.orientation, "HORIZONTAL") {
		start := rect.X0 + thumbWidth/2
		end := rect.X1 - thumbWidth/2
		if end <= start {
			fraction = 0
		} else {
			fraction = (pointX - start) / (end - start)
		}
	} else {
		start := rect.Y0 + 18
		end := rect.Y1 - 18
		travel := math.Max(0, end-start-thumbHeight)
		if travel == 0 {
			fraction = 0
		} else {
			fraction = (pointY - start - thumbHeight/2) / travel
		}
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	value := w.minValue + fraction*(w.maxValue-w.minValue)
	if w.valueStep > 0 {
		value = w.minValue + math.Round((value-w.minValue)/w.valueStep)*w.valueStep
	}
	if value < w.minValue {
		value = w.minValue
	}
	if value > w.maxValue {
		value = w.maxValue
	}
	if value == w.value {
		return
	}
	w.value = value
	if eng.Rt != nil {
		eng.Rt.fire(w, "OnValueChanged", []lua.LValue{w.luaValue(eng.Rt.L), lua.LNumber(value)})
	}
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

func (eng *UIEngine) worldChatEditBox() *widget {
	if eng == nil || eng.Rt == nil {
		return nil
	}
	chat := eng.Rt.widgets["ChatFrame1"]
	if chat != nil && chat.fields != nil {
		if value := chat.fields.RawGetString("editBox"); value != lua.LNil {
			if userData, ok := value.(*lua.LUserData); ok {
				if edit, ok := userData.Value.(*widget); ok {
					return edit
				}
			}
		}
	}
	return eng.Rt.widgets["ChatFrame1EditBox"]
}

func (eng *UIEngine) activateWorldChat() bool {
	edit := eng.worldChatEditBox()
	if edit == nil || eng.Rt.L.GetGlobal("ChatEdit_ActivateChat").Type() != lua.LTFunction {
		return false
	}
	if !eng.Rt.Execute(`ChatEdit_ActivateChat(ChatFrame1.editBox)`, "@world-chat-activate.lua") {
		return false
	}
	return eng.Rt.focused == edit
}

func (eng *UIEngine) HandleKey(key window.Key) bool { return eng.handleKey(key, false) }

func (eng *UIEngine) handleKey(key window.Key, extendSelection bool) bool {
	if eng.worldLoading && key == window.KeyEnter {
		return true
	}
	if eng.worldUIReady && key == window.KeyEnter && eng.Rt.focused == nil {
		return eng.activateWorldChat()
	}
	w := eng.Rt.focused
	if key == window.KeyEscape {
		// Focused edit boxes clear first (chat/login). With no focus, world ESC
		// runs the live TOGGLEGAMEMENU binding; glue keeps OnKeyDown dispatch.
		if w != nil {
			eng.Rt.setFocus(nil)
			return true
		}
		if eng.worldUIReady {
			return eng.ToggleGameMenu()
		}
		target := eng.keyboardTarget()
		if target != nil && !eng.isLoginTarget(target) {
			eng.Rt.fire(target, "OnKeyDown", []lua.LValue{target.luaValue(eng.Rt.L), lua.LString("ESCAPE")})
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
			if value, ok := eng.Rt.GetCVar("Sound_EnableMusic"); ok && value == "0" {
				eng.SetStatusText("Music Disabled")
			} else {
				eng.SetStatusText("Music Enabled")
			}
			return true
		case window.KeyS:
			eng.Rt.Execute("Sound_ToggleSound()", "@bindings.lua")
			if value, ok := eng.Rt.GetCVar("Sound_EnableSFX"); ok && value == "0" {
				eng.SetStatusText("Sound Effects Disabled")
			} else {
				eng.SetStatusText("Sound Effects Enabled")
			}
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
	if eng.worldActive && eng.worldRoot != nil {
		root = eng.worldRoot
	}
	if root == nil {
		return nil
	}
	for index := len(root.children) - 1; index >= 0; index-- {
		children := orderedChildren(root.children)
		if target := keyboardWidget(children[index]); target != nil {
			return target
		}
	}
	return nil
}

func keyboardWidget(w *widget) *widget {
	if !w.shown {
		return nil
	}
	children := orderedChildren(w.children)
	for index := len(children) - 1; index >= 0; index-- {
		if target := keyboardWidget(children[index]); target != nil {
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
		children := orderedChildren(w.children)
		for i := len(children) - 1; i >= 0; i-- {
			if target := visit(children[i], rect); target != nil {
				return target
			}
		}
		hitRect := Rect{X0: rect.X0 + w.hitInsetL, Y0: rect.Y0 + w.hitInsetB, X1: rect.X1 - w.hitInsetR, Y1: rect.Y1 - w.hitInsetT}
		if point.x < hitRect.X0 || point.x > hitRect.X1 || point.y < hitRect.Y0 || point.y > hitRect.Y1 {
			return nil
		}
		if (w.kind == kindButton || w.kind == kindCheckButton) && !w.enabled {
			return nil
		}
		if w.kind == kindButton || w.kind == kindCheckButton || w.kind == kindEditBox || w.kind == kindSlider || w.enableMouse {
			return w
		}
		return nil
	}
	root := eng.Rt.widgets["GlueParent"]
	if eng.worldActive && eng.worldRoot != nil {
		root = eng.worldRoot
	}
	if root == nil {
		return nil
	}
	rootRect := eng.screen
	if root.name == "GlueParent" {
		rootRect = eng.glueParentRect()
	}
	children := orderedChildren(root.children)
	for i := len(children) - 1; i >= 0; i-- {
		if target := visit(children[i], rootRect); target != nil {
			return target
		}
	}
	return nil
}

func frameStrataOrder(value string) int {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "BACKGROUND":
		return 0
	case "LOW":
		return 1
	case "HIGH":
		return 3
	case "DIALOG":
		return 4
	case "FULLSCREEN":
		return 5
	case "FULLSCREEN_DIALOG":
		return 6
	case "TOOLTIP":
		return 7
	default:
		return 2
	}
}

func frameStrataName(value int) string {
	switch value {
	case 0:
		return "BACKGROUND"
	case 1:
		return "LOW"
	case 3:
		return "HIGH"
	case 4:
		return "DIALOG"
	case 5:
		return "FULLSCREEN"
	case 6:
		return "FULLSCREEN_DIALOG"
	case 7:
		return "TOOLTIP"
	default:
		return "MEDIUM"
	}
}

func childDrawsBehindParent(child, parent *widget) bool {
	if child == nil || parent == nil {
		return false
	}
	// Regions are painted with their parent, not as independent frames.
	if child.kind == kindTexture || child.kind == kindFontString {
		return false
	}
	if child.frameStrata != parent.frameStrata {
		return child.frameStrata < parent.frameStrata
	}
	return child.frameLevel < parent.frameLevel
}

func childDrawOrderLess(first, second *widget) bool {
	if isCharacterSelectButton(first) && isCharacterSelectButton(second) && isHighlighted(first) != isHighlighted(second) {
		return isHighlighted(first) && !isHighlighted(second)
	}
	if first.frameStrata != second.frameStrata {
		return first.frameStrata < second.frameStrata
	}
	if first.frameLevel != second.frameLevel {
		return first.frameLevel < second.frameLevel
	}
	return first.layerLevel < second.layerLevel
}

func childrenAlreadyOrdered(children []*widget) bool {
	for index := 1; index < len(children); index++ {
		if childDrawOrderLess(children[index], children[index-1]) {
			return false
		}
	}
	return true
}

func orderedChildren(children []*widget) []*widget {
	if len(children) < 2 {
		return children
	}
	// Glue/FrameXML trees are usually already strata-ordered; avoid a
	// per-paint alloc+sort when the existing order is valid.
	if childrenAlreadyOrdered(children) {
		return children
	}
	ordered := append([]*widget(nil), children...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return childDrawOrderLess(ordered[left], ordered[right])
	})
	return ordered
}

func isCharacterSelectButton(w *widget) bool {
	return w != nil && strings.HasPrefix(w.name, "CharSelectCharacterButton")
}

func isHighlighted(w *widget) bool {
	return w != nil && (w.highlighted || w.highlightLocked)
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
			if bd.tile {
				eng.drawTiled(canvas, inner, bgImg, bd.tileSize)
			} else {
				xdraw.NearestNeighbor.Scale(canvas, inner, bgImg, bgImg.Bounds(), xdraw.Over, nil)
			}
		}
	} else if !bd.bgColor.isZero() {
		inL := int(bd.insetL * eng.uiScale)
		inR := int(bd.insetR * eng.uiScale)
		inT := int(bd.insetT * eng.uiScale)
		inB := int(bd.insetB * eng.uiScale)
		inner := image.Rect(dst.Min.X+inL, dst.Min.Y+inT, dst.Max.X-inR, dst.Max.Y-inB)
		if inner.Dx() > 0 && inner.Dy() > 0 {
			bgCol := color.RGBA{R: uint8(bd.bgColor.r * 255), G: uint8(bd.bgColor.g * 255), B: uint8(bd.bgColor.b * 255), A: uint8(bd.bgColor.a * 255)}
			draw.Draw(canvas, inner, &image.Uniform{C: bgCol}, image.Point{}, draw.Over)
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

func (eng *UIEngine) tintTextureImage(path string, source image.Image, tint rgba) image.Image {
	key := fmt.Sprintf("texture:%s:%.4f:%.4f:%.4f:%.4f", path, tint.r, tint.g, tint.b, tint.a)
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
	edge := int(math.Round(edgeSize))
	if edge <= 0 {
		edge = b.Dy()
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
		tileWidth := b.Dx() / 8
		if index < 0 || index >= 8 || tileWidth <= 0 {
			return
		}
		x0 := b.Min.X + index*tileWidth
		x1 := x0 + tileWidth
		y0 := b.Min.Y
		y1 := b.Max.Y
		if x1 <= x0 || y1 <= y0 {
			return
		}
		sourceWidth := x1 - x0
		sourceHeight := y1 - y0
		partialHeight := int(math.Round(float64(sourceHeight) * fraction))
		if partialHeight < 1 {
			return
		}
		var mapped *image.NRGBA
		if transpose {
			mapped = image.NewNRGBA(image.Rect(0, 0, partialHeight, sourceWidth))
			for y := 0; y < sourceWidth; y++ {
				for x := 0; x < partialHeight; x++ {
					mapped.Set(x, y, source.At(x0+y, y0+x))
				}
			}
		} else {
			mapped = image.NewNRGBA(image.Rect(0, 0, sourceWidth, partialHeight))
			for y := 0; y < partialHeight; y++ {
				for x := 0; x < sourceWidth; x++ {
					mapped.Set(x, y, source.At(x0+x, y0+y))
				}
			}
		}
		xdraw.NearestNeighbor.Scale(canvas, target, mapped, mapped.Bounds(), draw.Over, nil)
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

func (eng *UIEngine) drawTiledTexture(canvas *image.RGBA, source image.Image, r Rect, screenHeight float64, tc [4]float64, horiz, vert bool, additive bool) {
	dst := ScreenRect(r, screenHeight).Intersect(canvas.Bounds())
	if dst.Empty() {
		return
	}
	tile := textureSubImage(source, tc)
	if tile.Bounds().Empty() {
		return
	}
	tileWidth := dst.Dx()
	tileHeight := dst.Dy()
	if horiz {
		tileWidth = int(math.Round(float64(tile.Bounds().Dx()) * eng.uiScale))
	}
	if vert {
		tileHeight = int(math.Round(float64(tile.Bounds().Dy()) * eng.uiScale))
	}
	if tileWidth < 1 || tileHeight < 1 {
		return
	}
	resized := image.NewRGBA(image.Rect(0, 0, tileWidth, tileHeight))
	xdraw.NearestNeighbor.Scale(resized, resized.Bounds(), tile, tile.Bounds(), xdraw.Src, nil)
	for y := dst.Min.Y; y < dst.Max.Y; y += tileHeight {
		for x := dst.Min.X; x < dst.Max.X; x += tileWidth {
			part := image.Rect(x, y, x+tileWidth, y+tileHeight).Intersect(dst)
			if part.Empty() {
				continue
			}
			if !additive {
				draw.Draw(canvas, part, resized, image.Point{}, draw.Over)
				continue
			}
			for py := part.Min.Y; py < part.Max.Y; py++ {
				for px := part.Min.X; px < part.Max.X; px++ {
					sr, sg, sb, sa := resized.At(px-x, py-y).RGBA()
					if sa == 0 || (sr>>8 <= 2 && sg>>8 <= 2 && sb>>8 <= 8) {
						continue
					}
					dr, dg, db, da := canvas.At(px, py).RGBA()
					canvas.SetRGBA(px, py, color.RGBA{R: addChannel(dr, scaleChannel(sr, sa)), G: addChannel(dg, scaleChannel(sg, sa)), B: addChannel(db, scaleChannel(sb, sa)), A: maxChannel(da, sa)})
				}
			}
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
		case isHighlighted(w.parent) && w.parent.highlightFont != "":
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
		lowerFile := strings.ToLower(style.FontFile)
		switch {
		case strings.Contains(lowerFile, "morpheus"):
			fontObj = eng.FontObjSm
		case strings.Contains(lowerFile, "arialn"):
			if eng.FontObjArial != nil {
				fontObj = eng.FontObjArial
			}
		}
	} else if strings.Contains(strings.ToLower(w.fontObject), "large") || strings.Contains(strings.ToLower(w.fontObject), "huge") {
		return fallbackLarge
	}
	if size == 13 && style == nil {
		return fallback
	}
	return eng.cachedFace(fontKey, fontObj, size*eng.uiScale, fallback)
}

func (eng *UIEngine) cachedFace(key string, fontObj *opentype.Font, size float64, fallback ...font.Face) font.Face {
	if eng.textFaces == nil {
		eng.textFaces = make(map[string]font.Face)
	}
	cacheKey := fmt.Sprintf("%s|%.3f", key, size)
	if textFace, ok := eng.textFaces[cacheKey]; ok {
		return textFace
	}
	textFace, err := opentype.NewFace(fontObj, &opentype.FaceOptions{Size: size, DPI: 72})
	if err != nil {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return nil
	}
	eng.textFaces[cacheKey] = textFace
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

var uiBlendPool sync.Pool

func acquireBlendScratch(bounds image.Rectangle) *image.RGBA {
	if item := uiBlendPool.Get(); item != nil {
		if scratch, ok := item.(*image.RGBA); ok && scratch != nil && scratch.Bounds().Eq(bounds) {
			return scratch
		}
	}
	return image.NewRGBA(bounds)
}

func releaseBlendScratch(scratch *image.RGBA) {
	if scratch != nil {
		uiBlendPool.Put(scratch)
	}
}

func drawSub(canvas *image.RGBA, img image.Image, r Rect, screenHeight float64, tc [4]float64) {
	drawSubModeFilter(canvas, img, r, screenHeight, tc, false)
}

func drawSubMode(canvas *image.RGBA, img image.Image, r Rect, screenHeight float64, tc [4]float64, additive bool) {
	drawSubModeFilter(canvas, img, r, screenHeight, tc, additive)
}

func drawSubModeFilter(canvas *image.RGBA, img image.Image, r Rect, screenHeight float64, tc [4]float64, additive bool) {
	srcImg, ok := resolveTextureSubRGBA(img, tc)
	if !ok {
		return
	}
	dst := ScreenRect(r, screenHeight)
	if dst.Dx() <= 0 || dst.Dy() <= 0 {
		return
	}
	if !additive {
		xdraw.BiLinear.Scale(canvas, dst, srcImg, srcImg.Bounds(), xdraw.Over, nil)
		return
	}
	blend := acquireBlendScratch(dst)
	defer releaseBlendScratch(blend)
	xdraw.BiLinear.Scale(blend, blend.Bounds(), srcImg, srcImg.Bounds(), xdraw.Src, nil)
	for y := dst.Min.Y; y < dst.Max.Y; y++ {
		dstOff := canvas.PixOffset(dst.Min.X, y)
		srcOff := blend.PixOffset(dst.Min.X, y)
		for x := dst.Min.X; x < dst.Max.X; x++ {
			sr8 := blend.Pix[srcOff+0]
			sg8 := blend.Pix[srcOff+1]
			sb8 := blend.Pix[srcOff+2]
			sa8 := blend.Pix[srcOff+3]
			// Preserve additive keying used by Glue textures: near-black
			// source texels do not brighten the destination.
			if sa8 != 0 && !isAdditiveKeyColor(sr8, sg8, sb8) {
				sa := uint32(sa8) * 0x101
				sr := uint32(sr8) * 0x101
				sg := uint32(sg8) * 0x101
				sb := uint32(sb8) * 0x101
				dr := uint32(canvas.Pix[dstOff+0]) * 0x101
				dg := uint32(canvas.Pix[dstOff+1]) * 0x101
				db := uint32(canvas.Pix[dstOff+2]) * 0x101
				da := uint32(canvas.Pix[dstOff+3]) * 0x101
				canvas.Pix[dstOff+0] = addChannel(dr, scaleChannel(sr, sa))
				canvas.Pix[dstOff+1] = addChannel(dg, scaleChannel(sg, sa))
				canvas.Pix[dstOff+2] = addChannel(db, scaleChannel(sb, sa))
				canvas.Pix[dstOff+3] = maxChannel(da, sa)
			}
			dstOff += 4
			srcOff += 4
		}
	}
}

func resolveTextureSubRGBA(img image.Image, tc [4]float64) (*image.RGBA, bool) {
	b := img.Bounds()
	flipX := tc[1] < tc[0]
	flipY := tc[3] < tc[2]
	leftCoord, rightCoord := tc[0], tc[1]
	topCoord, bottomCoord := tc[2], tc[3]
	if flipX {
		leftCoord, rightCoord = rightCoord, leftCoord
	}
	if flipY {
		topCoord, bottomCoord = bottomCoord, topCoord
	}
	l := b.Min.X + int(float64(b.Dx())*leftCoord)
	rt := b.Min.X + int(float64(b.Dx())*rightCoord)
	tp := b.Min.Y + int(float64(b.Dy())*topCoord)
	bm := b.Min.Y + int(float64(b.Dy())*bottomCoord)
	if rt <= l || bm <= tp {
		l, rt, tp, bm = b.Min.X, b.Max.X, b.Min.Y, b.Max.Y
		flipX, flipY = false, false
	}
	// Non-flipped RGBA crops can share the source Pix via SubImage and
	// avoid a per-draw allocation on the common UI texture path.
	if rgba, ok := img.(*image.RGBA); ok && !flipX && !flipY {
		sub, ok := rgba.SubImage(image.Rect(l, tp, rt, bm)).(*image.RGBA)
		return sub, ok && sub.Bounds().Dx() > 0 && sub.Bounds().Dy() > 0
	}
	src := image.NewRGBA(image.Rect(0, 0, rt-l, bm-tp))
	copyTextureRect(src, img, l, tp, rt, bm, flipX, flipY)
	return src, true
}

func copyTextureRect(dst *image.RGBA, img image.Image, l, tp, rt, bm int, flipX, flipY bool) {
	width := rt - l
	height := bm - tp
	if rgba, ok := img.(*image.RGBA); ok {
		for y := 0; y < height; y++ {
			sourceY := tp + y
			if flipY {
				sourceY = bm - 1 - y
			}
			dstOff := dst.PixOffset(0, y)
			if !flipX {
				srcOff := rgba.PixOffset(l, sourceY)
				copy(dst.Pix[dstOff:dstOff+width*4], rgba.Pix[srcOff:srcOff+width*4])
				continue
			}
			for x := 0; x < width; x++ {
				sourceX := rt - 1 - x
				srcOff := rgba.PixOffset(sourceX, sourceY)
				copy(dst.Pix[dstOff:dstOff+4], rgba.Pix[srcOff:srcOff+4])
				dstOff += 4
			}
		}
		return
	}
	for y := 0; y < height; y++ {
		sourceY := tp + y
		if flipY {
			sourceY = bm - 1 - y
		}
		for x := 0; x < width; x++ {
			sourceX := l + x
			if flipX {
				sourceX = rt - 1 - x
			}
			dst.Set(x, y, img.At(sourceX, sourceY))
		}
	}
}

func textureSubImage(img image.Image, tc [4]float64) image.Image {
	src, ok := resolveTextureSubRGBA(img, tc)
	if !ok {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	return src
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

func isAdditiveKeyColor(r, g, b uint8) bool {
	const threshold = 24
	return r <= threshold && g <= threshold && b <= threshold
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
	clip := dst.Intersect(canvas.Bounds())
	if shadow != nil {
		padding := 0
		if strings.EqualFold(shadow.Outline, "NORMAL") {
			padding = 1
		}
		if shadow.Shadow {
			shadowY := int(math.Round(-shadow.ShadowOffsetY * shadowScale))
			if value := int(math.Abs(float64(shadowY))) + 1; value > padding {
				padding = value
			}
		}
		clip.Min.Y -= padding
		clip.Max.Y += padding
		clip = clip.Intersect(canvas.Bounds())
	}
	if clip.Empty() {
		return
	}
	destination := canvas.SubImage(clip).(*image.RGBA)

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
	drawLine := func(line string, x, y int, c color.Color) {
		d := &font.Drawer{Dst: destination, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, y)}
		d.DrawString(line)
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
		if shadow != nil && strings.EqualFold(shadow.Outline, "NORMAL") {
			outlineColor := color.RGBA{A: 255}
			for offsetY := -1; offsetY <= 1; offsetY++ {
				for offsetX := -1; offsetX <= 1; offsetX++ {
					if offsetX != 0 || offsetY != 0 {
						drawLine(line, dotX+offsetX, startY+index*height+offsetY, outlineColor)
					}
				}
			}
		}
		if shadow != nil && shadow.Shadow {
			shadowColor := color.RGBA{R: uint8(shadow.ShadowColor.r * 255), G: uint8(shadow.ShadowColor.g * 255), B: uint8(shadow.ShadowColor.b * 255), A: uint8(shadow.ShadowColor.a * 255)}
			shadowX := int(math.Round(shadow.ShadowOffsetX * shadowScale))
			shadowY := int(math.Round(-shadow.ShadowOffsetY * shadowScale))
			drawLine(line, dotX+shadowX, startY+index*height+shadowY, shadowColor)
		}
		drawLine(line, dotX, startY+index*height, c)
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
