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
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/math32"
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
	root     *core.Node

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
	reveal     []core.INode
	lastUpdate time.Time
}

func newWorldTerrainStreamer(loader *ui.Loader, mapName string, position world.WorldPosition, bigAlpha bool, cache *worldSceneAssetCache, root *core.Node) *worldTerrainStreamer {
	return &worldTerrainStreamer{
		loader: loader, mapName: mapName, position: position, bigAlpha: bigAlpha, cache: cache, root: root,
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

// adopt attaches an already built tile to the scene.
func (s *worldTerrainStreamer) adopt(key [2]int, tile *worldStreamedTile) {
	var hide func(node core.INode)
	hide = func(node core.INode) {
		if _, drawable := node.(graphic.IGraphic); drawable {
			node.GetNode().SetVisible(false)
			s.reveal = append(s.reveal, node)
		}
		for _, child := range node.GetNode().Children() {
			hide(child)
		}
	}
	hide(tile.node)
	s.root.Add(tile.node)
	s.tiles[key] = tile
}

// rebuildCollision merges the loaded tiles' collision in the background and
// swaps it in when ready.
func (s *worldTerrainStreamer) rebuildCollision(synchronous bool) {
	var chunks []worldADTChunk
	var solids []worldCollisionMesh
	for _, tile := range s.tiles {
		chunks = append(chunks, tile.collision.chunks...)
		solids = append(solids, tile.collision.solids...)
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
	shown, budget := 0, worldRevealPerFrame
	switch {
	case frame > 40*time.Millisecond:
		budget = 1
	case frame > 28*time.Millisecond:
		budget = 3
	case frame > 21*time.Millisecond:
		budget = 8
	}
	for shown < budget && len(s.reveal) > 0 {
		s.reveal[0].GetNode().SetVisible(true)
		s.reveal = s.reveal[1:]
		shown++
	}
	tileX, tileY := worldTileAt(x, y)
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
	}
}

// release takes a tile out of the scene and frees what only it used. Textures
// and models shared through the asset cache stay for the tiles that need them.
func (s *worldTerrainStreamer) release(key [2]int, tile *worldStreamedTile) {
	delete(s.tiles, key)
	s.root.Remove(tile.node)
	meshes := make(map[*graphic.Mesh]bool)
	var walk func(node core.INode)
	walk = func(node core.INode) {
		if mesh, ok := node.(*graphic.Mesh); ok {
			meshes[mesh] = true
			if geometry := mesh.GetGeometry(); geometry != nil {
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
			if _, loaded := s.tiles[key]; loaded || s.inflight[key] {
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
