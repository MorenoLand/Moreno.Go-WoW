package render

import (
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Combat log object flags (COMBATLOG_OBJECT_* in Constants.lua).
const (
	combatAffiliationMine     = 0x00000001
	combatAffiliationOutsider = 0x00000008
	combatReactionFriendly    = 0x00000010
	combatReactionNeutral     = 0x00000020
	combatReactionHostile     = 0x00000040
	combatControlPlayer       = 0x00000100
	combatControlNPC          = 0x00000200
	combatTypePlayer          = 0x00000400
	combatTypeNPC             = 0x00000800
)

// worldCombat turns the server's combat messages into interface events and
// the combat log, and keeps what a unit is called.
type worldCombat struct {
	rt       *ui.Runtime
	entities func() map[uint64]*worldEntity
	ownGUID  uint64
	names    *worldNames
	factions func() *worldFactions
	send     func(world.ClientOpcode, []byte) error
	// lastError throttles the repeated range and facing complaints.
	lastError time.Time
}

func (c *worldCombat) unit(guid uint64) ui.CombatUnit {
	unit := ui.CombatUnit{GUID: guid}
	if guid == 0 {
		return unit
	}
	if guid == c.ownGUID {
		unit.Flags = combatAffiliationMine | combatReactionFriendly | combatControlPlayer | combatTypePlayer
		if info := c.rt.UnitGUID("player"); info == guid {
			unit.Name = c.rt.UnitName("player")
		}
		return unit
	}
	entity := c.entities()[guid]
	if entity == nil {
		unit.Flags = combatAffiliationOutsider | combatReactionNeutral | combatControlNPC | combatTypeNPC
		return unit
	}
	c.names.request(entity, c.send)
	var playerFaction uint32
	if player := c.entities()[c.ownGUID]; player != nil {
		playerFaction = player.fields[unitFactionField]
	}
	hostile, friendly := c.factions().reaction(playerFaction, entity.fields[unitFactionField])
	reaction := uint32(combatReactionNeutral)
	switch {
	case hostile:
		reaction = combatReactionHostile
	case friendly:
		reaction = combatReactionFriendly
	}
	if entity.objectType == world.ObjectTypePlayer {
		unit.Flags = combatAffiliationOutsider | reaction | combatControlPlayer | combatTypePlayer
		unit.Name = c.names.players[guid].Name
	} else {
		unit.Flags = combatAffiliationOutsider | reaction | combatControlNPC | combatTypeNPC
		unit.Name = c.names.creatures[entity.fields[objectEntryField]].Name
	}
	return unit
}

func (c *worldCombat) complain(global string) {
	if time.Since(c.lastError) < time.Second {
		return
	}
	c.lastError = time.Now()
	c.rt.ShowGlobalError(global)
}

func flagValue(set bool) lua.LValue {
	if set {
		return lua.LNumber(1)
	}
	return lua.LNil
}

// apply handles a combat or name packet and reports whether it was one.
func (c *worldCombat) apply(packet world.Packet) (bool, error) {
	switch packet.Opcode {
	case world.NameQueryResponse:
		name, err := world.ParseNameQuery(packet.Body)
		if err == nil {
			c.names.players[name.GUID] = name
		}
		return true, nil
	case world.GuildQueryResponse:
		if id, name, err := world.ParseGuildQueryName(packet.Body); err == nil && name != "" {
			c.names.guilds[id] = name
		}
		return true, nil
	case world.CreatureQueryResponse:
		info, err := world.ParseCreatureQuery(packet.Body)
		if err != nil {
			return true, err
		}
		if info.Found {
			c.names.creatures[info.Entry] = info
		}
		return true, nil
	case world.AttackStartPacket:
		attacker, _, err := world.ParseAttackStart(packet.Body)
		if err == nil && attacker == c.ownGUID {
			c.rt.SetAttacking(true)
		}
		return true, err
	case world.AttackStopPacket:
		attacker, _, _, err := world.ParseAttackStop(packet.Body)
		if err == nil && attacker == c.ownGUID {
			c.rt.SetAttacking(false)
		}
		return true, err
	case world.AttackNotInRange:
		c.complain("ERR_BADATTACKPOS")
		return true, nil
	case world.AttackBadFacing:
		c.complain("ERR_BADATTACKFACING")
		return true, nil
	case world.AttackNotStanding:
		c.complain("SPELL_FAILED_NOT_STANDING")
		return true, nil
	case world.AttackDeadTarget, world.AttackCantAttack:
		c.complain("ERR_INVALID_ATTACK_TARGET")
		return true, nil
	case world.AttackerStateUpdate:
		hit, err := world.ParseAttackerStateUpdate(packet.Body)
		if err != nil {
			return true, err
		}
		c.meleeEvent(hit)
		return true, nil
	case world.SpellNonMeleeDamageLog:
		damage, err := world.ParseSpellDamageLog(packet.Body)
		if err != nil {
			return true, err
		}
		overkill := lua.LValue(lua.LNumber(0))
		if damage.Overkill > 0 {
			overkill = lua.LNumber(damage.Overkill)
		}
		school := lua.LNumber(damage.School)
		c.rt.CombatEvent("SPELL_DAMAGE", c.unit(damage.Caster), c.unit(damage.Victim),
			lua.LNumber(damage.SpellID), lua.LString(c.rt.SpellName(damage.SpellID)), school,
			lua.LNumber(damage.Damage), overkill, school, lua.LNumber(damage.Resisted), lua.LNumber(damage.Blocked), lua.LNumber(damage.Absorbed),
			flagValue(damage.Flags&0x02 != 0), lua.LNil, lua.LNil)
		return true, nil
	case world.SpellHealLog:
		heal, err := world.ParseSpellHealLog(packet.Body)
		if err != nil {
			return true, err
		}
		c.rt.CombatEvent("SPELL_HEAL", c.unit(heal.Caster), c.unit(heal.Victim),
			lua.LNumber(heal.SpellID), lua.LString(c.rt.SpellName(heal.SpellID)), lua.LNumber(2),
			lua.LNumber(heal.Amount), flagValue(heal.Critical))
		return true, nil
	}
	return false, nil
}

func (c *worldCombat) meleeEvent(hit world.MeleeHit) {
	source, dest := c.unit(hit.Attacker), c.unit(hit.Victim)
	missType := ""
	switch {
	case hit.HitInfo&world.HitInfoMiss != 0:
		missType = "MISS"
	case hit.VictimState == world.VictimStateDodge:
		missType = "DODGE"
	case hit.VictimState == world.VictimStateParry:
		missType = "PARRY"
	case hit.VictimState == world.VictimStateEvades:
		missType = "EVADE"
	case hit.VictimState == world.VictimStateImmune:
		missType = "IMMUNE"
	case hit.VictimState == world.VictimStateDeflect:
		missType = "DEFLECT"
	case hit.VictimState == world.VictimStateBlocks && hit.Damage == 0:
		missType = "BLOCK"
	case hit.Damage == 0 && hit.Absorbed > 0:
		missType = "ABSORB"
	}
	if missType != "" {
		c.rt.CombatEvent("SWING_MISSED", source, dest, lua.LString(missType))
		return
	}
	overkill := lua.LValue(lua.LNumber(0))
	if hit.Overkill > 0 {
		overkill = lua.LNumber(hit.Overkill)
	}
	school := hit.School
	if school == 0 {
		school = 1
	}
	c.rt.CombatEvent("SWING_DAMAGE", source, dest,
		lua.LNumber(hit.Damage), overkill, lua.LNumber(school), lua.LNumber(hit.Resisted), lua.LNumber(hit.BlockedValue), lua.LNumber(hit.Absorbed),
		flagValue(hit.HitInfo&world.HitInfoCritical != 0), flagValue(hit.HitInfo&world.HitInfoGlancing != 0), flagValue(hit.HitInfo&world.HitInfoCrushing != 0))
}
