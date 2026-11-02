package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// loadBattlefieldTables reads the tables of the battleground interface: the
// battlegrounds and arenas, the level brackets, the maps and the lines the world
// state frame can show.
func loadBattlefieldTables(loader *ui.Loader) ui.BattlefieldTables {
	tables := ui.BattlefieldTables{Maps: map[uint32]ui.MapRow{}}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\BattlemasterList.dbc`); err == nil && table.fields >= 32 {
		for row := 0; row < table.records; row++ {
			entry := ui.BattlemasterRow{
				ID: table.value(row, 0), Type: table.value(row, 9), Groups: table.value(row, 10),
				Name: table.string(row, 11), MaxGroup: table.value(row, 28), Holiday: table.value(row, 29),
				MinLevel: table.value(row, 30), MaxLevel: table.value(row, 31),
			}
			for index := range entry.Maps {
				entry.Maps[index] = table.value(row, 1+index)
			}
			tables.Battlemasters = append(tables.Battlemasters, entry)
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\PvpDifficulty.dbc`); err == nil && table.fields >= 6 {
		for row := 0; row < table.records; row++ {
			tables.Difficulty = append(tables.Difficulty, ui.PvpDifficultyRow{
				Map: table.value(row, 1), Bracket: table.value(row, 2),
				MinLevel: table.value(row, 3), MaxLevel: table.value(row, 4),
			})
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\Map.dbc`); err == nil && table.fields >= 57 {
		for row := 0; row < table.records; row++ {
			tables.Maps[table.value(row, 0)] = ui.MapRow{
				Type: table.value(row, 2), Name: table.string(row, 5),
				Horde: table.string(row, 23), Alliance: table.string(row, 40),
			}
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\WorldStateUI.dbc`); err == nil && table.fields >= 63 {
		for row := 0; row < table.records; row++ {
			entry := ui.WorldStateUIRow{
				ID: table.value(row, 0), Map: table.value(row, 1), Area: table.value(row, 2), Phase: table.value(row, 3),
				Icon: table.string(row, 4), Text: table.string(row, 5), Tooltip: table.string(row, 22),
				Variable: table.value(row, 39), Type: table.value(row, 40),
				DynamicIcon: table.string(row, 41), DynamicTooltip: table.string(row, 42),
				Extended: table.string(row, 59),
			}
			for index := range entry.ExtendedVariables {
				entry.ExtendedVariables[index] = table.value(row, 60+index)
			}
			tables.WorldStates = append(tables.WorldStates, entry)
		}
	}
	return tables
}
