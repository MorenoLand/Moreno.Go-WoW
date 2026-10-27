package render

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/texture"
)

// worldCollisionHolder is the collision the movement controller and entities
// ask. Its content is replaced as terrain tiles stream in and out; callers keep
// the holder.
type worldCollisionHolder struct {
	current atomic.Pointer[worldSceneCollision]
}

func (h *worldCollisionHolder) ground(x, y float32) (float32, bool) {
	if c := h.current.Load(); c != nil {
		return c.ground(x, y)
	}
	return 0, false
}

func (h *worldCollisionHolder) floor(x, y, reference float32) (float32, bool) {
	if c := h.current.Load(); c != nil {
		return c.floor(x, y, reference)
	}
	return 0, false
}

func (h *worldCollisionHolder) move(from, to [3]float32) [3]float32 {
	if c := h.current.Load(); c != nil {
		return c.move(from, to)
	}
	return to
}

func (h *worldCollisionHolder) cameraPosition(focus, eye math32.Vector3) math32.Vector3 {
	if c := h.current.Load(); c != nil {
		return c.cameraPosition(focus, eye)
	}
	return eye
}

// segmentBlocked reports whether ground or a wall lies between two points.
func (h *worldCollisionHolder) segmentBlocked(from, to [3]float32) bool {
	if c := h.current.Load(); c != nil {
		return c.segmentBlocked(from, to)
	}
	return false
}

func (h *worldCollisionHolder) liquidAt(x, y, z float32) (float32, bool) {
	if c := h.current.Load(); c != nil {
		return c.liquidAt(x, y, z)
	}
	return 0, false
}

// worldStreamedTile is one loaded terrain tile: its scene node and what it
// contributes to collision.
type worldStreamedTile struct {
	node      *core.Node
	collision worldSceneCollision
	info      worldSceneInfo
}

type worldTileResult struct {
	key  [2]int
	tile *worldStreamedTile
	err  error
}

// worldTerrainStreamer keeps the tiles around the player loaded. A single
// worker loads tiles (they share one asset cache); the render loop attaches
// finished tiles and releases distant ones.
type worldTerrainStreamer struct {
	loader   *ui.Loader
	mapName  string
	position world.WorldPosition
	bigAlpha bool
	cache    *worldSceneAssetCache
	// compacted are the geometries whose CPU copies have been released.
	compacted   map[*geometry.Geometry]bool
	compactedAt time.Time
	root        *core.Node

	collision *worldCollisionHolder
	liquid    *worldLiquidAnimator

	tiles    map[[2]int]*worldStreamedTile
	inflight map[[2]int]bool
	requests chan [2]int
	results  chan worldTileResult
	stopOnce sync.Once
	// collisionGeneration orders background collision builds so a stale one never
	// replaces a newer one.
	collisionGeneration atomic.Uint64
	appliedGeneration   atomic.Uint64
	lastTile            [2]int

	// reveal holds the parts of freshly attached tiles that are still hidden; a few
	// are shown each frame so their first draw (buffer and texture uploads) is
	// spread over many frames instead of stalling one.
	reveal []worldRevealItem
	// preloaded are tiles built before the world was entered that are not yet
	// attached.
	preloaded map[[2]int]*worldStreamedTile
	// instances are the instanced doodads of the attached tiles.
	instances *worldInstances
	// objectSolids are the collision of the game objects standing in the world,
	// by guid, and objectsChanged says it has to be merged in again.
	objectSolids   map[uint64]worldCollisionMesh
	objectsChanged time.Time
	// viewFrustum is what the camera sees this frame, for leaving out what is not.
	viewFrustum *math32.Frustum
	lastUpdate  time.Time
}

func newWorldTerrainStreamer(loader *ui.Loader, mapName string, position world.WorldPosition, bigAlpha bool, cache *worldSceneAssetCache, root *core.Node) *worldTerrainStreamer {
	cache.setFocus(position.X, position.Y)
	return &worldTerrainStreamer{
		loader: loader, mapName: mapName, position: position, bigAlpha: bigAlpha, cache: cache, root: root, compacted: make(map[*geometry.Geometry]bool),
		collision: &worldCollisionHolder{}, liquid: cache.liquid,
		tiles: make(map[[2]int]*worldStreamedTile), inflight: make(map[[2]int]bool),
		requests: make(chan [2]int, 64), results: make(chan worldTileResult, 16),
		lastTile: [2]int{-1000, -1000},
	}
}

// worldRevealPerFrame is how many parts of a newly attached tile appear each frame.
const worldRevealPerFrame = 24

// loadTile reads, parses and builds one tile.
func (s *worldTerrainStreamer) loadTile(tileX, tileY int) (*worldStreamedTile, error) {
	path := fmt.Sprintf(`World\Maps\%s\%s_%d_%d.adt`, s.mapName, s.mapName, tileX, tileY)
	data, err := s.loader.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	adt, err := parseWorldADTWithAlpha(data, s.bigAlpha)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	centre := world.WorldPosition{Map: s.position.Map, X: (32 - float32(tileY) - 0.5) * worldTileSize, Y: (32 - float32(tileX) - 0.5) * worldTileSize}
	node, info, err := buildWorldTerrainProgressWithCache(s.loader, adt, centre, s.cache, nil)
	if err != nil {
		return nil, fmt.Errorf("build %s: %w", path, err)
	}
	tile := &worldStreamedTile{node: node, info: info}
	if collision, ok := node.UserData().(worldSceneCollision); ok {
		tile.collision = collision
	}
	return tile, nil
}

// takePreloaded gives a tile built before the world was entered, once.
func (s *worldTerrainStreamer) takePreloaded(key [2]int) *worldStreamedTile {
	tile := s.preloaded[key]
	delete(s.preloaded, key)
	return tile
}

// adopt attaches an already built tile to the scene.
// adoptNow attaches a tile that is shown at once (the ones around the player
// when the world loads, behind the loading screen).
func (s *worldTerrainStreamer) adoptNow(key [2]int, tile *worldStreamedTile) {
	s.root.Add(tile.node)
	s.tiles[key] = tile
	s.attachInstances(key, tile, false)
}

// attachInstances hands the instanced doodads of a tile to the streamer; their
// parts first appear at the pace of the reveal queue unless the tile is shown at once.
func (s *worldTerrainStreamer) attachInstances(key [2]int, tile *worldStreamedTile, gradual bool) {
	if len(tile.collision.instances) == 0 {
		return
	}
	if s.instances == nil {
		s.instances = newWorldInstances(s.root)
	}
	var reveal func(*worldInstanceSet)
	if gradual {
		first := tile.collision.instances[0].placement
		reveal = func(set *worldInstanceSet) {
			s.reveal = append(s.reveal, worldRevealItem{node: set.mesh, cx: first.centre[0], cy: first.centre[1], ready: &set.ready})
		}
	}
	s.instances.add(key, tile.collision.instances, reveal)
}

// updateInstances fills the instance buffers for the frame.
func (s *worldTerrainStreamer) updateInstances(camera [3]float32, reach float32) int {
	if s == nil || s.instances == nil {
		return 0
	}
	return s.instances.update(camera, s.viewFrustum, reach)
}

func (s *worldTerrainStreamer) adopt(key [2]int, tile *worldStreamedTile) {
	var hide func(node core.INode)
	hide = func(node core.INode) {
		if _, drawable := node.(graphic.IGraphic); drawable {
			node.GetNode().SetVisible(false)
			s.reveal = append(s.reveal, newWorldRevealItem(node))
		}
		for _, child := range node.GetNode().Children() {
			hide(child)
		}
	}
	hide(tile.node)
	s.root.Add(tile.node)
	s.tiles[key] = tile
	s.attachInstances(key, tile, true)
	// What is nearest the player appears first.
	focusX, focusY := s.cache.getFocus()
	sort.SliceStable(s.reveal, func(i, j int) bool {
		return s.reveal[i].distance(focusX, focusY) < s.reveal[j].distance(focusX, focusY)
	})
}

// setObjectSolid gives a game object collision (or takes it away with none).
func (s *worldTerrainStreamer) setObjectSolid(guid uint64, mesh worldCollisionMesh) {
	if s == nil {
		return
	}
	if s.objectSolids == nil {
		s.objectSolids = make(map[uint64]worldCollisionMesh)
	}
	if len(mesh.triangles) == 0 {
		if _, had := s.objectSolids[guid]; !had {
			return
		}
		delete(s.objectSolids, guid)
	} else {
		s.objectSolids[guid] = mesh
	}
	s.objectsChanged = time.Now()
}

// rebuildCollision merges the loaded tiles' collision in the background and
// swaps it in when ready.
func (s *worldTerrainStreamer) rebuildCollision(synchronous bool) {
	var chunks []worldADTChunk
	var solids []worldCollisionMesh
	// A placement on a tile edge is in both tiles' lists but is one thing.
	seen := make(map[*worldCollisionTriangle]bool)
	seenLights := make(map[*worldPointLight]bool)
	for _, solid := range s.objectSolids {
		solids = append(solids, solid)
	}
	for _, tile := range s.tiles {
		chunks = append(chunks, tile.collision.chunks...)
		for _, solid := range tile.collision.solids {
			switch {
			case len(solid.triangles) > 0:
				if seen[&solid.triangles[0]] {
					continue
				}
				seen[&solid.triangles[0]] = true
			case len(solid.lights) > 0:
				if seenLights[&solid.lights[0]] {
					continue
				}
				seenLights[&solid.lights[0]] = true
			}
			solids = append(solids, solid)
		}
	}
	generation := s.collisionGeneration.Add(1)
	build := func() {
		collision := newWorldSceneCollision(chunks, solids)
		collision.liquid = s.liquid
		for {
			applied := s.appliedGeneration.Load()
			if generation <= applied {
				return
			}
			if s.appliedGeneration.CompareAndSwap(applied, generation) {
				s.collision.current.Store(&collision)
				return
			}
		}
	}
	if synchronous {
		build()
		return
	}
	go build()
}

func (s *worldTerrainStreamer) start() {
	go func() {
		for key := range s.requests {
			tile, err := s.loadTile(key[0], key[1])
			s.results <- worldTileResult{key: key, tile: tile, err: err}
		}
	}()
}

// stop ends the loader once the world is left.
func (s *worldTerrainStreamer) stop() {
	s.stopOnce.Do(func() { close(s.requests) })
}

// update runs once a frame on the render thread: it attaches loaded tiles,
// asks for the ones the player now needs and releases those far behind.
func (s *worldTerrainStreamer) update(x, y float32) {
	if s == nil {
		return
	}
	s.cache.setFocus(x, y)
	changed := false
	for done := false; !done; {
		select {
		case result := <-s.results:
			delete(s.inflight, result.key)
			if result.err == nil && result.tile != nil {
				if _, exists := s.tiles[result.key]; !exists {
					s.adopt(result.key, result.tile)
					changed = true
				}
			}
		default:
			done = true
		}
	}
	// The parts waiting appear at a pace the frame time can carry: when frames
	// are running long (their first draw uploads buffers) fewer are shown.
	now := time.Now()
	frame := now.Sub(s.lastUpdate)
	s.lastUpdate = now
	shown, budget, byteBudget := 0, worldRevealPerFrame, worldRevealBytes
	switch {
	case frame > 40*time.Millisecond:
		budget, byteBudget = 1, worldRevealBytes/6
	case frame > 28*time.Millisecond:
		budget, byteBudget = 3, worldRevealBytes/3
	case frame > 21*time.Millisecond:
		budget, byteBudget = 8, worldRevealBytes/2
	}
	// A part costs what it sends to the card when first drawn (buffers, textures not
	// yet there): that, more than how many appear, is what stalls a frame.
	spent := 0
	for shown < budget && len(s.reveal) > 0 && (shown == 0 || spent < byteBudget) {
		item := s.reveal[0]
		item.node.GetNode().SetVisible(true)
		if item.ready != nil {
			*item.ready = true
		}
		spent += item.upload()
		s.reveal = s.reveal[1:]
		shown++
	}
	tileX, tileY := worldTileAt(x, y)
	// Tiles built before the world was entered that the player now needs are
	// attached at once.
	for key, tile := range s.preloaded {
		if int(math.Abs(float64(key[0]-tileX))) <= worldStreamRadius && int(math.Abs(float64(key[1]-tileY))) <= worldStreamRadius {
			if _, exists := s.tiles[key]; !exists {
				s.adopt(key, tile)
				changed = true
			}
			delete(s.preloaded, key)
		}
	}
	if [2]int{tileX, tileY} != s.lastTile {
		s.lastTile = [2]int{tileX, tileY}
		s.requestNeighbours(x, y, tileX, tileY)
	}
	for key, tile := range s.tiles {
		if int(math.Abs(float64(key[0]-tileX))) > worldStreamRadius+1 || int(math.Abs(float64(key[1]-tileY))) > worldStreamRadius+1 {
			s.release(key, tile)
			changed = true
		}
	}
	if changed {
		s.rebuildCollision(false)
		s.objectsChanged = time.Time{}
	} else if !s.objectsChanged.IsZero() && now.Sub(s.objectsChanged) > 300*time.Millisecond {
		// Objects arrive in bursts; they are merged in once the burst is over.
		s.objectsChanged = time.Time{}
		s.rebuildCollision(false)
	}
	s.updateParticles(x, y, math.Min(frame.Seconds(), 0.1))
	if now.Sub(s.compactedAt) > 2*time.Second {
		s.compactedAt = now
		s.compact()
	}
}

// release takes a tile out of the scene and frees what only it used. Textures
// and models shared through the asset cache stay for the tiles that need them.
func (s *worldTerrainStreamer) release(key [2]int, tile *worldStreamedTile) {
	delete(s.tiles, key)
	s.root.Remove(tile.node)
	if s.instances != nil {
		s.instances.remove(key)
	}
	meshes := make(map[*graphic.Mesh]bool)
	var walk func(node core.INode)
	walk = func(node core.INode) {
		if mesh, ok := node.(*graphic.Mesh); ok {
			meshes[mesh] = true
			if geometry := mesh.GetGeometry(); geometry != nil {
				delete(s.compacted, geometry)
				geometry.Dispose()
			}
		}
		for _, child := range node.GetNode().Children() {
			walk(child)
		}
	}
	walk(tile.node)
	s.liquid.drop(meshes)
	for _, tex := range tile.collision.owned {
		tex.Dispose()
	}
	s.cache.releaseWMO(tile.collision.wmoInstances)
}

// requestNeighbours asks for the tiles around the player that are neither
// loaded nor on their way, the one nearest the player first.
func (s *worldTerrainStreamer) requestNeighbours(x, y float32, tileX, tileY int) {
	type wanted struct {
		key      [2]int
		distance float64
	}
	var missing []wanted
	for dy := -worldStreamRadius; dy <= worldStreamRadius; dy++ {
		for dx := -worldStreamRadius; dx <= worldStreamRadius; dx++ {
			key := [2]int{tileX + dx, tileY + dy}
			if key[0] < 0 || key[0] >= 64 || key[1] < 0 || key[1] >= 64 {
				continue
			}
			if _, loaded := s.tiles[key]; loaded || s.inflight[key] || s.preloaded[key] != nil {
				continue
			}
			centreX := (32 - float64(key[1]) - 0.5) * worldTileSize
			centreY := (32 - float64(key[0]) - 0.5) * worldTileSize
			missing = append(missing, wanted{key, math.Hypot(centreX-float64(x), centreY-float64(y))})
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].distance < missing[j].distance })
	for _, entry := range missing {
		select {
		case s.requests <- entry.key:
			s.inflight[entry.key] = true
		default:
			return
		}
	}
}

// worldRevealBytes is how much a frame may send to the card for newly arrived
// parts of the map.
const worldRevealBytes = 3 << 19

// worldRevealItem is a hidden part of a tile waiting to appear.
type worldRevealItem struct {
	node   core.INode
	cx, cy float32
	// ready, when set, is raised once the part is shown.
	ready *bool
}

func newWorldRevealItem(node core.INode) worldRevealItem {
	item := worldRevealItem{node: node}
	if drawable, ok := node.(graphic.IGraphic); ok {
		box := drawable.GetGeometry().BoundingBox()
		at := node.GetNode().Position()
		item.cx, item.cy = at.X+(box.Min.X+box.Max.X)/2, at.Y+(box.Min.Y+box.Max.Y)/2
	}
	return item
}

func (item worldRevealItem) distance(x, y float32) float32 {
	dx, dy := item.cx-x, item.cy-y
	return dx*dx + dy*dy
}

// upload is how many bytes drawing the part the first time sends to the card.
func (item worldRevealItem) upload() int {
	drawable, ok := item.node.(graphic.IGraphic)
	if !ok {
		return 0
	}
	bytes := 0
	geom := drawable.GetGeometry()
	if !geometryUploaded(geom) {
		for _, vbo := range geom.VBOs() {
			bytes += vbo.Buffer().Bytes()
		}
		bytes += len(geom.Indices()) * 4
	}
	for _, material := range drawable.GetGraphic().Materials() {
		for _, tex := range materialTextures(material.IMaterial()) {
			if tex != nil && !textureUploaded(tex) {
				bytes += textureDataBytes(tex)
			}
		}
	}
	return bytes
}

// textureDataBytes is the size of a texture's pixels held for upload.
func textureDataBytes(tex *texture.Texture2D) int {
	data := unexportedField(tex, "data")
	if data.IsNil() {
		return 0
	}
	switch pixels := data.Interface().(type) {
	case []byte:
		return len(pixels)
	}
	return 256 << 10
}
