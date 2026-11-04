package ui

import (
	"sort"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// The player fields and flags the soulstone and the release timer read.
const (
	playerSelfResSpellField = 1199
	playerFieldBytesField   = 1197
	// PLAYER_FIELD_BYTES bits: a release timer runs, and no window is shown.
	playerByteReleaseTimer   = 0x08
	playerByteNoRelease      = 0x10
	playerFlagSpiritReleased = 0x4000
	// releaseTimerSeconds is how long a dead character waits to release.
	releaseTimerSeconds = 360
	// unitFlagPossessed is the unit flag of a unit someone else controls.
	unitFlagPossessed = 0x01000000
	selfResurrectSend = 0x02B3
)

// releaseTimeLeft is the seconds a dead character has to release, -1 when no
// timer runs.
func (rt *Runtime) releaseTimeLeft() int {
	bytes, flags := rt.field(playerFieldBytesField), rt.field(playerFlagsField)
	switch {
	case bytes&playerByteNoRelease != 0:
		rt.death.releaseStart = time.Time{}
		return 0
	case bytes&playerByteReleaseTimer != 0 && flags&playerFlagSpiritReleased == 0:
		if rt.death.releaseStart.IsZero() {
			rt.death.releaseStart = time.Now()
		}
		return max(releaseTimerSeconds-int(time.Since(rt.death.releaseStart).Seconds()), 0)
	}
	rt.death.releaseStart = time.Time{}
	return -1
}

func registerDeathExtraAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("HasSoulstone", func(L *lua.LState) int {
		player := rt.unitInfo("player")
		if player != nil && player.Health < 1 {
			if spell := rt.field(playerSelfResSpellField); spell != 0 {
				name := "UNKNOWN"
				if info, ok := rt.gameplay.Catalog[spell]; ok {
					name = info.Name
				}
				L.Push(lua.LString(name))
				return 1
			}
			// Without a spell of its own, an item that brings the dead back counts.
			if item := rt.soulstoneItem(); item != nil {
				if template := rt.itemTemplate(item.Entry); template != nil {
					L.Push(lua.LString(template.Name))
					return 1
				}
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("UseSoulstone", func(L *lua.LState) int {
		if rt.field(playerSelfResSpellField) != 0 {
			rt.send(selfResurrectSend, nil)
		} else if item := rt.soulstoneItem(); item != nil {
			if bag, slot, ok := rt.itemLocation(item.GUID); ok {
				rt.useItem(L, item, bag, slot)
			}
		}
		return 0
	})
	reg("GetReleaseTimeRemaining", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.releaseTimeLeft()))
		return 1
	})
	reg("UnitIsPossessed", func(L *lua.LState) int {
		if info := rt.unitInfo(L.OptString(1, "")); info != nil && info.Exists && info.UnitFlags&unitFlagPossessed != 0 {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("GetQuestTimers", func(L *lua.LState) int {
		count := 0
		now := time.Now().Unix()
		for _, row := range rt.questRows() {
			if row.slot < 0 {
				continue
			}
			slot := rt.quests.slots[row.slot]
			if slot.Timer == 0 || slot.State&2 != 0 {
				continue
			}
			if left := int64(slot.Timer) - now - 1; left >= 0 {
				L.Push(lua.LNumber(left))
				count++
			}
		}
		return count
	})
}

// spellEffectSelfResurrect is the effect of a soulstone.
const spellEffectSelfResurrect = 0x5e

// soulstoneItem is an item the character carries that brings them back from the
// dead when used.
func (rt *Runtime) soulstoneItem() *ItemInstance {
	var guids []uint64
	for guid := range rt.inventory.items {
		guids = append(guids, guid)
	}
	sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
	for _, guid := range guids {
		item := rt.inventory.items[guid]
		if _, _, carried := rt.itemLocation(guid); !carried {
			continue
		}
		template := rt.itemTemplate(item.Entry)
		if template == nil {
			continue
		}
		for _, use := range template.Spells {
			if use.ID == 0 || use.Trigger != 0 {
				continue
			}
			for _, effect := range rt.gameplay.Catalog[use.ID].Effects {
				if effect.Effect == spellEffectSelfResurrect {
					return item
				}
			}
		}
	}
	return nil
}
