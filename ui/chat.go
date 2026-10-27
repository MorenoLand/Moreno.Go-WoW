package ui

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// pendingChat is a line waiting for the name of its sender.
type pendingChat struct {
	message world.ChatMessage
	since   time.Time
}

type chatState struct {
	pending []pendingChat
	// requestName asks the world for the name of a player.
	requestName func(guid uint64)
}

// SetNameRequester gives the interface a way to ask the world for the name of
// a player it only knows by guid.
func (rt *Runtime) SetNameRequester(request func(guid uint64)) { rt.chat.requestName = request }

// chatNeedsName reports whether a line names its sender by guid only.
func chatNeedsName(message world.ChatMessage) bool {
	if message.SenderName != "" || message.Sender == 0 {
		return false
	}
	switch message.Type {
	case world.ChatMonsterSay, world.ChatMonsterParty, world.ChatMonsterYell, world.ChatMonsterWhisper, world.ChatMonsterEmote,
		world.ChatRaidBossEmote, world.ChatRaidBossWhisper, world.ChatBattleNet, world.ChatWhisperForeign:
		return false
	}
	return message.Sender>>48 == 0
}

// senderName is the name a line shows for its sender.
func (rt *Runtime) senderName(message world.ChatMessage) string {
	if message.SenderName != "" {
		return message.SenderName
	}
	if message.Sender == 0 {
		return ""
	}
	if name := rt.nameOf(message.Sender); name != "" {
		return name
	}
	if player := rt.unitInfo("player"); player != nil && player.GUID == message.Sender {
		return player.Name
	}
	return ""
}

// deliverChat raises the event of a chat line with the arguments the chat
// frames read: text, author, language, channel, target, flags, the channel's
// row and place, its name, an unused number, the line number and the guid.
func (rt *Runtime) deliverChat(message world.ChatMessage) {
	event := message.Type.EventName()
	if event == "" {
		return
	}
	author := rt.senderName(message)
	channelText, channelIndex, channelRow, channelName := "", 0, uint32(0), ""
	if message.Type == world.ChatChannel {
		channel := rt.channelByName(message.Channel)
		if channel == nil {
			return
		}
		channelText, channelIndex, channelRow, channelName = channelLabel(channel), channel.index, channel.id, channel.name
	}
	target := ""
	switch message.Type {
	case world.ChatWhisperInform:
		author = rt.nameOf(message.Sender)
	case world.ChatMonsterSay, world.ChatMonsterParty, world.ChatMonsterYell, world.ChatMonsterWhisper, world.ChatMonsterEmote,
		world.ChatRaidBossEmote, world.ChatRaidBossWhisper, world.ChatBGSystemNeutral, world.ChatBGSystemAlliance, world.ChatBGSystemHorde:
		target = message.Receiver
		if target == "" && message.Target != 0 {
			target = rt.nameOf(message.Target)
		}
	}
	language := message.LanguageName()
	switch message.Type {
	case world.ChatSystem, world.ChatChannel, world.ChatAFK, world.ChatDND:
		if message.Type == world.ChatSystem {
			language = ""
		}
	}
	rt.FireEvent(event,
		lua.LString(message.Text), lua.LString(author), lua.LString(language), lua.LString(channelText), lua.LString(target),
		lua.LString(chatFlagText(message.Tag)), lua.LNumber(channelRow), lua.LNumber(channelIndex), lua.LString(channelName),
		lua.LNumber(message.Flags), lua.LNumber(rt.nextChatLine()), guidText(message.Sender))
}

// receiveChat takes a chat line from the server, holding it until the name of
// a sender it only knows by guid has arrived.
func (rt *Runtime) receiveChat(message world.ChatMessage) {
	if chatNeedsName(message) && rt.senderName(message) == "" {
		rt.chat.pending = append(rt.chat.pending, pendingChat{message: message, since: time.Now()})
		if rt.chat.requestName != nil {
			rt.chat.requestName(message.Sender)
		}
		return
	}
	rt.deliverChat(message)
}

// FlushChat delivers the lines whose senders have a name now, and, after a
// while, the ones whose senders never answered.
func (rt *Runtime) FlushChat() {
	if len(rt.chat.pending) == 0 {
		return
	}
	waiting := rt.chat.pending[:0:0]
	for _, entry := range rt.chat.pending {
		if rt.senderName(entry.message) != "" || time.Since(entry.since) > 5*time.Second {
			rt.deliverChat(entry.message)
		} else {
			waiting = append(waiting, entry)
		}
	}
	rt.chat.pending = waiting
	rt.flushEmotes()
}

// serverMessageKinds are the globals of the kinds of SMSG_CHAT_SERVER_MESSAGE.
var serverMessageKinds = map[uint32]string{
	1: "SERVER_MESSAGE_SHUTDOWN",
	2: "SERVER_MESSAGE_RESTART",
	4: "SERVER_MESSAGE_SHUTDOWN_CANCELLED",
	5: "SERVER_MESSAGE_RESTART_CANCELLED",
}

// chatErrorGlobals are the globals worded for the packets that refuse a chat
// line.
var chatErrorGlobals = map[uint16]string{
	0x0219: "ERR_CHAT_WRONG_FACTION",
	0x02FD: "ERR_CHAT_RESTRICTED",
	0x0299: "ERR_NOT_IN_GROUP",
}

// handleChatPacket takes the packets that carry chat.
func (rt *Runtime) handleChatPacket(packet world.Packet) bool {
	switch packet.Opcode {
	case world.MessageChat, world.GMMessageChat:
		parse := world.ParseMessageChat
		if packet.Opcode == world.GMMessageChat {
			parse = world.ParseGMMessageChat
		}
		if message, err := parse(packet.Body); err == nil {
			rt.receiveChat(message)
		}
	case world.ChannelNotifyPacket:
		if notify, err := world.ParseChannelNotify(packet.Body); err == nil {
			rt.applyChannelNotify(notify)
		}
	case packetWho:
		rt.applyWho(packet.Body)
	case world.TextEmotePacket:
		if emote, err := world.ParseTextEmote(packet.Body); err == nil {
			rt.receiveTextEmote(emote)
		}
	case world.ChannelListPacket:
		if roster, err := world.ParseChannelList(packet.Body); err == nil {
			rt.applyChannelList(roster)
		}
	default:
		return false
	}
	return true
}

// systemLine writes a line to the chat windows as a system message.
func (rt *Runtime) systemLine(text string) {
	if text == "" {
		return
	}
	rt.FireEvent("CHAT_MSG_SYSTEM",
		lua.LString(text), lua.LString(""), lua.LString(""), lua.LString(""), lua.LString(""), lua.LString(""),
		lua.LNumber(0), lua.LNumber(0), lua.LString(""), lua.LNumber(0), lua.LNumber(rt.nextChatLine()), lua.LString(""))
}

// formatGlobal fills the placeholders of a global string.
func (rt *Runtime) formatGlobal(name, fallback string, args ...any) string {
	return fmt.Sprintf(rt.globalString(name, fallback), args...)
}

// tickChat is called every frame.
func (rt *Runtime) tickChat() {
	rt.FlushChat()
	rt.flushEmotes()
	rt.tickPetition()
}

// chatTypeForName turns the name a script gives a chat kind into the one the
// client sends.
func chatTypeForName(name string) string { return strings.ToUpper(strings.TrimSpace(name)) }

// defaultLanguage is the language the character speaks unless told otherwise:
// Common for the Alliance, Orcish for the Horde.
func (rt *Runtime) defaultLanguage() string {
	if player := rt.unitInfo("player"); player != nil {
		switch player.RaceID {
		case 2, 5, 6, 8, 10:
			return "Orcish"
		}
	}
	return "Common"
}

// addonChatTypes are the kinds of line an addon message can go to.
var addonChatTypes = map[string]world.ChatType{
	"PARTY":        world.ChatParty,
	"RAID":         world.ChatRaid,
	"GUILD":        world.ChatGuild,
	"BATTLEGROUND": world.ChatBattleground,
	"WHISPER":      world.ChatWhisper,
}

// registerChatAPI defines the calls that send chat.
func registerChatAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetDefaultLanguage", func(L *lua.LState) int {
		L.Push(lua.LString(rt.defaultLanguage()))
		return 1
	})
	reg("SendChatMessage", func(L *lua.LState) int {
		host, ok := rt.Host.(WorldChatHost)
		if !ok {
			return 0
		}
		message := L.OptString(1, "")
		chatType := chatTypeForName(L.OptString(2, "SAY"))
		language := rt.defaultLanguage()
		if L.GetTop() >= 3 && L.Get(3) != lua.LNil {
			language = L.Get(3).String()
		}
		target := ""
		if L.GetTop() >= 4 && L.Get(4) != lua.LNil {
			target = L.Get(4).String()
		}
		if chatType == "CHANNEL" {
			target = rt.channelTargetName(target)
		}
		if err := host.SendChatMessage(message, chatType, language, target); err != nil {
			L.RaiseError("SendChatMessage: %v", err)
		}
		return 0
	})
	reg("SendAddonMessage", func(L *lua.LState) int {
		prefix, message := L.OptString(1, ""), L.OptString(2, "")
		if prefix == "" && message == "" {
			L.RaiseError("Usage: SendAddonMessage(\"prefix\", \"message\" [,\"type\"] [,\"target\"])")
			return 0
		}
		chatType := world.ChatParty
		if L.GetTop() >= 3 && L.Get(3).Type() == lua.LTString {
			kind, known := addonChatTypes[strings.ToUpper(L.Get(3).String())]
			if !known {
				L.RaiseError("Unknown addon chat type")
				return 0
			}
			chatType = kind
		}
		if chatType == world.ChatRaid && !rt.isRaid() {
			chatType = world.ChatParty
		}
		if chatType == world.ChatParty && !rt.group.in {
			return 0
		}
		target := L.OptString(4, "")
		if chatType == world.ChatWhisper && target == "" {
			L.RaiseError("SendAddonMessage(): Whisper message missing target player!")
			return 0
		}
		body := binary.LittleEndian.AppendUint32(nil, uint32(chatType))
		body = binary.LittleEndian.AppendUint32(body, 0xFFFFFFFF)
		if chatType == world.ChatWhisper {
			body = append(append(body, target...), 0)
		}
		text := prefix + "\t" + message
		if len(text) >= 3000 {
			text = text[:2999]
		}
		body = append(append(body, text...), 0)
		rt.send(world.ClientMessageChat, body)
		return 0
	})
}
