package world

import "encoding/binary"

const (
	// Loot opcodes: the client asks to open a corpse or chest, takes an item or
	// the money, and releases the loot; the server answers with the contents and
	// reports what was taken.
	ClientLoot        ClientOpcode = 0x015D
	ClientLootMoney   ClientOpcode = 0x015E
	ClientLootRelease ClientOpcode = 0x015F

	LootResponse        uint16 = 0x0160
	LootReleaseResponse uint16 = 0x0161
	LootRemoved         uint16 = 0x0162
	LootMoneyNotify     uint16 = 0x0163
	LootClearMoney      uint16 = 0x0165
)

// LootItem is one slot of a loot window.
type LootItem struct {
	Slot           uint8
	Entry, Count   uint32
	DisplayID      uint32
	RandomSuffix   uint32
	RandomProperty uint32
	// SlotType 0 is lootable; others are locked, rolls or master loot.
	SlotType uint8
}

// LootContents is SMSG_LOOT_RESPONSE.
type LootContents struct {
	GUID  uint64
	Kind  uint8
	Money uint32
	Items []LootItem
	// Error is set when the response names why nothing opened (Kind 0 and no body).
	Failed bool
}

// ParseLootResponse reads SMSG_LOOT_RESPONSE.
func ParseLootResponse(body []byte) (LootContents, error) {
	r := NewReader(body, "SMSG_LOOT_RESPONSE")
	var loot LootContents
	var err error
	if loot.GUID, err = r.U64(); err != nil {
		return loot, err
	}
	if loot.Kind, err = r.U8(); err != nil {
		return loot, err
	}
	if loot.Kind == 0 {
		loot.Failed = true
		return loot, nil
	}
	if loot.Money, err = r.U32(); err != nil {
		return loot, err
	}
	count, err := r.U8()
	if err != nil {
		return loot, err
	}
	for index := 0; index < int(count); index++ {
		var item LootItem
		if item.Slot, err = r.U8(); err != nil {
			return loot, err
		}
		if item.Entry, err = r.U32(); err != nil {
			return loot, err
		}
		if item.Count, err = r.U32(); err != nil {
			return loot, err
		}
		if item.DisplayID, err = r.U32(); err != nil {
			return loot, err
		}
		if item.RandomSuffix, err = r.U32(); err != nil {
			return loot, err
		}
		if item.RandomProperty, err = r.U32(); err != nil {
			return loot, err
		}
		if item.SlotType, err = r.U8(); err != nil {
			return loot, err
		}
		loot.Items = append(loot.Items, item)
	}
	return loot, nil
}

// ParseLootRemoved reads the slot SMSG_LOOT_REMOVED reports taken.
func ParseLootRemoved(body []byte) (uint8, error) {
	r := NewReader(body, "SMSG_LOOT_REMOVED")
	return r.U8()
}

// ParseLootReleaseResponse reads the corpse SMSG_LOOT_RELEASE_RESPONSE closes.
func ParseLootReleaseResponse(body []byte) (uint64, error) {
	r := NewReader(body, "SMSG_LOOT_RELEASE_RESPONSE")
	return r.U64()
}

// ParseLootMoneyNotify reads the money SMSG_LOOT_MONEY_NOTIFY reports.
func ParseLootMoneyNotify(body []byte) (uint32, error) {
	r := NewReader(body, "SMSG_LOOT_MONEY_NOTIFY")
	return r.U32()
}

// BuildLootGUID is the body of CMSG_LOOT and CMSG_LOOT_RELEASE.
func BuildLootGUID(guid uint64) []byte {
	body := make([]byte, 8)
	binary.LittleEndian.PutUint64(body, guid)
	return body
}

// BuildLootSlot is the body of CMSG_AUTOSTORE_LOOT_ITEM.
func BuildLootSlot(slot uint8) []byte { return []byte{slot} }
