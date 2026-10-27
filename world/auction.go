package world

const (
	// The auction house: the auctioneer's hello opens it, the client asks for
	// lists and the server answers with the auctions.
	ClientAuctionSellItem   ClientOpcode = 0x0256
	ClientAuctionRemoveItem ClientOpcode = 0x0257
	ClientAuctionListItems  ClientOpcode = 0x0258
	ClientAuctionListOwner  ClientOpcode = 0x0259
	ClientAuctionPlaceBid   ClientOpcode = 0x025A
	ClientAuctionListBidder ClientOpcode = 0x0264

	AuctionHello          uint16 = 0x0255
	AuctionCommandResult  uint16 = 0x025B
	AuctionListResult     uint16 = 0x025C
	AuctionOwnerResult    uint16 = 0x025D
	AuctionBidderNotice   uint16 = 0x025E
	AuctionOwnerNotice    uint16 = 0x025F
	AuctionBidderResult   uint16 = 0x0265
	AuctionRemovedNotice  uint16 = 0x028D
	auctionEnchantSlotsOn        = 7
)

// What an auction command result answers.
const (
	AuctionCommandSell   = 0
	AuctionCommandCancel = 1
	AuctionCommandBid    = 2
)

// Auction command errors.
const (
	AuctionOK            = 0
	AuctionErrInventory  = 1
	AuctionErrDatabase   = 2
	AuctionErrNoMoney    = 3
	AuctionErrNoItem     = 4
	AuctionErrHigherBid  = 5
	AuctionErrIncrement  = 7
	AuctionErrOwnBid     = 10
	AuctionErrRestricted = 13
)

// AuctionEntry is an auction of a list.
type AuctionEntry struct {
	ID             uint32
	Entry          uint32
	Enchant        uint32
	RandomProperty int32
	RandomSeed     uint32
	Count          uint32
	Charges        uint32
	Flags          uint32
	Owner          uint64
	StartBid       uint32
	MinOutbid      uint32
	Buyout         uint32
	TimeLeftMS     uint32
	Bidder         uint64
	Bid            uint32
}

// AuctionList is the answer to a list request.
type AuctionList struct {
	Auctions []AuctionEntry
	Total    uint32
	// DelayMS is how long to wait before the next search.
	DelayMS uint32
}

// ParseAuctionList reads SMSG_AUCTION_LIST_RESULT and its owner and bidder
// forms, which share a layout.
func ParseAuctionList(body []byte) (AuctionList, error) {
	r := NewReader(body, "SMSG_AUCTION_LIST_RESULT")
	var list AuctionList
	count, err := r.U32()
	if err != nil {
		return list, err
	}
	for index := uint32(0); index < count; index++ {
		var entry AuctionEntry
		fields := []*uint32{&entry.ID, &entry.Entry}
		for _, field := range fields {
			if *field, err = r.U32(); err != nil {
				return list, err
			}
		}
		for slot := 0; slot < auctionEnchantSlotsOn; slot++ {
			id, err := r.U32()
			if err != nil {
				return list, err
			}
			if slot == 0 {
				entry.Enchant = id
			}
			if err = r.Skip(8); err != nil {
				return list, err
			}
		}
		property, err := r.U32()
		if err != nil {
			return list, err
		}
		entry.RandomProperty = int32(property)
		tail := []*uint32{&entry.RandomSeed, &entry.Count, &entry.Charges, &entry.Flags}
		for _, field := range tail {
			if *field, err = r.U32(); err != nil {
				return list, err
			}
		}
		if entry.Owner, err = r.U64(); err != nil {
			return list, err
		}
		money := []*uint32{&entry.StartBid, &entry.MinOutbid, &entry.Buyout, &entry.TimeLeftMS}
		for _, field := range money {
			if *field, err = r.U32(); err != nil {
				return list, err
			}
		}
		if entry.Bidder, err = r.U64(); err != nil {
			return list, err
		}
		if entry.Bid, err = r.U32(); err != nil {
			return list, err
		}
		list.Auctions = append(list.Auctions, entry)
	}
	if list.Total, err = r.U32(); err != nil {
		return list, err
	}
	list.DelayMS, err = r.U32()
	return list, err
}

// AuctionHelloInfo is MSG_AUCTION_HELLO from the server.
type AuctionHelloInfo struct {
	GUID    uint64
	House   uint32
	Enabled bool
}

// ParseAuctionHello reads MSG_AUCTION_HELLO.
func ParseAuctionHello(body []byte) (AuctionHelloInfo, error) {
	r := NewReader(body, "MSG_AUCTION_HELLO")
	var info AuctionHelloInfo
	var err error
	if info.GUID, err = r.U64(); err != nil {
		return info, err
	}
	if info.House, err = r.U32(); err != nil {
		return info, err
	}
	enabled, err := r.U8()
	info.Enabled = enabled != 0
	return info, err
}

// AuctionResult is SMSG_AUCTION_COMMAND_RESULT.
type AuctionResult struct {
	ID      uint32
	Command uint32
	Error   uint32
}

// ParseAuctionResult reads SMSG_AUCTION_COMMAND_RESULT.
func ParseAuctionResult(body []byte) (AuctionResult, error) {
	r := NewReader(body, "SMSG_AUCTION_COMMAND_RESULT")
	var result AuctionResult
	var err error
	if result.ID, err = r.U32(); err != nil {
		return result, err
	}
	if result.Command, err = r.U32(); err != nil {
		return result, err
	}
	result.Error, err = r.U32()
	return result, err
}

// AuctionQuery is what a search asks for; zero-valued filters match all, and
// the "all" of a class, subclass, slot or quality is 0xFFFFFFFF.
type AuctionQuery struct {
	Name                  string
	MinLevel, MaxLevel    uint8
	Slot, Class, SubClass uint32
	Quality               uint32
	Usable, GetAll        bool
	Offset                uint32
}

// BuildAuctionListItems asks for a page of the auctions that match.
func BuildAuctionListItems(auctioneer uint64, query AuctionQuery) []byte {
	buffer := appendU64(nil, auctioneer)
	buffer = appendU32(buffer, query.Offset)
	buffer = appendString(buffer, query.Name)
	buffer = append(buffer, query.MinLevel, query.MaxLevel)
	buffer = appendU32(buffer, query.Slot)
	buffer = appendU32(buffer, query.Class)
	buffer = appendU32(buffer, query.SubClass)
	buffer = appendU32(buffer, query.Quality)
	buffer = append(buffer, boolByte(query.Usable), boolByte(query.GetAll))
	// No sort columns: the server does not sort, the window does.
	return append(buffer, 0)
}

// BuildAuctionListOwner asks for the auctions the player has up.
func BuildAuctionListOwner(auctioneer uint64) []byte {
	return appendU32(appendU64(nil, auctioneer), 0)
}

// BuildAuctionListBidder asks for the auctions the player bid on.
func BuildAuctionListBidder(auctioneer uint64) []byte {
	return appendU32(appendU32(appendU64(nil, auctioneer), 0), 0)
}

// BuildAuctionPlaceBid bids on an auction.
func BuildAuctionPlaceBid(auctioneer uint64, auction, price uint32) []byte {
	return appendU32(appendU32(appendU64(nil, auctioneer), auction), price)
}

// BuildAuctionRemove cancels an auction of the player's.
func BuildAuctionRemove(auctioneer uint64, auction uint32) []byte {
	return appendU32(appendU64(nil, auctioneer), auction)
}

// BuildAuctionSell puts a stack of an item up; minutes is the run time.
func BuildAuctionSell(auctioneer, item uint64, count, bid, buyout, minutes uint32) []byte {
	buffer := appendU32(appendU64(nil, auctioneer), 1)
	buffer = appendU32(appendU64(buffer, item), count)
	buffer = appendU32(buffer, bid)
	buffer = appendU32(buffer, buyout)
	return appendU32(buffer, minutes)
}
