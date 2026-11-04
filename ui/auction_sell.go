package ui

import (
	"encoding/binary"
	"sort"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The reasons an item cannot be put up for auction, by the number of the game
// error that words them.
const (
	auctionErrNotFound  = 25
	auctionErrRepair    = 400
	auctionErrCharges   = 401
	auctionErrQuest     = 388
	auctionErrBound     = 389
	auctionErrConjured  = 390
	auctionErrLimited   = 391
	auctionErrWrapped   = 392
	auctionErrLoot      = 393
	auctionErrBag       = 394
	auctionErrNotEnough = 724
)

// Item flags that matter to the auction house.
const (
	itemFlagSoulbound     = 0x00000001
	itemFlagWrapped       = 0x00000008
	templateConjured      = 0x00000002
	templateLootable      = 0x00000004
	templateNoEnchantBind = 0x00002000
	templateRealTime      = 0x00010000
	itemBondingQuest      = 4
	auctionStackLimit     = 199
)

// auctionRunMinutes is how long an auction runs for the run time the window
// picks, 1 to 3; anything else runs a day.
func auctionRunMinutes(runTime int) uint32 {
	switch runTime {
	case 1:
		return 720
	case 2:
		return 1440
	case 3:
		return 2880
	}
	return 1440
}

// SetSoulboundEnchants gives the enchantments that bind an item to its owner.
func (rt *Runtime) SetSoulboundEnchants(ids map[uint32]bool) { rt.soulboundEnchants = ids }

// itemBoundToOwner is whether the item belongs to its owner: it is soulbound, or an
// enchantment on it binds it.
func (rt *Runtime) itemBoundToOwner(item *ItemInstance, template *world.ItemTemplate) bool {
	if item.Flags&itemFlagSoulbound != 0 {
		return true
	}
	if template.Flags&templateNoEnchantBind == 0 {
		for _, enchant := range item.Enchants {
			if enchant != 0 && rt.soulboundEnchants[enchant] {
				return true
			}
		}
	}
	return false
}

// auctionItemError is the number of the error that keeps an item from being
// sold at auction, zero when it can be.
func (rt *Runtime) auctionItemError(item *ItemInstance) int {
	template := rt.itemTemplate(item.Entry)
	if template == nil {
		return auctionErrNotFound
	}
	wrapped := item.Flags&itemFlagWrapped != 0
	if !wrapped && item.MaxDurability != 0 && item.Durability < item.MaxDurability {
		return auctionErrRepair
	}
	if template.Spells[0].ID != 0 && item.Charges != template.Spells[0].Charges {
		return auctionErrCharges
	}
	if rt.itemBoundToOwner(item, template) {
		if template.Bonding == itemBondingQuest {
			return auctionErrQuest
		}
		return auctionErrBound
	}
	if template.Flags&templateConjured != 0 {
		return auctionErrConjured
	}
	if (template.Flags&templateRealTime != 0 && template.Duration > 0) || template.HolidayID != 0 || item.Duration != 0 {
		return auctionErrLimited
	}
	if template.Area != 0 || template.Map != 0 {
		return auctionErrConjured
	}
	for _, slot := range item.Slots {
		if slot != 0 {
			return auctionErrBag
		}
	}
	if wrapped {
		return auctionErrWrapped
	}
	if template.Flags&templateLootable != 0 {
		return auctionErrLoot
	}
	return 0
}

// multiSellPart is a stack of items waiting to be sold in a run of auctions.
type multiSellPart struct {
	guid  uint64
	count uint32
}

// multiSell is a run of auctions that goes out one at a time, each when the
// server has answered the one before.
type multiSell struct {
	active bool
	house  uint64
	bid    uint32
	buyout uint32
	run    uint32
	stack  uint32
	total  int
	left   int
	parts  []multiSellPart
}

// clearSellSlot empties the sell slot and tells the window.
func (rt *Runtime) clearSellSlot() {
	if rt.auction.sellGUID != 0 {
		rt.auction.sellGUID = 0
		rt.FireEvent("NEW_AUCTION_UPDATE")
	}
}

// endMultiSell finishes the run; failure says the window so.
func (rt *Runtime) endMultiSell(failure bool) {
	rt.multiSell.active = false
	rt.clearSellSlot()
	if failure {
		rt.FireEvent("AUCTION_MULTISELL_FAILURE")
	}
}

// sendNextAuction puts the next stack of the run up for auction.
func (rt *Runtime) sendNextAuction() {
	sell := &rt.multiSell
	if sell.left == 0 {
		sell.active = false
		rt.clearSellSlot()
		return
	}
	sell.left--
	index, need := len(sell.parts)-1, sell.stack
	for sell.parts[index].count < need {
		need -= sell.parts[index].count
		index--
		if index < 0 {
			rt.endMultiSell(true)
			return
		}
	}
	body := binary.LittleEndian.AppendUint64(nil, sell.house)
	body = binary.LittleEndian.AppendUint32(body, uint32(len(sell.parts)-index))
	for last := len(sell.parts) - 1; last > index; last-- {
		body = binary.LittleEndian.AppendUint64(body, sell.parts[last].guid)
		body = binary.LittleEndian.AppendUint32(body, sell.parts[last].count)
	}
	body = binary.LittleEndian.AppendUint64(body, sell.parts[index].guid)
	body = binary.LittleEndian.AppendUint32(body, need)
	sell.parts[index].count -= need
	if sell.parts[index].count != 0 {
		index++
	}
	sell.parts = sell.parts[:index]
	body = binary.LittleEndian.AppendUint32(body, sell.bid)
	body = binary.LittleEndian.AppendUint32(body, sell.buyout)
	body = binary.LittleEndian.AppendUint32(body, sell.run)
	if host := rt.auctionHost(); host != nil {
		_ = host.AuctionSend(world.ClientAuctionSellItem, body)
	}
}

// auctionSellable gathers the items of an entry that can go to auction until
// they make up the count.
func (rt *Runtime) auctionSellable(entry, count uint32) []multiSellPart {
	var guids []uint64
	for guid, item := range rt.inventory.items {
		if item.Entry == entry {
			guids = append(guids, guid)
		}
	}
	sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
	var parts []multiSellPart
	total := uint32(0)
	for _, guid := range guids {
		item := rt.inventory.items[guid]
		if rt.auctionItemError(item) != 0 {
			continue
		}
		total += uint32(item.Count)
		if len(parts) >= auctionStackLimit {
			return nil
		}
		parts = append(parts, multiSellPart{guid, uint32(item.Count)})
		if count != 0 && count <= total {
			return parts
		}
	}
	return nil
}

func registerAuctionSellAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("StartAuction", func(L *lua.LState) int {
		for index := 1; index <= 3; index++ {
			if L.Get(index).Type() != lua.LTNumber {
				L.RaiseError("Usage: StartAuction(minBid, buyoutPrice, runTime, count)")
				return 0
			}
		}
		state := &rt.auction
		item := rt.sellItem()
		if !state.open || item == nil {
			return 0
		}
		if template := rt.itemTemplate(item.Entry); template != nil {
			if item.MaxDurability != 0 && item.Durability < item.MaxDurability {
				rt.ShowGameError(auctionErrRepair)
				return 0
			}
			if template.Spells[0].ID != 0 && item.Charges != template.Spells[0].Charges {
				rt.ShowGameError(auctionErrCharges)
				return 0
			}
		}
		bid := uint32(L.CheckNumber(1))
		buyout := uint32(L.CheckNumber(2))
		if buyout != 0 && buyout < bid {
			buyout = bid
		}
		run := auctionRunMinutes(int(L.CheckNumber(3)))
		stack, stacks := uint32(0), 1
		if L.Get(4).Type() == lua.LTNumber {
			stack = uint32(L.CheckNumber(4))
		}
		if L.Get(5).Type() == lua.LTNumber {
			stacks = int(L.CheckNumber(5))
		}
		if stack <= uint32(item.Count) && stacks < 2 {
			body := binary.LittleEndian.AppendUint64(nil, state.auctioneer)
			body = binary.LittleEndian.AppendUint32(body, 1)
			body = binary.LittleEndian.AppendUint64(body, item.GUID)
			body = binary.LittleEndian.AppendUint32(body, stack)
			body = binary.LittleEndian.AppendUint32(body, bid)
			body = binary.LittleEndian.AppendUint32(body, buyout)
			body = binary.LittleEndian.AppendUint32(body, run)
			if host := rt.auctionHost(); host != nil {
				_ = host.AuctionSend(world.ClientAuctionSellItem, body)
			}
			rt.clearSellSlot()
			return 0
		}
		parts := rt.auctionSellable(item.Entry, stack*uint32(stacks))
		if len(parts) == 0 {
			rt.ShowGameError(auctionErrNotEnough)
			rt.clearSellSlot()
			return 0
		}
		rt.multiSell = multiSell{house: state.auctioneer, bid: bid, buyout: buyout, run: run, stack: stack, total: stacks, left: stacks, parts: parts}
		if stacks > 1 {
			rt.multiSell.active = true
			rt.FireEvent("AUCTION_MULTISELL_START", numberValue(stacks))
			rt.sendNextAuction()
			return 0
		}
		rt.sendNextAuction()
		rt.clearSellSlot()
		return 0
	})
	reg("CancelSell", func(L *lua.LState) int {
		if rt.multiSell.active {
			rt.endMultiSell(true)
		}
		return 0
	})
}

// multiSellAnswer is the server's answer to an auction put up: the run goes
// on after a good one and ends on a bad one.
func (rt *Runtime) multiSellAnswer(result world.AuctionResult) {
	if !rt.multiSell.active || result.Command != world.AuctionCommandSell {
		return
	}
	if result.Error != world.AuctionOK {
		rt.endMultiSell(true)
		return
	}
	rt.FireEvent("AUCTION_MULTISELL_UPDATE", numberValue(rt.multiSell.total-rt.multiSell.left), numberValue(rt.multiSell.total))
	rt.sendNextAuction()
}
