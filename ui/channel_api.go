package ui

import (
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// channelCommands are the calls that name a channel and a player.
var channelPlayerCommands = []struct {
	name   string
	opcode world.ClientOpcode
}{
	{"ChannelBan", world.ClientChannelBan},
	{"ChannelInvite", world.ClientChannelInvite},
	{"ChannelKick", world.ClientChannelKick},
	{"ChannelModerator", world.ClientChannelModerate},
	{"ChannelMute", world.ClientChannelMute},
	{"ChannelUnban", world.ClientChannelUnban},
	{"ChannelUnmoderator", world.ClientChannelUnmod},
	{"ChannelUnmute", world.ClientChannelUnmute},
	{"SetChannelOwner", world.ClientChannelSetOwner},
}

// channelOnlyCommands are the calls that name a channel alone.
var channelOnlyCommands = []struct {
	name   string
	opcode world.ClientOpcode
}{
	{"ChannelToggleAnnouncements", world.ClientChannelAnnounce},
	{"DisplayChannelOwner", world.ClientChannelOwner},
	{"ListChannelByName", world.ClientChannelList},
}

// channelDisplay is one line of the list the channel pane shows: a header, or
// a channel of the character's.
type channelDisplay struct {
	header   bool
	category string
	channel  *chatChannel
}

// displayChannels builds the lines of the channel pane: the channels of the
// world, then the ones the character made.
func (rt *Runtime) displayChannels() []channelDisplay {
	var world, custom []*chatChannel
	for _, channel := range rt.channels.slots {
		if channel == nil || !channel.joined || channel.left {
			continue
		}
		if channel.id != 0 {
			world = append(world, channel)
		} else {
			custom = append(custom, channel)
		}
	}
	var lines []channelDisplay
	if len(world) > 0 {
		lines = append(lines, channelDisplay{header: true, category: "CHANNEL_CATEGORY_WORLD"})
		for _, channel := range world {
			lines = append(lines, channelDisplay{channel: channel})
		}
	}
	if len(custom) > 0 {
		lines = append(lines, channelDisplay{header: true, category: "CHANNEL_CATEGORY_CUSTOM"})
		for _, channel := range custom {
			lines = append(lines, channelDisplay{channel: channel})
		}
	}
	return lines
}

func (rt *Runtime) selectedChannel() *chatChannel {
	if rt.channels.selected < 1 {
		return nil
	}
	lines := rt.displayChannels()
	if rt.channels.selected > len(lines) {
		return nil
	}
	return lines[rt.channels.selected-1].channel
}

// registerChannelAPI defines the calls that join, leave and list channels.
func registerChannelAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	join := func(L *lua.LState) int {
		name := L.CheckString(1)
		password := ""
		if L.GetTop() >= 2 && L.Get(2).Type() == lua.LTString {
			password = L.Get(2).String()
		}
		if name == "" || strings.ContainsRune(name, ' ') || len(name) >= 0x80 {
			return 0
		}
		window := 0
		if L.GetTop() >= 3 && L.Get(3).Type() == lua.LTNumber {
			window = int(L.Get(3).(lua.LNumber))
		}
		voice := L.GetTop() >= 4 && lua.LVAsBool(L.Get(4))
		id, shortcut, ok := rt.joinChannel(name, password, window, false, voice, false)
		if !ok {
			return 0
		}
		L.Push(lua.LNumber(id))
		if id != 0 {
			L.Push(lua.LString(shortcut))
		} else {
			L.Push(lua.LNil)
		}
		return 2
	}
	reg("JoinChannelByName", join)
	reg("JoinPermanentChannel", join)
	reg("LeaveChannelByName", func(L *lua.LState) int {
		name := rt.channelTargetName(L.CheckString(1))
		channel := rt.channelByName(name)
		if channel == nil {
			return 0
		}
		if channel.joined {
			rt.leaveChannel(channel)
		} else {
			rt.dropChannel(channel)
		}
		return 0
	})
	reg("ListChannels", func(L *lua.LState) int {
		var text strings.Builder
		for _, channel := range rt.channels.slots {
			if channel != nil && channel.joined && !channel.left {
				text.WriteString("[" + channelLabel(channel) + "] ")
			}
		}
		rt.fireChannelEvent("CHAT_MSG_CHANNEL_LIST", text.String(), "", "", nil, "", 0)
		return 0
	})
	reg("GetChannelList", func(L *lua.LState) int {
		count := 0
		for _, channel := range rt.channels.slots {
			if channel == nil || !channel.joined || channel.left {
				continue
			}
			name := channel.name
			if row := rt.channelRowByID(channel.id); row != nil {
				name = row.Shortcut
			}
			L.Push(lua.LNumber(channel.index))
			L.Push(lua.LString(name))
			count += 2
		}
		return count
	})
	reg("GetChannelName", func(L *lua.LState) int {
		var channel *chatChannel
		switch L.Get(1).Type() {
		case lua.LTNumber:
			if found := rt.channelByIndex(int(L.Get(1).(lua.LNumber))); found != nil && found.joined && !found.left {
				channel = found
			}
		case lua.LTString:
			channel = rt.channelByName(L.Get(1).String())
		default:
			L.ArgError(1, "Usage: GetChannelName([channelIndex] or [channelName])")
		}
		if channel == nil {
			L.Push(lua.LNumber(0))
			L.Push(lua.LNil)
			L.Push(lua.LNumber(0))
			return 3
		}
		L.Push(lua.LNumber(channel.index))
		L.Push(lua.LString(channel.name))
		L.Push(lua.LNumber(channel.instance))
		return 3
	})
	for _, command := range channelPlayerCommands {
		command := command
		reg(command.name, func(L *lua.LState) int {
			channel, player := L.CheckString(1), L.CheckString(2)
			if name := rt.channelTargetName(channel); name != "" {
				rt.send(command.opcode, world.BuildChannelCommand(name, player))
			}
			return 0
		})
	}
	for _, command := range channelOnlyCommands {
		command := command
		reg(command.name, func(L *lua.LState) int {
			if name := rt.channelTargetName(L.CheckString(1)); name != "" {
				rt.send(command.opcode, world.BuildChannelCommand(name))
			}
			return 0
		})
	}
	reg("SetChannelPassword", func(L *lua.LState) int {
		channel, password := L.CheckString(1), L.CheckString(2)
		if name := rt.channelTargetName(channel); name != "" {
			rt.send(world.ClientChannelPassword, world.BuildChannelCommand(name, password))
		}
		return 0
	})
	reg("GetNumDisplayChannels", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.displayChannels())))
		return 1
	})
	reg("GetChannelDisplayInfo", func(L *lua.LState) int {
		lines := rt.displayChannels()
		index := L.CheckInt(1)
		if index < 1 || index > len(lines) {
			return 0
		}
		line := lines[index-1]
		if line.header {
			L.Push(lua.LString(rt.globalString(line.category, line.category)))
			L.Push(lua.LNumber(1))
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			for range 5 {
				L.Push(lua.LNil)
			}
			return 9
		}
		channel := line.channel
		L.Push(lua.LString(channel.name))
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNumber(channel.index))
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 9
	})
	reg("SetSelectedDisplayChannel", func(L *lua.LState) int {
		rt.channels.selected = L.CheckInt(1)
		if channel := rt.selectedChannel(); channel != nil {
			rt.send(world.ClientChannelDisplay, world.BuildChannelCommand(channel.name))
		}
		return 0
	})
	reg("GetSelectedDisplayChannel", func(L *lua.LState) int {
		if rt.selectedChannel() == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LNumber(rt.channels.selected))
		return 1
	})
	reg("GetNumChannelMembers", func(L *lua.LState) int {
		lines := rt.displayChannels()
		index := L.CheckInt(1)
		if index < 1 || index > len(lines) || lines[index-1].channel == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LNumber(len(lines[index-1].channel.members)))
		return 1
	})
	reg("GetChannelRosterInfo", func(L *lua.LState) int {
		lines := rt.displayChannels()
		index, member := L.CheckInt(1), L.CheckInt(2)
		if index < 1 || index > len(lines) || lines[index-1].channel == nil {
			return 0
		}
		channel := lines[index-1].channel
		if member < 1 || member > len(channel.members) {
			return 0
		}
		entry := channel.members[member-1]
		owner, moderator := channelMemberFlags(entry.Flags)
		L.Push(lua.LString(rt.nameOf(entry.GUID)))
		L.Push(optionalFlag(owner))
		L.Push(optionalFlag(moderator))
		L.Push(optionalFlag(entry.Flags&0x08 != 0))
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		return 6
	})
	reg("IsDisplayChannelOwner", func(L *lua.LState) int {
		channel := rt.selectedChannel()
		if channel == nil {
			L.Push(lua.LNil)
			return 1
		}
		for _, member := range channel.members {
			if rt.nameOf(member.GUID) == rt.playerName() {
				owner, _ := channelMemberFlags(member.Flags)
				L.Push(optionalFlag(owner))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("IsDisplayChannelModerator", func(L *lua.LState) int {
		channel := rt.selectedChannel()
		if channel == nil {
			L.Push(lua.LNil)
			return 1
		}
		for _, member := range channel.members {
			if rt.nameOf(member.GUID) == rt.playerName() {
				owner, moderator := channelMemberFlags(member.Flags)
				L.Push(optionalFlag(owner || moderator))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("ExpandChannelHeader", func(L *lua.LState) int { return 0 })
	reg("CollapseChannelHeader", func(L *lua.LState) int { return 0 })
}

// playerName is the character's name.
func (rt *Runtime) playerName() string {
	if player := rt.unitInfo("player"); player != nil {
		return player.Name
	}
	return ""
}

// channelRowByID finds a row of ChatChannels.dbc by its id.
func (rt *Runtime) channelRowByID(id uint32) *ChatChannelRow {
	if id == 0 {
		return nil
	}
	for index := range rt.channels.rows {
		if rt.channels.rows[index].ID == id {
			return &rt.channels.rows[index]
		}
	}
	return nil
}
