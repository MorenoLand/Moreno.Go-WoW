package world

import "encoding/binary"

// Petition packets.
const (
	PetitionShowListPacket       uint16 = 0x01BC
	PetitionShowSignaturesPacket uint16 = 0x01BF
	PetitionSignResultsPacket    uint16 = 0x01C1
	PetitionDeclinePacket        uint16 = 0x01C2
	TurnInPetitionResultsPacket  uint16 = 0x01C5
	PetitionQueryPacket          uint16 = 0x01C7
	PetitionRenamePacket         uint16 = 0x02C1
	OfferPetitionErrorPacket     uint16 = 0x038F

	ClientPetitionBuy        ClientOpcode = 0x01BD
	ClientPetitionSign       ClientOpcode = 0x01C0
	ClientPetitionDecline    ClientOpcode = 0x01C2
	ClientOfferPetition      ClientOpcode = 0x01C3
	ClientTurnInPetition     ClientOpcode = 0x01C4
	ClientPetitionQuery      ClientOpcode = 0x01C6
	ClientPetitionRename     ClientOpcode = 0x02C1
	ClientPetitionShowSigned ClientOpcode = 0x01BE
)

// PetitionOffer is a charter a registrar sells.
type PetitionOffer struct {
	Index, Entry, Display, Cost, Extra, Signatures uint32
}

// PetitionList is SMSG_PETITION_SHOWLIST.
type PetitionList struct {
	Registrar uint64
	Offers    []PetitionOffer
}

// ParsePetitionList reads SMSG_PETITION_SHOWLIST.
func ParsePetitionList(body []byte) (PetitionList, error) {
	r := NewReader(body, "SMSG_PETITION_SHOWLIST")
	var list PetitionList
	var err error
	if list.Registrar, err = r.U64(); err != nil {
		return list, err
	}
	count, err := r.U8()
	if err != nil {
		return list, err
	}
	for index := 0; index < int(count) && index < 10; index++ {
		var offer PetitionOffer
		for _, field := range []*uint32{&offer.Index, &offer.Entry, &offer.Display, &offer.Cost, &offer.Extra, &offer.Signatures} {
			if *field, err = r.U32(); err != nil {
				return list, err
			}
		}
		list.Offers = append(list.Offers, offer)
	}
	return list, nil
}

// PetitionSignatures is SMSG_PETITION_SHOW_SIGNATURES.
type PetitionSignatures struct {
	Item, Owner uint64
	ID          uint32
	Signers     []uint64
}

// ParsePetitionSignatures reads SMSG_PETITION_SHOW_SIGNATURES.
func ParsePetitionSignatures(body []byte) (PetitionSignatures, error) {
	r := NewReader(body, "SMSG_PETITION_SHOW_SIGNATURES")
	var signatures PetitionSignatures
	var err error
	if signatures.Item, err = r.U64(); err != nil {
		return signatures, err
	}
	if signatures.Owner, err = r.U64(); err != nil {
		return signatures, err
	}
	if signatures.ID, err = r.U32(); err != nil {
		return signatures, err
	}
	count, err := r.U8()
	if err != nil {
		return signatures, err
	}
	for index := 0; index < int(count); index++ {
		guid, err := r.U64()
		if err != nil {
			return signatures, err
		}
		if _, err = r.U32(); err != nil {
			return signatures, err
		}
		signatures.Signers = append(signatures.Signers, guid)
	}
	return signatures, nil
}

// PetitionInfo is SMSG_PETITION_QUERY_RESPONSE.
type PetitionInfo struct {
	ID                 uint32
	Owner              uint64
	Name, Body         string
	MinSignatures      uint32
	MaxSignatures      uint32
	MinLevel, MaxLevel uint32
	Arena              bool
}

// ParsePetitionInfo reads SMSG_PETITION_QUERY_RESPONSE.
func ParsePetitionInfo(body []byte) (PetitionInfo, error) {
	r := NewReader(body, "SMSG_PETITION_QUERY_RESPONSE")
	var info PetitionInfo
	var err error
	if info.ID, err = r.U32(); err != nil {
		return info, err
	}
	if info.Owner, err = r.U64(); err != nil {
		return info, err
	}
	if info.Name, err = r.CString(); err != nil {
		return info, err
	}
	if info.Body, err = r.CString(); err != nil {
		return info, err
	}
	words := make([]uint32, 7)
	for index := range words {
		if words[index], err = r.U32(); err != nil {
			return info, err
		}
	}
	info.MinSignatures, info.MaxSignatures = words[0], words[1]
	if _, err = r.U16(); err != nil {
		return info, err
	}
	if info.MinLevel, err = r.U32(); err != nil {
		return info, err
	}
	if info.MaxLevel, err = r.U32(); err != nil {
		return info, err
	}
	if _, err = r.U32(); err != nil {
		return info, err
	}
	for index := 0; index < 10; index++ {
		if _, err = r.CString(); err != nil {
			return info, err
		}
	}
	if _, err = r.U32(); err != nil {
		return info, err
	}
	kind, err := r.U32()
	info.Arena = kind != 0
	return info, err
}

// BuildPetitionBuy is the body of CMSG_PETITION_BUY.
func BuildPetitionBuy(registrar uint64, name string, index uint32) []byte {
	body := binary.LittleEndian.AppendUint64(nil, registrar)
	body = binary.LittleEndian.AppendUint32(body, 0)
	body = binary.LittleEndian.AppendUint64(body, 0)
	body = append(append(body, name...), 0)
	body = append(body, 0)
	for range 7 {
		body = binary.LittleEndian.AppendUint32(body, 0)
	}
	body = binary.LittleEndian.AppendUint16(body, 0)
	for range 3 {
		body = binary.LittleEndian.AppendUint32(body, 0)
	}
	for range 10 {
		body = append(body, 0)
	}
	body = binary.LittleEndian.AppendUint32(body, index)
	return binary.LittleEndian.AppendUint32(body, 0)
}

// BuildPetitionSign is the body of CMSG_PETITION_SIGN.
func BuildPetitionSign(petition uint64) []byte {
	return append(binary.LittleEndian.AppendUint64(nil, petition), 1)
}

// BuildOfferPetition is the body of CMSG_OFFER_PETITION.
func BuildOfferPetition(slot uint32, petition, target uint64) []byte {
	body := binary.LittleEndian.AppendUint32(nil, slot)
	body = binary.LittleEndian.AppendUint64(body, petition)
	return binary.LittleEndian.AppendUint64(body, target)
}

// BuildPetitionQuery is the body of CMSG_PETITION_QUERY.
func BuildPetitionQuery(id uint32, item uint64) []byte {
	body := binary.LittleEndian.AppendUint32(nil, id)
	return binary.LittleEndian.AppendUint64(body, item)
}

// BuildPetitionRename is the body of MSG_PETITION_RENAME.
func BuildPetitionRename(petition uint64, name string) []byte {
	body := binary.LittleEndian.AppendUint64(nil, petition)
	return append(append(body, name...), 0)
}

// BuildTurnInPetition is the body of CMSG_TURN_IN_PETITION: the charter and,
// for an arena team, its colours.
func BuildTurnInPetition(petition uint64, colours ...uint32) []byte {
	body := binary.LittleEndian.AppendUint64(nil, petition)
	for _, colour := range colours {
		body = binary.LittleEndian.AppendUint32(body, colour)
	}
	return body
}
