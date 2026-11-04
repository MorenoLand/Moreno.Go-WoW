package ui

import (
	"encoding/binary"

	lua "github.com/yuin/gopher-lua"
)

const cancelTempEnchantOpcode = 0x0379


func registerTempEnchantAPI(rt *Runtime) {
	rt.L.SetGlobal("CancelItemTempEnchantment", rt.L.NewFunction(func(L *lua.LState) int {
		slot, ok := L.Get(1).(lua.LNumber)
		if !ok || (slot != 1 && slot != 2) {
			L.RaiseError("Usage: CancelItemTempEnchantment(slot)")
			return 0
		}
		equipped := equipSlotMainHand + int(slot) - 1
		if equipped < len(rt.inventory.slots) && rt.inventory.slots[equipped] != 0 {
			rt.send(cancelTempEnchantOpcode, binary.LittleEndian.AppendUint32(nil, uint32(equipped)))
		}
		return 0
	}))
}
