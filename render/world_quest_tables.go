package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// loadQuestTables reads the tables a quest's rewards are worked out from.
func loadQuestTables(loader *ui.Loader) ui.QuestTables {
	tables := ui.QuestTables{XP: map[int][10]uint32{}, FactionRep: map[uint32][10]int32{}, Titles: map[uint32]string{}}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\QuestXP.dbc`); err == nil && table.fields >= 11 {
		for row := 0; row < table.records; row++ {
			var columns [10]uint32
			for column := range columns {
				columns[column] = table.value(row, column+1)
			}
			tables.XP[int(table.value(row, 0))] = columns
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\QuestFactionReward.dbc`); err == nil && table.fields >= 11 {
		for row := 0; row < table.records; row++ {
			var columns [10]int32
			for column := range columns {
				columns[column] = int32(table.value(row, column+1))
			}
			tables.FactionRep[table.value(row, 0)] = columns
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\CharTitles.dbc`); err == nil && table.fields >= 3 {
		for row := 0; row < table.records; row++ {
			tables.Titles[table.value(row, 0)] = table.string(row, 2)
		}
	}
	return tables
}

// loadToolNames reads the names of the tools recipes ask for.
func loadToolNames(loader *ui.Loader) map[uint32]string {
	names := map[uint32]string{}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\TotemCategory.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			names[table.value(row, 0)] = table.string(row, 1)
		}
	}
	return names
}
