package ui

import (
	"math"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Death knight runes.
const (
	runeSlots = 8
	// runeRegenField is PLAYER_RUNE_REGEN_1: how fast each kind of rune comes
	// back, per second.
	runeRegenField = 148 + 0x485
	// runeCooldownMS is how long a rune takes to come back at the base rate.
	runeCooldownMS = 10000
)

// runeState is what the interface knows of the runes.
type runeState struct {
	kinds [runeSlots]uint32 // 0 blood, 1 unholy, 2 frost, 3 death
	// started is the time (ms of the runtime clock) a cooldown began, 0 for a
	// rune that is ready.
	started [runeSlots]int64
	ready   uint32
	count   int
}

func (state *runeState) reset() {
	*state = runeState{ready: 0xFFFFFFFF, count: 6}
	state.kinds = [runeSlots]uint32{0, 0, 1, 1, 2, 2}
}

func (rt *Runtime) nowMS() int64 { return int64(time.Since(rt.started) / time.Millisecond) }

// runeUsable reports whether a slot exists.
func (rt *Runtime) runeSlot(slot int) bool {
	return slot >= 0 && slot < runeSlots && (rt.runes.count == 0 && slot < 8 || slot < rt.runes.count)
}

// setRuneStart marks when a rune's cooldown began, a fraction of it already
// gone.
func (rt *Runtime) setRuneStart(slot int, fraction float64) {
	if fraction > 1 {
		fraction = 1
	}
	rt.runes.started[slot] = rt.nowMS() - int64(float64(runeCooldownMS)*fraction)
	if rt.runes.started[slot] == 0 {
		rt.runes.started[slot] = 1
	}
}

func (rt *Runtime) fireRunePower(slot int, energize bool) {
	rt.FireEvent("RUNE_POWER_UPDATE", lua.LNumber(slot+1), lua.LBool(energize))
}

// resyncRunes handles SMSG_RESYNC_RUNES.
func (rt *Runtime) resyncRunes(runes []world.Rune) {
	rt.runes.count = len(runes)
	for slot, rune := range runes {
		rt.runes.kinds[slot] = uint32(rune.Type)
		fraction := float64(rune.Cooldown) * (1.0 / 255.0)
		rt.setRuneStart(slot, fraction)
		if fraction >= 1 {
			rt.runes.started[slot] = 0
			rt.runes.ready |= 1 << slot
		} else {
			rt.runes.ready &^= 1 << slot
		}
		rt.fireRunePower(slot, false)
	}
}

// addRunePower handles SMSG_ADD_RUNE_POWER: runes that are ready again.
func (rt *Runtime) addRunePower(mask uint32) {
	for slot := 0; slot < rt.runes.count && slot < runeSlots; slot++ {
		bit := uint32(1) << slot
		if mask&bit != 0 && rt.runes.ready&bit == 0 {
			rt.runes.started[slot] = 0
			rt.runes.ready |= bit
			rt.fireRunePower(slot, false)
		}
	}
	rt.FireEvent("ACTIONBAR_UPDATE_COOLDOWN")
}

// castRunes handles the rune list of a cast: which runes were ready before and
// which are after, with how far each used rune's cooldown had come.
func (rt *Runtime) castRunes(runes world.SpellRunes) {
	before, after := uint32(runes.Before), uint32(runes.After)
	rt.runes.ready = after
	used := 0
	for slot := 0; slot < runeSlots; slot++ {
		bit := uint32(1) << slot
		switch {
		case before&bit != 0 && after&bit == 0:
			if rt.runes.started[slot] == 0 {
				fraction := 0.0
				if used < len(runes.Cooldowns) {
					fraction = float64(runes.Cooldowns[used]) * (1.0 / 255.0)
				}
				rt.setRuneStart(slot, fraction)
				rt.fireRunePower(slot, false)
			}
			used++
		case before&bit == 0 && after&bit != 0:
			rt.runes.started[slot] = 0
			rt.fireRunePower(slot, true)
		}
	}
}

// convertRune handles SMSG_CONVERT_RUNE.
func (rt *Runtime) convertRune(slot, kind uint8) {
	if int(slot) >= runeSlots {
		return
	}
	rt.runes.kinds[slot] = uint32(kind)
	rt.FireEvent("RUNE_TYPE_UPDATE", lua.LNumber(int(slot)+1))
}

// handleRunePacket takes the packets that carry the runes.
func (rt *Runtime) handleRunePacket(packet world.Packet) bool {
	switch packet.Opcode {
	case world.ResyncRunesPacket:
		if runes, err := world.ParseResyncRunes(packet.Body); err == nil {
			rt.resyncRunes(runes)
		}
	case world.AddRunePowerPacket:
		if mask, err := world.ParseAddRunePower(packet.Body); err == nil {
			rt.addRunePower(mask)
		}
	case world.ConvertRunePacket:
		if slot, kind, err := world.ParseConvertRune(packet.Body); err == nil {
			rt.convertRune(slot, kind)
		}
	default:
		return false
	}
	return true
}

// Observe lets the interface look at a packet other systems consume.
func (rt *Runtime) Observe(packet world.Packet) {
	if packet.Opcode != world.SpellGo {
		return
	}
	if runes, found, err := world.ParseSpellGoRunes(packet.Body); err == nil && found {
		if header, herr := world.ParseSpellCastHeader(packet.Body); herr == nil && header.Caster == rt.playerGUID() {
			rt.castRunes(runes)
		}
	}
}

// registerRuneAPI defines the calls the rune frame makes.
func registerRuneAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	rt.runes.reset()
	slotOf := func(L *lua.LState) (int, bool) {
		slot := L.CheckInt(1) - 1
		return slot, slot >= 0 && slot < runeSlots
	}
	reg("GetRuneType", func(L *lua.LState) int {
		slot, ok := slotOf(L)
		if !ok || !rt.runeSlot(slot) {
			return 0
		}
		L.Push(lua.LNumber(rt.runes.kinds[slot] + 1))
		return 1
	})
	reg("GetRuneCount", func(L *lua.LState) int {
		slot, ok := slotOf(L)
		if !ok {
			return 0
		}
		L.Push(lua.LNumber(boolNumber(rt.runes.ready&(1<<slot) != 0)))
		return 1
	})
	reg("GetRuneCooldown", func(L *lua.LState) int {
		slot, ok := slotOf(L)
		if !ok {
			return 0
		}
		rate := math.Float32frombits(rt.field(runeRegenField + int(rt.runes.kinds[slot])))
		duration := 0.0
		if rate > 0.001 {
			duration = float64(1 / rate)
		}
		started := rt.runes.started[slot]
		L.Push(lua.LNumber(float64(started) * 0.001))
		L.Push(lua.LNumber(duration))
		L.Push(lua.LBool(started == 0))
		return 3
	})
}
