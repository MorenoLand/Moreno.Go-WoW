package ui

import (
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

// trainerRows are the lines the filter lets through.
func (rt *Runtime) trainerRows() []world.TrainerSpell {
	var rows []world.TrainerSpell
	for _, spell := range rt.trainer.trainer.Spells {
		if rt.trainer.filter[trainerCategory(spell.State)] {
			rows = append(rows, spell)
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
		if index < 1 || index > len(rows) {
			return world.TrainerSpell{}, false
		}
		return rows[index-1], true
	}
	reg("GetNumTrainerServices", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.trainerRows())))
		return 1
	})
	reg("GetTrainerServiceInfo", func(L *lua.LState) int {
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
