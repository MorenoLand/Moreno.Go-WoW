package ui

import (
	"sort"
	"strings"

	"github.com/g3n/engine/window"
	lua "github.com/yuin/gopher-lua"
)

// bindingDef is one command declared by a Bindings.xml: the commands the
// key-binding frame lists and the Lua that runs when its key is pressed.
type bindingDef struct {
	name    string
	header  string
	runOnUp bool
	body    string
}

// bindingTable holds the declared commands and which keys run them.
type bindingTable struct {
	defs    []*bindingDef
	byName  map[string]*bindingDef
	keys    map[string]string // key combination -> command
	pressed map[window.Key]string
	// movementKeys remembers which movement command each held key started.
	movementKeys map[window.Key]string
}

func newBindingTable() *bindingTable {
	return &bindingTable{byName: make(map[string]*bindingDef), keys: make(map[string]string), pressed: make(map[window.Key]string)}
}

// loadBindingsXML adds the commands of a Bindings.xml.
func (table *bindingTable) loadBindingsXML(data []byte) error {
	root, err := parseXML(data)
	if err != nil {
		return err
	}
	for _, node := range root.children {
		if !strings.EqualFold(node.name, "Binding") {
			continue
		}
		name := node.attrDefault("name", "")
		if name == "" {
			continue
		}
		def := &bindingDef{
			name:    name,
			header:  node.attrDefault("header", ""),
			runOnUp: parseBool(node.attrDefault("runOnUp", "false"), false),
			body:    strings.TrimSpace(node.text.String()),
		}
		if existing, ok := table.byName[name]; ok {
			*existing = *def
			continue
		}
		table.defs = append(table.defs, def)
		table.byName[name] = def
	}
	return nil
}

// loadDefaultKeys reads the "bind KEY COMMAND" lines of DefaultBindings.wtf.
func (table *bindingTable) loadDefaultKeys(data []byte) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 || !strings.EqualFold(fields[0], "bind") {
			continue
		}
		table.keys[strings.ToUpper(fields[1])] = fields[2]
	}
}

// keysFor lists the keys bound to a command in a stable order.
func (table *bindingTable) keysFor(command string) []string {
	var keys []string
	for key, bound := range table.keys {
		if bound == command {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// bindingKeyName is the name a key goes by in a binding, or "" when keys of
// that kind are not bindable.
func bindingKeyName(key window.Key) string {
	switch {
	case key >= window.KeyA && key <= window.KeyZ:
		return string(rune('A' + (key - window.KeyA)))
	case key >= window.Key0 && key <= window.Key9:
		return string(rune('0' + (key - window.Key0)))
	case key >= window.KeyF1 && key <= window.KeyF12:
		return "F" + itoa(int(key-window.KeyF1)+1)
	case key >= window.KeyKP0 && key <= window.KeyKP9:
		return "NUMPAD" + itoa(int(key-window.KeyKP0))
	}
	switch key {
	case window.KeySpace:
		return "SPACE"
	case window.KeyEscape:
		return "ESCAPE"
	case window.KeyEnter, window.KeyKPEnter:
		return "ENTER"
	case window.KeyTab:
		return "TAB"
	case window.KeyBackspace:
		return "BACKSPACE"
	case window.KeyDelete:
		return "DELETE"
	case window.KeyInsert:
		return "INSERT"
	case window.KeyHome:
		return "HOME"
	case window.KeyEnd:
		return "END"
	case window.KeyPageUp:
		return "PAGEUP"
	case window.KeyPageDown:
		return "PAGEDOWN"
	case window.KeyUp:
		return "UP"
	case window.KeyDown:
		return "DOWN"
	case window.KeyLeft:
		return "LEFT"
	case window.KeyRight:
		return "RIGHT"
	case window.KeyMinus:
		return "-"
	case window.KeyEqual:
		return "="
	case window.KeySlash:
		return "/"
	case window.KeyBackslash:
		return "\\"
	case window.KeyComma:
		return ","
	case window.KeyPeriod:
		return "."
	case window.KeySemicolon:
		return ";"
	case window.KeyApostrophe:
		return "'"
	case window.KeyLeftBracket:
		return "["
	case window.KeyRightBracket:
		return "]"
	case window.KeyGraveAccent:
		return "`"
	case window.KeyNumLock:
		return "NUMLOCK"
	case window.KeyKPDivide:
		return "NUMPADDIVIDE"
	case window.KeyKPMultiply:
		return "NUMPADMULTIPLY"
	case window.KeyKPSubtract:
		return "NUMPADMINUS"
	case window.KeyKPAdd:
		return "NUMPADPLUS"
	case window.KeyKPDecimal:
		return "NUMPADDECIMAL"
	case window.KeyPrintScreen:
		return "PRINTSCREEN"
	}
	return ""
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for ; value > 0; value /= 10 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
	}
	return string(digits)
}

// bindingKeyCombo is the key as a binding names it: modifiers first, in the
// order the client writes them (ALT-CTRL-SHIFT-), then the key.
func bindingKeyCombo(key window.Key, mods window.ModifierKey) string {
	name := bindingKeyName(key)
	if name == "" {
		return ""
	}
	prefix := ""
	if mods&window.ModAlt != 0 {
		prefix += "ALT-"
	}
	if mods&window.ModControl != 0 {
		prefix += "CTRL-"
	}
	if mods&window.ModShift != 0 {
		prefix += "SHIFT-"
	}
	return prefix + name
}

// loadBindings reads the commands and default keys from the game data.
func (eng *UIEngine) loadBindings() {
	if eng.Rt.bindings != nil {
		return
	}
	table := newBindingTable()
	if data, err := eng.AssetLoader.ReadFile(`Interface\FrameXML\Bindings.xml`); err == nil {
		_ = table.loadBindingsXML(data)
	}
	if data, err := eng.AssetLoader.ReadFile(`WTF\DefaultBindings.wtf`); err == nil {
		table.loadDefaultKeys(data)
	}
	eng.Rt.bindings = table
}

// runBindingCommand runs the Lua of a command with keystate set, the way the
// client runs a binding.
func (rt *Runtime) runBindingCommand(def *bindingDef, keystate string) bool {
	if def == nil || def.body == "" {
		return false
	}
	if keystate == "up" && !def.runOnUp {
		return false
	}
	rt.L.SetGlobal("keystate", lua.LString(keystate))
	return rt.Execute(def.body, "@binding-"+def.name+".lua")
}

// handleBindingKey runs the command bound to a key press. Commands that run
// on key release remember the press so the release finds the same command.
func (eng *UIEngine) handleBindingKey(key window.Key, mods window.ModifierKey) bool {
	table := eng.Rt.bindings
	if table == nil {
		return false
	}
	combo := bindingKeyCombo(key, mods)
	if combo == "" {
		return false
	}
	command, ok := table.keys[combo]
	if !ok {
		return false
	}
	def := table.byName[command]
	if def == nil {
		return false
	}
	if def.runOnUp {
		table.pressed[key] = command
	}
	return eng.Rt.runBindingCommand(def, "down")
}

// handleBindingKeyUp finishes a command that runs on key release.
func (eng *UIEngine) handleBindingKeyUp(key window.Key) bool {
	table := eng.Rt.bindings
	if table == nil {
		return false
	}
	command, ok := table.pressed[key]
	if !ok {
		return false
	}
	delete(table.pressed, key)
	return eng.Rt.runBindingCommand(table.byName[command], "up")
}

// registerBindingAPI defines the C-API functions that read and edit bindings.
func registerBindingAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	// Entries are the headers and commands in declaration order: the binding
	// frame lists a header row before the first command of each group.
	type entry struct {
		header  string
		command *bindingDef
	}
	entries := func() []entry {
		if rt.bindings == nil {
			return nil
		}
		var list []entry
		for _, def := range rt.bindings.defs {
			if def.header != "" {
				list = append(list, entry{header: def.header})
			}
			list = append(list, entry{command: def})
		}
		return list
	}
	reg("GetNumBindings", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(entries())))
		return 1
	})
	reg("GetBinding", func(L *lua.LState) int {
		list := entries()
		index := L.CheckInt(1)
		if index < 1 || index > len(list) {
			return 0
		}
		item := list[index-1]
		if item.command == nil {
			L.Push(lua.LString("HEADER_" + item.header))
			L.Push(lua.LString(item.header))
			return 2
		}
		L.Push(lua.LString(item.command.name))
		L.Push(lua.LString(item.command.header))
		count := 2
		for _, key := range rt.bindings.keysFor(item.command.name) {
			L.Push(lua.LString(key))
			count++
		}
		return count
	})
	reg("GetBindingKey", func(L *lua.LState) int {
		if rt.bindings == nil {
			return 0
		}
		keys := rt.bindings.keysFor(L.CheckString(1))
		for _, key := range keys {
			L.Push(lua.LString(key))
		}
		return len(keys)
	})
	reg("GetBindingAction", func(L *lua.LState) int {
		if rt.bindings != nil {
			L.Push(lua.LString(rt.bindings.keys[strings.ToUpper(L.CheckString(1))]))
		} else {
			L.Push(lua.LString(""))
		}
		return 1
	})
	reg("GetBindingByKey", func(L *lua.LState) int {
		if rt.bindings != nil {
			L.Push(lua.LString(rt.bindings.keys[strings.ToUpper(L.CheckString(1))]))
		} else {
			L.Push(lua.LString(""))
		}
		return 1
	})
	reg("SetBinding", func(L *lua.LState) int {
		if rt.bindings == nil {
			L.Push(lua.LFalse)
			return 1
		}
		key := strings.ToUpper(L.CheckString(1))
		if L.Get(2) == lua.LNil {
			delete(rt.bindings.keys, key)
		} else {
			rt.bindings.keys[key] = L.CheckString(2)
		}
		rt.FireEvent("UPDATE_BINDINGS")
		L.Push(lua.LTrue)
		return 1
	})
	reg("RunBinding", func(L *lua.LState) int {
		if rt.bindings == nil {
			return 0
		}
		state := "down"
		if strings.EqualFold(L.OptString(2, ""), "up") {
			state = "up"
		}
		rt.runBindingCommand(rt.bindings.byName[L.CheckString(1)], state)
		return 0
	})
	reg("SaveBindings", func(L *lua.LState) int { return 0 })
	reg("LoadBindings", func(L *lua.LState) int { return 0 })
	reg("GetCurrentBindingSet", func(L *lua.LState) int { L.Push(lua.LNumber(1)); return 1 })
	reg("GetBindingFromClick", func(L *lua.LState) int {
		button := strings.ToUpper(L.CheckString(1))
		button = strings.TrimSuffix(button, "BUTTON")
		if rt.bindings != nil {
			var buttonName string
			switch button {
			case "LEFT":
				buttonName = "BUTTON1"
			case "RIGHT":
				buttonName = "BUTTON2"
			case "MIDDLE":
				buttonName = "BUTTON3"
			default:
				buttonName = button
			}
			L.Push(lua.LString(rt.bindings.keys[buttonName]))
		} else {
			L.Push(lua.LString(""))
		}
		return 1
	})
}

// movementCommands are the bindings that move the character. The movement
// controller handles them itself, so the binding dispatcher leaves them alone.
var movementCommands = map[string]bool{
	"MOVEFORWARD": true, "MOVEBACKWARD": true, "TURNLEFT": true, "TURNRIGHT": true, "STRAFELEFT": true, "STRAFERIGHT": true,
	"JUMP": true, "SITORSTAND": true, "PITCHUP": true, "PITCHDOWN": true, "TOGGLEAUTORUN": true, "TOGGLERUN": true,
}

// MovementCommandDown returns the movement command a key press is bound to,
// remembering it for the release. It returns "" when the key does something
// else or when text is being typed.
func (eng *UIEngine) MovementCommandDown(key window.Key, mods window.ModifierKey) string {
	table := eng.Rt.bindings
	if table == nil || eng.Rt.focused != nil {
		return ""
	}
	combo := bindingKeyCombo(key, mods)
	command := table.keys[combo]
	if !movementCommands[command] {
		return ""
	}
	if table.movementKeys == nil {
		table.movementKeys = make(map[window.Key]string)
	}
	table.movementKeys[key] = command
	return command
}

// MovementCommandUp returns the movement command a released key started.
func (eng *UIEngine) MovementCommandUp(key window.Key) string {
	table := eng.Rt.bindings
	if table == nil {
		return ""
	}
	command, ok := table.movementKeys[key]
	if !ok {
		return ""
	}
	delete(table.movementKeys, key)
	return command
}
