package ui

import (
	"time"

	lua "github.com/yuin/gopher-lua"
)

// calendarState is the month the calendar window shows.
type calendarState struct {
	month time.Month
	year  int
	set   bool
}

// calendarMinDate is the first day the calendar can be paged back to: the day
// the game launched.
var calendarMinDate = time.Date(2004, time.November, 23, 0, 0, 0, 0, time.Local)

// weekdayNumber is the calendar's weekday: 1 is Sunday.
func weekdayNumber(day time.Weekday) int { return int(day) + 1 }

func (rt *Runtime) calendarViewed() (time.Month, int) {
	state := &rt.calendar
	if !state.set {
		now := time.Now()
		state.month, state.year, state.set = now.Month(), now.Year(), true
	}
	return state.month, state.year
}

// calendarMonth is the month a number of months from the viewed one.
func (rt *Runtime) calendarMonth(offset int) (month time.Month, year, days, firstWeekday int) {
	viewed, viewedYear := rt.calendarViewed()
	first := time.Date(viewedYear, viewed+time.Month(offset), 1, 0, 0, 0, 0, time.Local)
	days = first.AddDate(0, 1, -1).Day()
	return first.Month(), first.Year(), days, weekdayNumber(first.Weekday())
}

// registerCalendarAPI defines the date calls of the calendar. Events and
// invitations are not carried: the calendar shows the days and no events.
func registerCalendarAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	pushDate := func(L *lua.LState, at time.Time) int {
		L.Push(lua.LNumber(weekdayNumber(at.Weekday())))
		L.Push(lua.LNumber(int(at.Month())))
		L.Push(lua.LNumber(at.Day()))
		L.Push(lua.LNumber(at.Year()))
		return 4
	}
	reg("CalendarGetDate", func(L *lua.LState) int { return pushDate(L, time.Now()) })
	reg("CalendarGetMinDate", func(L *lua.LState) int { return pushDate(L, calendarMinDate) })
	reg("CalendarGetMaxCreateDate", func(L *lua.LState) int { return pushDate(L, time.Now().AddDate(1, 0, 0)) })
	reg("CalendarGetMonth", func(L *lua.LState) int {
		month, year, days, weekday := rt.calendarMonth(L.OptInt(1, 0))
		L.Push(lua.LNumber(int(month)))
		L.Push(lua.LNumber(year))
		L.Push(lua.LNumber(days))
		L.Push(lua.LNumber(weekday))
		return 4
	})
	reg("CalendarSetAbsMonth", func(L *lua.LState) int {
		rt.calendar.month, rt.calendar.year, rt.calendar.set = time.Month(L.CheckInt(1)), L.CheckInt(2), true
		rt.FireEvent("CALENDAR_UPDATE_EVENT_LIST")
		return 0
	})
	reg("CalendarSetMonth", func(L *lua.LState) int {
		month, year, _, _ := rt.calendarMonth(L.CheckInt(1))
		rt.calendar.month, rt.calendar.year, rt.calendar.set = month, year, true
		rt.FireEvent("CALENDAR_UPDATE_EVENT_LIST")
		return 0
	})
	reg("CalendarGetNumDayEvents", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("CalendarGetNumPendingInvites", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("CalendarGetFirstPendingInvite", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("CalendarGetEventIndex", func(L *lua.LState) int { return 0 })
	reg("CalendarGetDayEvent", func(L *lua.LState) int { return 0 })
	reg("CalendarCanSendInvite", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("CalendarCanAddEvent", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("CalendarIsActionPending", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("CalendarEventGetNumInvites", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
}
