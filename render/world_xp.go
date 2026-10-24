package render

import (
	"fmt"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// handleProgressPacket reports experience and new levels; it says whether the
// packet was one. say puts a line in the chat as the given message type.
func handleProgressPacket(rt *ui.Runtime, names *worldNames, entities map[uint64]*worldEntity, packet world.Packet, say func(event, text string)) bool {
	switch packet.Opcode {
	case world.LogXPGain:
		gain, err := world.ParseLogXPGain(packet.Body)
		if err != nil {
			return true
		}
		text := fmt.Sprintf(rt.GlobalString("COMBATLOG_XPGAIN_FIRSTPERSON_UNNAMED", "You gain %d experience."), gain.Total)
		if gain.Kill {
			if victim := entities[gain.Victim]; victim != nil {
				if creature, ok := names.creatures[victim.fields[objectEntryField]]; ok && creature.Name != "" {
					text = fmt.Sprintf(rt.GlobalString("COMBATLOG_XPGAIN_FIRSTPERSON", "%s dies, you gain %d experience."), creature.Name, gain.Total)
				}
			}
		}
		say("CHAT_MSG_COMBAT_XP_GAIN", text)
	case world.LevelUpInfo:
		info, err := world.ParseLevelUpInfo(packet.Body)
		if err != nil {
			return true
		}
		power := uint32(0)
		if player := rt.PlayerPowerType(); player >= 0 && player < len(info.Powers) {
			power = info.Powers[player]
		}
		args := []lua.LValue{lua.LNumber(info.Level), lua.LNumber(info.Health), lua.LNumber(power), lua.LNumber(0)}
		for _, stat := range info.Stats {
			args = append(args, lua.LNumber(stat))
		}
		rt.FireEvent("PLAYER_LEVEL_UP", args...)
	default:
		return false
	}
	return true
}
