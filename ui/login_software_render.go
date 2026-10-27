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
	"strconv"
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
	// UserAddOns are the user add-ons loaded into the world interface.
	UserAddOns []string
	// cursorIconPath and cursorIconImage cache the scaled icon CursorIcon returns.
	cursorIconPath  string
	cursorIconImage *image.RGBA

	keyMods            window.ModifierKey // modifiers of the key press being handled
	curScale           float64            // effective scale of the widget being painted (1 outside painting)
	Rt                 *Runtime
	FontObj            *opentype.Font
	FontObjSm          *opentype.Font
	FontObjArial       *opentype.Font
	AssetLoader        *Loader
	Cache              map[string]image.Image
	BgImagePath        string // Path to a static background image (JPEG/PNG)
	statusKey          string
	statusText         string
	statusTTL          float64
	statusDialogType   string
	rememberMe         bool
	pressed            *widget
	presses            map[string]*mousePress
	dragging           *dragState
	sliderDragging     *widget
	selecting          *widget
	hovered            *widget
	screen             Rect
	uiScale            float64
	screenWidth        int
	screenHeight       int
	rects              map[*widget]Rect
	layoutActive       map[*widget]bool
	textFaces          map[textFaceKey]font.Face
	lowerNames         map[string]string
	scrollFrames       []*widget
	tints              map[tintKey]image.Image
	scrollFramesFor    int
	scrollRound        int
	layerPool          []*image.RGBA
	layerDepth         int
	paintCanvas        *image.RGBA
	layered            bool
	splitReached       bool
	planes             []*glueLayerPlane
	curPlane           *glueLayerPlane
	planePool          []*image.RGBA
	worldBaseCanvas    *image.RGBA
	movieFile          string
	movieImage         image.Image
	movie              *moviePlayback
	sceneBackground    bool
	debugPanel         debugPanelState
	lastClick          *widget
	lastClickAt        time.Time
	loading            bool
	loadingPath        string
	loadingProgress    float64
	worldRoot          *widget
	worldUIReady       bool
	worldLoading       bool
	worldActive        bool
	worldRenderScale   float64
	minimapImage       image.Image
	minimapTRS         map[string]string
	minimapTRSLoaded   bool
	minimapMapName     string
	minimapTileX       int
	minimapTileY       int
	minimapWorldX      float32
	minimapWorldY      float32
	minimapHasPosition bool
	// minimapIndoor is set while the minimap shows the rooms around the player.
	minimapIndoor bool
	minimapVersion     uint64
	caretOwner         *widget // the edit box whose caret is blinking
	caretCursor        int
	caretShown         bool
	caretTimer         float64
	minimapU, minimapV float64 // where the player is in the tile composite, 0..1
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
	return LoadUIEngineFromMPQSized(dataPath, locale, bgImagePath, 960, 640)
}

// LoadUIEngineFromMPQSized loads the glue UI as if the window were
// width x height pixels, which the scripts see through GetScreenWidth/Height
// and GlueParent:GetSize() while they load.
func LoadUIEngineFromMPQSized(dataPath, locale, bgImagePath string, width, height float64) (*UIEngine, error) {
	rt := NewRuntime(&hostScreen{w: width, h: height})
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
		textFaces:    make(map[textFaceKey]font.Face),
		BgImagePath:  bgImagePath,
		uiScale:      1,
	}
	rt.engine = eng
	rt.measureText = eng.measureFontString
	rt.layoutOf = eng.onDemandRect
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

// ClearHover ends the hover over a widget, with the leave its scripts expect
// (a tooltip it showed is hidden). The mouse going to the world, or the widget
// going away under it, would otherwise leave that tooltip up.
func (eng *UIEngine) ClearHover() {
	old := eng.hovered
	if old == nil {
		return
	}
	if !old.highlightLocked {
		old.highlighted = false
	}
	eng.hovered = nil
	eng.fireHoverChange(old, nil)
}

func (eng *UIEngine) Update(elapsed float64) bool {
	if elapsed <= 0 {
		return false
	}
	if eng.hovered != nil && !eng.hovered.visibleThroughParents() {
		eng.ClearHover()
	}
	statusChanged := false
	eng.Rt.dropHiddenFocus()
	eng.Rt.scriptDirty = false
	if eng.statusTTL > 0 {
		eng.statusTTL -= elapsed
		if eng.statusTTL <= 0 {
			eng.closeStatusDialog()
			statusChanged = true
		}
	}
	if eng.Rt != nil {
		eng.Rt.TickGroup(elapsed)
		eng.Rt.tickChat()
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
		for _, child := range eng.worldRoot.children {
			update(child)
		}
	}
	if eng.Rt.tickAnimations(elapsed) {
		statusChanged = true
	}
	caretChanged := eng.updateCaretBlink(elapsed)
	eng.Rt.updateStateDrivers(elapsed)
	movieChanged := eng.updateMovie(elapsed)
	return statusChanged || caretChanged || eng.Rt.scriptDirty || movieChanged
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
		return expandPluralByNumber(string(v.(lua.LString)))
	}
	return expandPluralByNumber(s)
}

// loadBLP loads an asset as an image, caching the result.
func (eng *UIEngine) loadBLP(path string) image.Image {
	if path == "" {
		return nil
	}
	if img, ok := eng.Cache[path]; ok {
		return img
	}
	if strings.HasPrefix(path, portraitImagePrefix) {
		id, err := strconv.ParseUint(path[len(portraitImagePrefix):], 10, 32)
		if err != nil || eng.Rt == nil || eng.Rt.PortraitSource == nil {
			return nil
		}
		img := eng.Rt.PortraitSource(uint32(id))
		if img == nil {
			return nil
		}
		eng.Cache[path] = img
		return img
	}
	data, err := eng.AssetLoader.ReadAsset(path)
	if err != nil {
		eng.Cache[path] = nil
		return nil
	}
	var img image.Image
	if len(data) >= 3 && string(data[:3]) == "BLP" {
		img, err = DecodeBLP(data)
	} else {
		img, err = decodeTGA(data)
	}
	if err != nil {
		log.Printf("decode %s: %v", path, err)
		eng.Cache[path] = nil
		return nil
	}
	eng.Cache[path] = img
	registerStaticTexture(img)
	return img
}

func (eng *UIEngine) Render(screenWidth, screenHeight int) *image.RGBA {
	eng.worldActive = false
	eng.worldRenderScale = 1
	return eng.render(screenWidth, screenHeight, eng.Rt.widgets["GlueParent"], true)
}

// RenderLayers renders the glue UI split around its scene models. back holds
// what the UI stacking order puts behind the 3D scene (for example the login
// scene's background texture) and is nil when nothing is behind it; front goes
// over the scene. Without any scene model front is the whole UI, as Render.
func (eng *UIEngine) RenderLayers(screenWidth, screenHeight int) (back *image.RGBA, planes []GluePlane) {
	eng.worldActive = false
	eng.worldRenderScale = 1
	eng.layered = true
	defer func() { eng.layered = false }()
	front := eng.render(screenWidth, screenHeight, eng.Rt.widgets["GlueParent"], true)
	if !eng.splitReached {
		return nil, []GluePlane{{Image: front, Blend: GlueBlendNormal}}
	}
	for _, plane := range eng.planes {
		if plane.blend == GlueBlendAdd {
			// Painted additively onto an empty canvas, so the colour channels
			// already hold the contribution; a full alpha lets the GL additive
			// blend apply it once.
			for index := 3; index < len(plane.img.Pix); index += 4 {
				plane.img.Pix[index] = 255
			}
		}
		if !glueLayerPlaneEmpty(plane) {
			planes = append(planes, GluePlane{Image: plane.img, Blend: plane.blend})
		}
	}
	return eng.paintCanvas, planes
}

func (eng *UIEngine) RenderWorld(screenWidth, screenHeight int) *image.RGBA {
	eng.worldActive = true
	eng.syncCombatLogButtons()
	eng.worldRenderScale = 1
	renderWidth, renderHeight := screenWidth, screenHeight
	bounds := image.Rect(0, 0, renderWidth, renderHeight)
	if eng.debugPanel.dragging && eng.worldBaseCanvas != nil && eng.worldBaseCanvas.Bounds().Eq(bounds) && eng.screenWidth == renderWidth && eng.screenHeight == renderHeight {
		if eng.paintCanvas == nil || !eng.paintCanvas.Bounds().Eq(bounds) {
			eng.paintCanvas = image.NewRGBA(bounds)
		}
		copy(eng.paintCanvas.Pix, eng.worldBaseCanvas.Pix)
		face := eng.cachedFace("__base13", eng.FontObj, 13*eng.uiScale)
		faceLg := eng.cachedFace("__base16", eng.FontObj, 16*eng.uiScale)
		eng.drawDebugPanel(eng.paintCanvas, face, faceLg)
		return eng.paintCanvas
	}
	return eng.render(renderWidth, renderHeight, eng.worldRoot, false)
}

func (eng *UIEngine) RenderWorldDebugPanel(screenWidth, screenHeight int) *image.RGBA {
	eng.worldActive = true
	eng.worldRenderScale = 1
	renderWidth, renderHeight := screenWidth, screenHeight
	bounds := image.Rect(0, 0, renderWidth, renderHeight)
	if eng.worldBaseCanvas == nil || !eng.worldBaseCanvas.Bounds().Eq(bounds) || eng.screenWidth != renderWidth || eng.screenHeight != renderHeight {
		return eng.RenderWorld(screenWidth, screenHeight)
	}
	if eng.paintCanvas == nil || !eng.paintCanvas.Bounds().Eq(bounds) {
		eng.paintCanvas = image.NewRGBA(bounds)
	}
	copy(eng.paintCanvas.Pix, eng.worldBaseCanvas.Pix)
	face := eng.cachedFace("__base13", eng.FontObj, 13*eng.uiScale)
	faceLg := eng.cachedFace("__base16", eng.FontObj, 16*eng.uiScale)
	if eng.debugPanel.visible {
		eng.drawDebugPanel(eng.paintCanvas, face, faceLg)
	}
	return eng.paintCanvas
}

func (eng *UIEngine) RenderWorldMinimap(screenWidth, screenHeight int) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, screenWidth, screenHeight))
	if eng == nil || eng.Rt == nil || eng.minimapImage == nil {
		return canvas
	}
	mapWidget := eng.Rt.widgets["MinimapMap"]
	if mapWidget == nil {
		return canvas
	}
	rect, ok := eng.rects[mapWidget]
	if !ok {
		return canvas
	}
	uiScale := eng.uiScale
	if uiScale <= 0 {
		uiScale = float64(screenHeight) / 768
	}
	tc := [4]float64{mapWidget.texCoordL, mapWidget.texCoordR, mapWidget.texCoordT, mapWidget.texCoordB}
	if tc[0] == 0 && tc[1] == 0 && tc[2] == 0 && tc[3] == 0 {
		tc = [4]float64{0, 1, 0, 1}
	}
	eng.drawCircularTexture(canvas, eng.minimapImage, screenScaledRect(rect, uiScale), float64(screenHeight), tc, strings.EqualFold(mapWidget.blendMode, "ADD") || strings.EqualFold(mapWidget.alphaMode, "ADD"))
	return canvas
}

// RenderWorldMinimapRegion draws the minimap into a canvas the size of its
// own rectangle and returns where on the screen that canvas belongs, so a
// minimap update does not allocate, clear and upload a whole screen.
func (eng *UIEngine) RenderWorldMinimapRegion(screenWidth, screenHeight int) (*image.RGBA, image.Rectangle) {
	full := image.Rect(0, 0, screenWidth, screenHeight)
	if eng == nil || eng.Rt == nil || eng.minimapImage == nil {
		return nil, full
	}
	mapWidget := eng.Rt.widgets["MinimapMap"]
	if mapWidget == nil {
		return nil, full
	}
	rect, ok := eng.rects[mapWidget]
	if !ok {
		return nil, full
	}
	uiScale := eng.uiScale
	if uiScale <= 0 {
		uiScale = float64(screenHeight) / 768
	}
	scaled := screenScaledRect(rect, uiScale)
	region := ScreenRect(scaled, float64(screenHeight)).Inset(-2).Intersect(full)
	if region.Empty() {
		return nil, full
	}
	// The canvas has its own origin: move the rectangle so it lands inside it.
	shift := float64(region.Dy()-screenHeight) + float64(region.Min.Y)
	local := Rect{X0: scaled.X0 - float64(region.Min.X), X1: scaled.X1 - float64(region.Min.X), Y0: scaled.Y0 + shift, Y1: scaled.Y1 + shift}
	tc := [4]float64{mapWidget.texCoordL, mapWidget.texCoordR, mapWidget.texCoordT, mapWidget.texCoordB}
	if tc[0] == 0 && tc[1] == 0 && tc[2] == 0 && tc[3] == 0 {
		tc = [4]float64{0, 1, 0, 1}
	}
	canvas := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	eng.drawCircularTexture(canvas, eng.minimapImage, local, float64(region.Dy()), tc, strings.EqualFold(mapWidget.blendMode, "ADD") || strings.EqualFold(mapWidget.alphaMode, "ADD"))
	return canvas, region
}

func (eng *UIEngine) RenderWorldChat(screenWidth, screenHeight int) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, screenWidth, screenHeight))
	if eng == nil || eng.Rt == nil {
		return canvas
	}
	edit := eng.worldChatEditBox()
	if edit == nil {
		return canvas
	}
	rect, ok := eng.rects[edit]
	if !ok {
		return canvas
	}
	uiScale := eng.uiScale
	if uiScale <= 0 {
		uiScale = float64(screenHeight) / 768
	}
	face := eng.cachedFace("__base13", eng.FontObj, 13*uiScale)
	faceLg := eng.cachedFace("__base16", eng.FontObj, 16*uiScale)
	eng.drawEditText(canvas, face, faceLg, edit, rect, float64(screenHeight))
	return canvas
}

func (eng *UIEngine) WorldChatFocused() bool {
	if eng != nil && eng.Rt != nil {
		eng.Rt.dropHiddenFocus()
	}
	return eng != nil && eng.Rt != nil && eng.Rt.focused == eng.worldChatEditBox()
}

func (eng *UIEngine) worldInputPoint(x, y float64) (float64, float64) {
	if eng.worldActive && eng.worldRenderScale > 0 && eng.worldRenderScale < 1 {
		return x * eng.worldRenderScale, y * eng.worldRenderScale
	}
	return x, y
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
	eng.splitReached = false
	eng.curPlane = nil
	for _, plane := range eng.planes {
		eng.planePool = append(eng.planePool, plane.img)
	}
	eng.planes = eng.planes[:0]
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
			// Only the frame's own area of the layer is cleared and composited.
			// The frame's own background, border and artwork regions lie under
			// what it scrolls.
			underContent := func(child *widget) bool {
				return (child.kind == kindTexture || child.kind == kindFontString) && child.layerLevel < layerOverlay
			}
			for _, child := range orderedChildren(w.children) {
				if child.shown && underContent(child) && !childDrawsBehindParent(child, w) {
					paint(target, child, rect)
				}
			}
			clip := ScreenRect(screenScaledRect(rect, eng.uiScale), float64(target.Bounds().Dy())).Intersect(target.Bounds())
			children := eng.acquireLayerRegion(target.Bounds(), clip)
			for _, child := range orderedChildren(w.children) {
				if child.shown && child == w.scrollChild {
					paint(children, child, rect)
				}
			}
			if !clip.Empty() {
				draw.Draw(target, clip, children, clip.Min, draw.Over)
			}
			eng.releaseLayer()
			for _, child := range orderedChildren(w.children) {
				if child.shown && child != w.scrollChild && !childDrawsBehindParent(child, w) && !underContent(child) {
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
				if w.kind == kindStatusBar && statusBarBackgroundRegion(child) {
					continue
				}
				paint(target, child, rect)
			}
		}
	}
	paintWidget = func(target *image.RGBA, w *widget, parent Rect) {
		rect := eng.layoutRect(w, parent)
		previousScale := eng.curScale
		eng.curScale = effectiveScaleOf(w)
		defer func() { eng.curScale = previousScale }()
		if eng.worldActive && w.name == "MinimapMap" {
			return
		}
		if w.unanchored && len(w.points) == 0 && w.kind != kindTexture && w.kind != kindFontString {
			return
		}

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
			if w.textureFile != "" || (w.name == "MinimapMap" && eng.minimapImage != nil) {
				tc := [4]float64{w.texCoordL, w.texCoordR, w.texCoordT, w.texCoordB}
				img := eng.minimapImage
				if w.name != "MinimapMap" || img == nil {
					img = eng.loadBLP(w.textureFile)
				}
				if img != nil {
					if tc[0] == 0 && tc[1] == 0 && tc[2] == 0 && tc[3] == 0 && w.name != "MinimapMap" {
						// A script set the rectangle to nothing (SetTexCoord(0, 0, 0, 0)); that shows
						// no texels, not the whole file.
						break
					}
					if tc[0] == 0 && tc[1] == 0 && tc[2] == 0 && tc[3] == 0 {
						tc = [4]float64{0, 1, 0, 1}
					}
					if !w.vertexColor.isZero() {
						img = eng.tintTextureImage(w.textureFile, img, w.vertexColor)
					}
					if w.name == "MinimapMap" {
						eng.drawCircularTexture(target, img, scaledRect, float64(screenHeight), tc, strings.EqualFold(w.blendMode, "ADD") || strings.EqualFold(w.alphaMode, "ADD"))
					} else if w.horizTile || w.vertTile {
						eng.drawTiledTexture(target, img, scaledRect, float64(screenHeight), tc, w.horizTile, w.vertTile, strings.EqualFold(w.blendMode, "ADD") || strings.EqualFold(w.alphaMode, "ADD"))
					} else {
						if w.rotation != 0 {
							drawRotatedTexture(target, img, scaledRect, float64(screenHeight), tc, w.rotation)
							break
						}
						drawSubModeFilter(target, img, scaledRect, float64(screenHeight), tc, strings.EqualFold(w.blendMode, "ADD") || strings.EqualFold(w.alphaMode, "ADD"))
					}
				}
			} else if !w.vertexColor.isZero() {
				if strings.EqualFold(w.blendMode, "MOD") || strings.EqualFold(w.alphaMode, "MOD") {
					eng.drawTextureModColor(target, scaledRect, w.vertexColor)
				} else {
					eng.drawTextureColor(target, scaledRect, w.vertexColor)
				}
			}

		case kindStatusBar:
			// The bar's own BACKGROUND regions lie under its fill.
			for _, child := range orderedChildren(w.children) {
				if child.shown && statusBarBackgroundRegion(child) {
					paint(target, child, rect)
				}
			}
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
					if !w.statusBarColor.isZero() {
						img = eng.tintTextureImage(w.statusBarTexture.textureFile, img, w.statusBarColor)
					}
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
				maxLines := w.maxLines
				if w.height > 0 && !w.autoTextWidth {
					// Lines that do not fit the height a FontString was given are not
					// drawn; a long name keeps its first line.
					if lineHeight := float64(f.Metrics().Height) / 64; lineHeight > 0 {
						fit := int(scaledRect.H()/lineHeight + 0.001)
						if fit < 1 {
							fit = 1
						}
						if maxLines == 0 || fit < maxLines {
							maxLines = fit
						}
					}
				}
				if maxLines > 0 {
					lines := strings.Split(text, "\n")
					if len(lines) > maxLines {
						text = strings.Join(lines[:maxLines], "\n")
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
		if w.kind == kindEditBox && !(eng.worldActive && w == eng.worldChatEditBox()) {
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
		if eng.layered {
			// Scene models sit in the UI stacking order: what was painted before
			// the first one belongs behind the 3D scene, the rest in front of it.
			if !eng.splitReached && (w.kind == kindModel || w.kind == kindModelFFX) && w.modelFile != "" {
				eng.splitReached = true
				eng.curPlane = nil
			}
			if eng.splitReached && (target == eng.paintCanvas || eng.isPlaneImage(target)) {
				target = eng.planeFor(frontBlend(w), target.Bounds()).img
			}
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
		// A model frame draws no 2D pixels, so there is nothing to fade; its
		// rectangle is still laid out, because the 3D scene is drawn into it.
		if (w.kind == kindModel || w.kind == kindModelFFX) && len(w.children) == 0 {
			eng.layoutRect(w, parent)
			return
		}
		// A texture only touches its own rectangle, so the fade and the
		// composite stay inside it instead of walking the whole window.
		region := target.Bounds()
		if w.kind == kindTexture && len(w.children) == 0 && !legacyFullCanvasPaint {
			scaled := screenScaledRect(eng.layoutRect(w, parent), uiScale)
			region = ScreenRect(scaled, float64(target.Bounds().Dy())).Intersect(target.Bounds())
			if region.Empty() {
				return
			}
		}
		if !legacyFullCanvasPaint && region == target.Bounds() {
			// A frame fades with everything inside it, so only that area needs a layer.
			if subtree := eng.subtreeRegion(w, parent, uiScale, target.Bounds()); !subtree.Empty() {
				region = subtree
			}
		}
		layer := eng.acquireLayerRegion(target.Bounds(), region)
		paintWidget(layer, w, parent)
		var fade [256]uint8
		for value := range fade {
			fade[value] = uint8(float64(value) * alpha)
		}
		additive := w.kind == kindTexture && (strings.EqualFold(w.blendMode, "ADD") || strings.EqualFold(w.alphaMode, "ADD"))
		// The layer is premultiplied RGBA, so fading it scales every channel;
		// scaling only alpha left the colour at full strength. A faded additive
		// texture then adds its faded colour to what is there; it does not blend
		// over it.
		for y := region.Min.Y; y < region.Max.Y; y++ {
			offset := layer.PixOffset(region.Min.X, y)
			row := layer.Pix[offset : offset+region.Dx()*4]
			var targetRow []byte
			if additive {
				targetRow = target.Pix[target.PixOffset(region.Min.X, y):][:region.Dx()*4]
			}
			for index := 0; index+3 < len(row); index += 4 {
				if row[index+3] == 0 {
					continue
				}
				row[index] = fade[row[index]]
				row[index+1] = fade[row[index+1]]
				row[index+2] = fade[row[index+2]]
				row[index+3] = fade[row[index+3]]
				if additive {
					for channel := 0; channel < 3; channel++ {
						sum := int(targetRow[index+channel]) + int(row[index+channel])
						if sum > 255 {
							sum = 255
						}
						targetRow[index+channel] = uint8(sum)
					}
					if row[index+3] > targetRow[index+3] {
						targetRow[index+3] = row[index+3]
					}
				}
			}
		}
		if !additive {
			draw.Draw(target, region, layer, region.Min, draw.Over)
		}
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
	// With a split, the debug panel and the transparent-pixel fixup go on the
	// normal front plane; otherwise on the single canvas as before.
	output := canvas
	if eng.layered && eng.splitReached {
		output = eng.planeFor(GlueBlendNormal, bounds).img
	}
	if drawBackground && eng.sceneBackground {
		targets := []*image.RGBA{output}
		if eng.layered && eng.splitReached {
			targets = targets[:0]
			for _, plane := range eng.planes {
				if plane.blend == GlueBlendNormal {
					targets = append(targets, plane.img)
				}
			}
		}
		for _, target := range targets {
			for index := 3; index < len(target.Pix); index += 4 {
				if target.Pix[index] == 0 {
					target.Pix[index] = 1
				}
			}
		}
	}
	// The picture without the debug panel is kept only while the panel is up, to
	// move it without a repaint.
	if eng.worldActive && eng.debugPanel.visible {
		if eng.worldBaseCanvas == nil || !eng.worldBaseCanvas.Bounds().Eq(canvas.Bounds()) {
			eng.worldBaseCanvas = image.NewRGBA(canvas.Bounds())
		}
		copy(eng.worldBaseCanvas.Pix, canvas.Pix)
	} else {
		eng.worldBaseCanvas = nil
	}
	if eng.debugPanel.visible {
		eng.drawDebugPanel(output, face, faceLg)
	}

	return output
}

func (eng *UIEngine) updateScrollFrames() {
	if eng == nil || eng.Rt == nil {
		return
	}
	// The scroll frames are found again only when widgets were made.
	if eng.scrollFramesFor != len(eng.Rt.widgets) {
		eng.scrollFrames = eng.scrollFrames[:0]
		for _, w := range eng.Rt.widgets {
			if w.kind == kindScrollFrame && w.scrollChild != nil {
				eng.scrollFrames = append(eng.scrollFrames, w)
			}
		}
		eng.scrollFramesFor = len(eng.Rt.widgets)
	}
	eng.scrollRound++
	for _, frame := range eng.scrollFrames {
		// A hidden frame's extent changes only when something is shown in it, so
		// it is looked at now and then, not every frame.
		if !frame.visibleThroughParents() && eng.scrollRound%30 != 0 {
			continue
		}
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

// acquireLayerRegion is acquireLayer that clears only region; the caller
// composites nothing outside it.
func (eng *UIEngine) acquireLayerRegion(bounds, region image.Rectangle) *image.RGBA {
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
	draw.Draw(layer, region.Intersect(bounds), &image.Uniform{C: color.Transparent}, image.Point{}, draw.Src)
	return layer
}

// subtreeRegion is the screen area a widget and its shown descendants cover,
// widened a little for text and borders that overhang their rectangles.
func (eng *UIEngine) subtreeRegion(w *widget, parent Rect, uiScale float64, bounds image.Rectangle) image.Rectangle {
	var union image.Rectangle
	visited := map[*widget]bool{}
	var visit func(*widget, Rect)
	visit = func(current *widget, parentRect Rect) {
		if visited[current] {
			return
		}
		visited[current] = true
		rect := eng.layoutRect(current, parentRect)
		union = union.Union(ScreenRect(screenScaledRect(rect, uiScale), float64(bounds.Dy())))
		for _, child := range current.children {
			if child != nil && child.shown {
				visit(child, rect)
			}
		}
	}
	visit(w, parent)
	margin := int(48 * uiScale / 1.78)
	return union.Inset(-margin).Intersect(bounds)
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
	if eng.fireMouseWheel(delta) {
		return true
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
	// What is hidden is measured when it is shown (and when its text is set).
	if !w.shown {
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
	if !eng.worldActive && w.buttonLabel != nil && w.buttonLabel.textWidth > 0 && eng.Rt.lookup(w.name+"Middle") == nil && w.width < w.buttonLabel.textWidth+50 {
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
	// Text is measured in the widget's own units, whatever its scale.
	previousScale := eng.curScale
	eng.curScale = 1
	defer func() { eng.curScale = previousScale }()
	text := eng.resolveText(w.text)
	if w.parent != nil && (w.parent.kind == kindButton || w.parent.kind == kindCheckButton) && w.parent.buttonLabel == w && w.parent.text != "" {
		text = eng.resolveText(w.parent.text)
	}
	fontFace := eng.faceFor(w, face, faceLg)
	if fontFace == nil {
		return
	}
	key := textMeasureKey{valid: true, text: text, face: fontFace, scale: eng.uiScale, autoWidth: w.autoTextWidth, explicit: w.explicitWidth, autoHeight: w.autoTextHeight, justify: eng.textJustify(w), width: w.width, height: w.height}
	if !w.autoTextWidth && w.width > 0 {
		key.wrap = int(w.width * eng.uiScale)
	}
	if w.measured.valid && w.measured == key {
		return
	}
	defer func() {
		key.width, key.height = w.width, w.height
		w.measured = key
		// The key was taken before the size was written back; measure again
		// from the written values next time only if they differ.
		w.measured.width, w.measured.height = w.width, w.height
	}()
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
		e := effectiveScaleOf(w)
		rect := Rect{X0: parentRect.X0, Y0: parentRect.Y1 - (shift+w.height)*e, X1: parentRect.X0 + w.width*e, Y1: parentRect.Y1 - shift*e}
		eng.rects[w] = rect
		w.renderRect = rect
		w.hasRenderRect = true
		return rect
	}
	// A texture given no size and at most one anchor is as large as its file.
	if w.kind == kindTexture && w.textureFile != "" && w.width == 0 && w.height == 0 && len(w.points) <= 1 {
		if img := eng.loadBLP(w.textureFile); img != nil {
			w.width, w.height = float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
			w.naturalSize = true
		}
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
	if w.unanchoredTopLevelFrame() {
		// A frame with no anchor points has no position in the original client,
		// so neither it nor anything anchored to it is drawn.
		rect = Rect{X0: -1e6, Y0: -1e6, X1: -1e6, Y1: -1e6}
	}
	eng.rects[w] = rect
	w.renderRect = rect
	w.hasRenderRect = true
	return rect
}

// glueParentRect is the glue root's rectangle. The original's GlueParent_OnLoad
// anchors it at 16:9 inside a wider screen, leaving bars; this client lets it
// span the whole screen (and GlueParent:GetSize() says so), so the login scene,
// its fade-in cover and the controls all stretch to fit the window.
func (eng *UIEngine) glueParentRect() Rect {
	return eng.screen
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
			if w.justifyH != "" {
				child.justifyH = w.justifyH
			}
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

// editCaretBand is the vertical extent of the caret and selection: one font
// line tall, centred on the text rectangle, never taller than it. (It used to
// span the whole rectangle minus a fixed margin, which overshot the box.)
func editCaretBand(face font.Face, text image.Rectangle) (top, bottom int) {
	metrics := face.Metrics()
	height := (metrics.Ascent + metrics.Descent).Ceil()
	if height < 1 || height > text.Dy() {
		height = text.Dy()
	}
	top = text.Min.Y + (text.Dy()-height)/2
	return top, top + height
}

// statusBarBackgroundRegion reports a texture or text of a status bar's
// BACKGROUND layer, which is drawn before the bar's fill.
func statusBarBackgroundRegion(w *widget) bool {
	return (w.kind == kindTexture || w.kind == kindFontString) && w.layerLevel == layerBackground
}

// editTextAlpha is the opacity of edit box text: a colour set without an alpha
// is opaque.
func editTextAlpha(alpha float64) float64 {
	if alpha <= 0 {
		return 1
	}
	return alpha
}

func (eng *UIEngine) drawEditText(canvas *image.RGBA, face, faceLg font.Face, w *widget, rect Rect, screenHeight float64) {
	text := editDisplayText(w)
	textWidget := eng.editTextWidget(w)
	textFace := eng.faceFor(textWidget, face, faceLg)
	textRect := eng.editTextRect(rect, w)
	screenTextRect := screenScaledRect(textRect, eng.uiScale)
	textColor := color.RGBA{R: 235, G: 235, B: 235, A: 255}
	if !textWidget.textColor.isZero() {
		textColor = color.RGBA{R: uint8(textWidget.textColor.r * 255), G: uint8(textWidget.textColor.g * 255), B: uint8(textWidget.textColor.b * 255), A: uint8(editTextAlpha(textWidget.textColor.a) * 255)}
	}
	start, end := editSelection(w)
	if eng.Rt.focused == w && start != end {
		dst := ScreenRect(screenTextRect, screenHeight)
		origin := eng.editTextOrigin(textFace, text, dst, textWidget)
		startWidth := font.MeasureString(textFace, string([]rune(text)[:start])).Ceil()
		endWidth := font.MeasureString(textFace, string([]rune(text)[:end])).Ceil()
		top, bottom := editCaretBand(textFace, dst)
		selection := image.Rect(origin+startWidth, top, origin+endWidth, bottom)
		draw.Draw(canvas, selection, &image.Uniform{C: color.RGBA{R: 35, G: 100, B: 180, A: 180}}, image.Point{}, draw.Over)
	}
	if text != "" {
		eng.drawTextAlignedWidget(canvas, textFace, text, screenTextRect, screenHeight, textColor, textWidget)
	}
	if eng.Rt.focused == w && eng.caretShown {
		dst := ScreenRect(screenTextRect, screenHeight)
		origin := eng.editTextOrigin(textFace, text, dst, textWidget)
		width := font.MeasureString(textFace, string([]rune(text)[:clampCursor(w.cursor, len([]rune(text)))])).Ceil()
		top, bottom := editCaretBand(textFace, dst)
		// Wow.exe (CSimpleEditBox layout): the caret is a white box one font line
		// tall and 4 / (768 * sqrt(aspect^2 + 1)) of the screen height wide.
		aspect := float64(eng.screenWidth) / math.Max(1, float64(eng.screenHeight))
		boxWidth := int(math.Round(4 * float64(eng.screenHeight) / (768 * math.Sqrt(aspect*aspect+1))))
		if boxWidth < 1 {
			boxWidth = 1
		}
		caretX := origin + width
		caret := image.Rect(caretX, top, caretX+boxWidth, bottom)
		draw.Draw(canvas, caret, &image.Uniform{C: color.RGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, draw.Over)
	}
}

// updateCaretBlink is CSimpleEditBox's OnUpdate: the caret of the focused edit
// box shows as soon as the cursor or focus changes, then flips every blink
// speed seconds (0.5 unless set). It reports whether the caret flipped.
func (eng *UIEngine) updateCaretBlink(elapsed float64) bool {
	focused := eng.Rt.focused
	if focused == nil || focused.kind != kindEditBox {
		if eng.caretOwner != nil {
			eng.caretOwner = nil
			return true
		}
		return false
	}
	if focused != eng.caretOwner || focused.cursor != eng.caretCursor {
		eng.caretOwner, eng.caretCursor = focused, focused.cursor
		eng.caretShown, eng.caretTimer = true, 0
		return true
	}
	speed := focused.blinkSpeed
	if speed == 0 && !focused.blinkSet {
		speed = 0.5
	}
	if speed == 0 {
		return false
	}
	eng.caretTimer += elapsed
	if eng.caretTimer > speed {
		eng.caretShown = !eng.caretShown
		eng.caretTimer = 0
		return true
	}
	return false
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

func (eng *UIEngine) drawTextureModColor(canvas *image.RGBA, r Rect, c rgba) {
	dst := ScreenRect(r, float64(canvas.Bounds().Dy())).Intersect(canvas.Bounds())
	if dst.Empty() || c.a <= 0 {
		return
	}
	alpha := c.a
	if alpha > 1 {
		alpha = 1
	}
	red := clampImageChannel(c.r)
	green := clampImageChannel(c.g)
	blue := clampImageChannel(c.b)
	// A lookup table per channel gives the same rounded result as computing it
	// for every pixel.
	var lutR, lutG, lutB [256]uint8
	for value := range lutR {
		lutR[value] = uint8(math.Round(float64(value) * (1 - alpha + red*alpha)))
		lutG[value] = uint8(math.Round(float64(value) * (1 - alpha + green*alpha)))
		lutB[value] = uint8(math.Round(float64(value) * (1 - alpha + blue*alpha)))
	}
	for y := dst.Min.Y; y < dst.Max.Y; y++ {
		offset := canvas.PixOffset(dst.Min.X, y)
		row := canvas.Pix[offset : offset+dst.Dx()*4]
		for index := 0; index+2 < len(row); index += 4 {
			row[index] = lutR[row[index]]
			row[index+1] = lutG[row[index+1]]
			row[index+2] = lutB[row[index+2]]
		}
	}
}

func (eng *UIEngine) drawCircularTexture(canvas *image.RGBA, source image.Image, rect Rect, screenHeight float64, tc [4]float64, additive bool) {
	layer := eng.acquireLayer(canvas.Bounds())
	drawSubModeFilter(layer, source, rect, screenHeight, tc, additive)
	cx := (rect.X0 + rect.X1) / 2
	cy := screenHeight - (rect.Y0+rect.Y1)/2
	radius := math.Min(rect.W(), rect.H()) / 2
	minX := int(math.Max(0, rect.X0))
	maxX := int(math.Min(float64(canvas.Bounds().Max.X), rect.X1+1))
	minY := int(math.Max(0, cy-radius))
	maxY := int(math.Min(float64(canvas.Bounds().Max.Y), cy+radius+1))
	radiusSquared := radius * radius
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			if dx*dx+dy*dy > radiusSquared {
				layer.SetRGBA(x, y, color.RGBA{})
			}
		}
	}
	draw.Draw(canvas, canvas.Bounds(), layer, layer.Bounds().Min, draw.Over)
	eng.releaseLayer()
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
	x, y = eng.worldInputPoint(x, y)
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
	eng.startDragsIfNeeded(x, y)
	target := eng.hitTest(x, y)
	if target == eng.hovered {
		return false
	}
	old := eng.hovered
	if eng.hovered != nil && !eng.hovered.highlightLocked {
		eng.hovered.highlighted = false
	}
	eng.hovered = target
	if target != nil && (target.kind == kindButton || target.kind == kindCheckButton) {
		target.highlighted = true
	}
	eng.fireHoverChange(old, target)
	return true
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
	eng.Rt.dropHiddenFocus()
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
	eng.Rt.dropHiddenFocus()
	if eng.worldLoading && key == window.KeyEnter {
		return true
	}
	if eng.worldUIReady && key == window.KeyEnter && eng.Rt.focused == nil {
		return eng.activateWorldChat()
	}
	if eng.worldUIReady && eng.worldActive && eng.Rt.focused == nil && eng.handleBindingKey(key, eng.keyMods) {
		return true
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
			if eng.worldActive && eng.closeWorldPanel() {
				return true
			}
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

func (eng *UIEngine) closeWorldPanel() bool {
	for _, name := range []string{"VideoOptionsFrame", "AudioOptionsFrame", "InterfaceOptionsFrame", "KeyBindingFrame", "MacroFrame", "QuestLogFrame", "CharacterFrame", "SpellBookFrame"} {
		frame := eng.Rt.widgets[name]
		if frame == nil || !frame.shown {
			continue
		}
		return eng.Rt.Execute(fmt.Sprintf("%s:Hide()", name), "@world-escape-close-"+name+".lua")
	}
	return false
}

func (eng *UIEngine) handleWorldHotkey(key window.Key) bool {
	var frame string
	switch key {
	case window.KeyL:
		frame = "QuestLogFrame"
	case window.KeyC:
		frame = "CharacterFrame"
	case window.KeyP:
		frame = "SpellBookFrame"
	default:
		return false
	}
	if eng.Rt.widgets[frame] == nil {
		return false
	}
	script := fmt.Sprintf(`
local target = %s
local wasShown = target:IsShown()
if GameMenuFrame then GameMenuFrame:Hide() end
for _, name in ipairs({"QuestLogFrame", "CharacterFrame", "SpellBookFrame"}) do
    local other = _G[name]
    if other and other ~= target then other:Hide() end
end
if wasShown then target:Hide() else target:Show() end
`, frame)
	return eng.Rt.Execute(script, "@world-hotkey-"+frame+".lua")
}

func (eng *UIEngine) HandleKeyWithMods(key window.Key, mods window.ModifierKey) bool {
	eng.Rt.dropHiddenFocus()
	eng.keyMods = mods
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
	eng.Rt.dropHiddenFocus()
	if eng.worldUIReady && eng.worldActive && eng.handleBindingKeyUp(key) {
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
	// A frame's regions are drawn with the frame at the frame's own strata, so
	// they come before a child frame placed in a higher strata (a LOW
	// LoginScene's textures lie under its MEDIUM model frames) whatever the
	// region's own default strata says. Other orderings are unchanged.
	if firstRegion, secondRegion := isRegionWidget(first), isRegionWidget(second); firstRegion != secondRegion {
		region, frame := first, second
		if secondRegion {
			region, frame = second, first
		}
		if region.parent != nil && frame.frameStrata > region.parent.frameStrata {
			return region == first
		}
	}
	if first.frameStrata != second.frameStrata {
		return first.frameStrata < second.frameStrata
	}
	if first.frameLevel != second.frameLevel {
		return first.frameLevel < second.frameLevel
	}
	// Frame levels are absolute: of two siblings of one level, the one holding a
	// frame of a higher level (a unit frame's border frame, nested two deep) is
	// drawn after the one that holds none, whichever was made first.
	if !isRegionWidget(first) && !isRegionWidget(second) && first.frameStrata == second.frameStrata {
		if firstTop, secondTop := deepestFrameLevel(first), deepestFrameLevel(second); firstTop != secondTop {
			return firstTop < secondTop
		}
	}
	return first.layerLevel < second.layerLevel
}

// deepestFrameLevel is the highest frame level among a frame and the frames below
// it that share its strata.
func deepestFrameLevel(w *widget) int {
	level := w.frameLevel
	for _, child := range w.children {
		if isRegionWidget(child) || child.frameStrata != w.frameStrata {
			continue
		}
		if deeper := deepestFrameLevel(child); deeper > level {
			level = deeper
		}
	}
	return level
}

func isRegionWidget(w *widget) bool {
	return w != nil && (w.kind == kindTexture || w.kind == kindFontString)
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
		inL := int(bd.insetL * eng.pxUnit())
		inR := int(bd.insetR * eng.pxUnit())
		inT := int(bd.insetT * eng.pxUnit())
		inB := int(bd.insetB * eng.pxUnit())
		inner := image.Rect(dst.Min.X+inL, dst.Min.Y+inT, dst.Max.X-inR, dst.Max.Y-inB)
		if inner.Dx() > 0 && inner.Dy() > 0 {
			if bd.tile {
				eng.drawTiled(canvas, inner, bgImg, bd.tileSize)
			} else {
				xdraw.NearestNeighbor.Scale(canvas, inner, bgImg, bgImg.Bounds(), xdraw.Over, nil)
			}
		}
	} else if !bd.bgColor.isZero() {
		inL := int(bd.insetL * eng.pxUnit())
		inR := int(bd.insetR * eng.pxUnit())
		inT := int(bd.insetT * eng.pxUnit())
		inB := int(bd.insetB * eng.pxUnit())
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
		drawBackdropEdge(canvas, dst, edgeImg, bd.edgeSize*eng.pxUnit())
	} else if bd.edgeFile != "" {
		drawBorder(canvas, dst, color.RGBA{R: 80, G: 120, B: 150, A: 200}, 1)
	}
}

// tintKey names a texture tinted with a colour (to a ten-thousandth, as the
// string keys it replaces were).
type tintKey struct {
	backdrop   bool
	path       string
	r, g, b, a int32
}

func makeTintKey(backdrop bool, path string, tint rgba) tintKey {
	round := func(v float64) int32 { return int32(math.Round(v * 10000)) }
	return tintKey{backdrop, path, round(tint.r), round(tint.g), round(tint.b), round(tint.a)}
}

func (eng *UIEngine) tintBackdropImage(path string, source image.Image, tint rgba) image.Image {
	return eng.tinted(makeTintKey(true, path, tint), source, tint)
}

func (eng *UIEngine) tintTextureImage(path string, source image.Image, tint rgba) image.Image {
	return eng.tinted(makeTintKey(false, path, tint), source, tint)
}

func (eng *UIEngine) tinted(key tintKey, source image.Image, tint rgba) image.Image {
	if imageData, ok := eng.tints[key]; ok {
		return imageData
	}
	if eng.tints == nil {
		eng.tints = make(map[tintKey]image.Image)
	}
	imageData := tintImage(source, tint)
	eng.tints[key] = imageData
	// Never changed once made, so its scaled copies can be kept.
	registerStaticTexture(imageData)
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
		size := int(tileSize * eng.pxUnit())
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
		tileWidth = int(math.Round(float64(tile.Bounds().Dx()) * eng.pxUnit()))
	}
	if vert {
		tileHeight = int(math.Round(float64(tile.Bounds().Dy()) * eng.pxUnit()))
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
		lowerFile := eng.lowerName(style.FontFile)
		switch {
		case strings.Contains(lowerFile, "morpheus"):
			fontObj = eng.FontObjSm
		case strings.Contains(lowerFile, "arialn"):
			if eng.FontObjArial != nil {
				fontObj = eng.FontObjArial
			}
		}
	} else if lowerObject := eng.lowerName(w.fontObject); strings.Contains(lowerObject, "large") || strings.Contains(lowerObject, "huge") {
		if unit := eng.pxUnit(); unit != eng.uiScale {
			return eng.cachedFace("__base16", eng.FontObj, 16*unit)
		}
		return fallbackLarge
	}
	if size == 13 && style == nil {
		if unit := eng.pxUnit(); unit != eng.uiScale {
			return eng.cachedFace("__base13", eng.FontObj, 13*unit)
		}
		return fallback
	}
	return eng.cachedFace(fontKey, fontObj, size*eng.pxUnit(), fallback)
}

// textFaceKey names a font at a size (in thousandths), without building a string.
type textFaceKey struct {
	name string
	size int64
}

// lowerName is strings.ToLower for the few names fonts and font objects go by.
func (eng *UIEngine) lowerName(name string) string {
	if lower, ok := eng.lowerNames[name]; ok {
		return lower
	}
	if eng.lowerNames == nil {
		eng.lowerNames = make(map[string]string)
	}
	lower := strings.ToLower(name)
	eng.lowerNames[name] = lower
	return lower
}

func (eng *UIEngine) cachedFace(key string, fontObj *opentype.Font, size float64, fallback ...font.Face) font.Face {
	if eng.textFaces == nil {
		eng.textFaces = make(map[textFaceKey]font.Face)
	}
	cacheKey := textFaceKey{name: key, size: int64(math.Round(size * 1000))}
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
	if full := ScreenRect(r, screenHeight); full.Dx() > 0 && full.Dy() > 0 {
		if onCanvas := full.Intersect(canvas.Bounds()); !onCanvas.Empty() && drawCachedTexture(canvas, img, full, onCanvas, tc, additive) {
			return
		}
	}
	srcImg, ok := resolveTextureSubRGBA(img, tc)
	if !ok {
		return
	}
	dst := ScreenRect(r, screenHeight)
	if dst.Dx() <= 0 || dst.Dy() <= 0 {
		return
	}
	clipped := dst.Intersect(canvas.Bounds())
	if clipped.Empty() {
		return
	}
	source := clipTextureSource(srcImg, dst, clipped)
	if !additive {
		xdraw.BiLinear.Scale(canvas, clipped, source, source.Bounds(), xdraw.Over, nil)
		return
	}
	blend := acquireBlendScratch(clipped)
	defer releaseBlendScratch(blend)
	xdraw.BiLinear.Scale(blend, blend.Bounds(), source, source.Bounds(), xdraw.Src, nil)
	for y := clipped.Min.Y; y < clipped.Max.Y; y++ {
		dstOff := canvas.PixOffset(clipped.Min.X, y)
		srcOff := blend.PixOffset(clipped.Min.X, y)
		for x := clipped.Min.X; x < clipped.Max.X; x++ {
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

func clipTextureSource(source image.Image, destination, clipped image.Rectangle) image.Image {
	if destination == clipped {
		return source
	}
	bounds := source.Bounds()
	if destination.Dx() <= 0 || destination.Dy() <= 0 || bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return source
	}
	x0 := bounds.Min.X + int(math.Floor(float64(clipped.Min.X-destination.Min.X)*float64(bounds.Dx())/float64(destination.Dx())))
	x1 := bounds.Min.X + int(math.Ceil(float64(clipped.Max.X-destination.Min.X)*float64(bounds.Dx())/float64(destination.Dx())))
	y0 := bounds.Min.Y + int(math.Floor(float64(clipped.Min.Y-destination.Min.Y)*float64(bounds.Dy())/float64(destination.Dy())))
	y1 := bounds.Min.Y + int(math.Ceil(float64(clipped.Max.Y-destination.Min.Y)*float64(bounds.Dy())/float64(destination.Dy())))
	x0 = max(x0, bounds.Min.X)
	y0 = max(y0, bounds.Min.Y)
	x1 = min(x1, bounds.Max.X)
	y1 = min(y1, bounds.Max.Y)
	if x1 <= x0 || y1 <= y0 {
		return source
	}
	if rgba, ok := source.(*image.RGBA); ok {
		return rgba.SubImage(image.Rect(x0, y0, x1, y1))
	}
	result := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	draw.Draw(result, result.Bounds(), source, image.Point{X: x0, Y: y0}, draw.Src)
	return result
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
	// Non-RGBA sources (BLP decodes to *image.NRGBA) convert through draw.Draw,
	// which has native fast paths, instead of a per-pixel dst.Set(img.At())
	// that allocates a color.Color for every pixel on every draw. Flips are
	// applied to the converted rows afterwards.
	draw.Draw(dst, image.Rect(0, 0, width, height), img, image.Point{X: l, Y: tp}, draw.Src)
	if flipX {
		for y := 0; y < height; y++ {
			row := dst.Pix[dst.PixOffset(0, y):]
			for left, right := 0, width-1; left < right; left, right = left+1, right-1 {
				for c := 0; c < 4; c++ {
					row[left*4+c], row[right*4+c] = row[right*4+c], row[left*4+c]
				}
			}
		}
	}
	if flipY {
		scratch := make([]byte, width*4)
		for top, bottom := 0, height-1; top < bottom; top, bottom = top+1, bottom-1 {
			a := dst.Pix[dst.PixOffset(0, top) : dst.PixOffset(0, top)+width*4]
			b := dst.Pix[dst.PixOffset(0, bottom) : dst.PixOffset(0, bottom)+width*4]
			copy(scratch, a)
			copy(a, b)
			copy(b, scratch)
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
		padding := fontOutlineRadius(shadow.Outline)
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
		radius := 0
		if shadow != nil {
			radius = fontOutlineRadius(shadow.Outline)
		}
		if radius > 0 {
			outlineColor := color.RGBA{A: 255}
			for offsetY := -radius; offsetY <= radius; offsetY++ {
				for offsetX := -radius; offsetX <= radius; offsetX++ {
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

// fontOutlineRadius is how far a font's outline reaches around its glyphs: one
// pixel for NORMAL and two for THICK.
func fontOutlineRadius(outline string) int {
	switch {
	case strings.EqualFold(outline, "THICK"):
		return 2
	case strings.EqualFold(outline, "NORMAL"):
		return 1
	}
	return 0
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

// expandPluralByNumber resolves each "|4singular:plural;" of drawn text by the
// number written before it.
func expandPluralByNumber(text string) string {
	if !strings.Contains(text, "|4") {
		return text
	}
	var out strings.Builder
	number := 0
	digits := false
	for index := 0; index < len(text); {
		if strings.HasPrefix(text[index:], "|4") {
			if end := strings.IndexByte(text[index:], ';'); end > 0 {
				out.WriteString(expandPluralMarkup(text[index:index+end+1], number))
				index += end + 1
				digits = false
				continue
			}
		}
		ch := text[index]
		if ch >= '0' && ch <= '9' {
			if !digits {
				number = 0
			}
			number = number*10 + int(ch-'0')
			digits = true
		} else {
			digits = false
		}
		out.WriteByte(ch)
		index++
	}
	return out.String()
}
