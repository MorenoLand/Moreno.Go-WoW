package ui

import (
	"encoding/binary"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The client keeps the last sixty chat lines so that one can be reported as
// spam.
const (
	chatLineKeep      = 60
	complainOpcode    = 0x03C7
	complainChatKind  = 1
	complaintAddedKey = "COMPLAINT_ADDED"
)

type chatLineRecord struct {
	line     int
	guid     uint64
	kind     uint32
	language uint32
	channel  uint32
	name     string
	text     string
	at       time.Time
}

type chatComplaints struct {
	lines []chatLineRecord
	// complained are the players already reported this session.
	complained map[uint64]bool
}

// recordChatLine remembers a delivered chat line.
func (rt *Runtime) recordChatLine(message world.ChatMessage, line int, author string, channel uint32) {
	lines := rt.complaints.lines
	if len(lines) >= chatLineKeep {
		lines = lines[1:]
	}
	rt.complaints.lines = append(lines, chatLineRecord{line: line, guid: message.Sender, kind: uint32(message.Type),
		language: message.Language, channel: channel, name: author, text: message.Text, at: time.Now()})
}

// chatLineFor finds the newest line that matches what is given: a line number,
// or a speaker and optionally the text.
func (rt *Runtime) chatLineFor(name, text string, line int) *chatLineRecord {
	lines := rt.complaints.lines
	for index := len(lines) - 1; index >= 0; index-- {
		record := &lines[index]
		if name != "" && !strings.EqualFold(name, record.name) {
			continue
		}
		if text != "" && !strings.EqualFold(text, record.text) {
			continue
		}
		if line == 0 || line == record.line {
			return record
		}
	}
	return nil
}

// canComplainOf is whether a speaker may be reported: not the player and not
// someone the player ignores.
func (rt *Runtime) canComplainOf(guid uint64) bool {
	if guid == rt.playerGUID() {
		return false
	}
	for _, contact := range rt.social.ignored {
		if contact.GUID == guid {
			return false
		}
	}
	return true
}

func (rt *Runtime) complainOf(record *chatLineRecord) {
	if !rt.canComplainOf(record.guid) {
		return
	}
	if rt.complaints.complained[record.guid] {
		rt.say("CHAT_MSG_SYSTEM", rt.globalString(complaintAddedKey, ""))
		return
	}
	if rt.complaints.complained == nil {
		rt.complaints.complained = map[uint64]bool{}
	}
	rt.complaints.complained[record.guid] = true
	body := []byte{complainChatKind}
	body = binary.LittleEndian.AppendUint64(body, record.guid)
	body = binary.LittleEndian.AppendUint32(body, record.kind)
	body = binary.LittleEndian.AppendUint32(body, record.language)
	body = binary.LittleEndian.AppendUint32(body, record.channel)
	body = binary.LittleEndian.AppendUint32(body, uint32(time.Since(record.at)/time.Second))
	body = append(append(body, record.text...), 0)
	rt.send(complainOpcode, body)
}

// complainArguments reads the three forms of the calls: a line number, a
// name, or a name and the text.
func complainArguments(L *lua.LState, usage string) (name, text string, line int) {
	switch {
	case L.Get(1).Type() == lua.LTNumber:
		line = L.CheckInt(1)
	case L.Get(1).Type() == lua.LTString:
		name = L.CheckString(1)
		if L.Get(2).Type() == lua.LTString {
			text = L.CheckString(2)
		}
	default:
		L.RaiseError("%s", usage)
	}
	return name, text, line
}

func registerChatComplainAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("CanComplainChat", L.NewFunction(func(L *lua.LState) int {
		name, text, line := complainArguments(L, "Usage: CanComplainChat(lineID) or CanComplainChat(name) or CanComplainChat(name, text)")
		if record := rt.chatLineFor(name, text, line); record != nil && rt.canComplainOf(record.guid) {
			L.Push(lua.LNumber(1))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))
	L.SetGlobal("ComplainChat", L.NewFunction(func(L *lua.LState) int {
		name, text, line := complainArguments(L, "Usage: ComplainChat(lineID) or ComplainChat(name) or ComplainChat(name, text)")
		if record := rt.chatLineFor(name, text, line); record != nil {
			rt.complainOf(record)
		}
		return 0
	}))
}
