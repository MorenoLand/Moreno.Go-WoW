package ui

import (
	"sort"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The sources a name can come from, as the chat edit box asks for them.
const (
	autoCompleteInGroup = 0x01
	autoCompleteInGuild = 0x02
	autoCompleteFriend  = 0x04
	autoCompleteBNet    = 0x08
	autoCompleteTalked  = 0x10
	autoCompleteOnline  = 0x20
)

// autoCompleteNames lists every known name with the flags that describe it.
func (rt *Runtime) autoCompleteNames() map[string]uint32 {
	names := map[string]uint32{}
	add := func(name string, flags uint32) {
		if name != "" && !strings.EqualFold(name, rt.playerName()) {
			names[name] |= flags
		}
	}
	for _, friend := range rt.social.friends {
		flags := uint32(autoCompleteFriend)
		if friend.Status != 0 {
			flags |= autoCompleteOnline
		}
		add(friend.Name, flags)
	}
	for _, member := range rt.guild.members {
		flags := uint32(autoCompleteInGuild)
		if member.Status&world.GuildMemberOnline != 0 {
			flags |= autoCompleteOnline
		}
		add(member.Name, flags)
	}
	for _, member := range rt.group.members {
		add(member.Name, autoCompleteInGroup|autoCompleteOnline)
	}
	for _, name := range rt.recentTalk {
		add(name, autoCompleteTalked)
	}
	return names
}

// rememberTalk keeps the names the character has whispered or been whispered
// by, newest first.
func (rt *Runtime) rememberTalk(name string) {
	if name == "" {
		return
	}
	for index, old := range rt.recentTalk {
		if strings.EqualFold(old, name) {
			rt.recentTalk = append(rt.recentTalk[:index], rt.recentTalk[index+1:]...)
			break
		}
	}
	rt.recentTalk = append([]string{name}, rt.recentTalk...)
	if len(rt.recentTalk) > 50 {
		rt.recentTalk = rt.recentTalk[:50]
	}
}

func registerAutoCompleteAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetAutoCompleteResults", func(L *lua.LState) int {
		if L.GetTop() < 4 || L.Get(1).Type() != lua.LTString {
			L.RaiseError("Usage: GetAutoCompleteResults(\"text\", include, exclude, numResults [, cursorPosition])")
			return 0
		}
		text := L.CheckString(1)
		include, exclude, limit := uint32(L.CheckInt64(2)), uint32(L.CheckInt64(3)), L.CheckInt(4)
		if cursor := L.OptInt(5, len(text)); cursor >= 0 && cursor < len(text) {
			text = text[:cursor]
		}
		if text == "" {
			return 0
		}
		var found []string
		for name, flags := range rt.autoCompleteNames() {
			if flags&include != 0 && flags&exclude == 0 && len(name) >= len(text) && strings.EqualFold(name[:len(text)], text) {
				found = append(found, name)
			}
		}
		sort.Strings(found)
		if limit > 0 && len(found) > limit {
			found = found[:limit]
		}
		for _, name := range found {
			L.Push(lua.LString(name))
		}
		return len(found)
	})
	reg("GetAutoCompletePresenceID", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError("Usage: GetAutoCompletePresenceID(\"name\")")
			return 0
		}
		return 0
	})
}
