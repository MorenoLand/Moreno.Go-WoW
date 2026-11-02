package ui

import (
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Kinds of raid instance message.
const (
	raidMessageHours = iota + 1
	raidMessageMinutes
	raidMessageSoon
	raidMessageWelcome
	raidMessageExpired
)

// handleRaidInstancePacket takes the warnings that a saved instance is about
// to reset and the welcome into one.
func (rt *Runtime) handleRaidInstancePacket(packet world.Packet) bool {
	if packet.Opcode != world.RaidInstanceMessage {
		return false
	}
	reader := world.NewReader(packet.Body, "raid instance message")
	var fields [4]uint32
	for index := range fields {
		value, err := reader.U32()
		if err != nil {
			return true
		}
		fields[index] = value
	}
	kind, name, left := fields[0], rt.bfMapName(fields[1]), fields[3]
	switch kind {
	case raidMessageHours:
		rt.say("CHAT_MSG_SYSTEM", tooltipFormat(rt.globalString("RAID_INSTANCE_WARNING_HOURS", ""), name, itoa(int(left/3600))))
	case raidMessageMinutes:
		rt.say("CHAT_MSG_SYSTEM", tooltipFormat(rt.globalString("RAID_INSTANCE_WARNING_MIN", ""), name, itoa(int(left/60))))
	case raidMessageSoon:
		rt.say("CHAT_MSG_SYSTEM", tooltipFormat(rt.globalString("RAID_INSTANCE_WARNING_MIN_SOON", ""), name, itoa(int(left/60))))
	case raidMessageWelcome:
		locked, _ := reader.U8()
		extended, _ := reader.U8()
		rt.FireEvent("RAID_INSTANCE_WELCOME", lua.LString(name), lua.LNumber(left), lua.LNumber(boolNumber(locked != 0)), lua.LNumber(boolNumber(extended != 0)))
	case raidMessageExpired:
		rt.say("CHAT_MSG_SYSTEM", tooltipFormat(rt.globalString("RAID_INSTANCE_EXPIRED", ""), name))
	}
	return true
}

// Game errors raised when a group-only instance refuses the character.
const raidGroupOnlyFirstError = 0x1A7

// handleRaidGroupOnlyPacket takes the countdown after which a character that
// is not in a group is put out of a raid instance.
func (rt *Runtime) handleRaidGroupOnlyPacket(packet world.Packet) bool {
	if packet.Opcode != world.RaidGroupOnly {
		return false
	}
	reader := world.NewReader(packet.Body, "raid group only")
	delay, err := reader.U32()
	if err != nil {
		return true
	}
	reason, err := reader.U32()
	if err != nil {
		return true
	}
	if int32(delay) == 0 && reason >= 1 && reason <= 4 {
		rt.ShowGameError(raidGroupOnlyFirstError + int(reason) - 1)
	}
	if int32(delay) > 0 {
		rt.bootDeadline = time.Now().Add(time.Duration(delay) * time.Millisecond)
		rt.FireEvent("INSTANCE_BOOT_START")
		return true
	}
	rt.bootDeadline = time.Time{}
	rt.FireEvent("INSTANCE_BOOT_STOP")
	return true
}

func registerInstanceBootAPI(rt *Runtime) {
	rt.L.SetGlobal("GetInstanceBootTimeRemaining", rt.L.NewFunction(func(L *lua.LState) int {
		left := time.Until(rt.bootDeadline)
		if rt.bootDeadline.IsZero() || left <= 0 {
			left = 0
		}
		L.Push(lua.LNumber(int64(left / time.Millisecond / 1000)))
		return 1
	}))
}
