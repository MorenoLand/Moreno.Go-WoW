package ui

import (
	"encoding/binary"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Game master tickets: the one open ticket of the character, the reply a game
// master left, and the reports of lag.
const (
	gmTicketCreateOpcode    = 0x0205
	gmTicketCreateResult    = 0x0206
	gmTicketUpdateOpcode    = 0x0207
	gmTicketUpdateResult    = 0x0208
	gmTicketGetOpcode       = 0x0211
	gmTicketGetResult       = 0x0212
	gmTicketDeleteOpcode    = 0x0217
	gmTicketDeleteResult    = 0x0218
	gmTicketStatusOpcode    = 0x021A
	gmTicketStatusResult    = 0x021B
	gmResponseErrorOpcode   = 0x04EE
	gmResponseOpcode        = 0x04EF
	gmResponseResolveOpcode = 0x04F0
	gmResponseStatusOpcode  = 0x04F1
	gmReportLagOpcode       = 0x0502

	// gmTicketTextLimit is the longest ticket the client accepts.
	gmTicketTextLimit = 500
	// gmTicketHasTicket is the status of a ticket answer that carries one.
	gmTicketHasTicket = 6
	// gmTicketDeleted is the status of a delete that went through.
	gmTicketDeleted = 9
)

type gmTicketState struct {
	// ticket is the id of the open ticket, response that of a reply waiting
	// to be answered; both are zero without one.
	ticket, response uint32
}

// gmTicketMessage checks the text of a ticket the way the client does and
// reports the game error of one it refuses.
func (rt *Runtime) gmTicketText(text string) bool {
	if utf8.RuneCountInString(text) > gmTicketTextLimit {
		rt.ShowGameError(0x16E)
		return false
	}
	if strings.TrimSpace(text) == "" {
		rt.ShowGameError(0x16D)
		return false
	}
	return true
}

func (rt *Runtime) gmPosition() (mapID uint32, x, y, z float32) {
	state := &rt.worldMap
	return state.playerMap, state.playerX, state.playerY, 0
}

// sendGMTicket asks for a new ticket, or for more help on one that was
// answered.
func (rt *Runtime) sendGMTicket(text string, needResponse, moreHelp bool) {
	if moreHelp {
		if rt.gmTicket.response == 0 {
			return
		}
	} else if rt.gmTicket.ticket != 0 {
		return
	}
	if !rt.gmTicketText(text) {
		return
	}
	mapID, x, y, z := rt.gmPosition()
	body := binary.LittleEndian.AppendUint32(nil, mapID)
	for _, value := range []float32{x, y, z} {
		body = binary.LittleEndian.AppendUint32(body, math.Float32bits(value))
	}
	body = append(body, text...)
	body = append(body, 0)
	kind := uint32(1)
	if needResponse {
		kind = 0x11
	}
	body = binary.LittleEndian.AppendUint32(body, kind)
	if moreHelp {
		body = append(body, 1)
	} else {
		body = append(body, 0)
	}
	// No chat lines are kept to send along: no lines, then an empty log.
	body = binary.LittleEndian.AppendUint32(body, 0)
	body = binary.LittleEndian.AppendUint32(body, 0)
	rt.send(gmTicketCreateOpcode, body)
}

func (rt *Runtime) handleGMTicketPacket(packet world.Packet) bool {
	reader := world.NewReader(packet.Body, "gm ticket")
	switch packet.Opcode {
	case gmTicketCreateResult, gmTicketUpdateResult:
		status, err := reader.U32()
		if err != nil {
			return true
		}
		switch status {
		case 1:
			rt.ShowGameError(0x169)
		case 2, 4:
			rt.send(gmTicketGetOpcode, nil)
		case 3:
			rt.ShowGameError(0x16A)
		case 5:
			rt.ShowGameError(0x16B)
		}
		return true
	case gmTicketGetResult:
		status, err := reader.U32()
		if err != nil {
			return true
		}
		if status != gmTicketHasTicket {
			if status == 0 {
				rt.ShowGameError(0x16C)
			}
			rt.FireEvent("UPDATE_TICKET")
			return true
		}
		rt.gmTicket.response = 0
		id, _ := reader.U32()
		rt.gmTicket.ticket = id
		text, _ := reader.CString()
		category, _ := reader.U8()
		age, _ := reader.U32()
		oldest, _ := reader.U32()
		updated, _ := reader.U32()
		assigned, _ := reader.U8()
		opened, err := reader.U8()
		if err != nil {
			return true
		}
		ageDays := math.Float32frombits(age)
		if ageDays < 0 {
			ageDays = 0
		}
		rt.FireEvent("UPDATE_TICKET", lua.LNumber(category), lua.LString(text), lua.LNumber(ageDays),
			lua.LNumber(math.Float32frombits(oldest)), lua.LNumber(math.Float32frombits(updated)), lua.LNumber(assigned), lua.LNumber(opened))
		return true
	case gmTicketDeleteResult:
		status, err := reader.U32()
		if err != nil {
			return true
		}
		if status != gmTicketDeleted {
			rt.ShowGameError(0x16C)
			return true
		}
		rt.gmTicket = gmTicketState{}
		rt.FireEvent("UPDATE_TICKET")
		return true
	case gmTicketStatusResult:
		status, err := reader.U32()
		if err == nil {
			rt.FireEvent("UPDATE_GM_STATUS", lua.LNumber(status))
		}
		return true
	case gmResponseErrorOpcode:
		rt.ShowGameError(0x2A1)
		return true
	case gmResponseOpcode:
		response, _ := reader.U32()
		ticket, _ := reader.U32()
		description, err := reader.CString()
		if err != nil {
			return true
		}
		var text strings.Builder
		for i := 0; i < 4; i++ {
			part, _ := reader.CString()
			text.WriteString(part)
		}
		rt.gmTicket.response, rt.gmTicket.ticket = response, ticket
		rt.FireEvent("GMRESPONSE_RECEIVED", lua.LString(description), lua.LString(text.String()))
		return true
	case gmResponseStatusOpcode:
		rt.gmTicket = gmTicketState{}
		survey, err := reader.U8()
		if err == nil && survey != 0 {
			rt.FireEvent("GMSURVEY_DISPLAY")
			return true
		}
		rt.FireEvent("UPDATE_TICKET")
		return true
	}
	return false
}

func registerGMTicketAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetGMTicket", func(L *lua.LState) int {
		rt.send(gmTicketGetOpcode, nil)
		return 0
	})
	reg("GetGMStatus", func(L *lua.LState) int {
		rt.send(gmTicketStatusOpcode, nil)
		return 0
	})
	reg("NewGMTicket", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString || L.Get(2).Type() != lua.LTBool {
			L.RaiseError("Usage: NewGMTicket(ticketDescription, needResponse)")
		}
		rt.sendGMTicket(L.CheckString(1), L.ToBool(2), false)
		return 0
	})
	reg("UpdateGMTicket", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError("Usage: UpdateGMTicket(ticketDescription)")
		}
		text := L.CheckString(1)
		if rt.gmTicket.ticket != 0 && rt.gmTicket.response == 0 && rt.gmTicketText(text) {
			rt.send(gmTicketUpdateOpcode, append([]byte(text), 0))
		}
		return 0
	})
	reg("DeleteGMTicket", func(L *lua.LState) int {
		if rt.gmTicket.ticket != 0 && rt.gmTicket.response == 0 {
			rt.send(gmTicketDeleteOpcode, nil)
		}
		return 0
	})
	reg("GMResponseResolve", func(L *lua.LState) int {
		if rt.gmTicket.response != 0 {
			rt.send(gmResponseResolveOpcode, nil)
		}
		return 0
	})
	reg("GMResponseNeedMoreHelp", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTString {
			L.RaiseError("Usage: GMResponseNeedMoreHelp(newTicketDescription)")
		}
		rt.sendGMTicket(L.CheckString(1), true, true)
		return 0
	})
	reg("GMReportLag", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GMReportLag(number)")
		}
		mapID, x, y, z := rt.gmPosition()
		body := binary.LittleEndian.AppendUint32(nil, uint32(L.CheckInt(1)-1))
		body = binary.LittleEndian.AppendUint32(body, mapID)
		for _, value := range []float32{x, y, z} {
			body = binary.LittleEndian.AppendUint32(body, math.Float32bits(value))
		}
		rt.send(gmReportLagOpcode, body)
		return 0
	})
}
