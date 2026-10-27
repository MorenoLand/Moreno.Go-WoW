package ui

import (
	"fmt"
	"github.com/MorenoLand/Moreno.WoW/world"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// QuestTables are the client tables the rewards of a quest are worked out from.
type QuestTables struct {
	// XP is the experience of each reward column by quest level (QuestXP.dbc).
	XP map[int][10]uint32
	// FactionRep is the reputation of each column of the two rows of
	// QuestFactionReward.dbc.
	FactionRep map[uint32][10]int32
	// Titles are the names of CharTitles.dbc.
	Titles map[uint32]string
}

// SetQuestTables installs the tables the quest rewards are read from.
func (rt *Runtime) SetQuestTables(tables QuestTables) { rt.quests.tables = tables }

// questXPFor is the experience a quest gives the player: the table's value for
// the quest level, scaled by how far the quest is above or below the player,
// then rounded as the server rounds it.
func (rt *Runtime) questXPFor(questLevel int32, row uint32) uint32 {
	player := 0
	if info := rt.unitInfo("player"); info != nil {
		player = info.Level
	}
	level := int(questLevel)
	if questLevel == -1 {
		level = player
	}
	columns, ok := rt.quests.tables.XP[level]
	if !ok || row >= 10 {
		return 0
	}
	factor := 2*(level-player) + 20
	factor = min(max(factor, 1), 10)
	xp := uint32(factor) * columns[row] / 10
	switch {
	case xp <= 100:
		xp = 5 * ((xp + 2) / 5)
	case xp <= 500:
		xp = 10 * ((xp + 5) / 10)
	case xp <= 1000:
		xp = 25 * ((xp + 12) / 25)
	default:
		xp = 50 * ((xp + 25) / 50)
	}
	return xp
}

// questFactionAmount is the reputation a quest's faction value stands for: a
// column of the table's first row, or of the second when it is negative.
func (rt *Runtime) questFactionAmount(value int32) int32 {
	row, column := uint32(1), value
	if value < 0 {
		row, column = 2, -value
	}
	if column >= 10 {
		return 0
	}
	return rt.quests.tables.FactionRep[row][column]
}

// titleReward is the name of a title a quest gives, without the player's name.
func (rt *Runtime) titleReward(id uint32) (string, bool) {
	name, ok := rt.quests.tables.Titles[id]
	if id == 0 || !ok {
		return "", false
	}
	name = strings.TrimSpace(strings.ReplaceAll(name, "%s", ""))
	return strings.TrimLeft(name, ", "), name != ""
}

// grayLevel is the level under which a player gets no experience from a kill.
func grayLevel(level int) int {
	switch {
	case level <= 5:
		return 0
	case level <= 39:
		return level - 5 - level/10
	case level <= 59:
		return level - 1 - level/5
	}
	return level - 9
}

// registerQuestExtraAPI defines the quest calls beyond the windows' basics.
func registerQuestExtraAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetQuestGreenRange", func(L *lua.LState) int {
		level := 1
		if info := rt.unitInfo("player"); info != nil {
			level = info.Level
		}
		L.Push(lua.LNumber(max(level-grayLevel(level)-1, 0)))
		return 1
	})
	row := func(index int) (QuestLogSlot, bool) {
		rows := rt.questRows()
		if index < 1 || index > len(rows) || rows[index-1].slot < 0 {
			return QuestLogSlot{}, false
		}
		return rt.quests.slots[rows[index-1].slot], true
	}
	selected := func() (QuestLogSlot, bool) { return row(rt.quests.selected) }
	reg("GetQuestLink", func(L *lua.LState) int {
		slot, ok := row(L.CheckInt(1))
		if !ok {
			return 0
		}
		info := rt.quests.info[slot.ID]
		title := rt.questTitle(slot.ID)
		if title == "" {
			return 0
		}
		L.Push(lua.LString(fmt.Sprintf("|cffffff00|Hquest:%d:%d|h[%s]|h|r", slot.ID, info.Level, title)))
		return 1
	})
	reg("GetQuestSortIndex", func(L *lua.LState) int {
		rows := rt.questRows()
		for index := L.CheckInt(1) - 1; index >= 1; index-- {
			if index <= len(rows) && rows[index-1].slot < 0 {
				L.Push(lua.LNumber(index))
				return 1
			}
		}
		return 0
	})
	reg("GetQuestWatchIndex", func(L *lua.LState) int {
		want, count := L.CheckInt(1), 0
		for index, questRow := range rt.questRows() {
			if questRow.slot >= 0 && rt.quests.watched[rt.quests.slots[questRow.slot].ID] {
				count++
				if index+1 == want {
					L.Push(lua.LNumber(count))
					return 1
				}
			}
		}
		return 0
	})
	reg("GetQuestLogSpecialItemCooldown", func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		return 3
	})
	reg("GetQuestLogRewardHonor", func(L *lua.LState) int {
		slot, ok := selected()
		L.Push(lua.LNumber(max(0, int(rt.quests.info[slot.ID].RewardHonor)*boolInt(ok))))
		return 1
	})
	reg("GetQuestLogRewardArenaPoints", func(L *lua.LState) int {
		slot, ok := selected()
		L.Push(lua.LNumber(int(rt.quests.info[slot.ID].RewardArena) * boolInt(ok)))
		return 1
	})
	reg("GetQuestLogRewardTalents", func(L *lua.LState) int {
		slot, ok := selected()
		L.Push(lua.LNumber(int(rt.quests.info[slot.ID].RewardTalents) * boolInt(ok)))
		return 1
	})
	reg("GetQuestLogRewardXP", func(L *lua.LState) int {
		slot, ok := selected()
		xp := uint32(0)
		if ok {
			info := rt.quests.info[slot.ID]
			xp = rt.questXPFor(info.Level, info.XPRow)
		}
		L.Push(lua.LNumber(xp))
		return 1
	})
	reg("GetQuestLogRewardSpell", func(L *lua.LState) int {
		if slot, ok := selected(); ok {
			if spell := rt.quests.info[slot.ID].RewardSpell; spell != 0 {
				L.Push(lua.LNumber(spell))
				return 1
			}
		}
		return 0
	})
	reg("GetQuestLogRewardTitle", func(L *lua.LState) int {
		if slot, ok := selected(); ok {
			if name, known := rt.titleReward(rt.quests.info[slot.ID].RewardTitle); known {
				L.Push(lua.LString(name))
				return 1
			}
		}
		return 0
	})
	reg("GetNumQuestLogRewardFactions", func(L *lua.LState) int {
		count := 0
		if slot, ok := selected(); ok {
			for _, faction := range rt.quests.info[slot.ID].RewardFactions {
				if faction.ID != 0 {
					count++
				}
			}
		}
		L.Push(lua.LNumber(count))
		return 1
	})
	reg("GetQuestLogRewardFactionInfo", func(L *lua.LState) int {
		slot, ok := selected()
		if !ok {
			return 0
		}
		want := L.CheckInt(1)
		for _, faction := range rt.quests.info[slot.ID].RewardFactions {
			if faction.ID == 0 {
				continue
			}
			if want--; want == 0 {
				L.Push(lua.LNumber(faction.ID))
				L.Push(lua.LNumber(rt.questFactionAmount(faction.Value)))
				return 2
			}
		}
		return 0
	})
	// The dialog's rewards come with the packet: the server worked them out.
	reg("GetRewardXP", func(L *lua.LState) int { L.Push(lua.LNumber(rt.quests.dialog.rewards.XPDifficulty)); return 1 })
	reg("GetRewardHonor", func(L *lua.LState) int { L.Push(lua.LNumber(rt.quests.dialog.rewards.Honor)); return 1 })
	reg("GetRewardTitle", func(L *lua.LState) int {
		if name, ok := rt.titleReward(rt.quests.dialog.rewards.TitleID); ok {
			L.Push(lua.LString(name))
			return 1
		}
		return 0
	})
	// What the map marks of a quest (its drops and objectives) is not kept.
	reg("GetNumQuestItemDrops", func(L *lua.LState) int { L.Push(lua.LNumber(0)); return 1 })
	reg("GetQuestLogItemDrop", func(L *lua.LState) int { return 0 })
	reg("GetQuestPOILeaderBoard", func(L *lua.LState) int { return 0 })
	reg("GetDebugZoneMap", func(L *lua.LState) int { return 0 })
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// dialogQuestItemEntry is an item of the quest window: what it asks for, or
// what it rewards or lets the player choose.
func (rt *Runtime) dialogQuestItemEntry(kind string, index int) uint32 {
	dialog := &rt.quests.dialog
	var list []world.QuestItem
	switch kind {
	case "choice":
		list = dialog.rewards.Choices
	case "reward":
		list = dialog.rewards.Items
	case "required":
		list = dialog.items
	}
	if index < 1 || index > len(list) {
		return 0
	}
	return list[index-1].Entry
}

// logQuestItemEntry is an item of the quest the log has selected.
func (rt *Runtime) logQuestItemEntry(kind string, index int) uint32 {
	rows := rt.questRows()
	if rt.quests.selected < 1 || rt.quests.selected > len(rows) || rows[rt.quests.selected-1].slot < 0 {
		return 0
	}
	info := rt.quests.info[rt.quests.slots[rows[rt.quests.selected-1].slot].ID]
	var source []world.QuestItem
	switch kind {
	case "choice":
		source = info.Choices[:]
	case "reward":
		source = info.Rewards[:]
	case "required":
		source = info.Items[:]
	}
	position := 0
	for _, item := range source {
		if item.Entry == 0 {
			continue
		}
		if position++; position == index {
			return item.Entry
		}
	}
	return 0
}
