package ui

import (
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// calRead reads a calendar packet; the first field it cannot read ends it.
type calRead struct {
	r   *world.Reader
	bad bool
}

func (c *calRead) u8() uint8 {
	v, err := c.r.U8()
	if err != nil {
		c.bad = true
	}
	return v
}

func (c *calRead) u32() uint32 {
	v, err := c.r.U32()
	if err != nil {
		c.bad = true
	}
	return v
}

func (c *calRead) u64() uint64 {
	v, err := c.r.U64()
	if err != nil {
		c.bad = true
	}
	return v
}

func (c *calRead) str() string {
	v, err := c.r.CString()
	if err != nil {
		c.bad = true
	}
	return v
}

func (c *calRead) guid() uint64 {
	v, err := world.ReadPackedGUID(c.r)
	if err != nil {
		c.bad = true
	}
	return v
}

// handleCalendarPacket takes the packets of the calendar.
func (rt *Runtime) handleCalendarPacket(packet world.Packet) bool {
	switch packet.Opcode {
	case calendarCommandResult:
		rt.calendarCommandResult(&calRead{r: world.NewReader(packet.Body, "SMSG_CALENDAR_COMMAND_RESULT")})
		return true
	case calendarSendCalendar, calendarSendEvent, calendarFilterGuild, calendarArenaTeam, calendarEventInvite,
		calendarInviteRemoved, calendarEventStatus, calendarLockoutAdded, calendarLockoutRemoved, calendarInviteAlert,
		calendarRemovedAlert, calendarStatusAlert, calendarEventRemoved, calendarEventUpdated, calendarModeratorAlert,
		calendarSendNumPending, calendarInviteNotes, calendarNotesAlert, calendarLockoutUpdated, calendarClearPending:
	default:
		return false
	}
	c := &calRead{r: world.NewReader(packet.Body, "SMSG_CALENDAR")}
	// answered is cleared by the packets that tell of something others did;
	// they do not end the wait for the answer to an action of our own.
	answered := true
	switch packet.Opcode {
	case calendarSendCalendar:
		rt.calendarReceive(c)
	case calendarSendEvent:
		rt.calendarReceiveEvent(c)
	case calendarFilterGuild, calendarArenaTeam:
		rt.calendarReceiveMassInvite(c)
	case calendarEventInvite:
		answered = rt.calendarInviteAdded(c)
	case calendarInviteRemoved:
		answered = rt.calendarInviteGone(c)
	case calendarEventStatus:
		answered = rt.calendarInviteStatus(c)
	case calendarLockoutAdded:
		rt.calendarLockoutAdd(c)
	case calendarLockoutRemoved:
		rt.calendarLockoutRemove(c)
	case calendarInviteAlert:
		rt.calendarInviteAlert(c)
		return true
	case calendarRemovedAlert:
		rt.calendarRemovedAlert(c)
		return true
	case calendarStatusAlert:
		rt.calendarStatusAlert(c)
		return true
	case calendarEventRemoved:
		answered = rt.calendarEventGone(c)
	case calendarEventUpdated:
		answered = rt.calendarEventUpdate(c)
	case calendarModeratorAlert:
		answered = rt.calendarModeratorSet(c)
	case calendarSendNumPending:
		before := rt.cal.serverPending
		rt.cal.serverPending = int(c.u32())
		if rt.cal.serverPending != before {
			rt.FireEvent("CALENDAR_UPDATE_PENDING_INVITES")
		}
	case calendarInviteNotes:
		answered = rt.calendarNotesSet(c, true)
	case calendarNotesAlert:
		rt.calendarNotesSet(c, false)
		return true
	case calendarLockoutUpdated:
		rt.calendarLockoutUpdate(c)
	}
	if answered {
		rt.calendarSetAction(false)
	}
	return true
}

// calendarCommandResult shows why the server refused something.
func (rt *Runtime) calendarCommandResult(c *calRead) {
	c.u32()
	c.str()
	name := c.str()
	result := c.u32()
	if c.bad {
		return
	}
	switch result {
	case 1:
		rt.calendarError("CALENDAR_ERROR_GUILD_EVENTS_EXCEEDED", 100)
	case 2:
		rt.calendarError("CALENDAR_ERROR_EVENTS_EXCEEDED", 30)
	case 3:
		rt.calendarError("CALENDAR_ERROR_SELF_INVITES_EXCEEDED", nil)
	case 4:
		rt.calendarError("CALENDAR_ERROR_OTHER_INVITES_EXCEEDED", name)
	case 5:
		rt.calendarError("CALENDAR_ERROR_PERMISSIONS", nil)
	case 6:
		rt.calendarError("CALENDAR_ERROR_EVENT_INVALID", nil)
	case 7:
		rt.calendarError("CALENDAR_ERROR_NOT_INVITED", nil)
	case 8:
		rt.calendarError("CALENDAR_ERROR_INTERNAL", nil)
	case 9:
		rt.calendarError("ERR_GUILD_PLAYER_NOT_IN_GUILD", nil)
	case 10:
		rt.calendarError("CALENDAR_ERROR_ALREADY_INVITED_TO_EVENT_S", name)
	case 11:
		rt.calendarError("PLAYER_NOT_FOUND", nil)
	case 12:
		rt.calendarError("CALENDAR_ERROR_NOT_ALLIED", nil)
	case 13:
		rt.calendarError("ERR_IGNORING_YOU_S", name)
	case 14:
		rt.calendarError("CALENDAR_ERROR_INVITES_EXCEEDED", 100)
	case 16:
		rt.calendarError("CALENDAR_ERROR_INVALID_DATE", nil)
	case 17:
		rt.calendarError("CALENDAR_ERROR_INVALID_TIME", nil)
	case 19:
		rt.calendarError("CALENDAR_ERROR_NEEDS_TITLE", nil)
	case 20:
		rt.calendarError("CALENDAR_ERROR_EVENT_PASSED", nil)
	case 21:
		rt.calendarError("CALENDAR_ERROR_EVENT_LOCKED", nil)
	case 22:
		rt.calendarError("CALENDAR_ERROR_DELETE_CREATOR_FAILED", nil)
	case 24:
		rt.calendarError("ERR_SYSTEM_DISABLED", nil)
	case 25:
		rt.calendarError("ERR_RESTRICTED_ACCOUNT", nil)
	case 26:
		rt.calendarError("CALENDAR_ERROR_ARENA_EVENTS_EXCEEDED", nil)
	case 27:
		rt.calendarError("CALENDAR_ERROR_RESTRICTED_LEVEL", nil)
	case 28:
		rt.calendarError("ERR_USER_SQUELCHED", nil)
	case 29:
		rt.calendarError("CALENDAR_ERROR_NO_INVITE", nil)
	case 36:
		rt.calendarError("CALENDAR_ERROR_EVENT_WRONG_SERVER", nil)
	case 37:
		rt.calendarError("CALENDAR_ERROR_INVITE_WRONG_SERVER", nil)
	case 38:
		rt.calendarError("CALENDAR_ERROR_NO_GUILD_INVITES", nil)
	case 39:
		rt.calendarError("CALENDAR_ERROR_INVALID_SIGNUP", nil)
	case 40:
		rt.calendarError("CALENDAR_ERROR_NO_MODERATOR", nil)
	}
}

// calendarReceive takes the whole calendar of the character.
func (rt *Runtime) calendarReceive(c *calRead) {
	data := &rt.cal
	me := rt.playerGUID()
	data.invites = data.invites[:0]
	for count, i := c.u32(), uint32(0); i < count && !c.bad; i++ {
		invite := &calPending{eventID: c.u64(), inviteID: c.u64(), status: c.u8(), rank: c.u8(), guildFlag: c.u8()}
		invite.inviter = c.guid()
		data.invites = append(data.invites, invite)
	}
	data.events = data.events[:0]
	for count, i := c.u32(), uint32(0); i < count && !c.bad; i++ {
		entry := &calEntry{id: c.u64(), title: c.str(), eventType: int(int32(c.u32()))}
		entry.start = unpackCalTime(c.u32())
		entry.flags = c.u32()
		entry.dungeon = int(int32(c.u32()))
		entry.creator = c.guid()
		if c.bad {
			break
		}
		entry.mod = 0
		if entry.creator == me {
			entry.mod = calModCreator
		}
		if entry.flags&calFlagGuildAnnounce != 0 {
			data.events = append(data.events, entry)
			continue
		}
		node := rt.calendarFindInvite(entry.id)
		if node == nil {
			if entry.flags&calFlagGuildEvent == 0 {
				continue
			}
			entry.mod |= calModSignUp
			entry.status = calStatusNotSigned
			data.events = append(data.events, entry)
			continue
		}
		entry.status = node.status
		entry.inviteID = node.inviteID
		if node.rank != 0 {
			entry.mod |= calModModerator
			if node.rank == 2 {
				entry.mod = calModModerator | calModCreator
			}
		}
		if node.guildFlag == 1 {
			entry.mod |= calModSignUp
		} else {
			entry.creator = node.inviter
		}
		if node.status == calStatusInvited && entry.start.time().After(rt.calendarNow()) && entry.flags&calFlagLocked == 0 {
			entry.mod |= calModPending
			node.pending = true
		}
		data.events = append(data.events, entry)
	}
	serverTime := c.u32()
	zone := unpackCalTime(c.u32())
	if !c.bad {
		data.clockOffset = zone.time().Sub(time.Unix(int64(serverTime), 0))
	}
	zoneNow := zone.time()
	data.lockouts = data.lockouts[:0]
	for count, i := c.u32(), uint32(0); i < count && !c.bad; i++ {
		lockout := calLockout{mapID: c.u32(), difficulty: c.u32()}
		seconds := int(int32(c.u32()))
		lockout.instance = c.u64()
		lockout.resetAt = zoneNow.Add(time.Duration(seconds/60) * time.Minute)
		data.lockouts = append(data.lockouts, lockout)
	}
	base := int64(c.u32())
	data.resets = data.resets[:0]
	for count, i := c.u32(), uint32(0); i < count && !c.bad; i++ {
		reset := calReset{mapID: c.u32(), period: int(c.u32())}
		offset := int64(c.u32())
		if reset.period > 0 {
			steps := (int64(serverTime) - (base + offset)) / int64(reset.period)
			reset.last = zoneNow.Add(time.Duration((base+offset+steps*int64(reset.period)-int64(serverTime))/60) * time.Minute)
		}
		data.resets = append(data.resets, reset)
	}
	for count, i := c.u32(), uint32(0); i < count && !c.bad; i++ {
		id := c.u32()
		server := CalendarHoliday{ID: id, Region: c.u32(), Looping: c.u32(), Priority: c.u32(), FilterType: c.u32()}
		for d := range server.Dates {
			server.Dates[d] = c.u32()
		}
		for d := range server.Durations {
			server.Durations[d] = c.u32()
		}
		for d := range server.CalFlags {
			server.CalFlags[d] = c.u32()
		}
		server.Texture = c.str()
		if local := data.holidays[id]; local != nil && !c.bad {
			local.Region, local.Looping, local.Priority, local.FilterType = server.Region, server.Looping, server.Priority, server.FilterType
			local.Dates, local.Durations, local.CalFlags, local.Texture = server.Dates, server.Durations, server.CalFlags, server.Texture
			local.FromServer = true
		}
	}
	// Only the invitations that wait for an answer stay.
	pending := data.invites[:0]
	for _, invite := range data.invites {
		if invite.pending && invite.inviter != 0 && !rt.isIgnoredGUID(invite.inviter) {
			pending = append(pending, invite)
		}
	}
	data.invites = pending
	data.received = true
	rt.calendarRelayOut(true)
	if len(data.invites) != 0 {
		rt.FireEvent("CALENDAR_UPDATE_PENDING_INVITES")
	}
}

// calendarReceiveEvent takes the event the server sent for the one asked for
// or the one being copied.
func (rt *Runtime) calendarReceiveEvent(c *calRead) {
	sendType := c.u8()
	event := &calEvent{creator: c.guid()}
	event.id = c.u64()
	event.title = c.str()
	event.description = c.str()
	event.eventType = int(c.u8())
	event.repeat = int(c.u8())
	event.maxSize = c.u32()
	event.dungeon = int(int32(c.u32()))
	event.flags = c.u32()
	event.at = unpackCalTime(c.u32())
	event.lockoutAt = unpackCalTime(c.u32())
	event.guildID = c.u32()
	count := c.u32()
	if c.bad {
		return
	}
	me := rt.playerGUID()
	ownStatus, found := uint8(0), false
	for i := uint32(0); i < count && !c.bad; i++ {
		invite := &calInvite{guid: c.guid()}
		invite.level = c.u8()
		invite.status = c.u8()
		invite.rank = c.u8()
		invite.inviteTyp = c.u8()
		invite.inviteID = c.u64()
		invite.statusAt = c.u32()
		invite.notes = c.str()
		if invite.guid == me {
			found = true
			ownStatus = invite.status
			if invite.rank != 0 {
				event.ownerInvite = invite.inviteID
			}
		}
		if rt.nameOf(invite.guid) == "" {
			rt.requestName(invite.guid)
		}
		event.invites = append(event.invites, invite)
	}
	if !found && event.flags&calFlagGuildEvent != 0 {
		ownStatus = calStatusNotSigned
	}
	event.sortCriterion = 3
	for i := range event.sortReverse {
		event.sortReverse[i] = false
	}
	rt.sortCalendarInvites(event)
	rt.cal.open = event
	if ownStatus == calStatusInvited && event.at.time().After(rt.calendarNow()) && rt.calendarFindInvite(event.id) == nil {
		entry := rt.calendarFindEvent(event.id)
		rt.cal.invites = append(rt.cal.invites, &calPending{eventID: event.id, pending: true, inviter: event.creator})
		if entry != nil {
			entry.mod |= calModPending
		}
		rt.FireEvent("CALENDAR_UPDATE_PENDING_INVITES")
	}
	switch sendType {
	case 1, 2:
		rt.FireEvent("CALENDAR_NEW_EVENT", lua.LBool(sendType == 2))
	default:
		rt.cal.openRef = calRef{id: event.id, start: event.at, flags: event.flags, set: true}
		rt.FireEvent("CALENDAR_OPEN_EVENT", lua.LString(calendarType(event.flags)))
	}
}

// calendarReceiveMassInvite makes the people of a guild or an arena team the
// invitations of the event that is being made.
func (rt *Runtime) calendarReceiveMassInvite(c *calRead) {
	event := rt.cal.open
	if event == nil {
		return
	}
	me := rt.playerGUID()
	event.invites = event.invites[:0]
	found := false
	for count, i := c.u32(), uint32(0); i < count && !c.bad; i++ {
		invite := &calInvite{guid: c.guid(), level: c.u8()}
		if invite.guid == me {
			invite.status, invite.rank = calStatusConfirmed, 2
			found = true
		}
		if rt.nameOf(invite.guid) == "" {
			rt.requestName(invite.guid)
		}
		event.invites = append(event.invites, invite)
	}
	if !found {
		level := uint8(0)
		if info := rt.unitInfo("player"); info != nil {
			level = uint8(info.Level)
		}
		event.invites = append(event.invites, &calInvite{guid: me, level: level, status: calStatusConfirmed, rank: 2})
	}
	event.ownerInvite = 1
	event.isNew = true
	event.selected = 0
	rt.sortCalendarInvites(event)
	rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST", lua.LTrue)
}

// calendarInviteAdded puts a new invitation on the event that is open.
func (rt *Runtime) calendarInviteAdded(c *calRead) bool {
	invitee := c.guid()
	eventID := c.u64()
	inviteID := c.u64()
	level := c.u8()
	status := c.u8()
	typ := c.u8()
	var statusAt uint32
	if typ == 1 {
		statusAt = c.u32()
	}
	answered := c.u8() != 0
	if c.bad {
		return false
	}
	if typ == 1 {
		if open := rt.cal.open; open == nil || open.id != eventID {
			if entry := rt.calendarFindEvent(eventID); entry != nil {
				entry.inviteID = inviteID
				entry.status = status
				rt.calendarRelayOut(true)
				return answered
			}
		}
	}
	if open := rt.cal.open; open != nil && open.id == eventID {
		invite := &calInvite{inviteID: inviteID, guid: invitee, level: level, status: status, inviteTyp: typ, statusAt: statusAt}
		if invitee == rt.playerGUID() && typ != 1 {
			invite.rank = 2
		}
		open.invites = append(open.invites, invite)
		if rt.nameOf(invitee) == "" {
			rt.requestName(invitee)
		}
		rt.sortCalendarInvites(open)
		rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST")
	}
	return answered
}

// calendarInviteGone takes an invitation off the event that is open.
func (rt *Runtime) calendarInviteGone(c *calRead) bool {
	invitee := c.guid()
	eventID := c.u64()
	flags := c.u32()
	answered := c.u8() != 0
	if c.bad {
		return false
	}
	if flags&calFlagGuildEvent != 0 {
		if entry := rt.calendarFindEvent(eventID); entry != nil {
			entry.inviteID, entry.status = 0, calStatusNotSigned
			rt.calendarRelayOut(true)
		}
	}
	if open := rt.cal.open; open != nil && open.id == eventID {
		for i, invite := range open.invites {
			if invite.guid == invitee {
				open.invites = append(open.invites[:i], open.invites[i+1:]...)
				break
			}
		}
		rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST")
	}
	return answered
}

// calendarInviteStatus changes the answer of an invitee.
func (rt *Runtime) calendarInviteStatus(c *calRead) bool {
	invitee := c.guid()
	eventID := c.u64()
	c.u32()
	c.u32()
	status := c.u8()
	answered := c.u8() != 0
	statusAt := c.u32()
	if c.bad {
		return false
	}
	entry := rt.calendarFindEvent(eventID)
	if entry == nil {
		return answered
	}
	if invitee == rt.playerGUID() {
		entry.status = status
		if status == calStatusInvited {
			rt.calendarAddPending(entry)
		} else if rt.calendarDropInvite(eventID) {
			entry.mod &^= calModPending
			rt.FireEvent("CALENDAR_UPDATE_PENDING_INVITES")
		}
		rt.calendarRelayOut(true)
	}
	if open := rt.cal.open; open != nil && open.id == eventID {
		for _, invite := range open.invites {
			if invite.guid == invitee {
				invite.status = status
				if statusAt != 0 {
					invite.statusAt = statusAt
				}
				break
			}
		}
		rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST")
	}
	return answered
}

// calendarAddPending makes an invitation that waits for an answer out of an
// entry.
func (rt *Runtime) calendarAddPending(entry *calEntry) {
	if rt.calendarFindInvite(entry.id) == nil {
		rt.cal.invites = append(rt.cal.invites, &calPending{eventID: entry.id, inviteID: entry.inviteID, inviter: entry.creator, pending: true})
	}
	entry.mod |= calModPending
	rt.FireEvent("CALENDAR_UPDATE_PENDING_INVITES")
}

// calendarLockoutAdd puts a raid the character was saved to on its reset day.
func (rt *Runtime) calendarLockoutAdd(c *calRead) {
	now := unpackCalTime(c.u32())
	mapID := c.u32()
	difficulty := c.u32()
	seconds := int(int32(c.u32()))
	instance := c.u64()
	if c.bad {
		return
	}
	rt.cal.lockouts = append(rt.cal.lockouts, calLockout{mapID: mapID, difficulty: difficulty, instance: instance,
		resetAt: now.time().Add(time.Duration(seconds/60) * time.Minute)})
	rt.calendarRelayOut(true)
}

// calendarLockoutRemove takes a raid off the days it was saved to.
func (rt *Runtime) calendarLockoutRemove(c *calRead) {
	mapID := int32(c.u32())
	var difficulty uint32 = 0xffffffff
	var instance uint64 = ^uint64(0)
	if mapID >= 0 {
		difficulty = c.u32()
		c.u32()
		instance = c.u64()
	}
	if c.bad {
		return
	}
	kept := rt.cal.lockouts[:0]
	for _, lockout := range rt.cal.lockouts {
		if mapID < 0 || (lockout.mapID == uint32(mapID) && (difficulty == 0xffffffff || lockout.difficulty == difficulty) &&
			(instance == ^uint64(0) || lockout.instance == instance)) {
			continue
		}
		kept = append(kept, lockout)
	}
	rt.cal.lockouts = kept
	rt.calendarRelayOut(true)
}

// calendarLockoutUpdate moves a saved raid to its new reset time.
func (rt *Runtime) calendarLockoutUpdate(c *calRead) {
	now := unpackCalTime(c.u32())
	mapID := c.u32()
	difficulty := c.u32()
	oldSeconds := int(int32(c.u32()))
	newSeconds := int(int32(c.u32()))
	if c.bad {
		return
	}
	old := now.time().Add(time.Duration(oldSeconds/60) * time.Minute)
	moved := now.time().Add(time.Duration(newSeconds/60) * time.Minute)
	for i := range rt.cal.lockouts {
		lockout := &rt.cal.lockouts[i]
		if lockout.mapID == mapID && lockout.difficulty == difficulty && lockout.resetAt.Year() == old.Year() && lockout.resetAt.YearDay() == old.YearDay() {
			lockout.resetAt = moved
		}
	}
	rt.calendarRelayOut(true)
}

// calendarInviteAlert tells of an invitation that came while playing.
func (rt *Runtime) calendarInviteAlert(c *calRead) {
	entry := &calEntry{id: c.u64(), title: c.str()}
	entry.start = unpackCalTime(c.u32())
	entry.flags = c.u32()
	entry.eventType = int(int32(c.u32()))
	entry.dungeon = int(int32(c.u32()))
	entry.inviteID = c.u64()
	entry.status = c.u8()
	rank := c.u8()
	creator := c.guid()
	inviter := c.guid()
	if c.bad {
		return
	}
	pending := false
	if entry.status == calStatusNotSigned || entry.status == calStatusSignedUp {
		entry.creator = creator
		entry.mod = calModSignUp
	} else {
		entry.creator = inviter
		if entry.status == calStatusInvited && entry.flags&calFlagGuildAnnounce == 0 {
			entry.mod = calModPending
			pending = true
		}
	}
	if rank != 0 {
		entry.mod |= calModModerator
	}
	if creator == rt.playerGUID() || rank == 2 {
		entry.mod |= calModCreator
	}
	rt.cal.events = append(rt.cal.events, entry)
	if pending {
		rt.cal.invites = append(rt.cal.invites, &calPending{eventID: entry.id, inviteID: entry.inviteID, inviter: entry.creator, pending: true})
	}
	rt.calendarRelayOut(true)
	if pending && !rt.isIgnoredGUID(entry.creator) {
		rt.FireEvent("CALENDAR_UPDATE_PENDING_INVITES")
	}
}

// calendarRemovedAlert tells that an invitation was taken away.
func (rt *Runtime) calendarRemovedAlert(c *calRead) {
	eventID := c.u64()
	c.u32()
	flags := c.u32()
	c.u8()
	if c.bad {
		return
	}
	entry := rt.calendarFindEvent(eventID)
	if entry != nil && entry.flags&calFlagGuildEvent != 0 && entry.mod&calModSignUp != 0 {
		entry.status = calStatusNotSigned
		entry.inviteID = 0
		if open := rt.cal.open; open != nil && open.id == eventID {
			rt.calendarDropOpenInvite(open, rt.playerGUID())
			rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST")
		}
		rt.FireEvent("CALENDAR_UPDATE_EVENT_LIST")
	} else {
		rt.calendarCloseIf(eventID)
		rt.calendarDropEvent(eventID)
	}
	rt.calendarDropInvite(eventID)
	rt.FireEvent("CALENDAR_UPDATE_PENDING_INVITES")
	rt.calendarRelayOut(true)
	_ = flags
}

func (rt *Runtime) calendarDropOpenInvite(event *calEvent, guid uint64) {
	for i, invite := range event.invites {
		if invite.guid == guid {
			event.invites = append(event.invites[:i], event.invites[i+1:]...)
			return
		}
	}
}

// calendarCloseIf closes the open event when it is the one named.
func (rt *Runtime) calendarCloseIf(eventID uint64) {
	if open := rt.cal.open; open != nil && open.id == eventID {
		rt.cal.open = nil
		rt.FireEvent("CALENDAR_CLOSE_EVENT")
	}
	if rt.cal.openRef.set && rt.cal.openRef.id == eventID {
		rt.cal.openRef = calRef{}
	}
	if rt.cal.contextRef.set && rt.cal.contextRef.id == eventID {
		rt.cal.contextRef = calRef{}
	}
}

// calendarStatusAlert tells of the answer the character gave elsewhere.
func (rt *Runtime) calendarStatusAlert(c *calRead) {
	if rt.playerGUID() == 0 {
		return
	}
	eventID := c.u64()
	c.u32()
	c.u32()
	status := c.u8()
	if c.bad {
		return
	}
	entry := rt.calendarFindEvent(eventID)
	if entry == nil {
		if !rt.cal.received {
			rt.send(calendarGetPending, nil)
		}
		return
	}
	entry.status = status
	if entry.flags&calFlagLocked == 0 {
		if status == calStatusInvited {
			rt.calendarAddPending(entry)
		} else if rt.calendarDropInvite(eventID) {
			entry.mod &^= calModPending
			rt.FireEvent("CALENDAR_UPDATE_PENDING_INVITES")
		}
	}
	if open := rt.cal.open; open != nil && open.id == eventID {
		for _, invite := range open.invites {
			if invite.guid == rt.playerGUID() {
				invite.status = status
			}
		}
	}
	rt.FireEvent("CALENDAR_UPDATE_EVENT_LIST")
}

// calendarEventGone takes an event that was removed off the calendar.
func (rt *Runtime) calendarEventGone(c *calRead) bool {
	answered := c.u8() != 0
	eventID := c.u64()
	c.u32()
	if c.bad {
		return false
	}
	rt.calendarCloseIf(eventID)
	if rt.calendarDropEvent(eventID) {
		rt.calendarDropInvite(eventID)
		rt.FireEvent("CALENDAR_UPDATE_PENDING_INVITES")
		rt.calendarRelayOut(true)
	}
	return answered
}

// calendarEventUpdate changes the event that is on the calendar.
func (rt *Runtime) calendarEventUpdate(c *calRead) bool {
	answered := c.u8() != 0
	eventID := c.u64()
	c.u32()
	flags := c.u32()
	moved := unpackCalTime(c.u32())
	eventType := int(c.u8())
	dungeon := int(int32(c.u32()))
	title := c.str()
	if c.bad {
		return false
	}
	entry := rt.calendarFindEvent(eventID)
	if entry == nil {
		if !rt.cal.received {
			rt.send(calendarGetPending, nil)
		}
	} else {
		wasLocked := entry.flags&calFlagLocked != 0
		entry.title, entry.flags, entry.start = title, flags, moved
		entry.eventType, entry.dungeon = eventType, dungeon
		if locked := flags&calFlagLocked != 0; locked != wasLocked && entry.status == calStatusInvited {
			if locked {
				entry.mod &^= calModPending
				rt.calendarDropInvite(eventID)
				rt.FireEvent("CALENDAR_UPDATE_PENDING_INVITES")
			} else {
				rt.calendarAddPending(entry)
			}
		}
		rt.calendarRelayOut(true)
	}
	if open := rt.cal.open; open != nil && open.id == eventID {
		open.title, open.flags, open.at = title, flags, moved
		open.eventType, open.dungeon = eventType, dungeon
		rt.FireEvent("CALENDAR_UPDATE_EVENT")
	}
	return answered
}

// calendarModeratorSet makes an invitee a moderator of the event or takes it
// away.
func (rt *Runtime) calendarModeratorSet(c *calRead) bool {
	invitee := c.guid()
	eventID := c.u64()
	flag := c.u8()
	answered := c.u8() != 0
	if c.bad {
		return false
	}
	if open := rt.cal.open; open != nil && open.id == eventID {
		for _, invite := range open.invites {
			if invite.guid == invitee {
				invite.rank = flag
				if invitee == rt.playerGUID() {
					if flag == 1 {
						open.ownerInvite = invite.inviteID
					} else {
						open.ownerInvite = 0
					}
				}
				break
			}
		}
		rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST")
	}
	if invitee == rt.playerGUID() {
		if entry := rt.calendarFindEvent(eventID); entry != nil {
			if flag == 1 {
				entry.mod |= calModModerator
			} else {
				entry.mod &^= calModModerator
			}
			rt.calendarRelayOut(true)
		}
	}
	return answered
}

// calendarNotesSet sets what an invitee wrote on the invitation. The notes of
// the character itself come without a name.
func (rt *Runtime) calendarNotesSet(c *calRead, named bool) bool {
	var invitee uint64
	if named {
		invitee = c.guid()
	} else {
		invitee = rt.playerGUID()
	}
	eventID := c.u64()
	notes := c.str()
	answered := true
	if named {
		answered = c.u8() != 0
	}
	if c.bad {
		return false
	}
	if open := rt.cal.open; open != nil && open.id == eventID {
		for _, invite := range open.invites {
			if invite.guid == invitee {
				invite.notes = notes
				rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST")
				break
			}
		}
	}
	return answered
}
