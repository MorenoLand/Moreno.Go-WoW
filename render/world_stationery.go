package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// loadStationery reads the stationeries of Stationery.dbc.
func loadStationery(loader *ui.Loader) []ui.StationeryRow {
	table, err := loadCharacterDBC(loader, `DBFilesClient\Stationery.dbc`)
	if err != nil || table.fields < 3 {
		return nil
	}
	rows := make([]ui.StationeryRow, 0, table.records)
	for row := 0; row < table.records; row++ {
		rows = append(rows, ui.StationeryRow{ID: table.value(row, 0), Item: table.value(row, 1), Texture: table.string(row, 2)})
	}
	return rows
}
