package render

import (
	"fmt"

	"github.com/MorenoLand/Moreno.WoW/network"
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// Update-field indexes of the 3.3.5a player and item objects.
const (
	fieldInvSlotHead    = 324 // PLAYER_FIELD_INV_SLOT_HEAD, two fields per slot
	fieldCoinage        = 1170
	fieldExploredZones  = 1041
	exploredZoneWords   = 128
	fieldItemStackCount = 14
	fieldItemFlags      = 21
	fieldItemEnchant    = 22
	fieldItemDurability = 60
	fieldItemMaxDura    = 61
	fieldItemOwner      = 6
	fieldItemContained  = 8
	fieldContainerSlots = 64 // CONTAINER_FIELD_NUM_SLOTS
	fieldContainerSlot1 = 66
	fieldObjectEntry    = 3
	inventorySlotCount  = 150
)

// readWorldItemCatalog reads what the item tooltips and icons need from the
// client's own tables: the display id of every entry (so an icon can be drawn
// before the server answers an item query), the icon of every display id and
// the localized class names.
func readWorldItemCatalog(loader *ui.Loader) (map[uint32]ui.ItemLocal, map[uint32]string, ui.ItemClassNames, error) {
	local := make(map[uint32]ui.ItemLocal)
	icons := make(map[uint32]string)
	classes := ui.ItemClassNames{Class: make(map[uint32]string), SubClass: make(map[[2]uint32]string)}
	items, err := loadCharacterDBC(loader, `DBFilesClient\Item.dbc`)
	if err != nil {
		return nil, nil, classes, err
	}
	for row := 0; row < items.records; row++ {
		local[items.value(row, 0)] = ui.ItemLocal{
			DisplayID:     items.value(row, 5),
			Class:         items.value(row, 1),
			SubClass:      items.value(row, 2),
			InventoryType: items.value(row, 6),
		}
	}
	display, err := loadCharacterDBC(loader, `DBFilesClient\ItemDisplayInfo.dbc`)
	if err != nil {
		return nil, nil, classes, err
	}
	for row := 0; row < display.records; row++ {
		if icon := display.string(row, 5); icon != "" {
			icons[display.value(row, 0)] = icon
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\ItemClass.dbc`); err == nil && table.fields >= 4 {
		for row := 0; row < table.records; row++ {
			classes.Class[table.value(row, 0)] = table.string(row, 3)
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\ItemSubClass.dbc`); err == nil && table.fields >= 11 {
		for row := 0; row < table.records; row++ {
			classes.SubClass[[2]uint32{table.value(row, 0), table.value(row, 1)}] = table.string(row, 10)
		}
	}
	return local, icons, classes, nil
}

func fieldGUID(fields map[uint16]uint32, field uint16) uint64 {
	return uint64(fields[field]) | uint64(fields[field+1])<<32
}

// syncWorldInventory rebuilds the inventory the UI shows from the player's
// update fields and the item and container objects the server has sent.
func syncWorldInventory(rt *ui.Runtime, entities map[uint64]*worldEntity, player *worldEntity) {
	if player == nil {
		return
	}
	var slots [inventorySlotCount]uint64
	for slot := range slots {
		slots[slot] = fieldGUID(player.fields, uint16(fieldInvSlotHead+2*slot))
	}
	items := make(map[uint64]*ui.ItemInstance)
	for guid, entity := range entities {
		if entity.objectType != world.ObjectTypeItem && entity.objectType != world.ObjectTypeContainer {
			continue
		}
		item := &ui.ItemInstance{
			GUID:          guid,
			Entry:         entity.fields[fieldObjectEntry],
			Count:         int(entity.fields[fieldItemStackCount]),
			Durability:    int(entity.fields[fieldItemDurability]),
			MaxDurability: int(entity.fields[fieldItemMaxDura]),
			Contained:     fieldGUID(entity.fields, fieldItemContained),
			Owner:         fieldGUID(entity.fields, fieldItemOwner),
			Flags:         entity.fields[fieldItemFlags],
			Enchant:       entity.fields[fieldItemEnchant],
		}
		if item.Count == 0 {
			item.Count = 1 // a sparse field set omits zeros, and there is no stack of none
		}
		if entity.objectType == world.ObjectTypeContainer {
			item.NumSlots = int(entity.fields[fieldContainerSlots])
			item.Slots = make([]uint64, item.NumSlots)
			for index := range item.Slots {
				item.Slots[index] = fieldGUID(entity.fields, uint16(fieldContainerSlot1+2*index))
			}
		}
		items[guid] = item
	}
	var explored [exploredZoneWords]uint32
	for word := range explored {
		explored[word] = player.fields[uint16(fieldExploredZones+word)]
	}
	rt.SetExploredZones(explored[:])
	rt.SetInventory(slots, items, player.fields[fieldCoinage])
}

func (h *clientHost) worldSession() (*network.Session, error) {
	if h.session == nil {
		return nil, fmt.Errorf("world session is closed")
	}
	session := h.session()
	if session == nil {
		return nil, fmt.Errorf("world session is closed")
	}
	return session, nil
}

func (h *clientHost) QueryItem(entry uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.QueryItem(entry)
}

func (h *clientHost) SwapItem(dstBag, dstSlot, srcBag, srcSlot uint8) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SwapItem(dstBag, dstSlot, srcBag, srcSlot)
}

func (h *clientHost) AutoEquipItem(bag, slot uint8) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.AutoEquipItem(bag, slot)
}

func (h *clientHost) AutoStoreBagItem(srcBag, srcSlot, dstBag uint8) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.AutoStoreBagItem(srcBag, srcSlot, dstBag)
}

func (h *clientHost) SplitItem(srcBag, srcSlot, dstBag, dstSlot, count uint8) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SplitItem(srcBag, srcSlot, dstBag, dstSlot, count)
}

func (h *clientHost) DestroyItem(bag, slot, count uint8) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.DestroyItem(bag, slot, count)
}

func (h *clientHost) UseItem(bag, slot, castCount uint8, spellID uint32, itemGUID uint64, targetMask uint32, targetGUID uint64) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.UseItem(bag, slot, castCount, spellID, itemGUID, targetMask, targetGUID)
}
