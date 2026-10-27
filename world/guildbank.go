package world

const (
	// The guild bank: the client opens it itself on seeing the object, activates
	// it, then asks for the tab it shows.
	ClientGuildBankActivate  ClientOpcode = 0x03E6
	ClientGuildBankQueryTab  ClientOpcode = 0x03E7
	ClientGuildBankSwapItems ClientOpcode = 0x03E9
	ClientGuildBankBuyTab    ClientOpcode = 0x03EA
	ClientGuildBankUpdateTab ClientOpcode = 0x03EB
	ClientGuildBankDeposit   ClientOpcode = 0x03EC
	ClientGuildBankWithdraw  ClientOpcode = 0x03ED
	ClientGuildBankLogQuery  ClientOpcode = 0x03EE
	ClientGuildBankTextQuery ClientOpcode = 0x040A
	ClientGuildBankSetText   ClientOpcode = 0x040B

	GuildBankList      uint16 = 0x03E8
	GuildBankLogResult uint16 = 0x03EE
	GuildBankTextReply uint16 = 0x040A
)

const (
	// The member's rank rights and what is left of the day's withdrawals; both
	// are asked with an empty request and answered with the same opcode.
	ClientGuildBankPermissions ClientOpcode = 0x03FD
	ClientGuildBankMoneyLeft   ClientOpcode = 0x03FE

	GuildPermissionsReply uint16 = 0x03FD
	GuildBankMoneyLeft    uint16 = 0x03FE
)

// GuildTabPermission is what a rank may do in one tab.
type GuildTabPermission struct {
	Flags uint32
	Limit int32
}

// GuildPermissions is the answer to MSG_GUILD_PERMISSIONS.
type GuildPermissions struct {
	Rank      uint32
	Rights    uint32
	GoldLimit int32
	NumTabs   uint8
	Tabs      [6]GuildTabPermission
}

// ParseGuildPermissions reads MSG_GUILD_PERMISSIONS from the server.
func ParseGuildPermissions(body []byte) (GuildPermissions, error) {
	r := NewReader(body, "MSG_GUILD_PERMISSIONS")
	var info GuildPermissions
	var err error
	if info.Rank, err = r.U32(); err != nil {
		return info, err
	}
	if info.Rights, err = r.U32(); err != nil {
		return info, err
	}
	limit, err := r.U32()
	if err != nil {
		return info, err
	}
	info.GoldLimit = int32(limit)
	if info.NumTabs, err = r.U8(); err != nil {
		return info, err
	}
	for index := range info.Tabs {
		flags, err := r.U32()
		if err != nil {
			return info, err
		}
		left, err := r.U32()
		if err != nil {
			return info, err
		}
		info.Tabs[index] = GuildTabPermission{Flags: flags, Limit: int32(left)}
	}
	return info, nil
}

// ParseGuildBankMoneyLeft reads MSG_GUILD_BANK_MONEY_WITHDRAWN from the server.
func ParseGuildBankMoneyLeft(body []byte) (int32, error) {
	value, err := NewReader(body, "MSG_GUILD_BANK_MONEY_WITHDRAWN").U32()
	return int32(value), err
}

// GuildBankSlots is how many squares a tab has.
const GuildBankSlots = 98

// GuildBankItem is a square of a tab.
type GuildBankItem struct {
	Slot           uint8
	Entry          uint32
	Flags          int32
	RandomProperty int32
	RandomSeed     int32
	Count          int32
	Enchant        int32
	Charges        uint8
}

// GuildBankTab is a tab's name and icon.
type GuildBankTab struct {
	Name string
	Icon string
}

// GuildBankContents is SMSG_GUILD_BANK_LIST.
type GuildBankContents struct {
	Money       uint64
	Tab         uint8
	Withdrawals int32
	Full        bool
	Tabs        []GuildBankTab
	Items       []GuildBankItem
}

// ParseGuildBankList reads SMSG_GUILD_BANK_LIST.
func ParseGuildBankList(body []byte) (GuildBankContents, error) {
	r := NewReader(body, "SMSG_GUILD_BANK_LIST")
	var list GuildBankContents
	var err error
	if list.Money, err = r.U64(); err != nil {
		return list, err
	}
	if list.Tab, err = r.U8(); err != nil {
		return list, err
	}
	withdrawals, err := r.U32()
	if err != nil {
		return list, err
	}
	list.Withdrawals = int32(withdrawals)
	full, err := r.U8()
	if err != nil {
		return list, err
	}
	list.Full = full != 0
	if list.Tab == 0 && list.Full {
		tabs, err := r.U8()
		if err != nil {
			return list, err
		}
		for index := 0; index < int(tabs); index++ {
			var tab GuildBankTab
			if tab.Name, err = r.CString(); err != nil {
				return list, err
			}
			if tab.Icon, err = r.CString(); err != nil {
				return list, err
			}
			list.Tabs = append(list.Tabs, tab)
		}
	}
	count, err := r.U8()
	if err != nil {
		return list, err
	}
	for index := 0; index < int(count); index++ {
		var item GuildBankItem
		if item.Slot, err = r.U8(); err != nil {
			return list, err
		}
		if item.Entry, err = r.U32(); err != nil {
			return list, err
		}
		if item.Entry != 0 {
			words := [2]uint32{}
			for word := range words {
				if words[word], err = r.U32(); err != nil {
					return list, err
				}
			}
			item.Flags, item.RandomProperty = int32(words[0]), int32(words[1])
			if item.RandomProperty != 0 {
				seed, err := r.U32()
				if err != nil {
					return list, err
				}
				item.RandomSeed = int32(seed)
			}
			tail := [2]uint32{}
			for word := range tail {
				if tail[word], err = r.U32(); err != nil {
					return list, err
				}
			}
			item.Count, item.Enchant = int32(tail[0]), int32(tail[1])
			if item.Charges, err = r.U8(); err != nil {
				return list, err
			}
			sockets, err := r.U8()
			if err != nil {
				return list, err
			}
			if err := r.Skip(int(sockets) * 5); err != nil {
				return list, err
			}
		}
		list.Items = append(list.Items, item)
	}
	return list, nil
}

// Kinds of guild bank log lines.
const (
	GuildBankLogDepositItem  = 1
	GuildBankLogWithdrawItem = 2
	GuildBankLogMoveItem     = 3
	GuildBankLogDepositMoney = 4
	GuildBankLogWithdrawMony = 5
	GuildBankLogRepairMoney  = 6
	GuildBankLogMoveItem2    = 7
	GuildBankLogBuySlot      = 9
	GuildBankLogCashFlow     = 10
)

// GuildBankLogLine is one line of a tab's log.
type GuildBankLogLine struct {
	Kind     int8
	Player   uint64
	Entry    uint32
	Count    uint32
	Money    uint32
	OtherTab uint8
	Ago      uint32
}

// GuildBankLog is the log of a tab (tab 6 is the money log).
type GuildBankLog struct {
	Tab   uint8
	Lines []GuildBankLogLine
}

// ParseGuildBankLog reads MSG_GUILD_BANK_LOG_QUERY from the server.
func ParseGuildBankLog(body []byte) (GuildBankLog, error) {
	r := NewReader(body, "MSG_GUILD_BANK_LOG_QUERY")
	var log GuildBankLog
	var err error
	if log.Tab, err = r.U8(); err != nil {
		return log, err
	}
	count, err := r.U8()
	if err != nil {
		return log, err
	}
	for index := 0; index < int(count); index++ {
		var line GuildBankLogLine
		kind, err := r.U8()
		if err != nil {
			return log, err
		}
		line.Kind = int8(kind)
		if line.Player, err = r.U64(); err != nil {
			return log, err
		}
		switch line.Kind {
		case GuildBankLogDepositItem, GuildBankLogWithdrawItem:
			if line.Entry, err = r.U32(); err == nil {
				line.Count, err = r.U32()
			}
		case GuildBankLogMoveItem, GuildBankLogMoveItem2:
			if line.Entry, err = r.U32(); err == nil {
				if line.Count, err = r.U32(); err == nil {
					line.OtherTab, err = r.U8()
				}
			}
		default:
			line.Money, err = r.U32()
		}
		if err != nil {
			return log, err
		}
		if line.Ago, err = r.U32(); err != nil {
			return log, err
		}
		log.Lines = append(log.Lines, line)
	}
	return log, nil
}

// ParseGuildBankText reads the text of a tab.
func ParseGuildBankText(body []byte) (uint8, string, error) {
	r := NewReader(body, "MSG_QUERY_GUILD_BANK_TEXT")
	tab, err := r.U8()
	if err != nil {
		return 0, "", err
	}
	text, err := r.CString()
	return tab, text, err
}

// BuildGuildBankActivate opens the bank (all the tabs' names when full).
func BuildGuildBankActivate(bank uint64, full bool) []byte {
	return append(appendU64(nil, bank), boolByte(full))
}

// BuildGuildBankQueryTab asks for the squares of a tab.
func BuildGuildBankQueryTab(bank uint64, tab uint8, full bool) []byte {
	return append(appendU64(nil, bank), tab, boolByte(full))
}

// BuildGuildBankBuyTab buys the next tab.
func BuildGuildBankBuyTab(bank uint64, tab uint8) []byte {
	return append(appendU64(nil, bank), tab)
}

// BuildGuildBankUpdateTab renames a tab and changes its icon.
func BuildGuildBankUpdateTab(bank uint64, tab uint8, name, icon string) []byte {
	buffer := append(appendU64(nil, bank), tab)
	return appendString(appendString(buffer, name), icon)
}

// BuildGuildBankMoney deposits or withdraws money.
func BuildGuildBankMoney(bank uint64, copper uint32) []byte {
	return appendU32(appendU64(nil, bank), copper)
}

// BuildGuildBankMoveInside moves a square of the bank to another.
func BuildGuildBankMoveInside(bank uint64, destTab, destSlot uint8, srcTab, srcSlot uint8, count uint32) []byte {
	buffer := append(appendU64(nil, bank), 1)
	buffer = append(buffer, destTab, destSlot)
	buffer = appendU32(buffer, 0)
	buffer = append(buffer, srcTab, srcSlot)
	buffer = appendU32(buffer, 0)
	buffer = append(buffer, 0)
	return appendU32(buffer, count)
}

// BuildGuildBankToBag swaps a square of the bank with a bag square: the item
// goes into the bank (toBank) or comes out to the bag.
func BuildGuildBankToBag(bank uint64, tab, slot uint8, bag, bagSlot uint8, toBank bool, count uint32) []byte {
	buffer := append(appendU64(nil, bank), 0)
	buffer = append(buffer, tab, slot)
	buffer = appendU32(buffer, 0)
	buffer = append(buffer, 0, bag, bagSlot, boolByte(!toBank))
	return appendU32(buffer, count)
}

// BuildGuildBankAutoStore moves a square of the bank to the first free bag square.
func BuildGuildBankAutoStore(bank uint64, tab, slot uint8, count uint32) []byte {
	buffer := append(appendU64(nil, bank), 0)
	buffer = append(buffer, tab, slot)
	buffer = appendU32(buffer, 0)
	buffer = append(buffer, 1)
	buffer = appendU32(buffer, count)
	buffer = append(buffer, 1)
	return appendU32(buffer, 0)
}

// BuildGuildBankTab names a tab for the log and text requests.
func BuildGuildBankTab(tab uint8) []byte { return []byte{tab} }

// BuildGuildBankSetText writes the text of a tab.
func BuildGuildBankSetText(tab uint8, text string) []byte {
	return appendString([]byte{tab}, text)
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}
