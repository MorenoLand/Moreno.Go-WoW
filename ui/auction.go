package ui

import (
	"sort"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// AuctionHost is what the auction house asks of the connection.
type AuctionHost interface {
	// AuctionSend sends a request to the auctioneer.
	AuctionSend(opcode world.ClientOpcode, body []byte) error
	// AuctionPlayerName names a seller or bidder by guid; empty until known.
	AuctionPlayerName(guid uint64) string
	// AuctionDepositRate is the house's deposit rate, in percent.
	AuctionDepositRate(house uint32) float64
}

const (
	auctionListBrowse = iota
	auctionListBidder
	auctionListOwner
	auctionLists
)

// auctionMinimumDeposit is the least deposit, in copper.
const auctionMinimumDeposit = 100

type auctionSortSpec struct {
	column  string
	reverse bool
}

type auctionList struct {
	entries []world.AuctionEntry
	total   int
	sort    []auctionSortSpec
	// selected is the 1-based row the player picked.
	selected int
}

type auctionState struct {
	open       bool
	auctioneer uint64
	house      uint32
	lists      [auctionLists]auctionList
	// nextQuery is when the server allows the next search.
	nextQuery time.Time
	delayMS   uint32
	// sell is the item put in the sell slot.
	sellGUID uint64
	// invTypes caches the inventory types of a class and subclass.
	invTypes map[[2]int][]uint32
}

func (rt *Runtime) auctionHost() AuctionHost {
	host, _ := rt.Host.(AuctionHost)
	return host
}

func auctionListIndex(name string) int {
	switch strings.ToLower(name) {
	case "bidder":
		return auctionListBidder
	case "owner":
		return auctionListOwner
	}
	return auctionListBrowse
}

// OpenAuctionHouse shows the auction house of the auctioneer who said hello.
func (rt *Runtime) OpenAuctionHouse(info world.AuctionHelloInfo) {
	if !info.Enabled {
		rt.FireEvent("AUCTION_HOUSE_DISABLED")
		return
	}
	state := &rt.auction
	state.open, state.auctioneer, state.house = true, info.GUID, info.House
	state.lists = [auctionLists]auctionList{}
	state.sellGUID = 0
	rt.CloseGossipWindow()
	rt.FireEvent("AUCTION_HOUSE_SHOW")
}

// CloseAuctionHouse closes the auction house window.
func (rt *Runtime) CloseAuctionHouse() {
	if !rt.auction.open {
		return
	}
	rt.auction.open = false
	rt.FireEvent("AUCTION_HOUSE_CLOSED")
}

// SetAuctionList installs a list the server sent.
func (rt *Runtime) SetAuctionList(kind int, list world.AuctionList) {
	state := &rt.auction
	state.lists[kind].entries = list.Auctions
	state.lists[kind].total = int(list.Total)
	state.lists[kind].selected = 0
	if kind == auctionListBrowse {
		state.delayMS = list.DelayMS
		state.nextQuery = time.Now().Add(time.Duration(list.DelayMS) * time.Millisecond)
	}
	for _, entry := range list.Auctions {
		rt.itemTemplate(entry.Entry)
	}
	rt.sortAuctionList(kind)
	switch kind {
	case auctionListBrowse:
		rt.FireEvent("AUCTION_ITEM_LIST_UPDATE")
	case auctionListBidder:
		rt.FireEvent("AUCTION_BIDDER_LIST_UPDATE")
	default:
		rt.FireEvent("AUCTION_OWNED_LIST_UPDATE")
	}
}

// SetAuctionListKind turns a packet opcode into the list it carries.
func AuctionListKind(opcode uint16) (int, bool) {
	switch opcode {
	case world.AuctionListResult:
		return auctionListBrowse, true
	case world.AuctionBidderResult:
		return auctionListBidder, true
	case world.AuctionOwnerResult:
		return auctionListOwner, true
	}
	return 0, false
}

// ApplyAuctionResult applies the server's answer to a command.
func (rt *Runtime) ApplyAuctionResult(result world.AuctionResult) {
	if result.Error != world.AuctionOK {
		rt.ShowGlobalError(auctionErrorName(result.Error))
		if result.Command == world.AuctionCommandSell {
			rt.FireEvent("AUCTION_MULTISELL_FAILURE")
		}
		return
	}
	host := rt.auctionHost()
	switch result.Command {
	case world.AuctionCommandSell:
		rt.ShowGlobalError("ERR_AUCTION_STARTED")
		if host != nil {
			_ = host.AuctionSend(world.ClientAuctionListOwner, world.BuildAuctionListOwner(rt.auction.auctioneer))
		}
	case world.AuctionCommandCancel:
		rt.ShowGlobalError("ERR_AUCTION_REMOVED")
		if host != nil {
			_ = host.AuctionSend(world.ClientAuctionListOwner, world.BuildAuctionListOwner(rt.auction.auctioneer))
		}
	case world.AuctionCommandBid:
		rt.ShowGlobalError("ERR_AUCTION_BID_PLACED")
		if host != nil {
			_ = host.AuctionSend(world.ClientAuctionListBidder, world.BuildAuctionListBidder(rt.auction.auctioneer))
		}
	}
}

func auctionErrorName(code uint32) string {
	switch code {
	case world.AuctionErrNoMoney:
		return "ERR_NOT_ENOUGH_MONEY"
	case world.AuctionErrNoItem:
		return "ERR_ITEM_NOT_FOUND"
	case world.AuctionErrHigherBid:
		return "ERR_AUCTION_HIGHER_BID"
	case world.AuctionErrIncrement:
		return "ERR_AUCTION_BID_INCREMENT"
	case world.AuctionErrOwnBid:
		return "ERR_AUCTION_BID_OWN"
	}
	return "ERR_AUCTION_DATABASE_ERROR"
}

// auctionTimeLeft is the duration word the window shows: 1 short to 4 very long.
func auctionTimeLeft(ms uint32) int {
	switch {
	case ms <= 30*60*1000:
		return 1
	case ms <= 2*3600*1000:
		return 2
	case ms <= 12*3600*1000:
		return 3
	}
	return 4
}

// auctionItemName is the name of an auctioned item, empty until it is known.
func (rt *Runtime) auctionItemName(entry uint32) (string, uint32, uint32) {
	if template := rt.itemTemplate(entry); template != nil {
		return template.Name, template.Quality, template.RequiredLevel
	}
	return "", 1, 0
}

func (rt *Runtime) auctionOwnerName(guid uint64) string {
	if host := rt.auctionHost(); host != nil {
		return host.AuctionPlayerName(guid)
	}
	return ""
}

// sortAuctionList puts a list in the order its sort asks for.
func (rt *Runtime) sortAuctionList(kind int) {
	list := &rt.auction.lists[kind]
	if len(list.sort) == 0 {
		return
	}
	value := func(entry world.AuctionEntry, column string) (number float64, text string, isText bool) {
		name, quality, level := rt.auctionItemName(entry.Entry)
		switch column {
		case "quality":
			return float64(quality), "", false
		case "level":
			return float64(level), "", false
		case "duration":
			return float64(entry.TimeLeftMS), "", false
		case "bid":
			if entry.Bid > 0 {
				return float64(entry.Bid), "", false
			}
			return float64(entry.StartBid), "", false
		case "quantity":
			return float64(entry.Count), "", false
		case "buyout", "minbidbuyout":
			if entry.Buyout > 0 {
				return float64(entry.Buyout), "", false
			}
			if entry.Bid > 0 {
				return float64(entry.Bid), "", false
			}
			return float64(entry.StartBid), "", false
		case "name":
			return 0, strings.ToLower(name), true
		case "seller":
			return 0, strings.ToLower(rt.auctionOwnerName(entry.Owner)), true
		}
		return 0, "", false
	}
	sort.SliceStable(list.entries, func(a, b int) bool {
		for _, spec := range list.sort {
			numberA, textA, isText := value(list.entries[a], spec.column)
			numberB, textB, _ := value(list.entries[b], spec.column)
			var order int
			if isText {
				order = strings.Compare(textA, textB)
			} else if numberA < numberB {
				order = -1
			} else if numberA > numberB {
				order = 1
			}
			if order == 0 {
				continue
			}
			if spec.reverse {
				return order > 0
			}
			return order < 0
		}
		return false
	})
}

// auctionClasses are the item classes the window filters by, in table order.
func (rt *Runtime) auctionClasses() []uint32 {
	var ids []uint32
	for id, name := range rt.inventory.classes.Class {
		if strings.Contains(strings.ToLower(name), "obsolete") {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	return ids
}

func (rt *Runtime) auctionSubClasses(class uint32) []uint32 {
	var ids []uint32
	for key, name := range rt.inventory.classes.SubClass {
		if key[0] != class || strings.Contains(strings.ToLower(name), "obsolete") {
			continue
		}
		ids = append(ids, key[1])
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	return ids
}

// auctionInvTypes are the inventory types the items of a class and subclass
// take.
func (rt *Runtime) auctionInvTypes(classIndex, subIndex int) []uint32 {
	state := &rt.auction
	key := [2]int{classIndex, subIndex}
	if cached, ok := state.invTypes[key]; ok {
		return cached
	}
	classes := rt.auctionClasses()
	if classIndex < 1 || classIndex > len(classes) {
		return nil
	}
	class := classes[classIndex-1]
	subs := rt.auctionSubClasses(class)
	if subIndex < 1 || subIndex > len(subs) {
		return nil
	}
	sub := subs[subIndex-1]
	seen := map[uint32]bool{}
	for _, local := range rt.inventory.local {
		if local.Class == class && local.SubClass == sub && local.InventoryType != 0 {
			seen[local.InventoryType] = true
		}
	}
	var kinds []uint32
	for kind := range seen {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(a, b int) bool { return kinds[a] < kinds[b] })
	if state.invTypes == nil {
		state.invTypes = map[[2]int][]uint32{}
	}
	state.invTypes[key] = kinds
	return kinds
}

// auctionDeposit is the deposit of putting an item up for a run time.
func (rt *Runtime) auctionDeposit(sellPrice, count uint32, minutes int) uint32 {
	rate := 0.0
	if host := rt.auctionHost(); host != nil {
		rate = host.AuctionDepositRate(rt.auction.house)
	}
	if sellPrice == 0 {
		return auctionMinimumDeposit
	}
	multiplier := rate * 3 / 100
	hours := uint32(minutes * 60 / 3600 / 12)
	base := float64(sellPrice) * multiplier
	deposit := uint32(base)
	remainder := base - float64(deposit)
	deposit *= hours * count
	whole := int(count)
	for whole > 0 && remainder*float64(whole) != float64(uint32(remainder*float64(whole))) {
		whole--
	}
	if whole > 0 {
		deposit += uint32(remainder * float64(whole) * float64(hours))
	}
	return max(deposit, auctionMinimumDeposit)
}

// sellItem is the item in the sell slot.
func (rt *Runtime) sellItem() *ItemInstance {
	if rt.auction.sellGUID == 0 {
		return nil
	}
	return rt.inventory.items[rt.auction.sellGUID]
}

func (rt *Runtime) auctionEntryAt(list, index int) (world.AuctionEntry, bool) {
	entries := rt.auction.lists[list].entries
	if index < 1 || index > len(entries) {
		return world.AuctionEntry{}, false
	}
	return entries[index-1], true
}

// registerAuctionAPI defines the calls the auction house window makes.
func registerAuctionAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	state := &rt.auction
	send := func(opcode world.ClientOpcode, body []byte) {
		if host := rt.auctionHost(); host != nil && state.open {
			_ = host.AuctionSend(opcode, body)
		}
	}
	reg("CloseAuctionHouse", func(L *lua.LState) int { rt.CloseAuctionHouse(); return 0 })
	reg("CanSendAuctionQuery", func(L *lua.LState) int {
		L.Push(lua.LBool(state.open && !time.Now().Before(state.nextQuery)))
		L.Push(lua.LFalse)
		return 2
	})
	reg("GetNumAuctionItems", func(L *lua.LState) int {
		list := &state.lists[auctionListIndex(L.CheckString(1))]
		L.Push(lua.LNumber(len(list.entries)))
		L.Push(lua.LNumber(max(list.total, len(list.entries))))
		return 2
	})
	reg("GetAuctionItemInfo", func(L *lua.LState) int {
		entry, ok := rt.auctionEntryAt(auctionListIndex(L.CheckString(1)), L.CheckInt(2))
		if !ok {
			return 0
		}
		name, quality, level := rt.auctionItemName(entry.Entry)
		own := uint64(0)
		if info := rt.unitInfo("player"); info != nil {
			own = info.GUID
		}
		owner := lua.LValue(lua.LNil)
		if text := rt.auctionOwnerName(entry.Owner); text != "" {
			owner = lua.LString(text)
		}
		L.Push(lua.LString(name))
		L.Push(lua.LString(rt.itemIcon(entry.Entry)))
		L.Push(lua.LNumber(entry.Count))
		L.Push(lua.LNumber(quality))
		L.Push(lua.LTrue)
		L.Push(lua.LNumber(level))
		L.Push(lua.LNumber(entry.StartBid))
		L.Push(lua.LNumber(entry.MinOutbid))
		L.Push(lua.LNumber(entry.Buyout))
		L.Push(lua.LNumber(entry.Bid))
		L.Push(lua.LBool(entry.Bidder != 0 && entry.Bidder == own))
		L.Push(owner)
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(entry.Entry))
		return 14
	})
	reg("GetAuctionItemTimeLeft", func(L *lua.LState) int {
		if entry, ok := rt.auctionEntryAt(auctionListIndex(L.CheckString(1)), L.CheckInt(2)); ok {
			L.Push(lua.LNumber(auctionTimeLeft(entry.TimeLeftMS)))
			return 1
		}
		return 0
	})
	reg("GetAuctionItemLink", func(L *lua.LState) int {
		if entry, ok := rt.auctionEntryAt(auctionListIndex(L.CheckString(1)), L.CheckInt(2)); ok {
			if link := rt.itemLinkFor(entry.Entry, entry.Enchant); link != "" {
				L.Push(lua.LString(link))
				return 1
			}
		}
		return 0
	})
	reg("SetSelectedAuctionItem", func(L *lua.LState) int {
		state.lists[auctionListIndex(L.CheckString(1))].selected = L.CheckInt(2)
		return 0
	})
	reg("GetSelectedAuctionItem", func(L *lua.LState) int {
		L.Push(lua.LNumber(state.lists[auctionListIndex(L.CheckString(1))].selected))
		return 1
	})
	reg("QueryAuctionItems", func(L *lua.LState) int {
		query := world.AuctionQuery{Name: L.OptString(1, ""), Slot: 0xFFFFFFFF, Class: 0xFFFFFFFF, SubClass: 0xFFFFFFFF, Quality: 0xFFFFFFFF}
		query.MinLevel, query.MaxLevel = uint8(toInt(L, 2)), uint8(toInt(L, 3))
		classes := rt.auctionClasses()
		var class uint32
		if index := toInt(L, 5); index >= 1 && index <= len(classes) {
			class = classes[index-1]
			query.Class = class
			if sub := toInt(L, 6); sub >= 1 {
				if subs := rt.auctionSubClasses(class); sub <= len(subs) {
					query.SubClass = subs[sub-1]
					if slot := toInt(L, 4); slot >= 1 {
						if kinds := rt.auctionInvTypes(toInt(L, 5), sub); slot <= len(kinds) {
							query.Slot = kinds[slot-1]
						}
					}
				}
			}
		}
		query.Offset = uint32(toInt(L, 7) * 50)
		query.Usable = L.OptBool(8, false)
		if quality := L.Get(9); quality != lua.LNil && toInt(L, 9) >= 0 {
			query.Quality = uint32(toInt(L, 9))
		}
		query.GetAll = L.OptBool(10, false)
		state.nextQuery = time.Now().Add(time.Duration(max(state.delayMS, 300)) * time.Millisecond)
		send(world.ClientAuctionListItems, world.BuildAuctionListItems(state.auctioneer, query))
		return 0
	})
	reg("GetOwnerAuctionItems", func(L *lua.LState) int {
		send(world.ClientAuctionListOwner, world.BuildAuctionListOwner(state.auctioneer))
		return 0
	})
	reg("GetBidderAuctionItems", func(L *lua.LState) int {
		send(world.ClientAuctionListBidder, world.BuildAuctionListBidder(state.auctioneer))
		return 0
	})
	reg("PlaceAuctionBid", func(L *lua.LState) int {
		if entry, ok := rt.auctionEntryAt(auctionListIndex(L.CheckString(1)), L.CheckInt(2)); ok {
			send(world.ClientAuctionPlaceBid, world.BuildAuctionPlaceBid(state.auctioneer, entry.ID, uint32(L.CheckNumber(3))))
		}
		return 0
	})
	reg("CancelAuction", func(L *lua.LState) int {
		if entry, ok := rt.auctionEntryAt(auctionListOwner, L.CheckInt(1)); ok {
			send(world.ClientAuctionRemoveItem, world.BuildAuctionRemove(state.auctioneer, entry.ID))
		}
		return 0
	})
	reg("CanCancelAuction", func(L *lua.LState) int {
		_, ok := rt.auctionEntryAt(auctionListOwner, state.lists[auctionListOwner].selected)
		L.Push(lua.LBool(ok))
		return 1
	})
	reg("GetCancelAuctionCost", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	// Sorting.
	reg("SortAuctionClearSort", func(L *lua.LState) int {
		state.lists[auctionListIndex(L.CheckString(1))].sort = nil
		return 0
	})
	reg("SortAuctionSetSort", func(L *lua.LState) int {
		list := &state.lists[auctionListIndex(L.CheckString(1))]
		list.sort = append(list.sort, auctionSortSpec{column: strings.ToLower(L.CheckString(2)), reverse: L.OptBool(3, false)})
		return 0
	})
	reg("SortAuctionApplySort", func(L *lua.LState) int {
		kind := auctionListIndex(L.CheckString(1))
		rt.sortAuctionList(kind)
		switch kind {
		case auctionListBrowse:
			rt.FireEvent("AUCTION_ITEM_LIST_UPDATE")
		case auctionListBidder:
			rt.FireEvent("AUCTION_BIDDER_LIST_UPDATE")
		default:
			rt.FireEvent("AUCTION_OWNED_LIST_UPDATE")
		}
		return 0
	})
	reg("GetAuctionSort", func(L *lua.LState) int {
		list := &state.lists[auctionListIndex(L.CheckString(1))]
		if index := L.CheckInt(2); index >= 1 && index <= len(list.sort) {
			L.Push(lua.LString(list.sort[index-1].column))
			L.Push(lua.LBool(list.sort[index-1].reverse))
			return 2
		}
		return 0
	})
	// Filters.
	reg("GetAuctionItemClasses", func(L *lua.LState) int {
		ids := rt.auctionClasses()
		for _, id := range ids {
			L.Push(lua.LString(rt.inventory.classes.Class[id]))
		}
		return len(ids)
	})
	reg("GetAuctionItemSubClasses", func(L *lua.LState) int {
		classes := rt.auctionClasses()
		index := L.CheckInt(1)
		if index < 1 || index > len(classes) {
			return 0
		}
		subs := rt.auctionSubClasses(classes[index-1])
		for _, sub := range subs {
			L.Push(lua.LString(rt.inventory.classes.SubClass[[2]uint32{classes[index-1], sub}]))
		}
		return len(subs)
	})
	reg("GetAuctionInvTypes", func(L *lua.LState) int {
		kinds := rt.auctionInvTypes(L.CheckInt(1), L.CheckInt(2))
		for _, kind := range kinds {
			L.Push(lua.LString("INVTYPE_" + inventoryTypeConstant(kind)))
			L.Push(lua.LTrue)
		}
		return len(kinds) * 2
	})
	reg("GetAuctionHouseDefaultFilter", func(L *lua.LState) int {
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 2
	})
	// Selling.
	reg("ClickAuctionSellItemButton", func(L *lua.LState) int {
		cursor := &rt.inventory.cursor
		if cursor.active && cursor.bag != cursorGuildBank {
			if item := rt.inventory.items[cursor.guid]; item != nil {
				state.sellGUID = item.GUID
				if rt.clearItemCursor != nil {
					rt.clearItemCursor()
				}
				rt.FireEvent("NEW_AUCTION_UPDATE")
			}
			return 0
		}
		if state.sellGUID != 0 {
			state.sellGUID = 0
			rt.FireEvent("NEW_AUCTION_UPDATE")
		}
		return 0
	})
	reg("GetAuctionSellItemInfo", func(L *lua.LState) int {
		item := rt.sellItem()
		if item == nil {
			return 0
		}
		name, quality, price := "", uint32(1), uint32(0)
		if template := rt.itemTemplate(item.Entry); template != nil {
			name, quality, price = template.Name, template.Quality, template.SellPrice
		}
		L.Push(lua.LString(name))
		L.Push(lua.LString(rt.itemIcon(item.Entry)))
		L.Push(lua.LNumber(item.Count))
		L.Push(lua.LNumber(quality))
		L.Push(lua.LTrue)
		L.Push(lua.LNumber(price))
		L.Push(lua.LNumber(item.Count))
		return 7
	})
	reg("CalculateAuctionDeposit", func(L *lua.LState) int {
		item := rt.sellItem()
		if item == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		price := uint32(0)
		if template := rt.itemTemplate(item.Entry); template != nil {
			price = template.SellPrice
		}
		count := uint32(max(L.OptInt(2, item.Count), 1))
		L.Push(lua.LNumber(rt.auctionDeposit(price, count, L.CheckInt(1))))
		return 1
	})
	reg("StartAuction", func(L *lua.LState) int {
		item := rt.sellItem()
		if item == nil {
			return 0
		}
		bid, buyout, minutes := uint32(L.CheckNumber(1)), uint32(L.CheckNumber(2)), uint32(L.CheckInt(3))
		stack, stacks := uint32(max(L.OptInt(4, item.Count), 1)), max(L.OptInt(5, 1), 1)
		if stacks > 1 {
			rt.FireEvent("AUCTION_MULTISELL_START", numberValue(stacks))
		}
		for index := 0; index < stacks; index++ {
			send(world.ClientAuctionSellItem, world.BuildAuctionSell(state.auctioneer, item.GUID, stack, bid, buyout, minutes))
			if stacks > 1 {
				rt.FireEvent("AUCTION_MULTISELL_UPDATE", numberValue(index+1), numberValue(stacks))
			}
		}
		state.sellGUID = 0
		rt.FireEvent("NEW_AUCTION_UPDATE")
		return 0
	})
	reg("CloseAuctionStaticPopups", func(L *lua.LState) int { return 0 })
	reg("SetAuctionsTabShowing", func(L *lua.LState) int { return 0 })
}

// refreshAuction redraws the open auction house after item data arrived.
func (rt *Runtime) refreshAuction() {
	if !rt.auction.open {
		return
	}
	for kind := range rt.auction.lists {
		rt.sortAuctionList(kind)
	}
	rt.FireEvent("AUCTION_ITEM_LIST_UPDATE")
	rt.FireEvent("AUCTION_BIDDER_LIST_UPDATE")
	rt.FireEvent("AUCTION_OWNED_LIST_UPDATE")
}
