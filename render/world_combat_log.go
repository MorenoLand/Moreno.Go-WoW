package render

import (
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

var environmentalNames = [...]string{"Fatigue", "Drowning", "Falling", "Lava", "Slime", "Fire"}

// spellArgs are the three fields every spell event starts with.
func (c *worldCombat) spellArgs(id uint32) []lua.LValue {
	return []lua.LValue{lua.LNumber(id), lua.LString(c.rt.SpellName(id)), lua.LNumber(c.rt.SpellSchool(id))}
}

func (c *worldCombat) spellEvent(subevent string, source, dest ui.CombatUnit, id uint32, extra ...lua.LValue) {
	c.rt.CombatEvent(subevent, source, dest, append(c.spellArgs(id), extra...)...)
}

// applyLog handles the combat messages besides damage and healing.
func (c *worldCombat) applyLog(packet world.Packet) (bool, error) {
	switch packet.Opcode {
	case world.SpellLogMissPacket:
		misses, err := world.ParseSpellLogMiss(packet.Body)
		for _, miss := range misses {
			name := "MISS"
			if int(miss.Miss) < len(world.SpellMissNames) && world.SpellMissNames[miss.Miss] != "" {
				name = world.SpellMissNames[miss.Miss]
			}
			c.spellEvent("SPELL_MISSED", c.unit(miss.Caster), c.unit(miss.Target), miss.SpellID, lua.LString(name))
		}
		return true, err
	case world.SpellEnergizeLogPacket:
		energize, err := world.ParseSpellEnergizeLog(packet.Body)
		if err == nil {
			c.spellEvent("SPELL_ENERGIZE", c.unit(energize.Caster), c.unit(energize.Victim), energize.SpellID,
				lua.LNumber(energize.Amount), lua.LNumber(energize.PowerType))
		}
		return true, err
	case world.PeriodicAuraLogPacket:
		ticks, err := world.ParsePeriodicAuraLog(packet.Body)
		for _, tick := range ticks {
			source, dest := c.unit(tick.Caster), c.unit(tick.Target)
			switch tick.AuraType {
			case 3, 89:
				c.spellEvent("SPELL_PERIODIC_DAMAGE", source, dest, tick.SpellID, lua.LNumber(tick.Amount), lua.LNumber(tick.Overkill),
					lua.LNumber(tick.School), lua.LNumber(tick.Resisted), lua.LNumber(0), lua.LNumber(tick.Absorbed),
					flagValue(tick.Critical), lua.LNil, lua.LNil)
			case 8, 20:
				c.spellEvent("SPELL_PERIODIC_HEAL", source, dest, tick.SpellID, lua.LNumber(tick.Amount), lua.LNumber(tick.Overkill),
					lua.LNumber(tick.Absorbed), flagValue(tick.Critical))
			case 21, 24:
				c.spellEvent("SPELL_PERIODIC_ENERGIZE", source, dest, tick.SpellID, lua.LNumber(tick.Amount), lua.LNumber(tick.PowerType))
			case 64:
				c.spellEvent("SPELL_PERIODIC_LEECH", source, dest, tick.SpellID, lua.LNumber(tick.Amount), lua.LNumber(tick.PowerType), lua.LNumber(0))
			}
		}
		return true, err
	case world.EnvironmentalDamagePacket:
		damage, err := world.ParseEnvironmentalDamage(packet.Body)
		if err == nil {
			name := "Fire"
			if int(damage.Type) < len(environmentalNames) {
				name = environmentalNames[damage.Type]
			}
			c.rt.CombatEvent("ENVIRONMENTAL_DAMAGE", ui.CombatUnit{}, c.unit(damage.Victim), lua.LString(name),
				lua.LNumber(damage.Amount), lua.LNumber(0), lua.LNumber(1), lua.LNumber(damage.Resisted), lua.LNumber(0),
				lua.LNumber(damage.Absorbed), lua.LNil, lua.LNil, lua.LNil)
		}
		return true, err
	case world.SpellDispelLogPacket, world.SpellStealLogPacket:
		stolen := packet.Opcode == world.SpellStealLogPacket
		dispel, err := world.ParseSpellDispelLog(packet.Body, stolen)
		for _, removed := range dispel.Removed {
			subevent, kind := "SPELL_DISPEL", "DEBUFF"
			if stolen {
				subevent, kind = "SPELL_STOLEN", "BUFF"
			}
			c.spellEvent(subevent, c.unit(dispel.Caster), c.unit(dispel.Victim), dispel.SpellID,
				lua.LNumber(removed), lua.LString(c.rt.SpellName(removed)), lua.LNumber(c.rt.SpellSchool(removed)), lua.LString(kind))
		}
		return true, err
	case world.SpellInstakillLogPacket:
		pair, err := world.ParseSpellPair(packet.Body)
		if err == nil {
			c.spellEvent("SPELL_INSTAKILL", c.unit(pair.Caster), c.unit(pair.Victim), pair.SpellID)
		}
		return true, err
	case world.SpellOrDamageImmunePacket, world.SpellProcResistPacket:
		pair, err := world.ParseSpellPair(packet.Body)
		if err == nil {
			miss := "IMMUNE"
			if packet.Opcode == world.SpellProcResistPacket {
				miss = "RESIST"
			}
			c.spellEvent("SPELL_MISSED", c.unit(pair.Caster), c.unit(pair.Victim), pair.SpellID, lua.LString(miss))
		}
		return true, err
	case world.SpellDamageShieldPacket:
		shield, err := world.ParseDamageShield(packet.Body)
		if err == nil {
			c.spellEvent("DAMAGE_SHIELD", c.unit(shield.Victim), c.unit(shield.Caster), shield.SpellID, lua.LNumber(shield.Damage),
				lua.LNumber(shield.School), lua.LNumber(0), lua.LNumber(0), lua.LNumber(0), lua.LNil, lua.LNil, lua.LNil)
		}
		return true, err
	case world.CastFailed:
		reader := world.NewReader(packet.Body, "SMSG_CAST_FAILED")
		if _, err := reader.U8(); err != nil {
			return false, nil
		}
		spell, err := reader.U32()
		if err != nil {
			return false, nil
		}
		code, err := reader.U8()
		if err == nil {
			c.castFailed(c.ownGUID, spell, code)
		}
		return false, nil
	case world.SpellFailedOtherPacket:
		reader := world.NewReader(packet.Body, "SMSG_SPELL_FAILED_OTHER")
		caster, err := world.ReadPackedGUID(reader)
		if err != nil || caster == c.ownGUID {
			return false, nil
		}
		if _, err = reader.U8(); err != nil {
			return false, nil
		}
		spell, err := reader.U32()
		if err != nil {
			return false, nil
		}
		code, err := reader.U8()
		if err == nil {
			c.castFailed(caster, spell, code)
		}
		return false, nil
	case world.SpellLogExecutePacket:
		return false, c.executeEvents(packet)
	case world.PartyKillLogPacket:
		reader := world.NewReader(packet.Body, "SMSG_PARTYKILLLOG")
		killer, err := reader.U64()
		if err != nil {
			return true, err
		}
		victim, err := reader.U64()
		if err == nil {
			c.rt.CombatEvent("PARTY_KILL", c.unit(killer), c.unit(victim))
		}
		return true, err
	case world.SpellStart, world.SpellGo:
		// The cast is reported here; the packet still goes on to the cast bar
		// and the spell visuals.
		header, err := world.ParseSpellCastHeader(packet.Body)
		if err != nil {
			return false, nil
		}
		if packet.Opcode == world.SpellStart {
			c.spellEvent("SPELL_CAST_START", c.unit(header.Caster), ui.CombatUnit{}, header.SpellID)
			return false, nil
		}
		_, hits, _ := world.ParseSpellGoHits(packet.Body)
		var target ui.CombatUnit
		if len(hits) > 0 {
			target = c.unit(hits[0])
		}
		c.spellEvent("SPELL_CAST_SUCCESS", c.unit(header.Caster), target, header.SpellID)
		return false, nil
	}
	return false, nil
}

// auraEvents reports the auras that came, went or changed.
func (c *worldCombat) auraEvents(auras *worldAuraState) {
	changes := auras.changes
	auras.changes = nil
	if c.rt == nil {
		return
	}
	for _, change := range changes {
		old, now := change.old, change.now
		switch {
		case old.Empty() && now.Empty():
			continue
		case old.Empty():
			c.auraEvent("SPELL_AURA_APPLIED", change.guid, now)
			if now.Charges > 1 {
				c.auraEvent("SPELL_AURA_APPLIED_DOSE", change.guid, now, lua.LNumber(now.Charges))
			}
		case now.Empty():
			c.auraEvent("SPELL_AURA_REMOVED", change.guid, old)
		case old.SpellID != now.SpellID:
			c.auraEvent("SPELL_AURA_REMOVED", change.guid, old)
			c.auraEvent("SPELL_AURA_APPLIED", change.guid, now)
		case now.Charges > old.Charges:
			c.auraEvent("SPELL_AURA_APPLIED_DOSE", change.guid, now, lua.LNumber(now.Charges))
		case now.Charges < old.Charges:
			c.auraEvent("SPELL_AURA_REMOVED_DOSE", change.guid, now, lua.LNumber(now.Charges))
		case now.MaxDuration != old.MaxDuration || now.Duration > old.Duration:
			c.auraEvent("SPELL_AURA_REFRESH", change.guid, now)
		}
	}
}

func (c *worldCombat) auraEvent(subevent string, guid uint64, slot world.AuraSlot, extra ...lua.LValue) {
	kind := "BUFF"
	if slot.Flags&0x80 != 0 {
		kind = "DEBUFF"
	}
	caster := slot.CasterGUID
	if caster == 0 {
		caster = guid
	}
	c.spellEvent(subevent, c.unit(caster), c.unit(guid), slot.SpellID, append([]lua.LValue{lua.LString(kind)}, extra...)...)
}

// aliveBefore notes which of the units an update names are alive.
func (c *worldCombat) aliveBefore(blocks []world.UpdateBlock) map[uint64]bool {
	if c.entities == nil {
		return nil
	}
	entities := c.entities()
	alive := map[uint64]bool{}
	for _, block := range blocks {
		entity := entities[block.GUID]
		if entity != nil && (entity.objectType == world.ObjectTypeUnit || entity.objectType == world.ObjectTypePlayer) && entity.fields[unitHealthField] > 0 {
			alive[block.GUID] = true
		}
	}
	return alive
}

// deathEvents reports the units that were alive and are not any more.
func (c *worldCombat) deathEvents(alive map[uint64]bool) {
	if len(alive) == 0 || c.rt == nil {
		return
	}
	entities := c.entities()
	for guid := range alive {
		if entity := entities[guid]; entity != nil && entity.fields[unitHealthField] == 0 {
			c.rt.CombatEvent("UNIT_DIED", ui.CombatUnit{}, c.unit(guid))
		}
	}
}

// executeEvents reports interrupts and summons.
func (c *worldCombat) executeEvents(packet world.Packet) error {
	caster, spell, entries, err := world.ParseSpellLogExecute(packet.Body)
	for _, entry := range entries {
		switch entry.Effect {
		case 68:
			c.spellEvent("SPELL_INTERRUPT", c.unit(caster), c.unit(entry.Target), spell,
				lua.LNumber(entry.Spell), lua.LString(c.rt.SpellName(entry.Spell)), lua.LNumber(c.rt.SpellSchool(entry.Spell)))
		case 19:
			c.spellEvent("SPELL_EXTRA_ATTACKS", c.unit(caster), c.unit(entry.Target), spell, lua.LNumber(entry.Spell))
		case 18, 113:
			c.spellEvent("SPELL_RESURRECT", c.unit(caster), c.unit(entry.Target), spell)
		default:
			c.spellEvent("SPELL_SUMMON", c.unit(caster), c.unit(entry.Target), spell)
		}
	}
	return err
}

// castFailed reports a spell that could not be cast.
func (c *worldCombat) castFailed(caster uint64, spell uint32, code uint8) {
	name := world.SpellFailedName(code)
	if name == "" {
		return
	}
	text := c.rt.GlobalString("SPELL_FAILED_"+name, name)
	c.spellEvent("SPELL_CAST_FAILED", c.unit(caster), ui.CombatUnit{}, spell, lua.LString(text))
}
