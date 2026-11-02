package world

import "encoding/binary"

// Readable text: books, letters and signs.
const (
	PageTextQueryResponse  uint16       = 0x005B
	ReadItemOKPacket       uint16       = 0x00AE
	ReadItemFailedPacket   uint16       = 0x00AF
	GameObjectPageTextPack uint16       = 0x01DF
	ClientPageTextQuery    ClientOpcode = 0x005A
	ClientReadItem         ClientOpcode = 0x00AD
)

// PageText is SMSG_PAGE_TEXT_QUERY_RESPONSE: one page and the page after it.
type PageText struct {
	ID, Next uint32
	Text     string
}

// ParsePageText reads SMSG_PAGE_TEXT_QUERY_RESPONSE.
func ParsePageText(body []byte) (PageText, error) {
	r := NewReader(body, "SMSG_PAGE_TEXT_QUERY_RESPONSE")
	var page PageText
	var err error
	if page.ID, err = r.U32(); err != nil {
		return page, err
	}
	if page.Text, err = r.CString(); err != nil {
		return page, err
	}
	page.Next, err = r.U32()
	return page, err
}

// BuildPageTextQuery is the body of CMSG_PAGE_TEXT_QUERY.
func BuildPageTextQuery(page uint32, guid uint64) []byte {
	return binary.LittleEndian.AppendUint64(binary.LittleEndian.AppendUint32(nil, page), guid)
}

// BuildReadItem is the body of CMSG_READ_ITEM.
func BuildReadItem(bag, slot uint8) []byte { return []byte{bag, slot} }
