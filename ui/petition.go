package ui

import (
	"encoding/binary"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// Numbers of the client's game errors that petitions raise.
const (
	errPetitionSigned        = 0x155
	errPetitionSignedBy      = 0x156
	errPetitionDeclinedBy    = 0x157
	errPetitionOfferError    = 0x20B
	errPetitionOffered       = 0x154
	errPetitionSelf          = 0x15C
	errPetitionInGuild       = 0x59
	errPetitionAlreadyAGuild = 0x5C
	errPetitionNeedMoney     = 0x28
	errPetitionNoCharter     = 0x223
	errNoGuildCharter        = 0x85
	errPetitionWrongTarget   = 0x86
	errPetitionOtherSide     = 0x76
	charterItemFlag          = 0x2000
)

// petitionSignResultErrors map the results of signing to game errors.
var petitionSignResultErrors = map[uint32]int{
	1: 0x158, 2: 0x15B, 3: 0x15C, 5: 0x15E, 8: 0x15F, 10: 0x15A, 11: 0x159,
}

// petitionTurnInErrors map the results of turning in a charter.
var petitionTurnInErrors = map[uint32]int{2: 0x15B, 4: 0x15D, 11: 0x5F}

// petitionState is the charters the character deals with.
type petitionState struct {
	// registrar and offers are what the NPC sells.
	registrar uint64
	offers    []world.PetitionOffer
	// chosen is the charter placed in the vendor's slot (arena turn-in).
	chosen uint64
	// info caches the answers to petition queries by id.
	info map[uint32]world.PetitionInfo
	// current is the charter shown in the petition window.
	current     *world.PetitionSignatures
	showPending bool
	// signed is set once the character signed the charter shown.
	signed bool
}

func (state *petitionState) cache() map[uint32]world.PetitionInfo {
	if state.info == nil {
		state.info = make(map[uint32]world.PetitionInfo)
	}
	return state.info
}

// charterItems lists the charters the character carries: the item guid and the
// petition it holds.
func (rt *Runtime) charterItems() map[uint64]uint32 {
	found := make(map[uint64]uint32)
	for guid, item := range rt.inventory.items {
		if item == nil {
			continue
		}
		template := rt.inventory.templates[item.Entry]
		if template != nil && template.Flags&charterItemFlag != 0 {
			found[guid] = item.Enchant
		}
	}
	return found
}

// queryCharters asks about the charters whose answers are not cached.
func (rt *Runtime) queryCharters() {
	for guid, id := range rt.charterItems() {
		if _, known := rt.petitions.info[id]; !known && id != 0 {
			rt.send(world.ClientPetitionQuery, world.BuildPetitionQuery(id, guid))
		}
	}
}

// handlePetitionPacket takes the packets of charters.
func (rt *Runtime) handlePetitionPacket(packet world.Packet) bool {
	state := &rt.petitions
	switch packet.Opcode {
	case world.PetitionShowListPacket:
		list, err := world.ParsePetitionList(packet.Body)
		if err != nil {
			return true
		}
		state.registrar, state.offers = list.Registrar, list.Offers
		state.chosen = 0
		if len(list.Offers) == 1 {
			rt.FireEvent("GUILD_REGISTRAR_SHOW")
		} else {
			rt.FireEvent("PETITION_VENDOR_SHOW")
			rt.FireEvent("PETITION_VENDOR_UPDATE")
		}
	case world.PetitionShowSignaturesPacket:
		signatures, err := world.ParsePetitionSignatures(packet.Body)
		if err != nil {
			return true
		}
		state.current = &signatures
		state.showPending = true
		state.signed = false
		if _, known := state.cache()[signatures.ID]; !known {
			rt.send(world.ClientPetitionQuery, world.BuildPetitionQuery(signatures.ID, signatures.Item))
		}
		for _, signer := range signatures.Signers {
			if rt.nameOf(signer) == "" && rt.chat.requestName != nil {
				rt.chat.requestName(signer)
			}
		}
		rt.tickPetition()
	case world.PetitionQueryPacket:
		info, err := world.ParsePetitionInfo(packet.Body)
		if err != nil {
			return true
		}
		state.cache()[info.ID] = info
		rt.tickPetition()
	case world.PetitionSignResultsPacket:
		reader := world.NewReader(packet.Body, "SMSG_PETITION_SIGN_RESULTS")
		petition, _ := reader.U64()
		signer, _ := reader.U64()
		result, err := reader.U32()
		if err != nil {
			return true
		}
		if player := rt.unitInfo("player"); player == nil || signer != player.GUID {
			if state.current != nil && state.current.Item == petition {
				state.current.Signers = append(state.current.Signers, signer)
			}
			rt.ShowGameError(errPetitionSignedBy, rt.nameOf(signer))
			rt.tickPetition()
			return true
		}
		if result == 0 {
			rt.ShowGameError(errPetitionSigned)
			rt.FireEvent("PETITION_CLOSED")
		} else if index, ok := petitionSignResultErrors[result]; ok {
			rt.ShowGameError(index)
		}
	case world.PetitionDeclinePacket:
		if guid, err := world.NewReader(packet.Body, "MSG_PETITION_DECLINE").U64(); err == nil {
			rt.ShowGameError(errPetitionDeclinedBy, rt.nameOf(guid))
		}
	case world.TurnInPetitionResultsPacket:
		result, err := world.NewReader(packet.Body, "SMSG_TURN_IN_PETITION_RESULTS").U32()
		if err != nil {
			return true
		}
		if result == 0 {
			rt.FireEvent("GUILD_REGISTRAR_CLOSED")
		} else if index, ok := petitionTurnInErrors[result]; ok {
			rt.ShowGameError(index)
		}
	case world.PetitionRenamePacket:
		reader := world.NewReader(packet.Body, "MSG_PETITION_RENAME")
		guid, _ := reader.U64()
		name, err := reader.CString()
		if err != nil || name == "" || state.current == nil || state.current.Item != guid {
			return true
		}
		if info, ok := state.cache()[state.current.ID]; ok {
			info.Name = name
			state.cache()[state.current.ID] = info
			rt.FireEvent("PETITION_SHOW")
		}
	case world.OfferPetitionErrorPacket:
		if guid, err := world.NewReader(packet.Body, "SMSG_OFFER_PETITION_ERROR").U64(); err == nil {
			rt.ShowGameError(errPetitionOfferError, rt.nameOf(guid))
		}
	default:
		return false
	}
	return true
}

// tickPetition shows the petition window once its charter and the names of
// those who signed are known.
func (rt *Runtime) tickPetition() {
	state := &rt.petitions
	if !state.showPending || state.current == nil {
		return
	}
	if _, known := state.cache()[state.current.ID]; !known {
		return
	}
	for _, signer := range state.current.Signers {
		if rt.nameOf(signer) == "" {
			return
		}
	}
	state.showPending = false
	rt.FireEvent("PETITION_SHOW")
}

func (rt *Runtime) currentPetition() (world.PetitionInfo, bool) {
	if rt.petitions.current == nil {
		return world.PetitionInfo{}, false
	}
	info, ok := rt.petitions.cache()[rt.petitions.current.ID]
	return info, ok
}

// registerPetitionAPI defines the calls of charters and their registrars.
func registerPetitionAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	state := &rt.petitions
	reg("GetNumPetitionItems", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(state.offers)))
		return 1
	})
	reg("GetPetitionItemInfo", func(L *lua.LState) int {
		index := L.CheckInt(1)
		if index < 1 || index > len(state.offers) {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNumber(0))
			return 3
		}
		offer := state.offers[index-1]
		template := rt.itemTemplate(offer.Entry)
		if template == nil {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNumber(0))
			return 3
		}
		L.Push(lua.LString(template.Name))
		L.Push(lua.LString(rt.itemIcon(offer.Entry)))
		L.Push(lua.LNumber(offer.Cost))
		return 3
	})
	reg("GetGuildCharterCost", func(L *lua.LState) int {
		cost := uint32(0)
		if len(state.offers) > 0 {
			cost = state.offers[0].Cost
		}
		L.Push(lua.LNumber(cost))
		return 1
	})
	buy := func(L *lua.LState, index int, name string) {
		if state.registrar == 0 || index < 0 || index >= len(state.offers) || name == "" {
			L.Push(lua.LNil)
			return
		}
		offer := state.offers[index]
		if offer.Extra == 0 && rt.unitInfo("player") != nil && rt.unitInfo("player").GuildID != 0 {
			rt.ShowGameError(errPetitionAlreadyAGuild)
			L.Push(lua.LNil)
			return
		}
		if rt.inventory.money < offer.Cost {
			rt.ShowGameError(errPetitionNeedMoney)
			L.Push(lua.LNil)
			return
		}
		rt.send(world.ClientPetitionBuy, world.BuildPetitionBuy(state.registrar, name, offer.Index))
		L.Push(lua.LTrue)
	}
	reg("BuyPetition", func(L *lua.LState) int {
		buy(L, L.CheckInt(1)-1, L.CheckString(2))
		return 1
	})
	reg("BuyGuildCharter", func(L *lua.LState) int {
		buy(L, 0, L.CheckString(1))
		return 1
	})
	reg("ClosePetitionVendor", func(L *lua.LState) int {
		if state.registrar != 0 {
			state.registrar = 0
			rt.FireEvent("PETITION_VENDOR_CLOSED")
		}
		return 0
	})
	reg("CloseGuildRegistrar", func(L *lua.LState) int {
		if state.registrar != 0 {
			state.registrar = 0
			rt.FireEvent("GUILD_REGISTRAR_CLOSED")
		}
		return 0
	})
	reg("ClickPetitionButton", func(L *lua.LState) int {
		cursor := &rt.inventory.cursor
		if state.registrar == 0 {
			return 0
		}
		if !cursor.active {
			state.chosen = 0
			rt.FireEvent("PETITION_VENDOR_UPDATE")
			return 0
		}
		if item := rt.inventory.items[cursor.guid]; item != nil {
			state.chosen = item.GUID
			if rt.clearItemCursor != nil {
				rt.clearItemCursor()
			}
			rt.FireEvent("PETITION_VENDOR_UPDATE")
		}
		return 0
	})
	reg("TurnInPetition", func(L *lua.LState) int {
		if state.registrar != 0 && state.chosen != 0 {
			rt.send(world.ClientTurnInPetition, world.BuildTurnInPetition(state.chosen, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF))
		}
		return 0
	})
	reg("TurnInArenaPetition", func(L *lua.LState) int {
		size := L.CheckInt(1)
		if state.registrar == 0 {
			return 0
		}
		colours := []uint32{0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF}
		if L.GetTop() >= 12 {
			colour := func(r, g, b int) uint32 {
				return 0xFF000000 | uint32(colorByte(float64(L.CheckNumber(r))))<<16 | uint32(colorByte(float64(L.CheckNumber(g))))<<8 | uint32(colorByte(float64(L.CheckNumber(b))))
			}
			colours = []uint32{colour(2, 3, 4), uint32(L.CheckInt(5)), colour(6, 7, 8), uint32(L.CheckInt(9)), colour(10, 11, 12)}
		}
		for guid, id := range rt.charterItems() {
			info, ok := state.info[id]
			if !ok || !info.Arena || int(info.MinSignatures) != size-1 {
				continue
			}
			rt.send(world.ClientTurnInPetition, world.BuildTurnInPetition(guid, colours...))
			return 0
		}
		rt.ShowGameError(errPetitionNoCharter)
		return 0
	})
	reg("TurnInGuildCharter", func(L *lua.LState) int {
		for guid, id := range rt.charterItems() {
			if info, ok := state.info[id]; ok && !info.Arena {
				rt.send(world.ClientTurnInPetition, world.BuildTurnInPetition(guid))
				return 0
			}
		}
		rt.ShowGameError(errNoGuildCharter)
		return 0
	})
	reg("HasFilledPetition", func(L *lua.LState) int {
		for _, id := range rt.charterItems() {
			if info, ok := state.info[id]; ok && info.Arena {
				L.Push(lua.LTrue)
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("GetPetitionInfo", func(L *lua.LState) int {
		info, ok := rt.currentPetition()
		if !ok {
			for range 3 {
				L.Push(lua.LNil)
			}
			L.Push(lua.LNumber(0))
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			L.Push(lua.LNumber(0))
			return 7
		}
		kind := "guild"
		if info.Arena {
			kind = "arena"
		}
		L.Push(lua.LString(kind))
		L.Push(lua.LString(info.Name))
		L.Push(lua.LString(info.Body))
		L.Push(lua.LNumber(info.MaxSignatures))
		if owner := rt.nameOf(state.current.Owner); owner != "" {
			L.Push(lua.LString(owner))
		} else {
			L.Push(lua.LNil)
		}
		if player := rt.unitInfo("player"); player != nil && player.GUID == state.current.Owner {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		L.Push(lua.LNumber(info.MinSignatures))
		return 7
	})
	reg("GetNumPetitionNames", func(L *lua.LState) int {
		count := 0
		if state.current != nil {
			count = len(state.current.Signers)
		}
		L.Push(lua.LNumber(count))
		return 1
	})
	reg("GetPetitionNameInfo", func(L *lua.LState) int {
		index := L.CheckInt(1)
		if state.current != nil && index >= 1 && index <= len(state.current.Signers) {
			if name := rt.nameOf(state.current.Signers[index-1]); name != "" {
				L.Push(lua.LString(name))
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	})
	reg("CanSignPetition", func(L *lua.LState) int {
		info, ok := rt.currentPetition()
		player := rt.unitInfo("player")
		if ok && player != nil {
			if !info.Arena && player.GuildID != 0 {
				L.Push(lua.LNil)
				return 1
			}
			if state.current.Owner == player.GUID || int(info.MaxSignatures) <= len(state.current.Signers) {
				L.Push(lua.LNil)
				return 1
			}
		}
		if state.current != nil && player != nil {
			for _, signer := range state.current.Signers {
				if signer == player.GUID {
					L.Push(lua.LNil)
					return 1
				}
			}
		}
		L.Push(lua.LNumber(1))
		return 1
	})
	reg("SignPetition", func(L *lua.LState) int {
		if state.current != nil {
			rt.send(world.ClientPetitionSign, world.BuildPetitionSign(state.current.Item))
			state.signed = true
		}
		return 0
	})
	reg("OfferPetition", func(L *lua.LState) int {
		info, ok := rt.currentPetition()
		target := rt.unitInfo("target")
		if state.current == nil || !ok {
			return 0
		}
		if target == nil || !target.Exists {
			rt.ShowGameError(errPetitionWrongTarget)
			return 0
		}
		if !target.Player {
			return 0
		}
		player := rt.unitInfo("player")
		if player != nil && target.GUID == player.GUID {
			rt.ShowGameError(errPetitionSelf)
			return 0
		}
		slot := uint32(0)
		if info.Arena {
			slot = info.MinSignatures + 1
		}
		rt.send(world.ClientOfferPetition, world.BuildOfferPetition(slot, state.current.Item, target.GUID))
		rt.ShowGameError(errPetitionOffered, target.Name)
		return 0
	})
	reg("RenamePetition", func(L *lua.LState) int {
		name := strings.TrimSpace(L.CheckString(1))
		if state.current != nil && name != "" {
			rt.send(world.ClientPetitionRename, world.BuildPetitionRename(state.current.Item, name))
		}
		return 0
	})
	reg("ClosePetition", func(L *lua.LState) int {
		if state.current != nil {
			if _, ok := rt.currentPetition(); ok && !state.signed {
				if player := rt.unitInfo("player"); player == nil || state.current.Owner != player.GUID {
					rt.send(world.ClientPetitionDecline, binary.LittleEndian.AppendUint64(nil, state.current.Item))
				}
			}
			state.current = nil
			rt.FireEvent("PETITION_CLOSED")
		}
		return 0
	})
}
