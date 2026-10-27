package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// tradeState is the trade window: both sides of it and who has accepted.
type tradeState struct {
	open     bool
	partner  uint64
	player   world.TradeView
	target   world.TradeView
	accepted bool // the character accepted
	targetOK bool // the other player accepted
}

// tradeMessages are the globals that word the statuses that end or refuse a
// trade.
var tradeMessages = map[uint32]string{
	world.TradeBusy:          "ERR_TRADE_BUSY",
	world.TradeBusy2:         "ERR_TRADE_BUSY",
	world.TradeCanceled:      "ERR_TRADE_CANCELLED",
	world.TradeComplete:      "ERR_TRADE_COMPLETE",
	world.TradeTargetTooFar:  "ERR_TRADE_TARGET_TOO_FAR",
	world.TradeWrongFaction:  "ERR_TRADE_WRONG_FACTION",
	world.TradeIgnoreYou:     "ERR_TRADE_TARGET_IGNORED",
	world.TradeYouStunned:    "ERR_TRADE_YOU_STUNNED",
	world.TradeTargetStunned: "ERR_TRADE_TARGET_STUNNED",
	world.TradeYouDead:       "ERR_TRADE_YOU_DEAD",
	world.TradeTargetDead:    "ERR_TRADE_TARGET_DEAD",
	world.TradeYouLogout:     "ERR_TRADE_YOU_LOGOUT",
	world.TradeTargetLogout:  "ERR_TRADE_TARGET_LOGOUT",
	world.TradeNoTarget:      "ERR_TRADE_NO_TARGET",
	world.TradeRejected:      "ERR_TRADE_REJECTED",
}

// handleTradePacket takes the packets of the trade window.
func (rt *Runtime) handleTradePacket(packet world.Packet) bool {
	switch packet.Opcode {
	case world.TradeStatusPacket:
		if status, err := world.ParseTradeStatus(packet.Body); err == nil {
			rt.applyTradeStatus(status)
		}
	case world.TradeStatusExtendedPacket:
		if view, err := world.ParseTradeView(packet.Body); err == nil {
			rt.applyTradeView(view)
		}
	default:
		return false
	}
	return true
}

func (rt *Runtime) applyTradeStatus(status world.TradeStatus) {
	state := &rt.trade
	switch status.Status {
	case world.TradeBegin:
		state.partner = status.Trader
		rt.FireEvent("TRADE_REQUEST", lua.LString(rt.nameOf(status.Trader)))
	case world.TradeOpenWindow:
		*state = tradeState{open: true, partner: state.partner}
		rt.FireEvent("TRADE_SHOW")
	case world.TradeAccept:
		state.targetOK = true
		rt.FireEvent("TRADE_ACCEPT_UPDATE", lua.LNumber(boolNumber(state.accepted)), lua.LNumber(1))
	case world.TradeBackToTrade:
		state.accepted, state.targetOK = false, false
		rt.FireEvent("TRADE_ACCEPT_UPDATE", lua.LNumber(0), lua.LNumber(0))
	case world.TradeCanceled, world.TradeCloseWindow, world.TradeComplete, world.TradeRejected:
		wasOpen := state.open
		if name, ok := tradeMessages[status.Status]; ok && status.Status != world.TradeCloseWindow {
			rt.say("CHAT_MSG_SYSTEM", rt.globalString(name, ""))
		}
		*state = tradeState{}
		if wasOpen {
			rt.FireEvent("TRADE_CLOSED")
		} else {
			rt.FireEvent("TRADE_REQUEST_CANCEL")
		}
	default:
		if name, ok := tradeMessages[status.Status]; ok {
			rt.ShowError(rt.globalString(name, ""))
		}
		if status.Status == world.TradeBusy || status.Status == world.TradeBusy2 || status.Status == world.TradeNoTarget {
			rt.FireEvent("TRADE_REQUEST_CANCEL")
		}
	}
}

func boolNumber(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (rt *Runtime) applyTradeView(view world.TradeView) {
	state := &rt.trade
	if !state.open {
		state.open = true
		rt.FireEvent("TRADE_SHOW")
	}
	previous := &state.player
	if view.Trader {
		previous = &state.target
	}
	old := *previous
	*previous = view
	state.accepted, state.targetOK = false, false
	for slot := range view.Items {
		if old.Items[slot] != view.Items[slot] {
			event := "TRADE_PLAYER_ITEM_CHANGED"
			if view.Trader {
				event = "TRADE_TARGET_ITEM_CHANGED"
			}
			rt.FireEvent(event, lua.LNumber(slot+1))
		}
	}
	if old.Money != view.Money {
		rt.FireEvent("TRADE_MONEY_CHANGED")
	}
	rt.FireEvent("TRADE_UPDATE")
	rt.FireEvent("TRADE_ACCEPT_UPDATE", lua.LNumber(0), lua.LNumber(0))
}

// tradeSideItem pushes the values GetTradePlayerItemInfo and
// GetTradeTargetItemInfo return for a square.
func (rt *Runtime) tradeSideItem(L *lua.LState, view *world.TradeView, index int) int {
	if index < 1 || index > world.TradeSlotCount || !view.Items[index-1].Present {
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		L.Push(lua.LNil)
		return 5
	}
	item := view.Items[index-1]
	template := rt.itemTemplate(item.Entry)
	name, quality, usable := "", 1, lua.LValue(lua.LTrue)
	if template != nil {
		name, quality = template.Name, int(template.Quality)
	}
	L.Push(lua.LString(name))
	L.Push(lua.LString(rt.itemIcon(item.Entry)))
	L.Push(lua.LNumber(item.Count))
	L.Push(lua.LNumber(quality))
	L.Push(usable)
	return 5
}

// tradeSideLink is the item link of a square.
func (rt *Runtime) tradeSideLink(view *world.TradeView, index int) lua.LValue {
	if index < 1 || index > world.TradeSlotCount || !view.Items[index-1].Present {
		return lua.LNil
	}
	item := view.Items[index-1]
	if link := rt.itemLinkFor(item.Entry, item.Enchant); link != "" {
		return lua.LString(link)
	}
	return lua.LNil
}

// registerTradeAPI defines the calls the trade window makes.
func registerTradeAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	state := &rt.trade
	reg("InitiateTrade", func(L *lua.LState) int {
		unit := rt.unitInfo(L.CheckString(1))
		if unit == nil || !unit.Exists {
			return 0
		}
		if unit.GUID == rt.playerGUID() {
			rt.ShowError(rt.globalString("ERR_TRADE_SELF", "You cannot trade with yourself."))
			return 0
		}
		rt.send(world.ClientInitiateTrade, world.BuildInitiateTrade(unit.GUID))
		return 0
	})
	reg("BeginTrade", func(L *lua.LState) int {
		rt.send(world.ClientBeginTrade, nil)
		return 0
	})
	reg("CancelTrade", func(L *lua.LState) int {
		rt.send(world.ClientCancelTrade, nil)
		return 0
	})
	reg("CloseTrade", func(L *lua.LState) int {
		if state.open {
			rt.send(world.ClientCancelTrade, nil)
		}
		return 0
	})
	reg("AcceptTrade", func(L *lua.LState) int {
		if state.open && !state.accepted {
			state.accepted = true
			rt.send(world.ClientAcceptTrade, nil)
			rt.FireEvent("TRADE_ACCEPT_UPDATE", lua.LNumber(1), lua.LNumber(boolNumber(state.targetOK)))
		}
		return 0
	})
	reg("CancelTradeAccept", func(L *lua.LState) int {
		if state.open && state.accepted {
			state.accepted = false
			rt.send(world.ClientUnacceptTrade, nil)
			rt.FireEvent("TRADE_ACCEPT_UPDATE", lua.LNumber(0), lua.LNumber(boolNumber(state.targetOK)))
		}
		return 0
	})
	reg("GetPlayerTradeMoney", func(L *lua.LState) int {
		L.Push(lua.LNumber(state.player.Money))
		return 1
	})
	reg("GetTargetTradeMoney", func(L *lua.LState) int {
		L.Push(lua.LNumber(state.target.Money))
		return 1
	})
	reg("SetTradeMoney", func(L *lua.LState) int {
		rt.send(world.ClientSetTradeGold, world.BuildSetTradeGold(uint32(max(L.CheckNumber(1), 0))))
		return 0
	})
	reg("PickupTradeMoney", func(L *lua.LState) int {
		amount := uint32(max(L.CheckNumber(1), 0))
		if amount > 0 && amount <= state.player.Money {
			rt.send(world.ClientSetTradeGold, world.BuildSetTradeGold(state.player.Money-amount))
		}
		return 0
	})
	reg("GetTradePlayerItemInfo", func(L *lua.LState) int {
		return rt.tradeSideItem(L, &state.player, L.CheckInt(1))
	})
	reg("GetTradeTargetItemInfo", func(L *lua.LState) int {
		return rt.tradeSideItem(L, &state.target, L.CheckInt(1))
	})
	reg("GetTradePlayerItemLink", func(L *lua.LState) int {
		L.Push(rt.tradeSideLink(&state.player, L.CheckInt(1)))
		return 1
	})
	reg("GetTradeTargetItemLink", func(L *lua.LState) int {
		L.Push(rt.tradeSideLink(&state.target, L.CheckInt(1)))
		return 1
	})
	reg("ClickTradeButton", func(L *lua.LState) int {
		index := L.CheckInt(1)
		if !state.open || index < 1 || index > world.TradeSlotCount {
			return 0
		}
		cursor := &rt.inventory.cursor
		if cursor.active {
			bag, slot, ok := rt.serverBagAndSlotOfCursor()
			if !ok {
				return 0
			}
			rt.send(world.ClientSetTradeItem, world.BuildSetTradeItem(uint8(index-1), bag, slot))
			if rt.clearItemCursor != nil {
				rt.clearItemCursor()
			}
			return 0
		}
		if state.player.Items[index-1].Present {
			rt.send(world.ClientClearTradeItem, world.BuildClearTradeItem(uint8(index-1)))
		}
		return 0
	})
	reg("ClickTargetTradeButton", func(L *lua.LState) int { return 0 })
	reg("AddTradeMoney", func(L *lua.LState) int { return 0 })
}
