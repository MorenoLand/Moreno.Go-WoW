package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// unitCharmedByField is where a unit's UNIT_FIELD_CHARMEDBY sits in its update
// fields (OBJECT_END + 12, two words).
const unitCharmedByField = 6 + 12

// loadVehicleTables reads the tables of the vehicle interface.
func loadVehicleTables(loader *ui.Loader) ui.VehicleTables {
	tables := ui.VehicleTables{Indicators: map[uint32]string{}}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\Vehicle.dbc`); err == nil && table.fields >= 14 {
		for row := 0; row < table.records; row++ {
			entry := ui.VehicleRow{ID: table.value(row, 0), Flags: table.value(row, 1)}
			for seat := range entry.Seats {
				entry.Seats[seat] = table.value(row, 6+seat)
			}
			tables.Vehicles = append(tables.Vehicles, entry)
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\VehicleSeat.dbc`); err == nil && table.fields >= 46 {
		for row := 0; row < table.records; row++ {
			tables.Seats = append(tables.Seats, ui.VehicleSeatRow{
				ID: table.value(row, 0), Flags: table.value(row, 1),
				UISkin: table.value(row, 44), FlagsB: table.value(row, 45),
			})
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\VehicleUIIndicator.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			tables.Indicators[table.value(row, 0)] = table.string(row, 1)
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\VehicleUIIndicSeat.dbc`); err == nil && table.fields >= 5 {
		for row := 0; row < table.records; row++ {
			tables.IndicatorSeats = append(tables.IndicatorSeats, ui.VehicleIndicatorSeat{
				Indicator: table.value(row, 1), Virtual: table.value(row, 2),
				X: table.valueFloat(row, 3), Y: table.valueFloat(row, 4),
			})
		}
	}
	return tables
}
