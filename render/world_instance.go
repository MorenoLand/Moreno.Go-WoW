package render

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/math32"
)

// A map doodad that is opaque and does not move is not made into a mesh of its
// own: every placement of one part of one model, in every tile that is loaded,
// is one instanced draw. The placements live in an instance buffer that is
// filled each frame with those that are within range and in view, so the number
// of draws follows the number of kinds of surface in sight rather than the number
// of things standing around, and what is behind the player is not drawn at all.

// worldInstancePlacement is where one doodad stands.
type worldInstancePlacement struct {
	matrix [16]float32
	centre [3]float32
	radius float32
	// tag names the map object the doodad belongs to, for the ones that must know
	// whether the player stands in it; zero for the rest.
	tag float32
}

// worldTileInstance is a placement and the set it is drawn with.
type worldTileInstance struct {
	set       *worldInstanceSet
	placement worldInstancePlacement
}

// worldInstanceSet is one part of one model drawn for all its placements.
type worldInstanceSet struct {
	mesh *graphic.Mesh
	vbo  *gls.VBO
	// centre and radius bound the part in its own coordinates.
	centre [3]float32
	radius float32
	// tiles holds the placements by the tile that owns them.
	tiles map[[2]int][]worldInstancePlacement
	// ready is set once the part has been shown for the first time, which is
	// when the card first receives it.
	ready  bool
	queued bool
	// tagged sets carry the tag of each placement after its matrix.
	tagged  bool
	scratch math32.ArrayF32
}

// worldInstanceSets are the sets made so far, by model part; nil marks a part
// that cannot be instanced. Sets are made on the loading thread.
type worldInstanceSets struct {
	mu   sync.Mutex
	sets map[string]*worldInstanceSet
}

func (c *worldInstanceSets) get(key string) (*worldInstanceSet, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	set, ok := c.sets[key]
	return set, ok
}

func (c *worldInstanceSets) put(key string, set *worldInstanceSet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sets == nil {
		c.sets = make(map[string]*worldInstanceSet)
	}
	c.sets[key] = set
}

// worldInstanceVertexShader is the model vertex program reading the placement
// from the instance attributes instead of the model matrix.
func worldInstanceVertexShader() string {
	const marker = "void main() {\n"
	index := strings.Index(m2VertexShader, marker)
	head, body := m2VertexShader[:index], m2VertexShader[index+len(marker):]
	body = strings.ReplaceAll(body, "VertexPosition", "instancedPosition")
	body = strings.ReplaceAll(body, "VertexNormal", "instancedNormal")
	return head + "in vec4 InstanceCol0;\nin vec4 InstanceCol1;\nin vec4 InstanceCol2;\nin vec4 InstanceCol3;\n" + marker +
		"    mat4 placement = mat4(InstanceCol0, InstanceCol1, InstanceCol2, InstanceCol3);\n" +
		"    vec3 instancedPosition = (placement * vec4(VertexPosition, 1.0)).xyz;\n" +
		"    vec3 instancedNormal = mat3(placement) * VertexNormal;\n" + body
}

// worldPlacementMatrix is the column-major matrix of a doodad: scale, then
// rotation, then position.
func worldPlacementMatrix(origin [3]float32, rotation *math32.Quaternion, scale float32) [16]float32 {
	x, y, z, w := rotation.X, rotation.Y, rotation.Z, rotation.W
	return [16]float32{
		(1 - 2*(y*y+z*z)) * scale, 2 * (x*y + z*w) * scale, 2 * (x*z - y*w) * scale, 0,
		2 * (x*y - z*w) * scale, (1 - 2*(x*x+z*z)) * scale, 2 * (y*z + x*w) * scale, 0,
		2 * (x*z + y*w) * scale, 2 * (y*z - x*w) * scale, (1 - 2*(x*x+y*y)) * scale, 0,
		origin[0], origin[1], origin[2], 1,
	}
}

// instanceSet gives the set for a part of a model, made if it is the first time,
// or nil when the part has to be drawn as a mesh of its own (it blends, so it
// must be ordered against what is around it).
func (cache *worldSceneAssetCache) instanceSet(loader *ui.Loader, path, partKey string, part *m2Part, light *worldLocalLight) *worldInstanceSet {
	key := path + "|" + partKey
	if light != nil {
		key += fmt.Sprintf("|%v", *light)
	}
	if set, ok := cache.instances.get(key); ok {
		return set
	}
	builder := &worldM2MeshBuilder{part: part, positions: math32.NewArrayF32(0, len(part.positions)), normals: math32.NewArrayF32(0, len(part.normals)), uvs: math32.NewArrayF32(0, len(part.uvs)), uvs2: math32.NewArrayF32(0, len(part.uvs2)), colors: math32.NewArrayF32(0, len(part.colors)), alphas: math32.NewArrayF32(0, len(part.alphas)), combiners: math32.NewArrayF32(0, len(part.combiners)), indices: math32.NewArrayU32(0, len(part.indices))}
	appendWorldM2Part(builder, part, [3]float32{}, &math32.Quaternion{W: 1}, 1)
	mesh := buildWorldM2BatchedMesh(loader, builder, cache.textures, cache.placeholder)
	if mesh == nil || mesh.GetMaterial(0).GetMaterial().Transparent() {
		cache.instances.put(key, nil)
		return nil
	}
	wm := mesh.GetMaterial(0).(*worldMaterial)
	if light != nil {
		lit := *light
		applyWorldLocalLight(mesh, &lit)
	}
	if wm.Shader() == "morenowow_world_m2_alpha_key" {
		wm.SetShader("morenowow_world_m2_inst_alpha_key")
	} else {
		wm.SetShader("morenowow_world_m2_inst")
	}
	set := &worldInstanceSet{mesh: mesh, tiles: make(map[[2]int][]worldInstancePlacement)}
	set.vbo = gls.NewVBO(math32.NewArrayF32(0, 16)).AddInstanceAttrib("InstanceCol0", 4).AddInstanceAttrib("InstanceCol1", 4).AddInstanceAttrib("InstanceCol2", 4).AddInstanceAttrib("InstanceCol3", 4)
	set.vbo.SetUsage(gls.DYNAMIC_DRAW)
	mesh.GetGeometry().AddVBO(set.vbo)
	mesh.SetCullable(false)
	mesh.SetVisible(false)
	box := mesh.GetGeometry().BoundingBox()
	set.centre = [3]float32{(box.Min.X + box.Max.X) / 2, (box.Min.Y + box.Max.Y) / 2, (box.Min.Z + box.Max.Z) / 2}
	set.radius = float32(math.Sqrt(float64((box.Max.X-box.Min.X)*(box.Max.X-box.Min.X)+(box.Max.Y-box.Min.Y)*(box.Max.Y-box.Min.Y)+(box.Max.Z-box.Min.Z)*(box.Max.Z-box.Min.Z)))) / 2
	cache.instances.put(key, set)
	return set
}

// newWorldTileInstance makes the placement of a doodad of a set.
func newWorldTileInstance(set *worldInstanceSet, origin [3]float32, rotation *math32.Quaternion, scale float32) worldTileInstance {
	matrix := worldPlacementMatrix(origin, rotation, scale)
	centre := [3]float32{
		matrix[0]*set.centre[0] + matrix[4]*set.centre[1] + matrix[8]*set.centre[2] + matrix[12],
		matrix[1]*set.centre[0] + matrix[5]*set.centre[1] + matrix[9]*set.centre[2] + matrix[13],
		matrix[2]*set.centre[0] + matrix[6]*set.centre[1] + matrix[10]*set.centre[2] + matrix[14],
	}
	return worldTileInstance{set: set, placement: worldInstancePlacement{matrix: matrix, centre: centre, radius: set.radius * scale}}
}

// worldInstances is what a streamer keeps of the instanced doodads of its tiles.
type worldInstances struct {
	root *core.Node
	sets map[*worldInstanceSet]bool
}

func newWorldInstances(parent *core.Node) *worldInstances {
	root := core.NewNode()
	if parent != nil {
		parent.Add(root)
	}
	return &worldInstances{root: root, sets: make(map[*worldInstanceSet]bool)}
}

// add takes the doodads of a tile that has been attached.
func (w *worldInstances) add(key [2]int, instances []worldTileInstance, reveal func(*worldInstanceSet)) {
	for _, instance := range instances {
		set := instance.set
		if !w.sets[set] {
			w.sets[set] = true
			if set.mesh.Parent() == nil {
				w.root.Add(set.mesh)
			}
		}
		set.tiles[key] = append(set.tiles[key], instance.placement)
		if !set.queued {
			set.queued = true
			if reveal != nil {
				reveal(set)
			} else {
				set.ready = true
			}
		}
	}
}

// remove takes the doodads of a tile that has gone.
func (w *worldInstances) remove(key [2]int) {
	for set := range w.sets {
		delete(set.tiles, key)
	}
}

// update fills every instance buffer with the doodads in range and in view.
func (w *worldInstances) update(camera [3]float32, frustum *math32.Frustum, reach float32) int {
	drawn := 0
	var sphere math32.Sphere
	for set := range w.sets {
		buffer := set.scratch[:0]
		for _, placements := range set.tiles {
			for index := range placements {
				placement := &placements[index]
				dx, dy := placement.centre[0]-camera[0], placement.centre[1]-camera[1]
				limit := reach + placement.radius
				if dx*dx+dy*dy > limit*limit {
					continue
				}
				if frustum != nil {
					sphere.Center.Set(placement.centre[0], placement.centre[1], placement.centre[2])
					sphere.Radius = placement.radius
					if !frustum.IntersectsSphere(&sphere) {
						continue
					}
				}
				buffer = append(buffer, placement.matrix[:]...)
				if set.tagged {
					buffer = append(buffer, placement.tag)
				}
			}
		}
		set.scratch = buffer
		count := len(buffer) / 16
		if set.tagged {
			count = len(buffer) / 17
		}
		drawn += count
		if count == 0 || !set.ready {
			set.mesh.SetVisible(false)
			set.mesh.SetInstances(0)
			continue
		}
		set.vbo.SetBuffer(buffer)
		set.mesh.SetInstances(count)
		set.mesh.SetVisible(true)
	}
	return drawn
}

// worldWMOInstanceVertexShader is the map object vertex program reading the
// placement and the tag of the map object from the instance attributes.
func worldWMOInstanceVertexShader() string {
	const marker = "void main() {\n"
	index := strings.Index(worldWMOVertexShader, marker)
	head, body := worldWMOVertexShader[:index], worldWMOVertexShader[index+len(marker):]
	body = strings.ReplaceAll(body, "VertexPosition", "instancedPosition")
	body = strings.ReplaceAll(body, "FragNormal = VertexNormal;", "FragNormal = mat3(placement) * VertexNormal;")
	body = strings.ReplaceAll(body, "FragInstanceTag = 0.0;", "FragInstanceTag = InstanceTag;")
	return head + "invariant gl_Position;\nin vec4 InstanceCol0;\nin vec4 InstanceCol1;\nin vec4 InstanceCol2;\nin vec4 InstanceCol3;\nin float InstanceTag;\n" + marker +
		"    mat4 placement = mat4(InstanceCol0, InstanceCol1, InstanceCol2, InstanceCol3);\n" +
		"    vec3 instancedPosition = (placement * vec4(VertexPosition, 1.0)).xyz;\n" + body
}

// worldWMOPlacementMatrix is the matrix a map object's placement applies to its
// own coordinates.
func worldWMOPlacementMatrix(placement worldWMOPlacement) [16]float32 {
	transform := newWorldWMOTransform(placement)
	x := transform.rotate([3]float32{transform.scale, 0, 0})
	y := transform.rotate([3]float32{0, transform.scale, 0})
	z := transform.rotate([3]float32{0, 0, transform.scale})
	return [16]float32{
		x[0], x[1], x[2], 0,
		y[0], y[1], y[2], 0,
		z[0], z[1], z[2], 0,
		transform.origin[0], transform.origin[1], transform.origin[2], 1,
	}
}

// wmoInstanceSet makes the instanced set of one surface of a map object from the
// mesh built for its first placement, which is turned back into the model's own
// coordinates, or gives the set made before; nil when the surface blends.
func (cache *worldSceneAssetCache) wmoInstanceSet(key string, mesh *graphic.Mesh, matrix [16]float32) *worldInstanceSet {
	if set, ok := cache.instances.get(key); ok {
		return set
	}
	wm, ok := mesh.GetMaterial(0).(*worldMaterial)
	if !ok || wm.Transparent() || (wm.Shader() != "morenowow_world_wmo" && wm.Shader() != "morenowow_world_wmo_alpha_key") {
		cache.instances.put(key, nil)
		return nil
	}
	scale2 := matrix[0]*matrix[0] + matrix[1]*matrix[1] + matrix[2]*matrix[2]
	if scale2 < 1e-12 {
		cache.instances.put(key, nil)
		return nil
	}
	// The placement is a turn and one scale, so its inverse is its transpose over
	// the square of the scale.
	local := func(point [3]float32) [3]float32 {
		d := [3]float32{point[0] - matrix[12], point[1] - matrix[13], point[2] - matrix[14]}
		return [3]float32{
			(matrix[0]*d[0] + matrix[1]*d[1] + matrix[2]*d[2]) / scale2,
			(matrix[4]*d[0] + matrix[5]*d[1] + matrix[6]*d[2]) / scale2,
			(matrix[8]*d[0] + matrix[9]*d[1] + matrix[10]*d[2]) / scale2,
		}
	}
	source := mesh.GetGeometry()
	geom := geometry.NewGeometry()
	geom.SetIndices(append(math32.ArrayU32(nil), source.Indices()...))
	for _, vbo := range source.VBOs() {
		buffer := append(math32.ArrayF32(nil), *vbo.Buffer()...)
		switch {
		case vbo.Attrib(gls.VertexPosition) != nil:
			for at := 0; at+2 < len(buffer); at += 3 {
				p := local([3]float32{buffer[at], buffer[at+1], buffer[at+2]})
				buffer[at], buffer[at+1], buffer[at+2] = p[0], p[1], p[2]
			}
		case vbo.Attrib(gls.VertexNormal) != nil:
			for at := 0; at+2 < len(buffer); at += 3 {
				n := [3]float32{
					(matrix[0]*buffer[at] + matrix[1]*buffer[at+1] + matrix[2]*buffer[at+2]) / scale2,
					(matrix[4]*buffer[at] + matrix[5]*buffer[at+1] + matrix[6]*buffer[at+2]) / scale2,
					(matrix[8]*buffer[at] + matrix[9]*buffer[at+1] + matrix[10]*buffer[at+2]) / scale2,
				}
				buffer[at], buffer[at+1], buffer[at+2] = n[0], n[1], n[2]
			}
		}
		out := gls.NewVBO(buffer)
		for _, attrib := range vbo.Attributes() {
			if attrib.Type != gls.Undefined {
				out.AddAttrib(attrib.Type)
			} else {
				out.AddCustomAttrib(attrib.Name, attrib.NumElements)
			}
		}
		geom.AddVBO(out)
	}
	set := &worldInstanceSet{tiles: make(map[[2]int][]worldInstancePlacement), tagged: true}
	set.vbo = gls.NewVBO(math32.NewArrayF32(0, 17)).AddInstanceAttrib("InstanceCol0", 4).AddInstanceAttrib("InstanceCol1", 4).AddInstanceAttrib("InstanceCol2", 4).AddInstanceAttrib("InstanceCol3", 4).AddInstanceAttrib("InstanceTag", 1)
	set.vbo.SetUsage(gls.DYNAMIC_DRAW)
	geom.AddVBO(set.vbo)
	wm.instance = 0
	wm.wmoInstanced = true
	if wm.Shader() == "morenowow_world_wmo_alpha_key" {
		wm.SetShader("morenowow_world_wmo_inst_alpha_key")
	} else {
		wm.SetShader("morenowow_world_wmo_inst")
	}
	set.mesh = graphic.NewMesh(geom, wm)
	set.mesh.SetRenderOrder(mesh.RenderOrder())
	set.mesh.SetCullable(false)
	set.mesh.SetVisible(false)
	box := geom.BoundingBox()
	set.centre = [3]float32{(box.Min.X + box.Max.X) / 2, (box.Min.Y + box.Max.Y) / 2, (box.Min.Z + box.Max.Z) / 2}
	dx, dy, dz := box.Max.X-box.Min.X, box.Max.Y-box.Min.Y, box.Max.Z-box.Min.Z
	set.radius = float32(math.Sqrt(float64(dx*dx+dy*dy+dz*dz))) / 2
	cache.instances.put(key, set)
	return set
}

// newWorldWMOTileInstance is the placement of a map object's surface.
func newWorldWMOTileInstance(set *worldInstanceSet, matrix [16]float32, tag float32) worldTileInstance {
	scale := float32(math.Sqrt(float64(matrix[0]*matrix[0] + matrix[1]*matrix[1] + matrix[2]*matrix[2])))
	centre := [3]float32{
		matrix[0]*set.centre[0] + matrix[4]*set.centre[1] + matrix[8]*set.centre[2] + matrix[12],
		matrix[1]*set.centre[0] + matrix[5]*set.centre[1] + matrix[9]*set.centre[2] + matrix[13],
		matrix[2]*set.centre[0] + matrix[6]*set.centre[1] + matrix[10]*set.centre[2] + matrix[14],
	}
	return worldTileInstance{set: set, placement: worldInstancePlacement{matrix: matrix, centre: centre, radius: set.radius * scale, tag: tag}}
}
