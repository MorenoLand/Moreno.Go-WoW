package ui

import lua "github.com/yuin/gopher-lua"

// AreaPOI is one row of AreaPOI.dbc: a place the world map marks.
type AreaPOI struct {
	ID         uint32
	Icons      [9]uint32
	X, Y       float32
	Continent  uint32
	Flags      uint32
	Area       uint32
	Name       string
	Desc       string
	WorldState uint32
	MapLink    uint32
}

// The flags of a landmark that say on which maps it is drawn.
const (
	poiOnZoneMap      = 0x004
	poiOnContinentMap = 0x008
	poiPickIcon       = 0x080
	poiNeedsPage      = 0x400
	poiBattleMap      = 0x200
)

// SetAreaPOIs gives the landmarks of AreaPOI.dbc.
func (rt *Runtime) SetAreaPOIs(pois []AreaPOI) { rt.areaPOIs = pois }

// landmark is a place of the map in view, placed on its page.
type landmark struct {
	poi  *AreaPOI
	x, y float64
}

// landmarks are the places drawn on the page in view. The world and cosmic
// views, which have no page of their own, show none.
func (rt *Runtime) landmarks() []landmark {
	state := &rt.worldMap
	if state.dungeon == nil && state.continent == worldMapWorld && state.zone == 0 {
		return rt.worldViewLandmarks()
	}
	page := state.currentPage()
	if page == nil || state.catalog == nil || !page.hasBounds() {
		return nil
	}
	zoneView := state.zone != 0
	var list []landmark
	for index := range rt.areaPOIs {
		poi := &rt.areaPOIs[index]
		if poi.Continent != page.MapID {
			continue
		}
		if zoneView && poi.Flags&poiOnZoneMap == 0 || !zoneView && poi.Flags&poiOnContinentMap == 0 {
			continue
		}
		if poi.Flags&poiNeedsPage != 0 && poi.Area != page.AreaID {
			continue
		}
		x, y := page.project(poi.X, poi.Y)
		if x < 0 || x > 1 || y < 0 || y > 1 || (x == 0 && y == 0) {
			continue
		}
		if poi.Area > 0 {
			if _, known := state.catalog.AreaNames[poi.Area]; !known {
				continue
			}
			if _, bound := state.catalog.AreaBits[poi.Area]; bound && !state.areaExplored(poi.Area) {
				continue
			}
		}
		if poi.WorldState != 0 && rt.worldStateValue(poi.WorldState) == 0 {
			continue
		}
		list = append(list, landmark{poi, x, y})
	}
	return list
}

func registerLandmarkAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	reg("GetNumMapLandmarks", func(L *lua.LState) int {
		L.Push(lua.LNumber(len(rt.landmarks())))
		return 1
	})
	reg("GetMapLandmarkInfo", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetMapLandmarkInfo(index)")
			return 0
		}
		list := rt.landmarks()
		index := L.CheckInt(1)
		if index < 1 || index > len(list) {
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			for i := 0; i < 3; i++ {
				L.Push(lua.LNumber(0))
			}
			L.Push(lua.LNil)
			L.Push(lua.LNil)
			return 7
		}
		found := list[index-1]
		poi := found.poi
		texture := uint32(0)
		if poi.Flags&poiPickIcon != 0 || rt.worldMap.zone == 0 {
			pick := 0
			if poi.WorldState != 0 {
				if pick = int(rt.worldStateValue(poi.WorldState)) - 1; pick < 0 || pick > 8 {
					pick = 0
				}
			}
			texture = poi.Icons[pick]
		}
		L.Push(lua.LString(poi.Name))
		L.Push(lua.LString(poi.Desc))
		L.Push(lua.LNumber(texture))
		L.Push(lua.LNumber(found.x))
		L.Push(lua.LNumber(found.y))
		if poi.MapLink == 0 {
			L.Push(lua.LNil)
		} else {
			L.Push(lua.LNumber(poi.MapLink))
		}
		L.Push(lua.LBool(poi.Flags&poiBattleMap != 0))
		return 7
	})
}

// The map the transforms move Outland's places onto; those places are left off
// the world map.
const outlandMap = 530

// poiShown is whether the place passes the checks every view makes: the area
// has been explored and its world state is up.
func (rt *Runtime) poiShown(poi *AreaPOI) bool {
	state := &rt.worldMap
	if poi.Area > 0 {
		if _, known := state.catalog.AreaNames[poi.Area]; !known {
			return false
		}
		if _, bound := state.catalog.AreaBits[poi.Area]; bound && !state.areaExplored(poi.Area) {
			return false
		}
	}
	return poi.WorldState == 0 || rt.worldStateValue(poi.WorldState) != 0
}

// worldViewLandmarks are the places drawn on the map of the whole world.
func (rt *Runtime) worldViewLandmarks() []landmark {
	state := &rt.worldMap
	if state.catalog == nil {
		return nil
	}
	var list []landmark
	for index := range rt.areaPOIs {
		poi := &rt.areaPOIs[index]
		if poi.Flags&poiOnWorldMap == 0 || poi.Flags&poiOnContinentMap == 0 {
			continue
		}
		mapID, x, y := poi.Continent, poi.X, poi.Y
		for _, transform := range state.catalog.Transforms {
			if transform.MapID == mapID && transform.MinX < x && x <= transform.MaxX && transform.MinY <= y && y < transform.MaxY {
				mapID, x, y = transform.NewMapID, x+transform.OffsetX, y+transform.OffsetY
				break
			}
		}
		if mapID == outlandMap {
			continue
		}
		place, ok := state.catalog.ContinentPlaces[mapID]
		if !ok || !rt.poiShown(poi) {
			continue
		}
		u := float64(place.OffsetX*worldMapOffsetXScale+0.5) - float64(y*worldMapXScale*place.Scale)
		v := float64(place.OffsetY*worldMapOffsetYScale+0.5) - float64(x*worldMapYScale*place.Scale)
		list = append(list, landmark{poi, u, v})
	}
	return list
}

// The constants that place a continent on the world map.
const (
	worldMapOffsetXScale = 0.015968063846230507
	worldMapOffsetYScale = 0.023952096700668335
	worldMapXScale       = 2.994011992996093e-05
	worldMapYScale       = 4.49101826234255e-05
	poiOnWorldMap        = 0x010
)

// clickLandmark opens the instance a landmark leads to, at the floor named.
func (rt *Runtime) clickLandmark(floorID uint32) {
	state := &rt.worldMap
	floor, ok := state.floorByID(floorID)
	if !ok || state.catalog == nil {
		return
	}
	var best *MapPage
	for index := range state.catalog.Pages {
		page := &state.catalog.Pages[index]
		if page.MapID == floor.MapID && page.AreaID != 0 {
			best = page
			break
		}
	}
	if best == nil {
		return
	}
	state.dungeon, state.floor = best, floorID
	state.continent, state.zone = worldMapCosmic, 0
	rt.FireEvent("WORLD_MAP_UPDATE")
}

func registerClickLandmarkAPI(rt *Runtime) {
	rt.L.SetGlobal("ClickLandmark", rt.L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: ClickLandmark(mapLinkID)")
			return 0
		}
		rt.clickLandmark(uint32(L.CheckInt(1)))
		return 0
	}))
}
