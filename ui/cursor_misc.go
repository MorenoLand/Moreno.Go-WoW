package ui

import (
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// cursorNames are the names SetCursor takes, in the order Wow.exe numbers the
// cursors from 1; the "error" variants follow the plain ones.
var cursorNames = [...]string{"POINT_CURSOR", "CAST_CURSOR", "BUY_CURSOR", "ATTACK_CURSOR", "INTERACT_CURSOR", "SPEAK_CURSOR",
	"INSPECT_CURSOR", "PICKUP_CURSOR", "TAXI_CURSOR", "TRAINER_CURSOR", "MINE_CURSOR", "SKIN_CURSOR", "GATHER_CURSOR",
	"LOCK_CURSOR", "MAIL_CURSOR", "LOOT_ALL_CURSOR", "REPAIR_CURSOR", "REPAIRNPC_CURSOR", "ITEM_CURSOR", "SKIN_HORDE_CURSOR",
	"SKIN_ALLIANCE_CURSOR", "INNKEEPER_CURSOR", "QUEST_CURSOR", "QUEST_REPEATABLE_CURSOR", "QUEST_TURNIN_CURSOR", "VEHICLE_CURSOR",
	"POINT_ERROR_CURSOR", "CAST_ERROR_CURSOR", "BUY_ERROR_CURSOR", "ATTACK_ERROR_CURSOR", "INTERACT_ERROR_CURSOR",
	"SPEAK_ERROR_CURSOR", "INSPECT_ERROR_CURSOR", "PICKUP_ERROR_CURSOR", "TAXI_ERROR_CURSOR", "TRAINER_ERROR_CURSOR",
	"MINE_ERROR_CURSOR", "SKIN_ERROR_CURSOR", "GATHER_ERROR_CURSOR", "LOCK_ERROR_CURSOR", "MAIL_ERROR_CURSOR",
	"LOOT_ALL_ERROR_CURSOR", "REPAIR_ERROR_CURSOR", "REPAIRNPC_ERROR_CURSOR", "ITEM_ERROR_CURSOR", "SKIN_HORDE_ERROR_CURSOR",
	"SKIN_ALLIANCE_ERROR_CURSOR", "INNKEEPER_ERROR_CURSOR", "QUEST_ERROR_CURSOR", "QUEST_REPEATABLE_ERROR_CURSOR",
	"QUEST_TURNIN_ERROR_CURSOR", "VEHICLE_ERROR_CURSOR"}

// cursorPlainCount is how many cursors come before the error variants.
const cursorPlainCount = 26

// CursorOverride is the cursor a script asked for (the index Wow.exe gives it,
// from 1) or zero for the cursor the world picks.
func (rt *Runtime) CursorOverride() int { return rt.cursorOverride }

// moneyCursor is money held on the cursor.
type moneyCursor struct {
	amount uint32
}

func registerCursorMiscAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("SetCursor", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			rt.cursorOverride = 0
			return 0
		}
		name := L.CheckString(1)
		for index, candidate := range cursorNames {
			if strings.EqualFold(candidate, name) {
				rt.cursorOverride = index + 1
				L.Push(lua.LNumber(1))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("ResetCursor", func(L *lua.LState) int {
		rt.cursorOverride = 0
		return 0
	})
	reg("SendSystemMessage", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError(`Usage: SendSystemMessage("message")`)
		}
		rt.say("CHAT_MSG_SYSTEM", L.CheckString(1))
		return 0
	})
	reg("NotWhileDeadError", func(L *lua.LState) int {
		rt.ShowGameError(0x87)
		return 0
	})
	reg("GetCursorMoney", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.cursorMoney.amount))
		return 1
	})
	reg("PickupPlayerMoney", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: PickupPlayerMoney(amount)")
		}
		amount := uint32(max(L.CheckNumber(1), 0))
		if amount != 0 && amount <= rt.inventory.money {
			rt.pickupMoney(amount)
		}
		return 0
	})
	reg("PickupGuildBankMoney", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: PickupGuildBankMoney(amount)")
		}
		amount := uint32(max(L.CheckNumber(1), 0))
		if amount != 0 && (rt.guildBank.moneyLeft < 0 || amount <= uint32(rt.guildBank.moneyLeft)) {
			rt.pickupMoney(amount)
		}
		return 0
	})
	reg("DropCursorMoney", func(L *lua.LState) int {
		if rt.cursorMoney.amount != 0 {
			rt.cursorMoney = moneyCursor{}
			rt.FireEvent("CURSOR_UPDATE")
		}
		return 0
	})
}

func (rt *Runtime) pickupMoney(amount uint32) {
	rt.cursorMoney = moneyCursor{amount: amount}
	rt.FireEvent("CURSOR_UPDATE")
}
