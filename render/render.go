package render

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/MorenoLand/Moreno.WoW/config"
	"github.com/MorenoLand/Moreno.WoW/network"
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/renderer"
	"github.com/g3n/engine/texture"
	"github.com/g3n/engine/window"
)

type loginResult struct {
	session  *network.Session
	account  string
	password string
	err      error
}

type clientHost struct {
	width        float64
	height       float64
	startLogin   func(string, string)
	quit         func()
	loginRunning bool
}

func (h *clientHost) ScreenSize() (float64, float64) { return h.width, h.height }
func (h *clientHost) PlaySound(string)               {}
func (h *clientHost) PlayMusic(string)               {}
func (h *clientHost) PlayAmbience(string)            {}
func (h *clientHost) StopMusic()                     {}
func (h *clientHost) StopAmbience()                  {}
func (h *clientHost) StopAllSFX()                    {}
func (h *clientHost) LaunchURL(string)               {}
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
	gl := win.Gls()
	r := renderer.NewRenderer(gl)
	if err := r.AddDefaultShaders(); err != nil {
		log.Printf("renderer: %v", err)
		return
	}
	scene := core.NewNode()
	gui.Manager().Set(scene)
	host := &clientHost{width: 960, height: 640}
	results := make(chan loginResult, 1)
	var uiEngine *ui.UIEngine
	var activeSession *network.Session
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
		uiEngine.SetInitialCredentials(clientConfig.Account, clientConfig.Password, rememberMe)
		if lastCharacter != "" {
			uiEngine.Rt.SetCVar("lastCharacter", lastCharacter)
		}
		uiImage = gui.NewImageFromRGBA(eng.Render(960, 640))
		uiImage.SetPosition(0, 0)
		scene.Add(uiImage)
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
		uiImage.SetTexture(texture.NewTexture2DFromRGBA(uiEngine.Render(width, height)))
		uiImage.SetSize(float32(width), float32(height))
	}

	cam := camera.New(1)
	cam.SetPosition(0, 0, 3)
	scene.Add(cam)
	onResize := func(string, interface{}) {
		width, height := win.GetSize()
		gl.Viewport(0, 0, int32(width), int32(height))
		if height > 0 {
			cam.SetAspect(float32(width) / float32(height))
		}
		refresh()
	}
	win.Subscribe(window.OnWindowSize, onResize)
	if uiEngine != nil {
		win.Subscribe(window.OnCursor, func(_ string, event interface{}) {
			cursor := event.(*window.CursorEvent)
			if uiEngine.HandleCursor(float64(cursor.Xpos), float64(cursor.Ypos)) {
				refresh()
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
			if uiEngine.HandleKey(key.Key) {
				refresh()
			}
		})
	}
	onResize("", nil)
	gl.ClearColor(.04, .06, .1, 1)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	for !win.ShouldClose() {
		select {
		case <-signals:
			win.SetShouldClose(true)
		default:
		}
		if uiEngine != nil {
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
					uiEngine.SetGlueState(glueState(result.session))
				}
				refresh()
			default:
			}
		}
		gl.Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
		_ = r.Render(scene, cam)
		win.SwapBuffers()
		win.PollEvents()
	}
	if activeSession != nil {
		_ = activeSession.Close()
	}
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

func glueState(session *network.Session) ui.GlueState {
	state := ui.GlueState{Connected: true, ServerName: session.Realm.Name, SelectedRealm: int(session.Realm.ID)}
	state.Realms = []ui.RealmInfo{{Name: session.Realm.Name, Address: session.Realm.Address, RealmType: strconv.Itoa(int(session.Realm.Kind))}}
	state.Characters = make([]ui.CharacterEntry, 0, len(session.Characters))
	for _, character := range session.Characters {
		state.Characters = append(state.Characters, ui.CharacterEntry{Name: character.Name, Race: raceName(character.Race), Class: className(character.Class), Gender: int(character.Gender), Level: int(character.Level), Zone: strconv.Itoa(int(character.Zone))})
	}
	return state
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
