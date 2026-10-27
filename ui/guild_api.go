package ui

import (
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// handleGuildPacket takes the packets of the guild.
func (rt *Runtime) handleGuildPacket(packet world.Packet) bool {
	switch packet.Opcode {
	case world.GuildQueryResponse:
		if query, err := world.ParseGuildQuery(packet.Body); err == nil {
			rt.SetGuildQuery(query)
		}
		return false
	case world.GuildRoster:
		if roster, err := world.ParseGuildRoster(packet.Body); err == nil {
			rt.SetGuildRoster(roster)
		}
	case world.GuildInvite:
		if invitation, err := world.ParseGuildInvite(packet.Body); err == nil {
			rt.SetGuildInvite(invitation)
		}
	case world.GuildDecline:
		if name, err := world.NewReader(packet.Body, "SMSG_GUILD_DECLINE").CString(); err == nil {
			rt.ShowError(strings.Replace(rt.globalString("ERR_GUILD_DECLINE_S", "%s declines your guild invitation."), "%s", name, 1))
		}
	case world.GuildInfo:
		if info, err := world.ParseGuildInfo(packet.Body); err == nil {
			rt.SetGuildSummary(info)
		}
	case world.GuildEvent:
		if event, err := world.ParseGuildEvent(packet.Body); err == nil {
			rt.ApplyGuildEvent(event)
		}
	case world.GuildCommandResult:
		if command, err := world.ParseGuildCommandResult(packet.Body); err == nil {
			rt.ShowGuildResult(command)
		}
	case world.GuildEventLogResult:
		if entries, err := world.ParseGuildEventLog(packet.Body); err == nil {
			rt.SetGuildEventLog(entries)
		}
	default:
		return false
	}
	return true
}

// registerGuildAPI defines the calls of the guild window and the rank editor.
func registerGuildAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	push := func(L *lua.LState, values ...lua.LValue) int {
		for _, value := range values {
			L.Push(value)
		}
		return len(values)
	}
	right := func(bit uint32) lua.LGFunction {
		return func(L *lua.LState) int { return push(L, luaFlag(rt.ownGuildRights()&bit != 0 && rt.guild.id != 0)) }
	}
	reg("IsInGuild", func(L *lua.LState) int { return push(L, luaFlag(rt.guild.id != 0)) })
	reg("CanGuildInvite", right(guildRightInvite))
	reg("CanGuildPromote", right(guildRightPromote))
	reg("CanGuildDemote", right(guildRightDemote))
	reg("CanGuildRemove", right(guildRightRemove))
	reg("CanEditMOTD", right(guildRightSetMotd))
	reg("CanEditGuildInfo", right(guildRightEditInfo))
	reg("CanEditPublicNote", right(guildRightEditPublic))
	reg("CanEditOfficerNote", right(guildRightEditOfficer))
	reg("CanViewOfficerNote", right(guildRightViewOfficer))
	reg("CanEditGuildEvent", right(guildRightCreateEvent))
	reg("GetGuildInfo", func(L *lua.LState) int {
		unit := L.CheckString(1)
		info := rt.unitInfo(unit)
		if info == nil || info.GuildName == "" {
			if rt.isSelf(unit) && rt.guild.id != 0 && rt.guild.query.Name != "" {
				return push(L, lua.LString(rt.guild.query.Name), lua.LString(rt.guildRankName(rt.guild.rank)), lua.LNumber(rt.guild.rank))
			}
			return push(L, lua.LNil, lua.LNil, lua.LNil)
		}
		return push(L, lua.LString(info.GuildName), lua.LString(info.GuildRank), lua.LNumber(info.GuildRankIndex))
	})
	reg("GuildRoster", func(L *lua.LState) int { rt.send(world.ClientGuildRoster, nil); return 0 })
	reg("GuildInfo", func(L *lua.LState) int { rt.send(world.ClientGuildInfo, nil); return 0 })
	reg("GetNumGuildMembers", func(L *lua.LState) int {
		if rt.guild.showOffline || L.Get(1) != lua.LNil && L.Get(1) != lua.LFalse {
			return push(L, lua.LNumber(len(rt.guild.members)))
		}
		return push(L, lua.LNumber(rt.guildOnline()))
	})
	reg("GetGuildRosterShowOffline", func(L *lua.LState) int { return push(L, luaFlag(rt.guild.showOffline)) })
	reg("SetGuildRosterShowOffline", func(L *lua.LState) int {
		show := L.Get(1) != lua.LNil && L.Get(1) != lua.LFalse && L.Get(1) != lua.LNumber(0)
		if show != rt.guild.showOffline {
			rt.guild.showOffline = show
			rt.sortGuildRoster()
			rt.FireEvent("GUILD_ROSTER_UPDATE")
		}
		return 0
	})
	reg("GetGuildRosterSelection", func(L *lua.LState) int { return push(L, lua.LNumber(rt.guild.selected)) })
	reg("SetGuildRosterSelection", func(L *lua.LState) int { rt.guild.selected = L.CheckInt(1); return 0 })
	reg("GetGuildRosterMOTD", func(L *lua.LState) int { return push(L, lua.LString(rt.guild.roster.Welcome)) })
	reg("GetGuildInfoText", func(L *lua.LState) int { return push(L, lua.LString(rt.guild.roster.Info)) })
	reg("SetGuildInfoText", func(L *lua.LState) int {
		rt.send(world.ClientGuildInfoText, world.BuildGuildText(L.CheckString(1)))
		return 0
	})
	reg("GuildSetMOTD", func(L *lua.LState) int {
		rt.send(world.ClientGuildMotd, world.BuildGuildText(L.CheckString(1)))
		return 0
	})
	member := func(index int) *guildMember {
		if index < 1 || index > len(rt.guild.sorted) {
			return nil
		}
		return rt.guild.sorted[index-1]
	}
	reg("GetGuildRosterInfo", func(L *lua.LState) int {
		entry := member(L.CheckInt(1))
		if entry == nil {
			return push(L, lua.LNil, lua.LNil, lua.LNumber(0), lua.LNumber(0), lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LString(""), lua.LNil)
		}
		status := ""
		switch {
		case entry.Status&world.GuildMemberDND != 0:
			status = rt.globalString("CHAT_FLAG_DND", "<DND>")
		case entry.Status&world.GuildMemberAFK != 0:
			status = rt.globalString("CHAT_FLAG_AFK", "<Away>")
		}
		officer := ""
		if rt.ownGuildRights()&guildRightViewOfficer != 0 {
			officer = entry.OfficerNote
		}
		return push(L, lua.LString(entry.Name), lua.LString(rt.guildRankName(int(entry.Rank))), lua.LNumber(entry.Rank), lua.LNumber(entry.Level),
			lua.LString(entry.className), lua.LString(entry.zone), lua.LString(entry.Note), lua.LString(officer),
			luaFlag(entry.Status&world.GuildMemberOnline != 0), lua.LString(status), lua.LString(entry.classFile))
	})
	reg("GetGuildRosterLastOnline", func(L *lua.LState) int {
		entry := member(L.CheckInt(1))
		if entry == nil || entry.Status&world.GuildMemberOnline != 0 {
			return push(L, lua.LNil)
		}
		hours := int(float64(entry.LastOnline) * 24)
		return push(L, lua.LNumber(hours/8760), lua.LNumber(hours%8760/720), lua.LNumber(hours%720/24), lua.LNumber(hours%24))
	})
	reg("SortGuildRoster", func(L *lua.LState) int {
		key := guildSortName
		switch strings.ToLower(L.CheckString(1)) {
		case "level":
			key = guildSortLevel
		case "class":
			key = guildSortClass
		case "group":
			key = guildSortGroup
		case "rank":
			key = guildSortRank
		case "zone":
			key = guildSortZone
		case "online":
			key = guildSortOnline
		case "note":
			key = guildSortNote
		}
		rt.setGuildSort(key)
		return 0
	})
	note := func(opcode world.ClientOpcode) lua.LGFunction {
		return func(L *lua.LState) int {
			if entry := member(L.CheckInt(1)); entry != nil {
				rt.send(opcode, world.BuildGuildNote(entry.Name, L.CheckString(2)))
			}
			return 0
		}
	}
	reg("GuildRosterSetPublicNote", note(world.ClientGuildPublicNote))
	reg("GuildRosterSetOfficerNote", note(world.ClientGuildOfficerNte))
	named := func(opcode world.ClientOpcode) lua.LGFunction {
		return func(L *lua.LState) int {
			name := L.CheckString(1)
			if info := rt.unitInfo(name); info != nil && info.Name != "" {
				name = info.Name
			}
			rt.send(opcode, world.BuildGuildText(name))
			return 0
		}
	}
	reg("GuildInvite", named(world.ClientGuildInvite))
	reg("GuildPromote", named(world.ClientGuildPromote))
	reg("GuildDemote", named(world.ClientGuildDemote))
	reg("GuildUninvite", named(world.ClientGuildRemove))
	reg("GuildSetLeader", named(world.ClientGuildLeader))
	reg("GuildLeave", func(L *lua.LState) int { rt.send(world.ClientGuildLeave, nil); return 0 })
	reg("GuildDisband", func(L *lua.LState) int { rt.send(world.ClientGuildDisband, nil); return 0 })
	reg("AcceptGuild", func(L *lua.LState) int {
		rt.send(world.ClientGuildAccept, nil)
		rt.guild.invited = false
		return 0
	})
	reg("DeclineGuild", func(L *lua.LState) int {
		rt.send(world.ClientGuildDecline, nil)
		rt.guild.invited = false
		return 0
	})

	// The rank editor.
	reg("GuildControlGetNumRanks", func(L *lua.LState) int { return push(L, lua.LNumber(len(rt.guild.roster.Ranks))) })
	reg("GuildControlGetRankName", func(L *lua.LState) int {
		return push(L, lua.LString(rt.guildRankName(L.CheckInt(1)-1)))
	})
	reg("GuildControlSetRank", func(L *lua.LState) int {
		state := &rt.guild
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(state.roster.Ranks) {
			state.controlRank = -1
			return 0
		}
		rank := state.roster.Ranks[index]
		state.controlRank = index
		for flag, bit := range guildControlFlags {
			state.controlFlags[flag] = rank.Flags&bit != 0
		}
		state.tabRights, state.tabLimit, state.goldLimit = rank.TabFlags, rank.TabLimit, rank.GoldLimit
		return 0
	})
	reg("GuildControlSetRankFlag", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		if index >= 0 && index < len(rt.guild.controlFlags) {
			rt.guild.controlFlags[index] = L.Get(2) != lua.LNil && L.Get(2) != lua.LFalse && L.Get(2) != lua.LNumber(0)
		}
		return 0
	})
	reg("GuildControlGetRankFlags", func(L *lua.LState) int {
		for _, set := range rt.guild.controlFlags {
			L.Push(lua.LBool(set))
		}
		return len(rt.guild.controlFlags)
	})
	reg("GuildControlSaveRank", func(L *lua.LState) int {
		state := &rt.guild
		name := L.CheckString(1)
		if name == "" || state.controlRank < 0 || state.controlRank >= len(state.roster.Ranks) {
			return 0
		}
		if len(name) > 15 {
			name = name[:15]
		}
		rank := state.roster.Ranks[state.controlRank]
		flags := rank.Flags
		for flag, bit := range guildControlFlags {
			if state.controlFlags[flag] {
				flags |= bit
			} else {
				flags &^= bit
			}
		}
		rank.Flags, rank.GoldLimit, rank.TabFlags, rank.TabLimit = flags, state.goldLimit, state.tabRights, state.tabLimit
		rt.send(world.ClientGuildRank, world.BuildGuildRank(uint32(state.controlRank), rank, name))
		return 0
	})
	reg("GuildControlAddRank", func(L *lua.LState) int {
		if name := L.CheckString(1); name != "" && len(rt.guild.roster.Ranks) < 10 {
			rt.send(world.ClientGuildAddRank, world.BuildGuildText(name))
		}
		return 0
	})
	reg("GuildControlDelRank", func(L *lua.LState) int {
		if len(rt.guild.roster.Ranks) > 5 {
			rt.send(world.ClientGuildDelRank, nil)
		}
		return 0
	})
	reg("GetGuildBankWithdrawLimit", func(L *lua.LState) int {
		limit := rt.guild.goldLimit
		if limit == 0xFFFFFFFF {
			return push(L, lua.LNumber(-1))
		}
		return push(L, lua.LNumber(limit/10000))
	})
	reg("SetGuildBankWithdrawLimit", func(L *lua.LState) int {
		limit := L.CheckInt(1)
		if limit < 0 {
			rt.guild.goldLimit = 0xFFFFFFFF
		} else {
			rt.guild.goldLimit = uint32(limit) * 10000
		}
		return 0
	})
	tab := func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		if index < 0 || index >= 6 {
			return -1
		}
		return index
	}
	reg("GetGuildBankTabPermissions", func(L *lua.LState) int {
		index := tab(L)
		if index < 0 {
			return 0
		}
		flags := rt.guild.tabRights[index]
		return push(L, lua.LBool(flags&guildBankViewTab != 0), lua.LBool(flags&guildBankPutItem != 0), lua.LBool(flags&guildBankUpdateText != 0), lua.LNumber(int32(rt.guild.tabLimit[index])))
	})
	reg("SetGuildBankTabPermissions", func(L *lua.LState) int {
		index := tab(L)
		if index < 0 {
			return 0
		}
		bits := [...]uint32{guildBankViewTab, guildBankPutItem, guildBankUpdateText}
		permission := L.CheckInt(2) - 1
		if permission >= 0 && permission < len(bits) {
			if L.Get(3) != lua.LNil && L.Get(3) != lua.LFalse && L.Get(3) != lua.LNumber(0) {
				rt.guild.tabRights[index] |= bits[permission]
			} else {
				rt.guild.tabRights[index] &^= bits[permission]
			}
		}
		return 0
	})
	reg("SetGuildBankTabWithdraw", func(L *lua.LState) int {
		if index := tab(L); index >= 0 {
			rt.guild.tabLimit[index] = uint32(L.CheckInt(2))
		}
		return 0
	})

	// Event log.
	reg("QueryGuildEventLog", func(L *lua.LState) int { rt.send(world.ClientGuildEventLog, nil); return 0 })
	reg("GetNumGuildEvents", func(L *lua.LState) int { return push(L, lua.LNumber(len(rt.guild.log))) })
	reg("GetGuildEventInfo", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		if index < 0 || index >= len(rt.guild.log) {
			return push(L, lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LNumber(0), lua.LNumber(0), lua.LNumber(0), lua.LNumber(0))
		}
		line := rt.guild.log[index]
		kinds := map[uint8]string{1: "invite", 2: "join", 3: "promote", 4: "demote", 5: "remove", 6: "quit"}
		other := lua.LValue(lua.LNil)
		if line.Other != 0 {
			other = lua.LString(rt.nameOf(line.Other))
		}
		rank := lua.LValue(lua.LNil)
		if line.Type == 3 || line.Type == 4 {
			rank = lua.LString(rt.guildRankName(int(line.Rank)))
		}
		hours := int(time.Now().Unix()-line.arrived+int64(line.Time)) / 3600
		return push(L, lua.LString(kinds[line.Type]), lua.LString(rt.nameOf(line.Player)), other, rank,
			lua.LNumber(hours/8760), lua.LNumber(hours%8760/720), lua.LNumber(hours%720/24), lua.LNumber(hours%24))
	})
	reg("GetGuildTabardFileNames", func(L *lua.LState) int {
		a, b, c, d, e, f := rt.guildTabardFiles()
		if a == "" {
			return push(L, lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LNil)
		}
		return push(L, lua.LString(a), lua.LString(b), lua.LString(c), lua.LString(d), lua.LString(e), lua.LString(f))
	})
}

// registerGuildPackets adds the guild to the packet routing.
func (rt *Runtime) registerGuildPackets() { rt.addPacketHandler(rt.handleGuildPacket) }
