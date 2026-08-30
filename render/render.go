package render

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"unicode/utf8"

	"github.com/MorenoLand/Moreno.WoW/auth"
	"github.com/MorenoLand/Moreno.WoW/config"
	"github.com/MorenoLand/Moreno.WoW/network"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/renderer"
	"github.com/g3n/engine/window"
	"github.com/sqweek/dialog"
)

type loginStage uint8

const (
	stageSettings loginStage = iota
	stageCredentials
	stageWorking
	stageRealms
	stageCharacters
)

type authCompletion struct {
	authenticated network.Authenticated
	err           error
}

type realmCompletion struct {
	session *network.Session
	err     error
}

type entryResult struct {
	position world.WorldPosition
	err      error
}

func Run(clientConfig network.Config, dataPath, lastCharacter, configPath string, debug bool) {
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
	cam := camera.New(1)
	cam.SetPosition(0, 0, 3)
	scene.Add(cam)
	onResize := func(name string, event interface{}) {
		width, height := win.GetSize()
		gl.Viewport(0, 0, int32(width), int32(height))
		if height > 0 {
			cam.SetAspect(float32(width) / float32(height))
		}
	}
	win.Subscribe(window.OnWindowSize, onResize)
	onResize("", nil)
	gl.ClearColor(.04, .06, .1, 1)
	accent := &math32.Color{R: .95, G: .78, B: .32}
	muted := &math32.Color{R: .72, G: .76, B: .84}
	addLabel(scene, "MorenoWoW", 80, 42, 28, accent)
	addLabel(scene, "3.3.5a client", 80, 82, 16, muted)
	dataLabel := addLabel(scene, "Data folder", 80, 136, 16, nil)
	localeLabel := addLabel(scene, "Locale", 80, 186, 16, nil)
	serverLabel := addLabel(scene, "Server", 80, 136, 16, nil)
	accountLabel := addLabel(scene, "Account", 80, 186, 16, nil)
	passwordLabel := addLabel(scene, "Password", 80, 236, 16, nil)
	dataEdit := gui.NewEdit(320, "WoW 3.3.5a Data folder")
	dataEdit.SetText(dataPath)
	dataEdit.SetPosition(210, 130)
	scene.Add(dataEdit)
	localeEdit := gui.NewEdit(320, "enUS")
	localeEdit.SetText(clientConfig.Locale)
	localeEdit.SetPosition(210, 180)
	scene.Add(localeEdit)
	serverEdit := gui.NewEdit(320, "auth host:port")
	serverEdit.SetText(clientConfig.AuthAddress)
	serverEdit.SetPosition(210, 130)
	scene.Add(serverEdit)
	accountEdit := gui.NewEdit(320, "account name")
	accountEdit.SetText(clientConfig.Account)
	accountEdit.SetPosition(210, 180)
	scene.Add(accountEdit)
	passwordEdit, passwordCover := newPasswordEdit(scene, clientConfig.Password, 210, 230)
	browseButton := gui.NewButton("Browse...")
	browseButton.SetPosition(540, 130)
	browseButton.SetSize(120, 32)
	scene.Add(browseButton)
	continueButton := gui.NewButton("Continue")
	continueButton.SetPosition(210, 230)
	continueButton.SetSize(140, 36)
	scene.Add(continueButton)
	loginButton := gui.NewButton("Sign In")
	loginButton.SetPosition(210, 280)
	loginButton.SetSize(140, 36)
	scene.Add(loginButton)
	settingsButton := gui.NewButton("Settings")
	settingsButton.SetPosition(365, 280)
	settingsButton.SetSize(140, 36)
	scene.Add(settingsButton)
	status := addLabel(scene, "", 80, 340, 16, muted)
	stageTitle := addLabel(scene, "", 590, 92, 22, accent)
	realmLabel := addLabel(scene, "", 590, 126, 16, muted)
	backButton := gui.NewButton("Back")
	backButton.SetPosition(590, 550)
	backButton.SetSize(120, 34)
	scene.Add(backButton)
	dataLabel.SetVisible(false)
	localeLabel.SetVisible(false)
	dataEdit.SetVisible(false)
	localeEdit.SetVisible(false)
	browseButton.SetVisible(false)
	continueButton.SetVisible(false)
	serverLabel.SetVisible(false)
	accountLabel.SetVisible(false)
	passwordLabel.SetVisible(false)
	serverEdit.SetVisible(false)
	accountEdit.SetVisible(false)
	passwordEdit.SetVisible(false)
	passwordCover.SetVisible(false)
	loginButton.SetVisible(false)
	settingsButton.SetVisible(false)
	backButton.SetVisible(false)
	resultChannel := make(chan authCompletion, 1)
	errorChannel := make(chan error, 1)
	realmChannel := make(chan realmCompletion, 1)
	entryChannel := make(chan entryResult, 1)
	var stage loginStage
	var authenticated *network.Authenticated
	var session *network.Session
	var realmRows []*gui.Button
	var characterRows []*gui.Button
	selectedRealm := strings.TrimSpace(clientConfig.Realm)
	selectedCharacter := strings.TrimSpace(lastCharacter)
	pending := false
	entering := false
	saveSettings := func() error {
		return config.Save(configPath, config.Options{AuthAddress: strings.TrimSpace(serverEdit.Text()), Account: strings.TrimSpace(accountEdit.Text()), Locale: strings.TrimSpace(localeEdit.Text()), Realm: selectedRealm, Character: selectedCharacter, DataPath: strings.TrimSpace(dataEdit.Text())})
	}
	setStage := func(next loginStage) {
		stage = next
		settingsVisible := next == stageSettings
		credentialsVisible := next == stageCredentials
		workingVisible := next == stageWorking
		listVisible := next == stageRealms || next == stageCharacters
		for _, panel := range []gui.IPanel{dataLabel, localeLabel, dataEdit, localeEdit, browseButton, continueButton} {
			setVisible(panel, settingsVisible)
		}
		for _, panel := range []gui.IPanel{serverLabel, accountLabel, passwordLabel, serverEdit, accountEdit, passwordEdit, passwordCover, loginButton, settingsButton} {
			setVisible(panel, credentialsVisible)
		}
		setVisible(status, true)
		setVisible(stageTitle, true)
		setVisible(realmLabel, listVisible)
		setVisible(backButton, listVisible || (settingsVisible && dataDirectoryExists(dataEdit.Text())))
		for _, row := range realmRows {
			row.SetVisible(next == stageRealms)
		}
		for _, row := range characterRows {
			row.SetVisible(next == stageCharacters)
		}
		switch next {
		case stageSettings:
			stageTitle.SetText("Game Data")
		case stageCredentials:
			stageTitle.SetText("Sign In")
		case stageWorking:
			stageTitle.SetText("Working...")
		case stageRealms:
			stageTitle.SetText("Choose Realm")
		case stageCharacters:
			stageTitle.SetText("Choose Character")
		}
		if workingVisible {
			loginButton.SetEnabled(false)
		}
		if credentialsVisible {
			loginButton.SetEnabled(true)
		}
	}
	beginRealm := func(login network.Authenticated, realm auth.Realm) {
		copy := login
		authenticated = &copy
		selectedRealm = realm.Name
		if err := saveSettings(); err != nil {
			status.SetText(fmt.Sprintf("Could not save options: %s", err))
			return
		}
		pending = true
		setStage(stageWorking)
		status.SetText(fmt.Sprintf("Entering %s...", realm.Name))
		if debug {
			log.Printf("world: opening realm %q at %s", realm.Name, realm.Address)
		}
		go func() {
			opened, err := network.OpenRealm(copy, strings.TrimSpace(accountEdit.Text()), realm, clientConfig.Timeout, debug)
			realmChannel <- realmCompletion{session: opened, err: err}
		}()
	}
	browseButton.Subscribe(gui.OnClick, func(name string, event interface{}) {
		start := strings.TrimSpace(dataEdit.Text())
		if !dataDirectoryExists(start) {
			start = "."
		}
		picked, err := dialog.Directory().Title("Choose the Data folder of a WoW 3.3.5a installation").SetStartDir(start).Browse()
		if err != nil {
			if !errors.Is(err, dialog.ErrCancelled) {
				status.SetText(fmt.Sprintf("Folder picker failed: %s", err))
			}
			return
		}
		dataEdit.SetText(picked)
		if err := saveSettings(); err != nil {
			status.SetText(fmt.Sprintf("Could not save options: %s", err))
		} else {
			status.SetText("Data folder selected.")
		}
	})
	continueButton.Subscribe(gui.OnClick, func(name string, event interface{}) {
		if !dataDirectoryExists(dataEdit.Text()) {
			status.SetText("Choose a valid Data folder first.")
			return
		}
		if err := saveSettings(); err != nil {
			status.SetText(fmt.Sprintf("Could not save options: %s", err))
			return
		}
		setStage(stageCredentials)
		status.SetText("Enter your credentials to begin.")
	})
	settingsButton.Subscribe(gui.OnClick, func(name string, event interface{}) {
		setStage(stageSettings)
		status.SetText("Choose the game data folder or update the locale.")
	})
	backButton.Subscribe(gui.OnClick, func(name string, event interface{}) {
		switch stage {
		case stageSettings:
			setStage(stageCredentials)
			status.SetText("Enter your credentials to begin.")
		case stageRealms, stageCharacters:
			if session != nil {
				_ = session.Close()
				session = nil
			}
			authenticated = nil
			setStage(stageCredentials)
			status.SetText("Enter your credentials to begin.")
		}
	})
	loginButton.Subscribe(gui.OnClick, func(name string, event interface{}) {
		if pending {
			return
		}
		if !dataDirectoryExists(dataEdit.Text()) {
			setStage(stageSettings)
			status.SetText("Choose a valid Data folder first.")
			return
		}
		account := strings.TrimSpace(accountEdit.Text())
		password := passwordEdit.Text()
		server := strings.TrimSpace(serverEdit.Text())
		locale := strings.TrimSpace(localeEdit.Text())
		if server == "" || account == "" || password == "" || locale == "" {
			status.SetText("Server, account, password, and locale are required.")
			return
		}
		if session != nil {
			_ = session.Close()
			session = nil
		}
		if err := saveSettings(); err != nil {
			status.SetText(fmt.Sprintf("Could not save options: %s", err))
			return
		}
		request := network.Config{AuthAddress: server, Account: account, Password: password, Locale: locale, Timeout: clientConfig.Timeout, Debug: debug}
		pending = true
		setStage(stageWorking)
		status.SetText(fmt.Sprintf("Signing in to %s...", server))
		if debug {
			log.Printf("auth: sign-in requested for %q", account)
		}
		go func() {
			result, err := network.Authenticate(request)
			resultChannel <- authCompletion{authenticated: result, err: err}
		}()
	})
	if dataDirectoryExists(dataEdit.Text()) {
		setStage(stageCredentials)
		status.SetText("Enter your credentials to begin.")
	} else {
		setStage(stageSettings)
		status.SetText("Choose the Data folder of a WoW 3.3.5a installation.")
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	for !win.ShouldClose() {
		select {
		case <-signals:
			if debug {
				log.Print("shutdown: Ctrl+C received")
			}
			win.SetShouldClose(true)
		case result := <-resultChannel:
			pending = false
			if result.err != nil {
				setStage(stageCredentials)
				status.SetText(fmt.Sprintf("Login failed: %s", result.err))
				break
			}
			copy := result.authenticated
			authenticated = &copy
			if len(copy.Realms) == 0 {
				setStage(stageCredentials)
				status.SetText("The auth server offered no realms.")
				break
			}
			if len(copy.Realms) == 1 {
				beginRealm(copy, copy.Realms[0])
				break
			}
			setStage(stageRealms)
			status.SetText("Choose a realm.")
			realmLabel.SetText(fmt.Sprintf("%d realms available", len(copy.Realms)))
			for i, realm := range copy.Realms {
				choice := realm
				row := gui.NewButton(fmt.Sprintf("%s  (%s)", choice.Name, describeRealm(choice)))
				row.SetPosition(590, float32(170+i*42))
				row.SetSize(330, 34)
				row.Subscribe(gui.OnClick, func(name string, event interface{}) {
					if authenticated != nil && !pending {
						beginRealm(*authenticated, choice)
					}
				})
				scene.Add(row)
				realmRows = append(realmRows, row)
			}
		case result := <-realmChannel:
			pending = false
			if result.err != nil {
				setStage(stageCredentials)
				status.SetText(fmt.Sprintf("Realm failed: %s", result.err))
				break
			}
			session = result.session
			authenticated = nil
			setStage(stageCharacters)
			status.SetText(fmt.Sprintf("Connected to %s", session.Realm.Name))
			realmLabel.SetText(fmt.Sprintf("%d character(s) returned", len(session.Characters)))
			if len(session.Characters) == 0 {
				addLabel(scene, "No characters returned", 610, 170, 16, muted)
				break
			}
			for i, character := range session.Characters {
				index := i
				row := gui.NewButton(fmt.Sprintf("%s  Lv %d  %s %s", character.Name, character.Level, world.RaceName(character.Race), world.ClassName(character.Class)))
				row.SetPosition(590, float32(170+i*42))
				row.SetSize(330, 34)
				row.SetEnabled(!character.NeedsRename())
				row.Subscribe(gui.OnClick, func(name string, event interface{}) {
					if entering || session == nil {
						return
					}
					entering = true
					selectedCharacter = session.Characters[index].Name
					if err := saveSettings(); err != nil {
						status.SetText(fmt.Sprintf("Could not save options: %s", err))
						entering = false
						return
					}
					status.SetText(fmt.Sprintf("Entering %s...", selectedCharacter))
					if debug {
						log.Printf("world: entering character %q", selectedCharacter)
					}
					current := session
					go func() {
						position, err := current.EnterWorld(index)
						entryChannel <- entryResult{position: position, err: err}
					}()
				})
				scene.Add(row)
				characterRows = append(characterRows, row)
			}
		case result := <-entryChannel:
			entering = false
			if result.err != nil {
				status.SetText(fmt.Sprintf("Character login failed: %s", result.err))
				break
			}
			status.SetText(fmt.Sprintf("Entered map %d at %.2f, %.2f, %.2f", result.position.Map, result.position.X, result.position.Y, result.position.Z))
			if debug {
				log.Printf("world: entered map %d at %.2f, %.2f, %.2f", result.position.Map, result.position.X, result.position.Y, result.position.Z)
			}
		case err := <-errorChannel:
			pending = false
			setStage(stageCredentials)
			status.SetText(fmt.Sprintf("Login failed: %s", err))
		default:
		}
		gl.Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
		_ = r.Render(scene, cam)
		win.SwapBuffers()
		win.PollEvents()
	}
	if session != nil {
		_ = session.Close()
	}
}

func addLabel(scene *core.Node, text string, x, y float32, size float64, color *math32.Color) *gui.Label {
	label := gui.NewLabel(text)
	label.SetFontSize(size)
	label.SetPosition(x, y)
	if color != nil {
		label.SetColor(color)
	}
	scene.Add(label)
	return label
}

func newPasswordEdit(scene *core.Node, initial string, x, y float32) (*gui.Edit, *gui.Panel) {
	edit := gui.NewEdit(320, "Password")
	edit.SetText(initial)
	edit.SetPosition(x, y)
	edit.SetZLayerDelta(1)
	scene.Add(edit)
	cover := gui.NewPanel(320, 30)
	cover.SetPosition(x, y)
	cover.SetColor(&math32.Color{R: .12, G: .15, B: .21})
	cover.SetEnabled(false)
	scene.Add(cover)
	mask := gui.NewLabel("")
	mask.SetFontSize(16)
	mask.SetPosition(8, 4)
	mask.SetEnabled(false)
	mask.SetZLayerDelta(1)
	cover.Add(mask)
	placeholderColor := math32.Color{R: .48, G: .52, B: .6}
	textColor := math32.Color{R: .92, G: .94, B: 1}
	update := func() {
		value := edit.Text()
		if value == "" {
			mask.SetText("Password")
			mask.SetColor(&placeholderColor)
			return
		}
		mask.SetText(strings.Repeat("*", utf8.RuneCountInString(value)))
		mask.SetColor(&textColor)
	}
	edit.Subscribe(gui.OnChange, func(name string, event interface{}) { update() })
	update()
	return edit, cover
}

func setVisible(panel gui.IPanel, visible bool) { panel.GetPanel().SetVisible(visible) }

func dataDirectoryExists(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func describeRealm(realm auth.Realm) string {
	if realm.IsOffline() {
		return "offline"
	}
	if realm.Locked {
		return "locked"
	}
	return fmt.Sprintf("population %.2f", realm.Population)
}
