package render

import (
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// worldNPCContext is what the packets about talking to creatures need of the world.
type worldNPCContext struct {
	rt       *ui.Runtime
	names    *worldNames
	entities map[uint64]*worldEntity
}

// canRepair reports whether a vendor mends gear.
func (c worldNPCContext) canRepair(guid uint64) bool {
	entity := c.entities[guid]
	return entity != nil && entity.fields[unitNPCFlagsField]&ui.NPCFlagRepair != 0
}

// handle applies a packet about gossip, merchants, trainers or quests; it
// reports whether it was one.
func (c worldNPCContext) handle(packet world.Packet) bool {
	rt := c.rt
	switch packet.Opcode {
	case world.GossipMessage:
		if menu, err := world.ParseGossipMessage(packet.Body); err == nil {
			rt.CloseQuestDialog()
			rt.ShowGossip(menu)
		}
	case world.GossipComplete:
		rt.CloseGossipWindow()
		rt.CloseQuestDialog()
	case world.NPCTextUpdate:
		if text, err := world.ParseNPCTextUpdate(packet.Body); err == nil {
			rt.SetNPCText(text)
		}
	case world.ListInventory:
		vendor, err := world.ParseListInventory(packet.Body)
		if err != nil {
			return true
		}
		if len(vendor.Items) == 0 {
			rt.ShowError(vendorErrorText(rt, vendor.Error))
			return true
		}
		rt.ShowMerchant(vendor, c.canRepair(vendor.GUID))
	case world.TrainerList:
		if trainer, err := world.ParseTrainerList(packet.Body); err == nil {
			rt.ShowTrainer(trainer)
		}
	case world.TrainerBuySucceeded:
		r := world.NewReader(packet.Body, "SMSG_TRAINER_BUY_SUCCEEDED")
		if _, err := r.U64(); err == nil {
			if spell, err := r.U32(); err == nil {
				rt.UpdateTrainerSpell(spell, 2)
			}
		}
	case world.TrainerBuyFailed:
		rt.ShowGlobalError("ERR_TRAINER_NOT_ENOUGH_MONEY")
	case world.BuyFailed:
		r := world.NewReader(packet.Body, "SMSG_BUY_FAILED")
		if _, err := r.U64(); err == nil {
			if _, err := r.U32(); err == nil {
				code, _ := r.U8()
				rt.ShowError(buyErrorText(rt, code))
			}
		}
	case world.QuestGiverQuestList:
		if list, err := world.ParseQuestGiverQuestList(packet.Body); err == nil {
			rt.ShowQuestGreeting(list)
		}
	case world.QuestGiverQuestDetails:
		if details, err := world.ParseQuestDetails(packet.Body); err == nil {
			rt.ShowQuestDetails(details)
		}
	case world.QuestGiverRequestItems:
		if progress, err := world.ParseQuestRequestItems(packet.Body); err == nil {
			rt.ShowQuestProgress(progress)
		}
	case world.QuestGiverOfferReward:
		if offer, err := world.ParseQuestOfferReward(packet.Body); err == nil {
			rt.ShowQuestReward(offer)
		}
	case world.QuestQueryResponse:
		if info, err := world.ParseQuestQueryResponse(packet.Body); err == nil {
			rt.SetQuestInfo(info)
		}
	case world.QuestUpdateAddKill:
		if kill, err := world.ParseQuestUpdateAddKill(packet.Body); err == nil {
			rt.AnnounceQuestKill(kill)
		}
	case world.QuestUpdateAddItem, world.QuestUpdateComplete, world.QuestUpdateFailed, world.QuestUpdateFailedTimer:
		rt.FireEvent("QUEST_LOG_UPDATE")
	case world.QuestGiverQuestComplete:
		rt.CloseQuestDialog()
		rt.FireEvent("QUEST_LOG_UPDATE")
	case world.QuestGiverQuestInvalid, world.QuestGiverQuestFailed:
		rt.ShowGlobalError("ERR_QUEST_FAILED")
	case world.QuestLogFull:
		rt.ShowGlobalError("ERR_QUEST_LOG_FULL")
	default:
		return false
	}
	return true
}

func vendorErrorText(rt *ui.Runtime, code uint8) string {
	return rt.GlobalString("MERCHANT_NO_INVENTORY", "This vendor has nothing to sell.")
}

// buyErrorText names why a purchase failed (BUY_ERR_*).
func buyErrorText(rt *ui.Runtime, code uint8) string {
	switch code {
	case 2:
		return rt.GlobalString("ERR_NOT_ENOUGH_MONEY", "You don't have enough money.")
	case 4:
		return rt.GlobalString("ERR_ITEM_SOLD_OUT", "That item is sold out.")
	case 5:
		return rt.GlobalString("ERR_ITEM_MAX_COUNT", "You can't carry any more of those items.")
	}
	return rt.GlobalString("ERR_VENDOR_DOESNT_BUY", "That item can't be bought.")
}

// worldNPCNames resolves the creature and object names quests refer to.
func worldQuestNames(rt *ui.Runtime, names *worldNames, loader *ui.Loader) {
	sorts := map[int32]string{}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\QuestSort.dbc`); err == nil {
		for row := 0; row < table.records; row++ {
			sorts[int32(table.value(row, 0))] = table.string(row, 1)
		}
	}
	rt.SetQuestNames(func(sort int32) string {
		if sort < 0 {
			return sorts[-sort]
		}
		if area, ok := worldAreas(loader).areas[uint32(sort)]; ok {
			return area.name
		}
		return ""
	}, func(entry int32) string {
		if entry > 0 {
			return names.creatures[uint32(entry)].Name
		}
		return fmt.Sprintf("Object %d", -entry)
	})
}

// interactWithUnit talks to a creature as a right click does and tells the
// interface who "npc" and "questnpc" are.
func (h *clientHost) InteractNPC(guid uint64, flags uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	switch {
	case flags&ui.NPCFlagGossip != 0 || flags&(ui.NPCFlagInnkeeper|ui.NPCFlagBanker|ui.NPCFlagSpiritHealer|ui.NPCFlagFlightMaster) != 0:
		return session.SendRaw(world.ClientGossipHello, world.BuildGossipHello(guid))
	case flags&ui.NPCFlagQuestGiver != 0:
		return session.SendRaw(world.ClientQuestGiverHello, world.BuildGossipHello(guid))
	case flags&ui.NPCFlagVendor != 0:
		return session.SendRaw(world.ClientListInventory, world.BuildGossipHello(guid))
	case flags&ui.NPCFlagTrainer != 0:
		return session.SendRaw(world.ClientTrainerList, world.BuildGossipHello(guid))
	}
	return session.SendRaw(world.ClientGossipHello, world.BuildGossipHello(guid))
}

func (h *clientHost) GossipSelect(guid uint64, menu, option uint32, code string, coded bool) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientGossipSelectOption, world.BuildGossipSelect(guid, menu, option, code, coded))
}

func (h *clientHost) QueryNPCText(textID uint32, guid uint64) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientNPCTextQuery, world.BuildNPCTextQuery(textID, guid))
}

func (h *clientHost) QueryQuest(id uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientQuestQuery, world.BuildQuestID(id))
}

func (h *clientHost) QuestGiverHello(guid uint64) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientQuestGiverHello, world.BuildGossipHello(guid))
}

func (h *clientHost) QuestGiverQuery(guid uint64, quest uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientQuestGiverQuery, world.BuildQuestGiverQuest(guid, quest, false))
}

func (h *clientHost) QuestGiverAccept(guid uint64, quest uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientQuestGiverAccept, world.BuildQuestGiverQuest(guid, quest, false))
}

func (h *clientHost) QuestGiverComplete(guid uint64, quest uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientQuestGiverComplete, world.BuildQuestGiverRequest(guid, quest))
}

func (h *clientHost) QuestGiverRequestReward(guid uint64, quest uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientQuestGiverReqReward, world.BuildQuestGiverRequest(guid, quest))
}

func (h *clientHost) QuestGiverChoose(guid uint64, quest, choice uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientQuestGiverChoose, world.BuildQuestGiverChoose(guid, quest, choice))
}

func (h *clientHost) QuestGiverCancel() error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientQuestGiverCancel, nil)
}

func (h *clientHost) AbandonQuest(slot uint8) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientQuestLogRemove, []byte{slot})
}

func (h *clientHost) BuyItem(vendor uint64, entry, slot, count uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientBuyItem, world.BuildBuyItem(vendor, entry, slot, count))
}

func (h *clientHost) SellItem(vendor, item uint64, count uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientSellItem, world.BuildSellItem(vendor, item, count))
}

func (h *clientHost) RepairAll(vendor uint64) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientRepairItem, world.BuildRepairItem(vendor, 0))
}

func (h *clientHost) CloseVendor() error { return nil }

func (h *clientHost) TrainSpell(trainer uint64, spell uint32) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientTrainerBuySpell, world.BuildTrainerBuy(trainer, spell))
}

// npcFlagsText is for diagnostics.
func npcFlagsText(flags uint32) string {
	var parts []string
	for _, entry := range []struct {
		bit  uint32
		name string
	}{{ui.NPCFlagGossip, "gossip"}, {ui.NPCFlagQuestGiver, "quest"}, {ui.NPCFlagTrainer, "trainer"}, {ui.NPCFlagVendor, "vendor"}, {ui.NPCFlagFlightMaster, "taxi"}, {ui.NPCFlagInnkeeper, "inn"}, {ui.NPCFlagBanker, "bank"}} {
		if flags&entry.bit != 0 {
			parts = append(parts, entry.name)
		}
	}
	return strings.Join(parts, ",")
}
