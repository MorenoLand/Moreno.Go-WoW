package render

import (
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// The unit and player fields the standings between units are read from.
const (
	fieldCharmedBy     = 12
	fieldCreatedBy     = 16
	unitFlags2Field    = 60
	unitBytes2Field    = 122
	playerFlagsFieldID = 150
	playerDuelArbiter  = 148
	playerDuelTeam     = 156
)

// Unit flags.
const (
	unitFlagNonAttackable  = 0x00000002
	unitFlagPlayerControl  = 0x00000008
	unitFlagImmuneToPlayer = 0x00000100
	unitFlagImmuneToNPC    = 0x00000200
	unitFlagNotAttackable1 = 0x00000080
	unitFlagUnknown10000   = 0x00010000
	unitFlagImmuneBit20    = 0x00100000
	unitFlagImmuneBit25    = 0x02000000
	unitFlags2NoAssist     = 0x00000004
	unitFlags2Vehicle      = 0x00010000
	// The PvP byte of the unit.
	pvpFlagFreeForAll = 0x04
	pvpFlagPVP        = 0x02
	pvpFlagSanctuary  = 0x08
	pvpFlagOther      = 0x01
	// Creature type flags that matter.
	creatureFlagGhostVisible = 0x00000002
	creatureFlagAssistOne    = 0x00001000
	creatureFlagAssistTwo    = 0x04000000
	// Faction template flag, vehicle flag.
	factionFlagContested = 0x1000
	vehicleFlagNoAttack  = 0x20000000
	playerFlagBit8       = 0x00000100
	playerFlagBit19      = 0x00080000
	playerFlagGhost      = 0x00000010
)

// worldRelations works out how units stand to each other the way the original
// client does, from the objects it knows.
type worldRelations struct {
	rt       *ui.Runtime
	entities func() map[uint64]*worldEntity
	factions func() *worldFactions
	names    func() *worldNames
	own      func() uint64
}

func (r *worldRelations) entity(guid uint64) *worldEntity {
	if guid == 0 {
		return nil
	}
	return r.entities()[guid]
}

func fieldGUIDAt(e *worldEntity, index uint16) uint64 {
	return uint64(e.fields[index]) | uint64(e.fields[index+1])<<32
}

func (e *worldEntity) isPlayer() bool { return e.objectType == world.ObjectTypePlayer }

func (e *worldEntity) flag(mask uint32) bool { return e.fields[unitFlagsField]&mask != 0 }

func (e *worldEntity) pvp() uint32 { return e.fields[unitBytes2Field] >> 8 & 0xFF }

// controllerGuid is who controls the unit: the one that charmed it, or made it.
func controllerGUID(e *worldEntity) uint64 {
	if guid := fieldGUIDAt(e, fieldCharmedBy); guid != 0 {
		return guid
	}
	return fieldGUIDAt(e, fieldCreatedBy)
}

// controller is the player in control of a unit: itself, or the player that
// controls it, directly or through one more unit.
func (r *worldRelations) controller(e *worldEntity) *worldEntity {
	current := e
	if guid := controllerGUID(e); guid != 0 {
		current = r.entity(guid)
	}
	if current == nil {
		return nil
	}
	if current.isPlayer() {
		return current
	}
	if next := r.entity(controllerGUID(current)); next != nil && next.isPlayer() {
		return next
	}
	return nil
}

func (r *worldRelations) creatureFlags(e *worldEntity) (uint32, bool) {
	if e.isPlayer() {
		return 0, false
	}
	info, ok := r.names().creatures[e.fields[objectEntryField]]
	return info.TypeFlag, ok
}

// templateReaction is how a unit of the first template stands to one of the
// second: hostile, neutral or friendly.
func (r *worldRelations) templateReaction(a, b worldFactionTemplate) int {
	if a.enemyGroup&b.group != 0 {
		return 1
	}
	for _, enemy := range a.enemies {
		if enemy == 0 {
			break
		}
		if enemy == b.faction {
			return 1
		}
	}
	if a.friendGroup&b.group != 0 {
		return 4
	}
	for _, friend := range a.friends {
		if friend == 0 {
			break
		}
		if friend == b.faction {
			return 4
		}
	}
	if b.friendGroup&a.group != 0 {
		return 4
	}
	for _, friend := range b.friends {
		if friend == 0 {
			break
		}
		if friend == a.faction {
			return 4
		}
	}
	if a.flags&factionFlagContested != 0 {
		return 1
	}
	return 3
}

// repReaction is how a standing with a faction reads.
func repReaction(standing int32) int {
	switch {
	case standing >= 42000:
		return 7
	case standing >= 21000:
		return 6
	case standing >= 9000:
		return 5
	case standing >= 3000:
		return 4
	case standing < 0 && standing > -3000:
		return 2
	case standing < 0 && standing > -6001:
		return 1
	case standing < 0:
		return 0
	}
	return 3
}

// reactionTo is how a unit of a template stands to another unit, bringing in
// the character's reputation when that unit is one of the character's.
func (r *worldRelations) reactionTo(template uint32, b *worldEntity) int {
	factions := r.factions()
	tA, okA := factions.templates[template]
	tB, okB := factions.templates[b.fields[unitFactionField]]
	if !okA || !okB {
		return 3
	}
	if b.flag(unitFlagPlayerControl) {
		if c := r.controller(b); c != nil && c.guid == r.own() {
			if tA.flags&factionFlagContested != 0 && c.fields[playerFlagsFieldID]&playerFlagBit8 != 0 {
				return 1
			}
			if forced, ok := r.rt.ForcedReaction(tA.faction); ok {
				return forced
			}
			if b.fields[unitFlags2Field]&unitFlags2NoAssist == 0 {
				if standing, _, listed := r.rt.FactionStanding(tA.faction); listed {
					return repReaction(standing)
				}
			}
		}
	}
	return r.templateReaction(tA, tB)
}

// inDuelWith is whether two players are on opposite sides of one duel.
func duelOpposed(a, b *worldEntity) bool {
	return a.fields[playerDuelTeam] != 0 && b.fields[playerDuelTeam] != 0 &&
		fieldGUIDAt(a, playerDuelArbiter) == fieldGUIDAt(b, playerDuelArbiter)
}

// duelPet is whether a unit with no player in charge belongs to the duel opponent.
func (r *worldRelations) duelPet(e *worldEntity) bool {
	guid := controllerGUID(e)
	return guid != 0 && guid == r.rt.DuelOpponent()
}

// duelHostile tells whether a duel puts two units against each other.
func (r *worldRelations) duelHostile(x, y *worldEntity) bool {
	if y == nil || y.objectType == world.ObjectTypeItem {
		return false
	}
	if !x.flag(unitFlagPlayerControl) || !y.flag(unitFlagPlayerControl) {
		return false
	}
	ax, by := x, y
	if !x.isPlayer() {
		ax = r.controller(x)
	}
	if !y.isPlayer() {
		by = r.controller(y)
	}
	if ax == nil || by == nil {
		switch {
		case x.guid == r.own() && by == nil:
			return r.duelPet(y)
		case y.guid == r.own() && ax == nil:
			return r.duelPet(x)
		}
		return false
	}
	return duelOpposed(ax, by) && ax.fields[playerDuelTeam] != by.fields[playerDuelTeam]
}

func (r *worldRelations) Reaction(aGUID, bGUID uint64) int {
	a, b := r.entity(aGUID), r.entity(bGUID)
	if a == nil || b == nil {
		return 3
	}
	return r.reaction(a, b)
}

func (r *worldRelations) reaction(a, b *worldEntity) int {
	if a == b {
		return 4
	}
	own := r.own()
	if a.flag(unitFlagPlayerControl) && b.flag(unitFlagPlayerControl) {
		ca, cb := r.controller(a), r.controller(b)
		if ca == nil || cb == nil {
			hostile := false
			if a.guid == own && cb == nil {
				hostile = r.duelHostile(a, b)
			} else if b.guid == own && ca == nil {
				hostile = r.duelHostile(b, a)
			}
			if hostile {
				return 1
			}
		} else {
			if duelOpposed(ca, cb) {
				if ca.fields[playerDuelTeam] != cb.fields[playerDuelTeam] {
					return 1
				}
				return 4
			}
			if ca == cb {
				return 4
			}
			if ca.guid == own && r.rt.MemberInGroup(cb.guid) || cb.guid == own && r.rt.MemberInGroup(ca.guid) {
				return r.reactionTo(a.fields[unitFactionField], b)
			}
		}
		if a.pvp()&pvpFlagFreeForAll != 0 && b.pvp()&pvpFlagFreeForAll != 0 {
			return 1
		}
	}
	if a.flag(unitFlagPlayerControl) {
		if c := r.controller(a); c != nil && c.guid == own {
			if tB, ok := r.factions().templates[b.fields[unitFactionField]]; ok {
				if forced, ok := r.rt.ForcedReaction(tB.faction); ok {
					return forced
				}
				if c.fields[unitFlags2Field]&unitFlags2NoAssist == 0 {
					if _, atWar, listed := r.rt.FactionStanding(tB.faction); listed {
						if tB.flags&factionFlagContested != 0 && c.fields[playerFlagsFieldID]&playerFlagBit8 != 0 {
							return 1
						}
						if atWar {
							return 1
						}
						return 4
					}
				}
			}
		}
	}
	result := r.reactionTo(a.fields[unitFactionField], b)
	if result > 6 {
		result = 7
	}
	return result
}

// friendly is whether a unit counts another a friend.
func (r *worldRelations) friendly(a, b *worldEntity) bool {
	if r.reaction(a, b) >= 4 {
		return true
	}
	flags, ok := r.creatureFlags(a)
	return ok && flags&creatureFlagAssistTwo != 0
}

func (r *worldRelations) Friendly(aGUID, bGUID uint64) bool {
	a, b := r.entity(aGUID), r.entity(bGUID)
	return a != nil && b != nil && r.friendly(a, b)
}

func (r *worldRelations) CanAttack(aGUID, bGUID uint64) bool {
	a, b := r.entity(aGUID), r.entity(bGUID)
	return a != nil && b != nil && r.canAttack(a, b)
}

func (r *worldRelations) canAttack(a, b *worldEntity) bool {
	if a.isPlayer() && a.fields[playerFlagsFieldID]&playerFlagBit19 != 0 {
		return false
	}
	if b.isPlayer() && b.fields[playerFlagsFieldID]&playerFlagGhost != 0 {
		if flags, ok := r.creatureFlags(a); !ok || flags&creatureFlagGhostVisible == 0 {
			return false
		}
	}
	flagsB, flagsA := b.fields[unitFlagsField], a.fields[unitFlagsField]
	if flagsB&(unitFlagNonAttackable|unitFlagImmuneBit20|unitFlagNotAttackable1|unitFlagUnknown10000|unitFlagImmuneBit25) != 0 {
		return false
	}
	aPlayer, bPlayer := flagsA&unitFlagPlayerControl != 0, flagsB&unitFlagPlayerControl != 0
	if aPlayer && flagsB&unitFlagImmuneToPlayer != 0 || !aPlayer && flagsB&unitFlagImmuneToNPC != 0 {
		return false
	}
	if bPlayer && flagsA&unitFlagImmuneToPlayer != 0 || !bPlayer && flagsA&unitFlagImmuneToNPC != 0 {
		return false
	}
	if a.fields[unitFlags2Field]&unitFlags2Vehicle != 0 || b.fields[unitFlags2Field]&unitFlags2Vehicle != 0 {
		if r.rt.VehicleHasFlag(a.guid, vehicleFlagNoAttack) || r.rt.VehicleHasFlag(b.guid, vehicleFlagNoAttack) {
			return false
		}
	}
	switch {
	case !aPlayer && !bPlayer:
		return r.hostile(a, b) || r.hostile(b, a)
	case aPlayer && bPlayer:
		if r.friendly(a, b) {
			return false
		}
		ca, cb := r.controller(a), r.controller(b)
		if ca == nil || cb == nil {
			if a.pvp()&pvpFlagSanctuary != 0 {
				return false
			}
			return b.pvp()&pvpFlagSanctuary == 0
		}
		if ca.fields[playerDuelTeam] != 0 && cb.fields[playerDuelTeam] != 0 && fieldGUIDAt(ca, playerDuelArbiter) == fieldGUIDAt(cb, playerDuelArbiter) {
			return true
		}
		pb, pa := b.pvp(), a.pvp()
		if pb&pvpFlagOther == 0 {
			if pa&pvpFlagFreeForAll != 0 && pb&pvpFlagFreeForAll != 0 {
				return true
			}
			if pa&pvpFlagPVP == 0 && pb&pvpFlagPVP == 0 {
				return false
			}
			if pa&pvpFlagSanctuary != 0 {
				return false
			}
			return pb&pvpFlagSanctuary == 0
		}
		if pa&pvpFlagSanctuary != 0 {
			return false
		}
		return pb&pvpFlagSanctuary == 0
	}
	if aPlayer && b.pvp()&pvpFlagSanctuary != 0 {
		return false
	}
	if bPlayer && a.pvp()&pvpFlagSanctuary != 0 {
		return false
	}
	return !r.friendly(a, b)
}

// hostile is whether a unit is hostile to another.
func (r *worldRelations) hostile(a, b *worldEntity) bool { return r.reaction(a, b) < 2 }

func (r *worldRelations) CanAssist(aGUID, bGUID uint64) bool {
	a, b := r.entity(aGUID), r.entity(bGUID)
	if a == nil || b == nil {
		return false
	}
	flagsB := b.fields[unitFlagsField]
	if flagsB&unitFlagImmuneBit25 != 0 {
		return false
	}
	aPlayer := a.fields[unitFlagsField]&unitFlagPlayerControl != 0
	if aPlayer && flagsB&unitFlagImmuneToPlayer != 0 || !aPlayer && flagsB&unitFlagImmuneToNPC != 0 {
		return false
	}
	if r.reaction(a, b) < 4 {
		if flags, ok := r.creatureFlags(a); !ok || flags&creatureFlagAssistTwo == 0 {
			return false
		}
	}
	if flagsB&unitFlagPlayerControl == 0 {
		if aPlayer && b.pvp()&pvpFlagOther == 0 {
			flags, ok := r.creatureFlags(a)
			if !ok || (flags&creatureFlagAssistOne == 0 && flags&creatureFlagAssistTwo == 0) {
				return false
			}
		}
		return true
	}
	ca, cb := r.controller(a), r.controller(b)
	if ca != nil && cb != nil && cb.fields[playerDuelTeam] != 0 &&
		(fieldGUIDAt(ca, playerDuelArbiter) != fieldGUIDAt(cb, playerDuelArbiter) || ca.fields[playerDuelTeam] != cb.fields[playerDuelTeam]) {
		return false
	}
	pb := b.pvp()
	if pb&pvpFlagFreeForAll != 0 && a.pvp()&pvpFlagFreeForAll == 0 {
		return false
	}
	if a.pvp()&pvpFlagSanctuary != 0 && pb&pvpFlagSanctuary == 0 && pb&pvpFlagOther != 0 {
		return false
	}
	return true
}

func (r *worldRelations) CanCooperate(aGUID, bGUID uint64) bool {
	a, b := r.entity(aGUID), r.entity(bGUID)
	if a == nil || b == nil || a == b {
		return false
	}
	if fieldGUIDAt(a, fieldCharmedBy) != 0 || fieldGUIDAt(b, fieldCharmedBy) != 0 {
		return false
	}
	factions := r.factions()
	tA, okA := factions.templates[a.fields[unitFactionField]]
	tB, okB := factions.templates[b.fields[unitFactionField]]
	if !okA || !okB || tA.group == tB.group {
		return !r.canAttack(a, b)
	}
	return false
}
