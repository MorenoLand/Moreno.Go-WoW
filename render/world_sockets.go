package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// loadSocketTables reads the tables the socketing window needs: the colours of
// each gem and the gem each enchantment stands for.
func loadSocketTables(loader *ui.Loader) ui.SocketTables {
	tables := ui.SocketTables{GemTypes: map[uint32]uint32{}, EnchantGems: map[uint32]uint32{}, EnchantNames: map[uint32]string{}}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\GemProperties.dbc`); err == nil && table.fields >= 5 {
		for row := 0; row < table.records; row++ {
			tables.GemTypes[table.value(row, 0)] = table.value(row, 4)
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\SpellItemEnchantment.dbc`); err == nil && table.fields >= 34 {
		for row := 0; row < table.records; row++ {
			tables.EnchantNames[table.value(row, 0)] = table.string(row, 14)
			if gem := table.value(row, 33); gem != 0 {
				tables.EnchantGems[table.value(row, 0)] = gem
			}
		}
	}
	return tables
}
