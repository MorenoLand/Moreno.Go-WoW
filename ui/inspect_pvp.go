package ui

import (
	"encoding/binary"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

const (
	packetInspectHonor = uint16(0x02D6)
	packetInspectArena = uint16(0x0377)
	clientInspectHonor = world.ClientOpcode(0x02D6)
	clientInspectArena = world.ClientOpcode(0x0377)
	// colorScale turns a byte of a colour into a fraction.
	colorScale = 1.0 / 255.0
)

// inspectArenaTeam is one team of the inspected player: its id and the figures
// of the player in it.
type inspectArenaTeam struct {
	id    uint32
	stats [5]uint32
}

// inspectPVPState is the honor and the arena standing of the player looked at.
type inspectPVPState struct {
	haveHonor, requested bool
	rank                 uint8
	todayKills           uint16
	yesterdayKills       uint16
	todayHonor           uint32
	yesterdayHonor       uint32
	lifetimeKills        uint32
	teams                [4]inspectArenaTeam
	// cache is what the arena team queries said about any team.
	cache map[uint32]world.ArenaTeamInfo
	asked map[uint32]bool
}

// resetInspectPVP forgets what was known of the player looked at.
func (rt *Runtime) resetInspectPVP() {
	cache, asked := rt.inspectPVP.cache, rt.inspectPVP.asked
	rt.inspectPVP = inspectPVPState{cache: cache, asked: asked}
}

// rememberArenaTeam keeps the answer of an arena team query.
func (rt *Runtime) rememberArenaTeam(info world.ArenaTeamInfo) {
	if rt.inspectPVP.cache == nil {
		rt.inspectPVP.cache = make(map[uint32]world.ArenaTeamInfo)
	}
	rt.inspectPVP.cache[info.ID] = info
}

// handleInspectPVPPacket takes the honor and arena figures of the inspected player.
func (rt *Runtime) handleInspectPVPPacket(packet world.Packet) bool {
	if packet.Opcode != packetInspectHonor && packet.Opcode != packetInspectArena {
		return false
	}
	reader := world.NewReader(packet.Body, "inspect pvp")
	guid, err := reader.U64()
	if err != nil || guid != rt.talents.inspectedGUID || guid == 0 {
		return true
	}
	state := &rt.inspectPVP
	if packet.Opcode == packetInspectHonor {
		rank, _ := reader.U8()
		today, _ := reader.U16()
		yesterday, _ := reader.U16()
		todayHonor, _ := reader.U32()
		yesterdayHonor, _ := reader.U32()
		lifetime, err := reader.U32()
		if err != nil {
			return true
		}
		state.rank, state.todayKills, state.yesterdayKills = rank, today, yesterday
		state.todayHonor, state.yesterdayHonor, state.lifetimeKills = todayHonor, yesterdayHonor, lifetime
		state.haveHonor, state.requested = true, false
		rt.FireEvent("INSPECT_HONOR_UPDATE")
		return true
	}
	slot, err := reader.U8()
	if err != nil || slot >= 4 {
		return true
	}
	team := &state.teams[slot]
	for _, field := range []*uint32{&team.id, &team.stats[0], &team.stats[1], &team.stats[2], &team.stats[3], &team.stats[4]} {
		if *field, err = reader.U32(); err != nil {
			return true
		}
	}
	if _, known := state.cache[team.id]; !known && team.id != 0 {
		if state.asked == nil {
			state.asked = make(map[uint32]bool)
		}
		if !state.asked[team.id] {
			state.asked[team.id] = true
			rt.send(world.ClientArenaTeamQuery, world.BuildArenaTeamID(team.id))
		}
	}
	rt.FireEvent("INSPECT_HONOR_UPDATE")
	return true
}

func registerInspectPVPAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("HasInspectHonorData", func(L *lua.LState) int {
		if rt.inspectPVP.haveHonor {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("RequestInspectHonorData", func(L *lua.LState) int {
		state := &rt.inspectPVP
		if guid := rt.talents.inspectedGUID; !state.haveHonor && !state.requested && guid != 0 {
			body := binary.LittleEndian.AppendUint64(nil, guid)
			rt.send(clientInspectHonor, body)
			rt.send(clientInspectArena, body)
			state.requested = true
		}
		return 0
	})
	reg("GetInspectHonorData", func(L *lua.LState) int {
		state := &rt.inspectPVP
		for _, value := range []float64{float64(state.todayKills), float64(state.todayHonor), float64(state.yesterdayKills),
			float64(state.yesterdayHonor), float64(state.lifetimeKills), float64(state.rank)} {
			L.Push(lua.LNumber(value))
		}
		return 6
	})
	reg("GetInspectArenaTeamData", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetInspectArenaTeamData(index)")
			return 0
		}
		index := L.CheckInt(1) - 1
		if index < 0 || index >= 3 || rt.inspectPVP.teams[index].id == 0 {
			return 0
		}
		team := rt.inspectPVP.teams[index]
		info, known := rt.inspectPVP.cache[team.id]
		if !known {
			return 0
		}
		L.Push(lua.LString(info.Name))
		L.Push(lua.LNumber(info.Size))
		for _, stat := range team.stats {
			L.Push(lua.LNumber(stat))
		}
		for _, value := range []struct {
			number uint32
			colour bool
		}{{info.Background, true}, {info.EmblemStyle, false}, {info.EmblemColor, true}, {info.BorderStyle, false}, {info.BorderColor, true}} {
			if !value.colour {
				L.Push(lua.LNumber(value.number))
				continue
			}
			L.Push(lua.LNumber(float64(value.number>>16&0xFF) * colorScale))
			L.Push(lua.LNumber(float64(value.number>>8&0xFF) * colorScale))
			L.Push(lua.LNumber(float64(value.number&0xFF) * colorScale))
		}
		return 18
	})
}
