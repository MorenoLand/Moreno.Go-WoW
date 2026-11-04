package render

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The floating combat text: what happens to the unit the interface follows
// (the character, or its vehicle) goes to COMBAT_TEXT_UPDATE.

// combatTextFor reports whether something that happens to a unit is shown by
// the combat text.
func (c *worldCombat) combatTextFor(guid uint64) bool {
	if c.rt == nil || guid == 0 {
		return false
	}
	active := c.rt.CombatTextUnit()
	if active == 0 {
		active = c.ownGUID
	}
	return guid == active
}

func (c *worldCombat) combatText(guid uint64, kind string, args ...lua.LValue) {
	if !c.combatTextFor(guid) {
		return
	}
	c.rt.FireEvent("COMBAT_TEXT_UPDATE", append([]lua.LValue{lua.LString(kind)}, args...)...)
}

// textMitigated is the kind of message for damage a unit took in part, or
// that nothing reached it: the damage itself, or what was resisted, absorbed
// or blocked of it.
func (c *worldCombat) textDamage(prefix string, victim uint64, amount, resisted, absorbed, blocked uint32, critical bool) {
	switch {
	case resisted > 0:
		c.combatText(victim, prefix+"RESIST", lua.LNumber(amount), lua.LNumber(resisted))
	case absorbed > 0:
		c.combatText(victim, prefix+"ABSORB", lua.LNumber(amount), lua.LNumber(absorbed))
	case blocked > 0 && prefix == "":
		c.combatText(victim, "BLOCK", lua.LNumber(amount), lua.LNumber(blocked))
	case blocked > 0:
		c.combatText(victim, "SPELL_BLOCK", lua.LNumber(amount), lua.LNumber(blocked))
	case critical:
		c.combatText(victim, prefix+"DAMAGE_CRIT", lua.LNumber(amount))
	default:
		c.combatText(victim, prefix+"DAMAGE", lua.LNumber(amount))
	}
}

// textMiss is the message for an attack that did not land.
func (c *worldCombat) textMiss(prefix string, victim uint64, miss string) {
	switch miss {
	case "MISS", "DODGE", "PARRY", "EVADE", "IMMUNE", "DEFLECT", "REFLECT":
		c.combatText(victim, prefix+miss)
	case "BLOCK", "ABSORB", "RESIST":
		c.combatText(victim, prefix+miss, lua.LNumber(0))
	}
}

func (c *worldCombat) textHeal(victim uint64, caster uint64, amount uint32, critical, periodic bool) {
	name := lua.LValue(lua.LString(c.unit(caster).Name))
	switch {
	case periodic:
		c.combatText(victim, "PERIODIC_HEAL", name, lua.LNumber(amount))
	case critical:
		c.combatText(victim, "HEAL_CRIT", name, lua.LNumber(amount))
	default:
		c.combatText(victim, "HEAL", name, lua.LNumber(amount))
	}
}

var powerTokens = [...]string{"MANA", "RAGE", "FOCUS", "ENERGY", "HAPPINESS", "RUNES", "RUNIC_POWER"}

func (c *worldCombat) textEnergize(victim uint64, amount int32, power uint32, periodic bool) {
	token := "MANA"
	if int(power) < len(powerTokens) {
		token = powerTokens[power]
	}
	kind := "ENERGIZE"
	if periodic {
		kind = "PERIODIC_ENERGIZE"
	}
	c.combatText(victim, kind, lua.LNumber(amount), lua.LString(token))
}

// textAura is the message for an aura the unit gained or lost.
func (c *worldCombat) textAura(guid uint64, slot world.AuraSlot, gained bool) {
	kind := "SPELL_AURA_START"
	if !gained {
		kind = "SPELL_AURA_END"
	}
	if slot.Flags&0x80 != 0 {
		kind += "_HARMFUL"
	}
	c.combatText(guid, kind, lua.LString(c.rt.SpellName(slot.SpellID)))
}
