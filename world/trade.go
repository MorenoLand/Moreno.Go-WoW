package world

import "encoding/binary"

// Trade packets.
const (
	TradeStatusPacket         uint16 = 0x0120
	TradeStatusExtendedPacket uint16 = 0x0121

	ClientInitiateTrade  ClientOpcode = 0x0116
	ClientBeginTrade     ClientOpcode = 0x0117
	ClientAcceptTrade    ClientOpcode = 0x011A
	ClientUnacceptTrade  ClientOpcode = 0x011B
	ClientCancelTrade    ClientOpcode = 0x011C
	ClientSetTradeItem   ClientOpcode = 0x011D
	ClientClearTradeItem ClientOpcode = 0x011E
	ClientSetTradeGold   ClientOpcode = 0x011F
)

// Trade statuses of SMSG_TRADE_STATUS.
const (
	TradeBusy          uint32 = 0
	TradeBegin         uint32 = 1
	TradeOpenWindow    uint32 = 2
	TradeCanceled      uint32 = 3
	TradeAccept        uint32 = 4
	TradeBusy2         uint32 = 5
	TradeNoTarget      uint32 = 6
	TradeBackToTrade   uint32 = 7
	TradeComplete      uint32 = 8
	TradeRejected      uint32 = 9
	TradeTargetTooFar  uint32 = 10
	TradeWrongFaction  uint32 = 11
	TradeCloseWindow   uint32 = 12
	TradeIgnoreYou     uint32 = 14
	TradeYouStunned    uint32 = 15
	TradeTargetStunned uint32 = 16
	TradeYouDead       uint32 = 17
	TradeTargetDead    uint32 = 18
	TradeYouLogout     uint32 = 19
	TradeTargetLogout  uint32 = 20
	TradeTrialAccount  uint32 = 21
	TradeWrongRealm    uint32 = 22
	TradeNotOnTapList  uint32 = 23
)

// TradeSlotCount is the number of squares of a trade window, the last of them
// the one whose item is not traded.
const TradeSlotCount = 7

// TradeStatus is SMSG_TRADE_STATUS.
type TradeStatus struct {
	Status uint32
	Trader uint64 // TradeBegin
	Result uint32 // TradeCloseWindow: the inventory result
	Slot   uint8  // TradeWrongRealm / TradeNotOnTapList
}

// ParseTradeStatus reads SMSG_TRADE_STATUS.
func ParseTradeStatus(body []byte) (TradeStatus, error) {
	r := NewReader(body, "SMSG_TRADE_STATUS")
	var status TradeStatus
	var err error
	if status.Status, err = r.U32(); err != nil {
		return status, err
	}
	switch status.Status {
	case TradeBegin:
		status.Trader, err = r.U64()
	case TradeCloseWindow:
		status.Result, err = r.U32()
	case TradeWrongRealm, TradeNotOnTapList:
		status.Slot, err = r.U8()
	}
	return status, err
}

// TradeItem is an item put in a trade square.
type TradeItem struct {
	Present    bool
	Entry      uint32
	Display    uint32
	Count      uint32
	Wrapped    bool
	GiftFrom   uint64
	Enchant    uint32
	Gems       [3]uint32
	Creator    uint64
	Charges    uint32
	Suffix     uint32
	Random     uint32
	Lock       uint32
	Durability uint32
	MaxDurab   uint32
}

// TradeView is SMSG_TRADE_STATUS_EXTENDED: one side of the trade window.
type TradeView struct {
	Trader bool // the other player's side
	Money  uint32
	Spell  uint32
	Items  [TradeSlotCount]TradeItem
}

// ParseTradeView reads SMSG_TRADE_STATUS_EXTENDED.
func ParseTradeView(body []byte) (TradeView, error) {
	r := NewReader(body, "SMSG_TRADE_STATUS_EXTENDED")
	var view TradeView
	side, err := r.U8()
	if err != nil {
		return view, err
	}
	view.Trader = side != 0
	for range 3 {
		if _, err = r.U32(); err != nil {
			return view, err
		}
	}
	if view.Money, err = r.U32(); err != nil {
		return view, err
	}
	if view.Spell, err = r.U32(); err != nil {
		return view, err
	}
	for r.Remaining() > 0 {
		slot, err := r.U8()
		if err != nil {
			return view, err
		}
		var item TradeItem
		fields := []*uint32{&item.Entry, &item.Display, &item.Count, nil}
		for _, field := range fields {
			value, err := r.U32()
			if err != nil {
				return view, err
			}
			if field != nil {
				*field = value
			} else {
				item.Wrapped = value != 0
			}
		}
		if item.GiftFrom, err = r.U64(); err != nil {
			return view, err
		}
		if item.Enchant, err = r.U32(); err != nil {
			return view, err
		}
		for index := range item.Gems {
			if item.Gems[index], err = r.U32(); err != nil {
				return view, err
			}
		}
		if item.Creator, err = r.U64(); err != nil {
			return view, err
		}
		for _, field := range []*uint32{&item.Charges, &item.Suffix, &item.Random, &item.Lock, &item.MaxDurab, &item.Durability} {
			if *field, err = r.U32(); err != nil {
				return view, err
			}
		}
		item.Present = item.Entry != 0
		if int(slot) < TradeSlotCount {
			view.Items[slot] = item
		}
	}
	return view, nil
}

// BuildInitiateTrade is the body of CMSG_INITIATE_TRADE.
func BuildInitiateTrade(guid uint64) []byte {
	return binary.LittleEndian.AppendUint64(nil, guid)
}

// BuildSetTradeItem is the body of CMSG_SET_TRADE_ITEM.
func BuildSetTradeItem(tradeSlot, bag, slot uint8) []byte {
	return []byte{tradeSlot, bag, slot}
}

// BuildClearTradeItem is the body of CMSG_CLEAR_TRADE_ITEM.
func BuildClearTradeItem(tradeSlot uint8) []byte { return []byte{tradeSlot} }

// BuildSetTradeGold is the body of CMSG_SET_TRADE_GOLD.
func BuildSetTradeGold(copper uint32) []byte {
	return binary.LittleEndian.AppendUint32(nil, copper)
}
