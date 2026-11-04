package ui

import lua "github.com/yuin/gopher-lua"

// The ground target of a spell: a spell that lands on a place (a Blizzard, a
// Flamestrike) waits for a click on the world before it is cast.
const (
	targetDestDynObjEnemy = 28
	targetDestDynObjAlly  = 29
)

// What a spell waits to be aimed at.
const (
	targetingGround = iota
	targetingItem
	targetingUnit
	// TargetingUnit and TargetingGround are the kinds the world click tells apart.
	TargetingUnit   = targetingUnit
	TargetingGround = targetingGround
)

// Spell target mask bits that say what a spell wants.
const (
	spellTargetUnit = 0x0002
	spellTargetItem = 0x0010
)

// WorldItemSpellHost casts a spell on an item.
type WorldItemSpellHost interface {
	CastSpellOnItem(id uint32, item uint64) error
}

// WorldGroundSpellHost casts a spell at a place.
type WorldGroundSpellHost interface {
	CastSpellAt(id uint32, x, y, z float32) error
}

// spellNeedsGround reports whether a spell is aimed at a place the player
// chooses.
func spellNeedsGround(spell SpellInfo) bool {
	for _, effect := range spell.Effects {
		for _, target := range [2]int{effect.TargetA, effect.TargetB} {
			if target == targetDestDynObjEnemy || target == targetDestDynObjAlly {
				return true
			}
		}
	}
	return false
}

// Targeting is the spell waiting for a click on the ground, or 0.
func (rt *Runtime) Targeting() uint32 { return rt.targetingSpell }

// StopTargeting ends the wait for a click.
func (rt *Runtime) StopTargeting() {
	if rt.targetingSpell != 0 {
		rt.targetingSpell = 0
		rt.FireEvent("CURSOR_UPDATE")
	}
}

// TargetGround casts the waiting spell at a place.
func (rt *Runtime) TargetGround(x, y, z float32) error {
	id := rt.targetingSpell
	if id == 0 {
		return nil
	}
	rt.targetingSpell = 0
	rt.FireEvent("CURSOR_UPDATE")
	if host, ok := rt.Host.(WorldGroundSpellHost); ok {
		return host.CastSpellAt(id, x, y, z)
	}
	return nil
}

// TargetingKind is what the waiting spell is aimed at.
func (rt *Runtime) TargetingKind() int { return rt.targetingKind }

// TargetUnit casts the waiting spell at a unit.
func (rt *Runtime) TargetUnit(guid uint64) error {
	id := rt.targetingSpell
	if id == 0 || guid == 0 {
		return nil
	}
	rt.targetingSpell = 0
	rt.FireEvent("CURSOR_UPDATE")
	if host, ok := rt.Host.(WorldSpellHost); ok {
		return host.CastSpell(id, guid)
	}
	return nil
}

// targetItem casts the waiting spell on an item.
func (rt *Runtime) targetItem(item *ItemInstance) bool {
	id := rt.targetingSpell
	if id == 0 || rt.targetingKind != targetingItem || item == nil {
		return false
	}
	rt.targetingSpell = 0
	rt.FireEvent("CURSOR_UPDATE")
	rt.pendingEnchant = &pendingEnchant{spell: id, item: item}
	rt.confirmEnchant(true)
	return true
}

func registerSpellTargetingAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("SpellTargetUnit", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError("Usage: SpellTargetUnit(\"unit\")")
			return 0
		}
		if rt.targetingSpell == 0 {
			return 0
		}
		if info := rt.unitByNameOrToken(L.CheckString(1)); info != nil && info.GUID != 0 {
			_ = rt.TargetUnit(info.GUID)
		}
		return 0
	})
	reg("SpellTargetItem", func(L *lua.LState) int {
		if rt.targetingSpell == 0 || rt.targetingKind != targetingItem {
			return 0
		}
		if value := L.Get(1); value.Type() != lua.LTNumber && value.Type() != lua.LTString {
			L.RaiseError("Usage: SpellTargetItem(itemID|\"name\"|\"itemlink\")")
			return 0
		}
		entry := rt.itemArgEntry(L, 1)
		for _, item := range rt.inventory.items {
			if item.Entry == entry && rt.itemIsCarried(item) {
				rt.targetItem(item)
				return 0
			}
		}
		return 0
	})
	reg("SpellIsTargeting", func(L *lua.LState) int { L.Push(lua.LBool(rt.targetingSpell != 0)); return 1 })
	reg("SpellStopTargeting", func(L *lua.LState) int { rt.StopTargeting(); return 0 })
}
