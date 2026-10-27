package ui

import (
	"math"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// chatWindowCount is how many chat windows the client keeps settings for.
const chatWindowCount = 10

type chatTypeColor struct {
	name    string
	r, g, b byte
}

type chatMessageGroup struct {
	name  string
	first bool
}

// chatWindowChannel is a channel a window shows: its name and the row of
// ChatChannels.dbc it belongs to.
type chatWindowChannel struct {
	name string
	zone uint32
}

type chatWindow struct {
	name           string
	size           int
	r, g, b, a     byte
	shown          bool
	locked         bool
	docked         int
	uninteractable int
	messages       [len(chatMessageGroups)]bool
	channels       []chatWindowChannel
	hasPosition    bool
	point          string
	x, y           float32
	width, height  float32
}

// chatSettings are the chat windows and chat colours the character keeps.
type chatSettings struct {
	windows [chatWindowCount]chatWindow
	colors  []chatTypeColor
	// channelColors are the colours of the numbered channels.
	channelColors [maxChatChannels]chatTypeColor
	ready         bool
}

// colorByte turns a colour component into the byte the client keeps.
func colorByte(value float64) byte { return byte(int(math.Round(value * 255))) }

func colorUnit(value byte) float64 { return float64(float32(value) * (1.0 / 255.0)) }

// resetChatWindows puts the chat windows back to what a new character has:
// the first window shows nearly every group and the default channels, the
// second the combat groups.
func (rt *Runtime) resetChatWindows() {
	settings := &rt.chatCfg
	settings.colors = append(settings.colors[:0], chatTypeDefaults[:]...)
	channelColor := settings.colors[17]
	for index := range settings.channelColors {
		settings.channelColors[index] = chatTypeColor{name: "CHANNEL" + strconv.Itoa(index+1), r: channelColor.r, g: channelColor.g, b: channelColor.b}
	}
	for index := range settings.windows {
		window := chatWindow{locked: true, a: 0x28, width: 430, height: 120}
		switch index {
		case 0:
			window.shown, window.docked = true, 1
			for group, entry := range chatMessageGroups {
				if group < 40 && entry.first {
					window.messages[group] = true
				}
			}
			for _, row := range rt.channels.rows {
				if row.Flags&channelRowInitial != 0 {
					window.channels = append(window.channels, chatWindowChannel{name: row.Shortcut, zone: row.ID})
				}
			}
		case 1:
			window.shown, window.docked = true, 2
			for group := 40; group < 46; group++ {
				window.messages[group] = true
			}
		}
		settings.windows[index] = window
	}
	settings.ready = true
}

// chatTypeIndex is the number the client gives a chat type, counted from 1: the
// ordinary types first, then the numbered channels.
func (rt *Runtime) chatTypeIndex(name string) int {
	for index, entry := range rt.chatCfg.colors {
		if strings.EqualFold(entry.name, name) {
			return index + 1
		}
	}
	for index, entry := range rt.chatCfg.channelColors {
		if strings.EqualFold(entry.name, name) {
			return len(rt.chatCfg.colors) + index + 1
		}
	}
	return 0
}

// SendChatColors raises UPDATE_CHAT_COLOR for every chat type, which is how the
// chat frames learn the colours.
func (rt *Runtime) SendChatColors() {
	if !rt.chatCfg.ready {
		rt.resetChatWindows()
	}
	fire := func(entry chatTypeColor) {
		rt.FireEvent("UPDATE_CHAT_COLOR", lua.LString(entry.name), lua.LNumber(colorUnit(entry.r)), lua.LNumber(colorUnit(entry.g)), lua.LNumber(colorUnit(entry.b)))
	}
	for _, entry := range rt.chatCfg.colors {
		fire(entry)
	}
	for _, entry := range rt.chatCfg.channelColors {
		fire(entry)
	}
}

// addWindowChannel puts a channel on the list a chat window shows.
func (rt *Runtime) addWindowChannel(window int, name string, zone uint32) {
	w := &rt.chatCfg.windows[window]
	for _, channel := range w.channels {
		if strings.EqualFold(channel.name, name) {
			return
		}
	}
	w.channels = append(w.channels, chatWindowChannel{name: name, zone: zone})
}

// windowArgument reads the index of a chat window; ok is false outside 1..10.
func windowArgument(L *lua.LState, position int) (int, bool) {
	index := int(L.CheckNumber(position))
	return index - 1, index >= 1 && index <= chatWindowCount
}

func optionalNumber(value int) lua.LValue {
	if value == 0 {
		return lua.LNil
	}
	return lua.LNumber(value)
}

func optionalFlag(value bool) lua.LValue {
	if value {
		return lua.LNumber(1)
	}
	return lua.LNil
}

// registerChatSettingsAPI defines the calls that read and write the chat
// windows and chat colours.
func registerChatSettingsAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	rt.resetChatWindows()
	window := func(L *lua.LState) (*chatWindow, bool) {
		index, ok := windowArgument(L, 1)
		if !ok {
			return nil, false
		}
		return &rt.chatCfg.windows[index], true
	}
	reg("ResetChatWindows", func(L *lua.LState) int {
		rt.resetChatWindows()
		return 0
	})
	reg("GetChatTypeIndex", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.chatTypeIndex(L.CheckString(1))))
		return 1
	})
	reg("ChangeChatColor", func(L *lua.LState) int {
		name := L.CheckString(1)
		r, g, b := colorByte(float64(L.CheckNumber(2))), colorByte(float64(L.CheckNumber(3))), colorByte(float64(L.CheckNumber(4)))
		changed := false
		for index := range rt.chatCfg.colors {
			if strings.EqualFold(rt.chatCfg.colors[index].name, name) {
				entry := &rt.chatCfg.colors[index]
				entry.r, entry.g, entry.b = r, g, b
				changed = true
				break
			}
		}
		for index := range rt.chatCfg.channelColors {
			if strings.EqualFold(rt.chatCfg.channelColors[index].name, name) {
				entry := &rt.chatCfg.channelColors[index]
				entry.r, entry.g, entry.b = r, g, b
				changed = true
				break
			}
		}
		if changed {
			rt.FireEvent("UPDATE_CHAT_COLOR", lua.LString(name), lua.LNumber(colorUnit(r)), lua.LNumber(colorUnit(g)), lua.LNumber(colorUnit(b)))
		}
		return 0
	})
	reg("GetChatWindowInfo", func(L *lua.LState) int {
		w, ok := window(L)
		if !ok {
			return 0
		}
		L.Push(lua.LString(w.name))
		L.Push(lua.LNumber(w.size))
		L.Push(lua.LNumber(colorUnit(w.r)))
		L.Push(lua.LNumber(colorUnit(w.g)))
		L.Push(lua.LNumber(colorUnit(w.b)))
		L.Push(lua.LNumber(colorUnit(w.a)))
		L.Push(optionalFlag(w.shown))
		L.Push(optionalFlag(w.locked))
		L.Push(optionalNumber(w.docked))
		L.Push(optionalNumber(w.uninteractable))
		return 10
	})
	reg("GetChatWindowMessages", func(L *lua.LState) int {
		w, ok := window(L)
		if !ok {
			return 0
		}
		count := 0
		for group, on := range w.messages {
			if on {
				L.Push(lua.LString(chatMessageGroups[group].name))
				count++
			}
		}
		return count
	})
	reg("GetChatWindowChannels", func(L *lua.LState) int {
		w, ok := window(L)
		if !ok {
			return 0
		}
		count := 0
		for _, channel := range w.channels {
			if channel.name != "" {
				L.Push(lua.LString(channel.name))
				L.Push(lua.LNumber(channel.zone))
				count += 2
			}
		}
		return count
	})
	reg("AddChatWindowMessages", func(L *lua.LState) int {
		w, ok := window(L)
		if !ok {
			return 0
		}
		name := L.CheckString(2)
		for group, entry := range chatMessageGroups {
			if strings.EqualFold(entry.name, name) {
				w.messages[group] = true
				break
			}
		}
		return 0
	})
	reg("RemoveChatWindowMessages", func(L *lua.LState) int {
		w, ok := window(L)
		if !ok {
			return 0
		}
		name := L.CheckString(2)
		for group, entry := range chatMessageGroups {
			if strings.EqualFold(entry.name, name) {
				w.messages[group] = false
				break
			}
		}
		return 0
	})
	reg("AddChatWindowChannel", func(L *lua.LState) int {
		w, ok := window(L)
		if !ok {
			return 0
		}
		name := L.CheckString(2)
		var zone uint32
		if row := rt.channelRowFor(name); row != nil {
			if rt.area.zone == "" {
				return 0
			}
			zone = row.ID
		}
		L.Push(lua.LNumber(zone))
		for _, channel := range w.channels {
			if strings.EqualFold(channel.name, name) {
				return 1
			}
		}
		w.channels = append(w.channels, chatWindowChannel{name: name, zone: zone})
		return 1
	})
	reg("RemoveChatWindowChannel", func(L *lua.LState) int {
		w, ok := window(L)
		if !ok {
			return 0
		}
		name := rt.channelTargetName(L.CheckString(2))
		for index, channel := range w.channels {
			if strings.EqualFold(channel.name, name) {
				w.channels = append(w.channels[:index], w.channels[index+1:]...)
				break
			}
		}
		return 0
	})
	reg("SetChatWindowName", func(L *lua.LState) int {
		if w, ok := window(L); ok {
			name := L.OptString(2, "")
			if len(name) > 127 {
				name = name[:127]
			}
			w.name = name
		}
		return 0
	})
	reg("SetChatWindowColor", func(L *lua.LState) int {
		w, ok := window(L)
		if !ok {
			return 0
		}
		w.r, w.g, w.b = colorByte(float64(L.CheckNumber(2))), colorByte(float64(L.CheckNumber(3))), colorByte(float64(L.CheckNumber(4)))
		return 0
	})
	reg("SetChatWindowAlpha", func(L *lua.LState) int {
		if w, ok := window(L); ok {
			w.a = colorByte(float64(L.CheckNumber(2)))
		}
		return 0
	})
	reg("SetChatWindowSize", func(L *lua.LState) int {
		if w, ok := window(L); ok {
			if size := int(L.CheckNumber(2)); size > 0 {
				w.size = size
			}
		}
		return 0
	})
	reg("SetChatWindowDocked", func(L *lua.LState) int {
		if w, ok := window(L); ok {
			w.docked = int(L.OptNumber(2, 0))
		}
		return 0
	})
	reg("SetChatWindowLocked", func(L *lua.LState) int {
		if w, ok := window(L); ok {
			w.locked = lua.LVAsBool(L.Get(2))
		}
		return 0
	})
	reg("SetChatWindowShown", func(L *lua.LState) int {
		if w, ok := window(L); ok {
			w.shown = L.GetTop() < 2 || lua.LVAsBool(L.Get(2))
		}
		return 0
	})
	reg("SetChatWindowUninteractable", func(L *lua.LState) int {
		if w, ok := window(L); ok {
			if L.GetTop() >= 2 && lua.LVAsBool(L.Get(2)) {
				w.uninteractable = 1
			} else {
				w.uninteractable = 0
			}
		}
		return 0
	})
	reg("GetChatWindowSavedDimensions", func(L *lua.LState) int {
		w, ok := window(L)
		if !ok {
			return 0
		}
		L.Push(lua.LNumber(w.width))
		L.Push(lua.LNumber(w.height))
		return 2
	})
	reg("SetChatWindowSavedDimensions", func(L *lua.LState) int {
		if w, ok := window(L); ok {
			w.width, w.height = float32(L.CheckNumber(2)), float32(L.CheckNumber(3))
		}
		return 0
	})
	reg("GetChatWindowSavedPosition", func(L *lua.LState) int {
		w, ok := window(L)
		if !ok || !w.hasPosition {
			return 0
		}
		L.Push(lua.LString(w.point))
		L.Push(lua.LNumber(w.x))
		L.Push(lua.LNumber(w.y))
		return 3
	})
	reg("SetChatWindowSavedPosition", func(L *lua.LState) int {
		w, ok := window(L)
		if !ok {
			return 0
		}
		point := strings.ToUpper(L.CheckString(2))
		switch point {
		case "TOPLEFT", "TOPRIGHT", "BOTTOMLEFT", "BOTTOMRIGHT", "TOP", "BOTTOM", "LEFT", "RIGHT", "CENTER":
		default:
			L.RaiseError("Unknown Region Point")
			return 0
		}
		w.point, w.x, w.y, w.hasPosition = point, float32(L.CheckNumber(3)), float32(L.CheckNumber(4)), true
		return 0
	})
}
