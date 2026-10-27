package render

import (
	"fmt"
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

// readWorldMapCatalog reads the world map tables: the pages and the patches
// drawn on them as areas are explored, the zone names and the bit each area
// has in the explored-areas bitfield.
func readWorldMapCatalog(loader *ui.Loader) (*ui.MapCatalog, error) {
	pages, err := loadCharacterDBC(loader, `DBFilesClient\WorldMapArea.dbc`)
	if err != nil {
		return nil, err
	}
	if pages.fields < 8 {
		return nil, fmt.Errorf("WorldMapArea.dbc has %d fields", pages.fields)
	}
	catalog := &ui.MapCatalog{AreaNames: make(map[uint32]string), AreaBits: make(map[uint32]uint32)}
	for row := 0; row < pages.records; row++ {
		catalog.Pages = append(catalog.Pages, ui.MapPage{
			ID:        pages.value(row, 0),
			MapID:     pages.value(row, 1),
			AreaID:    pages.value(row, 2),
			Directory: pages.string(row, 3),
			YMax:      math.Float32frombits(pages.value(row, 4)),
			YMin:      math.Float32frombits(pages.value(row, 5)),
			XMax:      math.Float32frombits(pages.value(row, 6)),
			XMin:      math.Float32frombits(pages.value(row, 7)),
		})
	}
	if overlays, err := loadCharacterDBC(loader, `DBFilesClient\WorldMapOverlay.dbc`); err == nil && overlays.fields >= 17 {
		for row := 0; row < overlays.records; row++ {
			catalog.Overlays = append(catalog.Overlays, ui.MapOverlay{
				ID:        overlays.value(row, 0),
				PageID:    overlays.value(row, 1),
				Areas:     [4]uint32{overlays.value(row, 2), overlays.value(row, 3), overlays.value(row, 4), overlays.value(row, 5)},
				Texture:   overlays.string(row, 8),
				Width:     int(overlays.value(row, 9)),
				Height:    int(overlays.value(row, 10)),
				OffsetX:   int(overlays.value(row, 11)),
				OffsetY:   int(overlays.value(row, 12)),
				HitTop:    int(overlays.value(row, 13)),
				HitLeft:   int(overlays.value(row, 14)),
				HitBottom: int(overlays.value(row, 15)),
				HitRight:  int(overlays.value(row, 16)),
			})
		}
	}
	if areas, err := loadCharacterDBC(loader, `DBFilesClient\AreaTable.dbc`); err == nil && areas.fields >= 12 {
		for row := 0; row < areas.records; row++ {
			id := areas.value(row, 0)
			if name := areas.string(row, 11); name != "" {
				catalog.AreaNames[id] = name
			}
			if bit := areas.value(row, 3); bit != 0 {
				catalog.AreaBits[id] = bit
			}
		}
	}
	if maps, err := loadCharacterDBC(loader, `DBFilesClient\Map.dbc`); err == nil && maps.fields >= 6 {
		names := make(map[uint32]string)
		catalog.MapNames = names
		catalog.MapTypes = make(map[uint32]uint32)
		for row := 0; row < maps.records; row++ {
			catalog.MapTypes[maps.value(row, 0)] = maps.value(row, 2)
		}
		for row := 0; row < maps.records; row++ {
			names[maps.value(row, 0)] = maps.string(row, 5)
		}
		// The four continents in the order the map's continent list shows them.
		for _, mapID := range []uint32{1, 0, 530, 571} {
			catalog.Continents = append(catalog.Continents, ui.MapContinent{MapID: mapID, Name: names[mapID]})
		}
	}
	return catalog, nil
}
