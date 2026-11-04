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

// weekdayNumber is the calendar's weekday: 1 is Sunday.
func weekdayNumber(day time.Weekday) int { return int(day) + 1 }

func (rt *Runtime) calendarViewed() (time.Month, int) {
	state := &rt.calendar
	if !state.set {
		now := rt.calendarNow()
		month, year := calendarClamp(int(now.Month())-1, now.Year()-2000)
		state.month, state.year, state.set = time.Month(month+1), 2000+year, true
	}
	return state.month, state.year
}

// calendarClamp keeps a month inside the years the calendar goes back to and
// forward to: November 2004 to 2030.
func calendarClamp(month, year int) (int, int) {
	if year > 30 {
		year = 30
	}
	if year < 4 || (year == 4 && month < 10) {
		year = 4
		if month < 10 {
			month = 10
		}
	}
	return month, year
}

// calendarMonth is the month a number of months from the viewed one, with the
// number of its days and the weekday it starts on.
func (rt *Runtime) calendarMonth(offset int) (month time.Month, year, days, firstWeekday int) {
	viewed, viewedYear := rt.calendarViewed()
	index := (viewedYear-2000)*12 + int(viewed) - 1 + offset
	m, y := index%12, index/12
	if m < 0 {
		m += 12
		y--
	}
	m, y = calendarClamp(m, y)
	first := time.Date(2000+y, time.Month(m+1), 1, 0, 0, 0, 0, time.Local)
	days = first.AddDate(0, 1, -1).Day()
	return first.Month(), first.Year(), days, weekdayNumber(first.Weekday())
}

// calendarAbsMonth is a month of a year, held to the years the calendar
// covers.
func (rt *Runtime) calendarAbsMonth(month, year int) (time.Month, int, int, int) {
	now := rt.calendarNow().Year() - 2000
	switch {
	case year < now-5:
		year = now - 5
	case year > now+5:
		year = now + 5
	}
	if year > 30 {
		year = 30
	}
	if year < 4 {
		year = 4
	}
	month %= 12
	if month < 0 {
		month = -month
	}
	first := time.Date(2000+year, time.Month(month+1), 1, 0, 0, 0, 0, time.Local)
	return first.Month(), first.Year(), first.AddDate(0, 1, -1).Day(), weekdayNumber(first.Weekday())
}

// registerCalendarAPI defines the date calls of the calendar.
func registerCalendarAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	pushDate := func(L *lua.LState, weekday, month, day, year int) int {
		return pushAll(L, calNumbers(weekday, month, day, year)...)
	}
	reg("CalendarGetDate", func(L *lua.LState) int {
		now := rt.calendarNow()
		return pushDate(L, weekdayNumber(now.Weekday()), int(now.Month()), now.Day(), now.Year())
	})
	// The weekday of the first and last day is not set by the original; it
	// reports 0.
	reg("CalendarGetMinDate", func(L *lua.LState) int { return pushDate(L, 0, 11, 23, 2004) })
	reg("CalendarGetMaxDate", func(L *lua.LState) int { return pushDate(L, 0, 1, 1, 2031) })
	reg("CalendarGetMinHistoryDate", func(L *lua.LState) int {
		now := rt.calendarNow()
		return pushDate(L, weekdayNumber(now.Weekday()), int(now.Month()), now.Day(), now.Year())
	})
	reg("CalendarGetMaxCreateDate", func(L *lua.LState) int {
		last := rt.calendarMaxCreate()
		return pushDate(L, weekdayNumber(last.Weekday()), int(last.Month()), last.Day(), last.Year())
	})
	reg("CalendarGetMonth", func(L *lua.LState) int {
		month, year, days, weekday := rt.calendarMonth(L.OptInt(1, 0))
		return pushDate(L, int(month), year, days, weekday)
	})
	reg("CalendarGetAbsMonth", func(L *lua.LState) int {
		month, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarGetAbsMonth(month[, year])")
			return 0
		}
		_, viewedYear := rt.calendarViewed()
		year := viewedYear - 2000
		if given, ok := calNumber(L, 2); ok {
			year = given - 2000
		}
		m, y, days, weekday := rt.calendarAbsMonth(month-1, year)
		return pushDate(L, int(m), y, days, weekday)
	})
	reg("CalendarSetAbsMonth", func(L *lua.LState) int {
		month, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarSetAbsMonth(month[, year])")
			return 0
		}
		_, viewedYear := rt.calendarViewed()
		year := viewedYear - 2000
		if given, ok := calNumber(L, 2); ok {
			year = given - 2000
		}
		m, y, _, _ := rt.calendarAbsMonth(month-1, year)
		rt.calendar.month, rt.calendar.year, rt.calendar.set = m, y, true
		rt.calendarRelayOut(true)
		return 0
	})
	reg("CalendarSetMonth", func(L *lua.LState) int {
		offset, ok := calNumber(L, 1)
		if !ok {
			L.RaiseError("Usage: CalendarSetMonth(offsetMonths)")
			return 0
		}
		month, year, _, _ := rt.calendarMonth(offset)
		rt.calendar.month, rt.calendar.year, rt.calendar.set = month, year, true
		rt.calendarRelayOut(true)
		return 0
	})
	reg("CalendarGetWeekdayNames", func(L *lua.LState) int {
		for _, name := range []string{"WEEKDAY_SUNDAY", "WEEKDAY_MONDAY", "WEEKDAY_TUESDAY", "WEEKDAY_WEDNESDAY", "WEEKDAY_THURSDAY", "WEEKDAY_FRIDAY", "WEEKDAY_SATURDAY"} {
			L.Push(lua.LString(rt.globalString(name, name)))
		}
		return 7
	})
	reg("CalendarGetMonthNames", func(L *lua.LState) int {
		for _, name := range []string{"MONTH_JANUARY", "MONTH_FEBRUARY", "MONTH_MARCH", "MONTH_APRIL", "MONTH_MAY", "MONTH_JUNE", "MONTH_JULY",
			"MONTH_AUGUST", "MONTH_SEPTEMBER", "MONTH_OCTOBER", "MONTH_NOVEMBER", "MONTH_DECEMBER"} {
			L.Push(lua.LString(rt.globalString(name, name)))
		}
		return 12
	})
	registerCalendarEventAPI(rt)
}

func registerOpenCalendarAPI(rt *Runtime) {
	rt.L.SetGlobal("OpenCalendar", rt.L.NewFunction(func(L *lua.LState) int {
		if !rt.cal.requested {
			rt.calendarEnsure()
		}
		return 0
	}))
}
