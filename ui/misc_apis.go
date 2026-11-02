package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// logFiles are the files the chat and the combat are logged to, in the order
// LoggingChat and LoggingCombat are asked for.
var logFiles = [2]string{`Logs\WoWChatLog.txt`, `Logs\WoWCombatLog.txt`}

// pvpTimerFlag is the player flag that says the PvP flag runs out on a timer;
// without it the timer reads the whole five minutes.
const (
	pvpTimerFlag     = 0x40000
	pvpTimerFull     = 300000
)

type logState struct {
	on   [2]bool
	file [2]*os.File
}

// logLine writes a line to the log of a kind when it is being kept.
func (rt *Runtime) logLine(kind int, text string) {
	state := &rt.logs
	if !state.on[kind] || state.file[kind] == nil {
		return
	}
	now := time.Now()
	fmt.Fprintf(state.file[kind], "%d/%d %02d:%02d:%02d.%03d  %s\n", int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second(),
		now.Nanosecond()/1e6, text)
}

func registerExeMiscAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	logging := func(kind int) lua.LGFunction {
		return func(L *lua.LState) int {
			state := &rt.logs
			if L.GetTop() > 0 {
				state.on[kind] = lua.LVAsBool(L.Get(1))
				if state.on[kind] && state.file[kind] == nil {
					path := filepath.FromSlash(strings.ReplaceAll(logFiles[kind], `\`, "/"))
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
						state.file[kind], _ = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
					}
					if state.file[kind] == nil {
						state.on[kind] = false
					}
				}
			}
			if state.on[kind] {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		}
	}
	reg("LoggingChat", logging(0))
	reg("LoggingCombat", logging(1))
	reg("ConsoleAddMessage", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError("Usage: ConsoleAddMessage(string)")
		}
		return 0
	})
	reg("GetClickFrame", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError(`Usage: GetClickFrame("name")`)
		}
		if frame := rt.lookup(L.CheckString(1)); frame != nil {
			L.Push(frame.luaValue(L))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("GetTrainerGreetingText", func(L *lua.LState) int {
		L.Push(lua.LString(rt.trainer.trainer.Greeting))
		return 1
	})
	reg("GetTradeskillRepeatCount", func(L *lua.LState) int {
		count := 1
		if state := &rt.tradeSkill; state.repeatLeft > 0 && state.repeatSpell == rt.gameplay.Cast.ID {
			count = state.repeatLeft + 1
		}
		L.Push(lua.LNumber(count))
		return 1
	})
	reg("GetPVPTimer", func(L *lua.LState) int {
		if rt.unitInfo("player") == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		left := pvpTimerFull
		if rt.field(playerFlagsField)&pvpTimerFlag != 0 {
			left = rt.pvpTimerLeft()
		}
		L.Push(lua.LNumber(left + 1000))
		return 1
	})
	reg("UnitIsControlling", func(L *lua.LState) int {
		if info := rt.unitByNameOrToken(L.CheckString(1)); info != nil && (info.Charm != 0 || info.Summon != 0) {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
}

// pvpTimerLeft is the milliseconds the PvP flag has left; -1 when no timer
// is known.
func (rt *Runtime) pvpTimerLeft() int {
	if rt.pvpTimerAt.IsZero() {
		return -1
	}
	if left := time.Until(rt.pvpTimerAt); left > 0 {
		return int(left / time.Millisecond)
	}
	return -1
}
