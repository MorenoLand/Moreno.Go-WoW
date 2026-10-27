package ui

import (
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

var (
	tooltipGreen = rgba{r: 0, g: 1, b: 0, a: 1}
	tooltipRed   = rgba{r: 1, g: 0.125, b: 0.125, a: 1}
)

// itemStatGlobals names the ITEM_MOD_* global string of each item stat type.
var itemStatGlobals = map[uint32]string{
	0: "MANA", 1: "HEALTH", 3: "AGILITY", 4: "STRENGTH", 5: "INTELLECT", 6: "SPIRIT", 7: "STAMINA",
	12: "DEFENSE_SKILL_RATING", 13: "DODGE_RATING", 14: "PARRY_RATING", 15: "BLOCK_RATING", 16: "HIT_MELEE_RATING",
	17: "HIT_RANGED_RATING", 18: "HIT_SPELL_RATING", 19: "CRIT_MELEE_RATING", 20: "CRIT_RANGED_RATING",
	21: "CRIT_SPELL_RATING", 22: "HIT_TAKEN_MELEE_RATING", 23: "HIT_TAKEN_RANGED_RATING", 24: "HIT_TAKEN_SPELL_RATING",
	25: "CRIT_TAKEN_MELEE_RATING", 26: "CRIT_TAKEN_RANGED_RATING", 27: "CRIT_TAKEN_SPELL_RATING",
	28: "HASTE_MELEE_RATING", 29: "HASTE_RANGED_RATING", 30: "HASTE_SPELL_RATING", 31: "HIT_RATING", 32: "CRIT_RATING",
	33: "HIT_TAKEN_RATING", 34: "CRIT_TAKEN_RATING", 35: "RESILIENCE_RATING", 36: "HASTE_RATING", 37: "EXPERTISE_RATING",
	38: "ATTACK_POWER", 39: "RANGED_ATTACK_POWER", 40: "FERAL_ATTACK_POWER", 41: "SPELL_HEALING_DONE", 42: "SPELL_DAMAGE_DONE",
	43: "MANA_REGENERATION", 44: "ARMOR_PENETRATION_RATING", 45: "SPELL_POWER", 46: "HEALTH_REGEN", 47: "SPELL_PENETRATION",
	48: "BLOCK_VALUE",
}

var resistanceNames = [...]string{"HOLY", "FIRE", "NATURE", "FROST", "SHADOW", "ARCANE"}

// replaceFormat fills the first %d / %s style verbs of a global string.
func replaceFormat(format string, args ...interface{}) string {
	format = strings.ReplaceAll(format, "%c", "%s")
	return fmt.Sprintf(format, args...)
}

func signedNumber(value int32) (string, int32) {
	if value < 0 {
		return "-", -value
	}
	return "+", value
}

func (rt *Runtime) gs(name, fallback string) string { return rt.globalString(name, fallback) }

// itemTooltipLines builds the tooltip lines of an item entry the way the
// client's item tooltip does: name, binding, slot and type, damage or armor,
// stats, requirements, spells and flavour text.
func (rt *Runtime) itemTooltipLines(template *world.ItemTemplate, instance *ItemInstance) []tooltipLine {
	var lines []tooltipLine
	add := func(left string, color rgba, wrap bool) {
		if left != "" {
			lines = append(lines, tooltipLine{left: left, leftColor: color, hasLeft: true, wrap: wrap})
		}
	}
	double := func(left, right string, leftColor, rightColor rgba) {
		lines = append(lines, tooltipLine{left: left, right: right, leftColor: leftColor, rightColor: rightColor, hasLeft: true, hasRight: true})
	}
	quality := template.Quality
	nameColor := rgba{a: 1}
	var red, green, blue int
	fmt.Sscanf(itemQualityHex(quality), "%02x%02x%02x", &red, &green, &blue)
	nameColor = rgba{r: float64(red) / 255, g: float64(green) / 255, b: float64(blue) / 255, a: 1}
	add(template.Name, nameColor, false)
	isGear := template.Class == 2 || template.Class == 4
	if isGear && template.ItemLevel > 0 {
		add(replaceFormat(rt.gs("ITEM_LEVEL", "Item Level %d"), template.ItemLevel), tooltipYellow, false)
	}
	bound := instance != nil && instance.Flags&0x1 != 0
	switch {
	case template.Bonding == 1:
		add(rt.gs("ITEM_BIND_ON_PICKUP", "Binds when picked up"), tooltipWhite, false)
	case template.Bonding == 2 && !bound:
		add(rt.gs("ITEM_BIND_ON_EQUIP", "Binds when equipped"), tooltipWhite, false)
	case template.Bonding == 3 && !bound:
		add(rt.gs("ITEM_BIND_ON_USE", "Binds when used"), tooltipWhite, false)
	case template.Bonding == 4 || template.Bonding == 5:
		add(rt.gs("ITEM_BIND_QUEST", "Quest Item"), tooltipWhite, false)
	}
	if bound && template.Bonding != 0 && template.Bonding != 4 && template.Bonding != 5 {
		add(rt.gs("ITEM_SOULBOUND", "Soulbound"), tooltipWhite, false)
	}
	if template.MaxCount == 1 && template.Class != 0 {
		add(rt.gs("ITEM_UNIQUE", "Unique"), tooltipWhite, false)
	}
	className := rt.inventory.classes.SubClass[[2]uint32{template.Class, template.SubClass}]
	slotName := ""
	if template.InventoryType != 0 {
		slotName = rt.gs(inventoryTypeName(template.InventoryType), "")
	}
	switch {
	case template.Class == 1 && template.ContainerSlots > 0: // bags
		add(replaceFormat(rt.gs("CONTAINER_SLOTS", "%d Slot %s"), template.ContainerSlots, className), tooltipWhite, false)
	case slotName != "" || (template.Class == 2 || template.Class == 4) && className != "":
		double(slotName, className, tooltipWhite, tooltipWhite)
	}
	if template.Class == 2 && template.Delay > 0 {
		low, high := template.Damage[0].Min, template.Damage[0].Max
		damage := replaceFormat(rt.gs("DAMAGE_TEMPLATE", "%d - %d Damage"), int(low), int(high))
		speed := fmt.Sprintf("%s %.2f", rt.gs("SPEED", "Speed"), float64(template.Delay)/1000)
		double(damage, speed, tooltipWhite, tooltipWhite)
		if dps := (float64(low) + float64(high)) / 2 / (float64(template.Delay) / 1000); dps > 0 {
			add(replaceFormat(rt.gs("DPS_TEMPLATE", "(%.1f damage per second)"), dps), tooltipWhite, false)
		}
	}
	if template.Armor > 0 {
		add(replaceFormat(rt.gs("ARMOR_TEMPLATE", "%d Armor"), template.Armor), tooltipWhite, false)
	}
	if template.Block > 0 {
		add(replaceFormat(rt.gs("SHIELD_BLOCK_TEMPLATE", "%d Block"), template.Block), tooltipWhite, false)
	}
	// Primary stats read "+5 Strength" in white; ratings and powers read as an
	// "Equip:" sentence in green.
	var equipLines []string
	for _, stat := range template.Stats {
		name, ok := itemStatGlobals[stat.Type]
		if !ok || stat.Value == 0 {
			continue
		}
		if stat.Type <= 7 && stat.Type != 2 {
			sign, value := signedNumber(stat.Value)
			add(replaceFormat(rt.gs("ITEM_MOD_"+name, "%c%d "+name), sign, value), tooltipWhite, false)
			continue
		}
		equipLines = append(equipLines, rt.gs("ITEM_SPELL_TRIGGER_ONEQUIP", "Equip:")+" "+replaceFormat(rt.gs("ITEM_MOD_"+name, "%d"), stat.Value))
	}
	for index, value := range template.Resistances {
		if value > 0 {
			add(replaceFormat(rt.gs("ITEM_RESIST_SINGLE", "+%d %s Resistance"), value, rt.gs("RESISTANCE"+fmt.Sprint(index+1)+"_NAME", resistanceNames[index])), tooltipWhite, false)
		}
	}
	for _, socket := range template.Sockets {
		var key, fallback string
		switch {
		case socket.Color&1 != 0:
			key, fallback = "EMPTY_SOCKET_META", "Meta Socket"
		case socket.Color&2 != 0:
			key, fallback = "EMPTY_SOCKET_RED", "Red Socket"
		case socket.Color&4 != 0:
			key, fallback = "EMPTY_SOCKET_YELLOW", "Yellow Socket"
		case socket.Color&8 != 0:
			key, fallback = "EMPTY_SOCKET_BLUE", "Blue Socket"
		default:
			continue
		}
		add(rt.gs(key, fallback), rgba{r: 0.5, g: 0.5, b: 0.5, a: 1}, false)
	}
	if instance != nil && template.MaxDurability > 0 {
		max := instance.MaxDurability
		if max == 0 {
			max = int(template.MaxDurability)
		}
		add(replaceFormat(rt.gs("DURABILITY_TEMPLATE", "Durability %d / %d"), instance.Durability, max), tooltipWhite, false)
	}
	if template.RequiredLevel > 1 {
		color := tooltipWhite
		if player := rt.unitInfo("player"); player != nil && player.Level > 0 && uint32(player.Level) < template.RequiredLevel {
			color = tooltipRed
		}
		add(replaceFormat(rt.gs("ITEM_MIN_LEVEL", "Requires Level %d"), template.RequiredLevel), color, false)
	}
	for _, line := range equipLines {
		add(line, tooltipGreen, true)
	}
	for _, spell := range template.Spells {
		if spell.ID == 0 {
			continue
		}
		info, ok := rt.gameplay.Catalog[spell.ID]
		if !ok {
			continue
		}
		text := rt.spellTextContext().expand(info.Description, info)
		if text == "" {
			continue
		}
		prefix := ""
		switch spell.Trigger {
		case 0:
			prefix = rt.gs("ITEM_SPELL_TRIGGER_ONUSE", "Use:")
		case 1:
			prefix = rt.gs("ITEM_SPELL_TRIGGER_ONEQUIP", "Equip:")
		case 2:
			prefix = rt.gs("ITEM_SPELL_TRIGGER_ONPROC", "Chance on hit:")
		default:
			continue
		}
		add(prefix+" "+text, tooltipGreen, true)
	}
	if template.Description != "" {
		add(`"`+template.Description+`"`, tooltipYellow, true)
	}
	return lines
}

// fillItemTooltip writes an item's tooltip. It reports whether the item's
// template is known; an unknown item has nothing to show yet and the server is
// asked for it.
func (rt *Runtime) fillItemTooltip(w *widget, entry uint32, instance *ItemInstance) bool {
	template := rt.itemTemplate(entry)
	if template == nil {
		return false
	}
	rt.clearTooltip(w)
	state := w.tooltip()
	state.itemLink = rt.itemLinkFor(entry, 0)
	state.spellKind = "item"
	for _, line := range rt.itemTooltipLines(template, instance) {
		rt.tooltipAddLine(w, line)
	}
	rt.showTooltip(w)
	rt.fire(w, "OnTooltipSetItem", []lua.LValue{w.luaValue(rt.L)})
	return true
}

// itemTooltipMethods are the item setters of GameTooltip.
func (rt *Runtime) itemTooltipMethods() map[string]func(L *lua.LState, w *widget) int {
	return map[string]func(L *lua.LState, w *widget) int{
		"SetBagItem": func(L *lua.LState, w *widget) int {
			item := rt.bagItem(L.CheckInt(2), L.CheckInt(3))
			if item == nil {
				L.Push(lua.LNil)
				return 1
			}
			rt.fillItemTooltip(w, item.Entry, item)
			L.Push(lua.LNumber(1))
			L.Push(lua.LFalse)
			return 2
		},
		"SetInventoryItem": func(L *lua.LState, w *widget) int {
			if L.CheckString(2) != "player" {
				return 0
			}
			item := rt.equippedItem(L.CheckInt(3))
			if item == nil {
				L.Push(lua.LNil)
				return 1
			}
			rt.fillItemTooltip(w, item.Entry, item)
			w.tooltip().spellKind = "equipped"
			L.Push(lua.LNumber(1))
			L.Push(lua.LFalse)
			L.Push(lua.LNumber(1))
			return 3
		},
		"SetHyperlink": func(L *lua.LState, w *widget) int {
			link := L.CheckString(2)
			switch {
			case strings.Contains(link, "item:"):
				rt.fillItemTooltip(w, itemEntryFromLink(link), nil)
			case strings.Contains(link, "spell:"):
				var id uint32
				if index := strings.Index(link, "spell:"); index >= 0 {
					fmt.Sscanf(link[index+6:], "%d", &id)
				}
				rt.fillSpellTooltip(w, id)
			}
			return 0
		},
		"SetLootItem": func(L *lua.LState, w *widget) int {
		if index := L.CheckInt(2); index >= 1 && index <= len(rt.loot.slots) && !rt.loot.slots[index-1].coin {
			rt.fillItemTooltip(w, rt.loot.slots[index-1].item.Entry, nil)
		}
		return 0
		},
		"SetQuestItem": func(L *lua.LState, w *widget) int {
		if entry := rt.dialogQuestItemEntry(L.CheckString(2), L.CheckInt(3)); entry != 0 {
			rt.fillItemTooltip(w, entry, nil)
		}
		return 0
		},
		"SetQuestLogItem": func(L *lua.LState, w *widget) int {
		if entry := rt.logQuestItemEntry(L.CheckString(2), L.CheckInt(3)); entry != 0 {
			rt.fillItemTooltip(w, entry, nil)
		}
		return 0
		},
		"SetMerchantItem": func(L *lua.LState, w *widget) int {
			if index := L.CheckInt(2); index >= 1 && index <= len(rt.merchant.vendor.Items) {
				rt.fillItemTooltip(w, rt.merchant.vendor.Items[index-1].Entry, nil)
			}
			return 0
		},
		"SetTradeSkillItem": func(L *lua.LState, w *widget) int {
		row, ok := rt.tradeRowAt(L.CheckInt(2))
		if !ok || row.header {
			return 0
		}
		info := rt.gameplay.Catalog[row.spell]
		if reagent := L.OptInt(3, 0); reagent > 0 {
			count := 0
			for _, candidate := range info.Reagents {
				if candidate.Item == 0 {
					continue
				}
				if count++; count == reagent {
					rt.fillItemTooltip(w, candidate.Item, nil)
				}
			}
			return 0
		}
		if entry := recipeCreates(info); entry != 0 {
			rt.fillItemTooltip(w, entry, nil)
		} else {
			rt.fillSpellTooltip(w, info.ID)
		}
		return 0
		},
		"SetInboxItem": func(L *lua.LState, w *widget) int {
			if mail := rt.mailAt(L.CheckInt(2)); mail != nil {
				if item := mailAttachmentAt(mail, L.OptInt(3, 1)); item != nil {
					rt.fillItemTooltip(w, item.Entry, nil)
				}
			}
			return 0
		},
		"SetSendMailItem": func(L *lua.LState, w *widget) int {
			if index := L.CheckInt(2) - 1; index >= 0 && index < mailAttachmentSlots && rt.mail.attached[index].guid != 0 {
				rt.fillItemTooltip(w, rt.mail.attached[index].entry, nil)
			}
			return 0
		},
		"SetGuildBankItem": func(L *lua.LState, w *widget) int {
			if item := rt.guildBankItem(L.CheckInt(2), L.CheckInt(3)); item != nil {
				rt.fillItemTooltip(w, item.Entry, nil)
			}
			return 0
		},
	}
}
