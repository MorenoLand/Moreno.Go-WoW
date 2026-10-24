package render

import (
	"math"
	"sync"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

// worldAreaRow is a row of AreaTable.dbc that matters to the place a player is in.
type worldAreaRow struct {
	name   string
	parent uint32
	mapID  uint32
	flags  uint32
	// factions is the factions the area belongs to (2 Alliance, 4 Horde, the faction masks).
	factions uint32
}

// worldAreaData is AreaTable.dbc and WMOAreaTable.dbc.
type worldAreaData struct {
	areas map[uint32]worldAreaRow
	// wmo maps a map object's id, name set and group id to an area; a group id
	// of 0xFFFFFFFF names every group of the set.
	wmo map[[3]uint32]uint32
	// groups are the faction groups (FactionGroup.dbc) in table order, which
	// name an area's controlling side.
	groups []worldFactionGroup
}

type worldFactionGroup struct {
	bit  uint32
	name string
}

var (
	worldAreaOnce sync.Once
	worldAreaInfo *worldAreaData
)

// worldAreas reads the area tables once.
func worldAreas(loader *ui.Loader) *worldAreaData {
	worldAreaOnce.Do(func() {
		data := &worldAreaData{areas: make(map[uint32]worldAreaRow), wmo: make(map[[3]uint32]uint32)}
		if loader != nil {
			if table, err := loadCharacterDBC(loader, `DBFilesClient\AreaTable.dbc`); err == nil && table.fields >= 12 {
				for row := 0; row < table.records; row++ {
					area := worldAreaRow{name: table.string(row, 11), mapID: table.value(row, 1), parent: table.value(row, 2), flags: table.value(row, 4)}
					if table.fields > 28 {
						area.factions = table.value(row, 28)
					}
					data.areas[table.value(row, 0)] = area
				}
			}
			if table, err := loadCharacterDBC(loader, `DBFilesClient\FactionGroup.dbc`); err == nil && table.fields >= 4 {
				for row := 0; row < table.records; row++ {
					data.groups = append(data.groups, worldFactionGroup{bit: 1 << (table.value(row, 1) & 31), name: table.string(row, 3)})
				}
			}
			if table, err := loadCharacterDBC(loader, `DBFilesClient\WMOAreaTable.dbc`); err == nil && table.fields >= 11 {
				for row := 0; row < table.records; row++ {
					if area := table.value(row, 10); area != 0 {
						data.wmo[[3]uint32{table.value(row, 1), table.value(row, 2), table.value(row, 3)}] = area
					}
				}
			}
		}
		worldAreaInfo = data
	})
	return worldAreaInfo
}

// factionText is the name of the side that holds an area, as GetZonePVPInfo
// gives it: the first faction group of the table whose bit the area has.
func (data *worldAreaData) factionText(row worldAreaRow) string {
	if data == nil {
		return ""
	}
	for _, group := range data.groups {
		if row.factions&group.bit != 0 && group.name != "" {
			return group.name
		}
	}
	return ""
}

// wmoArea is the area a map object's group is in.
func (data *worldAreaData) wmoArea(wmoID uint32, nameSet uint16, groupID uint32) uint32 {
	if data == nil {
		return 0
	}
	if area, ok := data.wmo[[3]uint32{wmoID, uint32(nameSet), groupID}]; ok {
		return area
	}
	return data.wmo[[3]uint32{wmoID, uint32(nameSet), math.MaxUint32}]
}

// place names an area as the zone it belongs to and, when it is a part of one,
// the sub zone.
func (data *worldAreaData) place(area uint32) (zone, subZone string, row worldAreaRow) {
	if data == nil {
		return "", "", worldAreaRow{}
	}
	row, ok := data.areas[area]
	if !ok {
		return "", "", worldAreaRow{}
	}
	if row.parent == 0 {
		return row.name, "", row
	}
	top := row
	for depth := 0; top.parent != 0 && depth < 4; depth++ {
		parent, found := data.areas[top.parent]
		if !found {
			break
		}
		top = parent
	}
	return top.name, row.name, row
}

// areaAt is the AreaTable id of the place at a position: the group of a map
// object the position is inside when WMOAreaTable names one, otherwise the
// terrain chunk's.
func (collision worldSceneCollision) areaAt(x, y, z float32) uint32 {
	cell := [2]int{int(math.Floor(float64(x / worldCollisionCell))), int(math.Floor(float64(y / worldCollisionCell)))}
	best := float32(-math.MaxFloat32)
	area := uint32(0)
	for _, ref := range collision.cells[cell] {
		mesh := collision.solids[ref.mesh]
		if mesh.instance == 0 || mesh.area == 0 {
			continue
		}
		triangle := &mesh.triangles[ref.triangle]
		if triangle.max[2] < z-6 || triangle.min[2] > z+0.75 {
			continue
		}
		if math.Abs(float64(triangle.normal[2])) < worldFloorNormalZ || triangle.normal[2] == 0 || !worldPointInTriangle2D([2]float32{x, y}, [2]float32{triangle.a[0], triangle.a[1]}, [2]float32{triangle.b[0], triangle.b[1]}, [2]float32{triangle.c[0], triangle.c[1]}) {
			continue
		}
		height := triangle.a[2] - (triangle.normal[0]*(x-triangle.a[0])+triangle.normal[1]*(y-triangle.a[1]))/triangle.normal[2]
		if height > z+0.75 || height < z-6 || height <= best {
			continue
		}
		best, area = height, mesh.area
	}
	if area != 0 {
		return area
	}
	return collision.terrainArea(x, y)
}

// terrainArea is the area of the terrain chunk under a position.
func (collision worldSceneCollision) terrainArea(x, y float32) uint32 {
	cellX := int(math.Floor(float64(x / float32(worldChunkSize))))
	cellY := int(math.Floor(float64(y / float32(worldChunkSize))))
	for offsetY := -1; offsetY <= 1; offsetY++ {
		for offsetX := -1; offsetX <= 1; offsetX++ {
			for _, chunkIndex := range collision.terrainCells[[2]int{cellX + offsetX, cellY + offsetY}] {
				if chunkIndex >= len(collision.chunks) {
					continue
				}
				if _, _, inside, _ := worldTerrainAt(collision.chunks[chunkIndex], x, y); inside {
					return collision.chunks[chunkIndex].area
				}
			}
		}
	}
	return 0
}

func (h *worldCollisionHolder) areaAt(x, y, z float32) uint32 {
	if c := h.current.Load(); c != nil {
		return c.areaAt(x, y, z)
	}
	return 0
}

// pvpType is the answer of GetZonePVPInfo for an area, for a player of a team
// (2 Alliance, 4 Horde, the faction masks).
func (row worldAreaRow) pvpType(team uint32) string {
	switch {
	case row.flags&0x800 != 0:
		return "sanctuary"
	case row.flags&0x80 != 0:
		return "arena"
	case row.factions == team:
		return "friendly"
	case row.factions != 0 && row.factions != 6 && row.factions&team == 0:
		return "hostile"
	}
	return "contested"
}

// worldTeamOfRace is the faction mask of a race: 2 for the Alliance races and 4
// for the Horde ones.
func worldTeamOfRace(race uint8) uint32 {
	switch race {
	case 2, 5, 6, 8, 10:
		return 4
	}
	return 2
}

// indoorAt reports whether a position is inside a map object group that is lit
// from inside, which is where the minimap shows the rooms rather than the map.
func (collision worldSceneCollision) indoorAt(x, y, z float32) bool {
	cell := [2]int{int(math.Floor(float64(x / worldCollisionCell))), int(math.Floor(float64(y / worldCollisionCell)))}
	best := float32(-math.MaxFloat32)
	indoor := false
	for _, ref := range collision.cells[cell] {
		mesh := collision.solids[ref.mesh]
		if mesh.instance == 0 {
			continue
		}
		triangle := &mesh.triangles[ref.triangle]
		if triangle.max[2] < z-6 || triangle.min[2] > z+0.75 {
			continue
		}
		if math.Abs(float64(triangle.normal[2])) < worldFloorNormalZ || triangle.normal[2] == 0 || !worldPointInTriangle2D([2]float32{x, y}, [2]float32{triangle.a[0], triangle.a[1]}, [2]float32{triangle.b[0], triangle.b[1]}, [2]float32{triangle.c[0], triangle.c[1]}) {
			continue
		}
		height := triangle.a[2] - (triangle.normal[0]*(x-triangle.a[0])+triangle.normal[1]*(y-triangle.a[1]))/triangle.normal[2]
		if height > z+0.75 || height < z-6 || height <= best {
			continue
		}
		best, indoor = height, mesh.indoor
	}
	return indoor
}

func (h *worldCollisionHolder) indoorAt(x, y, z float32) bool {
	if c := h.current.Load(); c != nil {
		return c.indoorAt(x, y, z)
	}
	return false
}
