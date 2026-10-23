package ui

import (
	"fmt"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// luaStringFormat is the client's string.format: C conversions plus the
// positional form %n$s the localized strings use to reorder arguments.
func luaStringFormat(L *lua.LState) int {
	format := L.CheckString(1)
	var out strings.Builder
	next := 2
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			out.WriteByte(c)
			continue
		}
		i++
		if i >= len(format) {
			L.RaiseError("invalid format string to 'format'")
		}
		if format[i] == '%' {
			out.WriteByte('%')
			continue
		}
		// Optional position: digits followed by '$'.
		position := 0
		start := i
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			i++
		}
		if i < len(format) && format[i] == '$' && i > start {
			position, _ = strconv.Atoi(format[start:i])
			i++
		} else {
			i = start
		}
		spec := "%"
		for i < len(format) && strings.IndexByte("-+ #0", format[i]) >= 0 {
			spec += string(format[i])
			i++
		}
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			spec += string(format[i])
			i++
		}
		if i < len(format) && format[i] == '.' {
			spec += "."
			i++
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				spec += string(format[i])
				i++
			}
		}
		if i >= len(format) {
			L.RaiseError("invalid format string to 'format'")
		}
		verb := format[i]
		index := next
		if position > 0 {
			index = position + 1
		} else {
			next++
		}
		if index > L.GetTop() {
			L.ArgError(index, "no value")
		}
		arg := L.Get(index)
		switch verb {
		case 'd', 'i':
			out.WriteString(fmt.Sprintf(spec+"d", int64(L.CheckNumber(index))))
		case 'u':
			out.WriteString(fmt.Sprintf(spec+"d", uint64(L.CheckNumber(index))))
		case 'c':
			out.WriteString(string(rune(int(L.CheckNumber(index)))))
		case 'x', 'X', 'o':
			out.WriteString(fmt.Sprintf(spec+string(verb), int64(L.CheckNumber(index))))
		case 'e', 'E', 'f', 'g', 'G':
			out.WriteString(fmt.Sprintf(spec+string(verb), float64(L.CheckNumber(index))))
		case 's':
			out.WriteString(fmt.Sprintf(spec+"s", L.ToStringMeta(arg).String()))
		case 'q':
			out.WriteString(strconv.Quote(L.ToStringMeta(arg).String()))
		default:
			L.RaiseError("invalid option '%%%c' to 'format'", verb)
		}
	}
	L.Push(lua.LString(out.String()))
	return 1
}

// luaTruthy reads a boolean argument the way the game's Lua does: anything but
// nil and false counts as true, so "1" or 0 are accepted where a flag is wanted.
func luaTruthy(L *lua.LState, index int) bool {
	return lua.LVAsBool(L.Get(index))
}

// luaTruthyOpt is luaTruthy with a default for a missing argument.
func luaTruthyOpt(L *lua.LState, index int, fallback bool) bool {
	if L.Get(index) == lua.LNil {
		return fallback
	}
	return luaTruthy(L, index)
}
