package render

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
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
)

type loginResult struct {
	session  *network.Session
	account  string
	password string
	err      error
}

type worldEntryResult struct {
	position world.WorldPosition
	err      error
}

type clientHost struct {
	width        float64
	height       float64
	startLogin   func(string, string)
	enterWorld   func(int)
	quit         func()
	audio        *audioManager
	loginRunning bool
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

func Run(clientConfig network.Config, dataPath, interfacePath, backgroundPath, lastCharacter, configPath string, debug, rememberMe bool) {
	if err := window.Init(960, 640, "MorenoWoW"); err != nil {
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
	host := &clientHost{width: 960, height: 640}
	results := make(chan loginResult, 1)
	worldResults := make(chan worldEntryResult, 1)
	var uiEngine *ui.UIEngine
	var activeSession *network.Session
	worldLoading := false
	host.enterWorld = func(index int) {
		if activeSession == nil || worldLoading {
			return
		}
		worldLoading = true
		session := activeSession
		go func() {
			position, err := session.EnterWorld(index)
			worldResults <- worldEntryResult{position: position, err: err}
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
	var eng *ui.UIEngine
	var err error
	lastUIRefresh := time.Time{}
	var sceneModel *core.Node
	sceneModelPath := ""
	sceneCameraDiagonalFOV := float32(0)
	debugModelLoadMS := float64(0)
	debugUIRenderMS := float64(0)
	debugModelError := ""
	worldMode := false
	var worldModel *core.Node
	var setSceneModel func()
	if dataPath != "" {
		eng, err = ui.LoadUIEngineFromMPQ(dataPath, clientConfig.Locale, backgroundPath)
	} else if root := resolveInterfaceRoot(dataPath, interfacePath); root != "" {
		eng, err = ui.LoadUIEngine(filepath.Join(root, "GlueXML"), filepath.Join(root, "FrameXML"), filepath.Join(root, "Interface-tree"), backgroundPath)
	}
	if err != nil {
		log.Printf("ui render error: %v", err)
	} else if eng != nil {
		uiEngine = eng
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
		uiEngine.SetInitialCredentials(clientConfig.Account, clientConfig.Password, rememberMe)
		if lastCharacter != "" {
			uiEngine.Rt.SetCVar("lastCharacter", lastCharacter)
		}
		setSceneModel = func() {
			if worldMode {
				return
			}
			path := uiEngine.CurrentModelPath()
			if path == sceneModelPath {
				return
			}
			if sceneModel != nil {
				scene.Remove(sceneModel)
				sceneModel.Dispose()
				sceneModel = nil
			}
			resetSceneCamera(cam)
			sceneCameraDiagonalFOV = 0
			sceneModelPath = path
			debugModelError = ""
			debugModelLoadMS = 0
			uiEngine.SetSceneBackground(false)
			if path == "" {
				return
			}
			if debug {
				log.Printf("scene: loading %s", path)
			}
			modelStarted := time.Now()
			model, modelErr := loadGlueModel(uiEngine.AssetLoader, path)
			debugModelLoadMS = time.Since(modelStarted).Seconds() * 1000
			if modelErr != nil {
				debugModelError = modelErr.Error()
				if debug {
					log.Printf("model %s: %v", path, modelErr)
				}
				return
			}
			debugModelError = ""
			sceneModel = model
			scene.Add(sceneModel)
			if info, ok := sceneModel.UserData().(glueModelInfo); ok {
				sceneCameraDiagonalFOV = info.fov
			}
			configureSceneCamera(cam, sceneModel)
			uiEngine.SetSceneBackground(true)
			if debug {
				log.Printf("scene: loaded %s with %d parts", path, len(sceneModel.Children()))
			}
		}
		setSceneModel()
		initialUI := eng.Render(960, 640)
		uiImage = gui.NewImageFromRGBA(initialUI)
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
		assetStats := uiEngine.AssetLoader.AssetStats()
		uiEngine.SetDebugPanelLines(debugPanelLines(debugPanelData{
			width: width, height: height, fps: debugFPS, frameMS: debugFrameMS, uiRenderMS: debugUIRenderMS,
			modelLoadMS: debugModelLoadMS, gpuVendor: gpuVendor, gpuRenderer: gpuRenderer, gpuVersion: gpuVersion,
			dataPath: dataPath, scenePath: sceneModelPath, connection: connection, authAddress: clientConfig.AuthAddress,
			model: modelStats, sceneParts: parts, assetCache: len(uiEngine.Cache), mpqArchives: assetStats.Archives,
			mpqCachedFiles: assetStats.CachedFiles, mpqMissingFiles: assetStats.MissingFiles, audio: host.audio != nil,
			cursor: wowCursor != nil, modelError: debugModelError, terminalDebug: debug,
		}))
	}

	refresh := func() {
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
		previous := uiImage.SetTexture(texture.NewTexture2DFromRGBA(uiEngine.Render(width, height)))
		if previous != nil {
			previous.Dispose()
		}
		debugUIRenderMS = time.Since(uiStarted).Seconds() * 1000
		uiImage.SetSize(float32(width), float32(height))
		lastUIRefresh = time.Now()
	}

	onResize := func(string, interface{}) {
		width, height := win.GetSize()
		gl.Viewport(0, 0, int32(width), int32(height))
		if height > 0 {
			cam.SetAspect(float32(width) / float32(height))
			setSceneCameraFOV(cam, sceneCameraDiagonalFOV)
		}
		refresh()
	}
	win.Subscribe(window.OnWindowSize, onResize)
	if uiEngine != nil {
		win.Subscribe(window.OnCursor, func(_ string, event interface{}) {
			cursor := event.(*window.CursorEvent)
			if uiEngine.HandleCursor(float64(cursor.Xpos), float64(cursor.Ypos)) {
				if !uiEngine.DebugPanelDragging() {
					refresh()
				}
			}
		})
		win.Subscribe(window.OnMouseDown, func(_ string, event interface{}) {
			mouse := event.(*window.MouseEvent)
			if uiEngine.HandleMouse(float64(mouse.Xpos), float64(mouse.Ypos), mouse.Button, true) {
				refresh()
			}
		})
		win.Subscribe(window.OnMouseUp, func(_ string, event interface{}) {
			mouse := event.(*window.MouseEvent)
			if uiEngine.HandleMouse(float64(mouse.Xpos), float64(mouse.Ypos), mouse.Button, false) {
				refresh()
			}
		})
		win.Subscribe(window.OnChar, func(_ string, event interface{}) {
			char := event.(*window.CharEvent)
			if uiEngine.HandleChar(char.Char) {
				refresh()
			}
		})
		win.Subscribe(window.OnKeyDown, func(_ string, event interface{}) {
			key := event.(*window.KeyEvent)
			if key.Key == window.KeyF2 {
				uiEngine.HandleKeyWithMods(key.Key, key.Mods)
				updateDebugPanel()
				debugPanelRefresh = time.Now()
				refresh()
				return
			}
			if uiEngine.HandleKeyWithMods(key.Key, key.Mods) {
				refresh()
			}
		})
		win.Subscribe(window.OnKeyUp, func(_ string, event interface{}) {
			key := event.(*window.KeyEvent)
			if uiEngine.HandleKeyUp(key.Key) {
				refresh()
			}
		})
	}
	onResize("", nil)
	gl.ClearColor(.04, .06, .1, 1)
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
				if info, ok := sceneModel.UserData().(glueModelInfo); ok && info.particles != nil {
					info.particles.Update(elapsed)
				}
			}
			movieChanged := uiEngine.Update(elapsed)
			lastUpdate = now
			if movieChanged {
				refresh()
			}
			if uiEngine.DebugPanelDragging() && (lastUIRefresh.IsZero() || frameAt.Sub(lastUIRefresh) >= time.Second/60) {
				refresh()
			}
			select {
			case result := <-results:
				host.loginRunning = false
				if result.err != nil {
					if debug {
						log.Printf("login: %v", result.err)
					}
					uiEngine.SetStatusKey("AUTH_FAILED")
				} else {
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
				worldLoading = false
				if entry.err != nil {
					if debug {
						log.Printf("enter world: %v", entry.err)
					}
					uiEngine.SetStatusText(entry.err.Error())
					refresh()
					break
				}
				loaded, info, loadErr := loadWorldTerrain(uiEngine.AssetLoader, entry.position)
				if loadErr != nil {
					if debug {
						log.Printf("world: %v", loadErr)
					}
					uiEngine.SetStatusText(loadErr.Error())
					refresh()
					break
				}
				if sceneModel != nil {
					scene.Remove(sceneModel)
					sceneModel.Dispose()
					sceneModel = nil
				}
				if worldModel != nil {
					scene.Remove(worldModel)
					worldModel.Dispose()
				}
				worldModel = loaded
				scene.Add(worldModel)
				worldMode = true
				sceneModelPath = ""
				if uiImage != nil {
					uiImage.SetVisible(false)
				}
				configureWorldCamera(cam, entry.position)
				gl.ClearColor(.08, .12, .16, 1)
				if debug {
					log.Printf("world: loaded %s tile %d,%d chunks=%d vertices=%d triangles=%d textures=%d wmoMeshes=%d", info.mapName, info.tileX, info.tileY, info.chunks, info.vertices, info.triangles, info.textures, info.wmoMeshes)
				}
			default:
			}
			if uiEngine.DebugPanelVisible() && (debugPanelRefresh.IsZero() || frameAt.Sub(debugPanelRefresh) >= 250*time.Millisecond) {
				updateDebugPanel()
				debugPanelRefresh = frameAt
				refresh()
			}
		}
		gl.Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
		if renderErr := r.Render(scene, cam); renderErr != nil && debug {
			log.Printf("render: %v", renderErr)
		}
		win.SwapBuffers()
		win.PollEvents()
	}
	if worldModel != nil {
		scene.Remove(worldModel)
		worldModel.Dispose()
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
		state.Characters = append(state.Characters, ui.CharacterEntry{Name: character.Name, Race: raceName(character.Race), RaceID: int(character.Race), Class: className(character.Class), ClassID: int(character.Class), Gender: int(character.Gender), Level: int(character.Level), Zone: areaNames[character.Zone], ZoneID: character.Zone, MapID: character.Map, Flags: character.Flags, CustomizeFlags: character.CustomizeFlags, BackgroundModel: raceModelName(character.Race)})
	}
	return state
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

func configureSceneCamera(cam *camera.Camera, model *core.Node) {
	resetSceneCamera(cam)
	info, ok := model.UserData().(glueModelInfo)
	if !ok {
		return
	}
	cam.SetPositionVec(&info.position)
	cam.LookAt(&info.target, math32.NewVector3(0, 1, 0))
	setSceneCameraFOV(cam, info.fov)
	if info.near > 0 {
		cam.SetNear(info.near)
	}
	if info.far > info.near {
		cam.SetFar(info.far)
	}
}

func configureWorldCamera(cam *camera.Camera, position world.WorldPosition) {
	cam.SetPosition(position.X, position.Y, position.Z+4)
	cam.SetFov(70)
	cam.SetNear(0.2)
	cam.SetFar(6000)
	directionX := math32.Cos(position.Orientation)
	directionY := math32.Sin(position.Orientation)
	target := math32.NewVector3(position.X+directionX*20, position.Y+directionY*20, position.Z+2)
	cam.LookAt(target, math32.NewVector3(0, 0, 1))
}

func setSceneCameraFOV(cam *camera.Camera, diagonal float32) {
	if diagonal <= 0 {
		return
	}
	vertical := 2 * math32.Atan(math32.Tan(diagonal/2)/math32.Sqrt(cam.Aspect()*cam.Aspect()+1))
	cam.SetFov(math32.RadToDeg(vertical))
}
