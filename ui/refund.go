package ui

import (
	"encoding/binary"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The packets of buying an item with honor, arena points or items and taking
// it back within a while of play.
const (
	refundInfoRequest  = world.ClientOpcode(0x04B3)
	refundInfoResponse = uint16(0x04B2)
	refundResult       = uint16(0x04B5)
	// templateRefundable is the item flag of what may be taken back.
	templateRefundable = 0x00001000
	// refundWindow is how many seconds of play an item can be returned in.
	refundWindow = 7200
	// The most points a character may hold.
	maxHonorPoints = 75000
	maxArenaPoints = 10000
)

// refundInfo is what the server says an item cost.
type refundInfo struct {
	money, honor, arena uint32
	items, counts       [5]uint32
	bought              int32
}

type refundState struct {
	info      map[uint64]*refundInfo
	requested map[uint64]bool
	// playedTotal and playedAt place the played time of the character.
	playedTotal int
	playedAt    time.Time
	playedKnown bool
	// quietPlayed counts the played time answers nobody asked to see.
	quietPlayed int
}

// playedNow is how many seconds the character has played.
func (rt *Runtime) playedNow() int {
	if !rt.refund.playedKnown {
		return 0
	}
	return rt.refund.playedTotal + int(time.Since(rt.refund.playedAt).Seconds())
}

// notePlayed takes the played time the server sent.
func (rt *Runtime) notePlayed(total int) {
	rt.refund.playedTotal, rt.refund.playedAt, rt.refund.playedKnown = total, time.Now(), true
}

// requestRefundInfo asks about every refundable item the character carries.
func (rt *Runtime) requestRefundInfo() {
	state := &rt.refund
	ask := func(item *ItemInstance) {
		if item == nil || state.requested[item.GUID] || state.info[item.GUID] != nil {
			return
		}
		if template := rt.itemTemplate(item.Entry); template == nil || template.Flags&templateRefundable == 0 {
			return
		}
		if state.requested == nil {
			state.requested = make(map[uint64]bool)
		}
		state.requested[item.GUID] = true
		if !state.playedKnown {
			state.quietPlayed++
			rt.send(clientPlayedTime, []byte{0})
		}
		rt.send(refundInfoRequest, binary.LittleEndian.AppendUint64(nil, item.GUID))
	}
	slots := rt.inventory.slots
	for slot := invEquipStart; slot < invEquipEnd; slot++ {
		ask(rt.inventory.items[slots[slot]])
	}
	for slot := invPackStart; slot < invPackEnd; slot++ {
		ask(rt.inventory.items[slots[slot]])
	}
	for slot := invBagStart; slot < invBagEnd; slot++ {
		if container := rt.inventory.items[slots[slot]]; container != nil {
			for _, guid := range container.Slots {
				ask(rt.inventory.items[guid])
			}
		}
	}
}

// handleRefundPacket takes the answers about what items cost and the result of
// taking one back.
func (rt *Runtime) handleRefundPacket(packet world.Packet) bool {
	switch packet.Opcode {
	case refundInfoResponse:
		reader := world.NewReader(packet.Body, "SMSG_ITEM_REFUND_INFO_RESPONSE")
		guid, err := reader.U64()
		if err != nil {
			return true
		}
		info := &refundInfo{}
		for _, field := range []*uint32{&info.money, &info.honor, &info.arena} {
			if *field, err = reader.U32(); err != nil {
				return true
			}
		}
		for index := range info.items {
			if info.items[index], err = reader.U32(); err != nil {
				return true
			}
			if info.counts[index], err = reader.U32(); err != nil {
				return true
			}
		}
		if _, err = reader.U32(); err != nil {
			return true
		}
		bought, err := reader.U32()
		if err != nil {
			return true
		}
		info.bought = int32(bought)
		if rt.refund.info == nil {
			rt.refund.info = make(map[uint64]*refundInfo)
		}
		rt.refund.info[guid] = info
		return true
	case refundResult:
		rt.applyRefundResult(packet.Body)
		return true
	}
	return false
}

// applyRefundResult tells what a returned item gave back, or why it stayed.
func (rt *Runtime) applyRefundResult(body []byte) {
	reader := world.NewReader(body, "SMSG_ITEM_REFUND_RESULT")
	guid, err := reader.U64()
	if err != nil {
		return
	}
	code, err := reader.U32()
	if err != nil {
		return
	}
	if code != 0 {
		switch code {
		case 10:
			rt.ShowGameError(0)
		case 11:
			rt.ShowGameError(712)
		default:
			rt.systemLine(rt.globalString("UNABLE_TO_REFUND_ITEM", "Unable to refund item."))
		}
		return
	}
	delete(rt.refund.info, guid)
	var money, honor, arena uint32
	var items, counts [5]uint32
	for _, field := range []*uint32{&money, &honor, &arena} {
		if *field, err = reader.U32(); err != nil {
			return
		}
	}
	for index := range items {
		if items[index], err = reader.U32(); err != nil {
			return
		}
		if counts[index], err = reader.U32(); err != nil {
			return
		}
	}
	rt.systemLine(rt.globalString("ITEM_REFUND_MSG", "You have been refunded:"))
	if money > 0 {
		rt.systemLine(rt.moneyText(money))
	}
	format := func(amount uint32, name string) string {
		return rt.formatGlobal("CURRENCY_AMOUNT_REFUND_FORMAT", "%d %s", amount, name)
	}
	if honor > 0 {
		have := rt.currency.honor
		if honor+have >= maxHonorPoints+1 {
			honor = maxHonorPoints - have
		}
		rt.systemLine(format(honor, rt.globalString("HONOR_POINTS", "Honor Points")))
	}
	if arena > 0 {
		have := rt.currency.arena
		if arena+have >= maxArenaPoints+1 {
			arena = maxArenaPoints - have
		}
		rt.systemLine(format(arena, rt.globalString("ARENA_POINTS", "Arena Points")))
	}
	for index, entry := range items {
		if counts[index] == 0 {
			continue
		}
		if template := rt.itemTemplate(entry); template != nil {
			rt.systemLine(format(counts[index], template.Name))
		}
	}
}

// refundItem is the item a purchase call names: the one in a bag square or
// worn.
func (rt *Runtime) refundItem(L *lua.LState, equippedArg int) *ItemInstance {
	if L.OptInt(equippedArg, 0) != 0 {
		return rt.equippedItem(L.CheckInt(2))
	}
	return rt.bagItem(L.CheckInt(1), L.CheckInt(2))
}

// refundLeft is how many seconds of play an item can still be returned in.
func (rt *Runtime) refundLeft(info *refundInfo) int {
	return int(info.bought) - rt.playedNow() + refundWindow
}

func registerRefundAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetContainerItemPurchaseInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetContainerItemPurchaseInfo(index, slot[, isEquipped])")
			return 0
		}
		item := rt.refundItem(L, 3)
		if item == nil {
			return 0
		}
		info := rt.refund.info[item.GUID]
		if info == nil || rt.itemHasEnchants(item) {
			return 0
		}
		left := rt.refundLeft(info)
		if left <= 0 {
			return 0
		}
		count := 0
		for _, amount := range info.counts {
			if amount > 0 {
				count++
			}
		}
		L.Push(lua.LNumber(info.money))
		L.Push(lua.LNumber(info.honor))
		L.Push(lua.LNumber(info.arena))
		L.Push(lua.LNumber(count))
		L.Push(lua.LNumber(left))
		return 5
	})
	reg("GetContainerItemPurchaseItem", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber || L.Get(3).Type() != lua.LTNumber {
			L.RaiseError("Usage: ContainerItemPurchaseItem(bag, slot, itemIndex[, isEquipped])")
			return 0
		}
		item := rt.refundItem(L, 4)
		if item == nil {
			return 0
		}
		info := rt.refund.info[item.GUID]
		if info == nil || rt.itemHasEnchants(item) || rt.refundLeft(info) <= 0 {
			return 0
		}
		want, seen := L.CheckInt(3)-1, 0
		for index, amount := range info.counts {
			if amount == 0 {
				continue
			}
			if seen == want {
				entry := info.items[index]
				if icon := rt.itemIcon(entry); icon != "" {
					L.Push(lua.LString(icon))
				} else {
					L.Push(lua.LNil)
				}
				L.Push(lua.LNumber(amount))
				if link := rt.itemLinkFor(entry, 0); link != "" {
					L.Push(lua.LString(link))
				} else {
					L.Push(lua.LNil)
				}
				return 3
			}
			seen++
		}
		return 0
	})
	reg("ContainerRefundItemPurchase", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber || L.Get(2).Type() != lua.LTNumber {
			L.RaiseError("Usage: ContainerRefundItemPurchase(index, slot[, isEquipped])")
			return 0
		}
		item := rt.refundItem(L, 3)
		if item == nil || rt.refund.info[item.GUID] == nil || !rt.merchant.open {
			rt.ShowGameError(0xc)
			return 0
		}
		rt.send(refundOpcode, binary.LittleEndian.AppendUint64(nil, item.GUID))
		return 0
	})
}

// itemHasEnchants is whether the item carries an enchantment or a gem, which
// keeps it from being returned.
func (rt *Runtime) itemHasEnchants(item *ItemInstance) bool {
	template := rt.itemTemplate(item.Entry)
	if template != nil && template.Flags&templateNoEnchantBind != 0 {
		return false
	}
	for _, slot := range [...]int{0, 2, 3, 6} {
		if item.Enchants[slot] != 0 {
			return true
		}
	}
	return false
}
