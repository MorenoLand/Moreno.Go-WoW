package ui

import (
	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// MailHost is what the mailbox asks of the connection.
type MailHost interface {
	RequestMailList(mailbox uint64) error
	MarkMailRead(mailbox uint64, mail uint32) error
	TakeMailMoney(mailbox uint64, mail uint32) error
	TakeMailItem(mailbox uint64, mail, attachment uint32) error
	DeleteMail(mailbox uint64, mail uint32) error
	ReturnMail(mailbox uint64, mail uint32, sender uint64) error
	CopyMailLetter(mailbox uint64, mail uint32) error
	SendMail(mailbox uint64, target, subject, body string, stationery uint32, items []world.MailAttachmentSend, money, cod uint32) error
	// MailSenderName names who sent a mail; empty until it is known.
	MailSenderName(kind uint8, id uint64) string
	// MailStationery gives the item whose icon marks a stationery and the name
	// of its letter texture.
	MailStationery(id int32) (item uint32, texture string)
}

// mailAttachmentSlots is how many items a mail being written holds.
const mailAttachmentSlots = 12

type mailAttachment struct {
	guid  uint64
	entry uint32
	count int
}

type mailState struct {
	open    bool
	mailbox uint64
	mails   []world.MailMessage
	total   int
	// hasNew is whether unread mail waits.
	hasNew bool
	// Mail being written.
	showingSend bool
	attached    [mailAttachmentSlots]mailAttachment
	money, cod  uint32
}

func (rt *Runtime) mailHost() MailHost {
	host, _ := rt.Host.(MailHost)
	return host
}

// OpenMailbox shows the mailbox object the player clicked.
func (rt *Runtime) OpenMailbox(mailbox uint64) {
	rt.mail.open, rt.mail.mailbox = true, mailbox
	rt.mail.mails, rt.mail.total = nil, 0
	rt.FireEvent("MAIL_SHOW")
}

// CloseMailbox closes the mailbox window.
func (rt *Runtime) CloseMailbox() {
	if !rt.mail.open {
		return
	}
	rt.mail.open = false
	rt.mail.showingSend = false
	rt.FireEvent("MAIL_CLOSED")
}

// MailboxOpen reports whether a mailbox is showing.
func (rt *Runtime) MailboxOpen() bool { return rt.mail.open }

// SetMailList installs the mailbox list the server sent.
func (rt *Runtime) SetMailList(listing world.MailListing) {
	rt.mail.mails, rt.mail.total = listing.Mails, int(listing.Total)
	rt.mail.hasNew = false
	for _, mail := range listing.Mails {
		if mail.Flags&world.MailFlagRead == 0 {
			rt.mail.hasNew = true
		}
		for _, item := range mail.Items {
			rt.itemTemplate(item.Entry)
		}
	}
	rt.FireEvent("MAIL_INBOX_UPDATE")
	rt.FireEvent("UPDATE_PENDING_MAIL")
}

// SetPendingMail records whether the server says mail waits to be read.
func (rt *Runtime) SetPendingMail(waiting bool) {
	if rt.mail.hasNew == waiting {
		return
	}
	rt.mail.hasNew = waiting
	rt.FireEvent("UPDATE_PENDING_MAIL")
}

// RefreshMail redraws the open mailbox after names or items arrived.
func (rt *Runtime) RefreshMail() {
	if rt.mail.open {
		rt.FireEvent("MAIL_INBOX_UPDATE")
	}
}

// ApplyMailResult applies the server's answer to a mail action.
func (rt *Runtime) ApplyMailResult(result world.MailResultInfo) {
	failed := result.Error != world.MailOK
	if result.Command == world.MailCommandSend {
		if failed {
			rt.FireEvent("MAIL_FAILED")
			rt.ShowGlobalError(mailErrorName(result.Error))
			return
		}
		rt.mail.attached = [mailAttachmentSlots]mailAttachment{}
		rt.mail.money, rt.mail.cod = 0, 0
		rt.FireEvent("MAIL_SEND_SUCCESS")
		rt.ShowGlobalError("ERR_MAIL_SENT")
		return
	}
	index := rt.mailIndexByID(result.MailID)
	if failed && result.Error != world.MailErrItemHasExpired {
		rt.FireEvent("MAIL_FAILED")
		rt.ShowGlobalError(mailErrorName(result.Error))
		return
	}
	if index < 0 {
		return
	}
	mail := &rt.mail.mails[index]
	switch result.Command {
	case world.MailCommandMoney:
		mail.Money = 0
	case world.MailCommandItem:
		items := mail.Items[:0:0]
		for _, item := range mail.Items {
			if item.ID != result.AttachID {
				items = append(items, item)
			}
		}
		mail.Items = items
		mail.COD = 0
	case world.MailCommandReturned, world.MailCommandDeleted:
		rt.mail.mails = append(rt.mail.mails[:index:index], rt.mail.mails[index+1:]...)
		rt.mail.total--
		rt.FireEvent("CLOSE_INBOX_ITEM", numberValue(index+1))
	}
	rt.FireEvent("MAIL_INBOX_UPDATE")
	rt.FireEvent("MAIL_SUCCESS")
}

func mailErrorName(code uint32) string {
	switch code {
	case world.MailErrToSelf:
		return "ERR_MAIL_TO_SELF"
	case world.MailErrNotEnoughMoney:
		return "ERR_NOT_ENOUGH_MONEY"
	case world.MailErrNotFound:
		return "ERR_MAIL_TARGET_NOT_FOUND"
	case world.MailErrNotYourTeam:
		return "ERR_NOT_YOUR_TEAM"
	case world.MailErrRecipientCap:
		return "ERR_MAIL_REACHED_CAP"
	case world.MailErrWrappedCOD:
		return "ERR_MAIL_WRAPPED_COD"
	case world.MailErrTooMany:
		return "ERR_MAIL_TOO_MANY_ATTACHMENTS"
	case world.MailErrInvalidAttach:
		return "ERR_MAIL_INVALID_ATTACHMENT"
	case world.MailErrItemHasExpired:
		return "ERR_MAIL_ATTACHMENT_EXPIRED"
	}
	return "ERR_MAIL_DATABASE_ERROR"
}

func (rt *Runtime) mailIndexByID(id uint32) int {
	for index, mail := range rt.mail.mails {
		if mail.ID == id {
			return index
		}
	}
	return -1
}

// mailAt is the mail at a 1-based inbox index.
func (rt *Runtime) mailAt(index int) *world.MailMessage {
	if index < 1 || index > len(rt.mail.mails) {
		return nil
	}
	return &rt.mail.mails[index-1]
}

// mailCanDelete tells whether a mail is deleted rather than returned: only mail
// a player sent and nobody returned goes back to its sender.
func mailCanDelete(mail *world.MailMessage) bool {
	return mail.Kind != world.MailFromPlayer || mail.Flags&world.MailFlagReturn != 0 || mail.Stationery == mailStationeryGM
}

// mailStationeryGM is the stationery of messages from the game masters.
const mailStationeryGM = 61

// mailPackageDisplay is the item display whose icon marks a mail with several
// attachments.
const mailPackageDisplay = 7913

// mailAttachmentAt finds an attachment by its 1-based position.
func mailAttachmentAt(mail *world.MailMessage, position int) *world.MailAttachment {
	for index := range mail.Items {
		if int(mail.Items[index].Position) == position-1 {
			return &mail.Items[index]
		}
	}
	return nil
}

// sendAttachmentFree is the first empty square of the mail being written.
func (rt *Runtime) sendAttachmentFree() int {
	for index := range rt.mail.attached {
		if rt.mail.attached[index].guid == 0 {
			return index
		}
	}
	return -1
}

func (rt *Runtime) sendAttachmentHolds(guid uint64) bool {
	for _, attached := range rt.mail.attached {
		if attached.guid == guid {
			return true
		}
	}
	return false
}

// registerMailAPI defines the calls the mail windows make.
func registerMailAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	state := &rt.mail
	reg("GetInboxNumItems", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(state.mails)))
		L.Push(lua.LNumber(max(state.total, len(state.mails))))
		return 2
	})
	reg("GetInboxHeaderInfo", func(L *lua.LState) int {
		mail := rt.mailAt(L.CheckInt(1))
		if mail == nil {
			return 0
		}
		host := rt.mailHost()
		sender, stationeryItem := lua.LValue(lua.LNil), uint32(0)
		if host != nil {
			id := mail.Sender
			if mail.Kind != world.MailFromPlayer {
				id = uint64(mail.Alt)
			}
			if name := host.MailSenderName(mail.Kind, id); name != "" {
				sender = lua.LString(name)
			}
			stationeryItem, _ = host.MailStationery(mail.Stationery)
		}
		stationeryIcon := rt.itemIcon(stationeryItem)
		// The package icon is the one item's own, or the generic package when
		// there are several; with nothing attached there is none, and the list
		// shows the stationery's.
		var packageIcon lua.LValue = lua.LNil
		firstCount := lua.LValue(lua.LNil)
		switch len(mail.Items) {
		case 0:
		case 1:
			if template := rt.itemTemplate(mail.Items[0].Entry); template != nil {
				packageIcon = lua.LString(rt.itemIcon(mail.Items[0].Entry))
			}
			firstCount = lua.LNumber(mail.Items[0].Count)
		default:
			if icon := rt.inventory.icons[mailPackageDisplay]; icon != "" {
				packageIcon = lua.LString(`Interface\Icons\` + icon)
			}
		}
		subject := mail.Subject
		if mail.Flags&world.MailFlagCODPaid != 0 {
			subject = rt.globalString("COD_PAYMENT", subject)
		}
		flag := func(set bool) lua.LValue {
			if set {
				return lua.LTrue
			}
			return lua.LNil
		}
		itemCount := lua.LValue(lua.LNil)
		if len(mail.Items) > 0 {
			itemCount = lua.LNumber(len(mail.Items))
		}
		L.Push(packageIcon)
		L.Push(lua.LString(stationeryIcon))
		L.Push(sender)
		L.Push(lua.LString(subject))
		L.Push(lua.LNumber(mail.Money))
		L.Push(lua.LNumber(mail.COD))
		L.Push(lua.LNumber(mail.DaysLeft))
		L.Push(itemCount)
		L.Push(flag(mail.Flags&world.MailFlagRead != 0))
		L.Push(flag(mail.Flags&(world.MailFlagReturn|0x200) != 0))
		L.Push(flag(mail.Flags&world.MailFlagCopied != 0))
		L.Push(flag(mail.Kind == world.MailFromPlayer && mail.Stationery != mailStationeryGM))
		L.Push(flag(mail.Stationery == mailStationeryGM))
		L.Push(firstCount)
		return 14
	})
	reg("GetInboxText", func(L *lua.LState) int {
		mail := rt.mailAt(L.CheckInt(1))
		if mail == nil {
			return 0
		}
		host := rt.mailHost()
		texture := ""
		if host != nil {
			_, texture = host.MailStationery(mail.Stationery)
			if mail.Flags&world.MailFlagRead == 0 {
				mail.Flags |= world.MailFlagRead
				_ = host.MarkMailRead(state.mailbox, mail.ID)
			}
		}
		L.Push(lua.LString(mail.Body))
		if texture != "" {
			L.Push(lua.LString(texture))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LBool(mail.Flags&world.MailFlagBody != 0 && mail.Flags&world.MailFlagCopied == 0))
		L.Push(lua.LBool(mail.Kind == world.MailFromAuction))
		return 4
	})
	reg("GetInboxInvoiceInfo", func(L *lua.LState) int { return 0 })
	reg("GetInboxItem", func(L *lua.LState) int {
		mail := rt.mailAt(L.CheckInt(1))
		if mail == nil {
			return 0
		}
		item := mailAttachmentAt(mail, L.OptInt(2, 1))
		if item == nil {
			return 0
		}
		name, quality := "", lua.LValue(lua.LNil)
		if template := rt.itemTemplate(item.Entry); template != nil {
			name, quality = template.Name, lua.LNumber(template.Quality)
		}
		L.Push(lua.LString(name))
		L.Push(lua.LString(rt.itemIcon(item.Entry)))
		L.Push(lua.LNumber(item.Count))
		L.Push(quality)
		L.Push(lua.LTrue)
		return 5
	})
	reg("GetInboxItemLink", func(L *lua.LState) int {
		if mail := rt.mailAt(L.CheckInt(1)); mail != nil {
			if item := mailAttachmentAt(mail, L.OptInt(2, 1)); item != nil {
				if link := rt.itemLinkFor(item.Entry, item.Enchant); link != "" {
					L.Push(lua.LString(link))
					return 1
				}
			}
		}
		return 0
	})
	reg("InboxItemCanDelete", func(L *lua.LState) int {
		mail := rt.mailAt(L.CheckInt(1))
		L.Push(lua.LBool(mail != nil && mailCanDelete(mail)))
		return 1
	})
	reg("CanComplainInboxItem", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("GetLatestThreeSenders", func(L *lua.LState) int { return 0 })
	reg("HasNewMail", func(L *lua.LState) int { L.Push(lua.LBool(state.hasNew)); return 1 })
	reg("CheckInbox", func(L *lua.LState) int {
		if host := rt.mailHost(); host != nil && state.open {
			_ = host.RequestMailList(state.mailbox)
		}
		return 0
	})
	reg("CloseMail", func(L *lua.LState) int {
		rt.CloseMailbox()
		return 0
	})
	reg("TakeInboxItem", func(L *lua.LState) int {
		mail := rt.mailAt(L.CheckInt(1))
		host := rt.mailHost()
		if mail == nil || host == nil {
			return 0
		}
		if item := mailAttachmentAt(mail, L.OptInt(2, 1)); item != nil {
			_ = host.TakeMailItem(state.mailbox, mail.ID, item.ID)
		}
		return 0
	})
	reg("TakeInboxMoney", func(L *lua.LState) int {
		if mail, host := rt.mailAt(L.CheckInt(1)), rt.mailHost(); mail != nil && host != nil {
			_ = host.TakeMailMoney(state.mailbox, mail.ID)
		}
		return 0
	})
	reg("AutoLootMailItem", func(L *lua.LState) int {
		mail, host := rt.mailAt(L.CheckInt(1)), rt.mailHost()
		if mail == nil || host == nil {
			return 0
		}
		if mail.Money > 0 {
			_ = host.TakeMailMoney(state.mailbox, mail.ID)
		}
		for _, item := range mail.Items {
			_ = host.TakeMailItem(state.mailbox, mail.ID, item.ID)
		}
		return 0
	})
	reg("TakeInboxTextItem", func(L *lua.LState) int {
		if mail, host := rt.mailAt(L.CheckInt(1)), rt.mailHost(); mail != nil && host != nil {
			mail.Flags |= world.MailFlagCopied
			_ = host.CopyMailLetter(state.mailbox, mail.ID)
		}
		return 0
	})
	reg("DeleteInboxItem", func(L *lua.LState) int {
		if mail, host := rt.mailAt(L.CheckInt(1)), rt.mailHost(); mail != nil && host != nil {
			_ = host.DeleteMail(state.mailbox, mail.ID)
		}
		return 0
	})
	reg("ReturnInboxItem", func(L *lua.LState) int {
		if mail, host := rt.mailAt(L.CheckInt(1)), rt.mailHost(); mail != nil && host != nil {
			_ = host.ReturnMail(state.mailbox, mail.ID, mail.Sender)
		}
		return 0
	})
	reg("GetCoinIcon", func(L *lua.LState) int {
		L.Push(lua.LString(moneyIcon(uint32(L.CheckNumber(1)))))
		return 1
	})

	// Writing mail.
	reg("SetSendMailShowing", func(L *lua.LState) int {
		state.showingSend = L.OptBool(1, false)
		return 0
	})
	reg("GetSendMailItem", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		if index < 0 || index >= mailAttachmentSlots || state.attached[index].guid == 0 {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNumber(0))
			L.Push(lua.LNil)
			return 4
		}
		attached := state.attached[index]
		name, quality := "", lua.LValue(lua.LNil)
		if template := rt.itemTemplate(attached.entry); template != nil {
			name, quality = template.Name, lua.LNumber(template.Quality)
		}
		L.Push(lua.LString(name))
		L.Push(lua.LString(rt.itemIcon(attached.entry)))
		L.Push(lua.LNumber(attached.count))
		L.Push(quality)
		return 4
	})
	reg("GetSendMailItemLink", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		if index >= 0 && index < mailAttachmentSlots && state.attached[index].guid != 0 {
			if link := rt.itemLinkFor(state.attached[index].entry, 0); link != "" {
				L.Push(lua.LString(link))
				return 1
			}
		}
		return 0
	})
	reg("ClickSendMailItemButton", func(L *lua.LState) int {
		slot := L.OptInt(1, 0) - 1
		rightClick := L.OptBool(2, false)
		cursor := &rt.inventory.cursor
		if slot >= 0 && slot < mailAttachmentSlots && state.attached[slot].guid != 0 && (!cursor.active || rightClick) {
			state.attached[slot] = mailAttachment{}
			rt.FireEvent("MAIL_SEND_INFO_UPDATE")
			return 0
		}
		if !cursor.active {
			return 0
		}
		item := rt.inventory.items[cursor.guid]
		if item == nil || rt.sendAttachmentHolds(item.GUID) {
			return 0
		}
		if slot < 0 || slot >= mailAttachmentSlots || state.attached[slot].guid != 0 {
			slot = rt.sendAttachmentFree()
		}
		if slot < 0 {
			return 0
		}
		state.attached[slot] = mailAttachment{guid: item.GUID, entry: item.Entry, count: max(item.Count, 1)}
		if rt.clearItemCursor != nil {
			rt.clearItemCursor()
		}
		rt.FireEvent("MAIL_SEND_INFO_UPDATE")
		return 0
	})
	reg("GetSendMailMoney", func(L *lua.LState) int { L.Push(lua.LNumber(state.money)); return 1 })
	reg("GetSendMailCOD", func(L *lua.LState) int { L.Push(lua.LNumber(state.cod)); return 1 })
	reg("SetSendMailMoney", func(L *lua.LState) int {
		state.money, state.cod = uint32(max(L.CheckNumber(1), 0)), 0
		rt.FireEvent("MAIL_SEND_INFO_UPDATE")
		return 0
	})
	reg("SetSendMailCOD", func(L *lua.LState) int {
		state.cod, state.money = uint32(max(L.CheckNumber(1), 0)), 0
		rt.FireEvent("MAIL_SEND_INFO_UPDATE")
		return 0
	})
	reg("GetSendMailPrice", func(L *lua.LState) int {
		// The postage the server charges: 30 copper for each item, at least one.
		count := 0
		for _, attached := range state.attached {
			if attached.guid != 0 {
				count++
			}
		}
		L.Push(lua.LNumber(30 * max(count, 1)))
		return 1
	})
	reg("SendMail", func(L *lua.LState) int {
		target, subject, body := L.CheckString(1), L.CheckString(2), L.OptString(3, "")
		host := rt.mailHost()
		if host == nil || !state.open {
			return 0
		}
		var items []world.MailAttachmentSend
		for index, attached := range state.attached {
			if attached.guid != 0 {
				items = append(items, world.MailAttachmentSend{Position: uint8(len(items)), GUID: attached.guid})
				_ = index
			}
		}
		stationery := uint32(41)
		if err := host.SendMail(state.mailbox, target, subject, body, stationery, items, state.money, state.cod); err != nil {
			rt.FireEvent("MAIL_FAILED")
		}
		return 0
	})
}
