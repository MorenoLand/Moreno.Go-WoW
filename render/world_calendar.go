package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// loadCalendarHolidays reads the holidays of Holidays.dbc with their names and
// descriptions.
func loadCalendarHolidays(loader *ui.Loader) []ui.CalendarHoliday {
	table, err := loadCharacterDBC(loader, `DBFilesClient\Holidays.dbc`)
	if err != nil || table.fields < 55 {
		return nil
	}
	names := map[uint32]string{}
	if names1, err := loadCharacterDBC(loader, `DBFilesClient\HolidayNames.dbc`); err == nil && names1.fields >= 2 {
		for row := 0; row < names1.records; row++ {
			names[names1.value(row, 0)] = names1.string(row, 1)
		}
	}
	descriptions := map[uint32]string{}
	if table2, err := loadCharacterDBC(loader, `DBFilesClient\HolidayDescriptions.dbc`); err == nil && table2.fields >= 2 {
		for row := 0; row < table2.records; row++ {
			descriptions[table2.value(row, 0)] = table2.string(row, 1)
		}
	}
	holidays := make([]ui.CalendarHoliday, 0, table.records)
	for row := 0; row < table.records; row++ {
		h := ui.CalendarHoliday{ID: table.value(row, 0)}
		for i := range h.Durations {
			h.Durations[i] = table.value(row, 1+i)
		}
		for i := range h.Dates {
			h.Dates[i] = table.value(row, 11+i)
		}
		h.Region, h.Looping = table.value(row, 37), table.value(row, 38)
		for i := range h.CalFlags {
			h.CalFlags[i] = table.value(row, 39+i)
		}
		h.NameID, h.DescID = table.value(row, 49), table.value(row, 50)
		h.Texture = table.string(row, 51)
		h.Priority, h.FilterType, h.Flags = table.value(row, 52), table.value(row, 53), table.value(row, 54)
		h.Name, h.Description = names[h.NameID], descriptions[h.DescID]
		holidays = append(holidays, h)
	}
	return holidays
}
