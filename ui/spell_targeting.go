package ui

import lua "github.com/yuin/gopher-lua"

// The ground target of a spell: a spell that lands on a place (a Blizzard, a
// Flamestrike) waits for a click on the world before it is cast.
const (
	targetDestDynObjEnemy = 28
	targetDestDynObjAlly  = 29
)

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

func registerSpellTargetingAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("SpellIsTargeting", func(L *lua.LState) int { L.Push(lua.LBool(rt.targetingSpell != 0)); return 1 })
	reg("SpellStopTargeting", func(L *lua.LState) int { rt.StopTargeting(); return 0 })
}
