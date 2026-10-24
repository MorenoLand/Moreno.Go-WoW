package ui

import (
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
)

// Slot ranges of the player's own item array (PLAYER_FIELD_INV_SLOT_HEAD), as
// the server numbers them. The Lua API numbers equipment slots from 1 (slot n
// is server slot n-1) and bags by container id.
const (
	invEquipStart   = 0
	invEquipEnd     = 19
	invBagStart     = 19
	invBagEnd       = 23
	invPackStart    = 23
	invPackEnd      = 39
	invBankStart    = 39
	invBankEnd      = 67
	invBankBagStart = 67
	invBankBagEnd   = 74
	invBuybackStart = 74
	invBuybackEnd   = 86
	invKeyStart     = 86
	invKeyEnd       = 118
	invSlotCount    = 150
	backpackSlots   = invPackEnd - invPackStart
	bankSlots       = invBankEnd - invBankStart
	keyringSlots    = invKeyEnd - invKeyStart
)

// ItemInstance is one item object the server told us about.
type ItemInstance struct {
	GUID                      uint64
	Entry                     uint32
	Count                     int
	Durability, MaxDurability int
	Contained, Owner          uint64
	Flags                     uint32
	NumSlots                  int      // containers: how many squares
	Slots                     []uint64 // containers: guid in each square
	Enchant                   uint32
}

// ItemLocal is what Item.dbc says about an entry: enough to draw an icon
// before the server has answered the item query.
type ItemLocal struct {
	DisplayID                      uint32
	Class, SubClass, InventoryType uint32
}

// ItemClassNames are the localized class and subclass names of ItemClass.dbc
// and ItemSubClass.dbc.
type ItemClassNames struct {
	Class    map[uint32]string
	SubClass map[[2]uint32]string
}

// WorldItemHost sends the item requests of the inventory API.
type WorldItemHost interface {
	QueryItem(entry uint32) error
	SwapItem(dstBag, dstSlot, srcBag, srcSlot uint8) error
	AutoEquipItem(bag, slot uint8) error
	AutoStoreBagItem(srcBag, srcSlot, dstBag uint8) error
	SplitItem(srcBag, srcSlot, dstBag, dstSlot, count uint8) error
	DestroyItem(bag, slot, count uint8) error
	UseItem(bag, slot, castCount uint8, spellID uint32, itemGUID uint64, targetMask uint32, targetGUID uint64) error
}

type inventoryState struct {
	slots      [invSlotCount]uint64
	items      map[uint64]*ItemInstance
	templates  map[uint32]*world.ItemTemplate
	local      map[uint32]ItemLocal
	icons      map[uint32]string // display id -> icon texture path
	classes    ItemClassNames
	money      uint32
	queried    map[uint32]bool
	cursor     cursorItem
	splitCount int
}

// cursorItem remembers where the item on the cursor was picked up from.
type cursorItem struct {
	active bool
	bag    int // Lua bag id; -3 for an equipment slot
	slot   int // Lua slot, 1-based
	guid   uint64
}

func (state *inventoryState) ensure() {
	if state.items == nil {
		state.items = make(map[uint64]*ItemInstance)
		state.templates = make(map[uint32]*world.ItemTemplate)
		state.queried = make(map[uint32]bool)
	}
}

// SetItemCatalog installs the item data read from the client's tables.
func (rt *Runtime) SetItemCatalog(local map[uint32]ItemLocal, icons map[uint32]string, classes ItemClassNames) {
	rt.inventory.ensure()
	rt.inventory.local, rt.inventory.icons, rt.inventory.classes = local, icons, classes
}

// SetItemTemplate stores a server item query answer and announces it.
func (rt *Runtime) SetItemTemplate(template world.ItemTemplate) {
	rt.inventory.ensure()
	stored := template
	rt.inventory.templates[template.Entry] = &stored
	rt.FireEvent("GET_ITEM_INFO_RECEIVED", numberValue(template.Entry))
	rt.FireEvent("BAG_UPDATE", numberValue(0))
	rt.FireEvent("UNIT_INVENTORY_CHANGED", stringValue("player"))
	rt.refreshItemActions()
}

// SetInventory replaces the item objects and the player's slot array with what
// the latest world updates describe, and fires the events a change deserves.
func (rt *Runtime) SetInventory(slots [invSlotCount]uint64, items map[uint64]*ItemInstance, money uint32) {
	state := &rt.inventory
	state.ensure()
	previousSlots, previousItems, previousMoney := state.slots, state.items, state.money
	state.slots, state.items, state.money = slots, items, money
	changedBags := map[int]bool{}
	for slot := range slots {
		before, after := previousSlots[slot], slots[slot]
		if before == after && itemsSame(previousItems[before], items[after]) {
			continue
		}
		switch {
		case slot >= invEquipStart && slot < invEquipEnd, slot >= invBagStart && slot < invBagEnd:
			rt.FireEvent("UNIT_INVENTORY_CHANGED", stringValue("player"))
		case slot >= invPackStart && slot < invPackEnd:
			changedBags[0] = true
		case slot >= invBankStart && slot < invBankEnd:
			changedBags[-1] = true
		}
	}
	for index := 0; index < invBagEnd-invBagStart; index++ {
		container := state.items[slots[invBagStart+index]]
		old := previousItems[previousSlots[invBagStart+index]]
		if container == nil && old == nil {
			continue
		}
		if container == nil || old == nil || container.GUID != old.GUID || !sameSlots(container.Slots, old.Slots) || !containerContentsSame(container, old, items, previousItems) {
			changedBags[index+1] = true
		}
	}
	for bag := range changedBags {
		rt.FireEvent("BAG_UPDATE", numberValue(bag))
	}
	if len(changedBags) > 0 {
		rt.refreshItemActions()
	}
	if len(changedBags) > 0 {
		rt.FireEvent("BAG_UPDATE_COOLDOWN")
	}
	if money != previousMoney {
		rt.FireEvent("PLAYER_MONEY")
	}
	rt.requestMissingTemplates()
}

func itemsSame(a, b *ItemInstance) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Entry == b.Entry && a.Count == b.Count && a.Durability == b.Durability
}

func sameSlots(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func containerContentsSame(a, b *ItemInstance, itemsA, itemsB map[uint64]*ItemInstance) bool {
	for index, guid := range a.Slots {
		if !itemsSame(itemsA[guid], itemsB[b.Slots[index]]) {
			return false
		}
	}
	return true
}

// requestMissingTemplates asks the server about every item entry that has no
// template yet.
func (rt *Runtime) requestMissingTemplates() {
	host, ok := rt.Host.(WorldItemHost)
	if !ok {
		return
	}
	for _, item := range rt.inventory.items {
		rt.requestTemplate(host, item.Entry)
	}
}

func (rt *Runtime) requestTemplate(host WorldItemHost, entry uint32) {
	state := &rt.inventory
	if entry == 0 || state.templates[entry] != nil || state.queried[entry] {
		return
	}
	state.queried[entry] = true
	_ = host.QueryItem(entry)
}

// itemTemplate returns the template of an entry, asking the server for it the
// first time it is needed.
func (rt *Runtime) itemTemplate(entry uint32) *world.ItemTemplate {
	state := &rt.inventory
	if template := state.templates[entry]; template != nil && !template.NotFound {
		return template
	}
	if host, ok := rt.Host.(WorldItemHost); ok {
		rt.requestTemplate(host, entry)
	}
	return nil
}

// itemIcon is the icon texture path of an entry: from the server template's
// display id when there is one, otherwise from Item.dbc.
func (rt *Runtime) itemIcon(entry uint32) string {
	state := &rt.inventory
	display := uint32(0)
	if template := state.templates[entry]; template != nil && !template.NotFound {
		display = template.DisplayID
	}
	if display == 0 {
		display = state.local[entry].DisplayID
	}
	if icon := state.icons[display]; icon != "" {
		return `Interface\Icons\` + icon
	}
	return `Interface\Icons\INV_Misc_QuestionMark`
}

var itemQualityColors = [...]string{"9d9d9d", "ffffff", "1eff00", "0070dd", "a335ee", "ff8000", "e6cc80", "e6cc80"}

func itemQualityHex(quality uint32) string {
	if int(quality) >= len(itemQualityColors) {
		quality = 1
	}
	return itemQualityColors[quality]
}

// itemLinkFor writes the chat link of an item instance (or bare entry).
func (rt *Runtime) itemLinkFor(entry uint32, enchant uint32) string {
	template := rt.itemTemplate(entry)
	if template == nil {
		return ""
	}
	return fmt.Sprintf("|cff%s|Hitem:%d:%d:0:0:0:0:0:0:0|h[%s]|h|r", itemQualityHex(template.Quality), entry, enchant, template.Name)
}

// itemEntryFromLink reads the item id out of an item link or a plain number.
func itemEntryFromLink(text string) uint32 {
	index := strings.Index(text, "item:")
	if index < 0 {
		return 0
	}
	var entry uint32
	fmt.Sscanf(text[index+5:], "%d", &entry)
	return entry
}

// bagItem returns the item in a Lua bag and 1-based slot.
func (rt *Runtime) bagItem(bag, slot int) *ItemInstance {
	state := &rt.inventory
	if slot < 1 {
		return nil
	}
	if guid := rt.bagSlotGUID(bag, slot); guid != 0 {
		return state.items[guid]
	}
	return nil
}

// bagSlotGUID is the guid sitting in a bag square, or 0.
func (rt *Runtime) bagSlotGUID(bag, slot int) uint64 {
	state := &rt.inventory
	index := slot - 1
	switch {
	case bag == 0:
		if index < backpackSlots {
			return state.slots[invPackStart+index]
		}
	case bag == -1:
		if index < bankSlots {
			return state.slots[invBankStart+index]
		}
	case bag == -2:
		if index < keyringSlots {
			return state.slots[invKeyStart+index]
		}
	default:
		if container := rt.bagContainer(bag); container != nil && index < len(container.Slots) {
			return container.Slots[index]
		}
	}
	return 0
}

// bagContainer is the bag item equipped in a Lua bag id (1-4 for the carried
// bags, 5-11 for the bank's).
func (rt *Runtime) bagContainer(bag int) *ItemInstance {
	state := &rt.inventory
	switch {
	case bag >= 1 && bag <= 4:
		return state.items[state.slots[invBagStart+bag-1]]
	case bag >= 5 && bag <= 11:
		return state.items[state.slots[invBankBagStart+bag-5]]
	}
	return nil
}

// bagSize is how many squares a Lua bag has.
func (rt *Runtime) bagSize(bag int) int {
	switch {
	case bag == 0:
		return backpackSlots
	case bag == -1:
		return bankSlots
	case bag == -2:
		return keyringSlots
	}
	if container := rt.bagContainer(bag); container != nil {
		return container.NumSlots
	}
	return 0
}

// equippedItem is the item worn in a 1-based Lua inventory slot (1-19 equipment,
// 20-23 carried bags, 68-74 bank bags).
func (rt *Runtime) equippedItem(slot int) *ItemInstance {
	state := &rt.inventory
	index := slot - 1
	if index < 0 || index >= invSlotCount {
		return nil
	}
	return state.items[state.slots[index]]
}

// serverSlotOf maps a Lua inventory slot id to the server's slot number.
func serverSlotOfInventory(slot int) int { return slot - 1 }

// serverBagAndSlot is the (bag, slot) pair CMSG_SWAP_ITEM and friends use for
// a Lua bag square: 255 for the player's own array, or the container's slot.
func (rt *Runtime) serverBagAndSlot(bag, slot int) (uint8, uint8, bool) {
	switch {
	case bag == 0:
		return world.PlayerInventoryBag, uint8(invPackStart + slot - 1), slot >= 1 && slot <= backpackSlots
	case bag == -1:
		return world.PlayerInventoryBag, uint8(invBankStart + slot - 1), slot >= 1 && slot <= bankSlots
	case bag == -2:
		return world.PlayerInventoryBag, uint8(invKeyStart + slot - 1), slot >= 1 && slot <= keyringSlots
	case bag >= 1 && bag <= 4:
		return uint8(invBagStart + bag - 1), uint8(slot - 1), slot >= 1
	case bag >= 5 && bag <= 11:
		return uint8(invBankBagStart + bag - 5), uint8(slot - 1), slot >= 1
	}
	return 0, 0, false
}
