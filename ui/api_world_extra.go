package ui

import (
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// registerWorldExtraAPI defines the client functions the world UI calls when
// the player enters the world and that have a definite answer.
func registerWorldExtraAPI(rt *Runtime) {
	L := rt.L
	if table, ok := L.GetGlobal("string").(*lua.LTable); ok {
		table.RawSetString("format", L.NewFunction(luaStringFormat))
	}
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("date", func(L *lua.LState) int {
		format := L.OptString(1, "%c")
		moment := time.Now()
		if L.GetTop() >= 2 && L.Get(2) != lua.LNil {
			moment = time.Unix(int64(L.CheckNumber(2)), 0)
		}
		if strings.HasPrefix(format, "!") {
			moment, format = moment.UTC(), format[1:]
		}
		if strings.HasPrefix(format, "*t") {
			t := L.NewTable()
			t.RawSetString("year", lua.LNumber(moment.Year()))
			t.RawSetString("month", lua.LNumber(int(moment.Month())))
			t.RawSetString("day", lua.LNumber(moment.Day()))
			t.RawSetString("hour", lua.LNumber(moment.Hour()))
			t.RawSetString("min", lua.LNumber(moment.Minute()))
			t.RawSetString("sec", lua.LNumber(moment.Second()))
			t.RawSetString("wday", lua.LNumber(int(moment.Weekday())+1))
			t.RawSetString("yday", lua.LNumber(moment.YearDay()))
			t.RawSetString("isdst", lua.LFalse)
			L.Push(t)
			return 1
		}
		L.Push(lua.LString(strftime(format, moment)))
		return 1
	})
	reg("CombatLog_Object_IsA", func(L *lua.LState) int {
		L.Push(lua.LBool(uint32(L.OptNumber(1, 0))&uint32(L.OptNumber(2, 0)) != 0))
		return 1
	})
	reg("GetPVPRankProgress", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetQuestTimers", func(L *lua.LState) int { return 0 })
	reg("CalendarGetNumPendingInvites", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetMirrorTimerInfo", func(L *lua.LState) int {
		L.Push(lua.LString("UNKNOWN"))
		for i := 0; i < 4; i++ {
			L.Push(lua.LNumber(0))
		}
		L.Push(lua.LString(""))
		return 6
	})
	reg("UnitPVPRank", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetPVPRankInfo", func(L *lua.LState) int { L.Push(lua.LNil); L.Push(lua.LNumber(0)); return 2 })
	if _, ok := rt.GetCVar("chatStyle"); !ok {
		rt.SetCVar("chatStyle", "classic")
	}
	if _, ok := rt.GetCVar("lastTalkedToGM"); !ok {
		rt.SetCVar("lastTalkedToGM", "")
	}
}

// strftime formats a time with the C conversions the interface uses.
func strftime(format string, t time.Time) string {
	var out strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			out.WriteByte(format[i])
			continue
		}
		i++
		switch format[i] {
		case 'a':
			out.WriteString(t.Format("Mon"))
		case 'A':
			out.WriteString(t.Format("Monday"))
		case 'b':
			out.WriteString(t.Format("Jan"))
		case 'B':
			out.WriteString(t.Format("January"))
		case 'c':
			out.WriteString(t.Format("01/02/06 15:04:05"))
		case 'd':
			out.WriteString(t.Format("02"))
		case 'H':
			out.WriteString(t.Format("15"))
		case 'I':
			out.WriteString(t.Format("03"))
		case 'j':
			out.WriteString(t.Format("002"))
		case 'm':
			out.WriteString(t.Format("01"))
		case 'M':
			out.WriteString(t.Format("04"))
		case 'p':
			out.WriteString(t.Format("PM"))
		case 'S':
			out.WriteString(t.Format("05"))
		case 'x':
			out.WriteString(t.Format("01/02/06"))
		case 'X':
			out.WriteString(t.Format("15:04:05"))
		case 'y':
			out.WriteString(t.Format("06"))
		case 'Y':
			out.WriteString(t.Format("2006"))
		case 'Z':
			out.WriteString(t.Format("MST"))
		case '%':
			out.WriteByte('%')
		default:
			out.WriteByte('%')
			out.WriteByte(format[i])
		}
	}
	return out.String()
}

// registerUnitReactionAPI defines the functions that answer from a unit's
// reaction towards the player and its combat state.
func registerUnitReactionAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	known := func(unit string) *UnitInfo {
		info := rt.unitInfo(unit)
		if info == nil || !info.Exists {
			return nil
		}
		return info
	}
	reg("UnitXP", func(L *lua.LState) int {
		if info := known(L.OptString(1, "player")); info != nil {
			L.Push(lua.LNumber(info.XP))
			return 1
		}
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("UnitXPMax", func(L *lua.LState) int {
		if info := known(L.OptString(1, "player")); info != nil && info.XPMax > 0 {
			L.Push(lua.LNumber(info.XPMax))
			return 1
		}
		L.Push(lua.LNumber(1))
		return 1
	})
	// relation answers how b stands to a: only reactions towards the player are
	// known, so other pairs are friendly if they are the same unit.
	reaction := func(aToken, bToken string) int {
		a, b := known(aToken), known(bToken)
		if a == nil || b == nil {
			return 0
		}
		if strings.EqualFold(aToken, bToken) {
			return 5
		}
		switch {
		case strings.EqualFold(aToken, "player"):
			if b.Hostile {
				return 2
			}
			if b.Friendly {
				return 5
			}
			return 4
		case strings.EqualFold(bToken, "player"):
			if a.Hostile {
				return 2
			}
			if a.Friendly {
				return 5
			}
			return 4
		}
		return 4
	}
	reg("UnitReaction", func(L *lua.LState) int {
		if r := reaction(L.OptString(1, ""), L.OptString(2, "")); r != 0 {
			L.Push(lua.LNumber(r))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	})
	// UnitSelectionColor is the colour a unit's name and target frame take from how
	// it stands to the player: hostile red, neutral yellow, friendly green (blue
	// for a friendly player).
	reg("UnitSelectionColor", func(L *lua.LState) int {
		unit := L.OptString(1, "")
		info := known(unit)
		red, green, blue := float64(1), float64(1), float64(0)
		switch r := reaction(unit, "player"); {
		case info == nil:
		case r != 0 && r <= 2:
			red, green, blue = 1, 0, 0
		case r == 3:
			red, green, blue = 1, 0.5, 0
		case r >= 5 && info.Player:
			red, green, blue = 0.4, 0.4, 1
		case r >= 5:
			red, green, blue = 0, 1, 0
		}
		L.Push(lua.LNumber(red))
		L.Push(lua.LNumber(green))
		L.Push(lua.LNumber(blue))
		L.Push(lua.LNumber(1))
		return 4
	})
	reg("UnitCanAttack", func(L *lua.LState) int {
		a, b := L.OptString(1, ""), L.OptString(2, "")
		r := reaction(a, b)
		target := known(b)
		L.Push(lua.LBool(r != 0 && r < 5 && target != nil && !target.Dead && !strings.EqualFold(a, b)))
		return 1
	})
	reg("UnitCanAssist", func(L *lua.LState) int {
		a, b := L.OptString(1, ""), L.OptString(2, "")
		r := reaction(a, b)
		target := known(b)
		L.Push(lua.LBool(r >= 5 && target != nil && !target.Dead))
		return 1
	})
	reg("UnitIsEnemy", func(L *lua.LState) int {
		L.Push(lua.LBool(reaction(L.OptString(1, ""), L.OptString(2, "")) == 2))
		return 1
	})
	reg("UnitIsFriend", func(L *lua.LState) int {
		L.Push(lua.LBool(reaction(L.OptString(1, ""), L.OptString(2, "")) >= 5))
		return 1
	})
	reg("UnitAffectingCombat", func(L *lua.LState) int {
		info := known(L.OptString(1, ""))
		L.Push(lua.LBool(info != nil && info.InCombat))
		return 1
	})
	reg("UnitClassification", func(L *lua.LState) int {
		if info := known(L.OptString(1, "")); info != nil && info.Classification != "" {
			L.Push(lua.LString(info.Classification))
			return 1
		}
		L.Push(lua.LString("normal"))
		return 1
	})
	reg("UnitCreatureType", func(L *lua.LState) int {
		if info := known(L.OptString(1, "")); info != nil && info.CreatureType != "" {
			L.Push(lua.LString(info.CreatureType))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	})
}
