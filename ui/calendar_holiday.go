package ui

import (
	"sort"
	"time"
)

// CalendarHoliday is a row of Holidays.dbc with its names. The server may
// send its own dates for a holiday; they replace the ones of the table.
type CalendarHoliday struct {
	ID          uint32
	Durations   [10]uint32
	Dates       [26]uint32
	Region      uint32
	Looping     uint32
	CalFlags    [10]uint32
	NameID      uint32
	DescID      uint32
	Texture     string
	Priority    uint32
	FilterType  uint32
	Flags       uint32
	Name        string
	Description string
	// FromServer is set once the server's dates replaced the table's.
	FromServer bool
}

// SetCalendarHolidays installs the holidays of the client's tables.
func (rt *Runtime) SetCalendarHolidays(holidays []CalendarHoliday) {
	rt.cal.holidays = map[uint32]*CalendarHoliday{}
	for i := range holidays {
		h := holidays[i]
		rt.cal.holidays[h.ID] = &h
	}
	rt.cal.laidOut = false
}

// calHolidayTime is a date of a holiday: a field of -1 is left open and taken
// from the day the calendar is at.
type calHolidayTime struct {
	minute, hour, weekday, day, month, year int
}

func unpackHolidayTime(packed uint32) calHolidayTime {
	field := func(value, mask uint32) int {
		if value == mask {
			return -1
		}
		return int(value)
	}
	return calHolidayTime{
		minute: field(packed&0x3f, 0x3f), hour: field(packed>>6&0x1f, 0x1f), weekday: field(packed>>11&7, 7),
		day: field(packed>>14&0x3f, 0x3f), month: field(packed>>20&0xf, 0xf), year: field(packed>>24&0x1f, 0x1f),
	}
}

func holidayHours(hours uint32) time.Duration {
	if hours == 0 {
		hours = 24
	}
	return time.Duration(hours) * time.Hour
}

// calendarPlaceHolidays puts the holidays that fall in the six weeks the
// calendar shows on their days.
func (rt *Runtime) calendarPlaceHolidays() {
	month, year := rt.calendarBase()
	first := time.Date(2000+year, time.Month(month+1), 1, 0, 0, 0, 0, time.Local)
	windowStart, windowEnd := first.AddDate(0, 0, -6), first.AddDate(0, 0, 42)
	now := rt.calendarNow()
	viewedYear := year
	if month == 0 {
		// A holiday of December still shown in January belongs to the year
		// before.
		viewedYear = year - 1
	}
	ids := make([]uint32, 0, len(rt.cal.holidays))
	for id := range rt.cal.holidays {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	for _, id := range ids {
		h := rt.cal.holidays[id]
		for slot := 0; slot < len(h.CalFlags); slot++ {
			// Both kinds of realm show the slots that name either.
			if h.CalFlags[slot]&3 == 0 || (slot > 0 && h.Durations[slot] == 0) {
				continue
			}
			var before time.Duration
			for j := 0; j < slot; j++ {
				before += holidayHours(h.Durations[j])
			}
			length := holidayHours(h.Durations[slot])
			for _, packed := range h.Dates {
				if packed == 0 {
					continue
				}
				t := unpackHolidayTime(packed)
				weekly := t.weekday >= 0 && t.day < 0 && t.month < 0 && t.year < 0
				reference := calTimeOf(now)
				if weekly {
					reference = calTimeOf(windowStart)
				}
				if t.minute < 0 {
					t.minute = reference.minute
				}
				if t.hour < 0 {
					t.hour = reference.hour
				}
				if t.day < 0 {
					t.day = reference.day
				}
				if t.month < 0 {
					t.month = reference.month
				}
				if t.year < 0 {
					t.year = viewedYear
				}
				start := time.Date(2000+t.year, time.Month(t.month+1), t.day+1, t.hour, t.minute, 0, 0, time.Local)
				if weekly {
					delta := (t.weekday - int(start.Weekday()) + 7) % 7
					start = start.AddDate(0, 0, delta)
				}
				start = start.Add(before)
				for {
					end := start.Add(length)
					if !start.After(windowEnd) && !end.Before(windowStart) {
						s, e := start, end
						if s.Hour() == 0 && s.Minute() == 0 {
							s = s.Add(time.Minute)
						}
						if e.Hour() == 0 && e.Minute() == 0 {
							e = e.Add(-time.Minute)
						}
						rt.calendarPlace(&calEntry{flags: calFlagHoliday, holiday: id, eventType: -1, dungeon: -1,
							start: calTimeOf(s), end: calTimeOf(e), hasEnd: true})
					}
					if !weekly {
						break
					}
					start = start.AddDate(0, 0, 7)
					if start.After(windowEnd) {
						break
					}
				}
			}
		}
	}
}
