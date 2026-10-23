package world

import "encoding/binary"

// CMSG_GUILD_QUERY asks for a guild's name; SMSG_GUILD_QUERY_RESPONSE answers.
const (
	GuildQueryOpcode   ClientOpcode = 0x0054
	GuildQueryResponse uint16       = 0x0055
)

// BuildGuildQuery is CMSG_GUILD_QUERY: the guild id.
func BuildGuildQuery(id uint32) []byte {
	body := make([]byte, 4)
	binary.LittleEndian.PutUint32(body, id)
	return body
}

// ParseGuildQueryName reads the id and name at the start of
// SMSG_GUILD_QUERY_RESPONSE; the rank names and emblem after them are not needed.
func ParseGuildQueryName(body []byte) (id uint32, name string, err error) {
	r := NewReader(body, "SMSG_GUILD_QUERY_RESPONSE")
	if id, err = r.U32(); err != nil {
		return
	}
	name, err = r.CString()
	return
}
