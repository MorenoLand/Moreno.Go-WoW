package ui

import lua "github.com/yuin/gopher-lua"

// bagFamilyKeys marks the items that go on the key ring.
const bagFamilyKeys = 0x100

// hasKey reports whether the character carries a key.
func (rt *Runtime) hasKey() bool {
	state := &rt.inventory
	holds := func(guid uint64) bool {
		item := state.items[guid]
		if guid == 0 || item == nil {
			return false
		}
		template := state.templates[item.Entry]
		return template != nil && !template.NotFound && template.BagFamily&bagFamilyKeys != 0
	}
	for slot := 23; slot < 39 && slot < len(state.slots); slot++ {
		if holds(state.slots[slot]) {
			return true
		}
	}
	for slot := 19; slot < 23 && slot < len(state.slots); slot++ {
		if bag := state.items[state.slots[slot]]; bag != nil {
			for _, guid := range bag.Slots {
				if holds(guid) {
					return true
				}
			}
		}
	}
	return false
}

func registerHasKeyAPI(rt *Runtime) {
	rt.L.SetGlobal("HasKey", rt.L.NewFunction(func(L *lua.LState) int {
		if rt.hasKey() {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	}))
}
