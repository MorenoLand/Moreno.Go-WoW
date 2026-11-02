package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// loadPageMaterials reads PageTextMaterial.dbc: the name of each paper a text
// is written on.
func loadPageMaterials(loader *ui.Loader) map[uint32]string {
	materials := make(map[uint32]string)
	if table, err := loadCharacterDBC(loader, `DBFilesClient\PageTextMaterial.dbc`); err == nil && table.fields >= 2 {
		for row := 0; row < table.records; row++ {
			materials[table.value(row, 0)] = table.string(row, 1)
		}
	}
	return materials
}
