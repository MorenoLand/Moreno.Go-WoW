package ui

import (
	"encoding/binary"
	"sort"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The calendar keeps what the server sent of the events the character may
// see: the invitations, the events, the raids it is saved to and when the
// raids reset. They are laid out on the days of the month shown and the month
// before and after it; the interface reads them day by day.
const (
	calendarSendCalendar   = 0x0436
	calendarSendEvent      = 0x0437
	calendarFilterGuild    = 0x0438
	calendarArenaTeam      = 0x0439
	calendarEventInvite    = 0x043A
	calendarInviteRemoved  = 0x043B
	calendarEventStatus    = 0x043C
	calendarCommandResult  = 0x043D
	calendarLockoutAdded   = 0x043E
	calendarLockoutRemoved = 0x043F
	calendarInviteAlert    = 0x0440
	calendarRemovedAlert   = 0x0441
	calendarStatusAlert    = 0x0442
	calendarEventRemoved   = 0x0443
	calendarEventUpdated   = 0x0444
	calendarModeratorAlert = 0x0445
	calendarSendNumPending = 0x0448
	calendarInviteNotes    = 0x0460
	calendarNotesAlert     = 0x0461
	calendarLockoutUpdated = 0x0471
	calendarClearPending   = 0x04BB

	calendarGetCalendar = 0x0429
	calendarGetEvent    = 0x042A
	calendarGuildFilter = 0x042B
	calendarArenaInvite = 0x042C
	calendarAddEvent    = 0x042D
	calendarUpdateEvent = 0x042E
	calendarRemoveEvent = 0x042F
	calendarCopyEvent   = 0x0430
	calendarInvite      = 0x0431
	calendarRSVP        = 0x0432
	calendarRemoveInv   = 0x0433
	calendarStatus      = 0x0434
	calendarModerator   = 0x0435
	calendarComplain    = 0x0446
	calendarGetPending  = 0x0447
	calendarSignUp      = 0x04BA
)

// Calendar flags of an entry. The server's own flags (all allowed, locked,
// without invites, guild event) share the word with the ones the client sets
// for what it makes up itself.
const (
	calFlagPlayer        = 0x001
	calFlagPlayerAlt     = 0x002
	calFlagSystem        = 0x004
	calFlagHoliday       = 0x008
	calFlagLocked        = 0x010
	calFlagAutoApprove   = 0x020
	calFlagGuildAnnounce = 0x040
	calFlagLockout       = 0x080
	calFlagPlayerOther   = 0x100
	calFlagReset         = 0x200
	calFlagGuildEvent    = 0x400
	calFlagGuildAny      = calFlagGuildAnnounce | calFlagGuildEvent
	// calFlagEventKinds are the flags of what a player or guild made.
	calFlagEventKinds = 0x543
	// calFlagOthers are the flags of the entries without an invitation list.
	calFlagOthers = 0x6cc
)

// What the player may do with an entry (the word of an entry that follows the
// flags).
const (
	calModPending   = 1
	calModModerator = 2
	calModCreator   = 4
	calModSignUp    = 8
)

// Invitation statuses.
const (
	calStatusInvited   = 0
	calStatusAccepted  = 1
	calStatusDeclined  = 2
	calStatusConfirmed = 3
	calStatusOut       = 4
	calStatusStandby   = 5
	calStatusSignedUp  = 6
	calStatusNotSigned = 7
	calStatusTentative = 8
)

// calMaxInvites is how many invitations an event takes.
const calMaxInvites = 100

var calStatusNames = [...]string{"CALENDAR_STATUS_INVITED", "CALENDAR_STATUS_ACCEPTED", "CALENDAR_STATUS_DECLINED",
	"CALENDAR_STATUS_CONFIRMED", "CALENDAR_STATUS_OUT", "CALENDAR_STATUS_STANDBY", "CALENDAR_STATUS_SIGNEDUP",
	"", "CALENDAR_STATUS_TENTATIVE"}

var calTypeNames = [...]string{"CALENDAR_TYPE_RAID", "CALENDAR_TYPE_DUNGEON", "CALENDAR_TYPE_PVP", "CALENDAR_TYPE_MEETING", "CALENDAR_TYPE_OTHER"}

var calRepeatNames = [...]string{"CALENDAR_REPEAT_NEVER", "CALENDAR_REPEAT_WEEKLY", "CALENDAR_REPEAT_BIWEEKLY", "CALENDAR_REPEAT_MONTHLY"}

// calStatusOrder is how the invite list sorts the statuses.
var calStatusOrder = [...]int{3, 2, 4, 0, 7, 5, 1, 6, 8}

// calTime is a time as the server packs it: broken down, in the realm's own
// time. Day and month count from zero, the year from 2000. A field of -1 is
// not given (the dates of the holidays leave fields open).
type calTime struct {
	minute, hour, weekday, day, month, year int
}

func unpackCalTime(packed uint32) calTime {
	return calTime{
		minute: int(packed & 0x3f), hour: int(packed >> 6 & 0x1f), weekday: int(packed >> 11 & 7),
		day: int(packed >> 14 & 0x3f), month: int(packed >> 20 & 0xf), year: int(packed >> 24 & 0x1f),
	}
}

func (t calTime) pack() uint32 {
	return uint32(t.minute&0x3f) | uint32(t.hour&0x1f)<<6 | uint32(t.weekday&7)<<11 | uint32(t.day&0x3f)<<14 |
		uint32(t.month&0xf)<<20 | uint32(t.year&0x1f)<<24
}

func (t calTime) time() time.Time {
	return time.Date(2000+t.year, time.Month(t.month+1), t.day+1, t.hour, t.minute, 0, 0, time.Local)
}

func calTimeOf(at time.Time) calTime {
	return calTime{minute: at.Minute(), hour: at.Hour(), weekday: int(at.Weekday()), day: at.Day() - 1, month: int(at.Month()) - 1, year: at.Year() - 2000}
}

// calEntry is one thing on a day of the calendar.
type calEntry struct {
	id    uint64
	title string
	start calTime
	// end is the last moment of a holiday that runs over days.
	end    calTime
	hasEnd bool
	flags  uint32
	mod    uint32
	// eventType is the kind of the event, -1 when it has none.
	eventType int
	// difficulty and mapID say which raid a lockout or reset is of.
	difficulty uint32
	mapID      uint32
	// dungeon is the dungeon an event is for.
	dungeon  int
	status   uint8
	inviteID uint64
	creator  uint64
	// seqIndex and seqDays are the day of a run of days this entry stands on,
	// and how many days it runs.
	seqIndex, seqDays int
	instance          uint64
	// holiday is the holiday of an entry that stands for one.
	holiday uint32
}

// calPending is an invitation the server named in the calendar.
type calPending struct {
	eventID, inviteID uint64
	status, rank      uint8
	guildFlag         uint8
	inviter           uint64
	pending           bool
}

// calLockout is a raid the character is saved to.
type calLockout struct {
	mapID, difficulty uint32
	resetAt           time.Time
	instance          uint64
}

// calReset is how a raid resets over the weeks.
type calReset struct {
	mapID  uint32
	last   time.Time
	period int
}

// calInvite is one invitee of the event that is open.
type calInvite struct {
	inviteID  uint64
	guid      uint64
	statusAt  uint32
	level     uint8
	status    uint8
	rank      uint8
	inviteTyp uint8
	notes     string
}

// calEvent is the event that is open: what the server said of it, and what
// the interface has changed since.
type calEvent struct {
	id, creator uint64
	title       string
	description string
	eventType   int
	repeat      int
	maxSize     uint32
	at          calTime
	lockoutAt   calTime
	flags       uint32
	dungeon     int
	guildID     uint32
	// ownerInvite is the invite of the one who may change the event.
	ownerInvite uint64
	// isNew is an event that has not been sent yet, changed whether the
	// interface changed it since it was read.
	isNew, changed bool
	invites        []*calInvite
	selected       uint64
	sortCriterion  int
	sortReverse    [6]bool
}

type calendarData struct {
	// received is set once the server sent the calendar; requested once it
	// was asked for.
	received, requested bool
	invites             []*calPending
	events              []*calEntry
	lockouts            []calLockout
	resets              []calReset
	// clockOffset is how far the realm's clock is from this machine's.
	clockOffset time.Duration
	grid        [3][31][]*calEntry
	laidOut     bool
	open        *calEvent
	// openRef is the event the interface named last (a day entry), and
	// contextRef the one a menu is open for.
	openRef, contextRef calRef
	clipboard           *calEntry
	context             *calEntry
	complained          map[uint64]bool
	serverPending       int
	actionPending       bool
	lastEvent           time.Time
	lastInvite          time.Time
	holidays            map[uint32]*CalendarHoliday
}

// calRef names an entry of the grid by what identifies it.
type calRef struct {
	id         uint64
	start      calTime
	eventType  int
	flags      uint32
	difficulty uint32
	mapID      uint32
	set        bool
}

func refOf(entry *calEntry) calRef {
	return calRef{id: entry.id, start: entry.start, eventType: entry.eventType, flags: entry.flags,
		difficulty: entry.difficulty, mapID: entry.mapID, set: true}
}

func (rt *Runtime) calendarNow() time.Time { return time.Now().Add(rt.cal.clockOffset) }

// calendarBase is the month the grid is laid out around.
func (rt *Runtime) calendarBase() (int, int) {
	month, year := rt.calendarViewed()
	return int(month) - 1, year - 2000
}

func (rt *Runtime) calendarOffsetOf(month, year int) int {
	baseMonth, baseYear := rt.calendarBase()
	return (year-baseYear)*12 + month - baseMonth
}

// calendarCell is the entries of a day; offset is -1, 0 or 1 from the month
// shown and day counts from zero.
func (rt *Runtime) calendarCell(offset, day int) []*calEntry {
	if offset < -1 || offset > 1 || day < 0 || day >= 31 {
		return nil
	}
	rt.calendarEnsure()
	return rt.cal.grid[offset+1][day]
}

func (rt *Runtime) calendarEntry(offset, day, index int) *calEntry {
	cell := rt.calendarCell(offset, day)
	if index < 0 || index >= len(cell) {
		return nil
	}
	return cell[index]
}

// calendarEnsure asks the server for the calendar the first time the
// interface wants it, and lays it out when something changed.
func (rt *Runtime) calendarEnsure() {
	data := &rt.cal
	if !data.requested {
		data.requested = true
		rt.send(calendarGetCalendar, nil)
		rt.send(calendarGetPending, nil)
	}
	if !data.laidOut {
		rt.calendarLayOut()
	}
}

// calendarPlace puts an entry on the days it covers.
func (rt *Runtime) calendarPlace(entry *calEntry) {
	first := entry.start.time()
	last := first
	if entry.hasEnd {
		last = entry.end.time()
	}
	days := 0
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		days++
		if days > 62 {
			break
		}
	}
	index := 0
	for day := first; index < days; day = day.AddDate(0, 0, 1) {
		copyOf := *entry
		copyOf.seqIndex, copyOf.seqDays = index, days
		if index > 0 {
			copyOf.start = calTimeOf(day)
			copyOf.start.hour, copyOf.start.minute = 0, 0
		}
		offset := rt.calendarOffsetOf(int(day.Month())-1, day.Year()-2000)
		if offset >= -1 && offset <= 1 {
			cell := &rt.cal.grid[offset+1][day.Day()-1]
			*cell = append(*cell, &copyOf)
		}
		index++
	}
}

// calendarLayOut places everything the server told on the days of the three
// months, in the order the interface lists them.
func (rt *Runtime) calendarLayOut() {
	data := &rt.cal
	for i := range data.grid {
		for d := range data.grid[i] {
			data.grid[i][d] = nil
		}
	}
	for _, event := range data.events {
		if event.creator != 0 && rt.isIgnoredGUID(event.creator) {
			continue
		}
		rt.calendarPlace(event)
	}
	for _, lockout := range data.lockouts {
		entry := &calEntry{id: lockout.instance, flags: calFlagLockout, eventType: -1, dungeon: -1, difficulty: lockout.difficulty,
			mapID: lockout.mapID, start: calTimeOf(lockout.resetAt), instance: lockout.instance}
		rt.calendarPlace(entry)
	}
	rt.calendarPlaceResets()
	rt.calendarPlaceHolidays()
	for i := range data.grid {
		for d := range data.grid[i] {
			cell := data.grid[i][d]
			sort.SliceStable(cell, func(a, b int) bool { return rt.calendarBefore(cell[a], cell[b]) })
		}
	}
	data.laidOut = true
}

// calendarPlaceResets puts the raid resets that are still to come on their
// days.
func (rt *Runtime) calendarPlaceResets() {
	month, year := rt.calendarBase()
	windowStart := time.Date(2000+year, time.Month(month+1)-1, 1, 0, 0, 0, 0, time.Local)
	windowEnd := time.Date(2000+year, time.Month(month+1)+2, 1, 0, 0, 0, 0, time.Local)
	today := rt.calendarNow()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.Local)
	for _, reset := range rt.cal.resets {
		if reset.period <= 0 {
			continue
		}
		at := reset.last
		if at.Before(windowStart) {
			steps := int(windowStart.Sub(at) / (time.Duration(reset.period) * time.Second))
			at = at.Add(time.Duration(steps*reset.period) * time.Second)
		}
		for ; at.Before(windowEnd); at = at.Add(time.Duration(reset.period) * time.Second) {
			if at.Before(today) {
				continue
			}
			rt.calendarPlace(&calEntry{flags: calFlagReset, eventType: -1, dungeon: -1, mapID: reset.mapID, start: calTimeOf(at)})
		}
	}
}

// calendarBefore orders two entries of a day: pending invitations first,
// then what players and guilds made, holidays by their priority, and the
// rest by when they start and by name.
func (rt *Runtime) calendarBefore(a, b *calEntry) bool {
	if pa, pb := a.mod&calModPending != 0, b.mod&calModPending != 0; pa != pb {
		return pa
	}
	if ka, kb := a.flags&calFlagEventKinds != 0, b.flags&calFlagEventKinds != 0; ka != kb {
		return ka
	}
	if ha, hb := a.flags&calFlagHoliday != 0, b.flags&calFlagHoliday != 0; ha != hb {
		return ha
	} else if ha {
		pa, pb := -1, -1
		if h := rt.cal.holidays[a.holiday]; h != nil {
			pa = int(h.Priority)
		}
		if h := rt.cal.holidays[b.holiday]; h != nil {
			pb = int(h.Priority)
		}
		if pa != pb {
			return pa < pb
		}
		sa, sb := calSequenceKind(a), calSequenceKind(b)
		if sa != sb {
			return sa < sb
		}
	}
	if a.start.hour != b.start.hour {
		return a.start.hour < b.start.hour
	}
	if a.start.minute != b.start.minute {
		return a.start.minute < b.start.minute
	}
	return strings.ToLower(rt.calendarTitle(a)) < strings.ToLower(rt.calendarTitle(b))
}

// calSequenceKind is where an entry stands in a run of days: 1 for the first,
// 2 for the days in between, 3 for the last, 4 for a single day.
func calSequenceKind(e *calEntry) int {
	if e.seqDays < 2 {
		return 4
	}
	if e.seqIndex == 0 {
		return 1
	}
	if e.seqIndex == e.seqDays-1 {
		return 3
	}
	return 2
}

// calendarSequence is the position of a day of an entry that runs over days.
func calendarSequence(e *calEntry) string {
	if e.flags&calFlagHoliday == 0 || e.seqDays < 2 {
		return ""
	}
	switch calSequenceKind(e) {
	case 1:
		return "START"
	case 3:
		return "END"
	}
	return "ONGOING"
}

// calendarTitle is the name an entry is listed under.
func (rt *Runtime) calendarTitle(e *calEntry) string {
	switch {
	case e.flags&calFlagHoliday != 0:
		if h := rt.cal.holidays[e.holiday]; h != nil {
			return h.Name
		}
		return ""
	case e.flags&(calFlagLockout|calFlagReset) != 0:
		return rt.mapName(e.mapID)
	}
	return e.title
}

// calendarType is the kind of an entry the interface names.
func calendarType(flags uint32) string {
	if flags&calFlagPlayer == 0 {
		switch {
		case flags&calFlagGuildAnnounce != 0:
			return "GUILD_ANNOUNCEMENT"
		case flags&calFlagGuildEvent != 0:
			return "GUILD_EVENT"
		case flags&calFlagSystem != 0:
			return "SYSTEM"
		case flags&calFlagHoliday != 0:
			return "HOLIDAY"
		case flags&calFlagLockout != 0:
			return "RAID_LOCKOUT"
		case flags&calFlagReset != 0:
			return "RAID_RESET"
		case flags&(calFlagPlayerAlt|calFlagPlayerOther) == 0:
			return ""
		}
	}
	return "PLAYER"
}

// calendarModStatus names what the player may do with an entry.
func calendarModStatus(mod uint32) string {
	switch {
	case mod&calModCreator != 0:
		return "CREATOR"
	case mod&calModModerator != 0:
		return "MODERATOR"
	}
	return ""
}

// pendingInviteCount is how many invitations wait for an answer.
func (rt *Runtime) pendingInviteCount() int {
	count := 0
	for _, invite := range rt.cal.invites {
		if invite.inviter != 0 && !rt.isIgnoredGUID(invite.inviter) {
			count++
		}
	}
	return max(rt.cal.serverPending, count)
}

func (rt *Runtime) isIgnoredGUID(guid uint64) bool {
	for _, contact := range rt.social.ignored {
		if contact.GUID == guid {
			return true
		}
	}
	return false
}

func (rt *Runtime) calendarFindInvite(eventID uint64) *calPending {
	for _, invite := range rt.cal.invites {
		if invite.eventID == eventID {
			return invite
		}
	}
	return nil
}

func (rt *Runtime) calendarDropInvite(eventID uint64) bool {
	for i, invite := range rt.cal.invites {
		if invite.eventID == eventID {
			rt.cal.invites = append(rt.cal.invites[:i], rt.cal.invites[i+1:]...)
			return true
		}
	}
	return false
}

// calendarFindEvent is the entry of the events the server told about.
func (rt *Runtime) calendarFindEvent(id uint64) *calEntry {
	for _, event := range rt.cal.events {
		if event.id == id {
			return event
		}
	}
	return nil
}

func (rt *Runtime) calendarDropEvent(id uint64) bool {
	for i, event := range rt.cal.events {
		if event.id == id {
			rt.cal.events = append(rt.cal.events[:i], rt.cal.events[i+1:]...)
			return true
		}
	}
	return false
}

// calendarRelayOut lays the entries out again and tells the interface.
func (rt *Runtime) calendarRelayOut(notify bool) {
	rt.cal.laidOut = false
	rt.calendarLayOut()
	if notify {
		rt.FireEvent("CALENDAR_UPDATE_EVENT_LIST")
	}
}

// calendarSetAction sets whether the calendar waits for the server's answer
// to an action.
func (rt *Runtime) calendarSetAction(waiting bool) {
	rt.cal.actionPending = waiting
	rt.FireEvent("CALENDAR_ACTION_PENDING", lua.LBool(waiting))
}

// calendarError shows the text of a calendar error the way the interface
// expects it and ends the wait for an answer.
func (rt *Runtime) calendarError(key string, arg any) {
	text := rt.globalString(key, key)
	if arg != nil {
		text = calFormat(text, arg)
	}
	rt.FireEvent("CALENDAR_UPDATE_ERROR", lua.LString(text))
	rt.calendarSetAction(false)
}

// calFormat puts one value into the %d or %s of a text.
func calFormat(text string, arg any) string {
	for _, verb := range []string{"%d", "%s", "%1$s", "%1$d"} {
		if strings.Contains(text, verb) {
			switch v := arg.(type) {
			case int:
				return strings.Replace(text, verb, itoa(v), 1)
			case string:
				return strings.Replace(text, verb, v, 1)
			}
		}
	}
	return text
}

// calWriter builds the body of a calendar request.
type calWriter struct{ body []byte }

func (w *calWriter) u8(v uint8)   { w.body = append(w.body, v) }
func (w *calWriter) u32(v uint32) { w.body = binary.LittleEndian.AppendUint32(w.body, v) }
func (w *calWriter) u64(v uint64) { w.body = binary.LittleEndian.AppendUint64(w.body, v) }
func (w *calWriter) packed(v uint64) {
	w.body = world.AppendPackedGUID(w.body, v)
}
func (w *calWriter) cstr(s string) { w.body = append(append(w.body, s...), 0) }

func (rt *Runtime) calendarSend(opcode world.ClientOpcode, w *calWriter) bool {
	var body []byte
	if w != nil {
		body = w.body
	}
	return rt.send(opcode, body)
}
