package ui

import lua "github.com/yuin/gopher-lua"

// The cursors the buying hand takes, by their number.
const (
	cursorBuy      = 3
	cursorBuyError = 29
)

// The standing each reputation rank asks for, from hated to exalted.
var reputationRankStanding = [...]int32{-42000, -6000, -3000, 0, 3000, 9000, 21000, 42000}

// The size of a team of each of the arena brackets.
var arenaBracketSize = [...]uint32{2, 3, 5}

// canBuyVendorItem is whether the character has what the merchant asks for an
// item: the gold, the standing, the honor and arena points, the items and the
// team rating.
func (rt *Runtime) canBuyVendorItem(entry, price, extendedCost uint32) bool {
	if rt.inventory.money < price {
		return false
	}
	if template := rt.itemTemplate(entry); template != nil && template.RequiredFaction != 0 {
		rank := int(template.RequiredFactionRank)
		for index := range rt.factions.entries {
			faction := &rt.factions.entries[index]
			if faction.ID == template.RequiredFaction && rank < len(reputationRankStanding) && faction.Total() < reputationRankStanding[rank] {
				return false
			}
		}
	}
	cost, found := rt.extendedCosts[extendedCost]
	if !found {
		return true
	}
	if rt.currency.honor < cost.Honor || rt.currency.arena < cost.Arena {
		return false
	}
	for slot, item := range cost.Items {
		if item != 0 && cost.Counts[slot] > 0 && rt.itemCountOf(item) < int(cost.Counts[slot]) {
			return false
		}
	}
	if cost.Rating != 0 && cost.Bracket < uint32(len(arenaBracketSize)) {
		met := false
		for slot := 0; slot < arenaTeamSlots; slot++ {
			field, ok := rt.playerArenaField(slot)
			if !ok || field.ID == 0 || field.Type < arenaBracketSize[cost.Bracket] || field.PersonalRating < cost.Rating {
				continue
			}
			for _, team := range rt.arena.teams {
				if team.id == field.ID && team.stats.Rating >= cost.Rating {
					met = true
				}
			}
		}
		if !met {
			return false
		}
	}
	return true
}

func registerMerchantCursorAPI(rt *Runtime) {
	rt.L.SetGlobal("ShowMerchantSellCursor", rt.L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: ShowMerchantSellCursor(index)")
			return 0
		}
		index := L.CheckInt(1)
		if !rt.merchant.open || index < 1 || index > len(rt.merchant.vendor.Items) {
			return 0
		}
		item := rt.merchant.vendor.Items[index-1]
		if item.Entry == 0 {
			return 0
		}
		if rt.canBuyVendorItem(item.Entry, item.Price, item.ExtendedCost) {
			rt.cursorOverride = cursorBuy
		} else {
			rt.cursorOverride = cursorBuyError
		}
		return 0
	}))
}
