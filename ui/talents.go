package ui

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// TalentDef is a talent of a specialization tree, from Talent.dbc.
type TalentDef struct {
	ID       uint32
	Row, Col int
	// RankSpells are the spells of its ranks; the first names and draws it.
	RankSpells []uint32
	// PrereqID is the talent that must have PrereqRank ranks before this one.
	PrereqID   uint32
	PrereqRank int
}

// TalentTabDef is a tree of talents, from TalentTab.dbc.
type TalentTabDef struct {
	ID         uint32
	Name       string
	Icon       string
	Background string
	Talents    []TalentDef
}

// TalentSpecState is what the character has learned in one talent group.
type TalentSpecState struct {
	Ranks map[uint32]int
}

type talentState struct {
	tabs    []TalentTabDef
	specs   []TalentSpecState
	active  int
	unspent int
	// preview holds the ranks added in the window and not yet learned.
	preview map[uint32]int
}

// rowPointsPerTier is how many points a tree must hold for each row above the
// first before that row opens.
const rowPointsPerTier = 5

// SetTalents replaces the character's talents and tells the interface.
func (rt *Runtime) SetTalents(tabs []TalentTabDef, specs []TalentSpecState, active, unspent int) {
	if rt == nil {
		return
	}
	rt.talents.tabs, rt.talents.specs, rt.talents.active, rt.talents.unspent = tabs, specs, active, unspent
	rt.talents.preview = nil
	rt.FireEvent("PLAYER_TALENT_UPDATE")
}

func (rt *Runtime) talentRank(spec int, id uint32) int {
	if spec < 0 || spec >= len(rt.talents.specs) {
		return 0
	}
	return rt.talents.specs[spec].Ranks[id]
}

func (rt *Runtime) talentRankWithPreview(spec int, id uint32) int {
	return rt.talentRank(spec, id) + rt.talents.preview[id]
}

func (rt *Runtime) talentTabPoints(spec int, tab *TalentTabDef, withPreview bool) int {
	total := 0
	for _, talent := range tab.Talents {
		if withPreview {
			total += rt.talentRankWithPreview(spec, talent.ID)
		} else {
			total += rt.talentRank(spec, talent.ID)
		}
	}
	return total
}

// talentSpec resolves the talent group an API call names (1-based, or the
// active one when it names none).
func (rt *Runtime) talentSpec(L *lua.LState, index int) int {
	if group, ok := L.Get(index).(lua.LNumber); ok && int(group) >= 1 {
		return int(group) - 1
	}
	return rt.talents.active
}

func (rt *Runtime) talentAt(tab, index int) (*TalentTabDef, *TalentDef) {
	if tab < 1 || tab > len(rt.talents.tabs) {
		return nil, nil
	}
	def := &rt.talents.tabs[tab-1]
	if index < 1 || index > len(def.Talents) {
		return def, nil
	}
	return def, &def.Talents[index-1]
}

// talentAvailable reports whether a talent's row is open and its prerequisite
// held.
func (rt *Runtime) talentAvailable(spec int, tab *TalentTabDef, talent *TalentDef) bool {
	if rt.talentTabPointsBelowRow(spec, tab, talent.Row) < talent.Row*rowPointsPerTier {
		return false
	}
	if talent.PrereqID != 0 && rt.talentRankWithPreview(spec, talent.PrereqID) < talent.PrereqRank {
		return false
	}
	return true
}

func (rt *Runtime) talentTabPointsBelowRow(spec int, tab *TalentTabDef, row int) int {
	total := 0
	for _, talent := range tab.Talents {
		if talent.Row < row {
			total += rt.talentRankWithPreview(spec, talent.ID)
		}
	}
	return total
}

func (rt *Runtime) previewPointsSpent() int {
	total := 0
	for _, count := range rt.talents.preview {
		total += count
	}
	return total
}

// registerTalentAPI defines the calls the talent window makes.
func registerTalentAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	isPet := func(L *lua.LState, index int) bool { return L.ToBool(index) }
	reg("GetNumTalentTabs", func(L *lua.LState) int {
		if isPet(L, 2) {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(len(rt.talents.tabs)))
		return 1
	})
	reg("GetNumTalents", func(L *lua.LState) int {
		tab, _ := rt.talentAt(L.CheckInt(1), 0)
		if tab == nil || isPet(L, 3) {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(len(tab.Talents)))
		return 1
	})
	reg("GetTalentTabInfo", func(L *lua.LState) int {
		tab, _ := rt.talentAt(L.CheckInt(1), 0)
		if tab == nil || isPet(L, 3) {
			return 0
		}
		spec := rt.talentSpec(L, 4)
		L.Push(lua.LString(tab.Name))
		L.Push(lua.LString(tab.Icon))
		L.Push(lua.LNumber(rt.talentTabPoints(spec, tab, false)))
		L.Push(lua.LString(tab.Background))
		L.Push(lua.LNumber(rt.talentTabPoints(spec, tab, true) - rt.talentTabPoints(spec, tab, false)))
		return 5
	})
	reg("GetTalentInfo", func(L *lua.LState) int {
		tab, talent := rt.talentAt(L.CheckInt(1), L.CheckInt(2))
		if talent == nil || isPet(L, 4) {
			return 0
		}
		spec := rt.talentSpec(L, 5)
		name, icon := "", ""
		if len(talent.RankSpells) > 0 {
			info := rt.gameplay.Catalog[talent.RankSpells[0]]
			name, icon = info.Name, info.Icon
		}
		available := rt.talentAvailable(spec, tab, talent)
		L.Push(lua.LString(name))
		L.Push(lua.LString(icon))
		L.Push(lua.LNumber(talent.Row + 1))
		L.Push(lua.LNumber(talent.Col + 1))
		L.Push(lua.LNumber(rt.talentRankWithPreview(spec, talent.ID)))
		L.Push(lua.LNumber(len(talent.RankSpells)))
		L.Push(lua.LNil)
		if available {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 8
	})
	reg("GetTalentPrereqs", func(L *lua.LState) int {
		tab, talent := rt.talentAt(L.CheckInt(1), L.CheckInt(2))
		if talent == nil || talent.PrereqID == 0 || isPet(L, 4) {
			return 0
		}
		spec := rt.talentSpec(L, 5)
		for index := range tab.Talents {
			if prereq := &tab.Talents[index]; prereq.ID == talent.PrereqID {
				L.Push(lua.LNumber(prereq.Row + 1))
				L.Push(lua.LNumber(prereq.Col + 1))
				if rt.talentRankWithPreview(spec, prereq.ID) >= talent.PrereqRank {
					L.Push(lua.LNumber(1))
				} else {
					L.Push(lua.LNil)
				}
				return 3
			}
		}
		return 0
	})
	reg("GetUnspentTalentPoints", func(L *lua.LState) int {
		if isPet(L, 2) {
			L.Push(lua.LNumber(0))
			return 1
		}
		L.Push(lua.LNumber(rt.talents.unspent))
		return 1
	})
	reg("GetGroupPreviewTalentPointsSpent", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.previewPointsSpent()))
		return 1
	})
	reg("GetNumTalentGroups", func(L *lua.LState) int {
		groups := len(rt.talents.specs)
		if groups < 1 {
			groups = 1
		}
		L.Push(lua.LNumber(groups))
		return 1
	})
	reg("GetActiveTalentGroup", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.talents.active + 1))
		return 1
	})
	reg("SetActiveTalentGroup", func(L *lua.LState) int { return 0 })
	reg("UnitCharacterPoints", func(L *lua.LState) int {
		L.Push(lua.LNumber(rt.talents.unspent))
		L.Push(lua.LNumber(0))
		return 2
	})
	reg("AddPreviewTalentPoints", func(L *lua.LState) int {
		tab, talent := rt.talentAt(L.CheckInt(1), L.CheckInt(2))
		if talent == nil || isPet(L, 4) {
			return 0
		}
		spec := rt.talentSpec(L, 5)
		delta := L.CheckInt(3)
		if rt.talents.preview == nil {
			rt.talents.preview = make(map[uint32]int)
		}
		rank := rt.talentRankWithPreview(spec, talent.ID)
		switch {
		case delta > 0:
			if rank < len(talent.RankSpells) && rt.previewPointsSpent() < rt.talents.unspent && rt.talentAvailable(spec, tab, talent) {
				rt.talents.preview[talent.ID]++
			}
		case delta < 0:
			if rt.talents.preview[talent.ID] > 0 {
				rt.talents.preview[talent.ID]--
				if rt.talents.preview[talent.ID] == 0 {
					delete(rt.talents.preview, talent.ID)
				}
			}
		}
		rt.FireEvent("PREVIEW_TALENT_POINTS_CHANGED", lua.LNumber(L.CheckInt(1)), lua.LNumber(L.CheckInt(2)), lua.LNumber(rt.previewPointsSpent()))
		return 0
	})
	reg("ResetGroupPreviewTalentPoints", func(L *lua.LState) int {
		rt.talents.preview = nil
		rt.FireEvent("PREVIEW_TALENT_POINTS_CHANGED", lua.LNumber(0), lua.LNumber(0), lua.LNumber(0))
		return 0
	})
	learn := func(talent *TalentDef, rank int) {
		if host, ok := rt.Host.(interface{ LearnTalent(id, rank uint32) error }); ok {
			_ = host.LearnTalent(talent.ID, uint32(rank))
		}
	}
	reg("LearnTalent", func(L *lua.LState) int {
		tab, talent := rt.talentAt(L.CheckInt(1), L.CheckInt(2))
		if talent != nil && !isPet(L, 3) && rt.talentAvailable(rt.talents.active, tab, talent) {
			learn(talent, rt.talentRank(rt.talents.active, talent.ID))
		}
		return 0
	})
	reg("LearnPreviewTalents", func(L *lua.LState) int {
		for _, tab := range rt.talents.tabs {
			for index := range tab.Talents {
				talent := &tab.Talents[index]
				for step := 0; step < rt.talents.preview[talent.ID]; step++ {
					learn(talent, rt.talentRank(rt.talents.active, talent.ID)+step)
				}
			}
		}
		rt.talents.preview = nil
		return 0
	})
	reg("GetTalentLink", func(L *lua.LState) int {
		_, talent := rt.talentAt(L.CheckInt(1), L.CheckInt(2))
		if talent == nil || len(talent.RankSpells) == 0 {
			return 0
		}
		spec := rt.talentSpec(L, 5)
		name := rt.gameplay.Catalog[talent.RankSpells[0]].Name
		L.Push(lua.LString(fmt.Sprintf("|cff4e96f7|Htalent:%d:%d|h[%s]|h|r", talent.ID, rt.talentRank(spec, talent.ID)-1, name)))
		return 1
	})
}
