package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// maxChatChannels is how many channels the character can be in at once.
const maxChatChannels = 10

// Flags of the rows of ChatChannels.dbc.
const (
	channelRowInitial = 0x00001
	channelRowCity2   = 0x00020
	channelRowGuild   = 0x20000
)

// ChatChannelRow is a row of ChatChannels.dbc.
type ChatChannelRow struct {
	ID, Flags uint32
	// Pattern is the name with its "%s" for the zone, Shortcut the name the
	// channel goes by in commands.
	Pattern, Shortcut string
}

// chatChannel is a place of the channel list.
type chatChannel struct {
	index    int
	name     string
	id       uint32 // the row of ChatChannels.dbc, 0 for a channel of the player's own
	flags    uint8
	instance uint32
	joined   bool // the server has taken the character in
	left     bool
	owner    string
	// members is the roster the server last listed.
	members []world.ChannelMember
	voice   bool
}

type channelState struct {
	rows     []ChatChannelRow
	slots    [maxChatChannels]*chatChannel
	started  bool
	selected int
	lineID   int
	// defaults is the bit mask of the rows joined on entering the world.
	defaults uint32
}

// SetChatChannelRows gives the interface the rows of ChatChannels.dbc.
func (rt *Runtime) SetChatChannelRows(rows []ChatChannelRow) {
	rt.channels.rows = rows
	rt.channels.defaults = 0
	for _, row := range rows {
		if row.Flags&channelRowInitial != 0 && row.ID >= 1 && row.ID <= 32 {
			rt.channels.defaults |= 1 << (row.ID - 1)
		}
	}
	// A character's first window shows the channels it joins by default.
	if first := &rt.chatCfg.windows[0]; len(first.channels) == 0 {
		for _, row := range rows {
			if row.Flags&channelRowInitial != 0 {
				first.channels = append(first.channels, chatWindowChannel{name: row.Shortcut, zone: row.ID})
			}
		}
	}
}

func (rt *Runtime) channelByName(name string) *chatChannel {
	for _, channel := range rt.channels.slots {
		if channel != nil && strings.EqualFold(channel.name, name) {
			return channel
		}
	}
	return nil
}

func (rt *Runtime) channelByID(id uint32) *chatChannel {
	if id == 0 {
		return nil
	}
	for _, channel := range rt.channels.slots {
		if channel != nil && channel.id == id {
			return channel
		}
	}
	return nil
}

func (rt *Runtime) channelByIndex(index int) *chatChannel {
	if index < 1 || index > maxChatChannels {
		return nil
	}
	return rt.channels.slots[index-1]
}

// findChannel finds a channel the way a server notice names it: by its row
// when there is one, otherwise by its name.
func (rt *Runtime) findChannel(id uint32, name string) *chatChannel {
	if id != 0 {
		return rt.channelByID(id)
	}
	return rt.channelByName(name)
}

// channelLabel is how a channel is named in a line: its place and its name.
func channelLabel(channel *chatChannel) string {
	return fmt.Sprintf("%d. %s", channel.index, channel.name)
}

// channelRowFor finds the row of ChatChannels.dbc a name answers to.
func (rt *Runtime) channelRowFor(name string) *ChatChannelRow {
	for index := range rt.channels.rows {
		if rt.channels.rows[index].Shortcut == name {
			return &rt.channels.rows[index]
		}
	}
	return nil
}

// joinChannel puts a channel in the list and asks the server for it, the way
// the interface's join does: a name of ChatChannels.dbc is expanded with the
// zone. It reports the row id and shortcut of such a channel.
func (rt *Runtime) joinChannel(name, password string, window int, first, voice, quiet bool) (uint32, string, bool) {
	var id uint32
	shortcut := ""
	full := name
	if row := rt.channelRowFor(name); row != nil {
		zone := rt.area.zone
		if zone == "" {
			return 0, "", false
		}
		id, shortcut = row.ID, row.Shortcut
		full = strings.Replace(row.Pattern, "%s", zone, 1)
	}
	channel := rt.findChannel(id, full)
	if channel == nil {
		slot := -1
		for index := 0; slot < 0 && index < maxChatChannels; index++ {
			if rt.channels.slots[index] == nil {
				slot = index
			}
		}
		if slot < 0 {
			if !quiet {
				rt.ShowGlobalError("ERR_TOO_MANY_CHAT_CHANNELS")
			}
			return 0, "", false
		}
		channel = &chatChannel{index: slot + 1, name: full, id: id, left: false}
		rt.channels.slots[slot] = channel
		rt.send(world.ClientJoinChannel, world.BuildJoinChannel(id, first, voice, full, password))
	}
	if window >= 1 && window <= chatWindowCount {
		label := full
		if shortcut != "" {
			label = shortcut
		}
		rt.addWindowChannel(window-1, label, id)
	}
	rt.FireEvent("CHANNEL_UI_UPDATE")
	return id, shortcut, true
}

// StartChannels joins the channels of a new session once the zone is known.
func (rt *Runtime) StartChannels() {
	if rt.channels.started || rt.area.zone == "" || len(rt.channels.rows) == 0 {
		return
	}
	rt.channels.started = true
	for _, row := range rt.channels.rows {
		if row.ID >= 1 && row.ID <= 32 && rt.channels.defaults&(1<<(row.ID-1)) != 0 {
			rt.joinChannel(row.Shortcut, "", 0, true, false, true)
		}
	}
}

// leaveChannel tells the server the character leaves a channel.
func (rt *Runtime) leaveChannel(channel *chatChannel) {
	rt.send(world.ClientLeaveChannel, world.BuildLeaveChannel(channel.id, channel.name))
	channel.left = true
	rt.FireEvent("CHANNEL_UI_UPDATE")
}

// dropChannel forgets a channel.
func (rt *Runtime) dropChannel(channel *chatChannel) {
	rt.channels.slots[channel.index-1] = nil
	rt.FireEvent("CHANNEL_UI_UPDATE")
}

// chatFlagText is the flag word of a chat line's sender.
func chatFlagText(tag uint8) string {
	switch {
	case tag&0x10 != 0:
		return "DEV"
	case tag&4 != 0:
		return "GM"
	case tag&2 != 0:
		return "DND"
	case tag&1 != 0:
		return "AFK"
	}
	return ""
}

func (rt *Runtime) nextChatLine() int {
	rt.channels.lineID++
	return rt.channels.lineID
}

func guidText(guid uint64) lua.LValue {
	if guid == 0 {
		return lua.LString("")
	}
	return lua.LString(fmt.Sprintf("0x%016X", guid))
}

// fireChannelEvent raises a channel line: the event, its text, the player
// names it carries, the channel it belongs to and the player's guid.
func (rt *Runtime) fireChannelEvent(event, text, author, other string, channel *chatChannel, flags string, guid uint64) {
	label, index, id, base := "", 0, uint32(0), ""
	if channel != nil {
		label, index, id, base = channelLabel(channel), channel.index, channel.id, channel.name
	}
	rt.FireEvent(event,
		lua.LString(text), lua.LString(author), lua.LString(""), lua.LString(label), lua.LString(other),
		lua.LString(flags), lua.LNumber(id), lua.LNumber(index), lua.LString(base), lua.LNumber(0),
		lua.LNumber(rt.nextChatLine()), guidText(guid))
}

// channelNoticeWords are the words of the notices that come with a player.
var channelNoticeWords = map[uint8]string{
	world.ChannelPasswordChanged:     "PASSWORD_CHANGED",
	world.ChannelOwnerChanged:        "OWNER_CHANGED",
	world.ChannelAnnouncementsOn:     "ANNOUNCEMENTS_ON",
	world.ChannelAnnouncementsOff:    "ANNOUNCEMENTS_OFF",
	world.ChannelModerationOn:        "MODERATION_ON",
	world.ChannelModerationOff:       "MODERATION_OFF",
	world.ChannelPlayerKicked:        "PLAYER_KICKED",
	world.ChannelPlayerBanned:        "PLAYER_BANNED",
	world.ChannelPlayerUnbanned:      "PLAYER_UNBANNED",
	world.ChannelPlayerAlreadyMember: "PLAYER_ALREADY_MEMBER",
	world.ChannelInvite:              "INVITE",
	world.ChannelVoiceOn:             "VOICE_ON",
	world.ChannelVoiceOff:            "VOICE_OFF",
}

// channelNoticeSolo are the words of the notices that come without a player.
var channelNoticeSolo = map[uint8]string{
	world.ChannelWrongPassword:      "WRONG_PASSWORD",
	world.ChannelNotMember:          "NOT_MEMBER",
	world.ChannelNotModerator:       "NOT_MODERATOR",
	world.ChannelNotOwner:           "NOT_OWNER",
	world.ChannelMuted:              "MUTED",
	world.ChannelBanned:             "BANNED",
	world.ChannelInviteWrongFaction: "INVITE_WRONG_FACTION",
	world.ChannelWrongFaction:       "WRONG_FACTION",
	world.ChannelInvalidName:        "INVALID_NAME",
	world.ChannelNotModerated:       "NOT_MODERATED",
	world.ChannelThrottled:          "THROTTLED",
	world.ChannelNotInArea:          "NOT_IN_AREA",
	world.ChannelNotInLFG:           "NOT_IN_LFG",
}

// channelNoticeNamed are the notices that name a player by text.
var channelNoticeNamed = map[uint8]string{
	world.ChannelPlayerNotFound:     "PLAYER_NOT_FOUND",
	world.ChannelChannelOwner:       "CHANNEL_OWNER",
	world.ChannelPlayerNotBanned:    "PLAYER_NOT_BANNED",
	world.ChannelPlayerInvited:      "PLAYER_INVITED",
	world.ChannelPlayerInviteBanned: "PLAYER_INVITE_BANNED",
}

// applyChannelNotify handles SMSG_CHANNEL_NOTIFY.
func (rt *Runtime) applyChannelNotify(notify world.ChannelNotify) {
	switch notify.Kind {
	case world.ChannelJoinedNotice, world.ChannelLeftNotice:
		channel := rt.channelByName(notify.Channel)
		if channel == nil {
			return
		}
		event := "CHAT_MSG_CHANNEL_JOIN"
		if notify.Kind == world.ChannelLeftNotice {
			event = "CHAT_MSG_CHANNEL_LEAVE"
		}
		rt.fireChannelEvent(event, "", rt.nameOf(notify.Player), "", channel, "", notify.Player)
		return
	case world.ChannelYouJoined:
		channel := rt.findChannel(notify.ID, notify.Channel)
		word := "YOU_JOINED"
		if channel != nil && channel.joined && !channel.left {
			word = "YOU_CHANGED"
		}
		if channel != nil {
			channel.name, channel.flags, channel.instance = notify.Channel, notify.Flags, notify.Instance
			channel.joined, channel.left = true, false
			channel.members = nil
		}
		rt.fireChannelEvent("CHAT_MSG_CHANNEL_NOTICE", word, "", "", channel, "", 0)
		if notify.Text != "" {
			rt.fireChannelEvent("CHAT_MSG_CHANNEL_NOTICE", notify.Text, "", "", channel, "", 0)
		}
		rt.FireEvent("CHANNEL_UI_UPDATE")
		return
	case world.ChannelYouLeft:
		channel := rt.findChannel(notify.ID, notify.Channel)
		word := "YOU_LEFT"
		if channel != nil && !channel.left && notify.Suspended {
			word = "SUSPENDED"
		}
		rt.fireChannelEvent("CHAT_MSG_CHANNEL_NOTICE", word, "", "", channel, "", 0)
		if channel != nil && word == "YOU_LEFT" {
			rt.dropChannel(channel)
		}
		return
	case world.ChannelModeChange:
		channel := rt.channelByName(notify.Channel)
		if channel != nil {
			rt.FireEvent("CHANNEL_UI_UPDATE")
		}
		return
	}
	channel := rt.channelByName(notify.Channel)
	if word, ok := channelNoticeSolo[notify.Kind]; ok {
		rt.fireChannelEvent("CHAT_MSG_CHANNEL_NOTICE", word, "", "", channel, "", 0)
		if notify.Kind == world.ChannelBanned || notify.Kind == world.ChannelWrongFaction || notify.Kind == world.ChannelInvalidName || notify.Kind == world.ChannelNotInArea || notify.Kind == world.ChannelNotInLFG {
			if channel != nil {
				rt.dropChannel(channel)
			}
		}
		return
	}
	if word, ok := channelNoticeNamed[notify.Kind]; ok {
		rt.fireChannelEvent("CHAT_MSG_CHANNEL_NOTICE_USER", word, notify.Name, "", channel, "", 0)
		return
	}
	if word, ok := channelNoticeWords[notify.Kind]; ok {
		other := ""
		if notify.Other != 0 {
			other = rt.nameOf(notify.Other)
		}
		rt.fireChannelEvent("CHAT_MSG_CHANNEL_NOTICE_USER", word, rt.nameOf(notify.Player), other, channel, "", notify.Player)
	}
}

// applyChannelList handles SMSG_CHANNEL_LIST.
func (rt *Runtime) applyChannelList(roster world.ChannelRoster) {
	channel := rt.channelByName(roster.Channel)
	if channel == nil {
		return
	}
	channel.members = roster.Members
	rt.FireEvent("CHANNEL_ROSTER_UPDATE", lua.LNumber(channel.index))
	rt.FireEvent("CHANNEL_UI_UPDATE")
}

// channelTargetName turns the target of a channel chat line (its place in the
// list, or its name) into the name the server knows it by.
func (rt *Runtime) channelTargetName(target string) string {
	if index, err := strconv.Atoi(strings.TrimSpace(target)); err == nil {
		if channel := rt.channelByIndex(index); channel != nil {
			return channel.name
		}
	}
	if channel := rt.channelByName(target); channel != nil {
		return channel.name
	}
	return target
}

// channelMemberFlagText names the rights of a member of a channel.
func channelMemberFlags(flags uint8) (owner, moderator bool) {
	return flags&0x01 != 0, flags&0x02 != 0
}
