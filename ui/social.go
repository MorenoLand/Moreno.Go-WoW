package ui

import (
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// ContactEntry is a friend or ignored player, with the name the server's name
// query gave.
type ContactEntry struct {
	GUID   uint64
	Name   string
	Ignore bool
	Note   string
	// Status is 0 offline, 1 online, 2 away, 3 busy.
	Status    uint8
	Area      string
	Level     int
	ClassFile string
}

// SocialHost is what the interface asks of the connection for friends and
// ignored players.
type SocialHost interface {
	RequestContacts() error
	AddFriend(name, note string) error
	AddIgnore(name string) error
	RemoveContact(guid uint64, ignore bool) error
	SetContactNote(guid uint64, note string) error
}

type socialState struct {
	friends []ContactEntry
	ignored []ContactEntry
}

// SetContacts replaces the social lists and tells the interface.
func (rt *Runtime) SetContacts(contacts []ContactEntry) {
	if rt == nil {
		return
	}
	rt.social.friends, rt.social.ignored = rt.social.friends[:0], rt.social.ignored[:0]
	for _, contact := range contacts {
		if contact.Name == "" {
			continue
		}
		if contact.Ignore {
			rt.social.ignored = append(rt.social.ignored, contact)
		} else {
			rt.social.friends = append(rt.social.friends, contact)
		}
	}
	sort.SliceStable(rt.social.friends, func(a, b int) bool {
		left, right := rt.social.friends[a], rt.social.friends[b]
		if (left.Status != 0) != (right.Status != 0) {
			return left.Status != 0
		}
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	})
	sort.SliceStable(rt.social.ignored, func(a, b int) bool {
		return strings.ToLower(rt.social.ignored[a].Name) < strings.ToLower(rt.social.ignored[b].Name)
	})
	rt.FireEvent("FRIENDLIST_UPDATE")
	rt.FireEvent("IGNORELIST_UPDATE")
}

func (rt *Runtime) socialHost() SocialHost {
	host, _ := rt.Host.(SocialHost)
	return host
}

// contactFor finds a contact by list position or by name.
func contactFor(list []ContactEntry, value lua.LValue) *ContactEntry {
	switch value := value.(type) {
	case lua.LNumber:
		if index := int(value); index >= 1 && index <= len(list) {
			return &list[index-1]
		}
	case lua.LString:
		for position := range list {
			if strings.EqualFold(list[position].Name, string(value)) {
				return &list[position]
			}
		}
	}
	return nil
}

// registerSocialAPI defines the friend and ignore calls.
func registerSocialAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetNumFriends", func(L *lua.LState) int {
		online := 0
		for _, friend := range rt.social.friends {
			if friend.Status != 0 {
				online++
			}
		}
		L.Push(lua.LNumber(len(rt.social.friends)))
		L.Push(lua.LNumber(online))
		return 2
	})
	reg("GetFriendInfo", func(L *lua.LState) int {
		friend := contactFor(rt.social.friends, L.Get(1))
		if friend == nil {
			L.Push(lua.LNil)
			return 1
		}
		class := friend.ClassFile
		if names, ok := L.GetGlobal("LOCALIZED_CLASS_NAMES_MALE").(*lua.LTable); ok {
			if name, ok := names.RawGetString(friend.ClassFile).(lua.LString); ok {
				class = string(name)
			}
		}
		status := ""
		switch friend.Status {
		case 2:
			status = rt.globalString("CHAT_FLAG_AFK", "<Away>")
		case 3:
			status = rt.globalString("CHAT_FLAG_DND", "<Busy>")
		}
		L.Push(lua.LString(friend.Name))
		L.Push(lua.LNumber(friend.Level))
		L.Push(lua.LString(class))
		L.Push(lua.LString(friend.Area))
		if friend.Status != 0 {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LString(status))
		L.Push(lua.LString(friend.Note))
		return 7
	})
	reg("GetNumIgnores", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.social.ignored)))
		return 1
	})
	reg("GetIgnoreName", func(L *lua.LState) int {
		if ignored := contactFor(rt.social.ignored, L.Get(1)); ignored != nil {
			L.Push(lua.LString(ignored.Name))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("ShowFriends", func(L *lua.LState) int {
		if host := rt.socialHost(); host != nil {
			_ = host.RequestContacts()
		}
		return 0
	})
	reg("AddFriend", func(L *lua.LState) int {
		if host := rt.socialHost(); host != nil {
			_ = host.AddFriend(L.CheckString(1), L.OptString(2, ""))
		}
		return 0
	})
	reg("AddIgnore", func(L *lua.LState) int {
		if host := rt.socialHost(); host != nil {
			_ = host.AddIgnore(L.CheckString(1))
		}
		return 0
	})
	reg("RemoveFriend", func(L *lua.LState) int {
		if host, friend := rt.socialHost(), contactFor(rt.social.friends, L.Get(1)); host != nil && friend != nil {
			_ = host.RemoveContact(friend.GUID, false)
		}
		return 0
	})
	reg("DelIgnore", func(L *lua.LState) int {
		if host, ignored := rt.socialHost(), contactFor(rt.social.ignored, L.Get(1)); host != nil && ignored != nil {
			_ = host.RemoveContact(ignored.GUID, true)
		}
		return 0
	})
	reg("SetFriendNotes", func(L *lua.LState) int {
		if host, friend := rt.socialHost(), contactFor(rt.social.friends, L.Get(1)); host != nil && friend != nil {
			friend.Note = L.CheckString(2)
			_ = host.SetContactNote(friend.GUID, friend.Note)
			rt.FireEvent("FRIENDLIST_UPDATE")
		}
		return 0
	})
}
