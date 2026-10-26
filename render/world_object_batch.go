package render

import (
	"math"
	"sort"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/texture"
)

// Game objects that never move or change (lanterns, benches, braziers' bodies)
// are not drawn one by one: those of a map cell are merged into a few meshes,
// as the map's own doodads are, so a city's worth of them costs a few draws.

const (
	// worldObjectCell is the side of the square that shares meshes.
	worldObjectCellSize = 64
	// worldObjectRebuildsPerFrame is how many cells are re-made in one frame.
	worldObjectRebuildsPerFrame = 2
)

// worldObjectEntry is where one object stands.
type worldObjectEntry struct {
	path     string
	origin   [3]float32
	rotation *math32.Quaternion
	scale    float32
}

type worldObjectCell struct {
	key     [2]int
	entries map[uint64]worldObjectEntry
	node    *core.Node
	meshes  []*graphic.Mesh
	dirty   bool
}

// worldObjectBatcher keeps the merged meshes of the static objects around.
type worldObjectBatcher struct {
	group       *core.Node
	cells       map[[2]int]*worldObjectCell
	where       map[uint64][2]int
	parts       map[string]map[string]*m2Part
	tops        map[string]float32
	unbatchable map[string]bool
	textures    map[string]*texture.Texture2D
	placeholder *texture.Texture2D
}

func newWorldObjectBatcher(scene *core.Node) *worldObjectBatcher {
	group := core.NewNode()
	scene.Add(group)
	return &worldObjectBatcher{
		group:       group,
		cells:       make(map[[2]int]*worldObjectCell),
		where:       make(map[uint64][2]int),
		parts:       make(map[string]map[string]*m2Part),
		tops:        make(map[string]float32),
		unbatchable: make(map[string]bool),
		textures:    make(map[string]*texture.Texture2D),
		placeholder: texture.NewTexture2DFromRGBA(worldPlaceholderTexture()),
	}
}

// worldObjects is the batcher of the world being shown, nil outside it.
var worldObjects *worldObjectBatcher

// batchable reports whether a model can be merged: it does not move, change
// colour or throw particles. The answer is kept.
func (b *worldObjectBatcher) batchable(loader *ui.Loader, path string) bool {
	if b == nil || b.unbatchable[path] {
		return false
	}
	if _, ok := b.parts[path]; ok {
		return true
	}
	model, err := loadSharedM2(loader, path)
	if err != nil || len(model.sequences) == 0 || modelHasAnimation(&model) || modelHasTextureAnimation(&model) || modelHasColorAnimation(&model) || len(model.particles) > 0 {
		b.unbatchable[path] = true
		return false
	}
	parts, _, err := loadWorldM2PartsAndEmitters(loader, path)
	if err != nil || len(parts) == 0 {
		b.unbatchable[path] = true
		return false
	}
	top := float32(0)
	for _, part := range parts {
		for index := 2; index < len(part.positions); index += 3 {
			if part.positions[index] > top {
				top = part.positions[index]
			}
		}
	}
	b.parts[path] = parts
	b.tops[path] = top
	return true
}

// top is the height of a batchable model.
func (b *worldObjectBatcher) top(path string) float32 { return b.tops[path] }

// add puts an object in its cell; it shows once the cell is made.
func (b *worldObjectBatcher) add(guid uint64, entry worldObjectEntry) {
	if b == nil {
		return
	}
	b.remove(guid)
	key := [2]int{int(math.Floor(float64(entry.origin[0] / worldObjectCellSize))), int(math.Floor(float64(entry.origin[1] / worldObjectCellSize)))}
	cell := b.cells[key]
	if cell == nil {
		cell = &worldObjectCell{key: key, entries: make(map[uint64]worldObjectEntry), node: core.NewNode()}
		b.group.Add(cell.node)
		b.cells[key] = cell
	}
	cell.entries[guid] = entry
	cell.dirty = true
	b.where[guid] = key
}

// remove takes an object out.
func (b *worldObjectBatcher) remove(guid uint64) {
	if b == nil {
		return
	}
	key, ok := b.where[guid]
	if !ok {
		return
	}
	delete(b.where, guid)
	if cell := b.cells[key]; cell != nil {
		delete(cell.entries, guid)
		cell.dirty = true
	}
}

// update makes the meshes of cells that changed, a few a frame, and shows the
// cells within range of the viewer.
func (b *worldObjectBatcher) update(loader *ui.Loader, collision *worldCollisionHolder, camera [3]float32, radius float32) {
	if b == nil {
		return
	}
	rebuilt := 0
	for key, cell := range b.cells {
		if cell.dirty && rebuilt < worldObjectRebuildsPerFrame {
			b.rebuild(loader, collision, cell)
			rebuilt++
		}
		if len(cell.entries) == 0 && !cell.dirty {
			b.group.Remove(cell.node)
			delete(b.cells, key)
			continue
		}
		centreX, centreY := (float32(key[0])+0.5)*worldObjectCellSize, (float32(key[1])+0.5)*worldObjectCellSize
		dx, dy := centreX-camera[0], centreY-camera[1]
		reach := radius + worldObjectCellSize*0.75
		cell.node.SetVisible(dx*dx+dy*dy <= reach*reach)
	}
}

// rebuild makes a cell's meshes again from its objects.
func (b *worldObjectBatcher) rebuild(loader *ui.Loader, collision *worldCollisionHolder, cell *worldObjectCell) {
	cell.dirty = false
	for _, mesh := range cell.meshes {
		cell.node.Remove(mesh)
		// The meshes' textures are shared and stay; only the buffers go.
		mesh.GetGeometry().Dispose()
	}
	cell.meshes = cell.meshes[:0]
	guids := make([]uint64, 0, len(cell.entries))
	for guid := range cell.entries {
		guids = append(guids, guid)
	}
	sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
	builders := make(map[worldDoodadBatchKey]*worldM2MeshBuilder)
	for _, guid := range guids {
		entry := cell.entries[guid]
		parts := b.parts[entry.path]
		if parts == nil {
			continue
		}
		var lit worldLocalLight
		indoor := false
		if collision != nil {
			if light := collision.localLightAt(entry.origin[0], entry.origin[1], entry.origin[2]+1); light != nil {
				lit, indoor = *light, true
			}
		}
		for key, part := range parts {
			if len(part.texturePaths) == 0 || len(part.indices) == 0 || len(part.positions) == 0 {
				continue
			}
			batchKey := worldDoodadBatchKey{part: entry.path + "|" + key, indoor: indoor}
			if indoor {
				batchKey.light = lit
			}
			builder := builders[batchKey]
			if builder == nil {
				builder = &worldM2MeshBuilder{part: part, positions: math32.NewArrayF32(0, len(part.positions)), normals: math32.NewArrayF32(0, len(part.normals)), uvs: math32.NewArrayF32(0, len(part.uvs)), uvs2: math32.NewArrayF32(0, len(part.uvs2)), colors: math32.NewArrayF32(0, len(part.colors)), alphas: math32.NewArrayF32(0, len(part.alphas)), combiners: math32.NewArrayF32(0, len(part.combiners)), indices: math32.NewArrayU32(0, len(part.indices))}
				builders[batchKey] = builder
			}
			appendWorldM2Part(builder, part, entry.origin, entry.rotation, entry.scale)
		}
	}
	for batchKey, builder := range builders {
		mesh := buildWorldDoodadBatch(loader, builder, b.textures, b.placeholder)
		if mesh == nil {
			continue
		}
		if batchKey.indoor {
			lit := batchKey.light
			applyWorldLocalLight(mesh, &lit)
		}
		cell.node.Add(mesh)
		cell.meshes = append(cell.meshes, mesh)
	}
}

// compact lets go of the pixels of the textures the objects were drawn with.
func (b *worldObjectBatcher) compact() {
	if b == nil {
		return
	}
	for _, tex := range b.textures {
		dropTextureData(tex)
	}
}

// clear takes every object and mesh away.
func (b *worldObjectBatcher) clear() {
	if b == nil {
		return
	}
	for _, cell := range b.cells {
		for _, mesh := range cell.meshes {
			mesh.GetGeometry().Dispose()
		}
	}
	if parent := b.group.Parent(); parent != nil {
		parent.GetNode().Remove(b.group)
	}
	b.cells = make(map[[2]int]*worldObjectCell)
	b.where = make(map[uint64][2]int)
}
