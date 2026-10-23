package world

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	ItemQuerySingleOpcode       ClientOpcode = 0x0056
	ItemQuerySingleResponse     uint16       = 0x0058
	AutoStoreLootItemOpcode     ClientOpcode = 0x0108
	AutoEquipItemOpcode         ClientOpcode = 0x010A
	AutoStoreBagItemOpcode      ClientOpcode = 0x010B
	SwapItemOpcode              ClientOpcode = 0x010C
	SwapInvItemOpcode           ClientOpcode = 0x010D
	SplitItemOpcode             ClientOpcode = 0x010E
	DestroyItemOpcode           ClientOpcode = 0x0111
	UseItemOpcode               ClientOpcode = 0x00AB
	InventoryChangeFailure      uint16       = 0x0112
	ItemPushResult              uint16       = 0x0166
	ItemCooldownResponse        uint16       = 0x00B0
	PlayerInventoryBag          uint8        = 0xFF // bag id of the player's own slot array
	itemNames                                = 4
	itemDamages                              = 2
	itemResistances                          = 6
	itemSpells                               = 5
	itemSpellFields                          = 6
	itemSockets                              = 3
	itemQueryEntryNotFound      uint32       = 0x80000000
	ItemSpellTriggerOnUse       uint32       = 0
	ItemSpellTriggerOnEquip     uint32       = 1
	ItemSpellTriggerChanceOnHit uint32       = 2
)

// ItemStat is one stat bonus of an item.
type ItemStat struct {
	Type  uint32
	Value int32
}

// ItemSpell is one of an item's five spell slots.
type ItemSpell struct {
	ID, Trigger                          uint32
	Charges                              int32
	Cooldown, Category, CategoryCooldown int32
}

// ItemSocket is one gem socket of an item.
type ItemSocket struct{ Color, Content uint32 }

// ItemDamage is one damage range of a weapon.
type ItemDamage struct {
	Min, Max float32
	Type     uint32
}

// ItemTemplate is what SMSG_ITEM_QUERY_SINGLE_RESPONSE says about an item
// entry.
type ItemTemplate struct {
	Entry                                uint32
	NotFound                             bool
	Class, SubClass                      uint32
	Name                                 string
	DisplayID, Quality                   uint32
	Flags, Flags2                        uint32
	BuyPrice                             int32
	SellPrice                            uint32
	InventoryType                        uint32
	AllowableClass, AllowableRace        int32
	ItemLevel, RequiredLevel             uint32
	RequiredSkill, RequiredSkillRank     uint32
	RequiredSpell                        uint32
	RequiredHonorRank, RequiredCityRank  uint32
	RequiredFaction, RequiredFactionRank uint32
	MaxCount, Stackable                  int32
	ContainerSlots                       uint32
	Stats                                []ItemStat
	ScalingStatDistribution              uint32
	ScalingStatValue                     uint32
	Damage                               [itemDamages]ItemDamage
	Armor                                uint32
	Resistances                          [itemResistances]int32 // holy, fire, nature, frost, shadow, arcane
	Delay                                uint32
	AmmoType                             uint32
	RangedModRange                       float32
	Spells                               [itemSpells]ItemSpell
	Bonding                              uint32
	Description                          string
	PageText, LanguageID, PageMaterial   uint32
	StartQuest, LockID                   uint32
	Material                             int32
	Sheath                               uint32
	RandomProperty, RandomSuffix         int32
	Block, ItemSet, MaxDurability        uint32
	Area, Map, BagFamily, TotemCategory  uint32
	Sockets                              [itemSockets]ItemSocket
	SocketBonus, GemProperties           uint32
	RequiredDisenchantSkill              int32
	ArmorDamageModifier                  float32
	Duration                             uint32
	ItemLimitCategory                    int32
	HolidayID                            uint32
}

// BuildItemQuery is the body of CMSG_ITEM_QUERY_SINGLE: the entry, then the
// guid of an item being held (zero when asking about an entry only). The
// server reads only the entry.
func BuildItemQuery(entry uint32, guid uint64) []byte {
	body := make([]byte, 12)
	binary.LittleEndian.PutUint32(body, entry)
	binary.LittleEndian.PutUint64(body[4:], guid)
	return body
}

// ParseItemQueryResponse reads SMSG_ITEM_QUERY_SINGLE_RESPONSE.
func ParseItemQueryResponse(body []byte) (ItemTemplate, error) {
	r := NewReader(body, "SMSG_ITEM_QUERY_SINGLE_RESPONSE")
	var item ItemTemplate
	entry, err := r.U32()
	if err != nil {
		return item, err
	}
	if entry&itemQueryEntryNotFound != 0 {
		item.Entry, item.NotFound = entry&^itemQueryEntryNotFound, true
		return item, nil
	}
	item.Entry = entry
	u32 := func(target *uint32) {
		if err == nil {
			*target, err = r.U32()
		}
	}
	i32 := func(target *int32) {
		if err == nil {
			var value uint32
			value, err = r.U32()
			*target = int32(value)
		}
	}
	f32 := func(target *float32) {
		if err == nil {
			*target, err = r.F32()
		}
	}
	skip := func(count int) {
		for range count {
			var ignored uint32
			u32(&ignored)
		}
	}
	u32(&item.Class)
	u32(&item.SubClass)
	skip(1) // sound override subclass
	for index := 0; index < itemNames && err == nil; index++ {
		var name string
		name, err = r.CString()
		if index == 0 {
			item.Name = name
		}
	}
	u32(&item.DisplayID)
	u32(&item.Quality)
	u32(&item.Flags)
	u32(&item.Flags2)
	i32(&item.BuyPrice)
	u32(&item.SellPrice)
	u32(&item.InventoryType)
	i32(&item.AllowableClass)
	i32(&item.AllowableRace)
	u32(&item.ItemLevel)
	u32(&item.RequiredLevel)
	u32(&item.RequiredSkill)
	u32(&item.RequiredSkillRank)
	u32(&item.RequiredSpell)
	u32(&item.RequiredHonorRank)
	u32(&item.RequiredCityRank)
	u32(&item.RequiredFaction)
	u32(&item.RequiredFactionRank)
	i32(&item.MaxCount)
	i32(&item.Stackable)
	u32(&item.ContainerSlots)
	var statCount uint32
	u32(&statCount)
	if err == nil && statCount > 10 {
		return item, fmt.Errorf("item %d reports %d stats", entry, statCount)
	}
	for index := uint32(0); index < statCount && err == nil; index++ {
		var stat ItemStat
		u32(&stat.Type)
		i32(&stat.Value)
		item.Stats = append(item.Stats, stat)
	}
	u32(&item.ScalingStatDistribution)
	u32(&item.ScalingStatValue)
	for index := range item.Damage {
		f32(&item.Damage[index].Min)
		f32(&item.Damage[index].Max)
		u32(&item.Damage[index].Type)
	}
	u32(&item.Armor)
	for index := range item.Resistances {
		i32(&item.Resistances[index])
	}
	u32(&item.Delay)
	u32(&item.AmmoType)
	f32(&item.RangedModRange)
	for index := range item.Spells {
		spell := &item.Spells[index]
		u32(&spell.ID)
		u32(&spell.Trigger)
		i32(&spell.Charges)
		i32(&spell.Cooldown)
		i32(&spell.Category)
		i32(&spell.CategoryCooldown)
	}
	u32(&item.Bonding)
	if err == nil {
		item.Description, err = r.CString()
	}
	u32(&item.PageText)
	u32(&item.LanguageID)
	u32(&item.PageMaterial)
	u32(&item.StartQuest)
	u32(&item.LockID)
	i32(&item.Material)
	u32(&item.Sheath)
	i32(&item.RandomProperty)
	i32(&item.RandomSuffix)
	u32(&item.Block)
	u32(&item.ItemSet)
	u32(&item.MaxDurability)
	u32(&item.Area)
	u32(&item.Map)
	u32(&item.BagFamily)
	u32(&item.TotemCategory)
	for index := range item.Sockets {
		u32(&item.Sockets[index].Color)
		u32(&item.Sockets[index].Content)
	}
	u32(&item.SocketBonus)
	u32(&item.GemProperties)
	i32(&item.RequiredDisenchantSkill)
	f32(&item.ArmorDamageModifier)
	u32(&item.Duration)
	i32(&item.ItemLimitCategory)
	u32(&item.HolidayID)
	if err != nil {
		return item, err
	}
	return item, r.Finish()
}

// The wire sends the armor and damage values the tooltip shows.
var _ = math.Float32bits

func (c *Connection) QueryItem(entry uint32, guid uint64) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	return c.send(ItemQuerySingleOpcode, BuildItemQuery(entry, guid))
}

// SwapItem moves an item between two named slots: CMSG_SWAP_ITEM
// {dstBag, dstSlot, srcBag, srcSlot}. A bag of 255 is the player's own slot
// array.
func (c *Connection) SwapItem(dstBag, dstSlot, srcBag, srcSlot uint8) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	return c.send(SwapItemOpcode, []byte{dstBag, dstSlot, srcBag, srcSlot})
}

// AutoEquipItem equips the item at (bag, slot).
func (c *Connection) AutoEquipItem(bag, slot uint8) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	return c.send(AutoEquipItemOpcode, []byte{bag, slot})
}

// AutoStoreBagItem stores the item at (srcBag, srcSlot) in a free slot of dstBag.
func (c *Connection) AutoStoreBagItem(srcBag, srcSlot, dstBag uint8) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	return c.send(AutoStoreBagItemOpcode, []byte{srcBag, srcSlot, dstBag})
}

// SplitItem moves count items of the stack at (srcBag, srcSlot) to (dstBag, dstSlot).
func (c *Connection) SplitItem(srcBag, srcSlot, dstBag, dstSlot, count uint8) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	return c.send(SplitItemOpcode, []byte{srcBag, srcSlot, dstBag, dstSlot, count})
}

// DestroyItem destroys count items of the stack at (bag, slot); zero is the
// whole stack.
func (c *Connection) DestroyItem(bag, slot, count uint8) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	return c.send(DestroyItemOpcode, []byte{bag, slot, count, 0, 0, 0})
}

// BuildUseItem is the body of CMSG_USE_ITEM for an item used on nothing in
// particular (or on the caster).
func BuildUseItem(bag, slot, castCount uint8, spellID uint32, itemGUID uint64, targetMask uint32, targetGUID uint64) []byte {
	body := make([]byte, 0, 32)
	body = append(body, bag, slot, castCount)
	body = binary.LittleEndian.AppendUint32(body, spellID)
	body = binary.LittleEndian.AppendUint64(body, itemGUID)
	body = binary.LittleEndian.AppendUint32(body, 0) // glyph slot
	body = append(body, 0)                           // cast flags
	body = binary.LittleEndian.AppendUint32(body, targetMask)
	if targetMask&0x2 != 0 || targetMask&0x800 != 0 { // unit or item target: packed guid
		body = appendPackedGUID(body, targetGUID)
	}
	return body
}

func appendPackedGUID(body []byte, guid uint64) []byte {
	mask := byte(0)
	var bytes []byte
	for index := 0; index < 8; index++ {
		value := byte(guid >> (8 * index))
		if value != 0 {
			mask |= 1 << index
			bytes = append(bytes, value)
		}
	}
	return append(append(body, mask), bytes...)
}

// UseItem uses the item at (bag, slot).
func (c *Connection) UseItem(bag, slot, castCount uint8, spellID uint32, itemGUID uint64, targetMask uint32, targetGUID uint64) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("world connection is closed")
	}
	return c.send(UseItemOpcode, BuildUseItem(bag, slot, castCount, spellID, itemGUID, targetMask, targetGUID))
}

// ParseInventoryChangeFailure reads SMSG_INVENTORY_CHANGE_FAILURE: the error
// code, then (for most codes) the guids involved.
func ParseInventoryChangeFailure(body []byte) (uint8, error) {
	if len(body) < 1 {
		return 0, fmt.Errorf("SMSG_INVENTORY_CHANGE_FAILURE is empty")
	}
	return body[0], nil
}
