package world

import (
	"encoding/binary"
	"fmt"
)

const (
	// Talking to a creature: the client says hello and picks options; the
	// server answers with the menu, the text of the greeting and the quests.
	ClientGossipHello        ClientOpcode = 0x017B
	ClientGossipSelectOption ClientOpcode = 0x017C
	ClientNPCTextQuery       ClientOpcode = 0x017F
	ClientListInventory      ClientOpcode = 0x019E
	ClientTrainerList        ClientOpcode = 0x01B0
	ClientTrainerBuySpell    ClientOpcode = 0x01B2
	ClientSellItem           ClientOpcode = 0x01A0
	ClientBuyItem            ClientOpcode = 0x01A2
	ClientBuybackItem        ClientOpcode = 0x0290
	ClientRepairItem         ClientOpcode = 0x02A8

	GossipMessage       uint16 = 0x017D
	GossipComplete      uint16 = 0x017E
	NPCTextUpdate       uint16 = 0x0180
	ListInventory       uint16 = 0x019F
	BuyFailed           uint16 = 0x01A5
	SellItemError       uint16 = 0x01A1
	TrainerList         uint16 = 0x01B1
	TrainerBuySucceeded uint16 = 0x01B3
	TrainerBuyFailed    uint16 = 0x01B4
)

// GossipOption is a line of a creature's menu.
type GossipOption struct {
	ID       uint32
	Icon     uint8
	Coded    bool
	BoxMoney uint32
	Text     string
	BoxText  string
}

// GossipQuest is a quest a gossip menu lists.
type GossipQuest struct {
	ID         uint32
	Icon       uint32
	Level      int32
	Flags      uint32
	Repeatable bool
	Title      string
}

// GossipMenu is SMSG_GOSSIP_MESSAGE.
type GossipMenu struct {
	GUID    uint64
	MenuID  uint32
	TextID  uint32
	Options []GossipOption
	Quests  []GossipQuest
}

// ParseGossipMessage reads SMSG_GOSSIP_MESSAGE.
func ParseGossipMessage(body []byte) (GossipMenu, error) {
	r := NewReader(body, "SMSG_GOSSIP_MESSAGE")
	var menu GossipMenu
	var err error
	if menu.GUID, err = r.U64(); err != nil {
		return menu, err
	}
	if menu.MenuID, err = r.U32(); err != nil {
		return menu, err
	}
	if menu.TextID, err = r.U32(); err != nil {
		return menu, err
	}
	count, err := r.U32()
	if err != nil {
		return menu, err
	}
	if count > 64 {
		return menu, fmt.Errorf("SMSG_GOSSIP_MESSAGE has %d options", count)
	}
	for index := uint32(0); index < count; index++ {
		var option GossipOption
		if option.ID, err = r.U32(); err != nil {
			return menu, err
		}
		if option.Icon, err = r.U8(); err != nil {
			return menu, err
		}
		coded, err := r.U8()
		if err != nil {
			return menu, err
		}
		option.Coded = coded != 0
		if option.BoxMoney, err = r.U32(); err != nil {
			return menu, err
		}
		if option.Text, err = r.CString(); err != nil {
			return menu, err
		}
		if option.BoxText, err = r.CString(); err != nil {
			return menu, err
		}
		menu.Options = append(menu.Options, option)
	}
	quests, err := r.U32()
	if err != nil {
		return menu, err
	}
	if quests > 64 {
		return menu, fmt.Errorf("SMSG_GOSSIP_MESSAGE has %d quests", quests)
	}
	for index := uint32(0); index < quests; index++ {
		quest, err := readGossipQuest(r)
		if err != nil {
			return menu, err
		}
		menu.Quests = append(menu.Quests, quest)
	}
	return menu, nil
}

func readGossipQuest(r *Reader) (GossipQuest, error) {
	var quest GossipQuest
	var err error
	if quest.ID, err = r.U32(); err != nil {
		return quest, err
	}
	if quest.Icon, err = r.U32(); err != nil {
		return quest, err
	}
	level, err := r.U32()
	if err != nil {
		return quest, err
	}
	quest.Level = int32(level)
	if quest.Flags, err = r.U32(); err != nil {
		return quest, err
	}
	repeatable, err := r.U8()
	if err != nil {
		return quest, err
	}
	quest.Repeatable = repeatable != 0
	quest.Title, err = r.CString()
	return quest, err
}

// NPCText is the greeting SMSG_NPC_TEXT_UPDATE gives, for male and female
// players, of the first option with a text.
type NPCText struct {
	ID           uint32
	Male, Female string
}

// ParseNPCTextUpdate reads SMSG_NPC_TEXT_UPDATE and keeps the first non-empty text.
func ParseNPCTextUpdate(body []byte) (NPCText, error) {
	r := NewReader(body, "SMSG_NPC_TEXT_UPDATE")
	var text NPCText
	var err error
	if text.ID, err = r.U32(); err != nil {
		return text, err
	}
	for option := 0; option < 8; option++ {
		if _, err = r.F32(); err != nil {
			return text, err
		}
		male, err := r.CString()
		if err != nil {
			return text, err
		}
		female, err := r.CString()
		if err != nil {
			return text, err
		}
		if err = r.Skip(7 * 4); err != nil {
			return text, err
		}
		if text.Male == "" && male != "" {
			text.Male = male
		}
		if text.Female == "" && female != "" {
			text.Female = female
		}
	}
	return text, nil
}

// BuildGossipHello is the body of CMSG_GOSSIP_HELLO, CMSG_LIST_INVENTORY,
// CMSG_TRAINER_LIST and the other requests made of one creature.
func BuildGossipHello(guid uint64) []byte {
	body := make([]byte, 8)
	binary.LittleEndian.PutUint64(body, guid)
	return body
}

// BuildGossipSelect is the body of CMSG_GOSSIP_SELECT_OPTION.
func BuildGossipSelect(guid uint64, menu, option uint32, code string, coded bool) []byte {
	body := make([]byte, 16)
	binary.LittleEndian.PutUint64(body, guid)
	binary.LittleEndian.PutUint32(body[8:], menu)
	binary.LittleEndian.PutUint32(body[12:], option)
	if coded {
		body = append(append(body, code...), 0)
	}
	return body
}

// BuildNPCTextQuery is the body of CMSG_NPC_TEXT_QUERY.
func BuildNPCTextQuery(textID uint32, guid uint64) []byte {
	body := make([]byte, 12)
	binary.LittleEndian.PutUint32(body, textID)
	binary.LittleEndian.PutUint64(body[4:], guid)
	return body
}

// VendorItem is a line of a merchant's list.
type VendorItem struct {
	Slot, Entry, DisplayID uint32
	// Available is -1 for an unlimited supply.
	Available     int32
	Price         uint32
	MaxDurability uint32
	StackSize     uint32
	ExtendedCost  uint32
}

// Vendor is SMSG_LIST_INVENTORY.
type Vendor struct {
	GUID  uint64
	Items []VendorItem
	// Error is set when the merchant has nothing to sell and says why.
	Error uint8
}

// ParseListInventory reads SMSG_LIST_INVENTORY.
func ParseListInventory(body []byte) (Vendor, error) {
	r := NewReader(body, "SMSG_LIST_INVENTORY")
	var vendor Vendor
	var err error
	if vendor.GUID, err = r.U64(); err != nil {
		return vendor, err
	}
	count, err := r.U8()
	if err != nil {
		return vendor, err
	}
	if count == 0 {
		vendor.Error, _ = r.U8()
		return vendor, nil
	}
	for index := 0; index < int(count); index++ {
		var item VendorItem
		if item.Slot, err = r.U32(); err != nil {
			return vendor, err
		}
		if item.Entry, err = r.U32(); err != nil {
			return vendor, err
		}
		if item.DisplayID, err = r.U32(); err != nil {
			return vendor, err
		}
		available, err := r.U32()
		if err != nil {
			return vendor, err
		}
		item.Available = int32(available)
		if item.Price, err = r.U32(); err != nil {
			return vendor, err
		}
		if item.MaxDurability, err = r.U32(); err != nil {
			return vendor, err
		}
		if item.StackSize, err = r.U32(); err != nil {
			return vendor, err
		}
		if item.ExtendedCost, err = r.U32(); err != nil {
			return vendor, err
		}
		vendor.Items = append(vendor.Items, item)
	}
	return vendor, nil
}

// BuildBuyItem is the body of CMSG_BUY_ITEM.
func BuildBuyItem(vendor uint64, entry, slot, count uint32) []byte {
	body := make([]byte, 21)
	binary.LittleEndian.PutUint64(body, vendor)
	binary.LittleEndian.PutUint32(body[8:], entry)
	binary.LittleEndian.PutUint32(body[12:], slot)
	binary.LittleEndian.PutUint32(body[16:], count)
	return body
}

// BuildSellItem is the body of CMSG_SELL_ITEM.
func BuildSellItem(vendor, item uint64, count uint32) []byte {
	body := make([]byte, 20)
	binary.LittleEndian.PutUint64(body, vendor)
	binary.LittleEndian.PutUint64(body[8:], item)
	binary.LittleEndian.PutUint32(body[16:], count)
	return body
}

// BuildRepairItem is the body of CMSG_REPAIR_ITEM: one item, or everything for
// item 0.
func BuildRepairItem(vendor, item uint64) []byte {
	body := make([]byte, 17)
	binary.LittleEndian.PutUint64(body, vendor)
	binary.LittleEndian.PutUint64(body[8:], item)
	return body
}

// TrainerSpell is a line of a trainer's list.
type TrainerSpell struct {
	ID            uint32
	State         uint8 // 0 available, 1 unavailable, 2 known
	Cost          uint32
	ProfessionDot [2]uint32
	RequiredLevel uint8
	RequiredSkill uint32
	RequiredRank  uint32
	Chain         [3]uint32
}

// Trainer is SMSG_TRAINER_LIST.
type Trainer struct {
	GUID     uint64
	Type     uint32
	Spells   []TrainerSpell
	Greeting string
}

// ParseTrainerList reads SMSG_TRAINER_LIST.
func ParseTrainerList(body []byte) (Trainer, error) {
	r := NewReader(body, "SMSG_TRAINER_LIST")
	var trainer Trainer
	var err error
	if trainer.GUID, err = r.U64(); err != nil {
		return trainer, err
	}
	if trainer.Type, err = r.U32(); err != nil {
		return trainer, err
	}
	count, err := r.U32()
	if err != nil {
		return trainer, err
	}
	if count > 1024 {
		return trainer, fmt.Errorf("SMSG_TRAINER_LIST has %d spells", count)
	}
	for index := uint32(0); index < count; index++ {
		var spell TrainerSpell
		if spell.ID, err = r.U32(); err != nil {
			return trainer, err
		}
		if spell.State, err = r.U8(); err != nil {
			return trainer, err
		}
		if spell.Cost, err = r.U32(); err != nil {
			return trainer, err
		}
		if spell.ProfessionDot[0], err = r.U32(); err != nil {
			return trainer, err
		}
		if spell.ProfessionDot[1], err = r.U32(); err != nil {
			return trainer, err
		}
		if spell.RequiredLevel, err = r.U8(); err != nil {
			return trainer, err
		}
		if spell.RequiredSkill, err = r.U32(); err != nil {
			return trainer, err
		}
		if spell.RequiredRank, err = r.U32(); err != nil {
			return trainer, err
		}
		for chain := range spell.Chain {
			if spell.Chain[chain], err = r.U32(); err != nil {
				return trainer, err
			}
		}
		trainer.Spells = append(trainer.Spells, spell)
	}
	trainer.Greeting, err = r.CString()
	return trainer, err
}

// BuildTrainerBuy is the body of CMSG_TRAINER_BUY_SPELL.
func BuildTrainerBuy(guid uint64, spell uint32) []byte {
	body := make([]byte, 12)
	binary.LittleEndian.PutUint64(body, guid)
	binary.LittleEndian.PutUint32(body[8:], spell)
	return body
}
