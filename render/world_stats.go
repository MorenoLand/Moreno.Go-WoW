package render

import (
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

// worldGameTables read the gt*.dbc tables of per class and level values once.
type worldGameTables struct {
	loaded bool
}

var gameTableFiles = []string{
	"CombatRatings", "ChanceToMeleeCrit", "ChanceToMeleeCritBase", "ChanceToSpellCrit", "ChanceToSpellCritBase",
	"OCTRegenHP", "OCTRegenMP", "RegenHPPerSpt", "RegenMPPerSpt", "OCTClassCombatRatingScalar",
}

// syncWorldStats hands the interface the fields the character sheet reads and,
// the first time, the tables it works the rest out from.
func syncWorldStats(rt *ui.Runtime, loader *ui.Loader, tables *worldGameTables, player *worldEntity) {
	if !tables.loaded {
		tables.loaded = true
		data := make(ui.GameTables)
		for _, name := range gameTableFiles {
			table, err := loadCharacterDBC(loader, `DBFilesClient\gt`+name+`.dbc`)
			if err != nil {
				continue
			}
			values := make([]float32, table.records)
			for row := range values {
				values[row] = math.Float32frombits(table.value(row, 0))
			}
			data[name] = values
		}
		rt.SetGameTables(data)
	}
	rt.SetPlayerFields(player.fields)
}
