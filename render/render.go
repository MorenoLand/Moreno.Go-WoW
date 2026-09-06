package render

import (
	"fmt"
	"log"
	"math"
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
	lua "github.com/yuin/gopher-lua"
)

type loginResult struct {
	session  *network.Session
	account  string
	password string
	err      error
}

type worldEntryResult struct {
	position  world.WorldPosition
	character world.Character
	err       error
}

type clientHost struct {
	width        float64
	height       float64
	startLogin   func(string, string)
	enterWorld   func(int)
	logout       func()
	quit         func()
	audio        *audioManager
	saveAudio    func(string, string)
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
func (h *clientHost) Logout() {
	if h.logout != nil {
		h.logout()
	}
}

func Run(clientConfig network.Config, dataPath, interfacePath, backgroundPath, lastCharacter, configPath string, savedOptions config.Options, debug, rememberMe bool) {
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
	host := &clientHost{width: 960, height: 640}
	results := make(chan loginResult, 1)
	worldResults := make(chan worldEntryResult, 1)
	var uiEngine *ui.UIEngine
	var activeSession *network.Session
	var worldPackets <-chan world.PacketEvent
	worldLoading := false
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
		go func() {
			position, err := session.EnterWorld(index)
			worldResults <- worldEntryResult{position: position, character: character, err: err}
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
	var eng *ui.UIEngine
	var err error
	lastUIRefresh := time.Time{}
	var sceneModel *core.Node
	var sceneCharacterModel *core.Node
	sceneCharacterFacing := float32(0)
	sceneModelPath := ""
	sceneCharacterKey := ""
	sceneCameraDiagonalFOV := float32(0)
	debugModelLoadMS := float64(0)
	debugUIRenderMS := float64(0)
	debugModelError := ""
	worldMode := false
	var worldCamera *worldCameraController
	var worldPlayer *core.Node
	var worldModel *core.Node
	var worldSky *core.Node
	worldEntities := make(map[uint64]*worldEntity)

	worldCreatureCache := worldCreatureTables{}
	var worldFloor func(float32, float32, float32) (float32, bool)
	var worldCharacter world.Character
	var glueCharacters []world.Character
	var setSceneModel func() bool
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
			characterKey := ""
			var selectedCharacter world.Character
			selectedIndex := uiEngine.SelectedCharacterIndex()
			if createState, ok := uiEngine.CreatePreviewState(); ok {
				selectedCharacter = world.Character{Race: createState.RaceID, Class: createState.ClassID, Gender: createState.Gender}
				characterKey = fmt.Sprintf("create:%d:%d:%d", createState.RaceID, createState.Gender, createState.ClassID)
			} else if uiEngine.CharacterSelectVisible() && selectedIndex >= 0 && selectedIndex < len(glueCharacters) {
				index := selectedIndex
				selectedCharacter = glueCharacters[index]
				characterKey = fmt.Sprintf("%d:%s", selectedCharacter.GUID, worldCharacterModelPath(selectedCharacter))
			}
			if path == sceneModelPath && characterKey == sceneCharacterKey {
				return false
			}
			if debug {
				log.Printf("scene request path=%s selected=%d key=%s previous=%s/%s", path, selectedIndex, characterKey, sceneModelPath, sceneCharacterKey)
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
			resetSceneCamera(cam)
			sceneCameraDiagonalFOV = 0
			sceneModelPath = path
			sceneCharacterKey = characterKey
			debugModelError = ""
			debugModelLoadMS = 0
			uiEngine.SetSceneBackground(false)
			if path == "" {
				return true
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
				return true
			}
			debugModelError = ""
			sceneModel = model
			scene.Add(sceneModel)
			if info, ok := sceneModel.UserData().(glueModelInfo); ok {
				sceneCameraDiagonalFOV = info.fov
			}
			configureSceneCamera(cam, sceneModel)
			if characterKey != "" {
				characterModel, characterErr := loadGlueCharacterModel(uiEngine.AssetLoader, selectedCharacter)
				if characterErr != nil {
					if debug {
						log.Printf("character select model %s: %v", worldCharacterModelPath(selectedCharacter), characterErr)
					}
				} else {
					if backgroundInfo, ok := sceneModel.UserData().(glueModelInfo); ok && backgroundInfo.hasStand {
						characterInfo, _ := characterModel.UserData().(glueModelInfo)
						characterScale, characterPosition := sceneCharacterTransform(backgroundInfo, characterInfo, characterModel.Position())
						characterModel.SetScale(characterScale, characterScale, characterScale)
						characterModel.SetPosition(characterPosition.X, characterPosition.Y, characterPosition.Z)
					}
					sceneCharacterFacing = uiEngine.SceneCharacterFacing()
					characterModel.SetRotation(0, sceneCharacterFacing*math.Pi/180, 0)
					sceneCharacterModel = characterModel
					scene.Add(sceneCharacterModel)
				}
			}
			uiEngine.SetSceneBackground(true)
			if debug {
				log.Printf("scene: loaded %s with %d parts", path, len(sceneModel.Children()))
			}
			return true
		}
		setSceneModel()
		initialUI := eng.Render(960, 640)
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
		uiFrame := uiEngine.Render(width, height)
		if worldMode {
			uiFrame = uiEngine.RenderWorld(width, height)
		}
		// Reuse one GPU texture and upload pixels in place. Recreating a
		// Texture2D every UI paint forced a full delete/alloc + material rebind.
		if uiTex == nil {
			uiTex = texture.NewTexture2DFromRGBA(uiFrame)
			uiImage.SetTexture(uiTex)
		} else {
			uiTex.SetFromRGBA(uiFrame)
		}
		debugUIRenderMS = time.Since(uiStarted).Seconds() * 1000
		uiImage.SetSize(float32(width), float32(height))
		lastUIRefresh = time.Now()
	}

	host.logout = func() {
		if !worldMode {
			return
		}
		worldMode = false
		worldCamera = nil
		worldPackets = nil
		worldLoading = false
		if uiEngine != nil {
			uiEngine.SetWorldLoading(false)
			uiEngine.ClearLoadingScreen()
			uiEngine.SetGlueState(uiEngine.Rt.Glue)
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
		if worldPlayer != nil {
			scene.Remove(worldPlayer)
			worldPlayer.Dispose()
			worldPlayer = nil
		}
		for guid, entity := range worldEntities {
			if entity != nil && entity.node != nil {
				scene.Remove(entity.node)
				entity.node.Dispose()
			}
			delete(worldEntities, guid)
		}
		gl.ClearColor(.04, .06, .1, 1)
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
				cam.SetFar(6000)
			} else {
				setSceneCameraFOV(cam, sceneCameraDiagonalFOV)
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
		})
		win.Subscribe(window.OnMouseDown, func(_ string, event interface{}) {
			mouse := event.(*window.MouseEvent)
			if worldMode && worldCamera != nil && worldCamera.handleMouse(float64(mouse.Xpos), float64(mouse.Ypos), mouse.Button, true) {
				return
			}
			if uiEngine.HandleMouse(float64(mouse.Xpos), float64(mouse.Ypos), mouse.Button, true) {
				refresh()
			}
		})
		win.Subscribe(window.OnMouseUp, func(_ string, event interface{}) {
			mouse := event.(*window.MouseEvent)
			if worldMode && worldCamera != nil && worldCamera.handleMouse(float64(mouse.Xpos), float64(mouse.Ypos), mouse.Button, false) {
				return
			}
			if uiEngine.HandleMouse(float64(mouse.Xpos), float64(mouse.Ypos), mouse.Button, false) {
				refresh()
			}
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
				refresh()
			}
		})
		win.Subscribe(window.OnKeyDown, func(_ string, event interface{}) {
			key := event.(*window.KeyEvent)
			if worldMode && worldCamera != nil && worldCamera.handleKey(key.Key, true) {
				return
			}
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
			if worldMode && worldCamera != nil && worldCamera.handleKey(key.Key, false) {
				return
			}
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
				if info, ok := sceneModel.UserData().(glueModelInfo); ok {
					if info.animation != nil {
						for _, soundID := range info.animation.Update(elapsed) {
							if debug {
								log.Printf("scene sound event id=%d", soundID)
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
			if worldMode && worldCamera != nil {
				worldCamera.update(elapsed, cam, worldPlayer)
				advanceWorldEntities(worldEntities, elapsed, worldFloor)
				if worldPlayer != nil {
					if info, ok := worldPlayer.UserData().(glueModelInfo); ok {
						if info.animation != nil {
							motion := uint16(0)
							if worldCamera.isAirborne() {
								motion = 38
							} else if worldCamera.isMoving() {
								motion = 5
							}
							info.animation.SetMotion(motion)
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
				}
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
					if info, ok := entity.node.UserData().(glueModelInfo); ok {
						if info.animation != nil {
							motion := uint16(0)
							if worldEntityMoving(entity) {
								motion = worldEntityMotion(entity)
							}
							info.animation.SetMotion(motion)
							for _, soundID := range info.animation.Update(elapsed) {
								if host.audio != nil {
									host.audio.PlaySoundID(soundID)
								}
							}
						}
						if info.particles != nil {
							info.particles.Update(elapsed)
						}
					}
				}
				if worldSky != nil {
					cameraPosition := cam.Position()
					worldSky.SetPositionVec(&cameraPosition)
				}
			}
			movieChanged := uiEngine.Update(elapsed)
			sceneChanged := false
			if !worldMode && setSceneModel != nil {
				sceneChanged = setSceneModel()
			}
			if !worldMode && sceneCharacterModel != nil && uiEngine.SceneCharacterFacing() != sceneCharacterFacing {
				sceneCharacterFacing = uiEngine.SceneCharacterFacing()
				sceneCharacterModel.SetRotation(0, sceneCharacterFacing*math.Pi/180, 0)
			}
			lastUpdate = now
			if movieChanged || sceneChanged {
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
				loadingPath := worldLoadingScreenPath(uiEngine.AssetLoader, entry.position.Map)
				uiEngine.SetLoadingScreen(loadingPath, 0)
				if worldUIErr := uiEngine.LoadWorldUI(); worldUIErr != nil && debug {
					log.Printf("world UI: %v", worldUIErr)
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
				loaded, info, loadErr := loadWorldTerrainProgress(uiEngine.AssetLoader, entry.position, showLoadingProgress)
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
				}
				worldEntities = make(map[uint64]*worldEntity)
				worldCreatureCache = worldCreatureTables{}
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
						scene.Add(worldPlayer)
						if debug {
							log.Printf("world: player model children=%d position=%v scale=%v", len(worldPlayer.Children()), worldPlayer.Position(), worldPlayer.Scale())
						}
					}
				}
				worldCamera = newWorldCameraController(entry.position)
				if collision, ok := loaded.UserData().(worldSceneCollision); ok {
					worldCamera.setGround(collision.ground)
					worldCamera.setFloor(collision.floor)
					worldCamera.setMovement(collision.move)
					worldCamera.setCameraTest(collision.cameraPosition)
					worldFloor = collision.floor
					if debug {
						if ground, found := collision.ground(entry.position.X, entry.position.Y); found {
							log.Printf("world: ground at entry=%.3f delta=%.3f", ground, ground-entry.position.Z)
						}
					}
				}
				configureWorldCamera(cam, entry.position)
				worldCamera.update(1.0/60.0, cam, worldPlayer)
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
				if uiImage != nil {
					uiImage.SetVisible(true)
				}
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
				switch event.Packet.Opcode {
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
					if debug {
						log.Printf("world update: blocks=%d entities=%d models=%d errors=%d", len(blocks), len(worldEntities), models, len(modelErrors))
						for _, modelErr := range modelErrors {
							log.Printf("world entity: %v", modelErr)
						}
					}
					// Entity model changes live in the GL scene; avoid a full software UI repaint.
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
						delete(worldEntities, guid)
					}
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
		state.Characters = append(state.Characters, ui.CharacterEntry{Name: character.Name, Race: raceName(character.Race), RaceID: int(character.Race), Class: className(character.Class), ClassID: int(character.Class), Gender: int(character.Gender), Level: int(character.Level), Zone: areaNames[character.Zone], ZoneID: character.Zone, MapID: character.Map, Flags: character.Flags, CustomizeFlags: character.CustomizeFlags, BackgroundModel: backgroundModelName(character)})
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

func sceneCharacterTransform(background, character glueModelInfo, normalizedPosition math32.Vector3) (float32, math32.Vector3) {
	factor := float32(1)
	if character.modelScale > 0 {
		factor = background.modelScale / character.modelScale
	}
	return background.modelScale, *math32.NewVector3(background.standPosition.X+normalizedPosition.X*factor, background.standPosition.Y+(normalizedPosition.Y-character.modelBottom)*factor, background.standPosition.Z+normalizedPosition.Z*factor)
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
