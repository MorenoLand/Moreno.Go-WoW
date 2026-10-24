package render

import (
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// worldSocial is the character's friends and ignored players.
type worldSocial struct {
	contacts map[uint64]world.Contact
	order    []uint64
	received bool
}

// handle takes a social packet; it reports whether the list changed.
func (social *worldSocial) handle(packet world.Packet) bool {
	switch packet.Opcode {
	case world.ContactList:
		contacts, err := world.ParseContactList(packet.Body)
		if err != nil && len(contacts) == 0 {
			return false
		}
		social.contacts = make(map[uint64]world.Contact, len(contacts))
		social.order = social.order[:0]
		for _, contact := range contacts {
			social.contacts[contact.GUID] = contact
			social.order = append(social.order, contact.GUID)
		}
		social.received = true
		return true
	case world.FriendStatus:
		change, err := world.ParseFriendStatus(packet.Body)
		if err != nil {
			return false
		}
		if social.contacts == nil {
			social.contacts = make(map[uint64]world.Contact)
		}
		contact, known := social.contacts[change.GUID]
		if !known {
			contact = world.Contact{GUID: change.GUID}
			social.order = append(social.order, change.GUID)
		}
		switch change.Result {
		case world.FriendOnline, world.FriendAddedOnline:
			contact.Flags |= world.ContactFriend
			contact.Status, contact.Area, contact.Level, contact.Class = change.Contact.Status, change.Contact.Area, change.Contact.Level, change.Contact.Class
			if change.Result == world.FriendAddedOnline {
				contact.Note = change.Contact.Note
			}
		case world.FriendOffline:
			contact.Status = 0
		case world.FriendAddedOffline:
			contact.Flags |= world.ContactFriend
			contact.Status, contact.Note = 0, change.Contact.Note
		case world.FriendIgnoreAdded:
			contact.Flags |= world.ContactIgnored
		case world.FriendRemoved, world.FriendIgnoreGone:
			delete(social.contacts, change.GUID)
			return true
		default:
			return false
		}
		social.contacts[change.GUID] = contact
		return true
	}
	return false
}

// push hands the interface the lists, asking for the names it lacks.
func (social *worldSocial) push(rt *ui.Runtime, loader *ui.Loader, names *worldNames, send func(world.ClientOpcode, []byte) error) {
	if !social.received {
		return
	}
	areas := worldAreas(loader)
	var entries []ui.ContactEntry
	for _, guid := range social.order {
		contact, ok := social.contacts[guid]
		if !ok {
			continue
		}
		name, known := names.players[guid]
		if !known {
			if !names.asked[guid] {
				names.asked[guid] = true
				_ = send(world.NameQueryOpcode, world.BuildNameQuery(guid))
			}
			continue
		}
		entry := ui.ContactEntry{GUID: guid, Name: name.Name, Ignore: contact.Flags&world.ContactIgnored != 0 && contact.Flags&world.ContactFriend == 0, Note: contact.Note, Status: contact.Status, Level: int(contact.Level), ClassFile: ui.ClassFileFromID(int(contact.Class))}
		if area, ok := areas.areas[contact.Area]; ok {
			entry.Area = area.name
		}
		entries = append(entries, entry)
	}
	rt.SetContacts(entries)
}

func (h *clientHost) RequestContacts() error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientContactList, world.BuildContactListRequest(7))
}

func (h *clientHost) AddFriend(name, note string) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientAddFriend, world.BuildAddContact(name, note, true))
}

func (h *clientHost) AddIgnore(name string) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientAddIgnore, world.BuildAddContact(name, "", false))
}

func (h *clientHost) RemoveContact(guid uint64, ignore bool) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	opcode := world.ClientDelFriend
	if ignore {
		opcode = world.ClientDelIgnore
	}
	return session.SendRaw(opcode, world.BuildContactGUID(guid))
}

func (h *clientHost) SetContactNote(guid uint64, note string) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientSetContactNotes, world.BuildContactNote(guid, note))
}
