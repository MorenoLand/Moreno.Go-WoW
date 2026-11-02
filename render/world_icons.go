package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// loadSpellIcons reads the icon of each SpellIcon.dbc row.
func loadSpellIcons(loader *ui.Loader) map[uint32]string {
	icons := map[uint32]string{}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\SpellIcon.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			icons[table.value(row, 0)] = table.string(row, 1)
		}
	}
	return icons
}
