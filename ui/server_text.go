package ui

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Packets that carry a line of text or a refusal.
const (
	packetNotification  uint16 = 0x01CB
	packetServerMessage uint16 = 0x0291
	packetDefense       uint16 = 0x033A
	packetZoneAttack    uint16 = 0x0254
	packetPlayerMissing uint16 = 0x02A9
	packetAmbiguous     uint16 = 0x032D
	packetNotInGroup    uint16 = 0x0299
	packetChatRestrict  uint16 = 0x02FD
	packetWrongFaction  uint16 = 0x0219
	packetSpiritHealer  uint16 = 0x0222
	packetTalentWipe    uint16 = 0x02AA
	packetBinder        uint16 = 0x02EB

	clientSpiritHealer world.ClientOpcode = 0x021C
	clientBinder       world.ClientOpcode = 0x01B5
	clientTalentWipe   world.ClientOpcode = 0x02AA
)

// Numbers of the client's game errors that these packets raise.
const (
	errChatWrongFaction  = 0xFF  // ERR_CHAT_WRONG_FACTION
	errChatPlayerMissing = 0x100 // ERR_CHAT_PLAYER_NOT_FOUND_S
	errNotInParty        = 0x50  // ERR_NOT_IN_PARTY
	errNotInRaid         = 0x1BD // ERR_NOT_IN_RAID
	errChatAmbiguous     = 0x1FA // ERR_CHAT_PLAYER_AMBIGUOUS_S
	errTalentWipeNobody  = 0x1C5 // the trainer is gone
)

// chatRestrictions are the game errors for the reasons SMSG_CHAT_RESTRICTED
// gives.
var chatRestrictions = [...]int{0x1EF, 0x1F0, 0x231, 0x2A7}

// serverMessageState is what the server messages need: the text of each kind
// and the creatures the character is dealing with.
type serverMessageState struct {
	texts map[uint32]string
	// spiritHealer, binder and trainer are the guids a pending confirmation
	// answers to.
	spiritHealer, binder, trainer uint64
}

// SetServerMessages gives the interface the text of each kind of server
// message (ServerMessages.dbc).
func (rt *Runtime) SetServerMessages(texts map[uint32]string) { rt.serverText.texts = texts }

// handleServerTextPacket takes the packets of notices, announcements and
// refusals.
func (rt *Runtime) handleServerTextPacket(packet world.Packet) bool {
	reader := world.NewReader(packet.Body, "text")
	switch packet.Opcode {
	case packetNotification:
		if text, err := reader.CString(); err == nil {
			rt.FireEvent("UI_ERROR_MESSAGE", lua.LString(text))
		}
	case packetServerMessage:
		kind, err := reader.U32()
		if err != nil {
			return true
		}
		text, _ := reader.CString()
		line := fmt.Sprintf("[%d]: %s", kind, text)
		if format, ok := rt.serverText.texts[kind]; ok {
			if text == "" {
				line = format
			} else {
				line = fmt.Sprintf(format, text)
			}
		}
		rt.ChatLine("CHAT_MSG_SYSTEM", line)
	case packetDefense:
		zone, err := reader.U32()
		if err != nil {
			return true
		}
		reader.U32()
		if text, err := reader.CString(); err == nil {
			rt.defenseLine(zone, text)
		}
	case packetZoneAttack:
		if zone, err := reader.U32(); err == nil {
			if name := rt.zoneName(zone); name != "" {
				rt.defenseLine(zone, fmt.Sprintf(rt.globalString("ZONE_UNDER_ATTACK", "%s is under attack!"), name))
			}
		}
	case packetPlayerMissing:
		if name, err := reader.CString(); err == nil {
			rt.ShowGameError(errChatPlayerMissing, name)
		}
	case packetAmbiguous:
		if name, err := reader.CString(); err == nil {
			rt.ShowGameError(errChatAmbiguous, name)
		}
	case packetNotInGroup:
		kind, err := reader.U32()
		if err != nil {
			return true
		}
		switch world.ChatType(kind) {
		case world.ChatParty, world.ChatPartyLeader:
			rt.ShowGameError(errNotInParty)
		case world.ChatRaid, world.ChatRaidLeader, world.ChatRaidWarning:
			rt.ShowGameError(errNotInRaid)
		}
	case packetChatRestrict:
		if reason, err := reader.U8(); err == nil && int(reason) < len(chatRestrictions) {
			rt.ShowGameError(chatRestrictions[reason])
		}
	case packetWrongFaction:
		rt.ShowGameError(errChatWrongFaction)
	case packetSpiritHealer:
		if guid, err := reader.U64(); err == nil {
			rt.serverText.spiritHealer = guid
			rt.FireEvent("CONFIRM_XP_LOSS")
		}
	case packetTalentWipe:
		guid, err := reader.U64()
		if err != nil {
			return true
		}
		cost, _ := reader.U32()
		if guid == 0 {
			rt.ShowGameError(errTalentWipeNobody)
			return true
		}
		rt.serverText.trainer = guid
		rt.FireEvent("CONFIRM_TALENT_WIPE", lua.LNumber(cost))
	case packetBinder:
		if guid, err := reader.U64(); err == nil {
			rt.serverText.binder = guid
			place := rt.area.subZone
			if place == "" {
				place = rt.area.zone
			}
			rt.FireEvent("CONFIRM_BINDER", lua.LString(place))
		}
	default:
		return false
	}
	return true
}

// defenseLine writes a line of a zone's defence to the defence channels the
// character is in: the ones of the whole world, or of this zone.
func (rt *Runtime) defenseLine(zone uint32, text string) {
	for _, channel := range rt.channels.slots {
		if channel == nil || !channel.joined || channel.left {
			continue
		}
		row := rt.channelRowByID(channel.id)
		if row == nil || row.Flags&0x10000 == 0 {
			continue
		}
		if row.Flags&2 != 0 && rt.zoneName(zone) != rt.area.zone {
			continue
		}
		rt.fireChannelEvent("CHAT_MSG_CHANNEL", text, "", "", channel, "", 0)
	}
}

// registerServerTextAPI defines the answers to the confirmations those
// packets ask.
func registerServerTextAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("AcceptXPLoss", func(L *lua.LState) int {
		if guid := rt.serverText.spiritHealer; guid != 0 {
			rt.send(clientSpiritHealer, binary.LittleEndian.AppendUint64(nil, guid))
		}
		return 0
	})
	reg("ConfirmTalentWipe", func(L *lua.LState) int {
		if guid := rt.serverText.trainer; guid != 0 {
			rt.send(clientTalentWipe, binary.LittleEndian.AppendUint64(nil, guid))
		}
		return 0
	})
	reg("ConfirmBinder", func(L *lua.LState) int {
		if guid := rt.serverText.binder; guid != 0 {
			rt.send(clientBinder, binary.LittleEndian.AppendUint64(nil, guid))
		}
		return 0
	})
	reg("GetResSicknessDuration", func(L *lua.LState) int {
		spell := uint32(15007)
		if player := rt.unitInfo("player"); player != nil {
			for _, race := range rt.who.tables.Races {
				if int(race.ID) == player.RaceID && race.SicknessSpell != 0 {
					spell = race.SicknessSpell
				}
			}
		}
		milliseconds := rt.gameplay.Catalog[spell].DurationMS
		if milliseconds < 1000 {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(rt.durationText(milliseconds / 1000)))
		return 1
	})
}

// durationText words a number of seconds in its largest unit.
func (rt *Runtime) durationText(seconds int) string {
	units := []struct {
		name    string
		seconds int
	}{{"INT_GENERAL_DURATION_DAYS", 86400}, {"INT_GENERAL_DURATION_HOURS", 3600}, {"INT_GENERAL_DURATION_MIN", 60}, {"INT_GENERAL_DURATION_SEC", 1}}
	for _, unit := range units {
		if seconds >= unit.seconds {
			format := rt.globalString(unit.name, "%d")
			return strings.TrimSpace(fmt.Sprintf(format, seconds/unit.seconds))
		}
	}
	return ""
}
