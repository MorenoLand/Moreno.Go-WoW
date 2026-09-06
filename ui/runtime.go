package ui

import (
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// ScriptError records one Lua load or runtime failure with the chunk name it
// occurred in, mirroring what the original client writes to its glue log.
type ScriptError struct {
	Source  string
	Message string
}

func (e ScriptError) Error() string { return e.Source + ": " + e.Message }

// Runtime owns the Lua state, widget registry, virtual template registry,
// cvar store, and glue event dispatch for one interface session.
type Runtime struct {
	L *lua.LState

	widgets           map[string]*widget
	virtuals          map[string]*xmlNode
	fonts             map[string]*Font
	units             map[string]*UnitInfo
	events            map[string][]*widget
	cvars             map[string]string
	cvarDefaults      map[string]string
	scriptErrors      []ScriptError
	chunkSource       string
	nested            int
	focused           *widget
	cursorX           float64
	cursorY           float64
	createFacing      float32
	selectedRace      int
	selectedSex       int
	selectedClass     int
	addonVersionCheck bool

	// Glue carries the connection-flow state surfaced by the realm and
	// character list API functions.
	Glue GlueState

	// scriptErrorHandler is the function scripts install through
	// seterrorhandler; the client invokes it for script errors.
	scriptErrorHandler *lua.LFunction

	// instantiateTemplate applies a named virtual template to a widget
	// created through CreateFrame; the loader installs it.
	instantiateTemplate func(w *widget, template string)

	// measureText updates FontString width/height from the current text when
	// auto-sizing is enabled. The UI engine installs this so GetWidth/GetHeight
	// match the original client's immediate post-SetText metrics.
	measureText func(w *widget)

	// validateMovie probes a cinematic path the way native StartMovie does.
	validateMovie func(file string, volume float64) bool

	traceInvocations []string

	// Host hooks supply client state that lives outside the interface
	// runtime: screen size, audio, login actions, realm and character
	// data. Nil hooks behave as an idle, unconnected client.
	Host Host

	// logoutPending / quitPending mirror the original client's camping and
	// quitting timers started by Logout() / Quit(). remaining counts down in
	// UIEngine.Update; zero completes the host Logout or Quit.
	logoutPending   bool
	quitPending     bool
	logoutRemaining float64

	// The original client aborts runaway scripts through a Lua debug hook
	// with a 110-instruction budget. The Go VM in use exposes no debug-hook
	// API, and its context deadline corrupts state on error unwinding, so
	// no instruction budget is enforced yet.
}

// Host is implemented by the client shell to back glue API functions that
// touch audio, the platform, or connection state.
type Host interface {
	ScreenSize() (width, height float64)
	PlaySound(id string)
	PlayMusic(name string)
	PlayAmbience(name string)
	StopMusic()
	StopAmbience()
	StopAllSFX()
	LaunchURL(url string)
	Quit(runLauncher bool)
	ConsoleExec(command string)
	Screenshot()
}

type LoginHost interface {
	DefaultServerLogin(account, password string)
}

type WorldHost interface {
	EnterWorld(index int)
}

// LogoutHost returns the client from the world to the character-select glue
// flow after Logout / ForceLogout completes.
type LogoutHost interface {
	Logout()
}

type AudioHost interface {
	SetAudioCVar(name, value string)
}

type MovieAudioHost interface {
	PlayMovieAudio(data []byte, sampleRate, channels int, volume float64)
	StopMovieAudio()
}

// Font is a named font object created from a <Font> element.
type Font struct {
	Name          string
	FontFile      string
	Height        float64
	Color         rgba
	Outline       string
	Shadow        bool
	ShadowColor   rgba
	ShadowOffsetX float64
	ShadowOffsetY float64
	JustifyH      string
	JustifyV      string
}

// NewRuntime creates the Lua state and registers the glue API and widget
// method tables. Callers must Close it when done.
func NewRuntime(host Host) *Runtime {
	rt := &Runtime{
		L:                 lua.NewState(lua.Options{SkipOpenLibs: false}),
		widgets:           make(map[string]*widget),
		virtuals:          make(map[string]*xmlNode),
		fonts:             make(map[string]*Font),
		units:             make(map[string]*UnitInfo),
		events:            make(map[string][]*widget),
		cvars:             make(map[string]string),
		cvarDefaults:      make(map[string]string),
		selectedRace:      1,
		selectedSex:       2,
		selectedClass:     1,
		addonVersionCheck: true,
		Host:              host,
	}
	registerWidgetMethods(rt.L, rt)
	registerGlueAPI(rt)
	registerUnitAPI(rt)
	registerStringHelpers(rt.L)
	return rt
}

// Close releases the Lua state.
func (rt *Runtime) Close() { rt.L.Close() }

func (rt *Runtime) setFocus(w *widget) {
	if w != nil && (!w.shown || !w.enabled) {
		return
	}
	if rt.focused == w {
		return
	}
	old := rt.focused
	rt.focused = w
	if old != nil {
		rt.fireHandler(old, "OnEditFocusLost")
	}
	if w != nil {
		w.cursor = len([]rune(w.text))
		w.selectionStart = w.cursor
		w.selectionEnd = w.cursor
		w.selectionAnchor = w.cursor
		rt.fireHandler(w, "OnEditFocusGained")
	}
}

func (rt *Runtime) setText(w *widget, text string) {
	if w.kind == kindEditBox {
		text = strings.ReplaceAll(text, "\n", "")
	}
	if w.text == text {
		w.cursor = len([]rune(text))
		w.selectionStart = w.cursor
		w.selectionEnd = w.cursor
		w.selectionAnchor = w.cursor
		return
	}
	w.text = text
	if w.buttonLabel != nil {
		w.buttonLabel.text = text
		if rt.measureText != nil {
			rt.measureText(w.buttonLabel)
		}
	}
	if w.kind == kindFontString && rt.measureText != nil {
		rt.measureText(w)
	}
	w.cursor = len([]rune(text))
	w.selectionStart = w.cursor
	w.selectionEnd = w.cursor
	w.selectionAnchor = w.cursor
	rt.fire(w, "OnTextChanged", []lua.LValue{w.luaValue(rt.L), lua.LBool(true)})
}

// ScriptErrors returns the script failures recorded so far.
func (rt *Runtime) ScriptErrors() []ScriptError { return rt.scriptErrors }

func (rt *Runtime) recordScriptError(source, message string) {
	rt.scriptErrors = append(rt.scriptErrors, ScriptError{Source: source, Message: message})
}

// lookup returns a registered widget by name (getglobal equivalent backing).
func (rt *Runtime) lookup(name string) *widget { return rt.widgets[name] }

func (rt *Runtime) register(w *widget) {
	if w.name != "" {
		rt.widgets[w.name] = w
		if w.luaObj == nil {
			rt.L.SetGlobal(w.name, w.luaValue(rt.L))
		} else {
			rt.L.SetGlobal(w.name, w.luaObj)
		}
	}
}

// Execute runs Lua source with the given chunk name, recording errors the
// way the original client logs them. Inline XML bodies pass their enclosing
// file as the chunk source for error attribution.
func (rt *Runtime) Execute(source, chunkName string) bool {
	rt.nested++
	outer := rt.chunkSource
	if rt.nested == 1 {
		rt.chunkSource = chunkName
	}
	if err := rt.doChunk(source, chunkName); err != nil {
		rt.recordScriptError(chunkName, err.Error())
		rt.nested--
		rt.chunkSource = outer
		return false
	}
	rt.nested--
	if rt.nested == 0 {
		rt.chunkSource = outer
	}
	return true
}

func (rt *Runtime) doChunk(source, chunkName string) error {
	top := rt.L.GetTop()
	defer rt.L.SetTop(top)
	fn, err := rt.L.Load(strings.NewReader(source), chunkName)
	if err != nil {
		return err
	}
	rt.L.Push(fn)
	if err := rt.L.PCall(0, lua.MultRet, rt.errorHandler()); err != nil {
		return err
	}
	return nil
}

// errorHandler builds the function the runtime installs for pcall, matching
// the original client's registry error handler that reports source and line.
func (rt *Runtime) errorHandler() *lua.LFunction {
	return rt.L.NewFunction(func(L *lua.LState) int {
		msg := L.Get(1)
		source := rt.chunkSource
		if source == "" {
			source = "?"
		}
		L.Push(lua.LString(source + ": " + msg.String()))
		return 1
	})
}

// doFileBody executes file content with the file-chunk convention:
// the chunk name is "@" plus the interface path, so error positions carry
// the file they came from.
func (rt *Runtime) doFileBody(source, interfacePath string) bool {
	chunk := "@" + interfacePath
	return rt.Execute(source, chunk)
}

// fireHandler invokes a widget script handler if set, with only the widget
// argument (used for OnShow/OnHide style handlers).
func (rt *Runtime) fireHandler(w *widget, handler string) {
	if fn, ok := w.scripts[handler]; ok {
		rt.traceInvocations = append(rt.traceInvocations, w.name+"|"+handler)
		top := rt.L.GetTop()
		defer rt.L.SetTop(top)
		// Handlers compiled from older interface code address their frame
		// through the legacy implicit `this` global.
		prevThis := rt.L.GetGlobal("this")
		rt.L.SetGlobal("this", w.luaValue(rt.L))
		rt.L.Push(fn)
		rt.L.Push(w.luaValue(rt.L))
		if err := rt.L.PCall(1, 0, nil); err != nil {
			rt.recordScriptError(w.name+"/"+handler, err.Error())
		}
		rt.L.SetGlobal("this", prevThis)
	}
}

// TraceInvocations records handler dispatches for diagnostics.
func (rt *Runtime) TraceInvocations() []string { return rt.traceInvocations }

// fire invokes a widget handler with explicit arguments.
func (rt *Runtime) fire(w *widget, handler string, args []lua.LValue) {
	fn, ok := w.scripts[handler]
	if !ok {
		return
	}
	top := rt.L.GetTop()
	defer rt.L.SetTop(top)
	prevThis := rt.L.GetGlobal("this")
	rt.L.SetGlobal("this", w.luaValue(rt.L))
	rt.L.Push(fn)
	for _, a := range args {
		rt.L.Push(a)
	}
	if err := rt.L.PCall(len(args), 0, nil); err != nil {
		rt.recordScriptError(w.name+"/"+handler, err.Error())
	}
	rt.L.SetGlobal("this", prevThis)
}

// registerEventWidget subscribes a widget to a glue event by name.
func (rt *Runtime) registerEventWidget(event string, w *widget) {
	w.events[event] = true
	for _, existing := range rt.events[event] {
		if existing == w {
			return
		}
	}
	rt.events[event] = append(rt.events[event], w)
}

func (rt *Runtime) unregisterEventWidget(event string, w *widget) {
	delete(w.events, event)
	kept := rt.events[event][:0]
	for _, existing := range rt.events[event] {
		if existing != w {
			kept = append(kept, existing)
		}
	}
	rt.events[event] = kept
}

// FireEvent dispatches a glue event to every subscribed widget, passing the
// event name and payload after the widget argument, matching the OnEvent
// calling convention the interface scripts expect.
func (rt *Runtime) FireEvent(event string, payload ...lua.LValue) int {
	count := 0
	for _, w := range append([]*widget(nil), rt.events[event]...) {
		args := make([]lua.LValue, 0, len(payload)+2)
		args = append(args, w.luaValue(rt.L), lua.LString(event))
		args = append(args, payload...)
		rt.fire(w, "OnEvent", args)
		count++
	}
	return count
}

// GetCVar reads a cvar through the same case-insensitive lookup the client
// uses for console variables.
func (rt *Runtime) GetCVar(name string) (string, bool) {
	v, ok := rt.cvars[strings.ToLower(name)]
	return v, ok
}

func (rt *Runtime) SetCVar(name, value string) {
	rt.cvars[strings.ToLower(name)] = value
}

// SetCVarDefault registers the default value returned by GetCVarDefault.
func (rt *Runtime) SetCVarDefault(name, value string) {
	rt.cvarDefaults[strings.ToLower(name)] = value
}

var _ = fmt.Sprintf

// sprintf formats using the Lua string.format-compatible subset of Go
// verbs used by the interface scripts (%s, %d, %f, and %.Nf).
func sprintf(format string, args []interface{}) string {
	return fmt.Sprintf(format, args...)
}
