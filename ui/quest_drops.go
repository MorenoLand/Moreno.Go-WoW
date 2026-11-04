package ui

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// questByNumber is the quest at a place in the log, or the selected one when
// the call names none.
func (rt *Runtime) questByNumber(number int, named bool) (QuestLogSlot, bool) {
	rows := rt.questRows()
	if !named {
		number = rt.quests.selected
	}
	if number < 1 || number > len(rows) || rows[number-1].slot < 0 {
		return QuestLogSlot{}, false
	}
	return rt.quests.slots[rows[number-1].slot], true
}

func registerQuestDropsAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetNumQuestItemDrops", func(L *lua.LState) int {
		count := 0
		if slot, ok := rt.questByNumber(L.OptInt(1, 0), L.Get(1).Type() == lua.LTNumber); ok {
			info := rt.quests.info[slot.ID]
			for index, entry := range info.ItemDrop {
				if entry == 0 {
					continue
				}
				if (entry != info.StartItem || info.ItemDropCount[index] > 1) && rt.itemCountOf(entry) == 0 {
					count++
				}
			}
		}
		L.Push(lua.LNumber(count))
		return 1
	})
	reg("GetQuestLogItemDrop", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetQuestLogItemDrop(index)")
			return 0
		}
		want := L.CheckInt(1) - 1
		slot, ok := rt.questByNumber(L.OptInt(2, 0), true)
		if ok && want >= 0 {
			info := rt.quests.info[slot.ID]
			seen := 0
			for index, entry := range info.ItemDrop {
				if entry == 0 {
					continue
				}
				if seen != want {
					seen++
					continue
				}
				name := ""
				if template := rt.itemTemplateIfKnown(entry); template != nil {
					name = template.Name
				}
				need := int(info.ItemDropCount[index])
				have := min(rt.itemCountOf(entry), need)
				L.Push(lua.LString(fmt.Sprintf(rt.globalString("QUEST_ITEMS_NEEDED", "%s: %d/%d"), name, have, need)))
				L.Push(lua.LString("item"))
				L.Push(lua.LBool(have >= need))
				return 3
			}
		}
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 3
	})
	reg("GetQuestPOILeaderBoard", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetQuestLogLeaderBoard(index)")
			return 0
		}
		slot, ok := rt.questByNumber(L.CheckInt(2), true)
		if ok {
			objectives := rt.questObjectives(slot)
			if index := L.CheckInt(1); index >= 1 && index <= len(objectives) {
				L.Push(lua.LString(objectives[index-1].text))
				L.Push(lua.LString(objectives[index-1].kind))
				L.Push(lua.LBool(objectives[index-1].finished))
				return 3
			}
		}
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 3
	})
}
