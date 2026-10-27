package ui

import (
	"fmt"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// Spell effects the trade skill window is built around.
const (
	spellEffectTradeSkill   = 47
	spellEffectLearnSpell   = 36
	spellEffectSkill        = 118
	spellEffectEnchantItem  = 53
	spellEffectEnchantTemp  = 54
	spellEffectCreateItem   = 24
	spellEffectCreateItem2  = 157
	spellEffectCreateRandom = 59
)

// tradeRow is a line of the recipe list: a category header or a recipe.
type tradeRow struct {
	header bool
	name   string
	spell  uint32
}

type tradeSkillState struct {
	open      bool
	line      uint32
	collapsed map[string]bool
	selected  int
	// Filters.
	nameFilter   string
	minLevel     int
	maxLevel     int
	onlyMakeable bool
	// subclassOff and slotOff list the categories and slots the player turned
	// off; the choice "all" is none of them.
	subclassOff map[string]bool
	slotOff     map[string]bool
	// repeat is the crafting still to do after the cast that is running.
	repeatSpell uint32
	repeatLeft  int
	// toolNames are the names of the tool categories (TotemCategory.dbc).
	toolNames map[uint32]string
}

// SetToolNames installs the names of the tools recipes ask for.
func (rt *Runtime) SetToolNames(names map[uint32]string) { rt.tradeSkill.toolNames = names }

// isTradeSkillSpell reports whether a spell opens a profession's window.
func isTradeSkillSpell(info SpellInfo) bool {
	for _, effect := range info.Effects {
		if effect.Effect == spellEffectTradeSkill {
			return true
		}
	}
	return false
}

// openTradeSkillFor opens the window of the profession a spell belongs to,
// as the client does once the profession's spell has been cast.
func (rt *Runtime) openTradeSkillFor(spell uint32) {
	info, ok := rt.gameplay.Catalog[spell]
	if !ok || !isTradeSkillSpell(info) || info.SkillLine == 0 {
		return
	}
	state := &rt.tradeSkill
	state.line = info.SkillLine
	state.selected = 0
	state.nameFilter, state.minLevel, state.maxLevel, state.onlyMakeable = "", 0, 0, false
	state.subclassOff, state.slotOff = nil, nil
	state.collapsed = map[string]bool{}
	state.repeatLeft = 0
	state.open = true
	for _, row := range rt.tradeRows() {
		if !row.header {
			break
		}
	}
	rt.FireEvent("TRADE_SKILL_SHOW")
}

// CloseTradeSkill closes the recipe window.
func (rt *Runtime) CloseTradeSkill() {
	if !rt.tradeSkill.open {
		return
	}
	rt.tradeSkill.open = false
	rt.tradeSkill.repeatLeft = 0
	rt.FireEvent("TRADE_SKILL_CLOSE")
}

// refreshTradeSkill tells the window its numbers changed.
func (rt *Runtime) refreshTradeSkill() {
	if rt.tradeSkill.open {
		rt.FireEvent("TRADE_SKILL_UPDATE")
	}
}

// tradeSkillCastDone goes on with a crafting run once a cast ends.
func (rt *Runtime) tradeSkillCastDone(spell uint32, failed bool) {
	state := &rt.tradeSkill
	if state.repeatLeft <= 0 || state.repeatSpell != spell {
		return
	}
	if failed {
		state.repeatLeft = 0
		return
	}
	state.repeatLeft--
	if rt.recipeAvailable(spell) <= 0 {
		state.repeatLeft = 0
		return
	}
	_ = rt.cast(spell, "player")
}

// recipeCreates is the item a recipe makes, when it makes one.
func recipeCreates(info SpellInfo) uint32 {
	for index, effect := range info.Effects {
		switch effect.Effect {
		case spellEffectCreateItem, spellEffectCreateItem2, spellEffectCreateRandom:
			if info.Created[index] != 0 {
				return info.Created[index]
			}
		}
	}
	return 0
}

// isRecipe tells whether a known spell of the open skill is something to make.
func isRecipe(info SpellInfo) bool {
	if isTradeSkillSpell(info) {
		return false
	}
	for _, effect := range info.Effects {
		switch effect.Effect {
		case spellEffectLearnSpell, spellEffectSkill:
			return false
		}
	}
	if recipeCreates(info) != 0 {
		return true
	}
	for _, reagent := range info.Reagents {
		if reagent.Item != 0 {
			return true
		}
	}
	for _, effect := range info.Effects {
		if effect.Effect == spellEffectEnchantItem || effect.Effect == spellEffectEnchantTemp {
			return true
		}
	}
	return false
}

// recipeAvailable is how many times the carried items allow a recipe to be made.
func (rt *Runtime) recipeAvailable(spell uint32) int {
	info := rt.gameplay.Catalog[spell]
	count := -1
	for _, reagent := range info.Reagents {
		if reagent.Item == 0 || reagent.Count == 0 {
			continue
		}
		made := rt.itemCountOf(reagent.Item) / int(reagent.Count)
		if count < 0 || made < count {
			count = made
		}
	}
	if count < 0 {
		return 0
	}
	return count
}

// recipeHeader names the category a recipe is listed under: the kind of item
// it makes.
func (rt *Runtime) recipeHeader(info SpellInfo) string {
	if entry := recipeCreates(info); entry != 0 {
		local := rt.inventory.local[entry]
		if name := rt.inventory.classes.SubClass[[2]uint32{local.Class, local.SubClass}]; name != "" {
			return name
		}
		if name := rt.inventory.classes.Class[local.Class]; name != "" {
			return name
		}
	}
	return rt.globalString("OTHER", "Other")
}

// recipeSlot names the inventory slot of the item a recipe makes.
func (rt *Runtime) recipeSlot(info SpellInfo) string {
	if entry := recipeCreates(info); entry != 0 {
		if kind := rt.inventory.local[entry].InventoryType; kind != 0 {
			return rt.globalString(fmt.Sprintf("INVTYPE_%s", strings.ToUpper(inventoryTypeConstant(kind))), inventoryTypeName(kind))
		}
	}
	return ""
}

func inventoryTypeConstant(kind uint32) string {
	names := map[uint32]string{1: "HEAD", 2: "NECK", 3: "SHOULDER", 4: "BODY", 5: "CHEST", 6: "WAIST", 7: "LEGS", 8: "FEET", 9: "WRIST", 10: "HAND",
		11: "FINGER", 12: "TRINKET", 13: "WEAPON", 14: "SHIELD", 15: "RANGED", 16: "CLOAK", 17: "2HWEAPON", 18: "BAG", 19: "TABARD", 20: "ROBE",
		21: "WEAPONMAINHAND", 22: "WEAPONOFFHAND", 23: "HOLDABLE", 24: "AMMO", 25: "THROWN", 26: "RANGEDRIGHT", 27: "QUIVER", 28: "RELIC"}
	return names[kind]
}

// skillRankOf is the character's rank in a skill line.
func (rt *Runtime) skillRankOf(line uint32) (name string, rank, max int) {
	for _, entry := range rt.skills.entries {
		if entry.ID == line {
			return entry.Name, entry.Rank, entry.Max
		}
	}
	return rt.gameplay.Skills[line].Name, 0, 0
}

// recipeDifficulty is the colour word of a recipe at the skill rank.
func recipeDifficulty(info SpellInfo, rank int) string {
	low, high := int(info.SkillLow), int(info.SkillHigh)
	switch {
	case rank >= high:
		return "trivial"
	case rank >= (low+high)/2:
		return "easy"
	case rank >= low:
		return "medium"
	}
	return "optimal"
}

// knownRecipes lists the recipes of the open skill the character knows.
func (rt *Runtime) knownRecipes() []uint32 {
	state := &rt.tradeSkill
	var list []uint32
	for id := range rt.gameplay.Known {
		info, ok := rt.gameplay.Catalog[id]
		if !ok || info.SkillLine != state.line || !isRecipe(info) {
			continue
		}
		if info.Superseded != 0 && rt.gameplay.Known[info.Superseded] {
			continue
		}
		list = append(list, id)
	}
	return list
}

// tradeSkillSubClasses are the categories the open skill lists, in order.
func (rt *Runtime) tradeSkillSubClasses() []string {
	seen := map[string]bool{}
	var names []string
	for _, id := range rt.knownRecipes() {
		name := rt.recipeHeader(rt.gameplay.Catalog[id])
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (rt *Runtime) tradeSkillSlots() []string {
	seen := map[string]bool{}
	var names []string
	for _, id := range rt.knownRecipes() {
		if name := rt.recipeSlot(rt.gameplay.Catalog[id]); name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// tradeRows lays the recipes out under their headers, through the filters.
func (rt *Runtime) tradeRows() []tradeRow {
	state := &rt.tradeSkill
	_, rank, _ := rt.skillRankOf(state.line)
	groups := map[string][]uint32{}
	for _, id := range rt.knownRecipes() {
		info := rt.gameplay.Catalog[id]
		header := rt.recipeHeader(info)
		if state.subclassOff[header] {
			continue
		}
		if slot := rt.recipeSlot(info); slot != "" && state.slotOff[slot] {
			continue
		}
		if state.nameFilter != "" && !strings.Contains(strings.ToLower(info.Name), strings.ToLower(state.nameFilter)) {
			continue
		}
		if state.onlyMakeable && rt.recipeAvailable(id) <= 0 {
			continue
		}
		if state.maxLevel > 0 {
			if entry := recipeCreates(info); entry != 0 {
				if template := rt.itemTemplateIfKnown(entry); template != nil && (int(template.ItemLevel) < state.minLevel || int(template.ItemLevel) > state.maxLevel) {
					continue
				}
			}
		}
		groups[header] = append(groups[header], id)
	}
	headers := make([]string, 0, len(groups))
	for header := range groups {
		headers = append(headers, header)
	}
	sort.Strings(headers)
	var rows []tradeRow
	for _, header := range headers {
		rows = append(rows, tradeRow{header: true, name: header})
		if state.collapsed[header] {
			continue
		}
		list := groups[header]
		sort.Slice(list, func(a, b int) bool {
			first, second := rt.gameplay.Catalog[list[a]], rt.gameplay.Catalog[list[b]]
			if first.SkillLow != second.SkillLow {
				return first.SkillLow < second.SkillLow
			}
			return first.Name < second.Name
		})
		for _, id := range list {
			rows = append(rows, tradeRow{name: rt.gameplay.Catalog[id].Name, spell: id})
		}
	}
	_ = rank
	return rows
}

func (rt *Runtime) tradeRowAt(index int) (tradeRow, bool) {
	rows := rt.tradeRows()
	if index < 1 || index > len(rows) {
		return tradeRow{}, false
	}
	return rows[index-1], true
}

// recipeLink is the chat link of a recipe: its item, or the enchant.
func (rt *Runtime) recipeLink(info SpellInfo) string {
	if entry := recipeCreates(info); entry != 0 {
		return rt.itemLinkFor(entry, 0)
	}
	skill, _, _ := rt.skillRankOf(rt.tradeSkill.line)
	return fmt.Sprintf("|cffffd000|Henchant:%d|h[%s: %s]|h|r", info.ID, skill, info.Name)
}

// registerTradeSkillAPI defines the calls the profession window makes.
func registerTradeSkillAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	state := &rt.tradeSkill
	reg("GetTradeSkillLine", func(L *lua.LState) int {
		name, rank, max := rt.skillRankOf(state.line)
		L.Push(lua.LString(name))
		L.Push(lua.LNumber(rank))
		L.Push(lua.LNumber(max))
		return 3
	})
	reg("GetNumTradeSkills", func(L *lua.LState) int { L.Push(lua.LNumber(len(rt.tradeRows()))); return 1 })
	reg("GetFirstTradeSkill", func(L *lua.LState) int {
		for index, row := range rt.tradeRows() {
			if !row.header {
				L.Push(lua.LNumber(index + 1))
				return 1
			}
		}
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("GetTradeSkillInfo", func(L *lua.LState) int {
		row, ok := rt.tradeRowAt(L.CheckInt(1))
		if !ok {
			return 0
		}
		if row.header {
			L.Push(lua.LString(row.name))
			L.Push(lua.LString("header"))
			L.Push(lua.LNumber(0))
			L.Push(lua.LBool(!state.collapsed[row.name]))
			return 4
		}
		info := rt.gameplay.Catalog[row.spell]
		_, rank, _ := rt.skillRankOf(state.line)
		L.Push(lua.LString(info.Name))
		L.Push(lua.LString(recipeDifficulty(info, rank)))
		L.Push(lua.LNumber(rt.recipeAvailable(row.spell)))
		L.Push(lua.LFalse)
		L.Push(lua.LNil)
		L.Push(lua.LNumber(1))
		return 6
	})
	spellOf := func(L *lua.LState) (SpellInfo, bool) {
		row, ok := rt.tradeRowAt(L.CheckInt(1))
		if !ok || row.header {
			return SpellInfo{}, false
		}
		return rt.gameplay.Catalog[row.spell], true
	}
	reg("GetTradeSkillIcon", func(L *lua.LState) int {
		if info, ok := spellOf(L); ok {
			icon := info.Icon
			if entry := recipeCreates(info); entry != 0 {
				icon = rt.itemIcon(entry)
			}
			L.Push(lua.LString(icon))
			return 1
		}
		return 0
	})
	reg("GetTradeSkillDescription", func(L *lua.LState) int {
		if info, ok := spellOf(L); ok && info.Description != "" {
			L.Push(lua.LString(rt.spellTextContext().expand(info.Description, info)))
			return 1
		}
		return 0
	})
	reg("GetTradeSkillItemLink", func(L *lua.LState) int {
		if info, ok := spellOf(L); ok {
			if link := rt.recipeLink(info); link != "" {
				L.Push(lua.LString(link))
				return 1
			}
		}
		return 0
	})
	reg("GetTradeSkillRecipeLink", func(L *lua.LState) int {
		if info, ok := spellOf(L); ok {
			L.Push(lua.LString(rt.recipeLink(info)))
			return 1
		}
		return 0
	})
	reg("GetTradeSkillNumMade", func(L *lua.LState) int {
		info, ok := spellOf(L)
		if !ok {
			return 0
		}
		made := 1
		for _, effect := range info.Effects {
			if effect.Effect == spellEffectCreateItem || effect.Effect == spellEffectCreateItem2 {
				made = max(effect.BasePoints+1, 1)
				if effect.DieSides > 1 {
					L.Push(lua.LNumber(made))
					L.Push(lua.LNumber(made + effect.DieSides - 1))
					return 2
				}
				break
			}
		}
		L.Push(lua.LNumber(made))
		L.Push(lua.LNumber(made))
		return 2
	})
	reagents := func(info SpellInfo) []SpellReagent {
		var list []SpellReagent
		for _, reagent := range info.Reagents {
			if reagent.Item != 0 {
				list = append(list, reagent)
			}
		}
		return list
	}
	reg("GetTradeSkillNumReagents", func(L *lua.LState) int {
		info, _ := spellOf(L)
		L.Push(lua.LNumber(len(reagents(info))))
		return 1
	})
	reg("GetTradeSkillReagentInfo", func(L *lua.LState) int {
		row, ok := rt.tradeRowAt(L.CheckInt(1))
		if !ok || row.header {
			return 0
		}
		list := reagents(rt.gameplay.Catalog[row.spell])
		index := L.CheckInt(2)
		if index < 1 || index > len(list) {
			return 0
		}
		reagent := list[index-1]
		name := ""
		if template := rt.itemTemplate(reagent.Item); template != nil {
			name = template.Name
		}
		L.Push(lua.LString(name))
		L.Push(lua.LString(rt.itemIcon(reagent.Item)))
		L.Push(lua.LNumber(reagent.Count))
		L.Push(lua.LNumber(rt.itemCountOf(reagent.Item)))
		return 4
	})
	reg("GetTradeSkillReagentItemLink", func(L *lua.LState) int {
		row, ok := rt.tradeRowAt(L.CheckInt(1))
		if !ok || row.header {
			return 0
		}
		list := reagents(rt.gameplay.Catalog[row.spell])
		if index := L.CheckInt(2); index >= 1 && index <= len(list) {
			if link := rt.itemLinkFor(list[index-1].Item, 0); link != "" {
				L.Push(lua.LString(link))
				return 1
			}
		}
		return 0
	})
	reg("GetTradeSkillTools", func(L *lua.LState) int {
		info, ok := spellOf(L)
		if !ok {
			return 0
		}
		count := 0
		for _, tool := range info.Tools {
			if name := state.toolNames[tool]; tool != 0 && name != "" {
				L.Push(lua.LString(name))
				L.Push(lua.LTrue)
				count += 2
			}
		}
		return count
	})
	reg("GetTradeSkillCooldown", func(L *lua.LState) int {
		info, ok := spellOf(L)
		if !ok {
			return 0
		}
		if left := rt.spellCooldownLeft(info.ID); left > 0 {
			L.Push(lua.LNumber(left))
			return 1
		}
		return 0
	})
	reg("GetTradeSkillListLink", func(L *lua.LState) int { return 0 })
	reg("IsTradeSkillLinked", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("GetTradeSkillSelectionIndex", func(L *lua.LState) int { L.Push(lua.LNumber(state.selected)); return 1 })
	reg("SelectTradeSkill", func(L *lua.LState) int { state.selected = L.CheckInt(1); return 0 })
	toggle := func(collapsed bool) lua.LGFunction {
		return func(L *lua.LState) int {
			index := L.CheckInt(1)
			if state.collapsed == nil {
				state.collapsed = map[string]bool{}
			}
			if index == 0 {
				for _, name := range rt.tradeSkillSubClasses() {
					state.collapsed[name] = collapsed
				}
			} else if row, ok := rt.tradeRowAt(index); ok && row.header {
				state.collapsed[row.name] = collapsed
			}
			rt.FireEvent("TRADE_SKILL_UPDATE")
			return 0
		}
	}
	reg("ExpandTradeSkillSubClass", toggle(false))
	reg("CollapseTradeSkillSubClass", toggle(true))
	reg("DoTradeSkill", func(L *lua.LState) int {
		row, ok := rt.tradeRowAt(L.CheckInt(1))
		if !ok || row.header {
			return 0
		}
		count := max(L.OptInt(2, 1), 1)
		state.repeatSpell, state.repeatLeft = row.spell, count-1
		if err := rt.cast(row.spell, "player"); err != nil {
			state.repeatLeft = 0
		}
		return 0
	})
	reg("StopTradeSkillRepeat", func(L *lua.LState) int { state.repeatLeft = 0; return 0 })
	reg("CloseTradeSkill", func(L *lua.LState) int { rt.CloseTradeSkill(); return 0 })
	// A filter is told only when it changes: the window sets the filters from
	// the very event that reports one.
	reg("TradeSkillOnlyShowMakeable", func(L *lua.LState) int {
		if value := L.OptBool(1, false); value != state.onlyMakeable {
			state.onlyMakeable = value
			rt.FireEvent("TRADE_SKILL_FILTER_UPDATE")
		}
		return 0
	})
	reg("SetTradeSkillItemNameFilter", func(L *lua.LState) int {
		if value := L.OptString(1, ""); value != state.nameFilter {
			state.nameFilter = value
			rt.FireEvent("TRADE_SKILL_FILTER_UPDATE")
		}
		return 0
	})
	reg("SetTradeSkillItemLevelFilter", func(L *lua.LState) int {
		minLevel, maxLevel := toInt(L, 1), toInt(L, 2)
		if minLevel != state.minLevel || maxLevel != state.maxLevel {
			state.minLevel, state.maxLevel = minLevel, maxLevel
			rt.FireEvent("TRADE_SKILL_FILTER_UPDATE")
		}
		return 0
	})
	reg("GetTradeSkillSubClasses", func(L *lua.LState) int {
		names := rt.tradeSkillSubClasses()
		for _, name := range names {
			L.Push(lua.LString(name))
		}
		return len(names)
	})
	reg("GetTradeSkillInvSlots", func(L *lua.LState) int {
		names := rt.tradeSkillSlots()
		for _, name := range names {
			L.Push(lua.LString(name))
		}
		return len(names)
	})
	filterOf := func(off map[string]bool, names func() []string) lua.LGFunction {
		return func(L *lua.LState) int {
			index := L.CheckInt(1)
			list := names()
			if index == 0 {
				all := len(off) == 0
				L.Push(lua.LBool(all))
				return 1
			}
			L.Push(lua.LBool(index <= len(list) && !off[list[index-1]]))
			return 1
		}
	}
	reg("GetTradeSkillSubClassFilter", func(L *lua.LState) int {
		return filterOf(state.subclassOff, rt.tradeSkillSubClasses)(L)
	})
	reg("GetTradeSkillInvSlotFilter", func(L *lua.LState) int {
		return filterOf(state.slotOff, rt.tradeSkillSlots)(L)
	})
	setFilter := func(target func() *map[string]bool, names func() []string) lua.LGFunction {
		return func(L *lua.LState) int {
			index, on, exclusive := L.CheckInt(1), L.OptInt(2, 1) != 0, L.OptInt(3, 0) != 0
			list := names()
			off := map[string]bool{}
			switch {
			case index == 0:
				// "All" turns every category back on.
			case exclusive:
				for position, name := range list {
					if position+1 != index {
						off[name] = true
					}
				}
			default:
				for name, hidden := range *target() {
					off[name] = hidden
				}
				if index >= 1 && index <= len(list) {
					off[list[index-1]] = !on
				}
			}
			*target() = off
			rt.FireEvent("TRADE_SKILL_FILTER_UPDATE")
			return 0
		}
	}
	reg("SetTradeSkillSubClassFilter", setFilter(func() *map[string]bool { return &state.subclassOff }, rt.tradeSkillSubClasses))
	reg("SetTradeSkillInvSlotFilter", setFilter(func() *map[string]bool { return &state.slotOff }, rt.tradeSkillSlots))
}

// toInt reads a number argument the way the C API does: text that looks like
// one counts, anything else is zero.
func toInt(L *lua.LState, index int) int {
	switch value := L.Get(index).(type) {
	case lua.LNumber:
		return int(value)
	case lua.LString:
		var number int
		fmt.Sscanf(string(value), "%d", &number)
		return number
	}
	return 0
}
