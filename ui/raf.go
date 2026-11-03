package ui

import (

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Refer a friend: a friend linked to the account can be granted a level or
// summoned.
const (
	rafProposeLevelGrant = 0x041F
	rafGrantLevel        = 0x040D
	rafAcceptLevelGrant  = 0x0420
	rafDynamicFlag       = 0x40
	// rafGrantableField is the byte of the player's bytes that counts the
	// levels the character may still grant.
	rafGrantableField = 1049
	rafSummonSpell    = 45927
	rafMaxGrantLevel  = 59
	rafSummonMaxLevel = 60
)

func (rt *Runtime) handleRAFPacket(packet world.Packet) bool {
	if packet.Opcode != rafProposeLevelGrant {
		return false
	}
	guid, err := world.ReadPackedGUID(world.NewReader(packet.Body, "SMSG_PROPOSE_LEVEL_GRANT"))
	if err != nil {
		return true
	}
	rt.rafGrantFrom = guid
	rt.FireEvent("LEVEL_GRANT_PROPOSED", lua.LString(rt.nameOf(guid)))
	return true
}

// rafLinked reports whether a unit is a friend linked to the character.
func rafLinked(info *UnitInfo) bool { return info != nil && info.DynamicFlags&rafDynamicFlag != 0 }

// rafGrantCheck is why a level cannot be granted to a unit: 0 when it can.
func (rt *Runtime) rafGrantCheck(info *UnitInfo) int {
	player := rt.unitInfo("player")
	switch {
	case info == nil || info.GUID == 0:
		return 8
	case !info.Exists:
		return 4
	case !rafLinked(info):
		return 1
	}
	if player != nil && info.RaceID != 0 && player.RaceID != 0 && hordeRace(uint8(info.RaceID)) != hordeRace(uint8(player.RaceID)) {
		return 5
	}
	switch {
	case info.Level > rafMaxGrantLevel:
		return 7
	case player != nil && player.Level <= info.Level:
		return 2
	case rt.field(rafGrantableField)>>8&0xff == 0:
		return 3
	}
	return 0
}

// rafSummonCheck is why a friend cannot be summoned: 0 when the friend can.
func (rt *Runtime) rafSummonCheck(info *UnitInfo) int {
	switch {
	case info == nil || info.GUID == 0:
		return 8
	case !rafLinked(info):
		return 1
	case !info.Exists || rt.memberByGUID(info.GUID) == nil:
		return 9
	case info.Level >= rafSummonMaxLevel+1:
		return 10
	}
	return 0
}

func (rt *Runtime) rafError(code int, name string) {
	switch code {
	case 1, 2, 3, 4, 5, 6:
		rt.ShowGameError(0x262 + code)
	case 7:
		rt.ShowGameError(0x269, 60)
	case 8:
		rt.ShowGameError(199)
	case 9:
		rt.ShowGameError(0x51, name)
	case 10:
		rt.ShowGameError(0x26a, 60)
	case 11:
		rt.ShowGameError(0x26b)
	}
}

func registerRAFAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	unitArg := func(L *lua.LState) *UnitInfo { return rt.unitByNameOrToken(L.OptString(1, "")) }
	reg("CanGrantLevel", func(L *lua.LState) int {
		if rt.rafGrantCheck(unitArg(L)) == 0 {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("GrantLevel", func(L *lua.LState) int {
		info := unitArg(L)
		if code := rt.rafGrantCheck(info); code != 0 {
			rt.rafError(code, "")
			return 0
		}
		rt.send(rafGrantLevel, world.AppendPackedGUID(nil, info.GUID))
		return 0
	})
	reg("AcceptLevelGrant", func(L *lua.LState) int {
		if rt.rafGrantFrom != 0 {
			rt.send(rafAcceptLevelGrant, world.AppendPackedGUID(nil, rt.rafGrantFrom))
			rt.rafGrantFrom = 0
		}
		return 0
	})
	reg("DeclineLevelGrant", func(L *lua.LState) int { rt.rafGrantFrom = 0; return 0 })
	reg("CanSummonFriend", func(L *lua.LState) int {
		if rt.rafSummonCheck(unitArg(L)) == 0 {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("SummonFriend", func(L *lua.LState) int {
		info := unitArg(L)
		if code := rt.rafSummonCheck(info); code != 0 {
			name := ""
			if info != nil {
				name = info.Name
			}
			rt.rafError(code, name)
			return 0
		}
		if host, ok := rt.Host.(WorldSpellHost); ok {
			_ = host.CastSpell(rafSummonSpell, info.GUID)
		}
		return 0
	})
}
