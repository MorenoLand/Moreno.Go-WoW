package ui

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Update fields and flags of the player block.
const (
	fieldDynamicFlags     = 6 + 0x49
	fieldUnitBytes0       = 6 + 0x11
	fieldUnitBytes2       = 6 + 0x74
	fieldChosenTitle      = 148 + 0xAD
	fieldKnownTitles      = 148 + 0x1DE
	playerFlagInPVP       = 0x200
	playerFlagHideHelm    = 0x400
	playerFlagHideCloak   = 0x800
	playerFlagPVPTimer    = 0x40000
	clientPlayedTime      = world.ClientOpcode(0x01CC)
	clientSetTitle        = world.ClientOpcode(0x0374)
	clientShowingHelm     = world.ClientOpcode(0x02B9)
	clientShowingCloak    = world.ClientOpcode(0x02BA)
	clientSetSheathed     = world.ClientOpcode(0x01E0)
	clientDuelAccepted    = world.ClientOpcode(0x016C)
	clientDuelCancelled   = world.ClientOpcode(0x016D)
	clientSummonResponse  = world.ClientOpcode(0x02AC)
	packetPlayedTime      = uint16(0x01CD)
	packetDuelRequested   = uint16(0x0167)
	packetDuelOutOfBounds = uint16(0x0168)
	packetDuelInBounds    = uint16(0x0169)
	packetDuelComplete    = uint16(0x016A)
	packetSummonRequest   = uint16(0x02AB)
	packetTitleEarned     = uint16(0x0373)
	spellDuel             = 7266
)

// CharTitle is a row of CharTitles.dbc.
type CharTitle struct {
	ID, Mask     uint32
	Male, Female string
}

// summonState is a summon the character was offered.
type summonState struct {
	summoner uint64
	zone     uint32
	deadline float64 // seconds on the runtime clock
	pending  bool
}

// miscState is what the character window and the small prompts keep.
type miscState struct {
	titles []CharTitle
	summon summonState
	duel   uint64
}

// SetTitles gives the interface the rows of CharTitles.dbc.
func (rt *Runtime) SetTitles(titles []CharTitle) { rt.misc.titles = titles }

func (rt *Runtime) playerFlagSet(flag uint32) bool { return rt.field(playerFlagsField)&flag != 0 }

// titleText is a title as it reads with the character's name in it.
func (rt *Runtime) titleText(mask uint32) (string, bool) {
	player := rt.unitInfo("player")
	female, name := false, ""
	if player != nil {
		female, name = player.Sex == 3, player.Name
	}
	for _, title := range rt.misc.titles {
		if title.Mask == mask {
			text := title.Male
			if female && title.Female != "" {
				text = title.Female
			}
			return strings.Replace(text, "%s", name, 1), true
		}
	}
	return "", false
}

// titleName is the text of a title for the character's sex with its "%s"
// placeholder and everything after it up to the next space removed.
func (rt *Runtime) titleName(mask uint32) (string, bool) {
	female := false
	if player := rt.unitInfo("player"); player != nil {
		female = player.Sex == 3
	}
	for _, title := range rt.misc.titles {
		if title.Mask != mask {
			continue
		}
		name := title.Male
		if female && title.Female != "" {
			name = title.Female
		}
		var out strings.Builder
		skipping := false
		for _, char := range name {
			switch {
			case char == '%':
				skipping = true
			case skipping && char != ' ':
			default:
				skipping = false
				out.WriteRune(char)
			}
		}
		return out.String(), true
	}
	return "", false
}

func (rt *Runtime) titleKnown(mask uint32) bool {
	if mask == 0 {
		return false
	}
	word := fieldKnownTitles + int(mask>>6)*2 + int(mask>>5&1)
	return rt.field(word)&(1<<(mask&31)) != 0
}

// handleMiscPacket takes the packets of duels, summons, titles and the
// played time.
func (rt *Runtime) handleMiscPacket(packet world.Packet) bool {
	reader := world.NewReader(packet.Body, "misc")
	switch packet.Opcode {
	case packetPlayedTime:
		total, err := reader.U32()
		if err != nil {
			return true
		}
		level, _ := reader.U32()
		rt.FireEvent("TIME_PLAYED_MSG", lua.LNumber(total), lua.LNumber(level))
	case packetDuelRequested:
		arbiter, err := reader.U64()
		if err != nil {
			return true
		}
		challenger, _ := reader.U64()
		rt.misc.duel = arbiter
		rt.FireEvent("DUEL_REQUESTED", lua.LString(rt.nameOf(challenger)))
	case packetDuelOutOfBounds:
		rt.FireEvent("DUEL_OUTOFBOUNDS")
	case packetDuelInBounds:
		rt.FireEvent("DUEL_INBOUNDS")
	case packetDuelComplete:
		rt.misc.duel = 0
		rt.FireEvent("DUEL_FINISHED")
	case packetSummonRequest:
		summoner, err := reader.U64()
		if err != nil {
			return true
		}
		zone, _ := reader.U32()
		timeout, _ := reader.U32()
		rt.misc.summon = summonState{summoner: summoner, zone: zone, pending: true, deadline: time.Since(rt.started).Seconds() + float64(timeout)/1000}
		rt.FireEvent("CONFIRM_SUMMON")
	case packetTitleEarned:
		title, err := reader.U32()
		if err != nil {
			return true
		}
		earned, _ := reader.U32()
		if text, ok := rt.titleText(title); ok {
			key, fallback := "NEW_TITLE_EARNED", "You have earned the title '%s'."
			if earned == 0 {
				key, fallback = "OLD_TITLE_LOST", "You are no longer entitled to the title '%s'."
			}
			rt.say("CHAT_MSG_SYSTEM", fmt.Sprintf(rt.globalString(key, fallback), text))
		}
		rt.FireEvent("KNOWN_TITLES_UPDATE")
	default:
		return false
	}
	return true
}

// unitByNameOrToken finds a unit by its token or, failing that, its name.
func (rt *Runtime) unitByNameOrToken(text string) *UnitInfo {
	if info := rt.unitInfo(text); info != nil && info.Exists {
		return info
	}
	for _, info := range rt.units {
		if info != nil && info.Exists && strings.EqualFold(info.Name, text) {
			return info
		}
	}
	return nil
}

// registerMiscAPI defines the calls of titles, helm and cloak, the PvP flag,
// duels and summons.
func registerMiscAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	flag := func(name string, test func(*lua.LState) bool) {
		reg(name, func(L *lua.LState) int {
			if test(L) {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNil)
			}
			return 1
		})
	}
	reg("GetNumTitles", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.misc.titles) + 1))
		return 1
	})
	reg("GetTitleName", func(L *lua.LState) int {
		name, ok := rt.titleName(uint32(L.CheckInt(1)))
		if !ok {
			return 0
		}
		L.Push(lua.LString(name))
		return 1
	})
	reg("IsTitleKnown", func(L *lua.LState) int {
		L.Push(lua.LNumber(boolNumber(rt.titleKnown(uint32(L.CheckInt(1))))))
		return 1
	})
	reg("GetCurrentTitle", func(L *lua.LState) int {
		L.Push(lua.LNumber(int32(rt.field(fieldChosenTitle))))
		return 1
	})
	reg("SetCurrentTitle", func(L *lua.LState) int {
		rt.send(clientSetTitle, binary.LittleEndian.AppendUint32(nil, uint32(int32(L.CheckInt(1)))))
		return 0
	})
	toggle := func(name string, opcode world.ClientOpcode) {
		reg(name, func(L *lua.LState) int {
			show := byte(1)
			if L.GetTop() >= 1 && !lua.LVAsBool(L.Get(1)) {
				show = 0
			}
			rt.send(opcode, []byte{show})
			return 0
		})
	}
	toggle("ShowHelm", clientShowingHelm)
	toggle("ShowCloak", clientShowingCloak)
	flag("ShowingHelm", func(*lua.LState) bool { return !rt.playerFlagSet(playerFlagHideHelm) })
	flag("ShowingCloak", func(*lua.LState) bool { return !rt.playerFlagSet(playerFlagHideCloak) })
	reg("GetPVPDesired", func(L *lua.LState) int {
		L.Push(lua.LNumber(boolNumber(rt.playerFlagSet(playerFlagInPVP))))
		return 1
	})
	flag("IsPVPTimerRunning", func(*lua.LState) bool { return rt.playerFlagSet(playerFlagPVPTimer) })
	reg("ToggleSheath", func(L *lua.LState) int {
		state := uint32(1)
		if rt.field(fieldUnitBytes2)&0xFF != 0 {
			state = 0
		}
		rt.send(clientSetSheathed, binary.LittleEndian.AppendUint32(nil, state))
		return 0
	})
	reg("RequestTimePlayed", func(L *lua.LState) int {
		rt.send(clientPlayedTime, []byte{1})
		return 0
	})

	unitFlag := func(name string, test func(*UnitInfo) bool) {
		flag(name, func(L *lua.LState) bool {
			info := rt.unitInfo(L.CheckString(1))
			return info != nil && info.Exists && test(info)
		})
	}
	unitFlag("UnitIsAFK", func(info *UnitInfo) bool { return info.Player && info.AFK })
	unitFlag("UnitIsDND", func(info *UnitInfo) bool { return info.Player && info.DND })
	unitFlag("UnitIsTapped", func(info *UnitInfo) bool { return info.Tapped })
	unitFlag("UnitIsTappedByPlayer", func(info *UnitInfo) bool { return info.TappedByPlayer })
	flag("UnitIsInMyGuild", func(L *lua.LState) bool {
		info, player := rt.unitByNameOrToken(L.CheckString(1)), rt.unitInfo("player")
		return info != nil && player != nil && info.GuildID != 0 && info.GuildID == player.GuildID
	})

	reg("StartDuel", func(L *lua.LState) int {
		if err := rt.cast(spellDuel, L.CheckString(1)); err != nil {
			L.RaiseError("StartDuel: %v", err)
		}
		return 0
	})
	reg("AcceptDuel", func(L *lua.LState) int {
		if rt.misc.duel != 0 {
			rt.send(clientDuelAccepted, binary.LittleEndian.AppendUint64(nil, rt.misc.duel))
		}
		return 0
	})
	reg("CancelDuel", func(L *lua.LState) int {
		if rt.misc.duel != 0 {
			rt.send(clientDuelCancelled, binary.LittleEndian.AppendUint64(nil, rt.misc.duel))
			rt.misc.duel = 0
		}
		return 0
	})

	summonAnswer := func(accept byte) {
		summon := &rt.misc.summon
		if !summon.pending {
			return
		}
		body := binary.LittleEndian.AppendUint64(nil, summon.summoner)
		rt.send(clientSummonResponse, append(body, accept))
		summon.pending = false
	}
	reg("ConfirmSummon", func(L *lua.LState) int { summonAnswer(1); return 0 })
	reg("CancelSummon", func(L *lua.LState) int { summonAnswer(0); return 0 })
	reg("GetSummonConfirmSummoner", func(L *lua.LState) int {
		L.Push(lua.LString(rt.nameOf(rt.misc.summon.summoner)))
		return 1
	})
	reg("GetSummonConfirmAreaName", func(L *lua.LState) int {
		L.Push(lua.LString(rt.zoneName(rt.misc.summon.zone)))
		return 1
	})
	reg("GetSummonConfirmTimeLeft", func(L *lua.LState) int {
		left := 0.0
		if rt.misc.summon.pending {
			left = max(rt.misc.summon.deadline-time.Since(rt.started).Seconds(), 0)
		}
		L.Push(lua.LNumber(int(left)))
		return 1
	})
}
