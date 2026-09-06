package world

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

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

func ChatTypeFromName(name string) (ChatType, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "", "SAY":
		return ChatSay, nil
	case "YELL":
		return ChatYell, nil
	case "PARTY":
		return ChatParty, nil
	case "RAID":
		return ChatRaid, nil
	case "GUILD":
		return ChatGuild, nil
	case "OFFICER":
		return ChatOfficer, nil
	case "WHISPER":
		return ChatWhisper, nil
	case "CHANNEL":
		return ChatChannel, nil
	case "EMOTE":
		return ChatEmote, nil
	default:
		return ChatSystem, fmt.Errorf("unsupported outgoing chat type %q", name)
	}
}

func ChatLanguageFromName(name string) (uint32, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 7, nil
	}
	if value, err := strconv.ParseUint(name, 10, 32); err == nil {
		if validChatLanguage(uint32(value)) {
			return uint32(value), nil
		}
	}
	switch strings.ToUpper(name) {
	case "UNIVERSAL":
		return 0, nil
	case "ORCISH":
		return 1, nil
	case "DARNASSIAN":
		return 2, nil
	case "TAURAHE":
		return 3, nil
	case "DWARVISH":
		return 6, nil
	case "COMMON":
		return 7, nil
	case "DEMONIC":
		return 8, nil
	case "TITAN":
		return 9, nil
	case "THALASSIAN":
		return 10, nil
	case "DRACONIC":
		return 11, nil
	case "KALIMAG":
		return 12, nil
	case "GNOMISH":
		return 13, nil
	case "TROLL":
		return 14, nil
	case "GUTTERSPEAK":
		return 33, nil
	case "DRAENEI":
		return 35, nil
	case "ZOMBIE":
		return 36, nil
	case "GNOMISHBINARY":
		return 37, nil
	case "GOBLINBINARY":
		return 38, nil
	default:
		return 0, fmt.Errorf("unsupported outgoing chat language %q", name)
	}
}

func validChatLanguage(language uint32) bool {
	switch language {
	case 0, 1, 2, 3, 6, 7, 8, 9, 10, 11, 12, 13, 14, 33, 35, 36, 37, 38, ^uint32(0):
		return true
	default:
		return false
	}
}

func BuildMessageChat(chatType ChatType, language uint32, target, text string) ([]byte, error) {
	switch chatType {
	case ChatSay, ChatYell, ChatParty, ChatRaid, ChatGuild, ChatOfficer, ChatWhisper, ChatChannel, ChatEmote:
	default:
		return nil, fmt.Errorf("unsupported outgoing chat type %d", chatType)
	}
	if text == "" {
		return nil, fmt.Errorf("outgoing chat message is empty")
	}
	if !validChatLanguage(language) {
		return nil, fmt.Errorf("unsupported outgoing chat language %d", language)
	}
	if (chatType == ChatWhisper || chatType == ChatChannel) && target == "" {
		return nil, fmt.Errorf("outgoing %s chat requires a target", chatType.label())
	}
	if strings.IndexByte(target, 0) >= 0 || strings.IndexByte(text, 0) >= 0 {
		return nil, fmt.Errorf("outgoing chat contains a NUL byte")
	}
	body := make([]byte, 0, 8+len(target)+len(text)+2)
	var word [4]byte
	binary.LittleEndian.PutUint32(word[:], uint32(chatType))
	body = append(body, word[:]...)
	binary.LittleEndian.PutUint32(word[:], language)
	body = append(body, word[:]...)
	if chatType == ChatWhisper || chatType == ChatChannel {
		body = append(body, target...)
		body = append(body, 0)
	}
	body = append(body, text...)
	body = append(body, 0)
	return body, nil
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
