package world

import "encoding/binary"

// Emote packets.
const (
	EmotePacket     uint16 = 0x0103
	TextEmotePacket uint16 = 0x0105

	ClientTextEmote ClientOpcode = 0x0104
)

// TextEmote is SMSG_TEXT_EMOTE: a player doing a text emote, with the name of
// whom it was aimed at.
type TextEmote struct {
	Sender uint64
	Emote  uint32
	Number uint32
	Target string
}

// ParseTextEmote reads SMSG_TEXT_EMOTE.
func ParseTextEmote(body []byte) (TextEmote, error) {
	r := NewReader(body, "SMSG_TEXT_EMOTE")
	var emote TextEmote
	var err error
	if emote.Sender, err = r.U64(); err != nil {
		return emote, err
	}
	if emote.Emote, err = r.U32(); err != nil {
		return emote, err
	}
	if emote.Number, err = r.U32(); err != nil {
		return emote, err
	}
	if _, err = r.U32(); err != nil {
		return emote, err
	}
	emote.Target, err = r.CString()
	return emote, err
}

// BuildTextEmote is the body of CMSG_TEXT_EMOTE.
func BuildTextEmote(emote, number uint32, target uint64) []byte {
	body := binary.LittleEndian.AppendUint32(nil, emote)
	body = binary.LittleEndian.AppendUint32(body, number)
	return binary.LittleEndian.AppendUint64(body, target)
}
