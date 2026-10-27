package world

import "encoding/binary"

// Game object queries.
const (
	GameObjectQueryOpcode   ClientOpcode = 0x005E
	GameObjectQueryResponse uint16       = 0x005F
)

// BuildGameObjectQuery is CMSG_GAMEOBJECT_QUERY: the entry and the object's GUID.
func BuildGameObjectQuery(entry uint32, guid uint64) []byte {
	body := make([]byte, 12)
	binary.LittleEndian.PutUint32(body, entry)
	binary.LittleEndian.PutUint64(body[4:], guid)
	return body
}

// GameObjectInfo is the part of SMSG_GAMEOBJECT_QUERY_RESPONSE the interface
// shows.
type GameObjectInfo struct {
	Entry     uint32
	Found     bool
	Type      uint32
	DisplayID uint32
	Name      string
}

// ParseGameObjectQuery reads SMSG_GAMEOBJECT_QUERY_RESPONSE: the entry (its high
// bit set when the object is unknown), then its type, display id and names.
func ParseGameObjectQuery(body []byte) (GameObjectInfo, error) {
	r := NewReader(body, "SMSG_GAMEOBJECT_QUERY_RESPONSE")
	var info GameObjectInfo
	entry, err := r.U32()
	if err != nil {
		return info, err
	}
	info.Entry = entry &^ 0x80000000
	if entry&0x80000000 != 0 {
		return info, nil
	}
	info.Found = true
	if info.Type, err = r.U32(); err != nil {
		return info, err
	}
	if info.DisplayID, err = r.U32(); err != nil {
		return info, err
	}
	info.Name, err = r.CString()
	return info, err
}
