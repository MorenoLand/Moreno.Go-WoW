package render

import (
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// worldTokenState remembers which unit each token follows and what of it the
// interface was last told.
type worldTokenState struct {
	guid map[string]uint64
	key  map[string]groupDetailKey
}

func newWorldTokenState() *worldTokenState {
	return &worldTokenState{guid: make(map[string]uint64), key: make(map[string]groupDetailKey)}
}

// sync makes a unit token (the pet, the focus, the target's target) the unit of
// a guid, and keeps its vitals current.
func (state *worldTokenState) sync(rt *ui.Runtime, token string, guid uint64, entities map[uint64]*worldEntity, player *worldEntity, names *worldNames, factions *worldFactions, send func(world.ClientOpcode, []byte) error, auras *worldAuraState, loader *ui.Loader) bool {
	entity := entities[guid]
	if guid == 0 || entity == nil {
		if state.guid[token] != 0 || rt.UnitGUID(token) != 0 {
			state.guid[token] = 0
			delete(state.key, token)
			rt.ClearUnit(token)
			return true
		}
		return false
	}
	names.request(entity, send)
	info := names.unitInfoFor(entity, factions, player.fields[unitFactionField])
	key := groupDetailKey{name: info.Name, race: info.RaceID, class: info.ClassID, sex: info.Sex, level: info.Level,
		hostile: info.Hostile, friendly: info.Friendly, displayID: info.DisplayID, connected: info.Connected}
	changed := false
	if state.guid[token] != guid || state.key[token] != key {
		state.guid[token], state.key[token] = guid, key
		rt.SetUnit(token, info)
		if auras != nil {
			auras.push(rt, loader, token, guid)
		}
		changed = true
	}
	if rt.ApplyUnitFields(token, entity.fields) {
		changed = true
	}
	return changed
}

// worldUnitTarget is the guid a unit has targeted.
func worldUnitTarget(entity *worldEntity) uint64 {
	if entity == nil {
		return 0
	}
	return uint64(entity.fields[world.UnitTargetField]) | uint64(entity.fields[world.UnitTargetField+1])<<32
}

// fieldSummon is UNIT_FIELD_SUMMON, the pet or guardian a unit has out.
const fieldSummon = 8

// syncWorldUnits keeps the tokens that follow another unit: the pet, the
// focus, and the targets of the target, the focus and the pet.
func syncWorldUnits(rt *ui.Runtime, state *worldTokenState, entities map[uint64]*worldEntity, player *worldEntity, names *worldNames, factions *worldFactions, send func(world.ClientOpcode, []byte) error, auras *worldAuraState, loader *ui.Loader) bool {
	changed := false
	pet := uint64(player.fields[fieldSummon]) | uint64(player.fields[fieldSummon+1])<<32
	if state.sync(rt, "pet", pet, entities, player, names, factions, send, auras, loader) {
		changed = true
	}
	if entity := entities[pet]; entity != nil {
		rt.SetPetUnitFields(entity.fields)
	} else {
		rt.ClearPetUnit()
	}
	if state.sync(rt, "focus", rt.FocusGUID(), entities, player, names, factions, send, auras, loader) {
		changed = true
	}
	targets := map[string]uint64{
		"targettarget": worldUnitTarget(entities[rt.UnitGUID("target")]),
		"focustarget":  worldUnitTarget(entities[rt.FocusGUID()]),
		"pettarget":    worldUnitTarget(entities[pet]),
	}
	for token, guid := range targets {
		if state.sync(rt, token, guid, entities, player, names, factions, send, nil, loader) {
			changed = true
		}
	}
	return changed
}
