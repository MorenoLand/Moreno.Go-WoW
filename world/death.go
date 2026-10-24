package world

import "encoding/binary"

const (
	// ResurrectRequest (SMSG_RESURRECT_REQUEST) offers to bring the character
	// back; CorpseReclaimDelay says how long until the corpse can be reclaimed.
	ResurrectRequest   uint16 = 0x015B
	CorpseReclaimDelay uint16 = 0x0269

	ClientRepopRequest      ClientOpcode = 0x015A
	ClientResurrectResponse ClientOpcode = 0x015C
	ClientReclaimCorpse     ClientOpcode = 0x01D2
)

// ResurrectOffer is what SMSG_RESURRECT_REQUEST carries.
type ResurrectOffer struct {
	GUID     uint64
	Name     string
	Sickness bool
	Timer    bool
}

// ParseResurrectRequest reads SMSG_RESURRECT_REQUEST.
func ParseResurrectRequest(body []byte) (ResurrectOffer, error) {
	r := NewReader(body, "SMSG_RESURRECT_REQUEST")
	var offer ResurrectOffer
	var err error
	if offer.GUID, err = r.U64(); err != nil {
		return offer, err
	}
	if _, err = r.U32(); err != nil {
		return offer, err
	}
	if offer.Name, err = r.CString(); err != nil {
		return offer, err
	}
	sickness, err := r.U8()
	if err != nil {
		return offer, err
	}
	timer, err := r.U8()
	if err != nil {
		return offer, err
	}
	offer.Sickness, offer.Timer = sickness != 0, timer != 0
	return offer, nil
}

// ParseCorpseReclaimDelay reads the milliseconds SMSG_CORPSE_RECLAIM_DELAY names.
func ParseCorpseReclaimDelay(body []byte) (uint32, error) {
	r := NewReader(body, "SMSG_CORPSE_RECLAIM_DELAY")
	return r.U32()
}

// BuildResurrectResponse is the body of CMSG_RESURRECT_RESPONSE.
func BuildResurrectResponse(guid uint64, accept bool) []byte {
	body := make([]byte, 9)
	binary.LittleEndian.PutUint64(body, guid)
	if accept {
		body[8] = 1
	}
	return body
}
