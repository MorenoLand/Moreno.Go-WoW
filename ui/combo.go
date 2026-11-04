package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

const comboPointsPacket = 0x039D

// comboState is the combo points the character has and the unit they are on.
type comboState struct {
	target uint64
	points int
}

// handleComboPacket takes SMSG_UPDATE_COMBO_POINTS.
func (rt *Runtime) handleComboPacket(packet world.Packet) bool {
	if packet.Opcode != comboPointsPacket {
		return false
	}
	reader := world.NewReader(packet.Body, "SMSG_UPDATE_COMBO_POINTS")
	target, err := world.ReadPackedGUID(reader)
	if err != nil {
		return true
	}
	points, err := reader.U8()
	if err != nil {
		return true
	}
	rt.combo = comboState{target: target, points: int(points)}
	rt.FireEvent("UNIT_COMBO_POINTS", lua.LString("player"))
	return true
}

func registerComboAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetComboPoints", L.NewFunction(func(L *lua.LState) int {
		unit := L.OptString(1, "player")
		target := L.OptString(2, "target")
		points := 0
		if info := rt.unitByNameOrToken(unit); info != nil && info.GUID == rt.playerGUID() || unit == "vehicle" {
			if on := rt.unitByNameOrToken(target); on != nil && on.GUID == rt.combo.target && rt.combo.target != 0 {
				points = rt.combo.points
			}
		}
		L.Push(lua.LNumber(points))
		return 1
	}))
}
