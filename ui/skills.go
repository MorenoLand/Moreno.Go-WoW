package ui

import (
	"encoding/binary"
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
	// Step is the tier of the skill the character has reached.
	Step int
	// Abandonable is set for the skills the character may unlearn.
	Abandonable bool
	// Unlearned is set for a skill the character may learn but has no rank in.
	Unlearned bool
	// Flags, MinLevel and CostType come from the race and class line of
	// SkillRaceClassInfo.
	Flags     uint32
	MinLevel  int
	CostType  int
	StepCost  int
	RankCosts []int
}

// skillFlag are the bits of the flags of a skill line for a race and class.
const (
	skillFlagStepTrain = 0x08
	skillFlagLearnable = 0x04
	skillFlagSingle    = 0x400
)

// skillRow is a line of the skill window: a category header or a skill.
type skillRow struct {
	header  bool
	name    string
	entry   *SkillEntry
	visible bool
	order   uint32
	catName string
}

type skillState struct {
	entries   []SkillEntry
	collapsed map[string]bool
	selected  uint32
	// pending is how many ranks of each skill the character has put points in.
	pending      map[uint32]int
	pendingTotal int
}

// SetSkills replaces the character's skills and tells the interface.
func (rt *Runtime) SetSkills(entries []SkillEntry) {
	if rt == nil {
		return
	}
	rt.skills.entries = append(rt.skills.entries[:0], entries...)
	rt.skills.pending, rt.skills.pendingTotal = nil, 0
	rt.FireEvent("SKILL_LINES_CHANGED")
}

// skillRows lays the skills out under their category headers: the lines in view
// first, then the ones a closed category hides.
func (rt *Runtime) skillRows() []skillRow {
	state := &rt.skills
	var rows []skillRow
	seen := map[string]bool{}
	for index := range state.entries {
		entry := &state.entries[index]
		if !seen[entry.CategoryName] {
			seen[entry.CategoryName] = true
			rows = append(rows, skillRow{header: true, name: entry.CategoryName, visible: true, order: entry.CategoryOrder, catName: entry.CategoryName})
		}
		rows = append(rows, skillRow{name: entry.Name, entry: entry, visible: !state.collapsed[entry.CategoryName], order: entry.CategoryOrder, catName: entry.CategoryName})
	}
	sort.SliceStable(rows, func(a, b int) bool {
		left, right := rows[a], rows[b]
		if left.visible != right.visible {
			return left.visible
		}
		if !left.visible {
			return false
		}
		if left.order != right.order {
			return left.order < right.order
		}
		if left.catName != right.catName {
			return left.catName < right.catName
		}
		if left.header != right.header {
			return left.header
		}
		if left.entry != nil && left.entry.Unlearned != right.entry.Unlearned {
			return !left.entry.Unlearned
		}
		return strings.ToLower(left.name) < strings.ToLower(right.name)
	})
	return rows
}

// skillRowsInView is how many lines the window shows.
func skillRowsInView(rows []skillRow) int {
	count := 0
	for _, row := range rows {
		if row.visible {
			count++
		}
	}
	return count
}

// skillCharacterPoints is how many skill points the character may still spend.
func (rt *Runtime) skillCharacterPoints() int {
	return int(rt.field(skillPointsField)) - rt.skills.pendingTotal
}

// skillPointsField is PLAYER_CHARACTER_POINTS2.
const skillPointsField = 1021

// skillCurrent is the rank of a skill with the permanent bonus, and the most it
// can reach.
func skillCurrent(entry *SkillEntry) (rank, maximum int) {
	rank, maximum = entry.Rank, entry.Max
	if rank != 0 {
		rank += entry.PermBonus
	}
	if maximum != 0 {
		maximum += entry.PermBonus
	}
	return
}

// skillRankCost is what raising a skill to its next rank costs when a number of
// ranks are already waiting.
func skillRankCost(entry *SkillEntry, next int) int {
	if next < 1 || next > len(entry.RankCosts) {
		return 0
	}
	return entry.RankCosts[next-1]
}

// skillEntryAt is the skill on a line of the window, nil for a header.
func (rt *Runtime) skillEntryAt(index int) *SkillEntry {
	rows := rt.skillRows()
	if index < 1 || index > len(rows) {
		return nil
	}
	return rows[index-1].entry
}

// registerSkillAPI defines the calls the skill window makes.
func registerSkillAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetNumSkillLines", func(L *lua.LState) int {
		L.Push(lua.LNumber(skillRowsInView(rt.skillRows())))
		return 1
	})
	reg("GetSkillLineInfo", func(L *lua.LState) int {
		rows := rt.skillRows()
		index := L.CheckInt(1)
		if index < 1 || index > len(rows) {
			// Nothing is selected: the empty answer a call with no skill gives.
			for i := 0; i < 3; i++ {
				L.Push(lua.LNil)
			}
			for i := 0; i < 4; i++ {
				L.Push(lua.LNumber(0))
			}
			for i := 0; i < 3; i++ {
				L.Push(lua.LNil)
			}
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			L.Push(lua.LNil)
			return 13
		}
		row := rows[index-1]
		if row.header {
			L.Push(lua.LString(row.name))
			L.Push(lua.LNumber(1))
			if rt.skills.collapsed[row.name] {
				L.Push(lua.LNil)
			} else {
				L.Push(lua.LNumber(1))
			}
			for i := 0; i < 4; i++ {
				L.Push(lua.LNumber(0))
			}
			for i := 0; i < 3; i++ {
				L.Push(lua.LNil)
			}
			L.Push(lua.LNumber(0))
			L.Push(lua.LNumber(0))
			return 12
		}
		entry := row.entry
		pending := rt.skills.pending[entry.ID]
		rank, maximum := skillCurrent(entry)
		if entry.Flags&skillFlagSingle != 0 {
			maximum = 1
			if rank > 1 {
				rank = 1
			}
		}
		L.Push(lua.LString(entry.Name))
		L.Push(lua.LNil)
		L.Push(lua.LNil)
		L.Push(lua.LNumber(rank))
		L.Push(lua.LNumber(pending))
		L.Push(lua.LNumber(entry.TempBonus))
		L.Push(lua.LNumber(maximum))
		if entry.Abandonable {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		if (entry.Flags&skillFlagStepTrain == 0 && (entry.Step != 0 || entry.Flags&skillFlagLearnable == 0)) || entry.StepCost == 0 {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LNumber(entry.StepCost))
		}
		if cost := skillRankCost(entry, rank+1+pending); cost == 0 {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LNumber(cost))
		}
		L.Push(lua.LNumber(entry.MinLevel))
		L.Push(lua.LNumber(entry.CostType + 1))
		L.Push(lua.LString(entry.Description))
		return 13
	})
	reg("AbandonSkill", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: AbandonSkill(index)")
			return 0
		}
		if entry := rt.skillEntryAt(L.CheckInt(1)); entry != nil {
			rt.send(0x0202, binary.LittleEndian.AppendUint32(nil, entry.ID))
		}
		return 0
	})
	reg("GetSelectedSkill", func(L *lua.LState) int {
		rows := rt.skillRows()
		if rt.skills.selected != 0 {
			for index := 0; index < skillRowsInView(rows); index++ {
				if rows[index].entry != nil && rows[index].entry.ID == rt.skills.selected {
					L.Push(lua.LNumber(index + 1))
					return 1
				}
			}
		}
		L.Push(lua.LNumber(0))
		return 1
	})
	reg("SetSelectedSkill", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: SetSelectedSkill(index)")
			return 0
		}
		rt.skills.selected = 0
		if entry := rt.skillEntryAt(L.CheckInt(1)); entry != nil {
			rt.skills.selected = entry.ID
		}
		return 0
	})
	setCollapsed := func(name string, collapsed bool) lua.LGFunction {
		return func(L *lua.LState) int {
			if L.Get(1).Type() != lua.LTNumber {
				L.RaiseError("Usage: %s(index)", name)
				return 0
			}
			index := L.CheckInt(1)
			rows := rt.skillRows()
			if rt.skills.collapsed == nil {
				rt.skills.collapsed = make(map[string]bool)
			}
			if index < 1 {
				for _, row := range rows {
					if row.header {
						rt.skills.collapsed[row.name] = collapsed
					}
				}
			} else if index <= len(rows) {
				rt.skills.collapsed[rows[index-1].catName] = collapsed
			} else {
				return 0
			}
			rt.FireEvent("SKILL_LINES_CHANGED")
			return 0
		}
	}
	reg("ExpandSkillHeader", setCollapsed("ExpandSkillHeader", false))
	reg("CollapseSkillHeader", setCollapsed("CollapseSkillHeader", true))
	// Skill points: spent a rank at a time, then handed to the server together.
	reg("AddSkillUp", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: AddSkillUp(index)")
			return 0
		}
		entry := rt.skillEntryAt(L.CheckInt(1))
		if entry == nil {
			return 0
		}
		state := &rt.skills
		rank, maximum := skillCurrent(entry)
		if state.pending[entry.ID]+rank >= maximum {
			return 0
		}
		cost := skillRankCost(entry, state.pending[entry.ID]+1+rank)
		if state.pendingTotal+cost > int(rt.field(skillPointsField)) {
			return 0
		}
		if state.pending == nil {
			state.pending = make(map[uint32]int)
		}
		state.pending[entry.ID]++
		state.pendingTotal += cost
		rt.FireEvent("SKILL_LINES_CHANGED")
		return 0
	})
	reg("RemoveSkillUp", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: RemoveSkillUp(index)")
			return 0
		}
		entry := rt.skillEntryAt(L.CheckInt(1))
		state := &rt.skills
		if entry == nil || state.pending[entry.ID] == 0 {
			return 0
		}
		rank, _ := skillCurrent(entry)
		state.pendingTotal -= skillRankCost(entry, state.pending[entry.ID]+rank)
		state.pending[entry.ID]--
		rt.FireEvent("SKILL_LINES_CHANGED")
		return 0
	})
	reg("AcceptSkillUps", func(L *lua.LState) int {
		state := &rt.skills
		if state.pendingTotal == 0 {
			return 0
		}
		body := binary.LittleEndian.AppendUint32(nil, 0)
		count := uint32(0)
		for _, row := range rt.skillRows() {
			if row.entry != nil && state.pending[row.entry.ID] != 0 {
				body = binary.LittleEndian.AppendUint32(body, row.entry.ID)
				body = binary.LittleEndian.AppendUint32(body, uint32(state.pending[row.entry.ID]))
				count++
			}
		}
		binary.LittleEndian.PutUint32(body, count)
		rt.send(0x0220, body)
		state.pending, state.pendingTotal = nil, 0
		return 0
	})
	reg("CancelSkillUps", func(L *lua.LState) int {
		rt.skills.pending, rt.skills.pendingTotal = nil, 0
		rt.FireEvent("SKILL_LINES_CHANGED")
		return 0
	})
	reg("GetAdjustedSkillPoints", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.skillCharacterPoints()))
		return 1
	})
	reg("BuySkillTier", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: BuySkillTier(index)")
			return 0
		}
		entry := rt.skillEntryAt(L.CheckInt(1))
		if entry == nil {
			return 0
		}
		if entry.Flags&skillFlagStepTrain != 0 || (entry.Step == 0 && entry.Flags&skillFlagLearnable != 0) {
			if entry.StepCost != 0 && entry.StepCost <= int(rt.field(skillPointsField)) {
				rt.send(0x021F, binary.LittleEndian.AppendUint32(nil, entry.ID))
			}
		}
		return 0
	})
}
