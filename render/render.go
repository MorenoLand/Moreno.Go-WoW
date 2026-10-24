package render

import (
	"encoding/binary"
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/MorenoLand/Moreno.WoW/config"
	"github.com/MorenoLand/Moreno.WoW/network"
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/renderer"
	"github.com/g3n/engine/texture"
	"github.com/g3n/engine/window"
	"github.com/go-gl/glfw/v3.3/glfw"
	lua "github.com/yuin/gopher-lua"
)

type loginResult struct {
	session  *network.Session
	account  string
	password string
	err      error
}

type worldEntryResult struct {
	position    world.WorldPosition
	character   world.Character
	err         error
	spells      map[uint32]ui.SpellInfo
	skills      map[uint32]ui.SpellSkillLine
	spellErr    error
	mapData     *ui.MapCatalog
	mapErr      error
	itemLocal   map[uint32]ui.ItemLocal
	itemIcons   map[uint32]string
	itemClasses ui.ItemClassNames
	itemErr     error
}

type sceneLoadResult struct {
	requestKey, path, sceneKey, characterKey string
	model, characterModel, petModel          *core.Node
	fov, characterFacing                     float32
	loadMS                                   float64
	err                                      error
}

type clientHost struct {
	width         float64
	height        float64
	startLogin    func(string, string)
	enterWorld    func(int)
	sendChat      func(string, string, string, string) error
	castSpell     func(uint32, uint64) error
	cancelCast    func(uint32) error
	setSelection  func(uint64) error
	cancelAura    func(uint32) error
	cancelMount   func() error
	startAttack   func(uint64) error
	stopAttack    func() error
	cycleEnemy    func(bool) error
	cancelChannel func(uint32) error
	setAction     func(int, world.ActionButton) error
	session       func() *network.Session
	logout        func()
	quit          func()
	audio         *audioManager
	saveAudio     func(string, string)
	loginRunning  bool
}

func (h *clientHost) ScreenSize() (float64, float64) { return h.width, h.height }
func (h *clientHost) PlaySound(name string) {
	if h.audio != nil {
		h.audio.PlaySound(name)
	}
}
func (h *clientHost) PlayMusic(name string) {
	if h.audio != nil {
		h.audio.PlayMusic(name)
	}
}
func (h *clientHost) PlayAmbience(name string) {
	if h.audio != nil {
		h.audio.PlayAmbience(name)
	}
}
func (h *clientHost) StopMusic() {
	if h.audio != nil {
		h.audio.StopMusic()
	}
}
func (h *clientHost) StopAmbience() {
	if h.audio != nil {
		h.audio.StopAmbience()
	}
}
func (h *clientHost) PlayMovieAudio(data []byte, sampleRate, channels int, volume float64) {
	if h.audio != nil {
		h.audio.PlayMovieAudio(data, sampleRate, channels, volume)
	}
}
func (h *clientHost) StopMovieAudio() {
	if h.audio != nil {
		h.audio.StopMovieAudio()
	}
}
func (h *clientHost) StopAllSFX() {
	if h.audio != nil {
		h.audio.StopAllSFX()
	}
}
func (h *clientHost) SetAudioCVar(name, value string) {
	if h.audio != nil {
		h.audio.SetAudioCVar(name, value)
	}
	if h.saveAudio != nil {
		h.saveAudio(name, value)
	}
}
func (h *clientHost) LaunchURL(string) {}
func (h *clientHost) Quit(bool) {
	if h.quit != nil {
		h.quit()
	}
}
func (h *clientHost) ConsoleExec(string) {}
func (h *clientHost) Screenshot()        {}
func (h *clientHost) DefaultServerLogin(account, password string) {
	if h.startLogin != nil {
		h.startLogin(account, password)
	}
}
func (h *clientHost) EnterWorld(index int) {
	if h.enterWorld != nil {
		h.enterWorld(index)
	}
}
func (h *clientHost) SendChatMessage(message, chatType, language, target string) error {
	if h.sendChat == nil {
		return fmt.Errorf("world session is closed")
	}
	return h.sendChat(message, chatType, language, target)
}
func (h *clientHost) Logout() {
	if h.logout != nil {
		h.logout()
	}
}

func (h *clientHost) SetActionButton(slot int, action world.ActionButton) error {
	if h.setAction == nil {
		return fmt.Errorf("world session is closed")
	}
	return h.setAction(slot, action)
}

func (h *clientHost) CastSpell(id uint32, target uint64) error {
	if h.castSpell == nil {
		return fmt.Errorf("world session is closed")
	}
	return h.castSpell(id, target)
}
func (h *clientHost) SetSelection(guid uint64) error {
	if h.setSelection == nil {
		return fmt.Errorf("world session is closed")
	}
	return h.setSelection(guid)
}
func (h *clientHost) CancelAura(spellID uint32) error {
	if h.cancelAura == nil {
		return fmt.Errorf("world session is closed")
	}
	return h.cancelAura(spellID)
}
func (h *clientHost) CancelMount() error {
	if h.cancelMount == nil {
		return fmt.Errorf("world session is closed")
	}
	return h.cancelMount()
}
func (h *clientHost) StartAttack(guid uint64) error {
	if h.startAttack == nil {
		return fmt.Errorf("world session is closed")
	}
	return h.startAttack(guid)
}
func (h *clientHost) StopAttack() error {
	if h.stopAttack == nil {
		return fmt.Errorf("world session is closed")
	}
	return h.stopAttack()
}
func (h *clientHost) CycleEnemy(reverse bool) error {
	if h.cycleEnemy == nil {
		return fmt.Errorf("world session is closed")
	}
	return h.cycleEnemy(reverse)
}
func (h *clientHost) CancelChannel(id uint32) error {
	if h.cancelChannel == nil {
		return fmt.Errorf("world session is closed")
	}
	return h.cancelChannel(id)
}

func (h *clientHost) CancelCast(id uint32) error {
	if h.cancelCast == nil {
		return fmt.Errorf("world session is closed")
	}
	return h.cancelCast(id)
}

func Run(clientConfig network.Config, dataPath, interfacePath, backgroundPath, lastCharacter, configPath string, savedOptions config.Options, debug, rememberMe bool) {
	startWidth, startHeight := savedOptions.WindowSize()
	if err := window.Init(startWidth, startHeight, "MorenoWoW"); err != nil {
		log.Printf("window: %v", err)
		return
	}
	win, ok := window.Get().(*window.GlfwWindow)
	if !ok {
		log.Print("window: unsupported window implementation")
		return
	}
	defer win.Destroy()
	installAppIcon(win)
	// Unlock the swap interval so menu/world FPS is not capped to display refresh.
	win.SetSwapInterval(0)
	gl := win.Gls()
	gpuVendor := gl.GetString(gls.VENDOR)
	gpuRenderer := gl.GetString(gls.RENDERER)
	gpuVersion := gl.GetString(gls.VERSION)
	r := renderer.NewRenderer(gl)
	if err := r.AddDefaultShaders(); err != nil {
		log.Printf("renderer: %v", err)
		return
	}
	installM2Shaders(r)
	scene := core.NewNode()
	gui.Manager().Set(scene)
	cam := camera.New(1)
	resetSceneCamera(cam)
	scene.Add(cam)
	host := &clientHost{width: float64(startWidth), height: float64(startHeight)}
	results := make(chan loginResult, 1)
	worldResults := make(chan worldEntryResult, 1)
	sceneResults := make(chan sceneLoadResult, 1)
	var uiEngine *ui.UIEngine
	var activeSession *network.Session
	var worldPackets <-chan world.PacketEvent
	worldLoading := false
	host.cancelChannel = func(id uint32) error {
		if activeSession == nil {
			return fmt.Errorf("world session is closed")
		}
		return activeSession.CancelChannel(id)
	}
	host.session = func() *network.Session { return activeSession }
	host.setAction = func(slot int, action world.ActionButton) error {
		if activeSession == nil {
			return fmt.Errorf("world session is closed")
		}
		return activeSession.SetActionButton(slot, action)
	}
	host.castSpell = func(id uint32, target uint64) error {
		if activeSession == nil {
			return fmt.Errorf("world session is closed")
		}
		return activeSession.CastSpell(id, target)
	}
	host.setSelection = func(guid uint64) error {
		if activeSession == nil {
			return fmt.Errorf("world session is closed")
		}
		return activeSession.SetSelection(guid)
	}
	host.cancelCast = func(id uint32) error {
		if activeSession == nil {
			return fmt.Errorf("world session is closed")
		}
		return activeSession.CancelCast(id)
	}
	host.sendChat = func(message, chatType, language, target string) error {
		if activeSession == nil {
			return fmt.Errorf("world session is closed")
		}
		return activeSession.SendChatMessage(message, chatType, language, target)
	}
	host.enterWorld = func(index int) {
		if activeSession == nil || worldLoading {
			return
		}
		worldLoading = true
		session := activeSession
		if uiEngine != nil {
			uiEngine.SetWorldLoading(true)
		}
		character := world.Character{}
		if index >= 0 && index < len(session.Characters) {
			character = session.Characters[index]
		}
		if uiEngine != nil {
			mapID := character.Map
			uiEngine.SetLoadingScreen(worldLoadingScreenPath(uiEngine.AssetLoader, mapID), 0)
		}
		loader := uiEngine.AssetLoader
		go func() {
			position, err := session.EnterWorld(index)
			result := worldEntryResult{position: position, character: character, err: err}
			if err == nil {
				result.spells, result.skills, result.spellErr = readWorldSpellCatalog(loader)
				result.mapData, result.mapErr = readWorldMapCatalog(loader)
				result.itemLocal, result.itemIcons, result.itemClasses, result.itemErr = readWorldItemCatalog(loader)
			}
			worldResults <- result
		}()
	}
	host.startLogin = func(account, password string) {
		if host.loginRunning {
			return
		}
		host.loginRunning = true
		if uiEngine != nil {
			uiEngine.SetStatusKey("GAME_SERVER_LOGIN")
		}
		cfg := clientConfig
		cfg.Account = account
		cfg.Password = password
		go func() {
			session, err := network.Login(cfg)
			results <- loginResult{session: session, account: account, password: password, err: err}
		}()
	}
	host.quit = func() { win.SetShouldClose(true) }

	var uiImage *gui.Image
	var uiTex *texture.Texture2D
	glueLayers := newGlueLayerStack(scene)
	var uiMapImage *gui.Image
	var uiMapTex *texture.Texture2D
	var uiChatImage *gui.Image
	var uiChatTex *texture.Texture2D
	var uiCursorImage *gui.Image
	var uiCursorTex *texture.Texture2D
	uiCursorKey := ""
	var playerEquipment *worldEquipment
	var playerLight worldLightState
	playerModelPath := ""
	var eng *ui.UIEngine
	var err error
	lastUIRefresh := time.Time{}
	var sceneModel *core.Node
	var sceneCharacterModel *core.Node
	var scenePetModel *core.Node
	sceneCharacterFacing := float32(0)
	sceneModelPath := ""
	sceneModelKey := ""
	sceneRequestKey := ""
	sceneLoadPending := false
	// The login script fades in from black ("to hide the setting up frame")
	// over a second of script time. That time must not run while the 3D scene
	// is still loading or has not been drawn yet, or the fade ends on a black
	// frame and the scene pops in afterwards. introFramesDrawn counts frames the
	// first scene has been on screen; the gate opens after a few.
	introGate := true
	introFramesDrawn := 0
	introStarted := time.Now()
	var sceneLoadMu sync.Mutex
	sceneCameraDiagonalFOV := float32(0)
	sceneCameraFrameFraction := float32(1)
	debugModelLoadMS := float64(0)
	debugUIRenderMS := float64(0)
	worldAnimFrame := uint32(0)
	lastInstanceCheck := time.Time{}
	debugUIRefreshes, debugUIRefreshCount := 0.0, 0
	debugUpdateMS, debugSceneMS, debugSwapMS := 0.0, 0.0, 0.0
	debugRateStarted := time.Now()
	debugModelError := ""
	worldMode := false
	var worldCamera *worldCameraController
	var worldLiquid *worldLiquidAnimator
	var worldStreamer *worldTerrainStreamer
	worldLiquidStart := time.Now()
	var worldLight *worldLightTables
	var worldClock worldGameClock
	var worldMovement *worldMovementReporter
	var worldMoveNet *worldMovementNet
	var worldPlayer *core.Node
	var playerMount *worldMountRig
	var worldModel *core.Node
	var worldSky *core.Node
	worldEntities := make(map[uint64]*worldEntity)
	worldAuras := worldAuraState{slots: make(map[uint64][]world.AuraSlot)}

	worldCreatureCache := worldCreatureTables{}
	var worldFloor func(float32, float32, float32) (float32, bool)
	var worldCharacter world.Character
	worldNameState := newWorldNames()
	worldPlates := newWorldNameplates()
	var worldFactionState *worldFactions
	worldSend := func(opcode world.ClientOpcode, body []byte) error {
		if activeSession == nil {
			return fmt.Errorf("world session is closed")
		}
		return activeSession.SendRaw(opcode, body)
	}
	var selectTarget func(guid uint64) error
	host.startAttack = func(guid uint64) error {
		// The client refuses to attack from a mount before asking the server.
		if player := worldEntities[worldCharacter.GUID]; player != nil && player.fields[world.UnitMountDisplayIDField] != 0 {
			uiEngine.Rt.ShowGlobalError("ERR_ATTACK_MOUNTED")
			return nil
		}
		return worldSend(world.AttackSwingOpcode, world.BuildAttackSwing(guid))
	}
	host.cancelMount = func() error { return worldSend(world.CancelMountAuraOpcode, nil) }
	host.cancelAura = func(spellID uint32) error {
		body := []byte{byte(spellID), byte(spellID >> 8), byte(spellID >> 16), byte(spellID >> 24)}
		return worldSend(world.CancelAuraOpcode, body)
	}
	host.stopAttack = func() error { return worldSend(world.AttackStopOpcode, nil) }
	host.cycleEnemy = func(reverse bool) error {
		player := worldEntities[worldCharacter.GUID]
		if player == nil || worldCamera == nil || activeSession == nil {
			return nil
		}
		targets := worldTabTargets(worldEntities, player, worldFactionState, worldCamera.position, 41)
		if len(targets) == 0 {
			return nil
		}
		current := uiEngine.Rt.UnitGUID("target")
		next := 0
		for index, guid := range targets {
			if guid == current {
				next = index + 1
				if reverse {
					next = index - 1 + len(targets)
				}
				next %= len(targets)
				break
			}
		}
		return selectTarget(targets[next])
	}
	worldCombatState := &worldCombat{names: worldNameState, send: worldSend}
	// selectTarget tells the server and shows the selection at once; the
	// server's own UNIT_FIELD_TARGET update then confirms it.
	selectTarget = func(guid uint64) error {
		if activeSession == nil {
			return fmt.Errorf("world session is closed")
		}
		if player := worldEntities[worldCharacter.GUID]; player != nil {
			player.fields[world.UnitTargetField] = uint32(guid)
			player.fields[world.UnitTargetField+1] = uint32(guid >> 32)
			syncWorldTarget(uiEngine.Rt, worldEntities, player, worldNameState, worldFactionState, worldSend)
		}
		return activeSession.SetSelection(guid)
	}
	host.setSelection = selectTarget
	// selectTarget tells the server and shows the selection at once; the	// server's own UNIT_FIELD_TARGET update then confirms it.	selectTarget := func(guid uint64) error {		if activeSession == nil {			return fmt.Errorf("world session is closed")		}		if player := worldEntities[worldCharacter.GUID]; player != nil {			player.fields[world.UnitTargetField] = uint32(guid)			player.fields[world.UnitTargetField+1] = uint32(guid >> 32)			syncWorldTarget(uiEngine.Rt, worldEntities, player, worldNameState, worldFactionState, worldSend)		}		return activeSession.SetSelection(guid)	}	host.setSelection = selectTarget
	var worldPress worldMousePress
	worldClick := func(button window.MouseButton, x, y float64) {
		if activeSession == nil || worldCamera == nil {
			return
		}
		width, height := win.GetSize()
		guid := worldPickEntity(cam, worldEntities, worldCharacter.GUID, x, y, float64(width), float64(height))
		if guid == 0 {
			return
		}
		_ = selectTarget(guid)
		entity, player := worldEntities[guid], worldEntities[worldCharacter.GUID]
		if button != window.MouseButtonRight || entity == nil || player == nil {
			return
		}
		// A right click on something that can be attacked starts the attack.
		if hostile, _ := worldFactionState.reaction(player.fields[unitFactionField], entity.fields[unitFactionField]); hostile && entity.fields[unitHealthField] > 0 {
			_ = host.startAttack(guid)
		}
	}
	worldMapName := ""
	var worldMapID uint32
	var glueCharacters []world.Character
	var setSceneModel func() bool
	if dataPath != "" {
		// The scripts see the window's size while they load (GetScreenWidth /
		// GlueParent:GetSize()), so load at the size the window really has.
		loadWidth, loadHeight := win.GetSize()
		eng, err = ui.LoadUIEngineFromMPQSized(dataPath, clientConfig.Locale, backgroundPath, float64(loadWidth), float64(loadHeight))
	} else if root := resolveInterfaceRoot(dataPath, interfacePath); root != "" {
		eng, err = ui.LoadUIEngine(filepath.Join(root, "GlueXML"), filepath.Join(root, "FrameXML"), filepath.Join(root, "Interface-tree"), backgroundPath)
	}
	if err != nil {
		log.Printf("ui render error: %v", err)
	} else if eng != nil {
		uiEngine = eng
		uiEngine.UserAddOns = savedOptions.Addons
		defer uiEngine.Close()
		uiEngine.Rt.Host = host
		if audio, audioErr := newAudioManager(uiEngine.AssetLoader, debug); audioErr != nil {
			if debug {
				log.Printf("audio: %v", audioErr)
			}
		} else {
			host.audio = audio
			defer audio.Close()
		}
		host.saveAudio = func(name, value string) {
			if !savedOptions.Audio.SetCVar(name, value) {
				return
			}
			if configPath != "" {
				if saveErr := config.Save(configPath, savedOptions); saveErr != nil && debug {
					log.Printf("saving audio settings: %v", saveErr)
				}
			}
		}
		for name, value := range savedOptions.Audio.CVars() {
			uiEngine.Rt.SetCVar(name, value)
			if host.audio != nil {
				host.audio.SetAudioCVar(name, value)
			}
		}
		uiEngine.SetInitialCredentials(clientConfig.Account, clientConfig.Password, rememberMe)
		if lastCharacter != "" {
			uiEngine.Rt.SetCVar("lastCharacter", lastCharacter)
		}
		setSceneModel = func() bool {
			if worldMode {
				return false
			}
			path := uiEngine.CurrentModelPath()
			sceneModels := append([]ui.GlueSceneModel(nil), uiEngine.VisibleGlueSceneModels()...)
			sceneKey := ui.GlueSceneStructureKey(sceneModels)
			characterKey := ""
			var selectedCharacter world.Character
			selectedIndex := uiEngine.SelectedCharacterIndex()
			if createState, ok := uiEngine.CreatePreviewState(); ok {
				selectedCharacter = world.Character{Race: createState.RaceID, Class: createState.ClassID, Gender: createState.Gender}
				characterKey = fmt.Sprintf("create:%d:%d:%d", createState.RaceID, createState.Gender, createState.ClassID)
			} else if uiEngine.CharacterSelectVisible() && selectedIndex >= 0 && selectedIndex < len(glueCharacters) {
				selectedCharacter = glueCharacters[selectedIndex]
				characterKey = fmt.Sprintf("%d:%s:%d", selectedCharacter.GUID, worldCharacterModelPath(selectedCharacter), selectedCharacter.PetDisplayID)
			}
			requestKey := fmt.Sprintf("%s\x00%s\x00%s", path, sceneKey, characterKey)
			if requestKey == sceneRequestKey && (sceneModel != nil || sceneLoadPending || path == "" && len(sceneModels) == 0) {
				if sceneModel != nil && !sceneLoadPending {
					// Same structure: follow the script's per-frame changes in place.
					if fraction, changed := applyGlueSceneState(sceneModel, sceneModels, cam.Aspect()); changed && fraction != sceneCameraFrameFraction {
						sceneCameraFrameFraction = fraction
						setSceneCameraFOV(cam, sceneCameraDiagonalFOV, fraction)
					}
				}
				return false
			}
			if debug {
				log.Printf("scene request path=%s selected=%d key=%s previous=%s/%s", path, selectedIndex, characterKey, sceneModelPath, sceneModelKey)
			}
			if sceneModel != nil {
				scene.Remove(sceneModel)
				sceneModel.Dispose()
				sceneModel = nil
			}
			if sceneCharacterModel != nil {
				scene.Remove(sceneCharacterModel)
				sceneCharacterModel.Dispose()
				sceneCharacterModel = nil
			}
			if scenePetModel != nil {
				scene.Remove(scenePetModel)
				scenePetModel.Dispose()
				scenePetModel = nil
			}
			resetSceneCamera(cam)
			sceneCameraDiagonalFOV = 0
			sceneCameraFrameFraction = 1
			sceneModelPath, sceneModelKey = path, sceneKey
			sceneRequestKey = requestKey
			debugModelError = ""
			debugModelLoadMS = 0
			sceneLoadPending = false
			uiEngine.SetSceneBackground(true)
			if path == "" && len(sceneModels) == 0 {
				return true
			}
			sceneLoadPending = true
			requestedCharacter, requestedFacing, requestedAspect := selectedCharacter, uiEngine.SceneCharacterFacing(), cam.Aspect()
			go func() {
				sceneLoadMu.Lock()
				loaded := loadGlueSceneRequest(uiEngine.AssetLoader, &worldCreatureCache, sceneModels, path, sceneKey, characterKey, requestedCharacter, requestedFacing, requestedAspect, debug)
				sceneLoadMu.Unlock()
				loaded.requestKey = requestKey
				sceneResults <- loaded
			}()
			return true
		}
		// The login script builds its scene over its first few updates, and the
		// first full UI render below blocks for most of a second. Running those
		// ticks now (script time barely moves) lets the scene's models load
		// alongside that render instead of after it.
		for tick := 0; tick < 4; tick++ {
			eng.Update(0.001)
		}
		setSceneModel()
		initialUI := eng.Render(960, 640)
		uiMapTex = texture.NewTexture2DFromRGBA(image.NewRGBA(image.Rect(0, 0, 1, 1)))
		uiMapImage = gui.NewImageFromTex(uiMapTex)
		uiMapImage.SetZLayerDelta(-1)
		uiMapImage.SetPosition(0, 0)
		uiMapImage.SetVisible(false)
		scene.Add(uiMapImage)
		uiChatTex = texture.NewTexture2DFromRGBA(image.NewRGBA(image.Rect(0, 0, 1, 1)))
		uiChatImage = gui.NewImageFromTex(uiChatTex)
		uiChatImage.SetZLayerDelta(1)
		uiChatImage.SetPosition(0, 0)
		uiChatImage.SetVisible(false)
		scene.Add(uiChatImage)
		uiCursorTex = texture.NewTexture2DFromRGBA(image.NewRGBA(image.Rect(0, 0, 1, 1)))
		uiCursorImage = gui.NewImageFromTex(uiCursorTex)
		uiCursorImage.SetZLayerDelta(2)
		uiCursorImage.SetVisible(false)
		scene.Add(uiCursorImage)
		uiTex = texture.NewTexture2DFromRGBA(initialUI)
		uiImage = gui.NewImageFromTex(uiTex)
		uiImage.SetColor4(&math32.Color4{R: 1, G: 1, B: 1, A: 0})
		uiImage.SetPosition(0, 0)
		scene.Add(uiImage)
	}
	var wowCursor *glfw.Cursor
	if uiEngine != nil {
		if wowCursor, err = installCursor(win, uiEngine.AssetLoader); err != nil {
			if debug {
				log.Printf("cursor: %v", err)
			}
			wowCursor = nil
		}
	}
	if wowCursor != nil {
		defer wowCursor.Destroy()
	}
	debugFPS := float64(0)
	debugFrameMS := float64(0)
	debugPanelRefresh := time.Time{}
	lastMinimapVersion := uint64(0)
	updateDebugPanel := func() {
		width, height := win.GetSize()
		parts := 0
		if sceneModel != nil {
			parts = len(sceneModel.Children())
		}
		connection := "idle"
		if host.loginRunning {
			connection = "connecting"
		} else if activeSession != nil {
			connection = "connected"
		}
		modelStats := glueModelStats{}
		if sceneModel != nil {
			if info, ok := sceneModel.UserData().(glueModelInfo); ok {
				modelStats = info.stats
			}
		}
		worldParts := 0
		if worldModel != nil {
			worldParts = len(worldModel.Children())
		}
		renderStats := r.Stats()
		if span := time.Since(debugRateStarted).Seconds(); span > 0 {
			debugUIRefreshes = float64(debugUIRefreshCount) / span
		}
		debugUIRefreshCount, debugRateStarted = 0, time.Now()
		assetStats := uiEngine.AssetLoader.AssetStats()
		uiEngine.SetDebugPanelLines(debugPanelLines(debugPanelData{
			width: width, height: height, fps: debugFPS, frameMS: debugFrameMS, uiRenderMS: debugUIRenderMS,
			modelLoadMS: debugModelLoadMS, gpuVendor: gpuVendor, gpuRenderer: gpuRenderer, gpuVersion: gpuVersion,
			dataPath: dataPath, scenePath: sceneModelPath, connection: connection, authAddress: clientConfig.AuthAddress,
			model: modelStats, sceneParts: parts, assetCache: len(uiEngine.Cache), mpqArchives: assetStats.Archives,
			mpqCachedFiles: assetStats.CachedFiles, mpqMissingFiles: assetStats.MissingFiles, audio: host.audio != nil,
			cursor: wowCursor != nil, modelError: debugModelError, terminalDebug: debug,
			uiRefreshes: debugUIRefreshes, updateMS: debugUpdateMS, sceneMS: debugSceneMS, swapMS: debugSwapMS, rendererMats: renderStats.GraphicMats, rendererPanels: renderStats.Panels, rendererOthers: renderStats.Others, worldParts: worldParts,
		}))
	}

	refreshMinimap := func(width, height int) {
		if !worldMode || uiMapImage == nil || uiEngine == nil {
			return
		}
		version := uiEngine.WorldMinimapVersion()
		if version == lastMinimapVersion && uiMapTex != nil {
			return
		}
		mapFrame, mapRegion := uiEngine.RenderWorldMinimapRegion(width, height)
		if mapFrame == nil {
			return
		}
		if uiMapTex == nil {
			uiMapTex = texture.NewTexture2DFromRGBA(mapFrame)
			uiMapImage.SetTexture(uiMapTex)
		} else {
			uiMapTex.SetFromRGBA(mapFrame)
		}
		uiMapImage.SetPosition(float32(mapRegion.Min.X), float32(mapRegion.Min.Y))
		uiMapImage.SetSize(float32(mapRegion.Dx()), float32(mapRegion.Dy()))
		uiMapImage.SetVisible(true)
		lastMinimapVersion = version
	}
	// The picked up item or spell follows the pointer as its own small image, so
	// moving it does not repaint the interface.
	updateCursorIcon := func() {
		if uiCursorImage == nil || uiEngine == nil || !worldMode {
			return
		}
		size := int(32*uiEngine.UIScale() + 0.5)
		key, icon := uiEngine.CursorIcon(size)
		key += fmt.Sprint(size)
		if icon == nil {
			uiCursorImage.SetVisible(false)
			return
		}
		if key != uiCursorKey {
			uiCursorTex.SetFromRGBA(icon)
			uiCursorKey = key
			uiCursorImage.SetSize(float32(size), float32(size))
		}
		x, y := uiEngine.CursorPosition()
		uiCursorImage.SetPosition(float32(x)-float32(size)/2, float32(y)-float32(size)/2)
		uiCursorImage.SetVisible(true)
	}
	refreshChat := func(width, height int) {
		if !worldMode || uiChatImage == nil || uiEngine == nil {
			return
		}
		if !uiEngine.WorldChatFocused() {
			// Only the edit box being typed in is drawn here; no full-screen canvas
			// is built or uploaded while it is idle.
			uiChatImage.SetVisible(false)
			return
		}
		chatFrame := uiEngine.RenderWorldChat(width, height)
		if uiChatTex == nil {
			uiChatTex = texture.NewTexture2DFromRGBA(chatFrame)
			uiChatImage.SetTexture(uiChatTex)
		} else {
			uiChatTex.SetFromRGBA(chatFrame)
		}
		uiChatImage.SetSize(float32(width), float32(height))
		uiChatImage.SetVisible(uiEngine.WorldChatFocused())
	}
	refreshFrame := func(panelOnly bool) {
		if uiImage == nil || uiEngine == nil {
			return
		}
		width, height := win.GetSize()
		host.width = float64(width)
		host.height = float64(height)
		if width < 1 || height < 1 {
			return
		}
		if setSceneModel != nil && !worldMode {
			setSceneModel()
		}
		uiStarted := time.Now()
		var uiFrame *image.RGBA
		layeredGlue := false
		if worldMode {
			glueLayers.hide()
			uiImage.SetVisible(true)
			if panelOnly {
				uiFrame = uiEngine.RenderWorldDebugPanel(width, height)
			} else {
				uiFrame = uiEngine.RenderWorld(width, height)
			}
		} else {
			back, planes := uiEngine.RenderLayers(width, height)
			if back == nil && len(planes) == 1 && planes[0].Blend == ui.GlueBlendNormal {
				uiFrame = planes[0].Image
				glueLayers.hide()
				uiImage.SetVisible(true)
			} else {
				// The UI stacking order puts something behind the scene models
				// (e.g. the login scene's background) or blends over them.
				layeredGlue = true
				glueLayers.show(back, planes, width, height)
				uiImage.SetVisible(false)
			}
			if uiMapImage != nil {
				uiMapImage.SetVisible(false)
			}
			if uiChatImage != nil {
				uiChatImage.SetVisible(false)
			}
		}
		// Reuse one GPU texture and upload pixels in place. Recreating a
		// Texture2D every UI paint forced a full delete/alloc + material rebind.
		if !layeredGlue {
			if uiTex == nil {
				uiTex = texture.NewTexture2DFromRGBA(uiFrame)
				uiImage.SetTexture(uiTex)
			} else {
				uiTex.SetFromRGBA(uiFrame)
			}
		}
		debugUIRenderMS = time.Since(uiStarted).Seconds() * 1000
		debugUIRefreshCount++
		uiImage.SetSize(float32(width), float32(height))
		refreshMinimap(width, height)
		refreshChat(width, height)
		lastUIRefresh = time.Now()
	}
	refresh := func() { refreshFrame(false) }
	refreshDebugPanel := func() { refreshFrame(true) }
	refreshMinimapOnly := func() {
		if !worldMode || uiEngine == nil || uiMapImage == nil {
			return
		}
		width, height := win.GetSize()
		if width > 0 && height > 0 {
			refreshMinimap(width, height)
		}
	}
	refreshChatOnly := func() {
		if !worldMode || uiEngine == nil || uiChatImage == nil {
			return
		}
		width, height := win.GetSize()
		if width > 0 && height > 0 {
			refreshChat(width, height)
		}
	}

	host.logout = func() {
		if !worldMode {
			return
		}
		worldPlates.hide()
		worldMode = false
		worldCamera = nil
		worldMovement = nil
		worldMoveNet = nil
		worldPackets = nil
		worldLoading = false
		if uiEngine != nil {
			uiEngine.SetWorldLoading(false)
			uiEngine.ClearLoadingScreen()
			uiEngine.SetGlueState(uiEngine.Rt.Glue)
		}
		if uiMapImage != nil {
			uiMapImage.SetVisible(false)
		}
		if uiChatImage != nil {
			uiChatImage.SetVisible(false)
		}
		if worldModel != nil {
			scene.Remove(worldModel)
			worldModel.Dispose()
			worldModel = nil
		}
		if worldSky != nil {
			scene.Remove(worldSky)
			worldSky.Dispose()
			worldSky = nil
		}
		if worldStreamer != nil {
			worldStreamer.stop()
			worldStreamer = nil
		}
		worldAtmo = worldAtmosphere{}
		if worldPlayer != nil {
			scene.Remove(worldPlayer)
			worldPlayer.Dispose()
			worldPlayer = nil
			if playerMount != nil {
				playerMount.dispose(scene)
				playerMount = nil
			}
		}
		for guid, entity := range worldEntities {
			if entity != nil && entity.node != nil {
				scene.Remove(entity.node)
				entity.node.Dispose()
			}
			delete(worldEntities, guid)
		}
		gl.ClearColor(0, 0, 0, 1) // the glue frame starts from opaque black (Wow.exe FUN_0040a270)
		refresh()
	}

	onResize := func(string, interface{}) {
		width, height := win.GetSize()
		gl.Viewport(0, 0, int32(width), int32(height))
		if height > 0 {
			cam.SetAspect(float32(width) / float32(height))
			if worldMode {
				cam.SetFov(70)
				cam.SetNear(0.2)
				cam.SetFar(worldFarClip(uiEngine.Rt))
			} else {
				setSceneCameraFOV(cam, sceneCameraDiagonalFOV, sceneCameraFrameFraction)
				applyGlueSceneAspect(sceneModel, cam.Aspect())
			}
		}
		refresh()
	}
	win.Subscribe(window.OnWindowSize, onResize)
	if uiEngine != nil {
		win.Subscribe(window.OnCursor, func(_ string, event interface{}) {
			cursor := event.(*window.CursorEvent)
			if worldMode && worldCamera != nil && worldCamera.handleCursor(float64(cursor.Xpos), float64(cursor.Ypos)) {
				return
			}
			if uiEngine.HandleCursor(float64(cursor.Xpos), float64(cursor.Ypos)) {
				if !uiEngine.DebugPanelDragging() {
					refresh()
				}
			}
			updateCursorIcon()
		})
		win.Subscribe(window.OnMouseDown, func(_ string, event interface{}) {
			mouse := event.(*window.MouseEvent)
			if worldMode && worldCamera != nil && !uiEngine.PointOverUI(float64(mouse.Xpos), float64(mouse.Ypos)) && worldCamera.handleMouse(float64(mouse.Xpos), float64(mouse.Ypos), mouse.Button, true) {
				worldPress = worldMousePress{x: float64(mouse.Xpos), y: float64(mouse.Ypos), button: mouse.Button, active: true}
				return
			}
			if uiEngine.HandleMouse(float64(mouse.Xpos), float64(mouse.Ypos), mouse.Button, true) {
				refresh()
			}
			updateCursorIcon()
		})
		win.Subscribe(window.OnMouseUp, func(_ string, event interface{}) {
			mouse := event.(*window.MouseEvent)
			if worldMode && worldCamera != nil && worldCamera.dragging && worldCamera.handleMouse(float64(mouse.Xpos), float64(mouse.Ypos), mouse.Button, false) {
				if worldPress.active && worldPress.button == mouse.Button {
					worldPress.active = false
					if math.Hypot(float64(mouse.Xpos)-worldPress.x, float64(mouse.Ypos)-worldPress.y) < worldClickSlop {
						worldClick(mouse.Button, float64(mouse.Xpos), float64(mouse.Ypos))
					}
				}
				return
			}
			if uiEngine.HandleMouse(float64(mouse.Xpos), float64(mouse.Ypos), mouse.Button, false) {
				refresh()
			}
			updateCursorIcon()
		})
		win.Subscribe(window.OnScroll, func(_ string, event interface{}) {
			scroll := event.(*window.ScrollEvent)
			if worldMode && worldCamera != nil && worldCamera.handleScroll(scroll.Yoffset) {
				return
			}
			if uiEngine.HandleScroll(float64(scroll.Yoffset)) {
				refresh()
			}
		})
		win.Subscribe(window.OnChar, func(_ string, event interface{}) {
			char := event.(*window.CharEvent)
			if uiEngine.HandleChar(char.Char) {
				if worldMode && uiEngine.WorldChatFocused() {
					refreshChatOnly()
				} else {
					refresh()
				}
			}
		})
		win.Subscribe(window.OnKeyDown, func(_ string, event interface{}) {
			key := event.(*window.KeyEvent)
			if worldMode && worldCamera != nil {
				if command := uiEngine.MovementCommandDown(key.Key, key.Mods); command != "" && worldCamera.handleCommand(command, true) {
					return
				}
			}
			if key.Key == window.KeyF2 {
				uiEngine.HandleKeyWithMods(key.Key, key.Mods)
				updateDebugPanel()
				debugPanelRefresh = time.Now()
				refresh()
				return
			}
			if uiEngine.HandleKeyWithMods(key.Key, key.Mods) {
				if worldMode && uiEngine.WorldChatFocused() {
					refreshChatOnly()
				} else {
					refresh()
				}
			}
		})
		win.Subscribe(window.OnKeyUp, func(_ string, event interface{}) {
			key := event.(*window.KeyEvent)
			if worldMode && worldCamera != nil {
				if command := uiEngine.MovementCommandUp(key.Key); command != "" {
					worldCamera.handleCommand(command, false)
					return
				}
			}
			if uiEngine.HandleKeyUp(key.Key) {
				refresh()
			}
		})
	}
	onResize("", nil)
	gl.ClearColor(0, 0, 0, 1) // the glue frame starts from opaque black (Wow.exe FUN_0040a270)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	lastUpdate := time.Now()
	lastFrame := lastUpdate
	for !win.ShouldClose() {
		frameAt := time.Now()
		if elapsed := frameAt.Sub(lastFrame).Seconds(); elapsed > 0 {
			debugFrameMS = elapsed * 1000
			if debugFPS == 0 {
				debugFPS = 1 / elapsed
			} else {
				debugFPS = debugFPS*.9 + (1/elapsed)*.1
			}
		}
		lastFrame = frameAt
		select {
		case <-signals:
			win.SetShouldClose(true)
		default:
		}
		if uiEngine != nil {
			now := time.Now()
			elapsed := now.Sub(lastUpdate).Seconds()
			if sceneModel != nil {
				for _, soundID := range updateGlueSceneNode(sceneModel, elapsed) {
					if debug {
						log.Printf("scene sound event id=%d", soundID)
					}
					if host.audio != nil {
						host.audio.PlaySoundID(soundID)
					}
				}
			}
			if sceneCharacterModel != nil {
				if info, ok := sceneCharacterModel.UserData().(glueModelInfo); ok && info.animation != nil {
					for _, soundID := range info.animation.Update(elapsed) {
						if debug {
							log.Printf("character sound event id=%d", soundID)
						}
						if host.audio != nil {
							host.audio.PlaySoundID(soundID)
						}
					}
				}
			}
			minimapChanged := false
			if worldMode && worldCamera != nil {
				worldCamera.update(elapsed, cam, worldPlayer)
				worldLiquid.update(time.Since(worldLiquidStart).Seconds())
				worldStreamer.update(worldCamera.position[0], worldCamera.position[1])
				if worldLight != nil {
					worldAtmo = worldLight.sample(worldMapID, worldCamera.position[0], worldCamera.position[1], worldClock.minuteOfDay(time.Now()))
				}
				worldMovement.update(time.Now(), worldCamera)
				if playerMount != nil && worldPlayer != nil {
					rider := playerMount.place(worldCamera.position, worldCamera.bodyYaw)
					worldPlayer.SetPosition(rider[0], rider[1], rider[2])
					playerMount.update(elapsed, worldMountMotion(worldCamera.flying, worldCamera.isMoving() || worldCamera.velocity[2]*worldCamera.velocity[2] > 0.04, true), func(id uint32) {
						if host.audio != nil {
							host.audio.PlaySoundID(id)
						}
					})
				}
				minimapChanged = uiEngine.SetWorldMinimapPosition(worldMapName, worldCamera.position[0], worldCamera.position[1])
				uiEngine.Rt.SetPlayerMapPosition(worldMapID, worldCamera.position[0], worldCamera.position[1])
				uiEngine.Rt.SetPlayerMapFacing(worldCamera.bodyYaw)
				worldLightChecksLeft, worldSkinBudget = 6, 24
				advanceWorldEntities(worldEntities, elapsed, worldFloor)
				if worldPlayer != nil {
					if info, ok := worldPlayer.UserData().(glueModelInfo); ok {
						if info.animation != nil {
							info.animation.SetMotion(worldCamera.motion())
							for _, soundID := range info.animation.Update(elapsed) {
								if debug {
									log.Printf("world player sound event id=%d", soundID)
								}
								if host.audio != nil {
									host.audio.PlaySoundID(soundID)
								}
							}
						}
						if info.particles != nil {
							info.particles.Update(elapsed)
						}
					}
					if own := worldEntities[worldCharacter.GUID]; own != nil {
						syncWorldEquipment(uiEngine.AssetLoader, worldPlayer, &playerEquipment, playerModelPath, own, worldCreatureExtra{})
					}
					poseWorldEquipment(worldPlayer, playerEquipment)
					updateWorldLocalLight(worldPlayer, &playerLight, worldStreamer.collision, frameAt)
					if frameAt.Sub(lastInstanceCheck) > 100*time.Millisecond {
						lastInstanceCheck = frameAt
						position := worldPlayer.Position()
						worldInsideInstance = worldStreamer.collision.instanceAt(position.X, position.Y, position.Z)
					}
				}
				worldAnimFrame++
				cam.UpdateMatrixWorld()
				var viewMatrix, projMatrix, viewProjection math32.Matrix4
				cam.ViewMatrix(&viewMatrix)
				cam.ProjMatrix(&projMatrix)
				viewProjection.MultiplyMatrices(&projMatrix, &viewMatrix)
				frustum := math32.NewFrustumFromMatrix(&viewProjection)
				cameraAt := cam.Position()
				for _, entity := range worldEntities {
					if entity.node == nil {
						continue
					}
					if worldCamera != nil && entity.hasPosition {
						dx := entity.movement.Position.X - worldCamera.position[0]
						dy := entity.movement.Position.Y - worldCamera.position[1]
						visible := dx*dx+dy*dy <= worldObjectDistance*worldObjectDistance
						entity.node.SetVisible(visible)
						if !visible {
							continue
						}
					}
					if entity.mount != nil {
						position := [3]float32{entity.movement.Position.X, entity.movement.Position.Y, entity.movement.Position.Z}
						rider := entity.mount.place(position, entity.movement.Position.Orientation)
						entity.node.SetPosition(rider[0], rider[1], rider[2])
						flags := entity.movement.MovementFlags
						entity.mount.update(elapsed, worldMountMotion(flags&world.MovementFlagFlying != 0, worldEntityMoving(entity), true), func(id uint32) {
							if host.audio != nil {
								host.audio.PlaySoundID(id)
							}
						})
					}
					if info, ok := entity.node.UserData().(glueModelInfo); ok {
						if info.animation != nil {
							motion := uint16(0)
							if worldEntityMoving(entity) {
								motion = worldEntityMotion(entity)
							}
							if entity.mount != nil {
								motion = worldAnimMount
							}
							info.animation.SetMotion(motion)
							skipped, idle := worldEntitySkinSkipped(entity.node, info, frustum, cameraAt, worldAnimFrame, entity.guid)
							info.animation.skipSkin = skipped
							// What nobody can see or make out moves a few frames at a time.
							step := elapsed + entity.animPending
							entity.animPending = 0
							if idle && (worldAnimFrame+uint32(entity.guid))%4 != 0 {
								entity.animPending = step
								step = 0
							}
							for _, soundID := range info.animation.Update(step) {
								if host.audio != nil {
									host.audio.PlaySoundID(soundID)
								}
							}
						}
						if info.particles != nil {
							info.particles.Update(elapsed)
						}
					}
					poseWorldEquipment(entity.node, entity.equipment)
					updateWorldLocalLight(entity.node, &entity.light, worldStreamer.collision, frameAt)
				}
				if screenWidth, screenHeight := win.GetSize(); screenWidth > 0 && screenHeight > 0 && worldFactionState != nil {
					playerFaction := uint32(0)
					if own := worldEntities[worldCharacter.GUID]; own != nil {
						playerFaction = own.fields[unitFactionField]
					}
					worldPlates.update(nameplateInputs{
						scene: scene, engine: uiEngine, cam: cam, width: float32(screenWidth), height: float32(screenHeight),
						entities: worldEntities, ownGUID: worldCharacter.GUID, playerNode: worldPlayer, cameraAt: worldCamera.position,
						names: worldNameState, factions: worldFactionState, send: worldSend, playerFaction: playerFaction, ownModelPath: playerModelPath,
					})
				}
				if worldSky != nil {
					cameraPosition := cam.Position()
					worldSky.SetPositionVec(&cameraPosition)
				}
			}
			updateElapsed := elapsed
			introHolding := false
			if introGate && !worldMode {
				if sceneModel != nil && !sceneLoadPending {
					introFramesDrawn++
				}
				if introFramesDrawn >= 3 || time.Since(introStarted) > 8*time.Second {
					// Opens after the scene has been drawn a few times, or after
					// a while if it never loads, so the fade can never hang.
					introGate = false
					if debug {
						log.Printf("intro: scene drawn %d frames, login fade starts %.2fs after launch", introFramesDrawn, time.Since(introStarted).Seconds())
					}
				} else if sceneModel != nil || sceneLoadPending || len(uiEngine.VisibleGlueSceneModels()) > 0 {
					// Frames still count (the script's timers step per update),
					// but script time barely advances until the scene is up.
					updateElapsed = math.Min(elapsed, 0.001)
					introHolding = true
				} else if elapsed > 0.05 {
					updateElapsed = 0.05
				}
			}
			movieChanged := uiEngine.Update(updateElapsed)
			sceneChanged := false
			if !worldMode && setSceneModel != nil {
				sceneChanged = setSceneModel()
			}
			if !worldMode && sceneCharacterModel != nil && uiEngine.SceneCharacterFacing() != sceneCharacterFacing {
				sceneCharacterFacing = uiEngine.SceneCharacterFacing()
				sceneCharacterModel.SetRotation(0, sceneCharacterFacing*math.Pi/180, 0)
			}
			lastUpdate = now
			if (movieChanged && !introHolding) || sceneChanged {
				refresh()
			} else if minimapChanged {
				refreshMinimapOnly()
			}
			if uiEngine.DebugPanelDragging() && (lastUIRefresh.IsZero() || frameAt.Sub(lastUIRefresh) >= time.Second/30) {
				refreshDebugPanel()
			}
			worldBuildDeadline = time.Now().Add(8 * time.Millisecond)
			select {
			case loaded := <-sceneResults:
				if loaded.requestKey != sceneRequestKey {
					if loaded.model != nil {
						loaded.model.Dispose()
					}
					if loaded.characterModel != nil {
						loaded.characterModel.Dispose()
					}
					if loaded.petModel != nil {
						loaded.petModel.Dispose()
					}
					break
				}
				sceneLoadPending = false
				debugModelLoadMS = loaded.loadMS
				if loaded.err != nil || loaded.model == nil {
					if loaded.err != nil {
						debugModelError = loaded.err.Error()
						if debug {
							log.Printf("model %s: %v", loaded.path, loaded.err)
						}
					}
					uiEngine.SetSceneBackground(true)
					refresh()
					break
				}
				debugModelError = ""
				sceneModel = loaded.model
				scene.Add(sceneModel)
				sceneCameraDiagonalFOV = loaded.fov
				sceneCameraFrameFraction = glueSceneFrameFraction(sceneModel)
				configureSceneCamera(cam, sceneModel)
				applyGlueSceneAspect(sceneModel, cam.Aspect())
				sceneCharacterFacing = loaded.characterFacing
				if loaded.characterModel != nil {
					sceneCharacterModel = loaded.characterModel
					scene.Add(sceneCharacterModel)
				}
				if loaded.petModel != nil {
					scenePetModel = loaded.petModel
					scene.Add(scenePetModel)
				}
				uiEngine.SetSceneBackground(true)
				if debug {
					log.Printf("scene: loaded %s with %d parts", loaded.path, len(sceneModel.Children()))
				}
				refresh()
			case result := <-results:
				host.loginRunning = false
				if result.err != nil {
					if debug {
						log.Printf("login: %v", result.err)
					}
					uiEngine.SetStatusKey("AUTH_FAILED")
				} else {
					glueCharacters = append(glueCharacters[:0], result.session.Characters...)
					if debug {
						for index, character := range glueCharacters {
							equipment := make([]string, 0, len(character.Equipment))
							for slot, item := range character.Equipment {
								if item.DisplayID != 0 {
									equipment = append(equipment, fmt.Sprintf("%d:%d/%d", slot, item.DisplayID, item.InventoryType))
								}
							}
							log.Printf("character %d %s race=%d class=%d gender=%d appearance=%d/%d/%d/%d/%d/%d equipment=%v", index, character.Name, character.Race, character.Class, character.Gender, character.Skin, character.Face, character.HairStyle, character.HairColor, character.FacialHair, character.Map, equipment)
						}
					}
					if activeSession != nil {
						_ = activeSession.Close()
					}
					activeSession = result.session
					if rememberMe {
						if err := config.SavePassword(result.account, clientConfig.AuthAddress, result.password); err != nil && debug {
							log.Printf("saving remembered password: %v", err)
						}
					} else {
						_ = config.DeletePassword(result.account, clientConfig.AuthAddress)
					}
					uiEngine.SetGlueState(glueState(result.session, uiEngine.AssetLoader))
				}
				refresh()
			case entry := <-worldResults:
				if entry.err != nil {
					worldLoading = false
					uiEngine.SetWorldLoading(false)
					if debug {
						log.Printf("enter world: %v", entry.err)
					}
					uiEngine.SetStatusText(entry.err.Error())
					refresh()
					break
				}
				worldCharacter = entry.character
				uiEngine.Rt.ResetGameplay()
				if entry.spellErr != nil {
					log.Printf("world spell catalog: %v", entry.spellErr)
				} else {
					uiEngine.Rt.SetSpellCatalog(entry.spells, entry.skills)
				}
				if entry.mapErr != nil {
					log.Printf("world map catalog: %v", entry.mapErr)
				} else {
					uiEngine.Rt.SetMapCatalog(entry.mapData)
				}
				uiEngine.Rt.SetPlayerMapPosition(entry.position.Map, entry.position.X, entry.position.Y)
				if entry.itemErr != nil {
					log.Printf("world item catalog: %v", entry.itemErr)
				} else {
					uiEngine.Rt.SetItemCatalog(entry.itemLocal, entry.itemIcons, entry.itemClasses)
					worldItemCatalog = entry.itemLocal
				}
				seedWorldUnits(uiEngine.Rt, worldCharacter)
				// The terrain is read and built on its own thread while the
				// interface loads here.
				var terrainProgress atomic.Uint64
				terrainDone := make(chan worldTerrainLoad, 1)
				go func(position world.WorldPosition) {
					node, sceneInfo, err := loadWorldTerrainProgress(uiEngine.AssetLoader, position, func(progress float64) {
						terrainProgress.Store(math.Float64bits(progress))
					})
					terrainDone <- worldTerrainLoad{node: node, info: sceneInfo, err: err}
				}(entry.position)
				loadingPath := worldLoadingScreenPath(uiEngine.AssetLoader, entry.position.Map)
				uiEngine.SetLoadingScreen(loadingPath, 0)
				if worldUIErr := uiEngine.LoadWorldUI(); worldUIErr != nil && debug {
					log.Printf("world UI: %v", worldUIErr)
				}
				uiEngine.Rt.RequestLogout = func(bool) {
					if activeSession != nil {
						_ = activeSession.SendRaw(world.ClientOpcode(0x004B), nil) // CMSG_LOGOUT_REQUEST
					}
				}
				uiEngine.Rt.RequestLogoutCancel = func() {
					if activeSession != nil {
						_ = activeSession.SendRaw(world.ClientOpcode(0x004E), nil) // CMSG_LOGOUT_CANCEL
					}
				}
				if activeSession != nil {
					worldPackets = activeSession.StartWorldPackets()
				}
				lastLoadingFrame := time.Time{}
				showLoadingProgress := func(progress float64) {
					if progress < 1 && !lastLoadingFrame.IsZero() && time.Since(lastLoadingFrame) < time.Second/30 {
						return
					}
					uiEngine.SetLoadingScreen(loadingPath, progress)
					refresh()
					gl.Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
					if renderErr := r.Render(scene, cam); renderErr != nil && debug {
						log.Printf("loading render: %v", renderErr)
					}
					win.SwapBuffers()
					win.PollEvents()
					lastLoadingFrame = time.Now()
				}
				showLoadingProgress(0.05)
				refresh()
				gl.Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
				if renderErr := r.Render(scene, cam); renderErr != nil && debug {
					log.Printf("loading render: %v", renderErr)
				}
				win.SwapBuffers()
				win.PollEvents()
				var terrain worldTerrainLoad
				for waiting := true; waiting; {
					select {
					case terrain = <-terrainDone:
						waiting = false
					case <-time.After(10 * time.Millisecond):
						showLoadingProgress(math.Float64frombits(terrainProgress.Load()))
					}
				}
				loaded, info, loadErr := terrain.node, terrain.info, terrain.err
				if loadErr != nil {
					worldLoading = false
					uiEngine.SetWorldLoading(false)
					uiEngine.ClearLoadingScreen()
					if debug {
						log.Printf("world: %v", loadErr)
					}
					uiEngine.SetStatusText(loadErr.Error())
					refresh()
					break
				}
				worldMapName = info.mapName
				worldMapID = entry.position.Map
				if sceneModel != nil {
					scene.Remove(sceneModel)
					sceneModel.Dispose()
					sceneModel = nil
				}
				if sceneCharacterModel != nil {
					scene.Remove(sceneCharacterModel)
					sceneCharacterModel.Dispose()
					sceneCharacterModel = nil
				}
				if worldModel != nil {
					scene.Remove(worldModel)
					worldModel.Dispose()
				}
				if worldSky != nil {
					scene.Remove(worldSky)
					worldSky.Dispose()
					worldSky = nil
				}
				if worldPlayer != nil {
					scene.Remove(worldPlayer)
					worldPlayer.Dispose()
					worldPlayer = nil
					if playerMount != nil {
						playerMount.dispose(scene)
						playerMount = nil
					}
				}
				worldEntities = make(map[uint64]*worldEntity)
				worldAuras.reset()
				worldCreatureCache = worldCreatureTables{}
				m2AnimationFallbacks = readAnimationFallbacks(uiEngine.AssetLoader)
				if worldStreamer != nil {
					worldStreamer.stop()
					worldStreamer = nil
				}
				worldFloor = nil
				worldModel = loaded
				scene.Add(worldModel)
				worldSky = buildWorldSky()
				worldSky.SetPosition(entry.position.X, entry.position.Y, entry.position.Z)
				scene.Add(worldSky)
				if entry.character.GUID != 0 {
					if player, playerErr := buildWorldPlayer(uiEngine.AssetLoader, entry.character, entry.position); playerErr != nil {
						if debug {
							log.Printf("world player: %v", playerErr)
						}
					} else {
						worldPlayer = player
						playerEquipment, playerModelPath = nil, normalizeModelPath(worldCharacterModelPath(entry.character))
						scene.Add(worldPlayer)
						if debug {
							log.Printf("world: player model children=%d position=%v scale=%v", len(worldPlayer.Children()), worldPlayer.Position(), worldPlayer.Scale())
						}
					}
				}
				worldCamera = newWorldCameraController(entry.position)
				worldMoveNet = newWorldMovementNet(worldCharacter.GUID, time.Now(), func() *network.Session { return activeSession }, debug)
				worldMovement = newWorldMovementReporter(worldCharacter.GUID, func(opcode world.ClientOpcode, info world.MovementInfo) error {
					if activeSession == nil {
						return fmt.Errorf("world session is closed")
					}
					return activeSession.SendMovement(opcode, worldCharacter.GUID, info)
				})
				if streamer, ok := loaded.UserData().(*worldTerrainStreamer); ok {
					collision := streamer.collision
					worldStreamer = streamer
					worldCamera.setGround(collision.ground)
					worldCamera.setFloor(collision.floor)
					worldCamera.setMovement(collision.move)
					worldCamera.setCameraTest(collision.cameraPosition)
					worldCamera.setLiquid(collision.liquidAt)
					worldLiquid = streamer.liquid
					worldLight = readWorldLightTables(uiEngine.AssetLoader)
					worldFactionState = readWorldFactions(uiEngine.AssetLoader)
					*worldNameState = *newWorldNames()
					worldCombatState.rt = uiEngine.Rt
					worldCombatState.ownGUID = worldCharacter.GUID
					worldCombatState.entities = func() map[uint64]*worldEntity { return worldEntities }
					worldCombatState.factions = func() *worldFactions { return worldFactionState }
					worldAtmo = worldLight.sample(entry.position.Map, entry.position.X, entry.position.Y, worldClock.minuteOfDay(time.Now()))
					worldFloor = collision.floor
					if debug {
						if ground, found := collision.ground(entry.position.X, entry.position.Y); found {
							log.Printf("world: ground at entry=%.3f delta=%.3f", ground, ground-entry.position.Z)
						}
					}
				}
				configureWorldCamera(cam, entry.position, worldFarClip(uiEngine.Rt))
				worldCamera.update(1.0/60.0, cam, worldPlayer)
				uiEngine.SetWorldMinimapPosition(worldMapName, worldCamera.position[0], worldCamera.position[1])
				cameraPosition := cam.Position()
				worldSky.SetPositionVec(&cameraPosition)
				if debug {
					cameraPosition := cam.Position()
					log.Printf("world: entry position=(%.3f,%.3f,%.3f) orientation=%.3f camera=(%.3f,%.3f,%.3f) near=%.3f far=%.3f", entry.position.X, entry.position.Y, entry.position.Z, entry.position.Orientation, cameraPosition.X, cameraPosition.Y, cameraPosition.Z, cam.Near(), cam.Far())
				}
				worldMode = true
				worldLoading = false
				uiEngine.SetWorldLoading(false)
				sceneModelPath = ""
				uiEngine.ClearLoadingScreen()
				glueLayers.hide()
				if uiImage != nil {
					uiImage.SetVisible(true)
				}
				if uiMapImage != nil {
					uiMapImage.SetVisible(true)
				}
				host.StopMusic()
				gl.ClearColor(.08, .12, .16, 1)
				if debug {
					log.Printf("world: loaded %s tile %d,%d chunks=%d vertices=%d triangles=%d textures=%d wmoMeshes=%d m2Meshes=%d", info.mapName, info.tileX, info.tileY, info.chunks, info.vertices, info.triangles, info.textures, info.wmoMeshes, info.m2Meshes)
				}
				refresh()
			case event, ok := <-worldPackets:
				if !ok {
					worldPackets = nil
					break
				}
				if event.Err != nil {
					if debug {
						log.Printf("world packets: %v", event.Err)
					}
					worldPackets = nil
					break
				}
				if handled, err := worldCombatState.apply(event.Packet); handled {
					if err != nil && debug {
						log.Printf("world combat packet: %v", err)
					}
					if player := worldEntities[worldCharacter.GUID]; player != nil {
						syncWorldTarget(uiEngine.Rt, worldEntities, player, worldNameState, worldFactionState, worldSend)
					}
					refresh()
					break
				}
				if handled, err := applyWorldSpellPacket(uiEngine.Rt, event.Packet, worldCharacter.GUID); handled {
					if err != nil {
						log.Printf("world spell packet: %v", err)
					} else {
						refresh()
					}
					break
				}
				if worldMoveNet != nil && worldMoveNet.handle(event.Packet, worldCamera, worldMovement, time.Now()) {
					break
				}
				switch event.Packet.Opcode {
				case world.ItemQuerySingleResponse:
					template, itemErr := world.ParseItemQueryResponse(event.Packet.Body)
					if itemErr != nil {
						log.Printf("world item query: %v", itemErr)
						break
					}
					uiEngine.Rt.SetItemTemplate(template)
					refresh()
				case world.AuraUpdate, world.AuraUpdateAll:
					all := event.Packet.Opcode == world.AuraUpdateAll
					auraUpdate, auraErr := world.ParseAuraUpdate(event.Packet.Body, all)
					if auraErr != nil {
						if debug {
							log.Printf("world aura update: %v", auraErr)
						}
						break
					}
					if worldAuras.apply(uiEngine.Rt, uiEngine.AssetLoader, auraUpdate, all, worldCharacter.GUID) {
						refresh()
					}
				case world.MessageChat:
					message, chatErr := world.ParseMessageChat(event.Packet.Body)
					if chatErr != nil {
						if debug {
							log.Printf("world chat: %v", chatErr)
						}
						break
					}
					eventName := message.Type.EventName()
					if eventName == "" {
						break
					}
					sender := message.SenderName
					if sender == "" && message.Sender == worldCharacter.GUID {
						sender = worldCharacter.Name
					}
					guid := ""
					if message.Sender != 0 {
						guid = fmt.Sprintf("0x%016X", message.Sender)
					}
					uiEngine.FireWorldChat(eventName,
						lua.LString(message.Text), lua.LString(sender), lua.LString(message.LanguageName()),
						lua.LString(message.Channel), lua.LString(""), lua.LString(""),
						lua.LNumber(0), lua.LNumber(0), lua.LString(message.Channel), lua.LNumber(0), lua.LString(guid))
					refresh()
				case world.LoginSetTimeSpeed:
					if gameTime, timeErr := world.ParseLoginSetTimeSpeed(event.Packet.Body); timeErr == nil {
						worldClock.set(gameTime, time.Now())
					} else if debug {
						log.Printf("world game time: %v", timeErr)
					}
				case world.UpdateObject, world.CompressedUpdateObject:
					var blocks []world.UpdateBlock
					var parseErr error
					if event.Packet.Opcode == world.CompressedUpdateObject {
						blocks, parseErr = world.ParseCompressedUpdateObject(event.Packet.Body)
					} else {
						blocks, parseErr = world.ParseUpdateObject(event.Packet.Body)
					}
					if parseErr != nil {
						if debug {
							log.Printf("world update: %v", parseErr)
						}
						break
					}
					models, modelErrors := applyWorldUpdateBlocks(scene, uiEngine.AssetLoader, &worldCreatureCache, worldEntities, blocks, worldCharacter.GUID, worldFloor)
					if player := worldEntities[worldCharacter.GUID]; player != nil {
						if worldCamera != nil {
							worldCamera.syncFromUpdate(player.movement)
							worldCamera.mounted = syncWorldPlayerMount(scene, uiEngine.AssetLoader, &worldCreatureCache, &playerMount, player.fields)
						}
						if uiEngine.Rt.ApplyUnitFields("player", player.fields) {
							refresh()
						}
						if syncWorldTarget(uiEngine.Rt, worldEntities, player, worldNameState, worldFactionState, worldSend) {
							refresh()
						}
						syncWorldInventory(uiEngine.Rt, worldEntities, player)
					}
					if debug {
						log.Printf("world update: blocks=%d entities=%d models=%d errors=%d", len(blocks), len(worldEntities), models, len(modelErrors))
						for _, modelErr := range modelErrors {
							log.Printf("world entity: %v", modelErr)
						}
					}
					// Entity model changes live in the GL scene; avoid a full software UI repaint.
				case 0x004C: // SMSG_LOGOUT_RESPONSE: reason, instant
					if body := event.Packet.Body; len(body) >= 5 {
						uiEngine.Rt.LogoutResponse(binary.LittleEndian.Uint32(body[:4]), body[4] != 0)
						refresh()
					}
				case 0x004D: // SMSG_LOGOUT_COMPLETE
					uiEngine.Rt.LogoutComplete()
					refresh()
				case 0x004F: // SMSG_LOGOUT_CANCEL_ACK
					uiEngine.Rt.LogoutCancelled()
					refresh()
				case world.MonsterMoveOpcode:
					move, moveErr := world.ParseMonsterMove(event.Packet.Body)
					if moveErr != nil {
						if debug {
							log.Printf("world monster move: %v", moveErr)
						}
						break
					}
					applyWorldMonsterMove(worldEntities, move)
					if entity := worldEntities[move.GUID]; entity != nil {
						if syncErr := syncWorldEntity(scene, uiEngine.AssetLoader, &worldCreatureCache, entity, worldCharacter.GUID, worldFloor); syncErr != nil && debug {
							log.Printf("world monster entity: %v", syncErr)
						}
					}
				case world.DestroyObject:
					guid, destroyErr := world.ParseDestroyObject(event.Packet.Body)
					if destroyErr != nil {
						if debug {
							log.Printf("world destroy: %v", destroyErr)
						}
						break
					}
					if entity := worldEntities[guid]; entity != nil {
						if entity.node != nil {
							scene.Remove(entity.node)
							entity.node.Dispose()
						}
						entity.mount.dispose(scene)
						delete(worldEntities, guid)
					}
				}
			default:
			}
			if worldMode && worldCamera != nil {
				syncPendingWorldEntities(scene, uiEngine.AssetLoader, &worldCreatureCache, worldEntities, worldCharacter.GUID, worldFloor, worldCamera.position)
			}
			if uiEngine.DebugPanelVisible() && (debugPanelRefresh.IsZero() || frameAt.Sub(debugPanelRefresh) >= time.Second) {
				updateDebugPanel()
				debugPanelRefresh = frameAt
				refreshDebugPanel()
			}
		}
		debugUpdateMS = time.Since(frameAt).Seconds() * 1000
		gl.Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
		sceneStarted := time.Now()
		drawn := false
		if !worldMode && sceneModel != nil {
			frameWidth, frameHeight := win.GetSize()
			var renderErr error
			if drawn, renderErr = glueLayers.renderGlueScene(r, gl, scene, cam, sceneModel, sceneCameraDiagonalFOV, frameWidth, frameHeight); renderErr != nil && debug {
				log.Printf("render: %v", renderErr)
			}
		}
		if !drawn {
			if renderErr := r.Render(scene, cam); renderErr != nil && debug {
				log.Printf("render: %v", renderErr)
			}
		}
		debugSceneMS = time.Since(sceneStarted).Seconds() * 1000
		swapStarted := time.Now()
		win.SwapBuffers()
		win.PollEvents()
		debugSwapMS = time.Since(swapStarted).Seconds() * 1000
	}
	if width, height := win.GetSize(); width >= 640 && height >= 480 && (width != savedOptions.WindowWidth || height != savedOptions.WindowHeight) {
		savedOptions.WindowWidth, savedOptions.WindowHeight = width, height
		if saveErr := config.Save(configPath, savedOptions); saveErr != nil && debug {
			log.Printf("saving window size: %v", saveErr)
		}
	}
	if worldModel != nil {
		scene.Remove(worldModel)
		worldModel.Dispose()
	}
	if worldSky != nil {
		scene.Remove(worldSky)
		worldSky.Dispose()
	}
	if worldPlayer != nil {
		scene.Remove(worldPlayer)
		worldPlayer.Dispose()
	}
	if sceneModel != nil {
		scene.Remove(sceneModel)
		sceneModel.Dispose()
	}
	if sceneCharacterModel != nil {
		scene.Remove(sceneCharacterModel)
		sceneCharacterModel.Dispose()
	}
	if activeSession != nil {
		_ = activeSession.Close()
	}
}

func installCursor(win *window.GlfwWindow, loader *ui.Loader) (*glfw.Cursor, error) {
	data, err := loader.ReadAsset("Interface\\Cursor\\Point")
	if err != nil {
		return nil, err
	}
	image, err := ui.DecodeBLP(data)
	if err != nil {
		return nil, err
	}
	cursor := glfw.CreateCursor(image, 0, 0)
	if cursor == nil {
		return nil, os.ErrInvalid
	}
	win.Window.SetCursor(cursor)
	return cursor, nil
}

func resolveInterfaceRoot(dataPath, interfacePath string) string {
	if interfacePath != "" {
		return interfacePath
	}
	for _, candidate := range []string{dataPath, filepath.Dir(dataPath)} {
		if candidate == "" || candidate == "." {
			continue
		}
		if _, err := os.Stat(filepath.Join(candidate, "GlueXML", "GlueXML.toc")); err == nil {
			return candidate
		}
	}
	return ""
}

func glueState(session *network.Session, loader *ui.Loader) ui.GlueState {
	state := ui.GlueState{Connected: true, ServerName: session.Realm.Name, SelectedRealm: int(session.Realm.ID)}
	areaNames := loadAreaNames(loader)
	population := strconv.FormatFloat(float64(session.Realm.Population), 'f', 3, 32)
	state.Realms = []ui.RealmInfo{{Name: session.Realm.Name, Address: session.Realm.Address, Population: population, RealmType: realmType(session.Realm.Kind), ID: int(session.Realm.ID), Characters: int(session.Realm.Characters), Down: session.Realm.IsOffline(), Current: true, Locked: session.Realm.Locked, Load: float64(session.Realm.Population)}}
	state.Characters = make([]ui.CharacterEntry, 0, len(session.Characters))
	for _, character := range session.Characters {
		state.Characters = append(state.Characters, ui.CharacterEntry{Name: character.Name, Race: raceName(character.Race), RaceID: int(character.Race), Class: className(character.Class), ClassID: int(character.Class), Gender: int(character.Gender), Level: int(character.Level), Zone: areaNames[character.Zone], ZoneID: character.Zone, MapID: character.Map, Flags: character.Flags, CustomizeFlags: character.CustomizeFlags, BackgroundModel: backgroundModelName(character), PetDisplayID: character.PetDisplayID, PetLevel: character.PetLevel, PetFamily: character.PetFamily})
	}
	return state
}

func seedWorldUnits(rt *ui.Runtime, character world.Character) {
	if rt == nil {
		return
	}
	sex := 2
	if character.Gender == 1 {
		sex = 3
	}
	rt.SetUnit("player", ui.UnitInfo{GUID: character.GUID, Exists: true, Name: character.Name, Level: int(character.Level), RaceID: int(character.Race), RaceFile: raceModelName(character.Race), RaceName: raceName(character.Race), ClassID: int(character.Class), ClassFile: className(character.Class), ClassName: className(character.Class), Sex: sex, Health: 1, HealthMax: 1, Power: 1, PowerMax: 1, PowerToken: "MANA", Connected: true, Player: true, Visible: true})
	if character.PetDisplayID != 0 {
		rt.SetUnit("pet", ui.UnitInfo{Exists: true, Name: "", Level: int(character.PetLevel), Health: 1, HealthMax: 1, Power: 1, PowerMax: 1, PowerToken: "MANA", Connected: true, Visible: true})
	} else {
		rt.ClearUnit("pet")
	}
}

func realmType(kind uint8) string {
	switch kind {
	case 1:
		return "PVP"
	case 6:
		return "RP"
	case 8:
		return "RP-PVP"
	default:
		return "Normal"
	}
}

func raceName(id uint8) string {
	if name, ok := map[uint8]string{1: "RACE_HUMAN", 2: "RACE_ORC", 3: "RACE_DWARF", 4: "RACE_NIGHTELF", 5: "RACE_SCOURGE", 6: "RACE_TAUREN", 7: "RACE_GNOME", 8: "RACE_TROLL", 10: "RACE_BLOODELF", 11: "RACE_DRAENEI"}[id]; ok {
		return name
	}
	return strconv.Itoa(int(id))
}

func className(id uint8) string {
	if name, ok := map[uint8]string{1: "WARRIOR", 2: "PALADIN", 3: "HUNTER", 4: "ROGUE", 5: "PRIEST", 6: "DEATHKNIGHT", 7: "SHAMAN", 8: "MAGE", 9: "WARLOCK", 11: "DRUID"}[id]; ok {
		return name
	}
	return strconv.Itoa(int(id))
}

func backgroundModelName(character world.Character) string {
	if character.Class == 6 {
		return "DeathKnight"
	}
	return raceModelName(character.Race)
}

func raceModelName(id uint8) string {
	if name, ok := map[uint8]string{1: "Human", 2: "Orc", 3: "Dwarf", 4: "NightElf", 5: "Scourge", 6: "Tauren", 7: "Dwarf", 8: "Orc", 10: "BloodElf", 11: "Draenei"}[id]; ok {
		return name
	}
	return "CharacterSelect"
}

func resetSceneCamera(cam *camera.Camera) {
	cam.SetPosition(0, 0, 3)
	cam.SetFov(60)
	cam.SetNear(0.3)
	cam.SetFar(1000)
	cam.LookAt(math32.NewVector3(0, 0, 0), math32.NewVector3(0, 1, 0))
}

func sceneCharacterTransform(background, character glueModelInfo, normalizedPosition math32.Vector3, aspect float32) (float32, math32.Vector3) {
	scale := background.modelScale * sceneCharacterFit(background, character, aspect)
	factor := float32(1)
	if character.modelScale > 0 {
		factor = scale / character.modelScale
	}
	return scale, *math32.NewVector3(background.standPosition.X+normalizedPosition.X*factor, background.standPosition.Y+(normalizedPosition.Y-character.modelBottom)*factor, background.standPosition.Z+normalizedPosition.Z*factor)
}

func configureSceneCamera(cam *camera.Camera, model *core.Node) {
	resetSceneCamera(cam)
	info, ok := model.UserData().(glueModelInfo)
	if !ok {
		return
	}
	cam.SetPositionVec(&info.position)
	cam.LookAt(&info.target, math32.NewVector3(0, 1, 0))
	setSceneCameraFOV(cam, info.fov, glueSceneFrameFraction(model))
	if info.near > 0 {
		cam.SetNear(info.near)
	}
	if info.far > info.near {
		cam.SetFar(info.far)
	}
}

// worldFarClip is the distance the world is drawn to: Wow.exe projects with
// the farclip cvar (177..1277, default 350) times horizonFarclipScale (default 4).
func worldFarClip(rt *ui.Runtime) float32 {
	farclip, scale := 350.0, 4.0
	if rt != nil {
		if value, ok := rt.GetCVar("farclip"); ok {
			if parsed, err := strconv.ParseFloat(value, 64); err == nil {
				farclip = math.Min(1277, math.Max(177, parsed))
			}
		}
		if value, ok := rt.GetCVar("horizonFarclipScale"); ok {
			if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed >= 1 && parsed <= 6 {
				scale = parsed
			}
		}
	}
	return float32(farclip * scale)
}

func configureWorldCamera(cam *camera.Camera, position world.WorldPosition, far float32) {
	cam.SetPosition(position.X, position.Y, position.Z+4)
	cam.SetFov(70)
	cam.SetNear(0.2)
	cam.SetFar(far)
	directionX := math32.Cos(position.Orientation)
	directionY := math32.Sin(position.Orientation)
	target := math32.NewVector3(position.X+directionX*20, position.Y+directionY*20, position.Z+2)
	cam.LookAt(target, math32.NewVector3(0, 0, 1))
}

// setSceneCameraFOV sets the vertical FOV from the camera's diagonal FOV. The
// original draws a Model frame's scene inside the frame rectangle, so when that
// rectangle is shorter than the window (frameFraction < 1, e.g. the 16:9 login
// scene in a 4:3 window) the window needs a proportionally larger vertical FOV
// for the rectangle to keep the original framing.
func setSceneCameraFOV(cam *camera.Camera, fov, frameFraction float32) {
	if cam == nil || fov <= 0 || cam.Aspect() <= 0 {
		return
	}
	vertical := fov / math32.Sqrt(cam.Aspect()*cam.Aspect()+1)
	if frameFraction > 0 && frameFraction != 1 {
		vertical = 2 * math32.Atan(math32.Tan(vertical/2)/frameFraction)
	}
	cam.SetFov(math32.RadToDeg(vertical))
}

// sceneCharacterFit shrinks a character that stands taller than the scene's
// camera shows. Each race's scene is framed for the characters of that race at
// their own size, with the camera looking at a point about a yard up; a model
// that rises past the top of the view is brought down to fit it, and one that
// fits is left alone.
func sceneCharacterFit(background, character glueModelInfo, aspect float32) float32 {
	if !background.hasStand || background.fov <= 0 || character.rawHeight <= 0 || aspect <= 0 {
		return 1
	}
	vertical := float64(background.fov) / math.Sqrt(float64(aspect*aspect)+1)
	dx, dy, dz := background.position.X-background.standPosition.X, background.position.Y-background.standPosition.Y, background.position.Z-background.standPosition.Z
	distance := math.Sqrt(float64(dx*dx + dy*dy + dz*dz))
	top := float64(background.target.Y) + 0.485*2*distance*math.Tan(vertical/2)
	room := top - float64(background.standPosition.Y)
	height := float64(character.rawHeight * background.modelScale)
	if height <= 0 || room >= height {
		return 1
	}
	if room <= 0 {
		return 1
	}
	return float32(room / height)
}
