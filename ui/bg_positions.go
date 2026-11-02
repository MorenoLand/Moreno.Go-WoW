package ui

import (
	"math"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// In a battleground the server tells where the players of the other groups
// and the flag carriers are, so the map can show them.
const (
	bgPositionsOpcode   = 0x02E9
	bgPositionsInterval = 5 * time.Second
	// bgEyeOfTheStorm is the map whose flag is drawn for the other team.
	bgEyeOfTheStorm = 566
)

var bgFlagNames = [2]string{"AllianceFlag", "HordeFlag"}

type bgPosition struct {
	guid uint64
	x, y float32
}

type bgPositionState struct {
	players []bgPosition
	// flags are the carriers of the flags, at most two.
	flags   [2]bgPosition
	nextAsk time.Time
}

func (rt *Runtime) handleBGPositionsPacket(packet world.Packet) bool {
	if packet.Opcode != bgPositionsOpcode {
		return false
	}
	reader := world.NewReader(packet.Body, "MSG_BATTLEGROUND_PLAYER_POSITIONS")
	state := &rt.bgPositions
	count, err := reader.U32()
	if err != nil {
		return true
	}
	read := func() (bgPosition, bool) {
		guid, err := reader.U64()
		if err != nil {
			return bgPosition{}, false
		}
		x, errX := reader.U32()
		y, errY := reader.U32()
		return bgPosition{guid: guid, x: math.Float32frombits(x), y: math.Float32frombits(y)}, errX == nil && errY == nil
	}
	state.players = state.players[:0]
	for i := uint32(0); i < count && i < 80; i++ {
		position, ok := read()
		if !ok {
			return true
		}
		state.players = append(state.players, position)
	}
	flagCount, err := reader.U32()
	if err != nil || int32(flagCount) <= 0 {
		state.flags = [2]bgPosition{}
		return true
	}
	if flagCount < 2 {
		state.flags[1] = bgPosition{}
	}
	// The carriers come last to first.
	for index := int(min(flagCount, 2)) - 1; index >= 0; index-- {
		position, ok := read()
		if !ok {
			return true
		}
		state.flags[index] = position
	}
	return true
}

// groupedOrSelf reports a player the map already shows: the character and the
// members of its party or raid.
func (rt *Runtime) groupedOrSelf(guid uint64) bool {
	return guid == rt.playerGUID() || rt.memberByGUID(guid) != nil
}

// projectBG places a world position on the map being shown.
func (rt *Runtime) projectBG(x, y float32) (float64, float64) {
	state := &rt.worldMap
	if page := state.currentPage(); page != nil && page.MapID == state.playerMap {
		return page.project(x, y)
	}
	return 0, 0
}

func (rt *Runtime) numBGFlags() int {
	switch {
	case rt.bgPositions.flags[1].guid != 0:
		return 2
	case rt.bgPositions.flags[0].guid != 0:
		return 1
	}
	return 0
}

// bgFlagName is the picture of the flag a carrier holds: by the carrier's
// team, the other way round on the Eye of the Storm.
func (rt *Runtime) bgFlagName(guid uint64) string {
	horde := false
	if rt.UnitRaceOfGUID != nil {
		if race, ok := rt.UnitRaceOfGUID(guid); ok {
			horde = hordeRace(race)
		}
	}
	if rt.worldMap.playerMap == bgEyeOfTheStorm {
		horde = !horde
	}
	if horde {
		return bgFlagNames[1]
	}
	return bgFlagNames[0]
}

func registerBGPositionsAPI(rt *Runtime) {
	L := rt.L
	state := &rt.bgPositions
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("RequestBattlefieldPositions", func(L *lua.LState) int {
		if rt.bf.active < 0 {
			state.players = nil
			return 0
		}
		if now := time.Now(); state.nextAsk.IsZero() || !now.Before(state.nextAsk) {
			state.nextAsk = now.Add(bgPositionsInterval)
			rt.send(bgPositionsOpcode, nil)
		}
		return 0
	})
	others := func() []bgPosition {
		var list []bgPosition
		for _, position := range state.players {
			if !rt.groupedOrSelf(position.guid) {
				list = append(list, position)
			}
		}
		return list
	}
	reg("GetNumBattlefieldPositions", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(others())))
		return 1
	})
	reg("GetBattlefieldPosition", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBattlefieldPosition(index)")
		}
		list := others()
		if index := L.CheckInt(1) - 1; index >= 0 && index < len(list) {
			x, y := rt.projectBG(list[index].x, list[index].y)
			L.Push(lua.LNumber(x))
			L.Push(lua.LNumber(y))
			if name := rt.nameOf(list[index].guid); name != "" {
				L.Push(lua.LString(name))
			} else {
				L.Push(lua.LNil)
			}
			return 3
		}
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		L.Push(lua.LNil)
		return 3
	})
	reg("GetNumBattlefieldFlagPositions", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.numBGFlags()))
		return 1
	})
	reg("GetBattlefieldFlagPosition", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetBattlefieldFlagPosition(index)")
		}
		index := L.CheckInt(1) - 1
		var flag bgPosition
		if index >= 0 && index < rt.numBGFlags() {
			flag = state.flags[index]
		}
		x, y := rt.projectBG(flag.x, flag.y)
		L.Push(lua.LNumber(x))
		L.Push(lua.LNumber(y))
		if index >= 0 && index < rt.numBGFlags() {
			L.Push(lua.LString(rt.bgFlagName(flag.guid)))
		} else {
			L.Push(lua.LNil)
		}
		return 3
	})
}
