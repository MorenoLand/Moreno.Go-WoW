package world

import "encoding/binary"

const (
	// InitializeFactions (SMSG_INITIALIZE_FACTIONS) lists the character's 128
	// reputation slots, SetFactionVisible shows one, and SetFactionStanding
	// changes some.
	InitializeFactions uint16 = 0x0122
	SetFactionVisible  uint16 = 0x0123
	SetFactionStanding uint16 = 0x0124
	// SetFactionAtWar (SMSG_SET_FACTION_ATWAR) is kept for completeness; the
	// server also sends the changed flags with the initial list.
	SetFactionAtWar uint16 = 0x0313

	ClientSetFactionAtWar    ClientOpcode = 0x0125
	ClientSetFactionInactive ClientOpcode = 0x0317
	ClientSetWatchedFaction  ClientOpcode = 0x0318

	// FactionSlots is how many reputation list entries a character has.
	FactionSlots = 128
)

// FactionState is one reputation slot: the flags and the standing the server
// keeps, which is counted from the faction's base reputation.
type FactionState struct {
	Flags    uint8
	Standing int32
}

// ParseInitializeFactions reads SMSG_INITIALIZE_FACTIONS.
func ParseInitializeFactions(body []byte) ([]FactionState, error) {
	r := NewReader(body, "SMSG_INITIALIZE_FACTIONS")
	count, err := r.U32()
	if err != nil {
		return nil, err
	}
	if count > 1024 {
		count = 1024
	}
	states := make([]FactionState, 0, count)
	for index := uint32(0); index < count; index++ {
		flags, err := r.U8()
		if err != nil {
			return nil, err
		}
		standing, err := r.U32()
		if err != nil {
			return nil, err
		}
		states = append(states, FactionState{Flags: flags, Standing: int32(standing)})
	}
	return states, nil
}

// FactionChange is a slot SMSG_SET_FACTION_STANDING reports.
type FactionChange struct {
	Index    uint32
	Standing int32
}

// ParseSetFactionStanding reads SMSG_SET_FACTION_STANDING: a bonus the
// interface ignores, whether to show it, then the changed slots.
func ParseSetFactionStanding(body []byte) ([]FactionChange, error) {
	r := NewReader(body, "SMSG_SET_FACTION_STANDING")
	if _, err := r.F32(); err != nil {
		return nil, err
	}
	if _, err := r.U8(); err != nil {
		return nil, err
	}
	count, err := r.U32()
	if err != nil {
		return nil, err
	}
	if count > 1024 {
		count = 1024
	}
	changes := make([]FactionChange, 0, count)
	for index := uint32(0); index < count; index++ {
		slot, err := r.U32()
		if err != nil {
			return nil, err
		}
		standing, err := r.U32()
		if err != nil {
			return nil, err
		}
		changes = append(changes, FactionChange{Index: slot, Standing: int32(standing)})
	}
	return changes, nil
}

// ParseSetFactionVisible reads the slot SMSG_SET_FACTION_VISIBLE shows.
func ParseSetFactionVisible(body []byte) (uint32, error) {
	r := NewReader(body, "SMSG_SET_FACTION_VISIBLE")
	return r.U32()
}

// BuildSetWatchedFaction is the body of CMSG_SET_WATCHED_FACTION.
func BuildSetWatchedFaction(index uint32) []byte {
	body := make([]byte, 4)
	binary.LittleEndian.PutUint32(body, index)
	return body
}

// BuildSetFactionFlag is the body of CMSG_SET_FACTION_ATWAR and
// CMSG_SET_FACTION_INACTIVE: a slot and the new state.
func BuildSetFactionFlag(index uint32, on bool) []byte {
	body := make([]byte, 5)
	binary.LittleEndian.PutUint32(body, index)
	if on {
		body[4] = 1
	}
	return body
}
