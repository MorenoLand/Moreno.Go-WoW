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
	{"ChannelVoiceOn", 0x03D6},
	{"ChannelVoiceOff", 0x03D7},
	{"SetChannelWatch", 0x03EF},
	{"DeclineInvite", 0x0410},
}

// What a line of the channel pane stands for.
const (
	channelLineChannel = iota
	channelLineParty
	channelLineRaid
	channelLineBattleground
)

// The three groups of channels the pane lists.
const (
	channelCategoryGroup = iota
	channelCategoryWorld
	channelCategoryCustom
)

var channelCategoryNames = [...]string{"CHANNEL_CATEGORY_GROUP", "CHANNEL_CATEGORY_WORLD", "CHANNEL_CATEGORY_CUSTOM"}

// channelDisplay is one line of the list the channel pane shows: a header, a
// line of the group chats, or a channel of the character's.
type channelDisplay struct {
	header   bool
	category int
	kind     int
	channel  *chatChannel
	// count is how many lines a header holds, or members a group chat has.
	count int
	world bool
}

// displayChannels builds the lines of the channel pane: the group chats, the
// channels of the world, then the ones the character made.
func (rt *Runtime) displayChannels() []channelDisplay {
	collapsed := &rt.channels.collapsed
	party := rt.group.in && len(rt.group.members) > 0 && !rt.isRaid()
	raid := rt.isRaid()
	battleground := raid && rt.inPVPInstance()
	var lines []channelDisplay
	group := channelDisplay{header: true, category: channelCategoryGroup}
	var groupLines []channelDisplay
	switch {
	case raid:
		groupLines = append(groupLines, channelDisplay{kind: channelLineRaid, category: channelCategoryGroup, count: len(rt.group.members) + 1})
	case party:
		groupLines = append(groupLines, channelDisplay{kind: channelLineParty, category: channelCategoryGroup, count: len(rt.group.members) + 1})
	}
	if battleground {
		groupLines = append(groupLines, channelDisplay{kind: channelLineBattleground, category: channelCategoryGroup, count: len(rt.group.members) + 1})
	}
	group.count = len(groupLines)
	lines = append(lines, group)
	if !collapsed[channelCategoryGroup] {
		lines = append(lines, groupLines...)
	}
	for category := channelCategoryWorld; category <= channelCategoryCustom; category++ {
		header := channelDisplay{header: true, category: category}
		var children []channelDisplay
		for _, channel := range rt.channels.slots {
			if channel == nil || !channel.joined || channel.left {
				continue
			}
			isWorld := channel.id != 0
			if isWorld != (category == channelCategoryWorld) {
				continue
			}
			children = append(children, channelDisplay{channel: channel, category: category, world: isWorld, count: len(channel.members)})
		}
		header.count = len(children)
		lines = append(lines, header)
		if !collapsed[category] {
			lines = append(lines, children...)
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
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetChannelDisplayInfo(index)")
			return 0
		}
		lines := rt.displayChannels()
		index := L.CheckInt(1)
		if index < 1 || index > len(lines) {
			return 0
		}
		line := lines[index-1]
		counted := func() lua.LValue {
			if line.count == 0 {
				return lua.LNil
			}
			return lua.LNumber(line.count)
		}
		yes := func(held bool) lua.LValue {
			if held {
				return lua.LNumber(1)
			}
			return lua.LNil
		}
		if line.header {
			L.Push(lua.LString(rt.globalString(channelCategoryNames[line.category], channelCategoryNames[line.category])))
			L.Push(lua.LNumber(1))
			L.Push(yes(rt.channels.collapsed[line.category]))
			L.Push(lua.LNil)
			L.Push(counted())
			for range 4 {
				L.Push(lua.LNil)
			}
			return 9
		}
		selected := index == rt.channels.selected
		if line.kind != channelLineChannel {
			name := map[int]string{channelLineParty: "CHAT_MSG_PARTY", channelLineRaid: "CHAT_MSG_RAID", channelLineBattleground: "CHAT_MSG_BATTLEGROUND"}[line.kind]
			L.Push(lua.LString(rt.globalString(name, name)))
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(counted())
			L.Push(yes(true))
			L.Push(lua.LString(rt.globalString(channelCategoryNames[channelCategoryGroup], channelCategoryNames[channelCategoryGroup])))
			L.Push(yes(false))
			L.Push(yes(selected))
			return 9
		}
		channel := line.channel
		L.Push(lua.LString(channel.name))
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNumber(channel.index))
		L.Push(counted())
		L.Push(yes(true))
		L.Push(lua.LString(rt.globalString(channelCategoryNames[line.category], channelCategoryNames[line.category])))
		L.Push(yes(channel.voice))
		L.Push(yes(selected))
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
	// Opening or closing the header of a group of channels.
	fold := func(name string, collapse bool) {
		reg(name, func(L *lua.LState) int {
			if L.Get(1).Type() != lua.LTNumber {
				return 0
			}
			lines := rt.displayChannels()
			if index := L.CheckInt(1); index >= 1 && index <= len(lines) && lines[index-1].header {
				rt.channels.collapsed[lines[index-1].category] = collapse
				rt.FireEvent("CHANNEL_UI_UPDATE")
			}
			return 0
		})
	}
	fold("ExpandChannelHeader", false)
	fold("CollapseChannelHeader", true)
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
