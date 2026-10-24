package world

import "encoding/binary"

const (
	// TalentsInfo (SMSG_TALENTS_INFO) lists the talents a character has learned
	// in each specialization.
	TalentsInfo uint16 = 0x04C0

	ClientLearnTalent ClientOpcode = 0x0251
)

// TalentSpec is one talent group: the ranks learned, by talent id (rank 1 is
// the first), and the glyphs set.
type TalentSpec struct {
	Ranks  map[uint32]int
	Glyphs []uint16
}

// Talents is the character's talent state.
type Talents struct {
	Unspent uint32
	Active  int
	Specs   []TalentSpec
}

// ParseTalentsInfo reads SMSG_TALENTS_INFO for the character; it reports false
// for the pet's talents.
func ParseTalentsInfo(body []byte) (Talents, bool, error) {
	r := NewReader(body, "SMSG_TALENTS_INFO")
	var talents Talents
	pet, err := r.U8()
	if err != nil {
		return talents, false, err
	}
	if pet != 0 {
		return talents, false, nil
	}
	if talents.Unspent, err = r.U32(); err != nil {
		return talents, true, err
	}
	groups, err := r.U8()
	if err != nil {
		return talents, true, err
	}
	active, err := r.U8()
	if err != nil {
		return talents, true, err
	}
	talents.Active = int(active)
	for group := 0; group < int(groups); group++ {
		spec := TalentSpec{Ranks: make(map[uint32]int)}
		count, err := r.U8()
		if err != nil {
			return talents, true, err
		}
		for index := 0; index < int(count); index++ {
			id, err := r.U32()
			if err != nil {
				return talents, true, err
			}
			rank, err := r.U8()
			if err != nil {
				return talents, true, err
			}
			spec.Ranks[id] = int(rank) + 1
		}
		glyphs, err := r.U8()
		if err != nil {
			return talents, true, err
		}
		for index := 0; index < int(glyphs); index++ {
			glyph, err := r.U16()
			if err != nil {
				return talents, true, err
			}
			spec.Glyphs = append(spec.Glyphs, glyph)
		}
		talents.Specs = append(talents.Specs, spec)
	}
	return talents, true, nil
}

// BuildLearnTalent is the body of CMSG_LEARN_TALENT: the talent and the rank
// to learn (counted from 0).
func BuildLearnTalent(id, rank uint32) []byte {
	body := make([]byte, 8)
	binary.LittleEndian.PutUint32(body, id)
	binary.LittleEndian.PutUint32(body[4:], rank)
	return body
}
