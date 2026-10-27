package render

import (
	"github.com/MorenoLand/Moreno.WoW/ui"
)

// PLAYER_SKILL_INFO_1_1 holds 128 skills of three fields each: the skill id,
// its rank and maximum, and its temporary and permanent bonuses.
const (
	fieldSkillInfo  = 636
	skillSlotCount  = 128
	skillFieldCount = 3
	// SkillLineCategory 12 is "Not Displayed".
	skillCategoryHidden = 12
)

type worldSkillTables struct {
	loaded     bool
	lines      map[uint32]worldSkillLine
	categories map[uint32]worldSkillCategory
}

type worldSkillLine struct {
	category    uint32
	name        string
	description string
	canLink     bool
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
				category: lines.value(row, 1), name: lines.string(row, 3), description: lines.string(row, 20), canLink: lines.value(row, 55) != 0,
			}
		}
	}
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
		if !ok || line.category == skillCategoryHidden {
			continue
		}
		category := tables.categories[line.category]
		entries = append(entries, ui.SkillEntry{
			ID: id, Name: line.name, Description: line.description, Category: line.category,
			CategoryName: category.name, CategoryOrder: category.order,
			Rank: int(value & 0xFFFF), Max: int(value >> 16),
			TempBonus: int(int16(bonus & 0xFFFF)), PermBonus: int(int16(bonus >> 16)),
			Abandonable: line.canLink,
		})
	}
	rt.SetSkillValues(values)
	rt.SetSkills(entries)
}
