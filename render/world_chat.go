package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// loadChatChannelRows reads ChatChannels.dbc: each channel's flags, the name
// with its place for the zone, and the short name commands use.
func loadChatChannelRows(loader *ui.Loader) []ui.ChatChannelRow {
	table, err := loadCharacterDBC(loader, `DBFilesClient\ChatChannels.dbc`)
	if err != nil || table.fields < 21 {
		return nil
	}
	rows := make([]ui.ChatChannelRow, 0, table.records)
	for row := 0; row < table.records; row++ {
		rows = append(rows, ui.ChatChannelRow{
			ID:       table.value(row, 0),
			Flags:    table.value(row, 1),
			Pattern:  table.string(row, 3),
			Shortcut: table.string(row, 20),
		})
	}
	return rows
}

// loadEmoteTables reads EmotesText.dbc and EmotesTextData.dbc.
func loadEmoteTables(loader *ui.Loader) ([]ui.EmoteRow, map[uint32]string) {
	var rows []ui.EmoteRow
	if table, err := loadCharacterDBC(loader, `DBFilesClient\EmotesText.dbc`); err == nil && table.fields >= 19 {
		for row := 0; row < table.records; row++ {
			entry := ui.EmoteRow{ID: table.value(row, 0), Token: table.string(row, 1), Emote: table.value(row, 2)}
			for index := range entry.Texts {
				entry.Texts[index] = table.value(row, 3+index)
			}
			rows = append(rows, entry)
		}
	}
	texts := make(map[uint32]string)
	if table, err := loadCharacterDBC(loader, `DBFilesClient\EmotesTextData.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			texts[table.value(row, 0)] = table.string(row, 1)
		}
	}
	return rows, texts
}

// loadServerMessages reads ServerMessages.dbc: the text of each kind of server
// announcement.
func loadServerMessages(loader *ui.Loader) map[uint32]string {
	texts := make(map[uint32]string)
	if table, err := loadCharacterDBC(loader, `DBFilesClient\ServerMessages.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			texts[table.value(row, 0)] = table.string(row, 1)
		}
	}
	return texts
}

// loadWhoTables reads the zones, races and classes a who query names.
func loadWhoTables(loader *ui.Loader) ui.WhoTables {
	tables := ui.WhoTables{Areas: loadAreaNames(loader)}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\AreaTable.dbc`); err == nil && table.fields >= 12 {
		for row := 0; row < table.records; row++ {
			if table.value(row, 2) == 0 {
				tables.Zones = append(tables.Zones, ui.WhoZone{ID: table.value(row, 0), Name: table.string(row, 11)})
			}
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\ChrRaces.dbc`); err == nil && table.fields >= 49 {
		for row := 0; row < table.records; row++ {
			tables.Races = append(tables.Races, ui.WhoRace{ID: table.value(row, 0), Name: table.string(row, 14), Female: table.string(row, 31), Male: table.string(row, 48), SicknessSpell: table.value(row, 9)})
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\ChrClasses.dbc`); err == nil && table.fields >= 56 {
		for row := 0; row < table.records; row++ {
			tables.Classes = append(tables.Classes, ui.WhoClass{ID: table.value(row, 0), Name: table.string(row, 4), Female: table.string(row, 21), Male: table.string(row, 38), File: table.string(row, 55)})
		}
	}
	return tables
}
