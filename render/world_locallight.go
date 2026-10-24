package render

import (
	"math"
	"sort"
	"time"

	"github.com/g3n/engine/core"
	"github.com/g3n/engine/graphic"
)

// worldLocalLightCount is how many omni lights reach one object.
const worldLocalLightCount = 4

// worldLocalLight is the light on an object inside a WMO: the WMO's own ambient
// and the nearest of its omni lights replace the sun and sky.
type worldLocalLight struct {
	ambient [3]float32
	lights  [worldLocalLightCount]worldPointLight
	count   int
	// exterior keeps the sun and sky and only adds the lights.
	exterior bool
}

// nearestWorldLights picks the lights that reach position, nearest first.
func nearestWorldLights(lights []worldPointLight, position [3]float32, ambient [3]float32) *worldLocalLight {
	type candidate struct {
		light    worldPointLight
		distance float64
	}
	var near []candidate
	for _, light := range lights {
		dx, dy, dz := float64(light.position[0]-position[0]), float64(light.position[1]-position[1]), float64(light.position[2]-position[2])
		distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if distance < float64(light.end) {
			near = append(near, candidate{light, distance})
		}
	}
	sort.Slice(near, func(i, j int) bool { return near[i].distance < near[j].distance })
	result := &worldLocalLight{ambient: ambient}
	for _, candidate := range near {
		if result.count == worldLocalLightCount {
			break
		}
		result.lights[result.count] = candidate.light
		result.count++
	}
	return result
}

// applyWorldLocalLight lights every surface under node with light (nil gives
// them the sun and sky again).
func applyWorldLocalLight(node core.INode, light *worldLocalLight) {
	if mesh, ok := node.(*graphic.Mesh); ok {
		if material, ok := mesh.GetMaterial(0).(*worldMaterial); ok {
			material.local = light
		}
	}
	for _, child := range node.GetNode().Children() {
		applyWorldLocalLight(child, light)
	}
}

// localLightAt is the light at a point of the world: the group of a WMO it
// stands in when that group is lit from inside, nil outdoors.
func (collision worldSceneCollision) localLightAt(x, y, z float32) *worldLocalLight {
	cell := [2]int{int(math.Floor(float64(x / worldCollisionCell))), int(math.Floor(float64(y / worldCollisionCell)))}
	best := float32(-math.MaxFloat32)
	indoor, inside := false, false
	var ambient [3]float32
	var groupLights []worldPointLight
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
		best, inside, indoor, ambient, groupLights = height, true, mesh.indoor, mesh.ambient, mesh.groupLights
		if triangle.hasColours {
			ambient = triangleColourAt(*triangle, x, y)
		}
	}
	if !inside {
		return nil
	}
	if !indoor {
		// A group lit by the sun still has the map object's omni lights (the
		// braziers of a city) light what stands near them, over the sun.
		lights := collision.nearbyLights(cell)
		if len(lights) == 0 {
			return nil
		}
		light := nearestWorldLights(lights, [3]float32{x, y, z + 1}, [3]float32{})
		if light.count == 0 {
			return nil
		}
		light.exterior = true
		return light
	}
	// An object inside a map object takes the light's ambient colour as its own
	// (Wow.exe FUN_00743c70 copies the Light.dbc ambient into the object). That
	// alone leaves what stands on a warm baked floor blue and dark, so half of it
	// goes to what the floor itself shows: its baked colour plus the half of the
	// ambient that its surfaces add inside the map object.
	if worldAtmo.valid {
		for channel := range ambient {
			ambient[channel] += 0.5 * worldAtmo.ambient[channel]
			if ambient[channel] > 1 {
				ambient[channel] = 1
			}
		}
	}
	// The group's own lights (MOLR) light what stands in it; a group without a
	// list falls back on the lights that reach the spot.
	lights := append(append([]worldPointLight(nil), groupLights...), collision.nearbyLights(cell)...)
	return nearestWorldLights(lights, [3]float32{x, y, z + 1}, ambient)
}

// nearbyLights are the map object omni lights indexed in a cell and the cells
// around it, which reach across the cell edges.
func (collision worldSceneCollision) nearbyLights(cell [2]int) []worldPointLight {
	var lights []worldPointLight
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			lights = append(lights, collision.lightCells[[2]int{cell[0] + dx, cell[1] + dy}]...)
		}
	}
	return lights
}

// instanceAt is the WMO placement whose floor is under a point, or 0.
func (collision worldSceneCollision) instanceAt(x, y, z float32) int {
	cell := [2]int{int(math.Floor(float64(x / worldCollisionCell))), int(math.Floor(float64(y / worldCollisionCell)))}
	best := float32(-math.MaxFloat32)
	instance := 0
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
		best, instance = height, mesh.instance
	}
	return instance
}

func (h *worldCollisionHolder) instanceAt(x, y, z float32) int {
	if c := h.current.Load(); c != nil {
		return c.instanceAt(x, y, z)
	}
	return 0
}

func (h *worldCollisionHolder) localLightAt(x, y, z float32) *worldLocalLight {
	if c := h.current.Load(); c != nil {
		return c.localLightAt(x, y, z)
	}
	return nil
}

// worldLightState remembers whether an object is lit from inside a WMO and
// when that was last decided.
type worldLightState struct {
	checkedAt time.Time
	indoor    bool
}

// worldLightChecksLeft is how many objects may look for their light this frame.
var worldLightChecksLeft int

// updateWorldLocalLight lights a moving object by the group of the WMO it
// stands in, looking again every so often.
func updateWorldLocalLight(node *core.Node, state *worldLightState, collision *worldCollisionHolder, now time.Time) {
	if node == nil || collision == nil || now.Sub(state.checkedAt) < 150*time.Millisecond {
		return
	}
	// A crowd's checks are spread over frames.
	if worldLightChecksLeft <= 0 {
		return
	}
	worldLightChecksLeft--
	state.checkedAt = now
	position := node.Position()
	light := collision.localLightAt(position.X, position.Y, position.Z)
	if light == nil && !state.indoor {
		return
	}
	applyWorldLocalLight(node, light)
	state.indoor = light != nil
}

// triangleColourAt is the baked colour of a triangle at a point over it.
func triangleColourAt(triangle worldCollisionTriangle, x, y float32) [3]float32 {
	ax, ay := triangle.a[0], triangle.a[1]
	v0x, v0y := triangle.b[0]-ax, triangle.b[1]-ay
	v1x, v1y := triangle.c[0]-ax, triangle.c[1]-ay
	v2x, v2y := x-ax, y-ay
	denominator := v0x*v1y - v1x*v0y
	if denominator == 0 {
		return [3]float32{triangle.colours[0], triangle.colours[1], triangle.colours[2]}
	}
	v := (v2x*v1y - v1x*v2y) / denominator
	w := (v0x*v2y - v2x*v0y) / denominator
	u := 1 - v - w
	var colour [3]float32
	for channel := 0; channel < 3; channel++ {
		colour[channel] = u*triangle.colours[channel] + v*triangle.colours[3+channel] + w*triangle.colours[6+channel]
	}
	return colour
}
