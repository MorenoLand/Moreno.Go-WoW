package world

const (
	// ContactList (SMSG_CONTACT_LIST) holds the friends, ignored and muted
	// players; FriendStatus (SMSG_FRIEND_STATUS) changes one.
	ContactList  uint16 = 0x0067
	FriendStatus uint16 = 0x0068

	ClientContactList     ClientOpcode = 0x0066
	ClientAddFriend       ClientOpcode = 0x0069
	ClientDelFriend       ClientOpcode = 0x006A
	ClientSetContactNotes ClientOpcode = 0x006B
	ClientAddIgnore       ClientOpcode = 0x006C
	ClientDelIgnore       ClientOpcode = 0x006D
)

// Contact flags.
const (
	ContactFriend  uint32 = 0x01
	ContactIgnored uint32 = 0x02
	ContactMuted   uint32 = 0x04
)

// Friend results of SMSG_FRIEND_STATUS.
const (
	FriendOnline       uint8 = 0x02
	FriendOffline      uint8 = 0x03
	FriendRemoved      uint8 = 0x05
	FriendAddedOnline  uint8 = 0x06
	FriendAddedOffline uint8 = 0x07
	FriendIgnoreAdded  uint8 = 0x0F
	FriendIgnoreGone   uint8 = 0x10
)

// Contact is a player on the character's social list.
type Contact struct {
	GUID  uint64
	Flags uint32
	Note  string
	// Status is 0 offline, 1 online, 2 away, 3 busy.
	Status uint8
	Area   uint32
	Level  uint32
	Class  uint32
}

// ParseContactList reads SMSG_CONTACT_LIST.
func ParseContactList(body []byte) ([]Contact, error) {
	r := NewReader(body, "SMSG_CONTACT_LIST")
	if _, err := r.U32(); err != nil {
		return nil, err
	}
	count, err := r.U32()
	if err != nil {
		return nil, err
	}
	if count > 1024 {
		count = 1024
	}
	contacts := make([]Contact, 0, count)
	for index := uint32(0); index < count; index++ {
		var contact Contact
		if contact.GUID, err = r.U64(); err != nil {
			return contacts, err
		}
		if contact.Flags, err = r.U32(); err != nil {
			return contacts, err
		}
		if contact.Note, err = r.CString(); err != nil {
			return contacts, err
		}
		if contact.Flags&ContactFriend != 0 {
			if contact.Status, err = r.U8(); err != nil {
				return contacts, err
			}
			if contact.Status != 0 {
				if contact.Area, err = r.U32(); err != nil {
					return contacts, err
				}
				if contact.Level, err = r.U32(); err != nil {
					return contacts, err
				}
				if contact.Class, err = r.U32(); err != nil {
					return contacts, err
				}
			}
		}
		contacts = append(contacts, contact)
	}
	return contacts, nil
}

// FriendChange is what SMSG_FRIEND_STATUS reports about one player.
type FriendChange struct {
	Result  uint8
	GUID    uint64
	Contact Contact
}

// ParseFriendStatus reads SMSG_FRIEND_STATUS.
func ParseFriendStatus(body []byte) (FriendChange, error) {
	r := NewReader(body, "SMSG_FRIEND_STATUS")
	var change FriendChange
	var err error
	if change.Result, err = r.U8(); err != nil {
		return change, err
	}
	if change.GUID, err = r.U64(); err != nil {
		return change, err
	}
	change.Contact.GUID = change.GUID
	if change.Result == FriendAddedOnline || change.Result == FriendAddedOffline {
		if change.Contact.Note, err = r.CString(); err != nil {
			return change, err
		}
	}
	if change.Result == FriendAddedOnline || change.Result == FriendOnline {
		if change.Contact.Status, err = r.U8(); err != nil {
			return change, err
		}
		if change.Contact.Area, err = r.U32(); err != nil {
			return change, err
		}
		if change.Contact.Level, err = r.U32(); err != nil {
			return change, err
		}
		if change.Contact.Class, err = r.U32(); err != nil {
			return change, err
		}
	}
	return change, nil
}

// BuildContactListRequest is the body of CMSG_CONTACT_LIST: which lists to send.
func BuildContactListRequest(flags uint32) []byte {
	return []byte{byte(flags), byte(flags >> 8), byte(flags >> 16), byte(flags >> 24)}
}

// BuildAddContact is the body of CMSG_ADD_FRIEND (name and note) and, without
// a note, of CMSG_ADD_IGNORE.
func BuildAddContact(name, note string, withNote bool) []byte {
	body := append([]byte(name), 0)
	if withNote {
		body = append(append(body, note...), 0)
	}
	return body
}

// BuildContactGUID is the body of CMSG_DEL_FRIEND and CMSG_DEL_IGNORE.
func BuildContactGUID(guid uint64) []byte {
	body := make([]byte, 8)
	for index := range body {
		body[index] = byte(guid >> (8 * index))
	}
	return body
}

// BuildContactNote is the body of CMSG_SET_CONTACT_NOTES.
func BuildContactNote(guid uint64, note string) []byte {
	return append(append(BuildContactGUID(guid), note...), 0)
}
