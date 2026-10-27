package ui

import (
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// ChatLine writes a line of a chat kind (an event such as CHAT_MSG_SYSTEM) to
// the chat windows.
func (rt *Runtime) ChatLine(event, text string) {
	if text == "" || !strings.HasPrefix(event, "CHAT_MSG_") {
		return
	}
	rt.FireEvent(event,
		lua.LString(text), lua.LString(""), lua.LString(""), lua.LString(""), lua.LString(""), lua.LString(""),
		lua.LNumber(0), lua.LNumber(0), lua.LString(""), lua.LNumber(0), lua.LNumber(rt.nextChatLine()), lua.LString(""))
}

// say writes a line to the chat windows.
func (rt *Runtime) say(event, text string) { rt.ChatLine(event, text) }

// HandlePacket takes the server packets of the systems the interface drives
// itself (groups, guilds, flight paths, pets and so on); it reports whether
// the packet was one of theirs.
func (rt *Runtime) HandlePacket(packet world.Packet) bool {
	for _, handler := range rt.packetHandlers {
		if handler(packet) {
			return true
		}
	}
	return false
}

// addPacketHandler registers the handler of one system.
func (rt *Runtime) addPacketHandler(handler func(world.Packet) bool) {
	rt.packetHandlers = append(rt.packetHandlers, handler)
}

func (rt *Runtime) registerPacketHandlers() {
	rt.addPacketHandler(rt.handleGroupPacket)
	rt.addPacketHandler(rt.handleTradePacket)
	rt.addPacketHandler(rt.handleMiscPacket)
	rt.addPacketHandler(rt.handleChatPacket)
	rt.addPacketHandler(rt.handleServerTextPacket)
	rt.addPacketHandler(rt.handleRunePacket)
	rt.addPacketHandler(rt.handleEquipmentPacket)
	rt.addPacketHandler(rt.handleWorldQueryPacket)
	rt.addPacketHandler(rt.handlePetitionPacket)
}

// handleGroupPacket takes the packets of groups, raids and loot rolls.
func (rt *Runtime) handleGroupPacket(packet world.Packet) bool {
	switch packet.Opcode {
	case world.GroupList:
		if info, err := world.ParseGroupList(packet.Body); err == nil {
			rt.SetGroup(info)
		}
	case world.GroupInvite:
		if invitation, err := world.ParseGroupInvite(packet.Body); err == nil {
			rt.SetGroupInvite(invitation)
		}
	case world.GroupCancel:
		rt.CancelGroupInvite()
	case world.GroupDecline:
		name := world.NewReader(packet.Body, "SMSG_GROUP_DECLINE")
		if text, err := name.CString(); err == nil {
			rt.ShowError(fmt.Sprintf(rt.globalString("ERR_INVITE_DECLINED_S", "%s declines your group invitation."), text))
		}
	case world.GroupUninvite:
		rt.ShowGlobalError("ERR_UNINVITE_YOU")
	case world.GroupDestroyed:
		rt.SetGroup(world.GroupInfo{})
	case world.GroupSetLeaderResponse:
		if name, err := world.NewReader(packet.Body, "SMSG_GROUP_SET_LEADER").CString(); err == nil {
			rt.say("CHAT_MSG_SYSTEM", fmt.Sprintf(rt.globalString("ERR_NEW_LEADER_S", "%s is now the group leader."), name))
		}
	case world.PartyCommandResult:
		if result, err := world.ParsePartyCommandResult(packet.Body); err == nil {
			rt.ShowPartyResult(result)
		}
	case world.PartyMemberStats:
		if stats, err := world.ParsePartyMemberStats(packet.Body, false); err == nil {
			rt.ApplyMemberStats(stats)
		}
	case world.PartyMemberStatsFull:
		if stats, err := world.ParsePartyMemberStats(packet.Body, true); err == nil {
			rt.ApplyMemberStats(stats)
		}
	case world.RaidTargetUpdate:
		if update, err := world.ParseRaidTargetUpdate(packet.Body); err == nil {
			rt.SetTargetIcons(update)
		}
	case world.RaidReadyCheck:
		if guid, err := world.NewReader(packet.Body, "MSG_RAID_READY_CHECK").U64(); err == nil {
			rt.BeginReadyCheck(guid)
		}
	case world.RaidReadyCheckConfirm:
		reader := world.NewReader(packet.Body, "MSG_RAID_READY_CHECK_CONFIRM")
		guid, err := reader.U64()
		if err != nil {
			return true
		}
		state, _ := reader.U8()
		rt.AnswerReadyCheck(guid, state != 0)
	case world.RaidReadyCheckFinished:
		rt.FinishReadyCheck()
	case world.LootStartRoll:
		if start, err := world.ParseLootStartRoll(packet.Body); err == nil {
			rt.StartLootRoll(start)
		}
	case world.LootRollPacket:
		if vote, err := world.ParseLootRoll(packet.Body); err == nil {
			rt.AddLootRollVote(vote)
		}
	case world.LootRollWon:
		if won, err := world.ParseLootRollWon(packet.Body); err == nil {
			rt.ReportLootRollWon(won)
		}
	case world.LootAllPassed:
		if passed, err := world.ParseLootAllPassed(packet.Body); err == nil {
			rt.ReportLootAllPassed(passed)
		}
	case world.LootMasterList:
		rt.SetMasterLootCandidates(packet.Body)
	case world.RandomRoll:
		if roll, err := world.ParseRandomRoll(packet.Body); err == nil {
			rt.ReportRandomRoll(roll)
		}
	case world.RaidInstanceInfo:
		if saved, err := world.ParseRaidInstanceInfo(packet.Body); err == nil {
			rt.SetSavedInstances(saved)
		}
	case world.DungeonDifficulty:
		if value, err := world.NewReader(packet.Body, "MSG_SET_DUNGEON_DIFFICULTY").U32(); err == nil {
			rt.group.dungeon = value
			rt.FireEvent("PLAYER_DIFFICULTY_CHANGED")
		}
	case world.RaidDifficulty:
		if value, err := world.NewReader(packet.Body, "MSG_SET_RAID_DIFFICULTY").U32(); err == nil {
			rt.group.raid = value
			rt.FireEvent("PLAYER_DIFFICULTY_CHANGED")
		}
	case world.InstanceReset:
		if mapID, err := world.NewReader(packet.Body, "SMSG_INSTANCE_RESET").U32(); err == nil {
			rt.say("CHAT_MSG_SYSTEM", fmt.Sprintf(rt.globalString("INSTANCE_RESET_SUCCESS", "%s has been reset."), rt.mapName(mapID)))
		}
	case world.InstanceResetFailed:
		reader := world.NewReader(packet.Body, "SMSG_INSTANCE_RESET_FAILED")
		reason, _ := reader.U32()
		mapID, _ := reader.U32()
		names := []string{"INSTANCE_RESET_FAILED", "INSTANCE_RESET_FAILED_OFFLINE", "INSTANCE_RESET_FAILED_ZONING"}
		if reason > 2 {
			reason = 2
		}
		rt.say("CHAT_MSG_SYSTEM", fmt.Sprintf(rt.globalString(names[reason], "%s cannot be reset now."), rt.mapName(mapID)))
	default:
		return false
	}
	return true
}
