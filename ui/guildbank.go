package ui

import (
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// GuildBankHost is what the guild bank asks of the connection.
type GuildBankHost interface {
	// GuildBankSend sends a request about the bank.
	GuildBankSend(opcode world.ClientOpcode, body []byte) error
	// GuildMemberName names a guild member by guid; empty until it is known.
	GuildMemberName(guid uint64) string
	// GuildBankTabCost is the price of the next tab, given how many are bought.
	GuildBankTabCost(purchased int) uint32
}

const (
	guildBankMaxTabs = 6
	// cursorGuildBank marks an item picked up from the guild bank.
	cursorGuildBank = -100
	// guildBankTabRight bits of a tab's rights.
	guildBankViewTab    = 0x01
	guildBankPutItem    = 0x02
	guildBankUpdateText = 0x04
	// guildRank rights.
	guildRightWithdrawRepair = 0x00040000
	guildRightWithdrawGold   = 0x00080000
	guildWithdrawUnlimited   = -1
)

type guildBankTabRights struct {
	flags uint32
	limit int32
}

type guildBankState struct {
	open    bool
	bank    uint64
	money   uint64
	tab     int
	numTabs int
	tabs    [guildBankMaxTabs]world.GuildBankTab
	items   [guildBankMaxTabs][world.GuildBankSlots]world.GuildBankItem
	// remaining is how many stacks the member may still take from each tab.
	remaining [guildBankMaxTabs]int32
	rights    uint32
	rank      uint32
	rankKnown bool
	goldLimit int32
	tabRights [guildBankMaxTabs]guildBankTabRights
	// moneyLeft is what the member may still withdraw today.
	moneyLeft int32
	logs      [guildBankMaxTabs + 1]world.GuildBankLog
	texts     [guildBankMaxTabs]string
	// cursorCount is the amount split off a stack picked up from the bank.
	cursorCount int
}

func (rt *Runtime) guildBankHost() GuildBankHost {
	host, _ := rt.Host.(GuildBankHost)
	return host
}

// OpenGuildBank shows the guild bank the player clicked.
func (rt *Runtime) OpenGuildBank(bank uint64) {
	state := &rt.guildBank
	*state = guildBankState{open: true, bank: bank, tab: 1, rights: state.rights, rank: state.rank, rankKnown: state.rankKnown,
		goldLimit: state.goldLimit, tabRights: state.tabRights, moneyLeft: state.moneyLeft}
	if host := rt.guildBankHost(); host != nil {
		_ = host.GuildBankSend(world.ClientGuildBankActivate, world.BuildGuildBankActivate(bank, true))
		_ = host.GuildBankSend(world.ClientGuildBankPermissions, nil)
		_ = host.GuildBankSend(world.ClientGuildBankMoneyLeft, nil)
	}
	rt.FireEvent("GUILDBANKFRAME_OPENED")
}

// CloseGuildBank closes the guild bank window.
func (rt *Runtime) CloseGuildBank() {
	if !rt.guildBank.open {
		return
	}
	rt.guildBank.open = false
	rt.FireEvent("GUILDBANKFRAME_CLOSED")
}

// GuildBankOpen reports whether a guild bank is showing.
func (rt *Runtime) GuildBankOpen() bool { return rt.guildBank.open }

// SetGuildBankList applies the contents of a tab the server sent.
func (rt *Runtime) SetGuildBankList(list world.GuildBankContents) {
	state := &rt.guildBank
	state.money = list.Money
	if list.Full && list.Tab == 0 {
		state.numTabs = len(list.Tabs)
		for index := range state.tabs {
			state.tabs[index] = world.GuildBankTab{}
			if index < len(list.Tabs) {
				state.tabs[index] = list.Tabs[index]
			}
		}
	}
	if int(list.Tab) < guildBankMaxTabs {
		state.remaining[list.Tab] = list.Withdrawals
		if list.Full {
			state.items[list.Tab] = [world.GuildBankSlots]world.GuildBankItem{}
		}
		for _, item := range list.Items {
			if int(item.Slot) < world.GuildBankSlots {
				state.items[list.Tab][item.Slot] = item
				rt.itemTemplate(item.Entry)
			}
		}
	}
	if list.Full && list.Tab == 0 {
		rt.FireEvent("GUILDBANK_UPDATE_TABS")
	}
	rt.FireEvent("GUILDBANKBAGSLOTS_CHANGED")
	rt.FireEvent("GUILDBANK_UPDATE_MONEY")
}

// SetGuildPermissions records the member's rank and what it allows.
func (rt *Runtime) SetGuildPermissions(info world.GuildPermissions) {
	state := &rt.guildBank
	state.rank, state.rights, state.rankKnown = info.Rank, info.Rights, true
	state.goldLimit = info.GoldLimit
	if state.numTabs == 0 {
		state.numTabs = int(info.NumTabs)
	}
	for index := range state.tabRights {
		state.tabRights[index] = guildBankTabRights{flags: info.Tabs[index].Flags, limit: info.Tabs[index].Limit}
	}
	rt.FireEvent("GUILDBANK_UPDATE_TABS")
}

// SetGuildBankMoneyLeft records what the member may still withdraw today.
func (rt *Runtime) SetGuildBankMoneyLeft(copper int32) {
	rt.guildBank.moneyLeft = copper
	rt.FireEvent("GUILDBANK_UPDATE_WITHDRAWMONEY")
}

// SetGuildBankLog records the log of a tab (the sixth is the money log).
func (rt *Runtime) SetGuildBankLog(log world.GuildBankLog) {
	if int(log.Tab) <= guildBankMaxTabs {
		rt.guildBank.logs[log.Tab] = log
		for _, line := range log.Lines {
			rt.itemTemplate(line.Entry)
		}
	}
	rt.FireEvent("GUILDBANKLOG_UPDATE")
}

// SetGuildBankText records the text of a tab.
func (rt *Runtime) SetGuildBankText(tab uint8, text string) {
	if int(tab) < guildBankMaxTabs {
		rt.guildBank.texts[tab] = text
		rt.FireEvent("GUILDBANK_UPDATE_TEXT", numberValue(int(tab)+1))
	}
}

// RefreshGuildBank redraws the bank after item data arrived.
func (rt *Runtime) RefreshGuildBank() {
	if rt.guildBank.open {
		rt.FireEvent("GUILDBANKBAGSLOTS_CHANGED")
	}
}

// guildBankItem is the square of a 1-based tab and slot.
func (rt *Runtime) guildBankItem(tab, slot int) *world.GuildBankItem {
	if tab < 1 || tab > guildBankMaxTabs || slot < 1 || slot > world.GuildBankSlots {
		return nil
	}
	if item := &rt.guildBank.items[tab-1][slot-1]; item.Entry != 0 {
		return item
	}
	return nil
}

// guildBankPickup picks up, drops onto or swaps with a bank square.
func (rt *Runtime) guildBankPickup(L *lua.LState, tab, slot int, autoStore bool) {
	state := &rt.guildBank
	host := rt.guildBankHost()
	if host == nil || !state.open || tab < 1 || tab > guildBankMaxTabs || slot < 1 || slot > world.GuildBankSlots {
		return
	}
	cursor := &rt.inventory.cursor
	switch {
	case autoStore:
		if item := rt.guildBankItem(tab, slot); item != nil {
			_ = host.GuildBankSend(world.ClientGuildBankSwapItems, world.BuildGuildBankAutoStore(state.bank, uint8(tab-1), uint8(slot-1), 0))
		}
	case cursor.active && cursor.bag == cursorGuildBank:
		_ = host.GuildBankSend(world.ClientGuildBankSwapItems, world.BuildGuildBankMoveInside(state.bank, uint8(tab-1), uint8(slot-1), uint8(cursor.bankTab-1), uint8(cursor.slot-1), uint32(state.cursorCount)))
		rt.dropItemCursor()
	case cursor.active:
		bag, bagSlot, ok := rt.serverBagAndSlotOfCursor()
		if ok {
			count := uint32(max(rt.inventory.splitCount, 0))
			rt.inventory.splitCount = 0
			_ = host.GuildBankSend(world.ClientGuildBankSwapItems, world.BuildGuildBankToBag(state.bank, uint8(tab-1), uint8(slot-1), bag, bagSlot, true, count))
		}
		rt.dropItemCursor()
	default:
		item := rt.guildBankItem(tab, slot)
		if item == nil {
			return
		}
		*cursor = cursorItem{active: true, bag: cursorGuildBank, slot: slot, bankTab: tab}
		rt.gameplay.Cursor.ID, rt.gameplay.Cursor.Type = item.Entry, 128
		rt.FireEvent("GUILDBANK_ITEM_LOCK_CHANGED")
		rt.FireEvent("CURSOR_UPDATE")
	}
}

// dropItemCursor puts the item on the cursor down without moving it.
func (rt *Runtime) dropItemCursor() {
	if rt.clearItemCursor != nil {
		rt.clearItemCursor()
	}
}

// guildBankWithdraw takes the item on the cursor out of the bank into a bag
// square.
func (rt *Runtime) guildBankWithdraw(bag, slot int) {
	state := &rt.guildBank
	host := rt.guildBankHost()
	cursor := rt.inventory.cursor
	if host == nil || !state.open {
		return
	}
	dstBag, dstSlot, ok := rt.serverBagAndSlot(bag, slot)
	if !ok {
		return
	}
	count := uint32(max(state.cursorCount, 0))
	state.cursorCount = 0
	_ = host.GuildBankSend(world.ClientGuildBankSwapItems, world.BuildGuildBankToBag(state.bank, uint8(cursor.bankTab-1), uint8(cursor.slot-1), dstBag, dstSlot, false, count))
}

// guildBankAutoStoreCursor takes the item on the cursor out of the bank into
// the first free bag square.
func (rt *Runtime) guildBankAutoStoreCursor() {
	state := &rt.guildBank
	cursor := rt.inventory.cursor
	if host := rt.guildBankHost(); host != nil && state.open {
		_ = host.GuildBankSend(world.ClientGuildBankSwapItems, world.BuildGuildBankAutoStore(state.bank, uint8(cursor.bankTab-1), uint8(cursor.slot-1), uint32(max(state.cursorCount, 0))))
		state.cursorCount = 0
	}
}

// logAge splits the seconds since a log line into the years, months, days and
// hours the interface words it with.
func logAge(seconds uint32) (years, months, days, hours int) {
	const hour, day = 3600, 86400
	years = int(seconds / (365 * day))
	seconds %= 365 * day
	months = int(seconds / (30 * day))
	seconds %= 30 * day
	days = int(seconds / day)
	hours = int(seconds % day / hour)
	return
}

func (rt *Runtime) guildBankTabRight(tab int) uint32 {
	if tab < 1 || tab > guildBankMaxTabs {
		return 0
	}
	return rt.guildBank.tabRights[tab-1].flags
}

// registerGuildBankAPI defines the calls the guild bank window makes.
func registerGuildBankAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	state := &rt.guildBank
	send := func(opcode world.ClientOpcode, body []byte) {
		if host := rt.guildBankHost(); host != nil && state.open {
			_ = host.GuildBankSend(opcode, body)
		}
	}
	reg("GetNumGuildBankTabs", func(L *lua.LState) int { L.Push(lua.LNumber(state.numTabs)); return 1 })
	reg("GetCurrentGuildBankTab", func(L *lua.LState) int { L.Push(lua.LNumber(max(state.tab, 1))); return 1 })
	reg("SetCurrentGuildBankTab", func(L *lua.LState) int { state.tab = L.CheckInt(1); return 0 })
	reg("GetGuildBankMoney", func(L *lua.LState) int { L.Push(lua.LNumber(state.money)); return 1 })
	reg("GetGuildBankWithdrawMoney", func(L *lua.LState) int {
		// Unlimited when the rank is the guild master's.
		if state.rankKnown && state.goldLimit == guildWithdrawUnlimited {
			L.Push(lua.LNumber(-1))
		} else {
			L.Push(lua.LNumber(state.moneyLeft))
		}
		return 1
	})
	reg("CanWithdrawGuildBankMoney", func(L *lua.LState) int {
		L.Push(lua.LBool(state.rights&guildRightWithdrawGold != 0))
		return 1
	})
	reg("CanGuildBankRepair", func(L *lua.LState) int {
		L.Push(lua.LBool(state.rights&guildRightWithdrawRepair != 0))
		return 1
	})
	reg("CanEditGuildTabInfo", func(L *lua.LState) int {
		tab := L.OptInt(1, state.tab)
		L.Push(lua.LBool(state.rankKnown && state.rank == 0 || rt.guildBankTabRight(tab)&guildBankUpdateText != 0))
		return 1
	})
	reg("IsGuildLeader", func(L *lua.LState) int { L.Push(lua.LBool(state.rankKnown && state.rank == 0)); return 1 })
	reg("GetGuildBankTabInfo", func(L *lua.LState) int {
		tab := L.CheckInt(1)
		if tab < 1 || tab > guildBankMaxTabs {
			return 0
		}
		if tab > state.numTabs {
			// A tab not bought yet has nothing to tell.
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			return 6
		}
		info := state.tabs[tab-1]
		rights := state.tabRights[tab-1]
		name, icon := info.Name, info.Icon
		if name == "" {
			name = strings.Replace(rt.globalString("GUILDBANK_TAB_NUMBER", "Tab %d"), "%d", strconv.Itoa(tab), 1)
		}
		// The icon is kept as its bare name; the call names the whole path.
		if icon == "" {
			icon = "INV_Misc_QuestionMark"
		}
		if !strings.HasPrefix(strings.ToLower(icon), "interface") {
			icon = `Interface\Icons\` + icon
		}
		remaining := state.remaining[tab-1]
		L.Push(lua.LString(name))
		L.Push(lua.LString(icon))
		L.Push(lua.LBool(rights.flags&guildBankViewTab != 0))
		L.Push(lua.LBool(rights.flags&guildBankPutItem != 0))
		L.Push(lua.LNumber(rights.limit))
		L.Push(lua.LNumber(remaining))
		return 6
	})
	reg("GetGuildBankTabCost", func(L *lua.LState) int {
		cost := uint32(0)
		if host := rt.guildBankHost(); host != nil {
			cost = host.GuildBankTabCost(state.numTabs)
		}
		L.Push(lua.LNumber(cost))
		return 1
	})
	reg("BuyGuildBankTab", func(L *lua.LState) int {
		send(world.ClientGuildBankBuyTab, world.BuildGuildBankBuyTab(state.bank, uint8(state.numTabs)))
		return 0
	})
	reg("SetGuildBankTabInfo", func(L *lua.LState) int {
		tab, name := L.CheckInt(1), L.CheckString(2)
		icon := rt.macroIconPath(L.CheckInt(3))
		if tab >= 1 && tab <= guildBankMaxTabs && icon != "" {
			icon = icon[strings.LastIndex(icon, `\`)+1:]
			send(world.ClientGuildBankUpdateTab, world.BuildGuildBankUpdateTab(state.bank, uint8(tab-1), name, icon))
		}
		return 0
	})
	reg("GetGuildBankItemInfo", func(L *lua.LState) int {
		tab, slot := L.CheckInt(1), L.CheckInt(2)
		item := rt.guildBankItem(tab, slot)
		if item == nil {
			L.Push(lua.LNil)
			L.Push(lua.LNumber(0))
			L.Push(lua.LFalse)
			return 3
		}
		cursor := rt.inventory.cursor
		locked := cursor.active && cursor.bag == cursorGuildBank && cursor.bankTab == tab && cursor.slot == slot
		L.Push(lua.LString(rt.itemIcon(item.Entry)))
		L.Push(lua.LNumber(item.Count))
		L.Push(lua.LBool(locked))
		return 3
	})
	reg("GetGuildBankItemLink", func(L *lua.LState) int {
		if item := rt.guildBankItem(L.CheckInt(1), L.CheckInt(2)); item != nil {
			if link := rt.itemLinkFor(item.Entry, uint32(item.Enchant)); link != "" {
				L.Push(lua.LString(link))
				return 1
			}
		}
		return 0
	})
	reg("QueryGuildBankTab", func(L *lua.LState) int {
		if tab := L.CheckInt(1); tab >= 1 && tab <= guildBankMaxTabs {
			send(world.ClientGuildBankQueryTab, world.BuildGuildBankQueryTab(state.bank, uint8(tab-1), false))
		}
		return 0
	})
	reg("QueryGuildBankLog", func(L *lua.LState) int {
		if tab := L.CheckInt(1); tab >= 1 && tab <= guildBankMaxTabs+1 {
			send(world.ClientGuildBankLogQuery, world.BuildGuildBankTab(uint8(tab-1)))
		}
		return 0
	})
	reg("QueryGuildBankText", func(L *lua.LState) int {
		if tab := L.CheckInt(1); tab >= 1 && tab <= guildBankMaxTabs {
			send(world.ClientGuildBankTextQuery, world.BuildGuildBankTab(uint8(tab-1)))
		}
		return 0
	})
	reg("GetGuildBankText", func(L *lua.LState) int {
		if tab := L.CheckInt(1); tab >= 1 && tab <= guildBankMaxTabs {
			L.Push(lua.LString(state.texts[tab-1]))
			return 1
		}
		return 0
	})
	reg("SetGuildBankText", func(L *lua.LState) int {
		tab, text := L.CheckInt(1), L.CheckString(2)
		if tab >= 1 && tab <= guildBankMaxTabs {
			state.texts[tab-1] = text
			send(world.ClientGuildBankSetText, world.BuildGuildBankSetText(uint8(tab-1), text))
		}
		return 0
	})
	reg("PickupGuildBankItem", func(L *lua.LState) int {
		rt.guildBankPickup(L, L.CheckInt(1), L.CheckInt(2), false)
		return 0
	})
	reg("AutoStoreGuildBankItem", func(L *lua.LState) int {
		rt.guildBankPickup(L, L.CheckInt(1), L.CheckInt(2), true)
		return 0
	})
	reg("SplitGuildBankItem", func(L *lua.LState) int {
		tab, slot, count := L.CheckInt(1), L.CheckInt(2), L.CheckInt(3)
		if item := rt.guildBankItem(tab, slot); item != nil && count > 0 && int32(count) < item.Count && !rt.inventory.cursor.active {
			state.cursorCount = count
			rt.guildBankPickup(L, tab, slot, false)
		}
		return 0
	})
	reg("DepositGuildBankMoney", func(L *lua.LState) int {
		send(world.ClientGuildBankDeposit, world.BuildGuildBankMoney(state.bank, uint32(L.CheckNumber(1))))
		return 0
	})
	reg("WithdrawGuildBankMoney", func(L *lua.LState) int {
		send(world.ClientGuildBankWithdraw, world.BuildGuildBankMoney(state.bank, uint32(L.CheckNumber(1))))
		return 0
	})
	reg("CloseGuildBankFrame", func(L *lua.LState) int {
		rt.CloseGuildBank()
		return 0
	})
	reg("GetNumGuildBankTransactions", func(L *lua.LState) int {
		tab := L.CheckInt(1)
		count := 0
		if tab >= 1 && tab <= guildBankMaxTabs {
			count = len(state.logs[tab-1].Lines)
		}
		L.Push(lua.LNumber(count))
		return 1
	})
	reg("GetGuildBankTransaction", func(L *lua.LState) int {
		tab, index := L.CheckInt(1), L.CheckInt(2)
		if tab < 1 || tab > guildBankMaxTabs || index < 1 || index > len(state.logs[tab-1].Lines) {
			return 0
		}
		line := state.logs[tab-1].Lines[index-1]
		kind := "move"
		switch line.Kind {
		case world.GuildBankLogDepositItem:
			kind = "deposit"
		case world.GuildBankLogWithdrawItem:
			kind = "withdraw"
		}
		name := lua.LValue(lua.LNil)
		if host := rt.guildBankHost(); host != nil {
			if text := host.GuildMemberName(line.Player); text != "" {
				name = lua.LString(text)
			}
		}
		link := lua.LValue(lua.LNil)
		if text := rt.itemLinkFor(line.Entry, 0); text != "" {
			link = lua.LString(text)
		}
		years, months, days, hours := logAge(line.Ago)
		L.Push(lua.LString(kind))
		L.Push(name)
		L.Push(link)
		L.Push(lua.LNumber(line.Count))
		L.Push(lua.LNumber(tab))
		L.Push(lua.LNumber(int(line.OtherTab) + 1))
		L.Push(lua.LNumber(years))
		L.Push(lua.LNumber(months))
		L.Push(lua.LNumber(days))
		L.Push(lua.LNumber(hours))
		return 10
	})
	reg("GetNumGuildBankMoneyTransactions", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(state.logs[guildBankMaxTabs].Lines)))
		return 1
	})
	reg("GetGuildBankMoneyTransaction", func(L *lua.LState) int {
		log := state.logs[guildBankMaxTabs].Lines
		index := L.CheckInt(1)
		if index < 1 || index > len(log) {
			return 0
		}
		line := log[index-1]
		kind := "deposit"
		switch line.Kind {
		case world.GuildBankLogWithdrawMony:
			kind = "withdraw"
		case world.GuildBankLogRepairMoney:
			kind = "repair"
		case world.GuildBankLogBuySlot:
			kind = "buyTab"
		case world.GuildBankLogCashFlow:
			kind = "withdrawForTab"
		}
		name := lua.LValue(lua.LNil)
		if host := rt.guildBankHost(); host != nil {
			if text := host.GuildMemberName(line.Player); text != "" {
				name = lua.LString(text)
			}
		}
		years, months, days, hours := logAge(line.Ago)
		L.Push(lua.LString(kind))
		L.Push(name)
		L.Push(lua.LNumber(line.Money))
		L.Push(lua.LNumber(years))
		L.Push(lua.LNumber(months))
		L.Push(lua.LNumber(days))
		L.Push(lua.LNumber(hours))
		return 7
	})
}
