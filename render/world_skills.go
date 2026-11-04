package render

import (
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// PLAYER_SKILL_INFO_1_1 holds 128 skills of three fields each: the skill id,
// its rank and maximum, and its temporary and permanent bonuses.
const (
	fieldSkillInfo  = 636
	skillSlotCount  = 128
	skillFieldCount = 3
)

type worldSkillTables struct {
	loaded     bool
	lines      map[uint32]worldSkillLine
	categories map[uint32]worldSkillCategory
	// races holds the lines of SkillRaceClassInfo of each skill, in table order.
	races map[uint32][]worldSkillRace
	// tiers is the cost of each step of a tier, costs the ranks of a group.
	tiers map[uint32][16]uint32
	costs map[uint32][][3]uint32
}

type worldSkillLine struct {
	category    uint32
	costsID     uint32
	name        string
	description string
	canLink     bool
}

// worldSkillRace is what a skill is for the races and classes in its masks.
type worldSkillRace struct {
	raceMask, classMask uint32
	flags, minLevel     uint32
	tier, costIndex     uint32
}

type worldSkillCategory struct {
	name  string
	order uint32
}

// load reads SkillLine and SkillLineCategory.
func (tables *worldSkillTables) load(loader *ui.Loader) {
	if tables.loaded {
		return
	}
	tables.loaded = true
	tables.lines = make(map[uint32]worldSkillLine)
	tables.categories = make(map[uint32]worldSkillCategory)
	if categories, err := loadCharacterDBC(loader, `DBFilesClient\SkillLineCategory.dbc`); err == nil && categories.fields >= 19 {
		for row := 0; row < categories.records; row++ {
			tables.categories[categories.value(row, 0)] = worldSkillCategory{name: categories.string(row, 1), order: categories.value(row, 18)}
		}
	}
	if lines, err := loadCharacterDBC(loader, `DBFilesClient\SkillLine.dbc`); err == nil && lines.fields >= 56 {
		for row := 0; row < lines.records; row++ {
			tables.lines[lines.value(row, 0)] = worldSkillLine{
				category: lines.value(row, 1), costsID: lines.value(row, 2), name: lines.string(row, 3), description: lines.string(row, 20), canLink: lines.value(row, 55) != 0,
			}
		}
	}
	tables.races = make(map[uint32][]worldSkillRace)
	if races, err := loadCharacterDBC(loader, `DBFilesClient\SkillRaceClassInfo.dbc`); err == nil && races.fields >= 8 {
		for row := 0; row < races.records; row++ {
			skill := races.value(row, 1)
			tables.races[skill] = append(tables.races[skill], worldSkillRace{raceMask: races.value(row, 2), classMask: races.value(row, 3),
				flags: races.value(row, 4), minLevel: races.value(row, 5), tier: races.value(row, 6), costIndex: races.value(row, 7)})
		}
	}
	tables.tiers = make(map[uint32][16]uint32)
	if tiers, err := loadCharacterDBC(loader, `DBFilesClient\SkillTiers.dbc`); err == nil && tiers.fields >= 17 {
		for row := 0; row < tiers.records; row++ {
			var cost [16]uint32
			for step := range cost {
				cost[step] = tiers.value(row, 1+step)
			}
			tables.tiers[tiers.value(row, 0)] = cost
		}
	}
	tables.costs = make(map[uint32][][3]uint32)
	if costs, err := loadCharacterDBC(loader, `DBFilesClient\SkillCostsData.dbc`); err == nil && costs.fields >= 5 {
		for row := 0; row < costs.records; row++ {
			group := costs.value(row, 1)
			tables.costs[group] = append(tables.costs[group], [3]uint32{costs.value(row, 2), costs.value(row, 3), costs.value(row, 4)})
		}
	}
}

// raceFor is the line of a skill that fits a race and a class.
func (tables *worldSkillTables) raceFor(skill, race, class uint32) (worldSkillRace, bool) {
	for _, line := range tables.races[skill] {
		if (line.raceMask == 0 || line.raceMask&(1<<(race-1)) != 0) && (line.classMask == 0 || line.classMask&(1<<(class-1)) != 0) {
			return line, true
		}
	}
	return worldSkillRace{}, false
}

// syncWorldSkills hands the interface the skills in the player's fields.
func syncWorldSkills(rt *ui.Runtime, loader *ui.Loader, tables *worldSkillTables, player *worldEntity, last *[skillSlotCount * skillFieldCount]uint32) {
	changed := false
	var current [skillSlotCount * skillFieldCount]uint32
	for index := range current {
		current[index] = player.fields[uint16(fieldSkillInfo+index)]
		if current[index] != last[index] {
			changed = true
		}
	}
	if !changed {
		return
	}
	*last = current
	tables.load(loader)
	values := make(map[uint32][4]int)
	var entries []ui.SkillEntry
	for slot := 0; slot < skillSlotCount; slot++ {
		id := current[slot*skillFieldCount] & 0xFFFF
		if id == 0 {
			continue
		}
		value, bonus := current[slot*skillFieldCount+1], current[slot*skillFieldCount+2]
		values[id] = [4]int{int(value & 0xFFFF), int(value >> 16), int(int16(bonus & 0xFFFF)), int(int16(bonus >> 16))}
		line, ok := tables.lines[id]
		category, known := tables.categories[line.category]
		if !ok || !known {
			continue
		}
		bytes0 := player.fields[world.UnitBytes0Field]
		race, ok := tables.raceFor(id, bytes0&0xFF, bytes0>>8&0xFF)
		if !ok || race.flags&2 != 0 {
			continue
		}
		rank := int(value & 0xFFFF)
		if race.flags&1 == 0 && rank == 0 && (race.flags&4 == 0 || race.minLevel > player.fields[unitLevelField]) {
			continue
		}
		step := int(current[slot*skillFieldCount] >> 16)
		entry := ui.SkillEntry{
			ID: id, Name: line.name, Description: line.description, Category: line.category,
			CategoryName: category.name, CategoryOrder: category.order,
			Rank: rank, Max: int(value >> 16),
			TempBonus: int(int16(bonus & 0xFFFF)), PermBonus: int(int16(bonus >> 16)),
			Step: step, Unlearned: rank == 0, Flags: race.flags, MinLevel: int(race.minLevel), CostType: int(race.costIndex),
			Abandonable: step != 0 && race.flags&0x20 != 0,
		}
		if costs, ok := tables.tiers[race.tier]; ok && step < len(costs) {
			entry.StepCost = int(costs[step])
		}
		for _, row := range tables.costs[line.costsID] {
			entry.RankCosts = append(entry.RankCosts, int(row[race.costIndex%3]))
		}
		entries = append(entries, entry)
	}
	rt.SetSkillValues(values)
	rt.SetSkills(entries)
}
