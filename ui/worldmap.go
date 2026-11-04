package ui

import (
	"sort"

	lua "github.com/yuin/gopher-lua"
)

// MapPage is one row of WorldMapArea.dbc: a drawable map page and the world
// rectangle it shows.
type MapPage struct {
	ID, MapID, AreaID      uint32
	Directory              string
	// DisplayMapID is the map the page borrows its terrain from; all ones when it
	// shows its own.
	DisplayMapID uint32
	// DefaultFloor is the floor map of the page that shows first, 0 for a page
	// with none.
	DefaultFloor uint32
	XMax, XMin, YMax, YMin float32
}

func (page MapPage) hasBounds() bool { return page.XMax > page.XMin && page.YMax > page.YMin }

func (page MapPage) contains(x, y float32) bool {
	return page.hasBounds() && x >= page.XMin && x <= page.XMax && y >= page.YMin && y <= page.YMax
}

func (page MapPage) worldArea() float32 {
	return (page.XMax - page.XMin) * (page.YMax - page.YMin)
}

// project places a world position on the page as fractions with (0, 0) at the
// top left of the picture: west is +y and north is +x, so the left edge is the
// largest y and the top edge the largest x.
func (page MapPage) project(x, y float32) (float64, float64) {
	u := float64(page.YMax-y) / float64(page.YMax-page.YMin)
	v := float64(page.XMax-x) / float64(page.XMax-page.XMin)
	return u, v
}

// MapOverlay is one row of WorldMapOverlay.dbc: a patch of a zone page that is
// drawn once the player has explored one of its areas.
type MapOverlay struct {
	ID, PageID                           uint32
	Areas                                [4]uint32
	Texture                              string
	Width, Height, OffsetX, OffsetY      int
	HitTop, HitLeft, HitBottom, HitRight int
}

// MapContinent names one of the four continents the map can zoom to.
type MapContinent struct {
	MapID uint32
	Name  string
}

// MapCatalog is the world map data read from the game files.
type MapCatalog struct {
	Pages      []MapPage
	Overlays   []MapOverlay
	Continents []MapContinent // Kalimdor, Eastern Kingdoms, Outland, Northrend
	AreaNames  map[uint32]string
	AreaBits   map[uint32]uint32 // AreaTable id -> explored-area bit
	// MapNames are the names of the maps (instances and continents), by id.
	MapNames map[uint32]string
	// MapTypes are the instance types of the maps: 1 dungeon, 2 raid.
	MapTypes map[uint32]uint32
	// MapFlags are the flags of the maps.
	MapFlags map[uint32]uint32
	// DungeonFloors are the floor maps of instances (DungeonMap.dbc).
	DungeonFloors []DungeonFloor
	// ContinentPlaces say where each continent sits on the world map, and
	// Transforms move the places of one map onto another.
	ContinentPlaces map[uint32]ContinentPlace
	Transforms      []MapTransform
}

// ContinentPlace is the offset and the scale of a continent on the world map.
type ContinentPlace struct{ OffsetX, OffsetY, Scale float32 }

// MapTransform moves a box of one map onto another map.
type MapTransform struct {
	MapID                  uint32
	MinX, MinY, MaxX, MaxY float32
	NewMapID               uint32
	OffsetX, OffsetY       float32
}

// DungeonFloor is one floor map of an instance.
type DungeonFloor struct {
	ID, MapID uint32
	Floor     int
}

// pageTerrainOwn is the display map of a page that shows its own terrain.
const pageTerrainOwn = 0xFFFFFFFF

// Page sizes in the pixel space overlays are stated in.
const (
	worldMapPageWidth  = 1002.0
	worldMapPageHeight = 668.0
)

const (
	worldMapCosmic = -1
	worldMapWorld  = 0
)

// worldMapState is what the map frame is currently showing and where the
// player is.
type worldMapState struct {
	catalog   *MapCatalog
	continent int
	zone      int // 0 shows the continent page
	// dungeon is the page of the instance in view, which has no continent, and
	// floor the floor map of it that is showing (0 for none).
	dungeon *MapPage
	floor   uint32
	playerMap uint32
	playerX   float32
	playerY   float32
	hasPlayer bool
	explored  [128]uint32
}

// SetMapCatalog installs the world map data.
func (rt *Runtime) SetMapCatalog(catalog *MapCatalog) {
	rt.worldMap.catalog = catalog
	if rt.worldMap.hasPlayer {
		rt.setMapToCurrentZone()
	}
}

// SetPlayerMapPosition records where the player stands, in world coordinates.
func (rt *Runtime) SetPlayerMapPosition(mapID uint32, x, y float32) {
	changedMap := !rt.worldMap.hasPlayer || rt.worldMap.playerMap != mapID
	rt.worldMap.playerMap, rt.worldMap.playerX, rt.worldMap.playerY, rt.worldMap.hasPlayer = mapID, x, y, true
	if changedMap {
		rt.setMapToCurrentZone()
	}
}

// SetExploredZones records the player's explored-areas bitfield.
func (rt *Runtime) SetExploredZones(words []uint32) {
	copy(rt.worldMap.explored[:], words)
}

func (state *worldMapState) areaExplored(area uint32) bool {
	if state.catalog == nil {
		return false
	}
	bit, ok := state.catalog.AreaBits[area]
	if !ok || bit == 0 || bit >= uint32(len(state.explored))*32 {
		return false
	}
	return state.explored[bit/32]&(1<<(bit%32)) != 0
}

// continentPage is the whole-continent page of a continent index.
func (state *worldMapState) continentPage(continent int) *MapPage {
	if state.catalog == nil || continent < 1 || continent > len(state.catalog.Continents) {
		return nil
	}
	mapID := state.catalog.Continents[continent-1].MapID
	for index := range state.catalog.Pages {
		page := &state.catalog.Pages[index]
		if page.MapID == mapID && page.AreaID == 0 {
			return page
		}
	}
	return nil
}

// zonePages are the zone pages of a continent, in the order the zone list
// shows them: alphabetical by the name players see.
func (state *worldMapState) zonePages(continent int) []*MapPage {
	if state.catalog == nil || continent < 1 || continent > len(state.catalog.Continents) {
		return nil
	}
	mapID := state.catalog.Continents[continent-1].MapID
	var pages []*MapPage
	for index := range state.catalog.Pages {
		page := &state.catalog.Pages[index]
		if page.MapID == mapID && page.AreaID != 0 && state.catalog.AreaNames[page.AreaID] != "" {
			pages = append(pages, page)
		}
	}
	sort.SliceStable(pages, func(a, b int) bool {
		return state.catalog.AreaNames[pages[a].AreaID] < state.catalog.AreaNames[pages[b].AreaID]
	})
	return pages
}

// currentPage is the page the map is showing, or nil for the world and cosmic
// views, which have no page of their own.
func (state *worldMapState) currentPage() *MapPage {
	if state.dungeon != nil {
		return state.dungeon
	}
	if state.zone == 0 {
		return state.continentPage(state.continent)
	}
	pages := state.zonePages(state.continent)
	if state.zone < 1 || state.zone > len(pages) {
		return nil
	}
	return pages[state.zone-1]
}

func (state *worldMapState) continentOfMap(mapID uint32) int {
	if state.catalog == nil {
		return 0
	}
	for index, continent := range state.catalog.Continents {
		if continent.MapID == mapID {
			return index + 1
		}
	}
	return 0
}

// showPage points the map at a page.
func (state *worldMapState) showPage(page *MapPage) bool {
	if page == nil || state.catalog == nil {
		return false
	}
	continent := state.continentOfMap(page.MapID)
	if continent == 0 {
		return false
	}
	if page.DefaultFloor > 0 {
		// A page that is a floor of a dungeon opens as the dungeon.
		state.dungeon, state.floor = page, page.DefaultFloor
		state.continent, state.zone = worldMapCosmic, 0
		return true
	}
	state.dungeon, state.floor = nil, 0
	state.continent = continent
	state.zone = 0
	if page.AreaID == 0 {
		return true
	}
	for index, candidate := range state.zonePages(continent) {
		if candidate == page {
			state.zone = index + 1
			return true
		}
	}
	return true
}

// setMapToCurrentZone shows the page the player is standing on: the smallest
// zone page containing them, so a city wins over the zone around it.
func (rt *Runtime) setMapToCurrentZone() {
	state := &rt.worldMap
	if state.catalog == nil || !state.hasPlayer {
		return
	}
	var best *MapPage
	for index := range state.catalog.Pages {
		page := &state.catalog.Pages[index]
		if page.MapID != state.playerMap || page.AreaID == 0 || !page.contains(state.playerX, state.playerY) {
			continue
		}
		if best == nil || page.worldArea() < best.worldArea() {
			best = page
		}
	}
	if best == nil {
		for index := range state.catalog.Pages {
			if page := &state.catalog.Pages[index]; page.MapID == state.playerMap && page.AreaID == 0 {
				best = page
				break
			}
		}
	}
	if !state.showPage(best) {
		if best != nil && state.continentOfMap(best.MapID) == 0 {
			// An instance has no continent: its own page is the whole view.
			state.dungeon, state.floor = best, 0
			state.continent, state.zone = worldMapCosmic, 0
		} else {
			state.dungeon, state.floor = nil, 0
			state.continent, state.zone = worldMapWorld, 0
		}
	}
	rt.FireEvent("WORLD_MAP_UPDATE")
}

// zoomOut moves up one level: zone to continent, continent to world, world to
// cosmic.
func (state *worldMapState) zoomOut() {
	if state.dungeon != nil {
		state.dungeon, state.floor = nil, 0
		state.continent, state.zone = worldMapWorld, 0
		return
	}
	switch {
	case state.zone != 0:
		state.zone = 0
	case state.continent > 0:
		state.continent = worldMapWorld
	case state.continent == worldMapWorld:
		state.continent = worldMapCosmic
	}
}

// zoneBox is where a zone page sits on its continent page, as fractions of the
// continent picture (left, top, right, bottom).
func zoneBox(continent, zone *MapPage) (left, top, right, bottom float64) {
	left, top = continent.project(zone.XMax, zone.YMax)
	right, bottom = continent.project(zone.XMin, zone.YMin)
	return left, top, right, bottom
}

// zoneAt returns the index and page of the smallest zone box that holds a point
// of the continent page.
func (state *worldMapState) zoneAt(x, y float64) (int, *MapPage) {
	continent := state.continentPage(state.continent)
	if continent == nil || !continent.hasBounds() {
		return 0, nil
	}
	bestIndex := 0
	var best *MapPage
	for index, zone := range state.zonePages(state.continent) {
		if !zone.hasBounds() {
			continue
		}
		left, top, right, bottom := zoneBox(continent, zone)
		if x < left || x > right || y < top || y > bottom {
			continue
		}
		if best == nil || zone.worldArea() < best.worldArea() {
			bestIndex, best = index+1, zone
		}
	}
	return bestIndex, best
}

// overlayExplored reports whether any area an overlay covers has been explored.
func (state *worldMapState) overlayExplored(overlay MapOverlay) bool {
	for _, area := range overlay.Areas {
		if area != 0 && state.areaExplored(area) {
			return true
		}
	}
	return false
}

func (state *worldMapState) visibleOverlays() []MapOverlay {
	page := state.currentPage()
	if page == nil || state.zone == 0 || state.catalog == nil {
		return nil
	}
	var overlays []MapOverlay
	for _, overlay := range state.catalog.Overlays {
		if overlay.PageID == page.ID && overlay.Texture != "" && overlay.Width > 0 && overlay.Height > 0 && state.overlayExplored(overlay) {
			overlays = append(overlays, overlay)
		}
	}
	return overlays
}

func (state *worldMapState) pageName(page *MapPage) string {
	if page == nil || state.catalog == nil {
		return ""
	}
	if page.AreaID != 0 {
		return state.catalog.AreaNames[page.AreaID]
	}
	for _, continent := range state.catalog.Continents {
		if continent.MapID == page.MapID {
			return continent.Name
		}
	}
	return page.Directory
}

// registerWorldMapAPI defines the map C-API the world map frame is built on.
func registerWorldMapAPI(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	state := &rt.worldMap
	reg("GetMapContinents", func(L *lua.LState) int {
		if state.catalog == nil {
			return 0
		}
		for _, continent := range state.catalog.Continents {
			L.Push(lua.LString(continent.Name))
		}
		return len(state.catalog.Continents)
	})
	reg("GetMapZones", func(L *lua.LState) int {
		pages := state.zonePages(L.CheckInt(1))
		for _, page := range pages {
			L.Push(lua.LString(state.catalog.AreaNames[page.AreaID]))
		}
		return len(pages)
	})
	reg("GetCurrentMapContinent", func(L *lua.LState) int { L.Push(lua.LNumber(state.continent)); return 1 })
	reg("GetCurrentMapZone", func(L *lua.LState) int { L.Push(lua.LNumber(state.zone)); return 1 })
	reg("GetCurrentMapAreaID", func(L *lua.LState) int {
		if page := state.currentPage(); page != nil {
			L.Push(lua.LNumber(page.ID))
		} else {
			L.Push(lua.LNumber(-1))
		}
		return 1
	})
	reg("GetCurrentMapDungeonLevel", func(L *lua.LState) int {
		level := 0
		if state.dungeon != nil {
			if floor, ok := state.floorByID(state.floor); ok {
				level = floor.Floor
			}
			if state.dungeon.DisplayMapID == pageTerrainOwn {
				level++
			}
		}
		L.Push(lua.LNumber(level))
		return 1
	})
	reg("GetNumDungeonMapLevels", func(L *lua.LState) int {
		L.Push(lua.LNumber(state.dungeonLevels()))
		return 1
	})
	reg("SetDungeonMapLevel", func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: SetDungeonMapLevel(level)")
			return 0
		}
		level := L.CheckInt(1)
		if state.dungeon == nil {
			return 0
		}
		if state.dungeon.DisplayMapID == pageTerrainOwn {
			level--
			if level < 0 {
				return 0
			}
		} else if level < 1 {
			return 0
		}
		if level == 0 {
			state.floor = 0
			rt.FireEvent("WORLD_MAP_UPDATE")
			return 0
		}
		for _, floor := range state.catalog.DungeonFloors {
			if floor.MapID == state.dungeon.MapID && floor.Floor == level && floor.ID > 0 {
				state.floor = floor.ID
				rt.FireEvent("WORLD_MAP_UPDATE")
				return 0
			}
		}
		return 0
	})
	reg("DungeonUsesTerrainMap", func(L *lua.LState) int {
		if state.dungeon != nil && state.dungeon.DisplayMapID == pageTerrainOwn {
			L.Push(lua.LNumber(1))
		} else {
			L.Push(lua.LNil)
		}
		return 1
	})
	reg("SetMapToCurrentZone", func(L *lua.LState) int { rt.setMapToCurrentZone(); return 0 })
	reg("SetMapZoom", func(L *lua.LState) int {
		continent := L.CheckInt(1)
		zone := L.OptInt(2, 0)
		if continent > 0 && state.catalog != nil && continent > len(state.catalog.Continents) {
			return 0
		}
		state.continent, state.zone = continent, zone
		if continent > 0 && zone > len(state.zonePages(continent)) {
			state.zone = 0
		}
		rt.FireEvent("WORLD_MAP_UPDATE")
		return 0
	})
	reg("SetMapByID", func(L *lua.LState) int {
		if state.catalog == nil {
			return 0
		}
		id := uint32(L.CheckInt(1))
		for index := range state.catalog.Pages {
			if state.catalog.Pages[index].ID == id {
				state.showPage(&state.catalog.Pages[index])
				break
			}
		}
		rt.FireEvent("WORLD_MAP_UPDATE")
		return 0
	})
	reg("ZoomOut", func(L *lua.LState) int { state.zoomOut(); rt.FireEvent("WORLD_MAP_UPDATE"); return 0 })
	reg("IsZoomOutAvailable", func(L *lua.LState) int { L.Push(lua.LBool(state.continent != worldMapCosmic)); return 1 })
	reg("GetMapInfo", func(L *lua.LState) int {
		page := state.currentPage()
		if page == nil {
			return 0
		}
		L.Push(lua.LString(page.Directory))
		L.Push(lua.LNumber(768))
		return 2
	})
	reg("GetNumMapOverlays", func(L *lua.LState) int { L.Push(lua.LNumber(len(state.visibleOverlays()))); return 1 })
	reg("GetMapOverlayInfo", func(L *lua.LState) int {
		overlays := state.visibleOverlays()
		index := L.CheckInt(1)
		page := state.currentPage()
		if index < 1 || index > len(overlays) || page == nil {
			return 0
		}
		overlay := overlays[index-1]
		L.Push(lua.LString(`Interface\WorldMap\` + page.Directory + `\` + overlay.Texture))
		L.Push(lua.LNumber(overlay.Width))
		L.Push(lua.LNumber(overlay.Height))
		L.Push(lua.LNumber(overlay.OffsetX))
		L.Push(lua.LNumber(overlay.OffsetY))
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		return 7
	})
	reg("GetPlayerMapPosition", func(L *lua.LState) int {
		x, y := 0.0, 0.0
		if page := state.currentPage(); page != nil && state.hasPlayer && L.CheckString(1) == "player" &&
			page.MapID == state.playerMap && page.contains(state.playerX, state.playerY) {
			x, y = page.project(state.playerX, state.playerY)
		}
		L.Push(lua.LNumber(x))
		L.Push(lua.LNumber(y))
		return 2
	})
	reg("GetCorpseMapPosition", func(L *lua.LState) int { L.Push(lua.LNumber(0)); L.Push(lua.LNumber(0)); return 2 })
	reg("GetDeathReleasePosition", func(L *lua.LState) int { L.Push(lua.LNumber(0)); L.Push(lua.LNumber(0)); return 2 })
	// The zone under the cursor: the continent view names the zone box the
	// point falls in, the zone view names the explored patch it falls on.
	reg("UpdateMapHighlight", func(L *lua.LState) int {
		x, y := float64(L.CheckNumber(1)), float64(L.CheckNumber(2))
		switch {
		case state.zone == 0 && state.continent > 0:
			if _, zone := state.zoneAt(x, y); zone != nil {
				L.Push(lua.LString(state.pageName(zone)))
				return 1
			}
		case state.zone != 0:
			page := state.currentPage()
			if page == nil {
				return 0
			}
			px, py := x*worldMapPageWidth, y*worldMapPageHeight
			var best *MapOverlay
			for _, overlay := range state.visibleOverlays() {
				overlay := overlay
				if px < float64(overlay.HitLeft) || px > float64(overlay.HitRight) || py < float64(overlay.HitTop) || py > float64(overlay.HitBottom) {
					continue
				}
				if best == nil || overlay.Width*overlay.Height < best.Width*best.Height {
					best = &overlay
				}
			}
			if best != nil {
				L.Push(lua.LString(state.catalog.AreaNames[best.Areas[0]]))
				return 1
			}
		}
		return 0
	})
	reg("ProcessMapClick", func(L *lua.LState) int {
		x, y := float64(L.CheckNumber(1)), float64(L.CheckNumber(2))
		switch {
		case state.continent == worldMapCosmic:
			// The cosmic view offers Azeroth and Outland as buttons.
		case state.continent == worldMapWorld:
			if x < 0.5 {
				state.continent = 1
			} else {
				state.continent = 2
			}
			state.zone = 0
		case state.zone == 0:
			if index, zone := state.zoneAt(x, y); zone != nil {
				state.zone = index
			}
		}
		rt.FireEvent("WORLD_MAP_UPDATE")
		return 0
	})

}

// registerWorldMapArrow defines the functions that manage the player arrow of
// the world map. The client creates the arrow as two model frames of
// Interface\Minimap\MinimapArrow.mdx under the map frame; model frames are not
// drawn by the software UI, so the frames exist, are positioned and shown, but
// leave no pixels.
func registerWorldMapArrow(rt *Runtime) {
	L := rt.L
	reg := func(name string, fn lua.LGFunction) { L.SetGlobal(name, L.NewFunction(fn)) }
	arrows := func() []*widget {
		var frames []*widget
		for _, name := range []string{"PlayerArrowFrame", "PlayerArrowEffectFrame"} {
			if frame := rt.widgets[name]; frame != nil {
				frames = append(frames, frame)
			}
		}
		return frames
	}
	reg("CreateWorldMapArrowFrame", func(L *lua.LState) int {
		parent := rt.widgetArg(L, 1)
		for _, name := range []string{"PlayerArrowFrame", "PlayerArrowEffectFrame"} {
			if rt.widgets[name] != nil {
				continue
			}
			frame := newWidget(kindFrame, name)
			frame.typeName = "Model"
			frame.parent = parent
			frame.shown = false
			frame.modelFile = `Interface\Minimap\MinimapArrow.mdx`
			frame.width, frame.height = 32, 32
			frame.frameLevel = 200 // above the map button and its markers
			// The client draws MinimapArrow.mdx, a textured quad; the software
			// UI draws the same texture turned to the player's facing.
			arrow := newWidget(kindTexture, "")
			arrow.parent = frame
			arrow.textureFile = `Interface\Minimap\MinimapArrow.blp`
			arrow.layerLevel = layerArtwork
			arrow.points = []anchorPoint{{point: "TOPLEFT", relativePoint: "TOPLEFT"}, {point: "BOTTOMRIGHT", relativePoint: "BOTTOMRIGHT"}}
			addWidgetChild(frame, arrow)
			rt.mapArrowTextures = append(rt.mapArrowTextures, arrow)
			if parent != nil {
				frame.frameStrata = parent.frameStrata
				addWidgetChild(parent, frame)
			}
			rt.register(frame)
		}
		return 0
	})
	reg("ShowWorldMapArrowFrame", func(L *lua.LState) int {
		show := L.Get(1) != lua.LNil && L.Get(1) != lua.LFalse
		for _, frame := range arrows() {
			frame.shown = show
		}
		return 0
	})
	reg("PositionWorldMapArrowFrame", func(L *lua.LState) int {
		point, relativeTo, relativePoint := L.CheckString(1), "", L.CheckString(3)
		if value, ok := L.Get(2).(lua.LString); ok {
			relativeTo = string(value)
		} else if frame := rt.widgetArg(L, 2); frame != nil {
			relativeTo = frame.name
		}
		x, y := float64(L.CheckNumber(4)), float64(L.CheckNumber(5))
		for _, frame := range arrows() {
			frame.points = []anchorPoint{{point: point, relativeTo: relativeTo, relativePoint: relativePoint, x: x, y: y}}
		}
		return 0
	})
	reg("UpdateWorldMapArrowFrames", func(L *lua.LState) int { return 0 })
}

// registerSetupFullscreenScale defines SetupFullscreenScale(frame): the frame
// is scaled down when the window is narrower than 4:3 and left at scale 1
// otherwise (Wow.exe: the scale is 0.75 times the aspect ratio, capped at 1).
func registerSetupFullscreenScale(rt *Runtime) {
	L := rt.L
	L.SetGlobal("SetupFullscreenScale", L.NewFunction(func(L *lua.LState) int {
		frame := rt.widgetArg(L, 1)
		if frame == nil {
			L.RaiseError("Usage: SetupFullscreenScale(frame)")
			return 0
		}
		scale := 1.0
		if rt.Host != nil {
			if width, height := rt.Host.ScreenSize(); width > 0 && height > 0 {
				scale = 0.75 * width / height
			}
		}
		if scale > 1 {
			scale = 1
		}
		frame.scale = scale
		return 0
	}))
}

// SetPlayerMapFacing records the player's facing (radians, counter-clockwise
// from north) and turns the world map's player arrow to match.
func (rt *Runtime) SetPlayerMapFacing(radians float32) {
	rt.playerFacing = radians
	for _, arrow := range rt.mapArrowTextures {
		arrow.rotation = float64(radians)
	}
}

// registerQuestMapAPI defines the quest points of interest on the map. The
// client learns them from the server's POI query, which this client does not
// make yet, so no quest has a point and the map counts none.
func registerQuestMapAPI(rt *Runtime) {
	L := rt.L
	L.SetGlobal("QuestMapUpdateAllQuests", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(0))
		return 1
	}))
	L.SetGlobal("QuestPOIGetQuestIDByVisibleIndex", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: QuestPOIGetQuestIDByVisibleIndex(index)")
		}
		return 0
	}))
	L.SetGlobal("GetQuestWorldMapAreaID", L.NewFunction(func(L *lua.LState) int {
		if L.Get(1).Type() != lua.LTNumber {
			L.RaiseError("Usage: GetQuestWorldMapAreaID(questID)")
		}
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		return 2
	}))
}

// floorByID finds a floor map of an instance.
func (state *worldMapState) floorByID(id uint32) (DungeonFloor, bool) {
	if state.catalog != nil {
		for _, floor := range state.catalog.DungeonFloors {
			if floor.ID == id {
				return floor, true
			}
		}
	}
	return DungeonFloor{}, false
}

// dungeonLevels is how many levels the instance in view has: its floors, and
// the ground map of the page itself when it shows its own terrain.
func (state *worldMapState) dungeonLevels() int {
	if state.dungeon == nil || state.catalog == nil {
		return 0
	}
	seen := map[int]bool{}
	for _, floor := range state.catalog.DungeonFloors {
		if floor.MapID == state.dungeon.MapID {
			seen[floor.Floor] = true
		}
	}
	count := len(seen)
	if state.dungeon.DisplayMapID == pageTerrainOwn {
		count++
	}
	return count
}
