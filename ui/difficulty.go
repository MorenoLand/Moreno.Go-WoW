package ui

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

const (
	packetDungeonDifficulty = uint16(0x0329)
	packetRaidDifficulty    = uint16(0x04EB)
	packetInstanceDiffCount = uint16(0x033B)
	packetChangeResult      = uint16(0x020E)
	clientDungeonDifficulty = world.ClientOpcode(0x0329)
	clientRaidDifficulty    = world.ClientOpcode(0x04EB)
	clientChangeDifficulty  = world.ClientOpcode(0x01FD)
	// mapFlagToggleDifficulty marks a map whose raid difficulty can be switched.
	mapFlagToggleDifficulty = 0x100
	mapTypeRaid             = 2
)

// difficultyState is the difficulty of dungeons and raids, the character's own
// and the group's.
type difficultyState struct {
	dungeon, dungeonGroup int
	raid, raidGroup       int
	// player is the difficulty the character plays at in an instance.
	player int
	// changeBlockedUntil is when the difficulty may be changed again.
	changeBlockedUntil time.Time
}

// inParty is whether the character is grouped and the group is not a raid.
func (rt *Runtime) partyOnly() bool { return rt.group.in && len(rt.group.members) > 0 && !rt.isRaid() }

func (rt *Runtime) raidRoster() bool { return rt.isRaid() }

// dungeonDifficulty is the difficulty dungeons are run at now.
func (rt *Runtime) dungeonDifficulty() int {
	if rt.partyOnly() {
		return rt.diff.dungeonGroup
	}
	return rt.diff.dungeon
}

// raidDifficulty is the difficulty raids are run at now.
func (rt *Runtime) raidDifficulty() int {
	if rt.raidRoster() {
		return rt.diff.raidGroup
	}
	return rt.diff.raid
}

// mapFlags are the flags of the map the character is on.
func (rt *Runtime) mapFlags() uint32 {
	if catalog := rt.worldMap.catalog; catalog != nil {
		return catalog.MapFlags[rt.worldMap.playerMap]
	}
	return 0
}

func (rt *Runtime) mapType() uint32 {
	if catalog := rt.worldMap.catalog; catalog != nil {
		return catalog.MapTypes[rt.worldMap.playerMap]
	}
	return 0
}

// say shows a text of the game errors with the name of a difficulty.
func (rt *Runtime) sayDifficulty(index int, global string, level int) {
	rt.ShowGameError(index, rt.globalString(fmt.Sprintf("%s%d", global, level+1), ""))
}

func (rt *Runtime) announceDungeonDifficulty() {
	rt.sayDifficulty(503, "DUNGEON_DIFFICULTY", rt.dungeonDifficulty())
}

func (rt *Runtime) announceRaidDifficulty() {
	if rt.mapFlags()&mapFlagToggleDifficulty != 0 {
		rt.sayDifficulty(671, "RAID_DIFFICULTY", rt.raidDifficulty())
		rt.sayDifficulty(672, "PLAYER_DIFFICULTY", rt.diff.player)
		return
	}
	if rt.mapType() == mapTypeRaid {
		rt.ShowGameError(670, rt.mapName(rt.worldMap.playerMap))
		return
	}
	rt.sayDifficulty(670, "RAID_DIFFICULTY", rt.raidDifficulty())
}

// applyDungeonDifficulty takes the difficulty the server set.
func (rt *Runtime) applyDungeonDifficulty(level int, own, group bool) {
	before := rt.dungeonDifficulty()
	if own {
		rt.diff.dungeon = level
	}
	if group {
		rt.diff.dungeonGroup = level
	}
	if before != rt.dungeonDifficulty() {
		rt.announceDungeonDifficulty()
	}
}

// applyRaidDifficulty takes the raid difficulty the server set.
func (rt *Runtime) applyRaidDifficulty(level int, own, group bool) {
	beforeRaid, beforeDungeon := rt.raidDifficulty(), rt.dungeonDifficulty()
	if own {
		rt.diff.raid = level
	}
	if group {
		if rt.mapFlags()&mapFlagToggleDifficulty == 0 {
			rt.diff.raidGroup = level
		} else {
			rt.askDifficultyChange(level > 1)
		}
	}
	if beforeRaid != rt.raidDifficulty() {
		rt.announceRaidDifficulty()
	}
	if beforeDungeon != rt.dungeonDifficulty() {
		rt.announceDungeonDifficulty()
	}
}

// canChangeDifficulty is whether the character may switch the difficulty of the
// raid they stand in: they lead its group.
func (rt *Runtime) canChangeDifficulty() bool {
	if rt.mapType() != mapTypeRaid || rt.mapFlags()&mapFlagToggleDifficulty == 0 {
		return false
	}
	return rt.raidRoster() && rt.group.info.Leader != 0 && rt.group.info.Leader == rt.playerGUID()
}

// askDifficultyChange asks the server to switch to the other difficulty.
func (rt *Runtime) askDifficultyChange(heroic bool) {
	if !rt.canChangeDifficulty() {
		return
	}
	if until := rt.diff.changeBlockedUntil; time.Now().Before(until) {
		rt.ShowGameError(714, rt.durationText(int(time.Until(until).Seconds())))
		return
	}
	value := uint32(0)
	if heroic {
		value = 1
	}
	rt.send(clientChangeDifficulty, binary.LittleEndian.AppendUint32(nil, value))
}

// handleDifficultyPacket takes the packets that set difficulties.
func (rt *Runtime) handleDifficultyPacket(packet world.Packet) bool {
	reader := world.NewReader(packet.Body, "difficulty")
	switch packet.Opcode {
	case packetDungeonDifficulty, packetRaidDifficulty:
		level, err := reader.U32()
		if err != nil {
			return true
		}
		own, _ := reader.U32()
		group, _ := reader.U32()
		if packet.Opcode == packetDungeonDifficulty {
			rt.applyDungeonDifficulty(int(level), own != 0, group != 0)
		} else {
			rt.applyRaidDifficulty(int(level), own != 0, group != 0)
		}
	case packetInstanceDiffCount:
		level, err := reader.U32()
		if err == nil {
			rt.diff.player = int(level)
		}
	case packetChangeResult:
		rt.applyChangeResult(reader)
	default:
		return false
	}
	return true
}

// applyChangeResult takes the answer to a request to switch difficulty.
func (rt *Runtime) applyChangeResult(reader *world.Reader) {
	code, err := reader.U32()
	if err != nil {
		return
	}
	switch code {
	case 0:
		if level, err := reader.U8(); err == nil {
			rt.diff.player = int(level)
		}
	case 1:
		if seconds, err := reader.U32(); err == nil {
			rt.ShowGameError(714, rt.durationText(int(seconds)))
		}
	case 2:
		rt.ShowGameError(715)
	case 3:
		rt.ShowGameError(716)
	case 4:
		rt.ShowGameError(717)
	case 5:
		rt.ShowGameError(718)
	case 6:
		if seconds, err := reader.U32(); err == nil {
			rt.diff.changeBlockedUntil = time.Now().Add(time.Duration(seconds) * time.Second)
		}
	case 7:
		rt.ShowGameError(719)
	case 9:
		rt.FireEvent("PLAYER_DIFFICULTY_CHANGED")
		rt.sayDifficulty(672, "PLAYER_DIFFICULTY", rt.diff.player)
	}
}

func registerDifficultyAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetDungeonDifficulty", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.dungeonDifficulty() + 1))
		L.Push(lua.LNumber(rt.diff.dungeon + 1))
		return 2
	})
	reg("GetRaidDifficulty", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.raidDifficulty() + 1))
		L.Push(lua.LNumber(rt.diff.raid + 1))
		return 2
	})
	reg("SetDungeonDifficulty", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: SetDungeonDifficulty(difficulty)")
			return 0
		}
		level := L.CheckInt(1) - 1
		if level < 0 || level > 1 {
			return 0
		}
		if (rt.group.in || rt.isRaid()) && rt.group.info.Leader != rt.playerGUID() {
			rt.ShowGameError(84)
			return 0
		}
		before := rt.dungeonDifficulty()
		rt.diff.dungeon, rt.diff.dungeonGroup = level, level
		if before != rt.dungeonDifficulty() {
			rt.announceDungeonDifficulty()
		}
		rt.send(clientDungeonDifficulty, binary.LittleEndian.AppendUint32(nil, uint32(level)))
		return 0
	})
	reg("SetRaidDifficulty", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: SetRaidDifficulty(difficulty)")
			return 0
		}
		level := L.CheckInt(1) - 1
		if level < 0 || level > 3 {
			return 0
		}
		if (rt.group.in || rt.isRaid()) && rt.group.info.Leader != rt.playerGUID() {
			rt.ShowGameError(84)
			return 0
		}
		rt.applyRaidDifficulty(level, true, true)
		rt.send(clientRaidDifficulty, binary.LittleEndian.AppendUint32(nil, uint32(level)))
		return 0
	})
	reg("CanChangePlayerDifficulty", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.canChangeDifficulty()))
		L.Push(lua.LBool(!time.Now().Before(rt.diff.changeBlockedUntil)))
		return 2
	})
}
