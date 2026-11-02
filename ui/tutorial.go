package ui

import (
	"encoding/binary"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Tutorials: the hints the client shows once and the server remembers which.
const (
	tutorialCount       = 60
	tutorialFlagsPacket = 0x00FD
	clientTutorialFlag  = world.ClientOpcode(0x00FE)
	clientTutorialReset = world.ClientOpcode(0x0100)
)

type tutorialState struct {
	loaded bool
	// seen are the tutorials shown this session, flagged the ones the server
	// remembers.
	seen, flagged []uint32
	// completed lists the tutorials finished, in the order they were.
	completed [tutorialCount + 1]int
}

func bitSet(words []uint32, index int) bool {
	return index>>5 < len(words) && words[index>>5]&(1<<(uint(index)&31)) != 0
}

func setBit(words []uint32, index int) {
	if index>>5 < len(words) {
		words[index>>5] |= 1 << (uint(index) & 31)
	}
}

// handleTutorialPacket takes the flags the server remembers.
func (rt *Runtime) handleTutorialPacket(packet world.Packet) bool {
	if packet.Opcode != tutorialFlagsPacket {
		return false
	}
	state := &rt.tutorials
	words := (len(packet.Body) + 3) / 4
	state.seen, state.flagged = make([]uint32, words), make([]uint32, words)
	padded := append([]byte(nil), packet.Body...)
	for len(padded)%4 != 0 {
		padded = append(padded, 0)
	}
	for index := range state.seen {
		word := binary.LittleEndian.Uint32(padded[index*4:])
		state.seen[index], state.flagged[index] = word, word
	}
	state.loaded = len(packet.Body) > 0
	return true
}

func (rt *Runtime) flagTutorial(index int) {
	state := &rt.tutorials
	if bitSet(state.flagged, index) {
		return
	}
	setBit(state.seen, index)
	setBit(state.flagged, index)
	for slot := range state.completed {
		if state.completed[slot] == index+1 {
			break
		}
		if state.completed[slot] == 0 {
			state.completed[slot] = index + 1
			break
		}
	}
	rt.send(clientTutorialFlag, binary.LittleEndian.AppendUint32(nil, uint32(index)))
}

// TriggerTutorial shows a tutorial the character has not seen.
func (rt *Runtime) TriggerTutorial(index int) {
	state := &rt.tutorials
	if !state.loaded || index < 0 || index >= tutorialCount || bitSet(state.seen, index) {
		return
	}
	if value, _ := rt.GetCVar("showTutorials"); value == "0" {
		setBit(state.seen, index)
		rt.flagTutorial(index)
		return
	}
	setBit(state.seen, index)
	rt.FireEvent("TUTORIAL_TRIGGER", lua.LNumber(index+1))
}

func registerTutorialAPI(rt *Runtime) {
	L := rt.L
	state := &rt.tutorials
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	index := func(L *lua.LState, usage string) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("%s", usage)
		}
		return L.CheckInt(1) - 1
	}
	reg("FlagTutorial", func(L *lua.LState) int {
		if id := index(L, `Usage: FlagTutorial("tutorial")`); id >= 0 && id < tutorialCount {
			rt.flagTutorial(id)
		}
		return 0
	})
	reg("IsTutorialFlagged", func(L *lua.LState) int {
		id := index(L, `Usage: Trigger("tutorial")`)
		if id < 0 || id >= tutorialCount {
			return 0
		}
		if state.loaded && bitSet(state.flagged, id) {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("TriggerTutorial", func(L *lua.LState) int {
		if id := index(L, `Usage: Trigger("tutorial")`); id >= 0 && id < tutorialCount {
			rt.TriggerTutorial(id)
		}
		return 0
	})
	reg("ResetTutorials", func(L *lua.LState) int {
		for slot := range state.seen {
			state.seen[slot] = 0
		}
		for slot := range state.flagged {
			state.flagged[slot] = 0
		}
		state.completed = [tutorialCount + 1]int{}
		rt.send(clientTutorialReset, nil)
		return 0
	})
	reg("CanResetTutorials", func(L *lua.LState) int {
		for _, word := range state.seen {
			if word != 0 {
				L.Push(lua.LNumber(1))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("GetNextCompleatedTutorial", func(L *lua.LState) int {
		id := index(L, `Usage: GetNextCompleatedTutorial("tutorial")`)
		found := false
		for slot := 0; slot < tutorialCount; slot++ {
			if found && state.completed[slot] != 0 {
				L.Push(lua.LNumber(state.completed[slot]))
				return 1
			}
			if state.completed[slot] == id+1 {
				found = true
			}
		}
		return 0
	})
	reg("GetPrevCompleatedTutorial", func(L *lua.LState) int {
		id := index(L, `Usage: GetPrevCompleatedTutorial("tutorial")`)
		found := false
		for slot := tutorialCount; slot >= 0; slot-- {
			if found && state.completed[slot] != 0 {
				L.Push(lua.LNumber(state.completed[slot]))
				return 1
			}
			if state.completed[slot] == id+1 {
				found = true
			}
		}
		return 0
	})
}
