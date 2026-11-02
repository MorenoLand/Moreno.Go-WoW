package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// worldStateEntry is a row of WorldStateUI.dbc the character sees now.
type worldStateEntry struct {
	id, kind uint32
}

type worldStateData struct {
	mapID, zone, area uint32
	values            map[uint32]uint32
	shown             []worldStateEntry
	// serverOffset is how far the server's clock is ahead of this machine's, in
	// seconds.
	serverOffset int64
	phase        uint32
	seen         bool
}

func (s *worldStateData) init() {
	s.values = map[uint32]uint32{}
	s.phase = 1
}

// SetServerTimeOffset tells the interface how far the server's clock is ahead.
func (rt *Runtime) SetServerTimeOffset(seconds int64) { rt.ws.serverOffset = seconds }

func (rt *Runtime) worldStateValue(id uint32) uint32 { return rt.ws.values[id] }

// serverNow is the server's clock, in seconds.
func (rt *Runtime) serverNow() int64 { return time.Now().Unix() + rt.ws.serverOffset }

// setWorldState sets a variable and reports whether it changed.
func (rt *Runtime) setWorldState(id, value uint32) bool {
	s := &rt.ws
	old, found := s.values[id]
	if found && old == value {
		return false
	}
	s.values[id] = value
	if id == 0xC77 {
		rt.FireEvent("ARENA_SEASON_WORLD_STATE")
	}
	return true
}

func (rt *Runtime) rebuildWorldStateList() {
	s := &rt.ws
	s.shown = s.shown[:0]
	for _, row := range rt.bf.tables.WorldStates {
		if row.Map != 0xFFFFFFFF && row.Map != s.mapID {
			continue
		}
		if row.Area != 0 && row.Area != s.zone && row.Area != s.area {
			continue
		}
		if row.Phase != 0 && s.phase&row.Phase == 0 {
			continue
		}
		if row.Type != 0 && row.Type != 3 && row.Type != 1 {
			continue
		}
		s.shown = append(s.shown, worldStateEntry{id: row.ID, kind: row.Type})
	}
}

func (rt *Runtime) applyInitWorldStates(init world.InitWorldStates) {
	s := &rt.ws
	changed := false
	if !s.seen || s.mapID != init.Map || s.zone != init.Zone || s.area != init.Area {
		s.mapID, s.zone, s.area = init.Map, init.Zone, init.Area
		s.seen = true
		rt.rebuildWorldStateList()
		changed = true
	}
	for _, state := range init.States {
		if rt.setWorldState(state.ID, state.Value) {
			changed = true
		}
	}
	if changed {
		rt.sortBattlegroundTypes()
		rt.FireEvent("UPDATE_WORLD_STATES")
	}
}

// worldStateText fills the %w and %k places of a text of the world state
// frame: %w123 with the value of the variable, %k123 with the time left until
// the time the variable holds.
func (rt *Runtime) worldStateText(text string) string {
	var out strings.Builder
	for {
		index := strings.IndexByte(text, '%')
		if index < 0 {
			out.WriteString(text)
			return out.String()
		}
		out.WriteString(text[:index])
		rest := text[index+1:]
		digits := 0
		for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
			digits++
		}
		id, _ := strconv.ParseUint(rest[:digits], 10, 32)
		if digits >= len(rest) {
			out.WriteString("%")
			return out.String()
		}
		switch rest[digits] {
		case 'w', 'W':
			fmt.Fprintf(&out, "%d", rt.worldStateValue(uint32(id)))
			text = rest[digits+1:]
		case 'k', 'K':
			left := int64(rt.worldStateValue(uint32(id))) - rt.serverNow()
			if left < 0 {
				left = 0
			}
			if hours := left / 3600; hours < 1 {
				fmt.Fprintf(&out, "%02d:%02d", left%3600/60, left%60)
			} else {
				fmt.Fprintf(&out, "%d:%02d:%02d", hours, left%3600/60, left%60)
			}
			text = rest[digits+1:]
		default:
			out.WriteString("%")
			text = rest[digits:]
		}
	}
}

func registerWorldStateAPI(rt *Runtime) {
	L := rt.L
	rt.ws.init()
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetNumWorldStateUI", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.ws.shown)))
		return 1
	})
	rowOf := func(id uint32) *WorldStateUIRow {
		for index := range rt.bf.tables.WorldStates {
			if rt.bf.tables.WorldStates[index].ID == id {
				return &rt.bf.tables.WorldStates[index]
			}
		}
		return nil
	}
	reg("GetWorldStateUIInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetWorldStateUIInfo(index)")
		}
		index := L.CheckInt(1) - 1
		var row *WorldStateUIRow
		if index >= 0 && index < len(rt.ws.shown) {
			row = rowOf(rt.ws.shown[index].id)
		}
		if row == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(row.Type))
		if row.Variable == 0 {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNumber(rt.worldStateValue(row.Variable)))
		}
		L.Push(lua.LString(rt.worldStateText(row.Text)))
		L.Push(lua.LString(row.Icon))
		L.Push(lua.LString(row.DynamicIcon))
		L.Push(lua.LString(row.Tooltip))
		L.Push(lua.LString(row.DynamicTooltip))
		L.Push(lua.LString(row.Extended))
		for _, variable := range row.ExtendedVariables {
			L.Push(lua.LNumber(rt.worldStateValue(variable)))
		}
		return 11
	})
	reg("GetWintergraspWaitTime", func(L *lua.LState) int {
		if rt.worldStateValue(0xED9) == 0 {
			return 0
		}
		left := int64(rt.worldStateValue(0x1102)) - rt.serverNow()
		if left < 0 {
			left = 0
		}
		L.Push(lua.LNumber(left))
		return 1
	})
}
