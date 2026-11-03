package ui

import (
	"sort"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// calNumber reads an argument that must be a number.
func calNumber(L *lua.LState, index int) (int, bool) {
	if value, ok := L.Get(index).(lua.LNumber); ok {
		return int(value), true
	}
	return 0, false
}

// calGuildRight reports whether the rank of the character may make guild
// events.
func (rt *Runtime) calGuildRight() bool {
	return rt.guild.id != 0 && rt.ownGuildRights()&guildRightCreateEvent != 0
}

// calOwnerOf reports whether the character may change the event that is
// open: it made it, or it was made a moderator.
func (rt *Runtime) calCanEdit(event *calEvent) bool {
	if event.ownerInvite == 0 && rt.playerGUID() != event.creator {
		return false
	}
	return true
}

// calEditAllowed is the right to change the open event; announcements need the
// right of the guild rank.
func (rt *Runtime) calEditAllowed(event *calEvent) bool {
	if event.flags&calFlagGuildAnnounce == 0 {
		return rt.calCanEdit(event)
	}
	return rt.calGuildRight()
}

// calEntryEditable is the right to change or remove the entry of a day.
func (rt *Runtime) calEntryEditable(entry *calEntry) bool {
	if entry.flags&(calFlagGuildAny) == 0 {
		return entry.mod&(calModModerator|calModCreator) != 0
	}
	return rt.calGuildRight()
}

// calOwnInvite is the invitation of the character to the open event.
func (rt *Runtime) calOwnInvite(event *calEvent) *calInvite {
	me := rt.playerGUID()
	for _, invite := range event.invites {
		if invite.guid == me {
			return invite
		}
	}
	return nil
}

func (rt *Runtime) calTexturesOf(kind int) []*LFGDungeonRow {
	var rows []*LFGDungeonRow
	for i := range rt.lfg.tables.Dungeons {
		row := &rt.lfg.tables.Dungeons[i]
		switch kind {
		case 0:
			if row.Type == lfgTypeRaid {
				rows = append(rows, row)
			}
		case 1:
			if row.Type == lfgTypeDungeon || row.Type == lfgTypeHeroic {
				rows = append(rows, row)
			}
		}
	}
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].ID < rows[b].ID })
	return rows
}

// calTextureIndex is the place of a dungeon among the ones an event type
// offers, or -1.
func (rt *Runtime) calTextureIndex(eventType, dungeon int) int {
	if eventType != 0 && eventType != 1 {
		return -1
	}
	for i, row := range rt.calTexturesOf(eventType) {
		if int(row.ID) == dungeon {
			return i
		}
	}
	return -1
}

// calDifficultyName is the text for the difficulty of a saved raid.
func (rt *Runtime) calDifficultyName(mapID, difficulty uint32) string {
	if rt.isRaidMap(mapID) {
		return rt.globalString("RAID_DIFFICULTY"+itoa(int(difficulty)+1), "")
	}
	if difficulty == 0 {
		return ""
	}
	return rt.globalString("DUNGEON_DIFFICULTY"+itoa(int(difficulty)+1), "")
}

func calClassOf(rt *Runtime, guid uint64) (name, file string) {
	if identity, ok := rt.identityOf(guid); ok {
		return rt.classNameFor(int(identity.Class), int(identity.Gender)), ClassFileFromID(int(identity.Class))
	}
	return "", ""
}

// sortCalendarInvites orders the invitations of the event by the criterion
// the interface chose.
func (rt *Runtime) sortCalendarInvites(event *calEvent) {
	less := func(a, b *calInvite) int {
		nameOf := func(i *calInvite) string { return strings.ToLower(rt.nameOf(i.guid)) }
		var result int
		switch event.sortCriterion {
		case 0:
			result = strings.Compare(nameOf(a), nameOf(b))
		case 1:
			result = int(a.level) - int(b.level)
		case 2:
			ca, _ := calClassOf(rt, a.guid)
			cb, _ := calClassOf(rt, b.guid)
			result = strings.Compare(ca, cb)
		case 3:
			result = calStatusOrder[min(int(a.status), 8)] - calStatusOrder[min(int(b.status), 8)]
		case 4:
			pa, pb := rt.unitTokenOf(a.guid) != "", rt.unitTokenOf(b.guid) != ""
			switch {
			case pa && !pb:
				result = -1
			case !pa && pb:
				result = 1
			}
		case 5:
			result = strings.Compare(strings.ToLower(a.notes), strings.ToLower(b.notes))
		}
		if event.sortReverse[event.sortCriterion] {
			result = -result
		}
		if event.sortCriterion != 0 && result == 0 {
			return strings.Compare(nameOf(a), nameOf(b))
		}
		return result
	}
	sort.SliceStable(event.invites, func(a, b int) bool { return less(event.invites[a], event.invites[b]) < 0 })
}

// calLocate finds where an entry the interface named stands in the grid.
func (rt *Runtime) calLocate(ref calRef) (offset, day, index int, ok bool) {
	if !ref.set || (ref.id == 0 && ref.mapID == 0) {
		return 0, 0, 0, false
	}
	offset = rt.calendarOffsetOf(ref.start.month, ref.start.year)
	if offset < -1 || offset > 1 {
		return 0, 0, 0, false
	}
	for i, entry := range rt.calendarCell(offset, ref.start.day) {
		match := false
		if ref.id != 0 {
			match = entry.id == ref.id
		}
		if ref.mapID != 0 {
			match = entry.mapID == ref.mapID
			if ref.flags&calFlagLockout != 0 {
				match = match && entry.difficulty == ref.difficulty
			}
		}
		if match {
			return offset, ref.start.day, i, true
		}
	}
	return 0, 0, 0, false
}

// calEntryArgs reads (offset, monthDay, index) from the arguments.
func (rt *Runtime) calEntryArgs(L *lua.LState) (*calEntry, bool) {
	offset, ok1 := calNumber(L, 1)
	day, ok2 := calNumber(L, 2)
	index, ok3 := calNumber(L, 3)
	if !ok1 || !ok2 || !ok3 {
		return nil, false
	}
	return rt.calendarEntry(offset, day-1, index-1), true
}

// calContext is the entry a context call acts on: the one named in the
// arguments, or the one the context menu was opened for.
func (rt *Runtime) calContext(L *lua.LState, usage string) (*calEntry, bool) {
	if entry, given := rt.calEntryArgs(L); given {
		return entry, entry != nil
	}
	if rt.cal.context == nil {
		L.RaiseError("%s", usage)
		return nil, false
	}
	return rt.cal.context, true
}

func pushAll(L *lua.LState, values ...lua.LValue) int {
	for _, value := range values {
		L.Push(value)
	}
	return len(values)
}

func calNumbers(values ...int) []lua.LValue {
	out := make([]lua.LValue, len(values))
	for i, value := range values {
		out[i] = lua.LNumber(value)
	}
	return out
}

// calTimeCheck reports whether the time of an event lies ahead of the realm
// clock and not beyond the last day events may be made for.
func (rt *Runtime) calTimeCheck(at calTime) bool {
	if at.hour < 0 || at.minute < 0 || at.day < 0 || at.month < 0 || at.year < 0 {
		return false
	}
	when := at.time()
	now := rt.calendarNow().Truncate(time.Minute)
	if when.Before(now) {
		return false
	}
	return !when.After(rt.calendarMaxCreate())
}

// calendarMaxCreate is the last moment an event may be made for: the end of
// the current month, a year ahead.
func (rt *Runtime) calendarMaxCreate() time.Time {
	now := rt.calendarNow()
	first := time.Date(now.Year()+1, now.Month()+1, 1, 23, 59, 0, 0, time.Local)
	return first.AddDate(0, 0, -1)
}

// calEventAllowed is the check before the client sends something that changes
// the calendar.
func (rt *Runtime) calEventAllowed() bool {
	if rt.cal.actionPending {
		return false
	}
	if rt.cal.lastEvent.IsZero() || time.Since(rt.cal.lastEvent) >= 5*time.Second {
		return true
	}
	return false
}

func (rt *Runtime) calInviteAllowed() bool {
	if rt.cal.actionPending {
		return false
	}
	return rt.cal.lastInvite.IsZero() || time.Since(rt.cal.lastInvite) >= 2*time.Second
}

// calNewEvent starts an event that has not been sent yet. Kind 0 is an event
// of a player, 1 an announcement of the guild and 2 an event of the guild.
func (rt *Runtime) calNewEvent(kind int) {
	me := rt.playerGUID()
	if me == 0 {
		return
	}
	if kind != 0 && rt.guild.id == 0 {
		rt.calendarError("ERR_GUILD_PLAYER_NOT_IN_GUILD", nil)
		return
	}
	open := &calEvent{creator: me, maxSize: 100, dungeon: -1, sortCriterion: 3, ownerInvite: 1, isNew: true,
		at: calTime{-1, -1, -1, -1, -1, -1}, lockoutAt: calTime{-1, -1, -1, -1, -1, -1}}
	switch kind {
	case 0:
		open.flags |= calFlagPlayer
	case 1:
		open.flags |= calFlagGuildAnnounce
	case 2:
		open.flags |= calFlagGuildEvent
	}
	if kind != 1 {
		level := 0
		if info := rt.unitInfo("player"); info != nil {
			level = info.Level
		}
		inviteType := uint8(0)
		if kind == 2 {
			inviteType = 1
		}
		open.invites = append(open.invites, &calInvite{guid: me, level: uint8(level), status: calStatusConfirmed, rank: 2, inviteTyp: inviteType})
	}
	rt.cal.open = open
	rt.cal.openRef = calRef{}
	if kind == 0 {
		rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST", lua.LTrue)
	}
}

// calSetTime sets the hour and minute of a time of the open event.
func (rt *Runtime) calSetTime(event *calEvent, at *calTime, hour, minute int) bool {
	if hour < 0 || hour >= 24 || minute < 0 || minute >= 60 {
		return false
	}
	if at.hour != hour || at.minute != minute {
		event.changed = true
	}
	at.hour, at.minute = hour, minute
	return true
}

// calSetDate sets the month, day and year of a time of the open event.
func (rt *Runtime) calSetDate(event *calEvent, at *calTime, month, day, year int, guildEvent bool) bool {
	if guildEvent {
		return false
	}
	if year >= 2000 {
		year -= 2000
	}
	if month < 0 || month >= 12 || day < 0 || day >= 32 || year < 0 || year >= 32 {
		return false
	}
	if at.month != month || at.day != day || at.year != year {
		event.changed = true
	}
	at.month, at.day, at.year = month, day, year
	at.weekday = int(time.Date(2000+year, time.Month(month+1), day+1, 0, 0, 0, 0, time.Local).Weekday())
	return true
}

// calEventSetter runs a change of the open event when the character may make
// it.
func (rt *Runtime) calEventSetter(change func(event *calEvent)) {
	if event := rt.cal.open; event != nil && rt.calEditAllowed(event) {
		change(event)
	}
}

func (rt *Runtime) calSendRSVP(eventID, inviteID uint64, status uint32) {
	w := &calWriter{}
	w.u64(eventID)
	w.u64(inviteID)
	w.u32(status)
	rt.calendarSend(calendarRSVP, w)
}

// calAnswer answers the invitation to the open event.
func (rt *Runtime) calAnswer(status int) {
	event := rt.cal.open
	if event == nil || rt.cal.actionPending {
		return
	}
	if event.flags&calFlagGuildEvent != 0 && event.guildID == rt.guildIDOfPlayer() && status != 8 {
		return
	}
	if event.isNew {
		return
	}
	invite := rt.calOwnInvite(event)
	if invite == nil {
		return
	}
	if event.flags&calFlagGuildEvent != 0 && invite.inviteTyp == 1 && status != 8 {
		return
	}
	rt.calSendRSVP(event.id, invite.inviteID, uint32(status))
	rt.calendarSetAction(true)
}

func (rt *Runtime) guildIDOfPlayer() uint32 {
	if info := rt.unitInfo("player"); info != nil {
		return info.GuildID
	}
	return 0
}

// calSendUpdate sends the changes made to the open event.
func (rt *Runtime) calSendUpdate(event *calEvent) bool {
	if !event.changed {
		return false
	}
	event.changed = false
	if event.at.time().Before(rt.calendarNow()) && event.at.year >= 0 && event.lockoutAt.year < 0 {
		rt.calendarError("CALENDAR_ERROR_EVENT_PASSED", nil)
		return false
	}
	w := &calWriter{}
	w.u64(event.id)
	w.u64(event.ownerInvite)
	w.cstr(event.title)
	w.cstr(event.description)
	w.u8(uint8(event.eventType))
	w.u8(uint8(event.repeat))
	w.u32(event.maxSize)
	w.u32(uint32(int32(event.dungeon)))
	w.u32(event.at.pack())
	w.u32(event.lockoutAt.pack())
	w.u32(event.flags)
	return rt.calendarSend(calendarUpdateEvent, w)
}

// calSendNew sends an event that was just made.
func (rt *Runtime) calSendNew(event *calEvent) bool {
	if !event.isNew {
		return false
	}
	if event.title == "" {
		rt.calendarError("CALENDAR_ERROR_NEEDS_TITLE", nil)
		return false
	}
	if event.flags&(calFlagGuildAny) == 0 {
		if event.maxSize == 0 {
			return false
		}
		if event.maxSize > calMaxInvites {
			rt.calendarError("CALENDAR_ERROR_INVITES_EXCEEDED", calMaxInvites)
			return false
		}
	} else if rt.guild.id == 0 {
		rt.calendarError("ERR_GUILD_PLAYER_NOT_IN_GUILD", nil)
		return false
	}
	if event.at.hour < 0 || event.at.minute < 0 {
		rt.calendarError("CALENDAR_ERROR_INVALID_TIME", nil)
		return false
	}
	if event.at.month < 0 || event.at.day < 0 || event.at.year < 0 {
		rt.calendarError("CALENDAR_ERROR_INVALID_DATE", nil)
		return false
	}
	if event.at.time().Before(rt.calendarNow()) {
		rt.calendarError("CALENDAR_ERROR_EVENT_TIME_PASSED", nil)
		return false
	}
	w := &calWriter{}
	w.cstr(event.title)
	w.cstr(event.description)
	w.u8(uint8(event.eventType))
	w.u8(uint8(event.repeat))
	w.u32(event.maxSize)
	w.u32(uint32(int32(event.dungeon)))
	w.u32(event.at.pack())
	w.u32(event.lockoutAt.pack())
	w.u32(event.flags)
	if event.flags&calFlagGuildAnnounce == 0 {
		w.u32(uint32(len(event.invites)))
		for _, invite := range event.invites {
			w.packed(invite.guid)
			w.u8(invite.status)
			w.u8(invite.rank)
		}
	}
	return rt.calendarSend(calendarAddEvent, w)
}

// calRemoveInviteAt takes the invitation at an index off the open event.
func (rt *Runtime) calRemoveInviteAt(event *calEvent, index int) bool {
	if index < 0 || index >= len(event.invites) {
		return false
	}
	invite := event.invites[index]
	if invite.rank == 2 {
		return false
	}
	me := rt.playerGUID()
	if invite.guid != me && !rt.calCanEdit(event) {
		rt.calendarError("CALENDAR_ERROR_PERMISSIONS", nil)
		return false
	}
	if !event.isNew {
		w := &calWriter{}
		w.packed(invite.guid)
		w.u64(invite.inviteID)
		w.u64(event.ownerInvite)
		w.u64(event.id)
		rt.calendarSend(calendarRemoveInv, w)
		return true
	}
	event.invites = append(event.invites[:index], event.invites[index+1:]...)
	return true
}

// calendarRemoveOpen removes the open event, or the character's invitation to
// it when it may not change it.
func (rt *Runtime) calendarRemoveOpen() {
	event := rt.cal.open
	if rt.cal.actionPending || event == nil || event.isNew {
		return
	}
	if rt.calEditAllowed(event) {
		if !rt.calCanEdit(event) && event.flags&calFlagGuildAnnounce == 0 {
			rt.calendarError("CALENDAR_ERROR_PERMISSIONS", nil)
			return
		}
		w := &calWriter{}
		w.u64(event.id)
		w.u64(event.ownerInvite)
		guild := uint8(0)
		if event.flags&calFlagGuildAny != 0 {
			guild = 1
		}
		w.u8(guild)
		if rt.calendarSend(calendarRemoveEvent, w) {
			rt.calendarSetAction(true)
		}
		return
	}
	me := rt.playerGUID()
	for i, invite := range event.invites {
		if invite.guid == me {
			if rt.calRemoveInviteAt(event, i) {
				rt.calendarSetAction(true)
			}
			return
		}
	}
	rt.calendarError("CALENDAR_DELETE_CREATOR_FAILED", nil)
}

func registerCalendarEventAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	str := func(s string) lua.LValue { return lua.LString(s) }
	flagOrNil := func(set bool) lua.LValue {
		if set {
			return lua.LNumber(1)
		}
		return lua.LNil
	}

	// What the calendar offers.
	reg("CalendarIsActionPending", func(L *lua.LState) int { L.Push(lua.LBool(rt.cal.actionPending)); return 1 })
	reg("CalendarCanAddEvent", func(L *lua.LState) int { L.Push(lua.LBool(rt.calEventAllowed())); return 1 })
	reg("CalendarCanSendInvite", func(L *lua.LState) int { L.Push(lua.LBool(rt.calInviteAllowed())); return 1 })
	reg("CalendarGetNumPendingInvites", func(L *lua.LState) int {
		rt.calendarEnsure()
		L.Push(lua.LNumber(rt.pendingInviteCount()))
		return 1
	})
	reg("CalendarGetEventIndex", func(L *lua.LState) int {
		rt.calendarEnsure()
		if offset, day, index, ok := rt.calLocate(rt.cal.openRef); ok {
			return pushAll(L, calNumbers(offset, day+1, index+1)...)
		}
		return pushAll(L, calNumbers(0, 0, 0)...)
	})
	reg("CalendarContextGetEventIndex", func(L *lua.LState) int {
		rt.calendarEnsure()
		if offset, day, index, ok := rt.calLocate(rt.cal.contextRef); ok {
			return pushAll(L, calNumbers(offset, day+1, index+1)...)
		}
		return pushAll(L, calNumbers(0, 0, 0)...)
	})
	reg("CalendarGetNumDayEvents", func(L *lua.LState) int {
		offset, ok1 := calNumber(L, 1)
		day, ok2 := calNumber(L, 2)
		if !ok1 || !ok2 {
			L.RaiseError("Usage: CalendarGetNumDayEvents([-1,0,1], monthDay)")
			return 0
		}
		L.Push(lua.LNumber(len(rt.calendarCell(offset, day-1))))
		return 1
	})
	reg("CalendarGetFirstPendingInvite", func(L *lua.LState) int {
		offset, ok1 := calNumber(L, 1)
		day, ok2 := calNumber(L, 2)
		if !ok1 || !ok2 {
			L.RaiseError("Usage: CalendarGetDay([-1,0,1], monthDay)")
			return 0
		}
		for i, entry := range rt.calendarCell(offset, day-1) {
			if entry.mod&calModPending != 0 {
				L.Push(lua.LNumber(i + 1))
				return 1
			}
		}
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("CalendarGetDayEvent", func(L *lua.LState) int {
		entry, given := rt.calEntryArgs(L)
		if !given {
			L.RaiseError("Usage: CalendarGetDayEvent([-1.0.1], monthDay, index)")
			return 0
		}
		if entry == nil {
			return pushAll(L, lua.LNil, lua.LNumber(0), lua.LNumber(0), str(""), str(""), lua.LNumber(0), str(""), str(""),
				lua.LNumber(0), str(""), lua.LNumber(0), lua.LNumber(0), lua.LNumber(0), lua.LNumber(0), str(""))
		}
		var title, texture, difficultyName string
		switch {
		case entry.flags&calFlagHoliday != 0:
			if h := rt.cal.holidays[entry.holiday]; h != nil {
				title, texture = h.Name, h.Texture
			}
		case entry.flags&(calFlagLockout|calFlagReset) != 0:
			title = rt.mapName(entry.mapID)
		default:
			title = entry.title
			if entry.flags&calFlagEventKinds != 0 && entry.dungeon >= 0 {
				if row := rt.lfg.dungeon(uint32(entry.dungeon)); row != nil {
					texture = row.Texture
				}
			}
		}
		difficulty := 0
		if entry.flags&(calFlagLockout|calFlagReset) != 0 {
			difficulty = int(entry.difficulty) + 1
			if entry.flags&calFlagLockout != 0 {
				difficultyName = rt.calDifficultyName(entry.mapID, entry.difficulty)
			}
		}
		inviteType := 1
		if entry.flags&calFlagGuildEvent != 0 && entry.mod&calModSignUp != 0 {
			inviteType = 2
		}
		return pushAll(L, str(title), lua.LNumber(entry.start.hour), lua.LNumber(entry.start.minute), str(calendarType(entry.flags)),
			str(calendarSequence(entry)), lua.LNumber(entry.eventType+1), str(texture), str(calendarModStatus(entry.mod)),
			lua.LNumber(int(entry.status)+1), str(rt.nameOf(entry.creator)), lua.LNumber(difficulty), lua.LNumber(inviteType),
			lua.LNumber(entry.seqIndex+1), lua.LNumber(entry.seqDays), str(difficultyName))
	})
	reg("CalendarGetDayEventSequenceInfo", func(L *lua.LState) int {
		entry, given := rt.calEntryArgs(L)
		if !given {
			L.RaiseError("Usage: CalendarGetDayEventSequenceInfo([-1.0.1], monthDay, index)")
			return 0
		}
		if entry == nil {
			return pushAll(L, lua.LNumber(0), lua.LNumber(0), str(""))
		}
		return pushAll(L, lua.LNumber(entry.seqIndex+1), lua.LNumber(entry.seqDays), str(calendarSequence(entry)))
	})
	reg("CalendarGetRaidInfo", func(L *lua.LState) int {
		entry, given := rt.calEntryArgs(L)
		if !given {
			L.RaiseError("Usage: CalendarGetRaidInfo([-1,0,1], monthDay, index)")
			return 0
		}
		if entry == nil {
			return pushAll(L, lua.LNil, str(""), lua.LNumber(0), lua.LNumber(0), lua.LNumber(0), lua.LNumber(0), str(""))
		}
		name := rt.mapName(entry.mapID)
		return pushAll(L, str(name), str(calendarType(entry.flags)), lua.LNumber(entry.id&0xffffffff), lua.LNumber(entry.start.minute),
			lua.LNumber(entry.start.hour), lua.LNumber(entry.difficulty+1), str(rt.calDifficultyName(entry.mapID, entry.difficulty)))
	})
	reg("CalendarGetHolidayInfo", func(L *lua.LState) int {
		entry, given := rt.calEntryArgs(L)
		if !given {
			L.RaiseError("Usage: CalendarGetHolidayInfo([-1,0,1], monthDay, index)")
			return 0
		}
		if entry != nil && entry.flags&calFlagHoliday != 0 {
			if h := rt.cal.holidays[entry.holiday]; h != nil && h.Name != "" && h.Description != "" {
				return pushAll(L, str(h.Name), str(h.Description), str(h.Texture))
			}
		}
		return pushAll(L, lua.LNil, lua.LNil, lua.LNil)
	})
	reg("CalendarOpenEvent", func(L *lua.LState) int {
		offset, ok1 := calNumber(L, 1)
		day, ok2 := calNumber(L, 2)
		index, ok3 := calNumber(L, 3)
		if !ok1 || !ok2 || !ok3 {
			L.RaiseError("Usage: CalendarOpenEvent([-1,0,1], monthDay, index)")
			return 0
		}
		entry := rt.calendarEntry(offset, day-1, index-1)
		if rt.cal.actionPending || entry == nil {
			return 0
		}
		if entry.flags&calFlagEventKinds != 0 {
			w := &calWriter{}
			w.u64(entry.id)
			if rt.calendarSend(calendarGetEvent, w) {
				rt.calendarSetAction(true)
			}
			return 0
		}
		rt.cal.openRef = refOf(entry)
		rt.FireEvent("CALENDAR_OPEN_EVENT", lua.LString(calendarType(entry.flags)))
		return 0
	})
	reg("CalendarCloseEvent", func(L *lua.LState) int { rt.cal.openRef = calRef{}; return 0 })
	reg("CalendarContextSelectEvent", func(L *lua.LState) int {
		offset, ok1 := calNumber(L, 1)
		day, ok2 := calNumber(L, 2)
		index, ok3 := calNumber(L, 3)
		if !ok1 || !ok2 || !ok3 {
			L.RaiseError("Usage: CalendarContextSelectEvent([-1,0,1], monthDay, index)")
			return 0
		}
		if entry := rt.calendarEntry(offset, day-1, index-1); entry != nil {
			rt.cal.context = entry
			rt.cal.contextRef = refOf(entry)
		}
		return 0
	})
	reg("CalendarContextDeselectEvent", func(L *lua.LState) int {
		rt.cal.context, rt.cal.contextRef = nil, calRef{}
		return 0
	})

	// The open event.
	reg("CalendarGetEventInfo", func(L *lua.LState) int {
		event := rt.cal.open
		if event == nil {
			values := []lua.LValue{lua.LNil, lua.LNil, str("")}
			for i := 0; i < 16; i++ {
				values = append(values, lua.LNumber(0))
			}
			values = append(values, lua.LNil, lua.LNil, lua.LNil, lua.LNumber(0), lua.LNumber(0), str(""))
			return pushAll(L, values...)
		}
		textureIndex := rt.calTextureIndex(event.eventType, event.dungeon)
		values := []lua.LValue{str(event.title), str(event.description), str(rt.nameOf(event.creator)),
			lua.LNumber(event.eventType + 1), lua.LNumber(event.repeat + 1), lua.LNumber(event.maxSize), lua.LNumber(textureIndex + 1)}
		for _, at := range []calTime{event.at, event.lockoutAt} {
			values = append(values, lua.LNumber(at.weekday+1), lua.LNumber(at.month+1), lua.LNumber(at.day+1), lua.LNumber(at.year+2000),
				lua.LNumber(at.hour), lua.LNumber(at.minute))
		}
		values = append(values, flagOrNil(event.flags&calFlagLocked != 0), flagOrNil(event.flags&calFlagAutoApprove != 0),
			flagOrNil(rt.calendarFindInvite(event.id) != nil))
		if invite := rt.calOwnInvite(event); invite != nil {
			values = append(values, lua.LNumber(int(invite.status)+1), lua.LNumber(int(invite.inviteTyp)+1))
		} else if event.flags&calFlagGuildEvent != 0 {
			values = append(values, lua.LNumber(8), lua.LNumber(2))
		} else {
			values = append(values, lua.LNumber(0), lua.LNumber(0))
		}
		values = append(values, str(calendarType(event.flags)))
		return pushAll(L, values...)
	})
	reg("CalendarEventGetCalendarType", func(L *lua.LState) int {
		if event := rt.cal.open; event != nil {
			L.Push(lua.LString(calendarType(event.flags)))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("CalendarEventCanEdit", func(L *lua.LState) int {
		event := rt.cal.open
		L.Push(lua.LBool(event != nil && rt.calEditAllowed(event)))
		return 1
	})
	reg("CalendarEventHaveSettingsChanged", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.cal.open != nil && rt.cal.open.changed))
		return 1
	})
	reg("CalendarEventHasPendingInvite", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.cal.open != nil && rt.calendarFindInvite(rt.cal.open.id) != nil))
		return 1
	})
	reg("CalendarEventIsModerator", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.cal.open != nil && rt.calCanEdit(rt.cal.open)))
		return 1
	})
	reg("CalendarEventGetTypes", func(L *lua.LState) int {
		for _, name := range calTypeNames {
			L.Push(lua.LString(rt.globalString(name, name)))
		}
		return len(calTypeNames)
	})
	reg("CalendarEventGetRepeatOptions", func(L *lua.LState) int {
		for _, name := range calRepeatNames {
			L.Push(lua.LString(rt.globalString(name, name)))
		}
		return len(calRepeatNames)
	})
	reg("CalendarEventGetTextures", func(L *lua.LState) int {
		kind, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarEventGetTextures(eventType)")
			return 0
		}
		var rows []*LFGDungeonRow
		switch kind {
		case 1:
			rows = rt.calTexturesOf(0)
		case 2:
			rows = rt.calTexturesOf(1)
		default:
			return 0
		}
		count := 0
		for _, row := range rows {
			difficulty := ""
			if kind == 1 {
				difficulty = rt.calDifficultyName(row.Map, row.Difficulty)
			}
			count += pushAll(L, str(row.Name), str(row.Texture), lua.LNumber(row.Expansion), str(difficulty))
		}
		return count
	})
	reg("CalendarEventGetStatusOptions", func(L *lua.LState) int {
		index, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarEventGetStatusOptions(index)")
			return 0
		}
		event := rt.cal.open
		if event == nil {
			return 0
		}
		if !rt.calCanEdit(event) {
			rt.calendarError("CALENDAR_ERROR_PERMISSIONS", nil)
			return 0
		}
		count := 0
		for status := 0; status < 9; status++ {
			if rt.calCanSetStatus(event, index-1, status) {
				count += pushAll(L, lua.LNumber(status+1), str(rt.globalString(calStatusNames[status], calStatusNames[status])))
			}
		}
		return count
	})

	// Changing the open event.
	reg("CalendarEventSetTitle", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			return 0
		}
		title := string(L.Get(1).(lua.LString))
		rt.calEventSetter(func(event *calEvent) {
			if title != "" && title != event.title {
				event.title = truncateBytes(title, 31)
				event.changed = true
			}
		})
		return 0
	})
	reg("CalendarEventSetDescription", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			return 0
		}
		text := string(L.Get(1).(lua.LString))
		rt.calEventSetter(func(event *calEvent) {
			if text != "" && text != event.description {
				event.description = truncateBytes(text, 255)
				event.changed = true
			}
		})
		return 0
	})
	reg("CalendarEventSetType", func(L *lua.LState) int {
		if value, ok := calNumber(L, 1); ok {
			rt.calEventSetter(func(event *calEvent) {
				if value-1 < 5 && value-1 != event.eventType {
					event.eventType = value - 1
					event.changed = true
				}
			})
		}
		return 0
	})
	reg("CalendarEventSetRepeatOption", func(L *lua.LState) int {
		if value, ok := calNumber(L, 1); ok {
			rt.calEventSetter(func(event *calEvent) {
				if value-1 < 4 && value-1 != event.repeat {
					event.repeat = value - 1
					event.changed = true
				}
			})
		}
		return 0
	})
	reg("CalendarEventSetSize", func(L *lua.LState) int {
		if value, ok := calNumber(L, 1); ok {
			rt.calEventSetter(func(event *calEvent) {
				if event.flags&calFlagGuildAny == 0 && value > 1 && uint32(value) != event.maxSize {
					event.maxSize = uint32(value)
					event.changed = true
				}
			})
		}
		return 0
	})
	reg("CalendarEventSetTime", func(L *lua.LState) int {
		hour, ok1 := calNumber(L, 1)
		minute, ok2 := calNumber(L, 2)
		if !ok1 || !ok2 {
			L.RaiseError("Usage: CalendarEventSetTime(hour, minute)")
			return 0
		}
		set := false
		if event := rt.cal.open; event != nil && rt.calEditAllowed(event) {
			set = rt.calSetTime(event, &event.at, hour, minute)
		} else {
			return 0
		}
		if !set {
			rt.calendarError("CALENDAR_ERROR_INVALID_TIME", nil)
		}
		return 0
	})
	reg("CalendarEventSetDate", func(L *lua.LState) int {
		month, ok1 := calNumber(L, 1)
		day, ok2 := calNumber(L, 2)
		year, ok3 := calNumber(L, 3)
		if !ok1 || !ok2 || !ok3 {
			L.RaiseError("Usage: CalendarEventSetDate(month, day, year)")
			return 0
		}
		event := rt.cal.open
		if event == nil || !rt.calEditAllowed(event) {
			return 0
		}
		if !rt.calSetDate(event, &event.at, month-1, day-1, year, false) {
			rt.calendarError("CALENDAR_ERROR_INVALID_DATE", nil)
		}
		return 0
	})
	reg("CalendarEventSetLockoutTime", func(L *lua.LState) int {
		hour, ok1 := calNumber(L, 1)
		minute, ok2 := calNumber(L, 2)
		if !ok1 || !ok2 {
			L.RaiseError("Usage: CalendarEvenSettLockoutTime(hour, minute)")
			return 0
		}
		event := rt.cal.open
		if event == nil || !rt.calEditAllowed(event) {
			return 0
		}
		if event.flags&calFlagGuildAny != 0 || !rt.calSetTime(event, &event.lockoutAt, hour, minute) {
			rt.calendarError("CALENDAR_ERROR_INVALID_TIME", nil)
		}
		return 0
	})
	reg("CalendarEventSetLockoutDate", func(L *lua.LState) int {
		month, ok1 := calNumber(L, 1)
		day, ok2 := calNumber(L, 2)
		year, ok3 := calNumber(L, 3)
		if !ok1 || !ok2 || !ok3 {
			L.RaiseError("Usage: CalendarEventSetLockoutDate(month, day, year)")
			return 0
		}
		event := rt.cal.open
		if event == nil || !rt.calEditAllowed(event) {
			return 0
		}
		if !rt.calSetDate(event, &event.lockoutAt, month-1, day-1, year, event.flags&calFlagGuildAny != 0) {
			rt.calendarError("CALENDAR_ERROR_INVALID_DATE", nil)
		}
		return 0
	})
	reg("CalendarEventSetTextureID", func(L *lua.LState) int {
		index, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarEventSetTextureID(textureIndex)")
			return 0
		}
		rt.calEventSetter(func(event *calEvent) {
			if event.eventType != 0 && event.eventType != 1 {
				return
			}
			rows := rt.calTexturesOf(event.eventType)
			if index-1 >= 0 && index-1 < len(rows) && int(rows[index-1].ID) != event.dungeon {
				event.dungeon = int(rows[index-1].ID)
				event.changed = true
			}
		})
		return 0
	})
	flagSetter := func(name string, bit uint32, set bool) {
		reg(name, func(L *lua.LState) int {
			rt.calEventSetter(func(event *calEvent) {
				if set && event.flags&bit == 0 {
					event.flags |= bit
					event.changed = true
				} else if !set && event.flags&bit != 0 {
					event.flags &^= bit
					event.changed = true
				}
			})
			return 0
		})
	}
	flagSetter("CalendarEventSetLocked", calFlagLocked, true)
	flagSetter("CalendarEventClearLocked", calFlagLocked, false)
	flagSetter("CalendarEventSetAutoApprove", calFlagAutoApprove, true)
	flagSetter("CalendarEventClearAutoApprove", calFlagAutoApprove, false)

	// Sending the changes.
	reg("CalendarNewEvent", func(L *lua.LState) int { rt.calNewEvent(0); return 0 })
	reg("CalendarNewGuildAnnouncement", func(L *lua.LState) int { rt.calNewEvent(1); return 0 })
	reg("CalendarNewGuildEvent", func(L *lua.LState) int { rt.calNewEvent(2); return 0 })
	reg("CalendarAddEvent", func(L *lua.LState) int {
		if rt.cal.actionPending {
			return 0
		}
		event := rt.cal.open
		if event == nil {
			rt.calendarError("CALENDAR_ERROR_INTERNAL", nil)
			return 0
		}
		if !rt.cal.lastEvent.IsZero() && time.Since(rt.cal.lastEvent) < 5*time.Second {
			rt.calendarError("CALENDAR_ERROR_EVENT_THROTTLED", nil)
			return 0
		}
		if !rt.calTimeCheck(event.at) {
			return 0
		}
		if rt.calSendNew(event) {
			rt.cal.lastEvent = time.Now()
			rt.calendarSetAction(true)
		}
		return 0
	})
	reg("CalendarUpdateEvent", func(L *lua.LState) int {
		event := rt.cal.open
		if rt.cal.actionPending || event == nil || event.isNew {
			return 0
		}
		if !rt.calEditAllowed(event) {
			rt.calendarError("CALENDAR_ERROR_PERMISSIONS", nil)
			return 0
		}
		if rt.calSendUpdate(event) {
			rt.calendarSetAction(true)
		}
		return 0
	})
	reg("CalendarRemoveEvent", func(L *lua.LState) int { rt.calendarRemoveOpen(); return 0 })
	reg("CalendarEventInvite", func(L *lua.LState) int {
		name, ok := L.Get(1).(lua.LString)
		event := rt.cal.open
		if !ok || event == nil || rt.cal.actionPending || !rt.calCanEdit(event) {
			return 0
		}
		if !rt.cal.lastInvite.IsZero() && time.Since(rt.cal.lastInvite) < 2*time.Second {
			rt.calendarError("CALENDAR_ERROR_INVITE_THROTTLED", nil)
			return 0
		}
		if event.flags&calFlagGuildAnnounce != 0 {
			rt.calendarError("CALENDAR_ERROR_INVITES_DISABLED", nil)
			return 0
		}
		if len(event.invites) >= calMaxInvites {
			rt.calendarError("CALENDAR_ERROR_INVITES_EXCEEDED", calMaxInvites)
			return 0
		}
		for _, invite := range event.invites {
			if strings.EqualFold(rt.nameOf(invite.guid), string(name)) {
				rt.calendarError("CALENDAR_ERROR_ALREADY_INVITED_TO_EVENT_S", string(name))
				return 0
			}
		}
		w := &calWriter{}
		w.u64(event.id)
		w.u64(event.ownerInvite)
		w.cstr(string(name))
		isNew := uint8(0)
		if event.isNew {
			isNew = 1
		}
		w.u8(isNew)
		guild := uint8(0)
		if event.flags&calFlagGuildEvent != 0 {
			guild = 1
		}
		w.u8(guild)
		if rt.calendarSend(calendarInvite, w) {
			rt.cal.lastInvite = time.Now()
			rt.calendarSetAction(true)
		}
		return 0
	})
	reg("CalendarEventRemoveInvite", func(L *lua.LState) int {
		index, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarEventRemoveInvite(index)")
			return 0
		}
		event := rt.cal.open
		if event == nil || rt.cal.actionPending {
			return 0
		}
		if rt.calRemoveInviteAt(event, index-1) {
			if event.isNew {
				rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST")
			} else {
				rt.calendarSetAction(true)
			}
		}
		return 0
	})
	reg("CalendarEventSignUp", func(L *lua.LState) int {
		event := rt.cal.open
		if rt.cal.actionPending || event == nil || event.flags&calFlagGuildEvent == 0 {
			return 0
		}
		if rt.calOwnInvite(event) != nil {
			return 0
		}
		w := &calWriter{}
		w.u64(event.id)
		w.u8(0)
		if rt.calendarSend(calendarSignUp, w) {
			rt.calendarSetAction(true)
		}
		return 0
	})
	reg("CalendarEventAvailable", func(L *lua.LState) int { rt.calAnswer(calStatusAccepted); return 0 })
	reg("CalendarEventDecline", func(L *lua.LState) int { rt.calAnswer(calStatusDeclined); return 0 })
	reg("CalendarEventTentative", func(L *lua.LState) int {
		event := rt.cal.open
		if event == nil || rt.cal.actionPending {
			return 0
		}
		if event.flags&calFlagGuildEvent != 0 && event.guildID == rt.guildIDOfPlayer() {
			if rt.calOwnInvite(event) == nil {
				w := &calWriter{}
				w.u64(event.id)
				w.u8(1)
				if rt.calendarSend(calendarSignUp, w) {
					rt.calendarSetAction(true)
				}
			}
			return 0
		}
		rt.calAnswer(calStatusTentative)
		return 0
	})

	// The invitations of the open event.
	reg("CalendarEventGetNumInvites", func(L *lua.LState) int {
		n := 0
		if rt.cal.open != nil {
			n = len(rt.cal.open.invites)
		}
		L.Push(lua.LNumber(n))
		return 1
	})
	reg("CalendarEventGetInvite", func(L *lua.LState) int {
		index, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarEventGetInvite(index)")
			return 0
		}
		event := rt.cal.open
		if event == nil || index < 1 || index > len(event.invites) {
			return pushAll(L, lua.LNil, lua.LNumber(0), lua.LNil, lua.LNil, lua.LNil, lua.LNil, lua.LFalse, lua.LNumber(0), lua.LNil)
		}
		invite := event.invites[index-1]
		className, classFile := calClassOf(rt, invite.guid)
		mod := ""
		switch invite.rank {
		case 1:
			mod = "MODERATOR"
		case 2:
			mod = "CREATOR"
		}
		return pushAll(L, str(rt.nameOf(invite.guid)), lua.LNumber(invite.level), str(className), str(classFile),
			lua.LNumber(int(invite.status)+1), str(mod), lua.LBool(invite.guid == rt.playerGUID()),
			lua.LNumber(int(invite.inviteTyp)+1), str(invite.notes))
	})
	reg("CalendarEventGetInviteResponseTime", func(L *lua.LState) int {
		index, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarEventGetInviteResponseTime(index)")
			return 0
		}
		event := rt.cal.open
		if event == nil || index < 1 || index > len(event.invites) || event.invites[index-1].statusAt == 0 {
			return pushAll(L, calNumbers(0, 0, 0, 0, 0, 0)...)
		}
		at := unpackCalTime(event.invites[index-1].statusAt)
		return pushAll(L, calNumbers(at.weekday+1, at.month+1, at.day+1, at.year+2000, at.hour, at.minute)...)
	})
	reg("CalendarEventGetSelectedInvite", func(L *lua.LState) int {
		event := rt.cal.open
		if event == nil {
			L.Push(lua.LNumber(0))
			return 1
		}
		for i, invite := range event.invites {
			if invite.guid == event.selected {
				L.Push(lua.LNumber(i + 1))
				return 1
			}
		}
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("CalendarEventSelectInvite", func(L *lua.LState) int {
		index, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarEventSelectInvite(index)")
			return 0
		}
		if event := rt.cal.open; event != nil && index >= 1 && index <= len(event.invites) {
			event.selected = event.invites[index-1].guid
		}
		return 0
	})
	reg("CalendarEventGetInviteSortCriterion", func(L *lua.LState) int {
		names := []string{"name", "level", "class", "status", "party", "notes"}
		if event := rt.cal.open; event != nil {
			return pushAll(L, str(names[event.sortCriterion]), lua.LBool(event.sortReverse[event.sortCriterion]))
		}
		return pushAll(L, str(""), lua.LBool(false))
	})
	reg("CalendarEventSortInvites", func(L *lua.LState) int {
		criterion, ok := L.Get(1).(lua.LString)
		if !ok {
			L.RaiseError("Usage: CalendarEventSortInvites(\"criteria\", reverse)")
			return 0
		}
		which := 3
		for i, name := range []string{"name", "level", "class", "status", "party", "notes"} {
			if strings.EqualFold(string(criterion), name) {
				which = i
				break
			}
		}
		toggle := L.OptBool(2, false)
		if event := rt.cal.open; event != nil {
			for i := range event.sortReverse {
				if i == which {
					if toggle {
						event.sortReverse[i] = !event.sortReverse[i]
					}
				} else {
					event.sortReverse[i] = false
				}
			}
			event.sortCriterion = which
			rt.sortCalendarInvites(event)
			rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST")
		}
		return 0
	})
	reg("CalendarEventCanModerate", func(L *lua.LState) int {
		index, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarEventCanModerate(index)")
			return 0
		}
		event := rt.cal.open
		allowed := false
		if event != nil && index-1 >= 0 && index-1 < len(event.invites) {
			if event.flags&calFlagGuildEvent == 0 || event.invites[index-1].inviteTyp != 0 {
				allowed = true
			} else {
				rt.calendarError("CALENDAR_ERROR_NO_MODERATOR", nil)
			}
		}
		L.Push(lua.LBool(allowed))
		return 1
	})
	moderator := func(name string, flag uint8) {
		reg(name, func(L *lua.LState) int {
			index, ok := calNumber(L, 1)
			if !ok {
				L.RaiseError("Usage: %s(index)", name)
				return 0
			}
			event := rt.cal.open
			if event == nil || !rt.calEditAllowed(event) {
				return 0
			}
			if !rt.calCanEdit(event) {
				rt.calendarError("CALENDAR_ERROR_PERMISSIONS", nil)
				return 0
			}
			index--
			if index < 0 || index >= len(event.invites) || flag > 1 {
				return 0
			}
			if event.flags&calFlagGuildEvent != 0 && event.invites[index].inviteTyp == 0 {
				rt.calendarError("CALENDAR_ERROR_NO_MODERATOR", nil)
				return 0
			}
			invite := event.invites[index]
			if invite.rank == flag {
				return 0
			}
			if event.isNew {
				invite.rank = flag
				rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST")
				return 0
			}
			w := &calWriter{}
			w.packed(invite.guid)
			w.u64(event.id)
			w.u64(invite.inviteID)
			w.u64(event.ownerInvite)
			w.u32(uint32(flag))
			rt.calendarSend(calendarModerator, w)
			return 0
		})
	}
	moderator("CalendarEventSetModerator", 1)
	moderator("CalendarEventClearModerator", 0)
	reg("CalendarEventSetStatus", func(L *lua.LState) int {
		index, ok1 := calNumber(L, 1)
		status, ok2 := calNumber(L, 2)
		if !ok1 || !ok2 {
			L.RaiseError("Usage: CalendarEventSetStatus(index, status)")
			return 0
		}
		event := rt.cal.open
		if event == nil || !rt.calEditAllowed(event) || !(event.isNew || !rt.cal.actionPending) {
			return 0
		}
		index, status = index-1, status-1
		if !rt.calCanSetStatus(event, index, status) {
			return 0
		}
		if event.isNew {
			event.invites[index].status = uint8(status)
			rt.FireEvent("CALENDAR_UPDATE_INVITE_LIST")
			return 0
		}
		invite := event.invites[index]
		w := &calWriter{}
		w.packed(invite.guid)
		w.u64(event.id)
		w.u64(invite.inviteID)
		w.u64(event.ownerInvite)
		w.u32(uint32(status))
		rt.calendarSend(calendarStatus, w)
		return 0
	})

	// The entries of the days: context menus, copying and complaints.
	reg("CalendarContextEventGetCalendarType", func(L *lua.LState) int {
		entry, given := rt.calEntryArgs(L)
		if !given {
			entry = rt.cal.context
			if entry == nil {
				L.RaiseError("Usage: CalendarContextEventGetCalendarType([-1.0.1], monthDay, index)")
				return 0
			}
		}
		if entry == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(calendarType(entry.flags)))
		return 1
	})
	reg("CalendarContextEventCanEdit", func(L *lua.LState) int {
		entry, given := rt.calEntryArgs(L)
		if !given {
			entry = rt.cal.context
			if entry == nil {
				L.RaiseError("Usage: CalendarContextEventCanEdit([-1.0.1], monthDay, index)")
				return 0
			}
		}
		L.Push(lua.LBool(entry != nil && rt.calEntryEditable(entry)))
		return 1
	})
	reg("CalendarContextEventCanComplain", func(L *lua.LState) int {
		entry, given := rt.calEntryArgs(L)
		if !given {
			entry = rt.cal.context
			if entry == nil {
				L.RaiseError("Usage: CalendarContextEventCanComplain([-1.0.1], monthDay, index)")
				return 0
			}
		}
		L.Push(lua.LBool(!rt.cal.actionPending && entry != nil && rt.calCanComplain(entry)))
		return 1
	})
	reg("CalendarContextEventComplain", func(L *lua.LState) int {
		entry, ok := rt.calContext(L, "Usage: CalendarContextEventComplain([-1.0.1], monthDay, index)")
		if ok && !rt.cal.actionPending && rt.calCanComplain(entry) {
			rt.calComplain(entry)
		}
		return 0
	})
	reg("CalendarContextEventClipboard", func(L *lua.LState) int { L.Push(lua.LBool(rt.cal.clipboard != nil)); return 1 })
	reg("CalendarContextEventCopy", func(L *lua.LState) int {
		entry, ok := rt.calContext(L, "Usage: Script_CalendarContextEventCopy([-1.0.1], monthDay, index)")
		if !ok {
			return 0
		}
		if !rt.calEntryEditable(entry) {
			rt.calendarError("CALENDAR_ERROR_PERMISSIONS", nil)
			return 0
		}
		copied := *entry
		rt.cal.clipboard = &copied
		return 0
	})
	reg("CalendarContextEventPaste", func(L *lua.LState) int {
		offset, ok1 := calNumber(L, 1)
		day, ok2 := calNumber(L, 2)
		if !ok1 || !ok2 {
			L.RaiseError("Usage: Script_CalendarContextEventPaste([-1.0.1], monthDay)")
			return 0
		}
		rt.calPaste(offset, day-1)
		return 0
	})
	reg("CalendarContextEventRemove", func(L *lua.LState) int {
		entry, ok := rt.calContext(L, "Usage: Script_CalendarContextEventRemove([-1.0.1], monthDay, index)")
		if !ok {
			return 0
		}
		if !rt.calEntryEditable(entry) {
			rt.calendarError("CALENDAR_ERROR_PERMISSIONS", nil)
			return 0
		}
		if !rt.cal.actionPending {
			w := &calWriter{}
			w.u64(entry.id)
			w.u64(entry.inviteID)
			guild := uint8(0)
			if entry.flags&calFlagGuildAny != 0 {
				guild = 1
			}
			w.u8(guild)
			if rt.calendarSend(calendarRemoveEvent, w) {
				rt.calendarSetAction(true)
			}
		}
		return 0
	})
	reg("CalendarContextEventSignUp", func(L *lua.LState) int {
		entry, ok := rt.calContext(L, "Usage: CalendarContextEventSignUp([-1.0.1], monthDay, index)")
		if ok && entry.flags&calFlagGuildEvent != 0 && !rt.cal.actionPending {
			w := &calWriter{}
			w.u64(entry.id)
			w.u8(0)
			if rt.calendarSend(calendarSignUp, w) {
				rt.calendarSetAction(true)
			}
		}
		return 0
	})
	reg("CalendarContextInviteRemove", func(L *lua.LState) int {
		entry, ok := rt.calContext(L, "Usage: CalendarContextInviteRemove([-1.0.1], monthDay, index)")
		if ok && entry.flags&(calFlagGuildEvent|calFlagPlayer) != 0 && !rt.cal.actionPending {
			w := &calWriter{}
			w.packed(rt.playerGUID())
			w.u64(entry.inviteID)
			w.u64(0)
			w.u64(entry.id)
			if rt.calendarSend(calendarRemoveInv, w) {
				rt.calendarSetAction(true)
			}
		}
		return 0
	})
	answer := func(name string, status uint32) {
		reg(name, func(L *lua.LState) int {
			entry, ok := rt.calContext(L, "Usage: "+name+"([-1.0.1], monthDay, index)")
			if !ok || rt.cal.actionPending {
				return 0
			}
			signUp := entry.flags&calFlagGuildEvent != 0 && entry.mod&calModSignUp != 0
			switch {
			case status == 8 && signUp:
				w := &calWriter{}
				w.u64(entry.id)
				w.u8(1)
				if rt.calendarSend(calendarSignUp, w) {
					rt.calendarSetAction(true)
				}
			case signUp:
			default:
				rt.calSendRSVP(entry.id, entry.inviteID, status)
				rt.calendarSetAction(true)
			}
			return 0
		})
	}
	answer("CalendarContextInviteAvailable", calStatusAccepted)
	answer("CalendarContextInviteDecline", calStatusDeclined)
	answer("CalendarContextInviteTentative", calStatusTentative)
	reg("CalendarContextInviteType", func(L *lua.LState) int {
		entry, given := rt.calEntryArgs(L)
		if !given {
			entry = rt.cal.context
			if entry == nil {
				L.RaiseError("Usage: CalendarContextInviteType([-1.0.1], monthDay, index)")
				return 0
			}
		}
		switch {
		case entry == nil:
			L.Push(lua.LNumber(0))
		case entry.flags&calFlagGuildEvent != 0 && entry.mod&calModSignUp != 0:
			L.Push(lua.LNumber(2))
		default:
			L.Push(lua.LNumber(1))
		}
		return 1
	})
	reg("CalendarContextInviteStatus", func(L *lua.LState) int {
		entry, given := rt.calEntryArgs(L)
		if !given {
			entry = rt.cal.context
			if entry == nil {
				L.RaiseError("Usage: CalendarContextInviteStatus([-1.0.1], monthDay, index)")
				return 0
			}
		}
		if entry == nil {
			L.Push(lua.LNumber(0))
		} else {
			L.Push(lua.LNumber(int(entry.status) + 1))
		}
		return 1
	})
	reg("CalendarContextInviteModeratorStatus", func(L *lua.LState) int {
		entry, given := rt.calEntryArgs(L)
		if !given {
			entry = rt.cal.context
			if entry == nil {
				L.RaiseError("Usage: CalendarContextInviteModeratorStatus([-1.0.1], monthDay, index)")
				return 0
			}
		}
		if entry != nil {
			if text := calendarModStatus(entry.mod); text != "" {
				L.Push(lua.LString(text))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("CalendarContextInviteIsPending", func(L *lua.LState) int {
		entry, given := rt.calEntryArgs(L)
		if !given && rt.cal.context == nil {
			L.RaiseError("Usage: CalendarContextInviteIsPending([-1.0.1], monthDay, index)")
			return 0
		}
		if !given {
			entry = rt.cal.context
		}
		L.Push(lua.LBool(entry != nil && entry.mod&calModPending != 0))
		return 1
	})
	reg("CalendarDefaultGuildFilter", func(L *lua.LState) int {
		if rt.playerGUID() == 0 {
			return pushAll(L, calNumbers(0, 0, 0)...)
		}
		levels := []int{60, 70, 80}
		level := levels[min(int(rt.lfg.level), 2)]
		return pushAll(L, lua.LNumber(level), lua.LNumber(level), lua.LNumber(len(rt.guild.roster.Ranks)))
	})
	reg("CalendarMassInviteGuild", func(L *lua.LState) int {
		low, ok1 := calNumber(L, 1)
		high, ok2 := calNumber(L, 2)
		rank, ok3 := calNumber(L, 3)
		if !ok1 && !ok2 && !ok3 {
			L.RaiseError("Usage: CalendarMassInviteGuild(minLevel, maxLevel, minRank)")
			return 0
		}
		if rt.cal.actionPending {
			return 0
		}
		if rt.guild.id == 0 {
			rt.calendarError("ERR_GUILD_PLAYER_NOT_IN_GUILD", nil)
			return 0
		}
		if event := rt.cal.open; event != nil && event.isNew && event.flags&calFlagGuildAny == 0 {
			w := &calWriter{}
			w.u32(uint32(low))
			w.u32(uint32(high))
			w.u32(uint32(rank - 1))
			rt.calendarSend(calendarGuildFilter, w)
			rt.calendarSetAction(true)
		}
		return 0
	})
	reg("CalendarMassInviteArenaTeam", func(L *lua.LState) int {
		index, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarNewArenaTeamEvent(index)")
			return 0
		}
		index--
		if rt.cal.actionPending || index < 0 || index >= len(rt.arena.teams) || rt.arena.teams[index].id == 0 {
			return 0
		}
		if event := rt.cal.open; event != nil && event.isNew && event.flags&calFlagGuildAny == 0 {
			w := &calWriter{}
			w.u32(rt.arena.teams[index].id)
			rt.calendarSend(calendarArenaInvite, w)
			rt.calendarSetAction(true)
		}
		return 0
	})
}

// calCanSetStatus reports whether an invitation of the open event may be set
// to a status.
func (rt *Runtime) calCanSetStatus(event *calEvent, index, status int) bool {
	if !rt.calCanEdit(event) || index < 0 || index >= len(event.invites) || status < 0 || status >= 9 {
		return false
	}
	if status == calStatusInvited || status == calStatusSignedUp || status == calStatusNotSigned {
		return false
	}
	if event.flags&calFlagGuildEvent != 0 && (status == calStatusAccepted || status == calStatusDeclined) {
		return false
	}
	return status != int(event.invites[index].status)
}

// calCanComplain reports whether an entry may be complained about: another
// player made it and the character has not ignored the player.
func (rt *Runtime) calCanComplain(entry *calEntry) bool {
	if entry.flags&calFlagOthers != 0 {
		return false
	}
	return entry.creator != 0 && entry.creator != rt.playerGUID() && !rt.isIgnoredGUID(entry.creator)
}

// calComplain reports the maker of an event.
func (rt *Runtime) calComplain(entry *calEntry) {
	if rt.cal.complained == nil {
		rt.cal.complained = map[uint64]bool{}
	}
	if !rt.cal.complained[entry.creator] {
		rt.cal.complained[entry.creator] = true
		w := &calWriter{}
		w.u64(entry.creator)
		w.u64(entry.id)
		w.u64(entry.inviteID)
		rt.calendarSend(calendarComplain, w)
	} else {
		rt.say("CHAT_MSG_SYSTEM", rt.globalString("COMPLAINT_ADDED", "Your complaint has been registered."))
		w := &calWriter{}
		w.packed(rt.playerGUID())
		w.u64(entry.inviteID)
		w.u64(0)
		w.u64(entry.id)
		rt.calendarSend(calendarRemoveInv, w)
	}
	rt.calendarSetAction(true)
}

// calPaste makes a copy of the event on the clipboard on a day.
func (rt *Runtime) calPaste(offset, day int) {
	entry := rt.cal.clipboard
	if rt.cal.actionPending || entry == nil {
		return
	}
	if !rt.cal.lastEvent.IsZero() && time.Since(rt.cal.lastEvent) < 5*time.Second {
		rt.calendarError("CALENDAR_ERROR_EVENT_THROTTLED", nil)
		return
	}
	month, year, days, _ := rt.calendarMonth(offset)
	if day < 0 || day >= days {
		return
	}
	at := entry.start
	at.day, at.month, at.year = day, int(month)-1, year-2000
	at.weekday = int(time.Date(year, month, day+1, 0, 0, 0, 0, time.Local).Weekday())
	if !rt.calTimeCheck(at) {
		return
	}
	w := &calWriter{}
	w.u64(entry.id)
	w.u64(entry.inviteID)
	w.u32(at.pack())
	if rt.calendarSend(calendarCopyEvent, w) {
		rt.cal.lastEvent = time.Now()
		rt.calendarSetAction(true)
	}
}

func truncateBytes(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}
