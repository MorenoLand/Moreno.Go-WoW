package render

import (
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// unitLootableFlag is the dynamic flag of a corpse that holds loot.
const unitLootableFlag = 0x1

// handleLootPacket applies a loot packet to the loot window; it reports
// whether it was one.
func handleLootPacket(rt *ui.Runtime, packet world.Packet) bool {
	switch packet.Opcode {
	case world.LootResponse:
		if loot, err := world.ParseLootResponse(packet.Body); err == nil && !loot.Failed {
			rt.OpenLoot(loot)
		}
	case world.LootRemoved:
		if slot, err := world.ParseLootRemoved(packet.Body); err == nil {
			rt.ClearLootSlot(slot)
		}
	case world.LootClearMoney:
		rt.ClearLootMoney()
	case world.LootReleaseResponse:
		if guid, err := world.ParseLootReleaseResponse(packet.Body); err == nil {
			rt.CloseLootWindow(guid)
		}
	case world.LootMoneyNotify:
	default:
		return false
	}
	return true
}

func (h *clientHost) LootCorpse(guid uint64) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientLoot, world.BuildLootGUID(guid))
}

func (h *clientHost) LootItemSlot(slot uint8) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.AutoStoreLootItemOpcode, world.BuildLootSlot(slot))
}

func (h *clientHost) LootMoney() error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientLootMoney, nil)
}

func (h *clientHost) ReleaseLoot(guid uint64) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientLootRelease, world.BuildLootGUID(guid))
}
