package render

import (
	"fmt"
	"sort"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The maps arenas are played on.
var arenaMaps = map[uint32]bool{559: true, 562: true, 572: true, 617: true, 618: true}

const (
	arenaSlots      = 5
	fieldSummonedBy = 14
)

// worldArena remembers the opposing players and pets of an arena by slot.
type worldArena struct {
	players [arenaSlots]uint64
	pets    [arenaSlots]uint64
	seen    [arenaSlots]bool
}

// syncWorldArena fills the slots of the arena tokens with the hostile players in
// view and their pets, and tells the interface when one comes or goes.
func syncWorldArena(rt *ui.Runtime, state *worldArena, mapID uint32, entities map[uint64]*worldEntity, player *worldEntity, names *worldNames, factions *worldFactions, tokens *worldTokenState, send func(world.ClientOpcode, []byte) error, loader *ui.Loader) {
	if !arenaMaps[mapID] {
		if *state != (worldArena{}) {
			*state = worldArena{}
			rt.SetArenaOpponents(0)
			for slot := 1; slot <= arenaSlots; slot++ {
				tokens.sync(rt, fmt.Sprintf("arena%d", slot), 0, entities, player, names, factions, send, nil, loader)
				tokens.sync(rt, fmt.Sprintf("arenapet%d", slot), 0, entities, player, names, factions, send, nil, loader)
			}
		}
		return
	}
	var found []uint64
	for guid, entity := range entities {
		if entity.objectType == world.ObjectTypePlayer && guid != player.guid && names.unitInfoFor(entity, factions, player.fields[unitFactionField]).Hostile {
			found = append(found, guid)
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i] < found[j] })
	// Opponents keep the slot they have; new ones take the first free one.
	for _, guid := range found {
		known := false
		for _, held := range state.players {
			known = known || held == guid
		}
		if known {
			continue
		}
		for slot := range state.players {
			if state.players[slot] == 0 {
				state.players[slot] = guid
				break
			}
		}
	}
	count := 0
	for slot, guid := range state.players {
		if guid == 0 {
			continue
		}
		count++
		visible := entities[guid] != nil
		if visible != state.seen[slot] {
			state.seen[slot] = visible
			status := "unseen"
			if visible {
				status = "seen"
			}
			rt.FireEvent("ARENA_OPPONENT_UPDATE", lua.LString(fmt.Sprintf("arena%d", slot+1)), lua.LString(status))
		}
		pet := uint64(0)
		for petGUID, entity := range entities {
			if entity.objectType == world.ObjectTypeUnit && uint64(entity.fields[fieldSummonedBy])|uint64(entity.fields[fieldSummonedBy+1])<<32 == guid {
				pet = petGUID
			}
		}
		state.pets[slot] = pet
	}
	for slot := range state.players {
		tokens.sync(rt, fmt.Sprintf("arena%d", slot+1), state.players[slot], entities, player, names, factions, send, nil, loader)
		tokens.sync(rt, fmt.Sprintf("arenapet%d", slot+1), state.pets[slot], entities, player, names, factions, send, nil, loader)
	}
	rt.SetArenaOpponents(count)
}
