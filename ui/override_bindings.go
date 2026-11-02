package ui

import (
	"strings"

	"github.com/g3n/engine/window"
	lua "github.com/yuin/gopher-lua"
)

// overrideBinding is a key bound by a frame for a while: the bar of a vehicle,
// a spell an addon puts on a key.
type overrideBinding struct {
	owner    lua.LValue
	priority bool
	key      string
	command  string
}

type overrideBindings struct {
	list []overrideBinding
}

func (o *overrideBindings) set(owner lua.LValue, priority bool, key, command string) {
	key = strings.ToUpper(key)
	for index, entry := range o.list {
		if entry.owner == owner && entry.key == key {
			if command == "" {
				o.list = append(o.list[:index:index], o.list[index+1:]...)
				return
			}
			o.list[index] = overrideBinding{owner: owner, priority: priority, key: key, command: command}
			return
		}
	}
	if command != "" {
		o.list = append(o.list, overrideBinding{owner: owner, priority: priority, key: key, command: command})
	}
}

func (o *overrideBindings) clear(owner lua.LValue) {
	kept := o.list[:0]
	for _, entry := range o.list {
		if entry.owner != owner {
			kept = append(kept, entry)
		}
	}
	o.list = kept
}

// lookup finds what a key is overridden to: the priority overrides win, the
// last set first.
func (o *overrideBindings) lookup(key string) (string, bool) {
	for _, wantPriority := range []bool{true, false} {
		for index := len(o.list) - 1; index >= 0; index-- {
			if entry := o.list[index]; entry.priority == wantPriority && entry.key == key {
				return entry.command, true
			}
		}
	}
	return "", false
}

// runOverride runs what a key is overridden to.
func (rt *Runtime) runOverride(command string) bool {
	space := strings.IndexByte(command, ' ')
	if space < 0 {
		def := (*bindingDef)(nil)
		if rt.bindings != nil {
			def = rt.bindings.byName[command]
		}
		return rt.runBindingCommand(def, "down")
	}
	kind, argument := command[:space], command[space+1:]
	quote := func(text string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(text) + `"` }
	var source string
	switch kind {
	case "CLICK":
		name, button := argument, "LeftButton"
		if colon := strings.LastIndexByte(argument, ':'); colon >= 0 {
			name, button = argument[:colon], argument[colon+1:]
		}
		source = "local f = _G[" + quote(name) + "]; if f then f:Click(" + quote(button) + ") end"
	case "SPELL":
		source = "CastSpellByName(" + quote(argument) + ")"
	case "ITEM":
		source = "UseItemByName(" + quote(argument) + ")"
	case "MACRO":
		source = "RunMacro(" + quote(argument) + ")"
	default:
		return false
	}
	return rt.Execute(source, "@override-binding.lua")
}

// handleOverrideKey runs the override of a key press, if it has one.
func (eng *UIEngine) handleOverrideKey(key window.Key, mods window.ModifierKey) bool {
	combo := bindingKeyCombo(key, mods)
	if combo == "" {
		return false
	}
	command, found := eng.Rt.overrides.lookup(combo)
	if !found {
		return false
	}
	return eng.Rt.runOverride(command)
}

func registerOverrideBindingAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	owner := func(L *lua.LState, usage string) lua.LValue {
		value := L.Get(1)
		if value.Type() != lua.LTTable && value.Type() != lua.LTUserData {
			L.RaiseError("%s", usage)
		}
		return value
	}
	setter := func(name, usage string, build func(L *lua.LState) string) {
		reg(name, func(L *lua.LState) int {
			table := owner(L, usage)
			if L.Get(3).Type() != lua.LTString {
				L.RaiseError("%s", usage)
			}
			rt.overrides.set(table, L.OptBool(2, false), L.ToString(3), build(L))
			rt.FireEvent("UPDATE_BINDINGS")
			return 0
		})
	}
	setter("SetOverrideBinding", `Usage: SetOverrideBinding(owner, isPriority, "KEY"[, "COMMAND"])`,
		func(L *lua.LState) string { return L.OptString(4, "") })
	setter("SetOverrideBindingSpell", `Usage: SetOverrideBindingSpell(owner, isPriority, "KEY", "SPELL")`,
		func(L *lua.LState) string { return "SPELL " + L.CheckString(4) })
	setter("SetOverrideBindingItem", `Usage: SetOverrideBindingItem(owner, isPriority, "KEY", "ITEM")`,
		func(L *lua.LState) string { return "ITEM " + L.CheckString(4) })
	setter("SetOverrideBindingMacro", `Usage: SetOverrideBindingMacro(owner, isPriority, "KEY", "MACRO")`,
		func(L *lua.LState) string { return "MACRO " + L.CheckString(4) })
	setter("SetOverrideBindingClick", `Usage: SetOverrideBindingClick(owner, isPriority, "KEY", "BUTTON"[, "mouseButton"])`,
		func(L *lua.LState) string { return "CLICK " + L.CheckString(4) + ":" + L.OptString(5, "LeftButton") })
	reg("ClearOverrideBindings", func(L *lua.LState) int {
		rt.overrides.clear(owner(L, "Usage: ClearOverrideBindings(owner)"))
		rt.FireEvent("UPDATE_BINDINGS")
		return 0
	})
}
