package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Guild rights of a rank (GuildRankRights).
const (
	guildRightChatListen    = 0x00000001
	guildRightChatSpeak     = 0x00000002
	guildRightOfficerListen = 0x00000004
	guildRightOfficerSpeak  = 0x00000008
	guildRightInvite        = 0x00000010
	guildRightRemove        = 0x00000020
	guildRightPromote       = 0x00000080
	guildRightDemote        = 0x00000100
	guildRightSetMotd       = 0x00001000
	guildRightEditPublic    = 0x00002000
	guildRightViewOfficer   = 0x00004000
	guildRightEditOfficer   = 0x00008000
	guildRightEditInfo      = 0x00010000
	guildRightCreateEvent   = 0x00100000
)

// guildControlFlags are the rights the rank editor lists, in its order.
var guildControlFlags = [...]uint32{
	0x1, 0x2, 0x4, 0x8, 0x80, 0x100, 0x10, 0x20, 0x1000, 0x2000, 0x4000, 0x8000, 0x10000, 0x20000, 0x40000, 0x80000, 0x100000,
}

// guild roster sort keys.
const (
	guildSortRank   = 0
	guildSortLevel  = 1
	guildSortName   = 2
	guildSortZone   = 3
	guildSortClass  = 4
	guildSortGroup  = 5
	guildSortOnline = 6
	guildSortNote   = 7
)

type guildSort struct {
	key     int
	reverse bool
}

type guildMember struct {
	world.GuildRosterMember
	className string
	classFile string
	zone      string
	arrived   float64
}

type guildLogLine struct {
	world.GuildLogEntry
	arrived int64
}

type guildState struct {
	id      uint32
	rank    int
	known   bool
	query   world.GuildQuery
	roster  world.GuildRosterInfo
	members []*guildMember
	sorted  []*guildMember
	sorts   [8]guildSort

	showOffline bool
	selected    int
	invite      world.GuildInvitation
	invited     bool
	summary     world.GuildSummary
	log         []guildLogLine
	loaded      bool

	// controlRank is the rank the rank editor works on and controlFlags the
	// rights it has set, controlGold the gold limit per day.
	controlRank  int
	controlFlags [len(guildControlFlags)]bool
	tabRights    [6]uint32
	tabLimit     [6]uint32
	goldLimit    uint32
	emblemKnown  bool
}

func (rt *Runtime) initGuild() {
	rt.guild.sorts = [8]guildSort{{key: guildSortName}, {key: guildSortRank}, {key: guildSortLevel}, {key: guildSortZone}, {key: guildSortClass}, {key: guildSortGroup}, {key: guildSortOnline}, {key: guildSortNote}}
	rt.guild.controlRank = -1
}

// SetGuildFields takes the guild the player's fields name: the guild id and the
// rank in it.
func (rt *Runtime) SetGuildFields(id uint32, rank int) {
	state := &rt.guild
	if id == state.id && rank == state.rank && state.known {
		return
	}
	changedGuild := id != state.id || !state.known
	state.id, state.rank, state.known = id, rank, true
	if changedGuild {
		state.roster, state.members, state.sorted = world.GuildRosterInfo{}, nil, nil
		state.query, state.loaded = world.GuildQuery{}, false
		if id != 0 {
			rt.send(world.GuildQueryOpcode, world.BuildGuildQuery(id))
			rt.send(world.ClientGuildInfo, nil)
			rt.send(world.ClientGuildRoster, nil)
		}
	}
	rt.FireEvent("PLAYER_GUILD_UPDATE", lua.LString("player"))
	rt.FireEvent("GUILD_ROSTER_UPDATE")
}

// SetGuildQuery takes the answer to a guild query; it is the player's own guild
// that the rank names are kept for.
func (rt *Runtime) SetGuildQuery(query world.GuildQuery) {
	if query.ID == rt.guild.id {
		rt.guild.query, rt.guild.emblemKnown = query, true
		rt.FireEvent("GUILD_ROSTER_UPDATE")
	}
}

// guildRankName names a rank of the player's guild.
func (rt *Runtime) guildRankName(rank int) string {
	if rank < 0 || rank >= len(rt.guild.query.Ranks) {
		return ""
	}
	return rt.guild.query.Ranks[rank]
}

// SetGuildRoster takes the roster the server sent.
func (rt *Runtime) SetGuildRoster(roster world.GuildRosterInfo) {
	state := &rt.guild
	state.roster, state.loaded = roster, true
	state.members = state.members[:0]
	for index := range roster.Members {
		member := &guildMember{GuildRosterMember: roster.Members[index]}
		member.classFile = ClassFileFromID(int(member.Class))
		member.className = rt.classNameFor(int(member.Class), int(member.Gender))
		member.zone = rt.zoneName(uint32(member.Area))
		state.members = append(state.members, member)
		rt.requestName(member.GUID)
	}
	if state.selected > len(state.members) {
		state.selected = 0
	}
	rt.sortGuildRoster()
	rt.FireEvent("GUILD_ROSTER_UPDATE")
}

// requestName asks for the name of a player a list names by guid.
func (rt *Runtime) requestName(guid uint64) {
	if host, ok := rt.Host.(interface{ RequestName(uint64) }); ok {
		host.RequestName(guid)
	}
}

// classNameFor is the class name in the language of the character.
func (rt *Runtime) classNameFor(class, gender int) string {
	return rt.unitClassText(&UnitInfo{ClassID: class, ClassFile: ClassFileFromID(class), Sex: gender + 2})
}

func guildLess(rt *Runtime, a, b *guildMember, key int) int {
	compare := func(x, y string) int { return strings.Compare(strings.ToLower(x), strings.ToLower(y)) }
	switch key {
	case guildSortRank:
		switch {
		case a.Rank < b.Rank:
			return -1
		case a.Rank > b.Rank:
			return 1
		}
	case guildSortLevel:
		switch {
		case a.Level > b.Level:
			return -1
		case a.Level < b.Level:
			return 1
		}
	case guildSortName:
		return compare(a.Name, b.Name)
	case guildSortZone:
		return compare(a.zone, b.zone)
	case guildSortClass:
		return compare(a.className, b.className)
	case guildSortOnline:
		aOnline, bOnline := a.Status&world.GuildMemberOnline != 0, b.Status&world.GuildMemberOnline != 0
		if aOnline != bOnline {
			if aOnline {
				return -1
			}
			return 1
		}
		switch {
		case a.LastOnline < b.LastOnline:
			return -1
		case a.LastOnline > b.LastOnline:
			return 1
		}
	case guildSortNote:
		return compare(a.Note, b.Note)
	}
	return 0
}

// sortGuildRoster orders the members by the sort keys, the newest first, and
// when offline members are hidden puts the online ones in front.
func (rt *Runtime) sortGuildRoster() {
	state := &rt.guild
	state.sorted = append(state.sorted[:0], state.members...)
	sort.SliceStable(state.sorted, func(a, b int) bool {
		first, second := state.sorted[a], state.sorted[b]
		if !state.showOffline {
			firstOnline, secondOnline := first.Status&world.GuildMemberOnline != 0, second.Status&world.GuildMemberOnline != 0
			if firstOnline != secondOnline {
				return firstOnline
			}
		}
		for _, sorter := range state.sorts {
			if result := guildLess(rt, first, second, sorter.key); result != 0 {
				if sorter.reverse {
					return result > 0
				}
				return result < 0
			}
		}
		return false
	})
}

// setGuildSort makes a key the first to sort by, reversing it when it already
// is.
func (rt *Runtime) setGuildSort(key int) {
	state := &rt.guild
	index := -1
	for position, sorter := range state.sorts {
		if sorter.key == key {
			index = position
			break
		}
	}
	if index < 0 {
		return
	}
	reverse := state.sorts[index].reverse
	if index == 0 {
		reverse = !reverse
	} else {
		copy(state.sorts[1:index+1], state.sorts[:index])
	}
	state.sorts[0] = guildSort{key: key, reverse: reverse}
	rt.sortGuildRoster()
	rt.FireEvent("GUILD_ROSTER_UPDATE")
}

// guildOnline is how many members are online.
func (rt *Runtime) guildOnline() int {
	count := 0
	for _, member := range rt.guild.members {
		if member.Status&world.GuildMemberOnline != 0 {
			count++
		}
	}
	return count
}

// ownGuildRights are the rights of the character's rank.
func (rt *Runtime) ownGuildRights() uint32 {
	state := &rt.guild
	if state.id == 0 {
		return 0
	}
	if state.loaded && state.rank >= 0 && state.rank < len(state.roster.Ranks) {
		return state.roster.Ranks[state.rank].Flags
	}
	return rt.guildBank.rights
}

// SetGuildInvite records an invitation to a guild.
func (rt *Runtime) SetGuildInvite(invitation world.GuildInvitation) {
	rt.guild.invite, rt.guild.invited = invitation, true
	rt.FireEvent("GUILD_INVITE_REQUEST", lua.LString(invitation.Inviter), lua.LString(invitation.Guild))
}

// SetGuildSummary takes SMSG_GUILD_INFO.
func (rt *Runtime) SetGuildSummary(info world.GuildSummary) { rt.guild.summary = info }

// guildCommandErrors name the message of a failed guild command.
var guildCommandErrors = map[int32]string{
	1: "ERR_GUILD_INTERNAL", 2: "ERR_ALREADY_IN_GUILD", 3: "ERR_ALREADY_IN_GUILD_S", 4: "ERR_INVITED_TO_GUILD", 5: "ERR_ALREADY_INVITED_TO_GUILD_S",
	6: "ERR_GUILD_NAME_INVALID", 7: "ERR_GUILD_NAME_EXISTS_S", 8: "ERR_GUILD_PERMISSIONS", 9: "ERR_GUILD_PLAYER_NOT_IN_GUILD",
	10: "ERR_GUILD_PLAYER_NOT_IN_GUILD_S", 11: "ERR_GUILD_PLAYER_NOT_FOUND_S", 12: "ERR_GUILD_NOT_ALLIED", 13: "ERR_GUILD_RANK_TOO_HIGH_S",
	14: "ERR_GUILD_RANK_TOO_LOW_S", 17: "ERR_GUILD_RANKS_LOCKED", 18: "ERR_GUILD_RANK_IN_USE", 19: "ERR_GUILD_IGNORING_YOU_S",
	25: "ERR_GUILD_WITHDRAW_LIMIT", 26: "ERR_GUILD_NOT_ENOUGH_MONEY", 28: "ERR_GUILD_BANK_FULL", 29: "ERR_GUILD_ITEM_NOT_FOUND",
}

// ShowGuildResult shows why a guild command failed.
func (rt *Runtime) ShowGuildResult(command world.GuildCommand) {
	if command.Result == 0 {
		return
	}
	if command.Result == 20 {
		rt.send(world.ClientGuildRoster, nil)
		return
	}
	if text, ok := rt.L.GetGlobal(guildCommandErrors[command.Result]).(lua.LString); ok {
		rt.ShowError(strings.Replace(string(text), "%s", command.Name, 1))
	}
}

// ApplyGuildEvent writes a guild event to the chat and asks for the roster
// again when the members changed.
func (rt *Runtime) ApplyGuildEvent(event world.GuildEventInfo) {
	param := func(index int) string {
		if index < len(event.Params) {
			return event.Params[index]
		}
		return ""
	}
	format := func(name, fallback string, args ...interface{}) string {
		return fmt.Sprintf(rt.globalString(name, fallback), args...)
	}
	var text string
	refresh := true
	switch event.Type {
	case world.GuildEventPromotion:
		text = format("ERR_GUILD_PROMOTE_SSS", "%s has promoted %s to %s.", param(0), param(1), param(2))
	case world.GuildEventDemotion:
		text = format("ERR_GUILD_DEMOTE_SSS", "%s has demoted %s to %s.", param(0), param(1), param(2))
	case world.GuildEventMotd:
		text = format("GUILD_MOTD_TEMPLATE", "Guild Message of the Day: %s", param(0))
		rt.guild.roster.Welcome = param(0)
		rt.FireEvent("GUILD_MOTD", lua.LString(param(0)))
	case world.GuildEventJoined:
		text = format("ERR_GUILD_JOIN_S", "%s has joined the guild.", param(0))
	case world.GuildEventLeft:
		text = format("ERR_GUILD_LEAVE_S", "%s has left the guild.", param(0))
	case world.GuildEventRemoved:
		text = format("ERR_GUILD_REMOVE_SS", "%s has been kicked out of the guild by %s.", param(0), param(1))
	case world.GuildEventLeaderIs:
		text = format("ERR_GUILD_LEADER_IS_S", "%s is the leader of your guild.", param(0))
	case world.GuildEventLeaderNew:
		text = format("ERR_GUILD_LEADER_CHANGED_SS", "%s has made %s the new Guild Master.", param(0), param(1))
	case world.GuildEventDisbanded:
		text = rt.globalString("ERR_GUILD_DISBANDED", "Guild disbanded.")
	case world.GuildEventSignedOn:
		text = format("ERR_FRIEND_ONLINE_SS", "|Hplayer:%s|h[%s]|h has come online.", param(0), param(0))
		text = strings.Replace(text, "|Hplayer:"+param(0), "|Hplayer:"+param(0), 1)
	case world.GuildEventSignedOff:
		text = format("ERR_FRIEND_OFFLINE_S", "%s has gone offline.", param(0))
	default:
		refresh = true
	}
	if event.Type == world.GuildEventSignedOn || event.Type == world.GuildEventSignedOff {
		rt.say("CHAT_MSG_SYSTEM", text)
	} else {
		rt.say("CHAT_MSG_SYSTEM", text)
	}
	if refresh {
		rt.send(world.ClientGuildRoster, nil)
	}
	rt.FireEvent("GUILD_ROSTER_UPDATE", lua.LTrue)
}

// SetGuildEventLog takes the guild's event log.
func (rt *Runtime) SetGuildEventLog(entries []world.GuildLogEntry) {
	rt.guild.log = rt.guild.log[:0]
	for _, entry := range entries {
		rt.guild.log = append(rt.guild.log, guildLogLine{GuildLogEntry: entry, arrived: rt.guildNow()})
		rt.requestName(entry.Player)
		if entry.Other != 0 {
			rt.requestName(entry.Other)
		}
	}
	rt.FireEvent("GUILD_EVENT_LOG_UPDATE")
}

// guildNow is the time the client counts guild times from, in seconds.
func (rt *Runtime) guildNow() int64 { return time.Now().Unix() }

// guildTabardFiles names the textures of the emblem of the guild.
func (rt *Runtime) guildTabardFiles() (string, string, string, string, string, string) {
	query := rt.guild.query
	if !rt.guild.emblemKnown {
		return "", "", "", "", "", ""
	}
	background := func(part string) string {
		return fmt.Sprintf(`Textures\GuildEmblems\Background_%02d_%s`, query.BackgroundColor, part)
	}
	emblem := func(part string) string {
		return fmt.Sprintf(`Textures\GuildEmblems\Emblem_%02d_%02d_%s`, query.EmblemStyle, query.EmblemColor, part)
	}
	border := func(part string) string {
		return fmt.Sprintf(`Textures\GuildEmblems\Border_%02d_%02d_%s`, query.BorderStyle, query.BorderColor, part)
	}
	return background("TU_U"), background("TL_U"), emblem("TU_U"), emblem("TL_U"), border("TU_U"), border("TL_U")
}
