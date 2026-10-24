package render

import (
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// PLAYER_FIELD_WATCHED_FACTION_INDEX of the 3.3.5a player object.
const fieldWatchedFaction = 1230

// worldFactionRecord is a row of Faction.dbc.
type worldFactionRecord struct {
	id, index, parent   uint32
	raceMask, classMask [4]uint32
	base                [4]int32
	flags               [4]uint32
	name, description   string
}

// worldReputation is the character's reputation: the slots the server sends
// and the Faction table that names them.
type worldReputation struct {
	loaded  bool
	records []worldFactionRecord
	// slots is the server's list, indexed by reputation list id.
	slots [world.FactionSlots]world.FactionState
	// received is set once the server has sent the list.
	received bool
	watched  int
}

func (rep *worldReputation) load(loader *ui.Loader) {
	if rep.loaded {
		return
	}
	rep.loaded = true
	table, err := loadCharacterDBC(loader, `DBFilesClient\Faction.dbc`)
	if err != nil || table.fields < 57 {
		return
	}
	for row := 0; row < table.records; row++ {
		record := worldFactionRecord{id: table.value(row, 0), index: table.value(row, 1), parent: table.value(row, 18), name: table.string(row, 23), description: table.string(row, 40)}
		for slot := 0; slot < 4; slot++ {
			record.raceMask[slot] = table.value(row, 2+slot)
			record.classMask[slot] = table.value(row, 6+slot)
			record.base[slot] = int32(table.value(row, 10+slot))
			record.flags[slot] = table.value(row, 14+slot)
		}
		rep.records = append(rep.records, record)
	}
}

// slotFor is the base-reputation slot of a faction for a race and a class, as
// Wow.exe (FUN_005d0ce0) and the server pick it.
func (record worldFactionRecord) slotFor(race, class uint8) int {
	raceBit, classBit := uint32(1)<<(race-1), uint32(1)<<(class-1)
	for slot := 0; slot < 4; slot++ {
		raceMask, classMask := record.raceMask[slot], record.classMask[slot]
		raceOK := raceMask&raceBit != 0 || (raceMask == 0 && classMask != 0)
		classOK := classMask&classBit != 0 || classMask == 0
		if raceOK && classOK {
			return slot
		}
	}
	return -1
}

// entries is the list the interface shows, for a character.
func (rep *worldReputation) entries(race, class uint8) []ui.FactionEntry {
	var entries []ui.FactionEntry
	for _, record := range rep.records {
		if int32(record.index) < 0 || record.index >= world.FactionSlots {
			continue
		}
		entry := ui.FactionEntry{ID: record.id, ListIndex: int(record.index), ParentID: record.parent, Name: record.name, Description: record.description}
		if slot := record.slotFor(race, class); slot >= 0 {
			entry.Base = record.base[slot]
			entry.HasRep = record.flags[slot]&0x80 != 0
		}
		state := rep.slots[record.index]
		entry.Flags, entry.Standing = state.Flags, state.Standing
		entries = append(entries, entry)
	}
	return entries
}

// handle takes a reputation packet; it reports whether the list changed.
func (rep *worldReputation) handle(packet world.Packet) bool {
	switch packet.Opcode {
	case world.InitializeFactions:
		states, err := world.ParseInitializeFactions(packet.Body)
		if err != nil {
			return false
		}
		for index := range rep.slots {
			rep.slots[index] = world.FactionState{}
		}
		copy(rep.slots[:], states)
		rep.received = true
		return true
	case world.SetFactionStanding:
		changes, err := world.ParseSetFactionStanding(packet.Body)
		if err != nil {
			return false
		}
		for _, change := range changes {
			if change.Index < world.FactionSlots {
				rep.slots[change.Index].Standing = change.Standing
			}
		}
		return true
	case world.SetFactionVisible:
		index, err := world.ParseSetFactionVisible(packet.Body)
		if err != nil || index >= world.FactionSlots {
			return false
		}
		rep.slots[index].Flags |= 0x01
		return true
	}
	return false
}

// push hands the interface the list and the watched faction.
func (rep *worldReputation) push(rt *ui.Runtime, loader *ui.Loader, race, class uint8, watched uint32) {
	if !rep.received {
		return
	}
	rep.load(loader)
	rep.watched = int(int32(watched))
	rt.SetFactions(rep.entries(race, class), rep.watched)
}

func (h *clientHost) SetWatchedFaction(index uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientSetWatchedFaction, world.BuildSetWatchedFaction(index))
}

func (h *clientHost) SetFactionAtWar(index uint32, on bool) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientSetFactionAtWar, world.BuildSetFactionFlag(index, on))
}

func (h *clientHost) SetFactionInactive(index uint32, on bool) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientSetFactionInactive, world.BuildSetFactionFlag(index, on))
}
