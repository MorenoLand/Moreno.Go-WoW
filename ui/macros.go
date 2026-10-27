package ui

import (
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Macro limits: the first MaxAccountMacros indices are the account's, the next
// MaxCharacterMacros the character's.
const (
	MaxAccountMacros   = 36
	MaxCharacterMacros = 18
	// actionTypeMacro is the action button type of a macro.
	actionTypeMacro = 64
)

// Macro is a written macro: its name, icon texture and text.
type Macro struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	Body string `json:"body"`
}

// MacroStore keeps the macros between runs.
type MacroStore interface {
	LoadMacros(character bool) []Macro
	SaveMacros(character bool, macros []Macro)
}

type macroState struct {
	account   []Macro
	character []Macro
	loaded    bool
}

func (rt *Runtime) macroStore() MacroStore {
	store, _ := rt.Host.(MacroStore)
	return store
}

// loadMacros reads the saved macros the first time they are wanted.
func (rt *Runtime) loadMacros() {
	state := &rt.macros
	if state.loaded {
		return
	}
	state.loaded = true
	if store := rt.macroStore(); store != nil {
		state.account, state.character = store.LoadMacros(false), store.LoadMacros(true)
	}
}

func (rt *Runtime) saveMacros() {
	if store := rt.macroStore(); store != nil {
		store.SaveMacros(false, rt.macros.account)
		store.SaveMacros(true, rt.macros.character)
	}
}

// macroAt finds a macro by its 1-based index over both lists.
func (rt *Runtime) macroAt(index int) (*Macro, bool) {
	rt.loadMacros()
	switch {
	case index >= 1 && index <= len(rt.macros.account):
		return &rt.macros.account[index-1], false
	case index > MaxAccountMacros && index-MaxAccountMacros <= len(rt.macros.character):
		return &rt.macros.character[index-MaxAccountMacros-1], true
	}
	return nil, false
}

// macroIndexByName finds the index of the macro with a name.
func (rt *Runtime) macroIndexByName(name string) int {
	rt.loadMacros()
	for index, macro := range rt.macros.account {
		if strings.EqualFold(macro.Name, name) {
			return index + 1
		}
	}
	for index, macro := range rt.macros.character {
		if strings.EqualFold(macro.Name, name) {
			return MaxAccountMacros + index + 1
		}
	}
	return 0
}

// macroArg is the macro an index or name argument stands for.
func (rt *Runtime) macroArg(L *lua.LState, position int) int {
	switch value := L.Get(position).(type) {
	case lua.LNumber:
		return int(value)
	case lua.LString:
		return rt.macroIndexByName(string(value))
	}
	return 0
}

// iconArg turns an icon argument (an index of the icon list or a texture) into
// the texture.
func (rt *Runtime) iconArg(L *lua.LState, position int) string {
	switch value := L.Get(position).(type) {
	case lua.LNumber:
		if path := rt.macroIconPath(int(value)); path != "" {
			return path
		}
	case lua.LString:
		return string(value)
	}
	return `Interface\Icons\INV_Misc_QuestionMark`
}

// macroText is the text a macro action button runs.
func (rt *Runtime) macroText(index int) string {
	if macro, _ := rt.macroAt(index); macro != nil {
		return macro.Body
	}
	return ""
}

// registerMacroAPI defines the calls that keep and run macros.
func registerMacroAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	state := &rt.macros
	reg("GetNumMacros", func(L *lua.LState) int {
		rt.loadMacros()
		L.Push(lua.LNumber(len(state.account)))
		L.Push(lua.LNumber(len(state.character)))
		return 2
	})
	reg("GetMacroInfo", func(L *lua.LState) int {
		macro, local := rt.macroAt(rt.macroArg(L, 1))
		if macro == nil {
			return 0
		}
		L.Push(lua.LString(macro.Name))
		L.Push(lua.LString(macro.Icon))
		L.Push(lua.LString(macro.Body))
		L.Push(lua.LBool(local))
		return 4
	})
	reg("GetMacroBody", func(L *lua.LState) int {
		if macro, _ := rt.macroAt(rt.macroArg(L, 1)); macro != nil {
			L.Push(lua.LString(macro.Body))
			return 1
		}
		return 0
	})
	reg("GetMacroIndexByName", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.macroIndexByName(L.CheckString(1))))
		return 1
	})
	reg("CreateMacro", func(L *lua.LState) int {
		rt.loadMacros()
		macro := Macro{Name: L.CheckString(1), Icon: rt.iconArg(L, 2), Body: L.OptString(3, "")}
		if L.OptBool(4, false) {
			if len(state.character) >= MaxCharacterMacros {
				L.RaiseError("You have too many character specific macros.")
				return 0
			}
			state.character = append(state.character, macro)
			L.Push(lua.LNumber(MaxAccountMacros + len(state.character)))
		} else {
			if len(state.account) >= MaxAccountMacros {
				L.RaiseError("You have too many account macros.")
				return 0
			}
			state.account = append(state.account, macro)
			L.Push(lua.LNumber(len(state.account)))
		}
		rt.saveMacros()
		return 1
	})
	reg("EditMacro", func(L *lua.LState) int {
		index := rt.macroArg(L, 1)
		macro, _ := rt.macroAt(index)
		if macro == nil {
			return 0
		}
		if name := L.OptString(2, ""); name != "" {
			macro.Name = name
		}
		if L.Get(3) != lua.LNil {
			macro.Icon = rt.iconArg(L, 3)
		}
		if L.Get(4) != lua.LNil {
			macro.Body = L.CheckString(4)
		}
		rt.saveMacros()
		L.Push(lua.LNumber(index))
		return 1
	})
	reg("DeleteMacro", func(L *lua.LState) int {
		index := rt.macroArg(L, 1)
		if macro, local := rt.macroAt(index); macro != nil {
			if local {
				at := index - MaxAccountMacros - 1
				state.character = append(state.character[:at:at], state.character[at+1:]...)
			} else {
				state.account = append(state.account[:index-1:index-1], state.account[index:]...)
			}
			rt.saveMacros()
		}
		return 0
	})
	reg("SaveMacros", func(L *lua.LState) int { rt.saveMacros(); return 0 })
	reg("PickupMacro", func(L *lua.LState) int {
		index := rt.macroArg(L, 1)
		if macro, _ := rt.macroAt(index); macro != nil {
			rt.gameplay.Cursor = world.ActionButton{ID: uint32(index), Type: actionTypeMacro}
			rt.FireEvent("CURSOR_UPDATE")
		}
		return 0
	})
	reg("StopMacro", func(L *lua.LState) int { return 0 })
	reg("RunMacro", func(L *lua.LState) int {
		if text := rt.macroText(rt.macroArg(L, 1)); text != "" {
			rt.runMacroText(text)
		}
		return 0
	})
	reg("RunMacroText", func(L *lua.LState) int {
		rt.runMacroText(L.CheckString(1))
		return 0
	})
}

// runMacroText runs a macro's lines: each one that starts with a slash goes
// through the chat window's command handling.
func (rt *Runtime) runMacroText(text string) {
	rt.L.SetGlobal("__macro_text", lua.LString(text))
	rt.Execute(macroRunner, "@macro.lua")
}

// macroRunner is the script that runs a macro: it keeps a hidden edit box of
// its own, so the typed chat line is left alone.
const macroRunner = `
local text = __macro_text
__macro_text = nil
local box = MacroRunnerEditBox
if not box then
	box = CreateFrame("EditBox", "MacroRunnerEditBox", UIParent)
	box:Hide()
	box:SetAutoFocus(false)
	box.chatType = "SAY"
	box.chatFrame = DEFAULT_CHAT_FRAME
end
for line in string.gmatch(text .. "\n", "([^\n]*)\n") do
	line = string.gsub(line, "\r", "")
	if string.sub(line, 1, 1) == "/" then
		local command, rest = string.match(line, "^(/%S+)%s*(.*)$")
		local parsed = SecureCmdOptionParse and SecureCmdOptionParse(rest) or rest
		if parsed ~= nil then
			box:SetText(command .. " " .. parsed)
			ChatEdit_ParseText(box, 1)
		end
	end
end
`
