package render

import "math"
import "github.com/MorenoLand/Moreno.WoW/ui"

// loadAreaPOIs reads the places the world map marks.
func loadAreaPOIs(loader *ui.Loader) []ui.AreaPOI {
	table, err := loadCharacterDBC(loader, `DBFilesClient\AreaPOI.dbc`)
	if err != nil || table.fields < 54 {
		return nil
	}
	pois := make([]ui.AreaPOI, 0, table.records)
	for row := 0; row < table.records; row++ {
		poi := ui.AreaPOI{
			ID: table.value(row, 0), X: math.Float32frombits(table.value(row, 12)), Y: math.Float32frombits(table.value(row, 13)),
			Continent: table.value(row, 15), Flags: table.value(row, 16), Area: table.value(row, 17),
			Name: table.string(row, 18), Desc: table.string(row, 35), WorldState: table.value(row, 52), MapLink: table.value(row, 53),
		}
		for index := range poi.Icons {
			poi.Icons[index] = table.value(row, 2+index)
		}
		pois = append(pois, poi)
	}
	return pois
}

// loadShapeshiftBars reads the action bar each form of a shapeshifter shows.
func loadShapeshiftBars(loader *ui.Loader) map[uint32]ui.ShapeshiftBar {
	table, err := loadCharacterDBC(loader, `DBFilesClient\ShapeshiftForm.dbc`)
	if err != nil || table.fields < 33 {
		return nil
	}
	bars := make(map[uint32]ui.ShapeshiftBar, table.records)
	for row := 0; row < table.records; row++ {
		bar := ui.ShapeshiftBar{Bar: table.value(row, 1)}
		for column := 25; column < 33; column++ {
			if table.value(row, column) != 0 {
				bar.HasPresets = true
			}
		}
		bars[table.value(row, 0)] = bar
	}
	return bars
}

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

// loadRelicClasses lists the classes that carry a relic in the ranged slot.
func loadRelicClasses(loader *ui.Loader) map[uint32]bool {
	table, err := loadCharacterDBC(loader, `DBFilesClient\ChrClasses.dbc`)
	if err != nil || table.fields < 58 {
		return nil
	}
	classes := map[uint32]bool{}
	for row := 0; row < table.records; row++ {
		if table.value(row, 57)>>3&1 != 0 {
			classes[table.value(row, 0)] = true
		}
	}
	return classes
}

// loadSkillAbilities lists, for each skill, the spells it teaches in the order
// of the table.
func loadSkillAbilities(loader *ui.Loader) map[uint32][]ui.SkillAbility {
	table, err := loadCharacterDBC(loader, `DBFilesClient\SkillLineAbility.dbc`)
	if err != nil || table.fields < 5 {
		return nil
	}
	abilities := map[uint32][]ui.SkillAbility{}
	for row := 0; row < table.records; row++ {
		skill := table.value(row, 1)
		abilities[skill] = append(abilities[skill], ui.SkillAbility{Spell: table.value(row, 2), RaceMask: table.value(row, 3), ClassMask: table.value(row, 4)})
	}
	return abilities
}
