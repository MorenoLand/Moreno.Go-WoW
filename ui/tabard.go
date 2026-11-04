package ui

import (
	"encoding/binary"
	"fmt"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The tabard designer: a guild master changes the emblem of the guild at a
// tabard vendor for a price.
const (
	tabardVendorPacket = 0x01F2
	tabardSavePacket   = 0x01F1
	tabardCost         = 100000

	// How many of each part the textures offer.
	tabardEmblemStyles    = 170
	tabardEmblemColors    = 17
	tabardBorderStyles    = 10
	tabardBorderColors    = 17
	tabardBackgroundColor = 51
)

// The part a customization changes, by the id of its row in the frame.
const (
	tabardPartEmblem = iota + 1
	tabardPartEmblemColor
	tabardPartBorder
	tabardPartBorderColor
	tabardPartBackground
)

type tabardState struct {
	vendor uint64
	open   bool
	// design is the emblem being made: style, color, border style, border
	// color and background color.
	style, color, borderStyle, borderColor, background uint32
	pending                                            bool
}

func (rt *Runtime) tabardFile(kind string, part string) string {
	t := &rt.tabard
	switch kind {
	case "border":
		return fmt.Sprintf(`Textures\GuildEmblems\Border_%02d_%02d_%s`, t.borderStyle, t.borderColor, part)
	case "background":
		return fmt.Sprintf(`Textures\GuildEmblems\Background_%02d_%s`, t.background, part)
	}
	return fmt.Sprintf(`Textures\GuildEmblems\Emblem_%02d_%02d_%s`, t.style, t.color, part)
}

// tabardChanged reports whether the design differs from the emblem the guild
// has.
func (rt *Runtime) tabardChanged() bool {
	t, q := &rt.tabard, rt.guild.query
	return !rt.guild.emblemKnown || t.style != q.EmblemStyle || t.color != q.EmblemColor ||
		t.borderStyle != q.BorderStyle || t.borderColor != q.BorderColor || t.background != q.BackgroundColor
}

func (rt *Runtime) tabardCanSave() bool {
	return rt.guild.id != 0 && !rt.tabard.pending && rt.tabardChanged() && uint32(rt.inventory.money) >= tabardCost
}

// tabardCycle moves a part of the design to the next or the previous choice.
func (rt *Runtime) tabardCycle(part, direction int) {
	t := &rt.tabard
	step := func(value uint32, count uint32) uint32 {
		return uint32((int(value)+direction)%int(count)+int(count)) % count
	}
	switch part {
	case tabardPartEmblem:
		t.style = step(t.style, tabardEmblemStyles)
	case tabardPartEmblemColor:
		t.color = step(t.color, tabardEmblemColors)
	case tabardPartBorder:
		t.borderStyle = step(t.borderStyle, tabardBorderStyles)
	case tabardPartBorderColor:
		t.borderColor = step(t.borderColor, tabardBorderColors)
	case tabardPartBackground:
		t.background = step(t.background, tabardBackgroundColor)
	default:
		return
	}
	rt.FireEvent("TABARD_CANSAVE_CHANGED")
}

func (rt *Runtime) handleTabardPacket(packet world.Packet) bool {
	switch packet.Opcode {
	case tabardVendorPacket:
		guid, err := world.NewReader(packet.Body, "MSG_TABARDVENDOR_ACTIVATE").U64()
		if err != nil {
			return true
		}
		rt.tabard.vendor, rt.tabard.open = guid, true
		rt.FireEvent("OPEN_TABARD_FRAME", lua.LString("player"))
	case tabardSavePacket:
		result, err := world.NewReader(packet.Body, "MSG_SAVE_GUILD_EMBLEM").U32()
		if err != nil || result >= 6 {
			return true
		}
		errors := [...]int{0x13d, 0x13e, 0x13f, 0x140, 0x141, 0x2da}
		if errors[result] != 0x2da {
			rt.ShowGameError(errors[result])
		}
		rt.tabard.pending = false
		if result == 0 {
			rt.FireEvent("GUILDTABARD_UPDATE")
			return true
		}
		rt.FireEvent("TABARD_SAVE_PENDING")
	default:
		return false
	}
	return true
}

func registerTabardAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetTabardCreationCost", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(tabardCost))
		return 1
	}))
	L.SetGlobal("CloseTabardCreation", L.NewFunction(func(L *lua.LState) int {
		if rt.tabard.open {
			rt.tabard.open = false
			rt.FireEvent("CLOSE_TABARD_FRAME")
		}
		return 0
	}))
}

// tabardMethods are the methods of the model that shows the tabard.
func (rt *Runtime) tabardMethods() map[string]func(L *lua.LState, w *widget) int {
	setTexture := func(part string) func(L *lua.LState, w *widget) int {
		return func(L *lua.LState, w *widget) int {
			if user, ok := L.Get(2).(*lua.LUserData); ok {
				if target, ok := user.Value.(*widget); ok {
					target.textureFile = rt.tabardFile("emblem", part)
				}
			}
			return 0
		}
	}
	return map[string]func(L *lua.LState, w *widget) int{
		"InitializeTabardColors": func(L *lua.LState, w *widget) int {
			t, q := &rt.tabard, rt.guild.query
			if rt.guild.emblemKnown {
				t.style, t.color, t.borderStyle, t.borderColor, t.background = q.EmblemStyle, q.EmblemColor, q.BorderStyle, q.BorderColor, q.BackgroundColor
			}
			return 0
		},
		"CycleVariation": func(L *lua.LState, w *widget) int {
			rt.tabardCycle(L.CheckInt(2), L.CheckInt(3))
			return 0
		},
		"CanSaveTabardNow": func(L *lua.LState, w *widget) int {
			L.Push(lua.LBool(rt.tabardCanSave()))
			return 1
		},
		"Save": func(L *lua.LState, w *widget) int {
			if !rt.tabardCanSave() {
				return 0
			}
			t := &rt.tabard
			body := binary.LittleEndian.AppendUint64(nil, t.vendor)
			for _, value := range []uint32{t.style, t.color, t.borderStyle, t.borderColor, t.background} {
				body = binary.LittleEndian.AppendUint32(body, value)
			}
			if rt.send(tabardSavePacket, body) {
				t.pending = true
				rt.FireEvent("TABARD_SAVE_PENDING")
			}
			return 0
		},
		"GetUpperEmblemTexture": setTexture("TU_U"),
		"GetLowerEmblemTexture": setTexture("TL_U"),
		"GetUpperEmblemFileName": func(L *lua.LState, w *widget) int {
			L.Push(lua.LString(rt.tabardFile("emblem", "TU_U")))
			return 1
		},
		"GetLowerEmblemFileName": func(L *lua.LState, w *widget) int {
			L.Push(lua.LString(rt.tabardFile("emblem", "TL_U")))
			return 1
		},
	}
}
