package world

import (
	"encoding/binary"
	"fmt"
)

const (
	// The mailbox: the client opens it itself on seeing the object, asks for the
	// list, and every action names the mailbox object and the mail.
	ClientSendMail           ClientOpcode = 0x0238
	ClientGetMailList        ClientOpcode = 0x023A
	ClientMailTakeMoney      ClientOpcode = 0x0245
	ClientMailTakeItem       ClientOpcode = 0x0246
	ClientMailMarkAsRead     ClientOpcode = 0x0247
	ClientMailReturnToSender ClientOpcode = 0x0248
	ClientMailDelete         ClientOpcode = 0x0249
	ClientMailCreateTextItem ClientOpcode = 0x024A

	MailResult   uint16 = 0x0239
	// ShowMailbox is SMSG_SHOW_MAILBOX: the server opens the mailbox of a game object
	// the player used (body: its guid).
	ShowMailbox uint16 = 0x0297
	MailList     uint16 = 0x023B
	MailReceived uint16 = 0x0285
)

// Where a mail came from (MailMessageType).
const (
	MailFromPlayer     = 0
	MailFromAuction    = 2
	MailFromCreature   = 3
	MailFromGameObject = 4
	MailFromCalendar   = 5
)

// Bits of a mail's flags.
const (
	MailFlagRead    = 0x01
	MailFlagReturn  = 0x02
	MailFlagCopied  = 0x04
	MailFlagCODPaid = 0x08
	MailFlagBody    = 0x10
)

// What a mail result answers (MailResponseType).
const (
	MailCommandSend     = 0
	MailCommandMoney    = 1
	MailCommandItem     = 2
	MailCommandReturned = 3
	MailCommandDeleted  = 4
)

// Mail result codes (MailResponseResult).
const (
	MailOK                 = 0
	MailErrEquip           = 1
	MailErrToSelf          = 2
	MailErrNotEnoughMoney  = 3
	MailErrNotFound        = 4
	MailErrNotYourTeam     = 5
	MailErrInternal        = 6
	MailErrRecipientCap    = 15
	MailErrWrappedCOD      = 16
	MailErrTooMany         = 18
	MailErrInvalidAttach   = 19
	MailErrItemHasExpired  = 21
	mailEnchantSlotsInList = 7
)

// MailAttachment is an item a mail carries.
type MailAttachment struct {
	Position       uint8
	ID             uint32
	Entry          uint32
	Enchant        uint32
	RandomProperty int32
	RandomSeed     int32
	Count          int32
	Charges        int32
	MaxDurability  uint32
	Durability     int32
	Unlocked       bool
}

// MailMessage is one entry of the mailbox list.
type MailMessage struct {
	ID         uint32
	Kind       uint8
	Sender     uint64
	Alt        uint32
	COD        uint32
	Package    int32
	Stationery int32
	Money      uint32
	Flags      uint32
	DaysLeft   float32
	Template   int32
	Subject    string
	Body       string
	Items      []MailAttachment
}

// MailListing is SMSG_MAIL_LIST_RESULT.
type MailListing struct {
	Total int32
	Mails []MailMessage
}

// ParseMailList reads SMSG_MAIL_LIST_RESULT.
func ParseMailList(body []byte) (MailListing, error) {
	r := NewReader(body, "SMSG_MAIL_LIST_RESULT")
	var listing MailListing
	total, err := r.U32()
	if err != nil {
		return listing, err
	}
	listing.Total = int32(total)
	count, err := r.U8()
	if err != nil {
		return listing, err
	}
	for index := 0; index < int(count); index++ {
		start := len(body) - r.Remaining()
		size, err := r.U16()
		if err != nil {
			return listing, err
		}
		var mail MailMessage
		if mail.ID, err = r.U32(); err != nil {
			return listing, err
		}
		if mail.Kind, err = r.U8(); err != nil {
			return listing, err
		}
		if mail.Kind == MailFromPlayer {
			mail.Sender, err = r.U64()
		} else {
			mail.Alt, err = r.U32()
		}
		if err != nil {
			return listing, err
		}
		fields := []*uint32{&mail.COD, nil, nil, &mail.Money, &mail.Flags}
		for position, field := range fields {
			value, err := r.U32()
			if err != nil {
				return listing, err
			}
			switch position {
			case 1:
				mail.Package = int32(value)
			case 2:
				mail.Stationery = int32(value)
			default:
				*field = value
			}
		}
		if mail.DaysLeft, err = r.F32(); err != nil {
			return listing, err
		}
		template, err := r.U32()
		if err != nil {
			return listing, err
		}
		mail.Template = int32(template)
		if mail.Subject, err = r.CString(); err != nil {
			return listing, err
		}
		if mail.Body, err = r.CString(); err != nil {
			return listing, err
		}
		attached, err := r.U8()
		if err != nil {
			return listing, err
		}
		for item := 0; item < int(attached); item++ {
			attachment, err := parseMailAttachment(r)
			if err != nil {
				return listing, err
			}
			mail.Items = append(mail.Items, attachment)
		}
		if used := len(body) - r.Remaining() - start; used < int(size) {
			if err := r.Skip(int(size) - used); err != nil {
				return listing, err
			}
		}
		listing.Mails = append(listing.Mails, mail)
	}
	return listing, nil
}

func parseMailAttachment(r *Reader) (MailAttachment, error) {
	var item MailAttachment
	var err error
	if item.Position, err = r.U8(); err != nil {
		return item, err
	}
	words := make([]uint32, 2+mailEnchantSlotsInList*3+6)
	for index := range words {
		if words[index], err = r.U32(); err != nil {
			return item, err
		}
	}
	item.ID, item.Entry = words[0], words[1]
	item.Enchant = words[2]
	rest := words[2+mailEnchantSlotsInList*3:]
	item.RandomProperty, item.RandomSeed, item.Count, item.Charges = int32(rest[0]), int32(rest[1]), int32(rest[2]), int32(rest[3])
	item.MaxDurability, item.Durability = rest[4], int32(rest[5])
	unlocked, err := r.U8()
	item.Unlocked = unlocked != 0
	return item, err
}

// MailResultInfo is SMSG_SEND_MAIL_RESULT.
type MailResultInfo struct {
	MailID   uint32
	Command  uint32
	Error    uint32
	AttachID uint32
	Quantity uint32
}

// ParseMailResult reads SMSG_SEND_MAIL_RESULT.
func ParseMailResult(body []byte) (MailResultInfo, error) {
	r := NewReader(body, "SMSG_SEND_MAIL_RESULT")
	var info MailResultInfo
	var err error
	if info.MailID, err = r.U32(); err != nil {
		return info, err
	}
	if info.Command, err = r.U32(); err != nil {
		return info, err
	}
	if info.Error, err = r.U32(); err != nil {
		return info, err
	}
	if info.Error == MailErrEquip {
		if _, err = r.U32(); err != nil {
			return info, err
		}
	}
	if info.Command == MailCommandItem && (info.Error == MailOK || info.Error == MailErrItemHasExpired) {
		if info.AttachID, err = r.U32(); err != nil {
			return info, err
		}
		info.Quantity, err = r.U32()
	}
	return info, err
}

func appendU32(buffer []byte, value uint32) []byte {
	return binary.LittleEndian.AppendUint32(buffer, value)
}
func appendU64(buffer []byte, value uint64) []byte {
	return binary.LittleEndian.AppendUint64(buffer, value)
}
func appendString(buffer []byte, text string) []byte {
	return append(append(buffer, text...), 0)
}

// BuildMailbox is the body of the requests that only name the mailbox.
func BuildMailbox(mailbox uint64) []byte { return appendU64(nil, mailbox) }

// BuildMailAction is the body of the requests about one mail: mark as read,
// take the money, make a copy of its letter.
func BuildMailAction(mailbox uint64, mail uint32) []byte {
	return appendU32(appendU64(nil, mailbox), mail)
}

// BuildMailDelete deletes a mail.
func BuildMailDelete(mailbox uint64, mail uint32) []byte {
	return appendU32(BuildMailAction(mailbox, mail), 0)
}

// BuildMailTakeItem takes one attachment of a mail.
func BuildMailTakeItem(mailbox uint64, mail, attachment uint32) []byte {
	return appendU32(BuildMailAction(mailbox, mail), attachment)
}

// BuildMailReturn sends a mail back to its sender.
func BuildMailReturn(mailbox uint64, mail uint32, sender uint64) []byte {
	return appendU64(BuildMailAction(mailbox, mail), sender)
}

// MailAttachmentSend names an item of a mail being sent by its position and guid.
type MailAttachmentSend struct {
	Position uint8
	GUID     uint64
}

// BuildSendMail writes a mail.
func BuildSendMail(mailbox uint64, target, subject, body string, stationery, packageID uint32, items []MailAttachmentSend, money, cod uint32) []byte {
	buffer := appendU64(nil, mailbox)
	buffer = appendString(buffer, target)
	buffer = appendString(buffer, subject)
	buffer = appendString(buffer, body)
	buffer = appendU32(buffer, stationery)
	buffer = appendU32(buffer, packageID)
	buffer = append(buffer, byte(len(items)))
	for _, item := range items {
		buffer = append(buffer, item.Position)
		buffer = appendU64(buffer, item.GUID)
	}
	buffer = appendU32(buffer, money)
	buffer = appendU32(buffer, cod)
	buffer = appendU64(buffer, 0)
	return append(buffer, 0)
}

func (m MailMessage) String() string { return fmt.Sprintf("mail %d %q", m.ID, m.Subject) }
