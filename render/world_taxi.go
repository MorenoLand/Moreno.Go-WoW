package render

import (
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

// loadTaxiTables reads the flight nodes and the flights between them.
func loadTaxiTables(loader *ui.Loader) ui.TaxiTables {
	tables := ui.TaxiTables{Nodes: make(map[uint32]ui.TaxiNode)}
	if nodes, err := loadCharacterDBC(loader, `DBFilesClient\TaxiNodes.dbc`); err == nil && nodes.fields >= 24 {
		for row := 0; row < nodes.records; row++ {
			id := nodes.value(row, 0)
			tables.Nodes[id] = ui.TaxiNode{
				ID: id, Map: nodes.value(row, 1),
				X:          math.Float32frombits(nodes.value(row, 2)),
				Y:          math.Float32frombits(nodes.value(row, 3)),
				Z:          math.Float32frombits(nodes.value(row, 4)),
				Name:       nodes.string(row, 5),
				AllianceOK: nodes.value(row, 23) != 0,
				HordeOK:    nodes.value(row, 22) != 0,
			}
		}
	}
	if paths, err := loadCharacterDBC(loader, `DBFilesClient\TaxiPath.dbc`); err == nil && paths.fields >= 4 {
		for row := 0; row < paths.records; row++ {
			tables.Paths = append(tables.Paths, ui.TaxiPath{From: paths.value(row, 1), To: paths.value(row, 2), Cost: paths.value(row, 3)})
		}
	}
	return tables
}
