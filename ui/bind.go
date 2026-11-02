package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// How an item binds and what its flags say.
const (
	bindOnPickup = 1
	bindOnEquip  = 2
	bindOnUse    = 3

	itemInstanceBound       = 0x1   // ITEM_FIELD_FLAGS: the item is soulbound
	itemTemplatePartyLoot   = 0x800 // everyone in the party may loot it
	itemTemplateNoDestroy   = 0x20
	itemTemplateIgnoreSkill = 0x20040000

	lootSlotRoll   = 1
	lootSlotMaster = 2
	lootSlotLocked = 3
	lootSlotOwner  = 4
)

// pendingBind is an action held back until the player confirms that the item
// will bind.
type pendingBind struct {
	run func()
}

type bindState struct {
	lootPending int // the loot window slot waiting for confirmation, 0 when none
	equips      []*pendingBind
	use         func()
	// proficiency is the item subclasses the character may use, by item class.
	proficiency map[uint32]uint32
}

// handleBindPacket takes the packets that tell what the character may use.
func (rt *Runtime) handleBindPacket(packet world.Packet) bool {
	if packet.Opcode != world.SetProficiencyPacket {
		return false
	}
	r := world.NewReader(packet.Body, "SMSG_SET_PROFICIENCY")
	class, err := r.U8()
	if err != nil {
		return true
	}
	mask, err := r.U32()
	if err != nil {
		return true
	}
	if rt.bind.proficiency == nil {
		rt.bind.proficiency = map[uint32]uint32{}
	}
	rt.bind.proficiency[uint32(class)] = mask
	return true
}

func (rt *Runtime) hasSkill(id uint32) bool {
	if _, ok := rt.skillValues[id]; ok {
		return true
	}
	for _, entry := range rt.skills.entries {
		if entry.ID == id {
			return true
		}
	}
	return false
}

// itemBound reports whether an item is already bound to the character.
func (rt *Runtime) itemBound(item *ItemInstance) bool {
	return item.Flags&itemInstanceBound != 0
}

// meetsItemRequirements reports whether the character may use an item: the
// level, class, race, skill and spell it asks for and the kind of arms and
// armour the character may wear.
func (rt *Runtime) meetsItemRequirements(template *world.ItemTemplate) bool {
	player := rt.unitInfo("player")
	if player == nil {
		return false
	}
	if uint32(player.Level) < template.RequiredLevel {
		return false
	}
	if uint32(template.AllowableClass)&(1<<((uint(player.ClassID)-1)&31)) == 0 {
		return false
	}
	if uint32(template.AllowableRace)&(1<<((uint(player.RaceID)-1)&31)) == 0 {
		return false
	}
	if mask := rt.bind.proficiency[template.Class]; template.Class < 0x11 && mask != 0 && mask&(1<<(template.SubClass&31)) == 0 {
		return false
	}
	if template.RequiredSkill != 0 && template.Flags&itemTemplateIgnoreSkill == 0 {
		if !rt.hasSkill(template.RequiredSkill) {
			return false
		}
		if rank, _ := rt.skillRankByID(template.RequiredSkill); uint32(rank) < template.RequiredSkillRank {
			return false
		}
	}
	if template.RequiredSpell != 0 && !rt.gameplay.Known[template.RequiredSpell] {
		return false
	}
	return true
}

func (rt *Runtime) addPendingEquip(run func()) int {
	s := &rt.bind
	for index, pending := range s.equips {
		if pending == nil {
			s.equips[index] = &pendingBind{run: run}
			return index
		}
	}
	s.equips = append(s.equips, &pendingBind{run: run})
	return len(s.equips) - 1
}

// confirmEquip holds an equip back and asks the player when the item would
// bind. It reports whether it did.
func (rt *Runtime) confirmEquip(item *ItemInstance, auto bool, run func()) bool {
	if item == nil {
		return false
	}
	template := rt.itemTemplateIfKnown(item.Entry)
	if template == nil || template.Bonding != bindOnEquip || rt.itemBound(item) || !rt.meetsItemRequirements(template) {
		return false
	}
	index := rt.addPendingEquip(run)
	event := "EQUIP_BIND_CONFIRM"
	if auto {
		event = "AUTOEQUIP_BIND_CONFIRM"
	}
	rt.FireEvent(event, lua.LNumber(index))
	return true
}

// confirmUse holds the use of an item back and asks the player when the item
// would bind.
func (rt *Runtime) confirmUse(item *ItemInstance, run func()) bool {
	template := rt.itemTemplateIfKnown(item.Entry)
	if template == nil || template.Bonding != bindOnUse || rt.itemBound(item) {
		return false
	}
	rt.bind.use = run
	rt.FireEvent("USE_BIND_CONFIRM")
	return true
}

// canCarry reports whether the character may hold more of an item.
func (rt *Runtime) canCarry(template *world.ItemTemplate, count int) bool {
	if template.MaxCount != 0 && rt.itemCountOf(template.Entry)+count > int(template.MaxCount) {
		rt.ShowGameError(0x14)
		return false
	}
	return true
}

// lootSlotAt loots a line of the loot window, asking first when the item binds.
func (rt *Runtime) lootSlotAt(index int, confirmed bool) {
	if index < 1 || index > len(rt.loot.slots) {
		return
	}
	slot := &rt.loot.slots[index-1]
	host := rt.lootHost()
	if slot.cleared || host == nil {
		return
	}
	if slot.coin {
		_ = host.LootMoney()
		return
	}
	switch slot.item.SlotType {
	case lootSlotRoll:
		rt.ShowGameError(0x1BB)
		return
	case lootSlotLocked:
		rt.ShowGameError(0x252)
		return
	case lootSlotMaster:
		rt.FireEvent("OPEN_MASTER_LOOT_LIST")
		return
	}
	template := rt.itemTemplateIfKnown(slot.item.Entry)
	if template == nil {
		return
	}
	if !confirmed && template.Bonding == bindOnPickup && template.Quality > 1 &&
		template.Flags&itemTemplatePartyLoot == 0 && slot.item.SlotType != lootSlotOwner {
		if rt.canCarry(template, int(slot.item.Count)) {
			rt.bind.lootPending = index
			rt.FireEvent("LOOT_BIND_CONFIRM", lua.LNumber(index))
		}
		return
	}
	_ = host.LootItemSlot(slot.item.Slot)
}

// CursorHoldsItem reports whether the cursor carries an item.
func (rt *Runtime) CursorHoldsItem() bool { return rt.inventory.cursor.active }

func (rt *Runtime) cursorItemInstance() *ItemInstance {
	cursor := rt.inventory.cursor
	if !cursor.active {
		return nil
	}
	return rt.inventory.items[cursor.guid]
}

// ConfirmDropCursorItem is the click on the world with an item on the cursor:
// it asks whether to destroy the item.
func (rt *Runtime) ConfirmDropCursorItem() {
	item := rt.cursorItemInstance()
	if item == nil || item.Owner != rt.playerGUID() {
		return
	}
	template := rt.itemTemplateIfKnown(item.Entry)
	if template == nil {
		return
	}
	if template.Flags&itemTemplateNoDestroy != 0 {
		rt.ShowGameError(0x168)
		return
	}
	rt.FireEvent("DELETE_ITEM_CONFIRM", lua.LString(template.Name), lua.LNumber(template.Quality))
}

// DropCursorItemOnPlayer is the click on another player with an item on the
// cursor: it starts a trade, or puts the item into the one that is open.
func (rt *Runtime) DropCursorItemOnPlayer(guid uint64) {
	if rt.cursorItemInstance() == nil || guid == 0 {
		return
	}
	if guid == rt.playerGUID() {
		rt.autoEquipCursorItem()
		return
	}
	if rt.trade.open && rt.trade.partner == guid {
		return
	}
	if !rt.trade.open {
		rt.send(world.ClientInitiateTrade, world.BuildInitiateTrade(guid))
	}
}

// autoEquipCursorItem equips the item on the cursor, asking first when it would bind.
func (rt *Runtime) autoEquipCursorItem() {
	bag, slot, ok := rt.serverBagAndSlotOfCursor()
	if !ok {
		return
	}
	host, hasHost := rt.Host.(WorldItemHost)
	if !hasHost {
		return
	}
	send := func() { _ = host.AutoEquipItem(bag, slot) }
	if !rt.confirmEquip(rt.cursorItemInstance(), true, send) {
		send()
	}
	if rt.clearItemCursor != nil {
		rt.clearItemCursor()
	}
}

func registerBindAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("LootSlot", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: LootSlot(slot)")
		}
		rt.lootSlotAt(L.CheckInt(1), false)
		return 0
	})
	reg("ConfirmLootSlot", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: LootSlot(slot)")
		}
		index := L.CheckInt(1)
		if index != 0 && index == rt.bind.lootPending {
			rt.bind.lootPending = 0
			rt.lootSlotAt(index, true)
		}
		return 0
	})
	pending := func(L *lua.LState) (*pendingBind, int) {
		index := L.CheckInt(1)
		if index < 0 || index >= len(rt.bind.equips) {
			return nil, index
		}
		return rt.bind.equips[index], index
	}
	reg("EquipPendingItem", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: EquipPendingItem(index)")
		}
		if entry, index := pending(L); entry != nil {
			rt.bind.equips[index] = nil
			entry.run()
		}
		return 0
	})
	reg("CancelPendingEquip", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: CancelPendingEquip(index)")
		}
		if entry, index := pending(L); entry != nil {
			rt.bind.equips[index] = nil
		}
		return 0
	})
	reg("ConfirmBindOnUse", func(L *lua.LState) int {
		if run := rt.bind.use; run != nil {
			rt.bind.use = nil
			run()
		}
		return 0
	})
	reg("DropItemOnUnit", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError(`Usage: DropItemOnUnit("unit")`)
		}
		info := rt.unitInfo(L.ToString(1))
		if info != nil && info.Exists && info.Player {
			rt.DropCursorItemOnPlayer(info.GUID)
		}
		return 0
	})
}
