package render

import "math"
import "github.com/MorenoLand/Moreno.WoW/ui"

// loadMapIconScales reads how large each map wants its battlefield icons.
func loadMapIconScales(loader *ui.Loader) map[uint32]float32 {
	table, err := loadCharacterDBC(loader, `DBFilesClient\Map.dbc`)
	if err != nil || table.fields < 59 {
		return nil
	}
	scales := make(map[uint32]float32, table.records)
	for row := 0; row < table.records; row++ {
		scales[table.value(row, 0)] = math.Float32frombits(table.value(row, 58))
	}
	return scales
}

// loadExtendedCosts reads what ItemExtendedCost.dbc asks besides gold.
func loadExtendedCosts(loader *ui.Loader) map[uint32]ui.ExtendedCost {
	table, err := loadCharacterDBC(loader, `DBFilesClient\ItemExtendedCost.dbc`)
	if err != nil || table.fields < 14 {
		return nil
	}
	costs := make(map[uint32]ui.ExtendedCost, table.records)
	for row := 0; row < table.records; row++ {
		cost := ui.ExtendedCost{Honor: table.value(row, 1), Arena: table.value(row, 2), Bracket: table.value(row, 3)}
		if table.fields > 14 {
			cost.Rating = table.value(row, 14)
		}
		for i := 0; i < 5; i++ {
			cost.Items[i], cost.Counts[i] = table.value(row, 4+i), table.value(row, 9+i)
		}
		costs[table.value(row, 0)] = cost
	}
	return costs
}
