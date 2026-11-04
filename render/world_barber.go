package render

import (
	"encoding/binary"
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

// loadBarberTables reads what the barber shop offers: the styles of
// BarberShopStyle.dbc, how many hair colors each style has (CharSections.dbc)
// and what the barber charges at each level (gtBarberShopCostBase.dbc).
func loadBarberTables(loader *ui.Loader) ui.BarberTables {
	tables := ui.BarberTables{HairColors: map[[3]uint32]int{}}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\BarberShopStyle.dbc`); err == nil && table.fields >= 40 {
		for row := 0; row < table.records; row++ {
			tables.Styles = append(tables.Styles, ui.BarberStyle{
				ID: table.value(row, 0), Type: table.value(row, 1), Name: table.string(row, 2), Description: table.string(row, 19),
				CostMod: math.Float32frombits(table.value(row, 36)), Race: table.value(row, 37), Sex: table.value(row, 38), Data: table.value(row, 39),
			})
		}
	}
	if table, err := loadCharacterDBC(loader, `DBFilesClient\CharSections.dbc`); err == nil && table.fields >= 10 {
		for row := 0; row < table.records; row++ {
			// Section type 3 is hair: the variation is the style, the color
			// index the color, and flag 1 marks the ones a player may pick.
			if table.value(row, 3) == 3 && table.value(row, 7)&1 != 0 {
				tables.HairColors[[3]uint32{table.value(row, 1), table.value(row, 2), table.value(row, 8)}]++
			}
		}
	}
	if data, err := loader.ReadFile(`DBFilesClient\gtBarberShopCostBase.dbc`); err == nil && len(data) >= 20 && string(data[:4]) == "WDBC" {
		records := int(binary.LittleEndian.Uint32(data[4:8]))
		for i := 0; i < records && 20+i*4+4 <= len(data); i++ {
			tables.Costs = append(tables.Costs, math.Float32frombits(binary.LittleEndian.Uint32(data[20+i*4:])))
		}
	}
	return tables
}
