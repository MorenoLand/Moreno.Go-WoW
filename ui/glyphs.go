package ui

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

const (
	glyphSockets = 6
	// glyphApplyEffect is the spell effect of a glyph item.
	glyphApplyEffect = 74
	// glyphLinkColor opens a glyph link.
	glyphLinkColor = "|cff66bbff"
)

// Spell failure codes the glyph window reports.
const (
	spellFailedInvalidGlyph      = 175
	spellFailedUniqueGlyph       = 176
	spellFailedGlyphSocketLocked = 177
)

// GlyphProperty is a row of GlyphProperties.dbc.
type GlyphProperty struct {
	Spell uint32
	// Type is the socket kind the glyph fits: 0 major, 1 minor.
	Type int
	// Icon is the texture of the glyph.
	Icon string
}

// GlyphTables are GlyphProperties.dbc and GlyphSlot.dbc.
type GlyphTables struct {
	Properties map[uint32]GlyphProperty
	// Slots maps a glyph slot id to its kind: 0 major, 1 minor.
	Slots map[uint32]int
}

// GlyphHost sends the glyph requests.
type GlyphHost interface {
	// UseGlyphItem uses a glyph item on a socket (counted from 0).
	UseGlyphItem(bag, slot uint8, spellID uint32, itemGUID uint64, socket uint32) error
	// RemoveGlyph takes the glyph out of a socket (counted from 0).
	RemoveGlyph(socket uint32) error
}

// glyphPending is the glyph item used and waiting for a socket to be chosen.
type glyphPending struct {
	bag, slot uint8
	item      uint64
	spell     uint32
}

type glyphState struct {
	tables GlyphTables
	// slots is the slot id of each socket and enabled the mask of those open,
	// from the player's fields; active is the glyph in each socket.
	slots   [glyphSockets]uint32
	enabled uint32
	active  [glyphSockets]uint32
	known   bool
	pending *glyphPending
}

// SetGlyphTables gives the interface the glyph data files.
func (rt *Runtime) SetGlyphTables(tables GlyphTables) { rt.glyphs.tables = tables }

// SetGlyphFields takes the glyph fields of the player: each socket's slot, the
// glyph in it and which sockets are open. Sockets that changed tell the glyph
// window.
func (rt *Runtime) SetGlyphFields(slots, glyphs [glyphSockets]uint32, enabled uint32) {
	state := &rt.glyphs
	previous, known := *state, state.known
	state.slots, state.active, state.enabled, state.known = slots, glyphs, enabled, true
	if !known {
		return
	}
	for socket := 0; socket < glyphSockets; socket++ {
		index := lua.LNumber(socket + 1)
		bit := uint32(1) << uint(socket)
		switch {
		case previous.enabled&bit == 0 && enabled&bit != 0:
			rt.FireEvent("GLYPH_ENABLED", index)
		case previous.enabled&bit != 0 && enabled&bit == 0:
			rt.FireEvent("GLYPH_DISABLED", index)
		}
		switch {
		case previous.active[socket] == 0 && glyphs[socket] != 0:
			rt.FireEvent("GLYPH_ADDED", index)
		case previous.active[socket] != 0 && glyphs[socket] == 0:
			rt.FireEvent("GLYPH_REMOVED", index)
		case previous.active[socket] != glyphs[socket]:
			rt.FireEvent("GLYPH_UPDATED", index)
		}
	}
}

// glyphIn is the glyph a talent group holds in a socket.
func (rt *Runtime) glyphIn(group, socket int) uint32 {
	specs := rt.talents.specs
	if group >= 0 && group < len(specs) {
		if glyphs := specs[group].Glyphs; socket < len(glyphs) {
			return uint32(glyphs[socket])
		}
		return 0
	}
	return rt.glyphs.active[socket]
}

// glyphPendingProperty is the glyph the pending item places.
func (rt *Runtime) glyphPendingProperty() (uint32, bool) {
	pending := rt.glyphs.pending
	if pending == nil {
		return 0, false
	}
	for _, effect := range rt.gameplay.Catalog[pending.spell].Effects {
		if effect.Effect == glyphApplyEffect {
			return uint32(effect.Misc), true
		}
	}
	return 0, false
}

// useGlyphItem holds a glyph item that was used until a socket is chosen. It
// reports whether the spell is a glyph's.
func (rt *Runtime) useGlyphItem(bag, slot uint8, item uint64, spell uint32) bool {
	for _, effect := range rt.gameplay.Catalog[spell].Effects {
		if effect.Effect == glyphApplyEffect {
			rt.glyphs.pending = &glyphPending{bag: bag, slot: slot, item: item, spell: spell}
			rt.FireEvent("USE_GLYPH", lua.LNumber(spell))
			return true
		}
	}
	return false
}

// registerGlyphAPI defines the calls the glyph window makes.
func registerGlyphAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	group := func(L *lua.LState, index int) (int, bool) {
		if number, ok := L.Get(index).(lua.LNumber); ok && int(number) >= 1 {
			return int(number) - 1, int(number)-1 < max(len(rt.talents.specs), 1)
		}
		return rt.talents.active, true
	}
	reg("GetGlyphSocketInfo", func(L *lua.LState) int {
		socket := L.CheckInt(1) - 1
		talentGroup, ok := group(L, 2)
		if socket < 0 || socket >= glyphSockets || !ok {
			return 0
		}
		state := &rt.glyphs
		if state.enabled&(1<<uint(socket)) != 0 {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		if kind, found := state.tables.Slots[state.slots[socket]]; found {
			L.Push(lua.LNumber(kind + 1))
		} else {
			L.Push(lua.LNil)
		}
		if property, found := state.tables.Properties[rt.glyphIn(talentGroup, socket)]; found {
			L.Push(lua.LNumber(property.Spell))
			L.Push(lua.LString(property.Icon))
			return 4
		}
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 4
	})
	reg("GetGlyphLink", func(L *lua.LState) int {
		socket := L.CheckInt(1) - 1
		talentGroup, ok := group(L, 2)
		if socket < 0 || socket >= glyphSockets || !ok {
			return 0
		}
		state := &rt.glyphs
		slot, glyph := state.slots[socket], rt.glyphIn(talentGroup, socket)
		property, found := state.tables.Properties[glyph]
		name := rt.gameplay.Catalog[property.Spell].Name
		if _, slotKnown := state.tables.Slots[slot]; !found || !slotKnown || name == "" {
			L.Push(lua.LString(""))
			return 1
		}
		L.Push(lua.LString(fmt.Sprintf("%s|Hglyph:%d:%d|h[%s]|h|r", glyphLinkColor, slot, glyph, name)))
		return 1
	})
	reg("GlyphMatchesSocket", func(L *lua.LState) int {
		socket := L.CheckInt(1) - 1
		property, ok := rt.glyphPendingProperty()
		if !ok || socket < 0 || socket >= glyphSockets {
			L.Push(lua.LNil)
			return 1
		}
		kind, slotKnown := rt.glyphs.tables.Slots[rt.glyphs.slots[socket]]
		glyph, glyphKnown := rt.glyphs.tables.Properties[property]
		if slotKnown && glyphKnown && kind == glyph.Type {
			L.Push(lua.LNumber(1))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("PlaceGlyphInSocket", func(L *lua.LState) int {
		socket := L.CheckInt(1) - 1
		if socket < 0 || socket >= glyphSockets {
			return 0
		}
		state := &rt.glyphs
		pending := state.pending
		if pending == nil {
			L.Push(lua.LNil)
			return 1
		}
		if state.enabled&(1<<uint(socket)) == 0 {
			rt.ShowSpellFailure(spellFailedGlyphSocketLocked, "")
			state.pending = nil
			L.Push(lua.LNil)
			return 1
		}
		glyph, ok := rt.glyphPendingProperty()
		kind, slotKnown := state.tables.Slots[state.slots[socket]]
		property, glyphKnown := state.tables.Properties[glyph]
		if !ok || !slotKnown || !glyphKnown || kind != property.Type {
			rt.ShowSpellFailure(spellFailedInvalidGlyph, "")
			state.pending = nil
			L.Push(lua.LNil)
			return 1
		}
		for _, held := range state.active {
			if held == glyph {
				rt.ShowSpellFailure(spellFailedUniqueGlyph, "")
				state.pending = nil
				L.Push(lua.LNil)
				return 1
			}
		}
		host, hosted := rt.Host.(GlyphHost)
		if !hosted {
			L.RaiseError("the glyph transport is unavailable")
			return 0
		}
		if err := host.UseGlyphItem(pending.bag, pending.slot, pending.spell, pending.item, uint32(socket)); err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		state.pending = nil
		L.Push(lua.LNumber(1))
		return 1
	})
	reg("RemoveGlyphFromSocket", func(L *lua.LState) int {
		socket := L.CheckInt(1) - 1
		if socket < 0 || socket >= glyphSockets || rt.glyphs.active[socket] == 0 {
			return 0
		}
		if host, ok := rt.Host.(GlyphHost); ok {
			if err := host.RemoveGlyph(uint32(socket)); err != nil {
				L.RaiseError("%v", err)
			}
		}
		return 0
	})
}
