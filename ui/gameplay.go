package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

type SpellInfo struct {
	ID, SkillLine, Attributes, Category uint32
	Name, Rank, Icon                    string
	Cost, PowerType, CastTime           int
	MinRange, MaxRange                  float64
	Recipe, Funnel, Interruptible       bool
}
type SpellSkillLine struct {
	ID, Category uint32
	Name, Icon   string
}
type spellTab struct {
	Name, Icon    string
	IDs           []uint32
	Offset, Order int
}
type spellCooldown struct {
	Start, Duration float64
	Disabled        bool
}
type gameplayState struct {
	Catalog   map[uint32]SpellInfo
	Skills    map[uint32]SpellSkillLine
	Known     map[uint32]bool
	Cooldowns map[uint32]spellCooldown
	Actions   []world.ActionButton
	Tabs      []spellTab
	Book      []uint32
	Cast      activeSpellCast
	Cursor    world.ActionButton
	Page      int
}

type WorldActionHost interface {
	SetActionButton(slot int, action world.ActionButton) error
}

type activeSpellCast struct {
	ID         uint32
	Count      uint8
	Start, End float64
	Channel    bool
}
type WorldSpellHost interface {
	CastSpell(id uint32, target uint64) error
	CancelCast(id uint32) error
	CancelChannel(id uint32) error
}

func (rt *Runtime) ResetGameplay() {
	rt.gameplay.Known = make(map[uint32]bool)
	rt.gameplay.Cooldowns = make(map[uint32]spellCooldown)
	rt.gameplay.Actions = nil
	rt.gameplay.Tabs = nil
	rt.gameplay.Book = nil
	rt.gameplay.Cast = activeSpellCast{}
	rt.gameplay.Cursor = world.ActionButton{}
	rt.gameplay.Page = 1
}

func (rt *Runtime) StartSpellCast(cast world.SpellCastHeader) {
	now := time.Since(rt.started).Seconds()
	rt.gameplay.Cast = activeSpellCast{ID: cast.SpellID, Count: cast.Count, Start: now, End: now + float64(cast.TimeMS)/1000}
	info := rt.gameplay.Catalog[cast.SpellID]
	rt.FireEvent("UNIT_SPELLCAST_START", lua.LString("player"), lua.LString(info.Name), lua.LString(info.Rank), lua.LNumber(cast.Count))
}

func (rt *Runtime) StartSpellChannel(id uint32, duration uint32) {
	now := time.Since(rt.started).Seconds()
	rt.gameplay.Cast = activeSpellCast{ID: id, Start: now, End: now + float64(duration)/1000, Channel: true}
	info := rt.gameplay.Catalog[id]
	rt.FireEvent("UNIT_SPELLCAST_CHANNEL_START", lua.LString("player"), lua.LString(info.Name), lua.LString(info.Rank))
}
func (rt *Runtime) UpdateSpellChannel(duration uint32) {
	cast := rt.gameplay.Cast
	if !cast.Channel || cast.ID == 0 {
		return
	}
	info := rt.gameplay.Catalog[cast.ID]
	event := "UNIT_SPELLCAST_CHANNEL_UPDATE"
	if duration == 0 {
		rt.gameplay.Cast = activeSpellCast{}
		event = "UNIT_SPELLCAST_CHANNEL_STOP"
	} else {
		rt.gameplay.Cast.End = time.Since(rt.started).Seconds() + float64(duration)/1000
	}
	rt.FireEvent(event, lua.LString("player"), lua.LString(info.Name), lua.LString(info.Rank))
}
func (rt *Runtime) StopSpellCast(id uint32, count uint8, failed bool) {
	if rt.gameplay.Cast.ID != id || rt.gameplay.Cast.Count != count || rt.gameplay.Cast.Channel {
		return
	}
	info := rt.gameplay.Catalog[id]
	rt.gameplay.Cast = activeSpellCast{}
	event := "UNIT_SPELLCAST_STOP"
	if failed {
		event = "UNIT_SPELLCAST_FAILED"
	}
	rt.FireEvent(event, lua.LString("player"), lua.LString(info.Name), lua.LString(info.Rank), lua.LNumber(count))
}
func (rt *Runtime) SetSpellCatalog(catalog map[uint32]SpellInfo, skills map[uint32]SpellSkillLine) {
	rt.gameplay.Catalog = catalog
	rt.gameplay.Skills = skills
	rt.rebuildSpellBook()
}
func (rt *Runtime) SetKnownSpells(spells []world.KnownSpell) {
	rt.gameplay.Known = make(map[uint32]bool, len(spells))
	for _, spell := range spells {
		if spell.ID != 0 {
			rt.gameplay.Known[spell.ID] = true
		}
	}
	rt.rebuildSpellBook()
	rt.FireEvent("SPELLS_CHANGED")
}
func (rt *Runtime) LearnSpell(id uint32, learned bool) {
	if rt.gameplay.Known == nil {
		rt.gameplay.Known = make(map[uint32]bool)
	}
	if learned {
		rt.gameplay.Known[id] = true
	} else {
		delete(rt.gameplay.Known, id)
	}
	rt.rebuildSpellBook()
	rt.FireEvent("SPELLS_CHANGED")
}
func (rt *Runtime) SetActionButtons(buttons []world.ActionButton) {
	rt.gameplay.Actions = append([]world.ActionButton(nil), buttons...)
	rt.FireEvent("ACTIONBAR_UPDATE_STATE")
	rt.FireEvent("ACTIONBAR_SLOT_CHANGED", lua.LNumber(0))
}
func (rt *Runtime) SetSpellCooldowns(cooldowns []world.SpellCooldown) {
	if rt.gameplay.Cooldowns == nil {
		rt.gameplay.Cooldowns = make(map[uint32]spellCooldown)
	}
	now := time.Since(rt.started).Seconds()
	for _, entry := range cooldowns {
		duration := entry.DurationMS
		if entry.CategoryDurationMS > duration {
			duration = entry.CategoryDurationMS
		}
		cooldown := spellCooldown{Start: now, Duration: float64(duration) / 1000, Disabled: entry.CategoryDurationMS == 0x80000000}
		rt.gameplay.Cooldowns[entry.SpellID] = cooldown
		if entry.Category != 0 {
			for id, info := range rt.gameplay.Catalog {
				if info.Category == uint32(entry.Category) && rt.gameplay.Known[id] {
					rt.gameplay.Cooldowns[id] = cooldown
				}
			}
		}
	}
	rt.FireEvent("SPELL_UPDATE_COOLDOWN")
	rt.FireEvent("ACTIONBAR_UPDATE_COOLDOWN")
}
func (rt *Runtime) rebuildSpellBook() {
	grouped := make(map[uint32][]uint32)
	for id := range rt.gameplay.Known {
		info, ok := rt.gameplay.Catalog[id]
		if !ok || info.Name == "" {
			continue
		}
		line := rt.gameplay.Skills[info.SkillLine]
		key := uint32(0)
		if line.Category == 7 || line.Category == 11 || line.Category == 9 && info.Recipe {
			key = info.SkillLine
		}
		grouped[key] = append(grouped[key], id)
	}
	rt.gameplay.Tabs = nil
	for key, ids := range grouped {
		sort.Slice(ids, func(i, j int) bool {
			a, b := rt.gameplay.Catalog[ids[i]], rt.gameplay.Catalog[ids[j]]
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return ids[i] < ids[j]
		})
		line := rt.gameplay.Skills[key]
		if key == 0 {
			line.Name = rt.L.GetGlobal("GENERAL").String()
			if line.Name == "nil" {
				line.Name = "General"
			}
			line.Icon = `Interface\Icons\INV_Misc_Book_09`
		}
		order := 0
		if key != 0 {
			switch line.Category {
			case 7:
				order = 1
			case 11:
				order = 2
			case 9:
				order = 3
			}
		}
		rt.gameplay.Tabs = append(rt.gameplay.Tabs, spellTab{Name: line.Name, Icon: line.Icon, IDs: ids, Order: order})
	}
	sort.Slice(rt.gameplay.Tabs, func(i, j int) bool {
		a, b := rt.gameplay.Tabs[i], rt.gameplay.Tabs[j]
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.Name < b.Name
	})
	rt.gameplay.Book = nil
	for i := range rt.gameplay.Tabs {
		tab := &rt.gameplay.Tabs[i]
		tab.Offset = len(rt.gameplay.Book)
		rt.gameplay.Book = append(rt.gameplay.Book, tab.IDs...)
	}
}
func (rt *Runtime) spellForCall(L *lua.LState) uint32 {
	if L.Get(1).Type() == lua.LTNumber {
		id := uint32(L.CheckInt(1))
		if L.Get(2).Type() == lua.LTString && (L.Get(2).String() == "spell" || L.Get(2).String() == "pet") {
			if L.Get(2).String() == "pet" || id < 1 || int(id) > len(rt.gameplay.Book) {
				return 0
			}
			return rt.gameplay.Book[id-1]
		}
		return id
	}
	name := L.CheckString(1)
	var found uint32
	for id := range rt.gameplay.Known {
		info := rt.gameplay.Catalog[id]
		if strings.EqualFold(info.Name, name) || strings.EqualFold(info.Name+"("+info.Rank+")", name) {
			if id > found {
				found = id
			}
		}
	}
	return found
}
func (rt *Runtime) action(slot int) world.ActionButton {
	if slot < 1 || slot > len(rt.gameplay.Actions) {
		return world.ActionButton{}
	}
	return rt.gameplay.Actions[slot-1]
}
func (rt *Runtime) pushCooldown(L *lua.LState, id uint32) int {
	cd := rt.gameplay.Cooldowns[id]
	enabled := 1
	if cd.Disabled {
		enabled = 0
		cd.Start, cd.Duration = 0, 0
	} else if cd.Start+cd.Duration <= time.Since(rt.started).Seconds() {
		cd.Start, cd.Duration = 0, 0
	}
	L.Push(lua.LNumber(cd.Start))
	L.Push(lua.LNumber(cd.Duration))
	L.Push(lua.LNumber(enabled))
	return 3
}
func (rt *Runtime) cast(id uint32, unit string) error {
	if id == 0 || !rt.gameplay.Known[id] {
		return fmt.Errorf("spell is not known")
	}
	host, ok := rt.Host.(WorldSpellHost)
	if !ok {
		return fmt.Errorf("world spell transport is unavailable")
	}
	target := uint64(0)
	if unit == "" {
		unit = "target"
	}
	if info := rt.unitInfo(unit); info != nil {
		target = info.GUID
	}
	return host.CastSpell(id, target)
}
func registerGameplayAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("SpellIsTargeting", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("SpellCanTargetItem", func(L *lua.LState) int { L.Push(lua.LFalse); return 1 })
	reg("GetSpellAutocast", func(L *lua.LState) int { L.Push(lua.LFalse); L.Push(lua.LFalse); return 2 })
	reg("GetActionBarPage", func(L *lua.LState) int {
		page := rt.gameplay.Page
		if page == 0 {
			page = 1
		}
		L.Push(lua.LNumber(page))
		return 1
	})
	reg("ChangeActionBarPage", func(L *lua.LState) int {
		page := L.CheckInt(1)
		if page >= 1 && page <= 6 && page != rt.gameplay.Page {
			rt.gameplay.Page = page
			rt.FireEvent("ACTIONBAR_PAGE_CHANGED")
		}
		return 0
	})
	reg("GetCursorInfo", func(L *lua.LState) int {
		cursor := rt.gameplay.Cursor
		if cursor.ID == 0 {
			L.Push(lua.LNil)
			return 1
		}
		switch cursor.Type {
		case 0:
			L.Push(lua.LString("spell"))
			slot := 0
			for i, id := range rt.gameplay.Book {
				if id == cursor.ID {
					slot = i + 1
					break
				}
			}
			L.Push(lua.LNumber(slot))
			L.Push(lua.LString("spell"))
			L.Push(lua.LNumber(cursor.ID))
			return 4
		case 64:
			L.Push(lua.LString("macro"))
		case 128:
			L.Push(lua.LString("item"))
		default:
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LNumber(cursor.ID))
		return 2
	})
	reg("CursorHasSpell", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.gameplay.Cursor.ID != 0 && rt.gameplay.Cursor.Type == 0))
		return 1
	})
	reg("CursorHasItem", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.gameplay.Cursor.ID != 0 && rt.gameplay.Cursor.Type == 128))
		return 1
	})
	reg("ClearCursor", func(L *lua.LState) int {
		rt.gameplay.Cursor = world.ActionButton{}
		rt.FireEvent("CURSOR_UPDATE")
		return 0
	})
	reg("PickupSpell", func(L *lua.LState) int {
		id := rt.spellForCall(L)
		if rt.gameplay.Known[id] && rt.gameplay.Catalog[id].Attributes&0x40 == 0 {
			rt.gameplay.Cursor = world.ActionButton{ID: id}
			rt.FireEvent("CURSOR_UPDATE")
		}
		return 0
	})
	reg("IsSelectedSpell", func(L *lua.LState) int {
		id := rt.spellForCall(L)
		L.Push(lua.LBool(id != 0 && rt.gameplay.Cursor.Type == 0 && rt.gameplay.Cursor.ID == id))
		return 1
	})
	for _, name := range []string{"PickupAction", "PlaceAction"} {
		name := name
		reg(name, func(L *lua.LState) int {
			slot := L.CheckInt(1)
			if slot < 1 || slot > 144 {
				return 0
			}
			cursor := rt.gameplay.Cursor
			existing := rt.action(slot)
			if name == "PlaceAction" && cursor.ID == 0 {
				return 0
			}
			host, ok := rt.Host.(WorldActionHost)
			if !ok {
				L.RaiseError("%s: world action transport is unavailable", name)
				return 0
			}
			if err := host.SetActionButton(slot-1, cursor); err != nil {
				L.RaiseError("%s: %v", name, err)
				return 0
			}
			if len(rt.gameplay.Actions) < 144 {
				rt.gameplay.Actions = append(rt.gameplay.Actions, make([]world.ActionButton, 144-len(rt.gameplay.Actions))...)
			}
			rt.gameplay.Actions[slot-1] = cursor
			rt.gameplay.Cursor = existing
			rt.FireEvent("ACTIONBAR_SLOT_CHANGED", lua.LNumber(slot))
			rt.FireEvent("CURSOR_UPDATE")
			return 0
		})
	}
	reg("HasAction", func(L *lua.LState) int { L.Push(lua.LBool(rt.action(L.CheckInt(1)).ID != 0)); return 1 })
	reg("GetActionInfo", func(L *lua.LState) int {
		action := rt.action(L.CheckInt(1))
		if action.ID == 0 {
			L.Push(lua.LNil)
			return 1
		}
		kind := map[uint8]string{0: "spell", 64: "macro", 128: "item", 32: "equipmentset"}[action.Type]
		if kind == "" {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(kind))
		L.Push(lua.LNumber(action.ID))
		L.Push(lua.LNil)
		return 3
	})
	reg("GetActionTexture", func(L *lua.LState) int {
		action := rt.action(L.CheckInt(1))
		icon := ""
		if action.Type == 0 {
			icon = rt.gameplay.Catalog[action.ID].Icon
		}
		if icon == "" {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(icon))
		}
		return 1
	})
	reg("GetActionCooldown", func(L *lua.LState) int { return rt.pushCooldown(L, rt.action(L.CheckInt(1)).ID) })
	reg("IsUsableAction", func(L *lua.LState) int {
		action := rt.action(L.CheckInt(1))
		info := rt.gameplay.Catalog[action.ID]
		player := rt.unitInfo("player")
		noPower := player != nil && player.PowerType == info.PowerType && player.Power < info.Cost
		L.Push(lua.LBool(action.ID != 0 && action.Type == 0 && rt.gameplay.Known[action.ID] && info.Attributes&0x40 == 0 && !noPower))
		L.Push(lua.LBool(noPower))
		return 2
	})
	reg("UseAction", func(L *lua.LState) int {
		action := rt.action(L.CheckInt(1))
		if action.ID == 0 {
			return 0
		}
		if action.Type != 0 {
			L.RaiseError("UseAction: action type %d is not implemented", action.Type)
			return 0
		}
		unit := ""
		if L.Get(2).Type() == lua.LTString {
			unit = L.Get(2).String()
		} else if L.Get(2) == lua.LTrue {
			unit = "player"
		}
		if err := rt.cast(action.ID, unit); err != nil {
			L.RaiseError("UseAction: %v", err)
		}
		return 0
	})
	reg("GetNumSpellTabs", func(L *lua.LState) int { L.Push(lua.LNumber(len(rt.gameplay.Tabs))); return 1 })
	reg("GetSpellTabInfo", func(L *lua.LState) int {
		index := L.CheckInt(1) - 1
		tab := spellTab{}
		if index >= 0 && index < len(rt.gameplay.Tabs) {
			tab = rt.gameplay.Tabs[index]
		}
		L.Push(lua.LString(tab.Name))
		L.Push(lua.LString(tab.Icon))
		L.Push(lua.LNumber(tab.Offset))
		L.Push(lua.LNumber(len(tab.IDs)))
		L.Push(lua.LNumber(tab.Offset))
		L.Push(lua.LNumber(len(tab.IDs)))
		return 6
	})
	reg("GetSpellName", func(L *lua.LState) int {
		info, ok := rt.gameplay.Catalog[rt.spellForCall(L)]
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(info.Name))
		L.Push(lua.LString(info.Rank))
		return 2
	})
	reg("GetSpellBookItemName", func(L *lua.LState) int {
		info, ok := rt.gameplay.Catalog[rt.spellForCall(L)]
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(info.Name))
		L.Push(lua.LString(info.Rank))
		return 2
	})
	reg("GetSpellInfo", func(L *lua.LState) int {
		info, ok := rt.gameplay.Catalog[rt.spellForCall(L)]
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(info.Name))
		L.Push(lua.LString(info.Rank))
		L.Push(lua.LString(info.Icon))
		L.Push(lua.LNumber(info.Cost))
		L.Push(lua.LBool(info.Funnel))
		L.Push(lua.LNumber(info.PowerType))
		L.Push(lua.LNumber(info.CastTime))
		L.Push(lua.LNumber(info.MinRange))
		L.Push(lua.LNumber(info.MaxRange))
		return 9
	})
	reg("GetSpellBookItemInfo", func(L *lua.LState) int {
		id := rt.spellForCall(L)
		if id == 0 {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString("SPELL"))
		L.Push(lua.LNumber(id))
		return 2
	})
	reg("GetSpellTexture", func(L *lua.LState) int {
		info, ok := rt.gameplay.Catalog[rt.spellForCall(L)]
		if !ok || info.Icon == "" {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(info.Icon))
		}
		return 1
	})
	reg("GetSpellCooldown", func(L *lua.LState) int { return rt.pushCooldown(L, rt.spellForCall(L)) })
	reg("UnitCastingInfo", func(L *lua.LState) int {
		cast := rt.gameplay.Cast
		if L.CheckString(1) != "player" || cast.ID == 0 || cast.Channel {
			L.Push(lua.LNil)
			return 1
		}
		info := rt.gameplay.Catalog[cast.ID]
		L.Push(lua.LString(info.Name))
		L.Push(lua.LString(info.Rank))
		L.Push(lua.LString(info.Name))
		L.Push(lua.LString(info.Icon))
		L.Push(lua.LNumber(cast.Start * 1000))
		L.Push(lua.LNumber(cast.End * 1000))
		L.Push(lua.LBool(info.Recipe))
		L.Push(lua.LNumber(cast.Count))
		L.Push(lua.LBool(!info.Interruptible))
		return 9
	})
	reg("UnitChannelInfo", func(L *lua.LState) int {
		cast := rt.gameplay.Cast
		if L.CheckString(1) != "player" || cast.ID == 0 || !cast.Channel {
			L.Push(lua.LNil)
			return 1
		}
		info := rt.gameplay.Catalog[cast.ID]
		L.Push(lua.LString(info.Name))
		L.Push(lua.LString(info.Rank))
		L.Push(lua.LString(info.Name))
		L.Push(lua.LString(info.Icon))
		L.Push(lua.LNumber(cast.Start * 1000))
		L.Push(lua.LNumber(cast.End * 1000))
		L.Push(lua.LBool(info.Recipe))
		L.Push(lua.LBool(!info.Interruptible))
		return 8
	})
	reg("SpellStopCasting", func(L *lua.LState) int {
		cast := rt.gameplay.Cast
		if cast.ID == 0 {
			L.Push(lua.LFalse)
			return 1
		}
		host, ok := rt.Host.(WorldSpellHost)
		if !ok {
			L.RaiseError("SpellStopCasting: world spell transport is unavailable")
			return 0
		}
		var err error
		if cast.Channel {
			err = host.CancelChannel(cast.ID)
		} else {
			err = host.CancelCast(cast.ID)
		}
		if err != nil {
			L.RaiseError("SpellStopCasting: %v", err)
			return 0
		}
		L.Push(lua.LTrue)
		return 1
	})
	reg("IsCurrentSpell", func(L *lua.LState) int {
		id := rt.spellForCall(L)
		L.Push(lua.LBool(id != 0 && rt.gameplay.Cast.ID == id))
		return 1
	})
	reg("IsCurrentAction", func(L *lua.LState) int {
		action := rt.action(L.CheckInt(1))
		L.Push(lua.LBool(action.Type == 0 && action.ID != 0 && rt.gameplay.Cast.ID == action.ID))
		return 1
	})
	reg("IsSpellKnown", func(L *lua.LState) int { L.Push(lua.LBool(rt.gameplay.Known[uint32(L.CheckInt(1))])); return 1 })
	reg("IsPlayerSpell", func(L *lua.LState) int { L.Push(lua.LBool(rt.gameplay.Known[uint32(L.CheckInt(1))])); return 1 })
	reg("IsPassiveSpell", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.gameplay.Catalog[rt.spellForCall(L)].Attributes&0x40 != 0))
		return 1
	})
	reg("GetKnownSlotFromHighestRankSlot", func(L *lua.LState) int { L.Push(lua.LNumber(L.CheckInt(1))); return 1 })
	reg("UpdateSpells", func(L *lua.LState) int { rt.rebuildSpellBook(); rt.FireEvent("SPELLS_CHANGED"); return 0 })
	reg("IsUsableSpell", func(L *lua.LState) int {
		id := rt.spellForCall(L)
		info := rt.gameplay.Catalog[id]
		player := rt.unitInfo("player")
		noPower := player != nil && player.PowerType == info.PowerType && player.Power < info.Cost
		L.Push(lua.LBool(rt.gameplay.Known[id] && info.Attributes&0x40 == 0 && !noPower))
		L.Push(lua.LBool(noPower))
		return 2
	})
	for _, name := range []string{"CastSpell", "CastSpellByName", "CastSpellByID"} {
		name := name
		reg(name, func(L *lua.LState) int {
			id := rt.spellForCall(L)
			unit := ""
			if name != "CastSpell" {
				unit = L.OptString(2, "")
			}
			if err := rt.cast(id, unit); err != nil {
				L.RaiseError("%s: %v", name, err)
			}
			return 0
		})
	}
	reg("GetSpellLink", func(L *lua.LState) int {
		id := rt.spellForCall(L)
		info, ok := rt.gameplay.Catalog[id]
		if !ok {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LString(fmt.Sprintf("|cff71d5ff|Hspell:%d|h[%s]|h|r", id, info.Name)))
		}
		return 1
	})
}
