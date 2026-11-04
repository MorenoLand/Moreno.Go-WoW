package ui

import (
	"sort"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/world"
	lua "github.com/yuin/gopher-lua"
)

// TrainerHost is what training asks of the connection.
type TrainerHost interface {
	TrainSpell(trainer uint64, spell uint32) error
}

type trainerState struct {
	open     bool
	trainer  world.Trainer
	selected int
	// filter shows each kind of line: available, unavailable, used.
	filter map[string]bool
	// collapsed are the skill lines whose spells are hidden.
	collapsed map[uint32]bool
}

func (rt *Runtime) trainerHost() TrainerHost {
	host, _ := rt.Host.(TrainerHost)
	return host
}

// ShowTrainer opens a trainer's list.
func (rt *Runtime) ShowTrainer(trainer world.Trainer) {
	rt.CloseGossipWindow()
	rt.trainer = trainerState{open: true, trainer: trainer, selected: 0, filter: map[string]bool{"available": true, "unavailable": true, "used": false}}
	rt.FireEvent("TRAINER_SHOW")
}

// UpdateTrainerSpell changes a line of the list after the server taught a spell.
func (rt *Runtime) UpdateTrainerSpell(spell uint32, state uint8) {
	for index := range rt.trainer.trainer.Spells {
		if rt.trainer.trainer.Spells[index].ID == spell {
			rt.trainer.trainer.Spells[index].State = state
		}
	}
	rt.FireEvent("TRAINER_UPDATE")
}

// CloseTrainerWindow closes the trainer's list.
func (rt *Runtime) CloseTrainerWindow() {
	if !rt.trainer.open {
		return
	}
	rt.trainer.open = false
	rt.FireEvent("TRAINER_CLOSED")
}

func trainerCategory(state uint8) string {
	switch state {
	case 0:
		return "available"
	case 2:
		return "used"
	}
	return "unavailable"
}

// trainerRow is a line of the list: a skill line heading or a spell.
type trainerRow struct {
	header bool
	line   uint32
	spell  world.TrainerSpell
}

// trainerLineOf is the skill line a spell is listed under: the line of the
// spell it teaches when it only teaches another, or its own.
func (rt *Runtime) trainerLineOf(id uint32) uint32 {
	spell := rt.gameplay.Catalog[id]
	for index, effect := range spell.Effects {
		if effect.Effect == 36 && effect.TargetA != 5 && spell.Created[index] != 0 {
			return rt.gameplay.Catalog[spell.Created[index]].SkillLine
		}
	}
	return spell.SkillLine
}

// trainerRows are the lines the list shows: the skill lines that have a spell
// the filter lets through, and under each that is not collapsed its spells by
// the level they need, then by name.
func (rt *Runtime) trainerRows() []trainerRow {
	state := &rt.trainer
	if state.trainer.Type == 2 {
		var rows []trainerRow
		for _, spell := range state.trainer.Spells {
			if state.filter[trainerCategory(spell.State)] {
				rows = append(rows, trainerRow{spell: spell})
			}
		}
		return rows
	}
	type group struct {
		line   uint32
		dotted bool
		spells []world.TrainerSpell
	}
	var groups []*group
	byLine := map[uint32]*group{}
	for _, spell := range state.trainer.Spells {
		line := rt.trainerLineOf(spell.ID)
		if line == 0 {
			continue
		}
		g := byLine[line]
		if g == nil {
			g = &group{line: line}
			byLine[line] = g
			groups = append(groups, g)
		}
		if spell.ProfessionDot[0] != 0 || spell.ProfessionDot[1] != 0 {
			g.dotted = true
		}
		if state.filter[trainerCategory(spell.State)] {
			g.spells = append(g.spells, spell)
		}
	}
	name := func(g *group) string { return rt.gameplay.Skills[g.line].Name }
	sort.SliceStable(groups, func(a, b int) bool {
		if groups[a].dotted != groups[b].dotted {
			return !groups[a].dotted
		}
		return strings.Compare(name(groups[a]), name(groups[b])) < 0
	})
	var rows []trainerRow
	for _, g := range groups {
		if len(g.spells) == 0 {
			continue
		}
		rows = append(rows, trainerRow{header: true, line: g.line})
		if state.collapsed[g.line] {
			continue
		}
		sort.SliceStable(g.spells, func(a, b int) bool {
			x, y := g.spells[a], g.spells[b]
			if x.RequiredLevel != y.RequiredLevel {
				return x.RequiredLevel < y.RequiredLevel
			}
			if x.RequiredRank != y.RequiredRank {
				return x.RequiredRank < y.RequiredRank
			}
			return strings.Compare(rt.gameplay.Catalog[x.ID].Name, rt.gameplay.Catalog[y.ID].Name) < 0
		})
		for _, spell := range g.spells {
			rows = append(rows, trainerRow{line: g.line, spell: spell})
		}
	}
	return rows
}

// registerTrainerAPI defines the calls the trainer window makes.
func registerTrainerAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	rowAt := func(index int) (world.TrainerSpell, bool) {
		rows := rt.trainerRows()
		if index < 1 || index > len(rows) || rows[index-1].header {
			return world.TrainerSpell{}, false
		}
		return rows[index-1].spell, true
	}
	reg("GetNumTrainerServices", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.trainerRows())))
		return 1
	})
	reg("GetTrainerServiceInfo", func(L *lua.LState) int {
		if rows := rt.trainerRows(); L.CheckInt(1) >= 1 && L.CheckInt(1) <= len(rows) && rows[L.CheckInt(1)-1].header {
			row := rows[L.CheckInt(1)-1]
			L.Push(lua.LString(rt.gameplay.Skills[row.line].Name))
			L.Push(lua.LString(""))
			L.Push(lua.LString("header"))
			if rt.trainer.collapsed[row.line] {
				L.Push(lua.LNil)
			} else {
				L.Push(lua.LTrue)
			}
			return 4
		}
		spell, ok := rowAt(L.CheckInt(1))
		if !ok {
			return 0
		}
		info := rt.gameplay.Catalog[spell.ID]
		L.Push(lua.LString(info.Name))
		L.Push(lua.LString(info.Rank))
		L.Push(lua.LString(trainerCategory(spell.State)))
		L.Push(lua.LTrue)
		return 4
	})
	reg("GetTrainerServiceCost", func(L *lua.LState) int {
		spell, ok := rowAt(L.CheckInt(1))
		if !ok {
			return 0
		}
		L.Push(lua.LNumber(spell.Cost))
		L.Push(lua.LNumber(spell.ProfessionDot[0]))
		L.Push(lua.LNumber(spell.ProfessionDot[1]))
		return 3
	})
	reg("GetTrainerServiceDescription", func(L *lua.LState) int {
		spell, ok := rowAt(L.CheckInt(1))
		if !ok {
			return 0
		}
		info := rt.gameplay.Catalog[spell.ID]
		L.Push(lua.LString(rt.spellTextContext().expand(info.Description, info)))
		return 1
	})
	reg("GetTrainerServiceIcon", func(L *lua.LState) int {
		spell, ok := rowAt(L.CheckInt(1))
		if !ok {
			return 0
		}
		L.Push(lua.LString(rt.gameplay.Catalog[spell.ID].Icon))
		return 1
	})
	reg("GetTrainerServiceLevelReq", func(L *lua.LState) int {
		spell, ok := rowAt(L.CheckInt(1))
		if !ok {
			return 0
		}
		L.Push(lua.LNumber(spell.RequiredLevel))
		return 1
	})
	reg("GetTrainerServiceSkillReq", func(L *lua.LState) int {
		spell, ok := rowAt(L.CheckInt(1))
		if !ok || spell.RequiredSkill == 0 {
			return 0
		}
		L.Push(lua.LString(rt.gameplay.Skills[spell.RequiredSkill].Name))
		L.Push(lua.LNumber(spell.RequiredRank))
		L.Push(lua.LTrue)
		return 3
	})
	reg("GetTrainerServiceSkillLine", func(L *lua.LState) int {
		if spell, ok := rowAt(L.CheckInt(1)); ok {
			L.Push(lua.LString(rt.gameplay.Skills[spell.RequiredSkill].Name))
			return 1
		}
		return 0
	})
	reg("GetTrainerServiceNumAbilityReq", func(L *lua.LState) int {
		spell, ok := rowAt(L.CheckInt(1))
		count := 0
		if ok {
			for _, chain := range spell.Chain {
				if chain != 0 {
					count++
				}
			}
		}
		L.Push(lua.LNumber(count))
		return 1
	})
	reg("GetTrainerServiceAbilityReq", func(L *lua.LState) int {
		spell, ok := rowAt(L.CheckInt(1))
		if !ok {
			return 0
		}
		want := L.CheckInt(2)
		count := 0
		for _, chain := range spell.Chain {
			if chain == 0 {
				continue
			}
			count++
			if count == want {
				L.Push(lua.LString(rt.gameplay.Catalog[chain].Name))
				L.Push(lua.LBool(rt.gameplay.Known[chain]))
				return 2
			}
		}
		return 0
	})
	setCollapsed := func(name string, collapse bool) {
		reg(name, func(L *lua.LState) int {
			if L.Get(1).Type() != lua.LTNumber {
				L.RaiseError("Usage: %sTrainerSkillLine(index)", map[bool]string{true: "Collapse", false: "Expand"}[collapse])
				return 0
			}
			index := L.CheckInt(1)
			rows := rt.trainerRows()
			if index < 1 {
				rt.trainer.collapsed = map[uint32]bool{}
				if collapse {
					for _, row := range rows {
						if row.header {
							rt.trainer.collapsed[row.line] = true
						}
					}
				}
			} else if index > len(rows) || !rows[index-1].header {
				L.RaiseError("Bad skill line in %sTrainerSkillLine", map[bool]string{true: "Collapse", false: "Expand"}[collapse])
				return 0
			} else {
				if rt.trainer.collapsed == nil {
					rt.trainer.collapsed = map[uint32]bool{}
				}
				rt.trainer.collapsed[rows[index-1].line] = collapse
			}
			rt.FireEvent("TRAINER_UPDATE")
			return 0
		})
	}
	setCollapsed("ExpandTrainerSkillLine", false)
	setCollapsed("CollapseTrainerSkillLine", true)
	reg("SelectTrainerService", func(L *lua.LState) int {
		rt.trainer.selected = L.CheckInt(1)
		rt.FireEvent("TRAINER_UPDATE")
		return 0
	})
	reg("GetTrainerSelectionIndex", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.trainer.selected))
		return 1
	})
	reg("BuyTrainerService", func(L *lua.LState) int {
		if spell, ok := rowAt(L.CheckInt(1)); ok {
			if host := rt.trainerHost(); host != nil {
				_ = host.TrainSpell(rt.trainer.trainer.GUID, spell.ID)
			}
		}
		return 0
	})
	reg("CloseTrainer", func(L *lua.LState) int {
		rt.CloseTrainerWindow()
		return 0
	})
	reg("IsTradeskillTrainer", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.trainer.trainer.Type == 2))
		return 1
	})
	reg("SetTrainerServiceTypeFilter", func(L *lua.LState) int {
		if rt.trainer.filter == nil {
			rt.trainer.filter = map[string]bool{}
		}
		rt.trainer.filter[L.CheckString(1)] = L.OptInt(2, 1) != 0
		rt.FireEvent("TRAINER_UPDATE")
		return 0
	})
	reg("GetTrainerServiceTypeFilter", func(L *lua.LState) int {
		L.Push(lua.LBool(rt.trainer.filter[L.CheckString(1)]))
		return 1
	})
	reg("CheckTalentMasterDist", func(L *lua.LState) int { L.Push(lua.LTrue); return 1 })
}
