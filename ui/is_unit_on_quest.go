package ui

import lua "github.com/yuin/gopher-lua"

func registerUnitOnQuestAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("IsUnitOnQuest", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTString {
			L.RaiseError("Usage: IsUnitOnQuest(index, \"unit\")")
			return 0
		}
		rows := rt.questRows()
		index := L.CheckInt(1)
		if index >= 1 && index <= len(rows) && rows[index-1].slot >= 0 {
			id := rt.quests.slots[rows[index-1].slot].ID
			if info := rt.unitByNameOrToken(L.CheckString(2)); info != nil && info.Player && id != 0 {
				if info.GUID == rt.playerGUID() {
					L.Push(lua.LNumber(1))
					return 1
				}
				for _, quest := range info.QuestIDs {
					if quest == id {
						L.Push(lua.LNumber(1))
						return 1
					}
				}
			}
		}
		L.Push(lua.LNil)
		return 1
	}))
}
