package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// loadLFGTables reads the tables of the dungeon finder: the dungeons and the
// headings they list under, the levels each takes for an expansion, how many
// players each map takes and the encounters of each map.
func loadLFGTables(loader *ui.Loader) ui.LFGTables {
	var tables ui.LFGTables
	if table, err := loadCharacterDBC(loader, `DBFilesClient\LFGDungeons.dbc`); err == nil && table.fields >= 49 {
		for row := 0; row < table.records; row++ {
			tables.Dungeons = append(tables.Dungeons, ui.LFGDungeonRow{
				ID: table.value(row, 0), Name: table.string(row, 1),
				MinLevel: table.value(row, 18), MaxLevel: table.value(row, 19), Target: table.value(row, 20),
				TargetMin: table.value(row, 21), TargetMax: table.value(row, 22),
				Map: table.value(row, 23), Difficulty: table.value(row, 24), Flags: table.value(row, 25),
				Type: table.value(row, 26), Faction: table.value(row, 27), Texture: table.string(row, 28),
				Expansion: table.value(row, 29), Order: table.value(row, 30), Group: table.value(row, 31),
				Description: table.string(row, 32),
			})
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\LFGDungeonGroup.dbc`); err == nil && table.fields >= 21 {
		for row := 0; row < table.records; row++ {
			tables.Groups = append(tables.Groups, ui.LFGGroupRow{
				ID: table.value(row, 0), Name: table.string(row, 1),
				Order: table.value(row, 18), Parent: table.value(row, 19), Type: table.value(row, 20),
			})
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\LFGDungeonExpansion.dbc`); err == nil && table.fields >= 8 {
		for row := 0; row < table.records; row++ {
			tables.Expansions = append(tables.Expansions, ui.LFGExpansionRow{
				Dungeon: table.value(row, 1), Expansion: table.value(row, 2),
				HardMin: table.value(row, 4), HardMax: table.value(row, 5),
				TargetMin: table.value(row, 6), TargetMax: table.value(row, 7),
			})
		}
	}
	tables.MapPlayers = make(map[[2]uint32]uint32)
	if table, err := loadCharacterDBC(loader, `DBFilesClient\MapDifficulty.dbc`); err == nil && table.fields >= 22 {
		for row := 0; row < table.records; row++ {
			key := [2]uint32{table.value(row, 1), table.value(row, 2)}
			if _, found := tables.MapPlayers[key]; !found {
				tables.MapPlayers[key] = table.value(row, 21)
			}
		}
	}
	icons := make(map[uint32]string)
	if table, err := loadCharacterDBC(loader, `DBFilesClient\SpellIcon.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			icons[table.value(row, 0)] = table.string(row, 1)
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\DungeonEncounter.dbc`); err == nil && table.fields >= 23 {
		for row := 0; row < table.records; row++ {
			tables.Encounters = append(tables.Encounters, ui.LFGEncounterRow{
				Map: table.value(row, 1), Difficulty: table.value(row, 2),
				Order: table.value(row, 3), Bit: table.value(row, 4), Name: table.string(row, 5),
				Icon: icons[table.value(row, 22)],
			})
		}
	}
	return tables
}
