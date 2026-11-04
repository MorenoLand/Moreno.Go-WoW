package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// loadSoulboundEnchants lists the enchantments that bind the item they are on.
func loadSoulboundEnchants(loader *ui.Loader) map[uint32]bool {
	table, err := loadCharacterDBC(loader, `DBFilesClient\SpellItemEnchantment.dbc`)
	if err != nil || table.fields < 33 {
		return nil
	}
	bound := map[uint32]bool{}
	for row := 0; row < table.records; row++ {
		if table.value(row, 32)&1 != 0 {
			bound[table.value(row, 0)] = true
		}
	}
	return bound
}

// loadEnchantGems lists the gem item each socket enchantment comes from.
func loadEnchantGems(loader *ui.Loader) map[uint32]uint32 {
	table, err := loadCharacterDBC(loader, `DBFilesClient\SpellItemEnchantment.dbc`)
	if err != nil || table.fields < 34 {
		return nil
	}
	gems := map[uint32]uint32{}
	for row := 0; row < table.records; row++ {
		if item := table.value(row, 33); item != 0 {
			gems[table.value(row, 0)] = item
		}
	}
	return gems
}

// loadEnchantNames reads the names of the enchantments of
// SpellItemEnchantment.dbc.
func loadEnchantNames(loader *ui.Loader) map[uint32]string {
	table, err := loadCharacterDBC(loader, `DBFilesClient\SpellItemEnchantment.dbc`)
	if err != nil || table.fields < 15 {
		return nil
	}
	names := make(map[uint32]string, table.records)
	for row := 0; row < table.records; row++ {
		names[table.value(row, 0)] = table.string(row, 14)
	}
	return names
}
