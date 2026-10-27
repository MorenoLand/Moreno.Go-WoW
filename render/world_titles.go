package render

import "github.com/MorenoLand/Moreno.WoW/ui"

// loadWorldTitles hands the interface the rows of CharTitles.dbc once.
func loadWorldTitles(rt *ui.Runtime, loader *ui.Loader, loaded *bool) {
	if *loaded {
		return
	}
	*loaded = true
	table, err := loadCharacterDBC(loader, `DBFilesClient\CharTitles.dbc`)
	if err != nil || table.fields < 37 {
		return
	}
	titles := make([]ui.CharTitle, 0, table.records)
	for row := 0; row < table.records; row++ {
		titles = append(titles, ui.CharTitle{
			ID:     table.value(row, 0),
			Mask:   table.value(row, 36),
			Male:   table.string(row, 2),
			Female: table.string(row, 19),
		})
	}
	rt.SetTitles(titles)
}
