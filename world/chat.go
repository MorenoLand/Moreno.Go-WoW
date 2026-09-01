package world

import "fmt"

type ChatType uint8

const (
	ChatSystem ChatType = iota
	ChatSay
	ChatParty
	ChatRaid
	ChatGuild
	ChatOfficer
	ChatYell
	ChatWhisper
	ChatWhisperForeign
	ChatWhisperInform
	ChatEmote
	ChatTextEmote
	ChatMonsterSay
	ChatMonsterParty
	ChatMonsterYell
	ChatMonsterWhisper
	ChatMonsterEmote
	ChatChannel
)

func chatTypeFromID(id uint8) ChatType {
	switch id {
	case 0x00:
		return ChatSystem
	case 0x01:
		return ChatSay
	case 0x02:
		return ChatParty
	case 0x03:
		return ChatRaid
	case 0x04:
		return ChatGuild
	case 0x05:
		return ChatOfficer
	case 0x06:
		return ChatYell
	case 0x07:
		return ChatWhisper
	case 0x08:
		return ChatWhisperForeign
	case 0x09:
		return ChatWhisperInform
	case 0x0a:
		return ChatEmote
	case 0x0b:
		return ChatTextEmote
	case 0x0c:
		return ChatMonsterSay
	case 0x0d:
		return ChatMonsterParty
	case 0x0e:
		return ChatMonsterYell
	case 0x0f:
		return ChatMonsterWhisper
	case 0x10:
		return ChatMonsterEmote
	case 0x11:
		return ChatChannel
	default:
		return ChatType(id)
	}
}

func (t ChatType) label() string {
	switch t {
	case ChatSay, ChatMonsterSay:
		return "SAY"
	case ChatParty, ChatMonsterParty:
		return "PARTY"
	case ChatRaid:
		return "RAID"
	case ChatGuild:
		return "GUILD"
	case ChatOfficer:
		return "OFFICER"
	case ChatYell, ChatMonsterYell:
		return "YELL"
	case ChatWhisper, ChatWhisperForeign, ChatWhisperInform, ChatMonsterWhisper:
		return "WHISPER"
	case ChatEmote, ChatTextEmote, ChatMonsterEmote:
		return "EMOTE"
	case ChatChannel:
		return "CHANNEL"
	case ChatSystem:
		return "SYSTEM"
	default:
		return "CHAT"
	}
}

type ChatMessage struct {
	Type       ChatType
	Language   uint32
	Sender     uint64
	SenderName string
	Target     uint64
	Channel    string
	Text       string
	Tag        uint8
}

func (t ChatType) EventName() string {
	switch t {
	case ChatSystem:
		return "CHAT_MSG_SYSTEM"
	case ChatSay, ChatMonsterSay:
		return "CHAT_MSG_SAY"
	case ChatParty, ChatMonsterParty:
		return "CHAT_MSG_PARTY"
	case ChatRaid:
		return "CHAT_MSG_RAID"
	case ChatGuild:
		return "CHAT_MSG_GUILD"
	case ChatOfficer:
		return "CHAT_MSG_OFFICER"
	case ChatYell, ChatMonsterYell:
		return "CHAT_MSG_YELL"
	case ChatWhisper, ChatWhisperForeign, ChatMonsterWhisper:
		return "CHAT_MSG_WHISPER"
	case ChatWhisperInform:
		return "CHAT_MSG_WHISPER_INFORM"
	case ChatEmote, ChatMonsterEmote:
		return "CHAT_MSG_EMOTE"
	case ChatTextEmote:
		return "CHAT_MSG_TEXT_EMOTE"
	case ChatChannel:
		return "CHAT_MSG_CHANNEL"
	default:
		return ""
	}
}

func (m ChatMessage) LanguageName() string {
	switch m.Language {
	case 0:
		return "Universal"
	case 1:
		return "Orcish"
	case 2:
		return "Darnassian"
	case 3:
		return "Taurahe"
	case 6:
		return "Dwarvish"
	case 7:
		return "Common"
	case 8:
		return "Demonic"
	case 9:
		return "Titan"
	case 10:
		return "Thalassian"
	default:
		return "Universal"
	}
}

func (m ChatMessage) Line() string {
	if m.SenderName != "" {
		return fmt.Sprintf("[%s] %s: %s", m.Type.label(), m.SenderName, m.Text)
	}
	return fmt.Sprintf("[%s] %s", m.Type.label(), m.Text)
}

func ParseMessageChat(body []byte) (ChatMessage, error) {
	r := NewReader(body, "SMSG_MESSAGECHAT")
	chatType, err := r.U8()
	if err != nil {
		return ChatMessage{}, err
	}
	language, err := r.U32()
	if err != nil {
		return ChatMessage{}, err
	}
	sender, err := r.U64()
	if err != nil {
		return ChatMessage{}, err
	}
	if err := r.Skip(4); err != nil {
		return ChatMessage{}, err
	}
	message := ChatMessage{Type: chatTypeFromID(chatType), Language: language, Sender: sender}
	if message.Type == ChatMonsterSay || message.Type == ChatMonsterParty || message.Type == ChatMonsterYell || message.Type == ChatMonsterWhisper || message.Type == ChatMonsterEmote {
		if _, err := r.U32(); err != nil {
			return ChatMessage{}, err
		}
		if message.SenderName, err = r.CString(); err != nil {
			return ChatMessage{}, err
		}
		if message.Target, err = r.U64(); err != nil {
			return ChatMessage{}, err
		}
	} else {
		if message.Type == ChatChannel {
			if message.Channel, err = r.CString(); err != nil {
				return ChatMessage{}, err
			}
		}
		if message.Target, err = r.U64(); err != nil {
			return ChatMessage{}, err
		}
	}
	if _, err := r.U32(); err != nil {
		return ChatMessage{}, err
	}
	if message.Text, err = r.CString(); err != nil {
		return ChatMessage{}, err
	}
	if message.Tag, err = r.U8(); err != nil {
		return ChatMessage{}, err
	}
	if err := r.Finish(); err != nil {
		return ChatMessage{}, err
	}
	return message, nil
}
