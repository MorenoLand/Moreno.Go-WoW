package ui

import (
	"encoding/binary"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// questConfirm is the quest another player shared that waits for the player to
// say yes.
type questConfirm struct {
	quest uint32
}

// handleQuestConfirmPacket takes a shared quest's offer: it asks the player
// when the sharer is known.
func (rt *Runtime) handleQuestConfirmPacket(packet world.Packet) bool {
	if packet.Opcode != world.QuestConfirmAcceptPacket {
		return false
	}
	reader := world.NewReader(packet.Body, "quest confirm accept")
	quest, err := reader.U32()
	if err != nil {
		return true
	}
	title, err := reader.CString()
	if err != nil {
		return true
	}
	guid, err := reader.U64()
	if err != nil {
		return true
	}
	rt.questConfirm.quest = quest
	name := rt.memberName(guid)
	if name == "" && rt.playerNames != nil {
		name = rt.playerNames(guid)
	}
	if name != "" {
		rt.FireEvent("QUEST_ACCEPT_CONFIRM", lua.LString(name), lua.LString(title))
	}
	return true
}

func registerQuestConfirmAPI(rt *Runtime) {
	rt.L.SetGlobal("ConfirmAcceptQuest", rt.L.NewFunction(func(L *lua.LState) int {
		rt.send(world.ClientQuestConfirmAccept, binary.LittleEndian.AppendUint32(nil, rt.questConfirm.quest))
		return 0
	}))
}
