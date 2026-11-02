package ui

import (
	"encoding/binary"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// The personal bank: a banker's menu answers with the bank's window, the
// extra bag slots are bought from it.
const (
	bankShowOpcode        = 0x01B8
	bankBuySlotOpcode     = 0x01B9
	bankMaxBagSlots       = 7
	playerBytes2Field     = 154
	gameErrorNotEnoughMon = 0x10F
)

type bankState struct {
	open   bool
	banker uint64
	// prices are the copper costs of the first bag slot upward.
	prices []uint32
}

// SetBankSlotPrices installs what BankBagSlotPrices.dbc asks for each slot.
func (rt *Runtime) SetBankSlotPrices(prices []uint32) { rt.bank.prices = prices }

func (rt *Runtime) bankSlotCost(bought int) uint32 {
	if bought < 0 || bought >= len(rt.bank.prices) {
		return 0
	}
	return rt.bank.prices[bought]
}

func (rt *Runtime) handleBankPacket(packet world.Packet) bool {
	if packet.Opcode != bankShowOpcode {
		return false
	}
	if len(packet.Body) < 8 {
		return true
	}
	rt.bank.open, rt.bank.banker = true, binary.LittleEndian.Uint64(packet.Body)
	rt.CloseGossipWindow()
	rt.FireEvent("BANKFRAME_OPENED")
	return true
}

// CloseBank closes the bank window.
func (rt *Runtime) CloseBank() {
	if !rt.bank.open {
		return
	}
	rt.bank = bankState{prices: rt.bank.prices}
	rt.FireEvent("BANKFRAME_CLOSED")
}

func registerBankAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetNumBankSlots", L.NewFunction(func(L *lua.LState) int {
		bought := 0
		if info := rt.unitInfo("player"); info != nil {
			bought = int(info.BankSlots)
		}
		L.Push(lua.LNumber(bought))
		L.Push(lua.LBool(bought > bankMaxBagSlots-1))
		return 2
	}))
	L.SetGlobal("GetBankSlotCost", L.NewFunction(func(L *lua.LState) int {
		bought := 0
		if info := rt.unitInfo("player"); info != nil {
			bought = int(info.BankSlots)
		}
		L.Push(lua.LNumber(rt.bankSlotCost(bought)))
		return 1
	}))
	L.SetGlobal("PurchaseSlot", L.NewFunction(func(L *lua.LState) int {
		info := rt.unitInfo("player")
		if info == nil || !rt.bank.open || int(info.BankSlots) >= bankMaxBagSlots {
			return 0
		}
		if rt.inventory.money < rt.bankSlotCost(int(info.BankSlots)) {
			rt.ShowGameError(gameErrorNotEnoughMon)
			return 0
		}
		body := make([]byte, 8)
		binary.LittleEndian.PutUint64(body, rt.bank.banker)
		rt.send(bankBuySlotOpcode, body)
		return 0
	}))
	L.SetGlobal("CloseBankFrame", L.NewFunction(func(L *lua.LState) int {
		rt.CloseBank()
		return 0
	}))
}

// The spirit healer of a battleground says when it next raises the dead.
func (rt *Runtime) handleSpiritHealerTime(packet world.Packet) bool {
	if packet.Opcode != world.AreaSpiritHealerTimePacket {
		return false
	}
	reader := world.NewReader(packet.Body, "SMSG_AREA_SPIRIT_HEALER_TIME")
	if _, err := reader.U64(); err != nil {
		return true
	}
	if milliseconds, err := reader.U32(); err == nil {
		rt.spiritHealerAt = time.Now().Add(time.Duration(milliseconds) * time.Millisecond)
	}
	return true
}

func registerSpiritHealerAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("GetAreaSpiritHealerTime", L.NewFunction(func(L *lua.LState) int {
		seconds := 0
		if !rt.spiritHealerAt.IsZero() {
			if left := time.Until(rt.spiritHealerAt); left > 0 {
				seconds = int(left / time.Second)
			}
		}
		L.Push(lua.LNumber(seconds))
		return 1
	}))
}
