package ui

import (
	"sort"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Arena teams: the three teams a character can belong to, their roster and
// their standing.
const (
	arenaTeamSlots      = 3
	arenaTeamFieldFirst = 1108
	arenaTeamFieldCount = 7
	arenaRosterWait     = 10 * time.Second
	arenaSortKeys       = 7
	// Game errors the team events raise.
	arenaErrorInvite = 0x58
	arenaErrorOther  = 0x72
)

// Sort keys of the roster.
const (
	arenaSortName = iota
	arenaSortClass
	arenaSortPlayed
	arenaSortWon
	arenaSortSeasonPlayed
	arenaSortSeasonWon
	arenaSortRating
)

// ArenaTeamField is the part of a team the character's own fields hold.
type ArenaTeamField struct {
	ID, Type, Rank            uint32
	PlayedWeek, PlayedSeason  uint32
	WonSeason, PersonalRating uint32
}

type arenaTeam struct {
	id      uint32
	info    world.ArenaTeamInfo
	known   bool
	stats   world.ArenaTeamStats
	members []world.ArenaTeamMember
	// wait is when the roster may be asked for again.
	wait time.Time
}

type arenaSort struct {
	key  int
	back bool
}

type arenaState struct {
	teams       [arenaTeamSlots]arenaTeam
	showOffline bool
	sorts       [arenaSortKeys]arenaSort
	selected    uint64
	ready       bool
}

func (a *arenaState) init() {
	if a.ready {
		return
	}
	a.ready = true
	a.showOffline = true
	for index := range a.sorts {
		a.sorts[index] = arenaSort{key: index}
	}
}

// arenaFields reads a character's team fields.
func arenaFields(fields map[uint16]uint32) (teams [arenaTeamSlots]ArenaTeamField, found bool) {
	for slot := range teams {
		base := uint16(arenaTeamFieldFirst + slot*arenaTeamFieldCount)
		if _, ok := fields[base]; !ok {
			continue
		}
		found = true
		teams[slot] = ArenaTeamField{ID: fields[base], Type: fields[base+1], Rank: fields[base+2],
			PlayedWeek: fields[base+3], PlayedSeason: fields[base+4], WonSeason: fields[base+5], PersonalRating: fields[base+6]}
	}
	return
}

func (rt *Runtime) playerArenaField(slot int) (ArenaTeamField, bool) {
	info := rt.unitInfo("player")
	if info == nil || slot < 0 || slot >= arenaTeamSlots {
		return ArenaTeamField{}, false
	}
	return info.ArenaTeams[slot], true
}

// syncArenaTeams follows the character's team fields: it asks for the roster
// and the name of the teams that changed.
func (rt *Runtime) syncArenaTeams(fire bool) {
	a := &rt.arena
	a.init()
	for slot := range a.teams {
		field, _ := rt.playerArenaField(slot)
		team := &a.teams[slot]
		if team.id != field.ID {
			*team = arenaTeam{id: field.ID}
		}
		if field.ID == 0 {
			continue
		}
		if !team.known {
			rt.send(world.ClientArenaTeamQuery, world.BuildArenaTeamID(field.ID))
		}
		rt.requestArenaRoster(slot)
	}
	if fire {
		rt.FireEvent("ARENA_TEAM_UPDATE")
	}
}

func (rt *Runtime) requestArenaRoster(slot int) {
	field, _ := rt.playerArenaField(slot)
	team := &rt.arena.teams[slot]
	if field.ID == 0 || time.Now().Before(team.wait) {
		return
	}
	team.wait = time.Now().Add(arenaRosterWait)
	rt.send(world.ClientArenaTeamRoster, world.BuildArenaTeamID(field.ID))
}

func (rt *Runtime) arenaSlotOf(id uint32) int {
	for slot := range rt.arena.teams {
		if rt.arena.teams[slot].id == id {
			return slot
		}
	}
	return -1
}

func (rt *Runtime) arenaClassName(member world.ArenaTeamMember) string {
	name, _ := rt.localizedClassName(member.Class, 0)
	return name
}

// sortArenaRoster orders the members by the sort keys, the newest first.
func (rt *Runtime) sortArenaRoster(slot int) {
	a := &rt.arena
	members := a.teams[slot].members
	sort.SliceStable(members, func(i, j int) bool {
		left, right := members[i], members[j]
		if !a.showOffline && left.Online != right.Online {
			return left.Online
		}
		for _, order := range a.sorts {
			var result int
			switch order.key {
			case arenaSortName:
				result = strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
			case arenaSortClass:
				result = strings.Compare(rt.arenaClassName(left), rt.arenaClassName(right))
			default:
				var first, second uint32
				switch order.key {
				case arenaSortPlayed:
					first, second = left.PlayedWeek, right.PlayedWeek
				case arenaSortWon:
					first, second = left.WonWeek, right.WonWeek
				case arenaSortSeasonPlayed:
					first, second = left.PlayedSeason, right.PlayedSeason
				case arenaSortSeasonWon:
					first, second = left.WonSeason, right.WonSeason
				default:
					first, second = left.PersonalRating, right.PersonalRating
				}
				if first != second {
					result = 1
					if second < first {
						result = -1
					}
				}
			}
			if result == 0 {
				continue
			}
			if order.back {
				result = -result
			}
			return result < 0
		}
		return false
	})
}

func (rt *Runtime) handleArenaPacket(packet world.Packet) bool {
	a := &rt.arena
	a.init()
	switch packet.Opcode {
	case world.ArenaTeamQueryResponse:
		if info, err := world.ParseArenaTeamQuery(packet.Body); err == nil {
			if slot := rt.arenaSlotOf(info.ID); slot >= 0 {
				a.teams[slot].info, a.teams[slot].known = info, true
				rt.FireEvent("ARENA_TEAM_UPDATE")
			}
		}
	case world.ArenaTeamRosterPacket:
		if roster, err := world.ParseArenaTeamRoster(packet.Body); err == nil {
			if slot := rt.arenaSlotOf(roster.ID); slot >= 0 {
				a.teams[slot].members = roster.Members
				rt.sortArenaRoster(slot)
				rt.FireEvent("ARENA_TEAM_ROSTER_UPDATE")
			}
		}
	case world.ArenaTeamStatsPacket:
		if stats, err := world.ParseArenaTeamStats(packet.Body); err == nil {
			if slot := rt.arenaSlotOf(stats.ID); slot >= 0 {
				a.teams[slot].stats = stats
				rt.FireEvent("ARENA_TEAM_UPDATE")
			}
		}
	case world.ArenaTeamInvitePacket:
		reader := world.NewReader(packet.Body, "SMSG_ARENA_TEAM_INVITE")
		inviter, err := reader.CString()
		if err != nil {
			return true
		}
		team, err := reader.CString()
		if err != nil {
			return true
		}
		rt.FireEvent("ARENA_TEAM_INVITE_REQUEST", lua.LString(inviter), lua.LString(team))
		rt.ShowGameError(arenaErrorInvite, inviter, inviter, team)
	case world.ArenaTeamEventPacket:
		rt.handleArenaEvent(packet.Body)
	default:
		return false
	}
	return true
}

// handleArenaEvent tells what happened to a team: somebody joined, left or was
// removed.
func (rt *Runtime) handleArenaEvent(body []byte) {
	reader := world.NewReader(body, "SMSG_ARENA_TEAM_EVENT")
	event, err := reader.U8()
	if err != nil {
		return
	}
	count, err := reader.U8()
	if err != nil {
		return
	}
	names := make([]any, 0, count)
	for index := byte(0); index < count; index++ {
		name, err := reader.CString()
		if err != nil {
			return
		}
		names = append(names, name)
	}
	index := arenaErrorOther
	switch event {
	case 3:
		index = 0x216
		if player := rt.unitInfo("player"); player != nil && len(names) > 1 && strings.EqualFold(player.Name, names[0].(string)) {
			index, names = 0x217, names[1:2]
		}
	case 4:
		index = 0x218
	case 5:
		index = 0x21B
	case 6:
		index = 0x219
	case 7:
		index = 0x21A
	case 8:
		index = 0x21C
	}
	if count > 3 {
		names = nil
	}
	rt.ShowGameError(index, names...)
	for slot := range rt.arena.teams {
		rt.arena.teams[slot].wait = time.Time{}
		rt.FireEvent("ARENA_TEAM_ROSTER_UPDATE", lua.LNumber(1))
	}
}

func registerArenaAPI(rt *Runtime) {
	L := rt.L
	a := &rt.arena
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	teamSlot := func(L *lua.LState) int {
		a.init()
		return L.CheckInt(1) - 1
	}
	teamID := func(slot int) uint32 {
		if slot < 0 || slot >= arenaTeamSlots {
			return 0
		}
		return a.teams[slot].id
	}
	num := func(L *lua.LState, values ...uint32) {
		for _, value := range values {
			L.Push(lua.LNumber(value))
		}
	}
	rgb := func(L *lua.LState, color uint32) {
		for _, shift := range []uint{16, 8, 0} {
			L.Push(lua.LNumber(float64(color>>shift&0xFF) / 255))
		}
	}
	reg("IsInArenaTeam", func(L *lua.LState) int {
		for slot := 0; slot < arenaTeamSlots; slot++ {
			if field, _ := rt.playerArenaField(slot); field.ID != 0 {
				L.Push(lua.LTrue)
				return 1
			}
		}
		L.Push(lua.LFalse)
		return 1
	})
	reg("IsArenaTeamCaptain", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: IsArenaTeamCaptain(index)")
		}
		slot := L.CheckInt(1) - 1
		if info := rt.unitInfo("player"); info != nil && slot >= 0 && slot < arenaTeamSlots && info.ArenaTeams[slot].Rank == 0 {
			L.Push(lua.LNumber(1))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("GetArenaTeam", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetArenaTeam(team)")
		}
		slot := teamSlot(L)
		field, _ := rt.playerArenaField(slot)
		if slot < 0 || slot >= arenaTeamSlots || field.ID == 0 || !a.teams[slot].known {
			L.Push(lua.LNil)
			for range 21 {
				L.Push(lua.LNumber(0))
			}
			return 22
		}
		team := &a.teams[slot]
		L.Push(lua.LString(team.info.Name))
		num(L, team.info.Size, team.stats.Rating, team.stats.Played, team.stats.Won, team.stats.SeasonPlayed, team.stats.SeasonWon,
			field.PlayedWeek, field.PlayedSeason, team.stats.Rank, field.PersonalRating)
		rgb(L, team.info.Background)
		num(L, team.info.EmblemStyle)
		rgb(L, team.info.EmblemColor)
		num(L, team.info.BorderStyle)
		rgb(L, team.info.BorderColor)
		return 22
	})
	reg("ArenaTeamRoster", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: ArenaTeamRoster(team)")
		}
		if slot := teamSlot(L); slot >= 0 && slot < arenaTeamSlots {
			rt.requestArenaRoster(slot)
		}
		return 0
	})
	reg("GetNumArenaTeamMembers", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetNumArenaTeamMembers(team, showOffline)")
		}
		slot := teamSlot(L)
		if slot < 0 || slot >= arenaTeamSlots {
			L.Push(lua.LNumber(0))
			return 1
		}
		count := 0
		for _, member := range a.teams[slot].members {
			if member.Online || a.showOffline || L.ToBool(2) {
				count++
			}
		}
		L.Push(lua.LNumber(count))
		return 1
	})
	reg("GetArenaTeamRosterInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetArenaTeamRosterInfo(team, index)")
		}
		slot, index := teamSlot(L), L.CheckInt(2)-1
		if slot < 0 || slot >= arenaTeamSlots || index < 0 || index >= len(a.teams[slot].members) {
			L.RaiseError("Usage: GetArenaTeamRosterInfo(team, index)")
		}
		member := a.teams[slot].members[index]
		L.Push(lua.LString(member.Name))
		num(L, member.Rank, uint32(member.Level))
		L.Push(lua.LString(rt.arenaClassName(member)))
		if member.Online {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		num(L, member.PlayedWeek, member.WonWeek, member.PlayedSeason, member.WonSeason, member.PersonalRating)
		return 10
	})
	reg("SetArenaTeamRosterSelection", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: SetArenaTeamRosterSelection(team, index)")
		}
		slot, index := teamSlot(L), L.CheckInt(2)-1
		a.selected = 0
		if slot >= 0 && slot < arenaTeamSlots && index >= 0 && index < len(a.teams[slot].members) {
			a.selected = a.teams[slot].members[index].GUID
		}
		return 0
	})
	reg("GetArenaTeamRosterSelection", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetArenaTeamRosterSelection(team)")
		}
		slot := teamSlot(L)
		if slot >= 0 && slot < arenaTeamSlots && a.selected != 0 {
			for index, member := range a.teams[slot].members {
				if member.GUID == a.selected {
					L.Push(lua.LNumber(index + 1))
					return 1
				}
			}
		}
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("SortArenaTeamRoster", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError(`Usgae: SortArenaTeamRoster("type")`)
		}
		a.init()
		key := arenaSortName
		switch strings.ToLower(L.CheckString(1)) {
		case "class":
			key = arenaSortClass
		case "played":
			key = arenaSortPlayed
		case "win":
			key = arenaSortWon
		case "seasonplayed":
			key = arenaSortSeasonPlayed
		case "seasonwon":
			key = arenaSortSeasonWon
		case "rating":
			key = arenaSortRating
		}
		position := 0
		for index, order := range a.sorts {
			if order.key == key {
				position = index
			}
		}
		order := a.sorts[position]
		if position == 0 {
			order.back = !order.back
		} else {
			copy(a.sorts[1:position+1], a.sorts[:position])
		}
		a.sorts[0] = order
		for slot := range a.teams {
			if len(a.teams[slot].members) > 0 {
				rt.sortArenaRoster(slot)
				rt.FireEvent("ARENA_TEAM_ROSTER_UPDATE")
			}
		}
		return 0
	})
	reg("CloseArenaTeamRoster", func(L *lua.LState) int { return 0 })
	reg("AcceptArenaTeam", func(L *lua.LState) int {
		rt.send(world.ClientArenaTeamAccept, nil)
		return 0
	})
	reg("DeclineArenaTeam", func(L *lua.LState) int {
		rt.send(world.ClientArenaTeamDecline, nil)
		return 0
	})
	byTeam := func(name string, opcode world.ClientOpcode) {
		reg(name, func(L *lua.LState) int {
			if L.Get(1).Type() != lua.LTNumber {
				L.RaiseError("Usage: %s(team)", name)
			}
			rt.send(opcode, world.BuildArenaTeamID(teamID(teamSlot(L))))
			return 0
		})
	}
	byTeam("ArenaTeamLeave", world.ClientArenaTeamLeave)
	byTeam("ArenaTeamDisband", world.ClientArenaTeamDisband)
	byName := func(name string, opcode world.ClientOpcode, allowEmpty bool) {
		reg(name, func(L *lua.LState) int {
			if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTString {
				L.RaiseError("Usage: %s(team, name)", name)
			}
			player := L.CheckString(2)
			if len(player) > 0x30 {
				L.RaiseError("Name too long")
			}
			if player == "" && !allowEmpty {
				return 0
			}
			rt.send(opcode, world.BuildArenaTeamName(teamID(teamSlot(L)), player))
			return 0
		})
	}
	byName("ArenaTeamInviteByName", world.ClientArenaTeamInvite, false)
	byName("ArenaTeamUninviteByName", world.ClientArenaTeamRemove, true)
	byName("ArenaTeamSetLeaderByName", world.ClientArenaTeamSetLeader, true)
}
