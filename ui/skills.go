package ui

import (
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// SkillEntry is one skill the character has, as its update fields and the
// client's SkillLine tables describe it.
type SkillEntry struct {
	ID          uint32
	Name        string
	Description string
	Category    uint32
	// CategoryName and CategoryOrder come from SkillLineCategory; the skill
	// window groups the lines under it in that order.
	CategoryName  string
	CategoryOrder uint32
	Rank, Max     int
	TempBonus     int
	PermBonus     int
	// Abandonable is set for the skills the character may unlearn (SkillLine
	// canLink is the table's flag for professions).
	Abandonable bool
}

// skillRow is a line of the skill window: a category header or a skill.
type skillRow struct {
	header bool
	name   string
	entry  *SkillEntry
}

type skillState struct {
	entries   []SkillEntry
	collapsed map[string]bool
	selected  int
}

// SetSkills replaces the character's skills and tells the interface.
func (rt *Runtime) SetSkills(entries []SkillEntry) {
	if rt == nil {
		return
	}
	rt.skills.entries = append(rt.skills.entries[:0], entries...)
	rt.FireEvent("SKILL_LINES_CHANGED")
}

// skillRows lays the skills out under their category headers.
func (rt *Runtime) skillRows() []skillRow {
	state := &rt.skills
	groups := make(map[string][]*SkillEntry)
	order := make(map[string]uint32)
	for index := range state.entries {
		entry := &state.entries[index]
		groups[entry.CategoryName] = append(groups[entry.CategoryName], entry)
		order[entry.CategoryName] = entry.CategoryOrder
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Slice(names, func(a, b int) bool { return order[names[a]] < order[names[b]] })
	var rows []skillRow
	for _, name := range names {
		rows = append(rows, skillRow{header: true, name: name})
		if state.collapsed[name] {
			continue
		}
		list := groups[name]
		sort.SliceStable(list, func(a, b int) bool { return strings.Compare(list[a].Name, list[b].Name) < 0 })
		for _, entry := range list {
			rows = append(rows, skillRow{name: entry.Name, entry: entry})
		}
	}
	return rows
}

// registerSkillAPI defines the calls the skill window makes.
func registerSkillAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetNumSkillLines", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.skillRows())))
		return 1
	})
	reg("GetSkillLineInfo", func(L *lua.LState) int {
		rows := rt.skillRows()
		index := L.CheckInt(1)
		if index < 1 || index > len(rows) {
			// Nothing is selected: the empty answer a call with no skill gives.
			L.Push(lua.LString(""))
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			for i := 0; i < 4; i++ {
				L.Push(lua.LNumber(0))
			}
			L.Push(lua.LNil)
			for i := 0; i < 4; i++ {
				L.Push(lua.LNil)
			}
			L.Push(lua.LString(""))
			return 13
		}
		row := rows[index-1]
		if row.header {
			L.Push(lua.LString(row.name))
			L.Push(lua.LNumber(1))
			L.Push(lua.LBool(!rt.skills.collapsed[row.name]))
			for i := 0; i < 4; i++ {
				L.Push(lua.LNumber(0))
			}
			L.Push(lua.LNil)
			for i := 0; i < 4; i++ {
				L.Push(lua.LNil)
			}
			L.Push(lua.LString(""))
			return 13
		}
		entry := row.entry
		L.Push(lua.LString(entry.Name))
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNumber(entry.Rank))
		L.Push(lua.LNumber(entry.TempBonus))
		L.Push(lua.LNumber(entry.PermBonus))
		L.Push(lua.LNumber(entry.Max))
		L.Push(lua.LBool(entry.Abandonable))
		// No cost, level or cost type: only skills the trainer sells have them.
		for i := 0; i < 4; i++ {
			L.Push(lua.LNil)
		}
		L.Push(lua.LString(entry.Description))
		return 13
	})
	reg("GetSelectedSkill", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.skills.selected))
		return 1
	})
	reg("SetSelectedSkill", func(L *lua.LState) int {
		rt.skills.selected = L.CheckInt(1)
		return 0
	})
	setCollapsed := func(L *lua.LState, collapsed bool) int {
		index := L.CheckInt(1)
		rows := rt.skillRows()
		if rt.skills.collapsed == nil {
			rt.skills.collapsed = make(map[string]bool)
		}
		if index == 0 {
			for _, row := range rows {
				if row.header {
					rt.skills.collapsed[row.name] = collapsed
				}
			}
		} else if index >= 1 && index <= len(rows) && rows[index-1].header {
			rt.skills.collapsed[rows[index-1].name] = collapsed
		}
		rt.FireEvent("SKILL_LINES_CHANGED")
		return 0
	}
	reg("ExpandSkillHeader", func(L *lua.LState) int { return setCollapsed(L, false) })
	reg("CollapseSkillHeader", func(L *lua.LState) int { return setCollapsed(L, true) })
}
