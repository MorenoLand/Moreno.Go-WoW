package render

import (
	"encoding/binary"
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/math32"
)

// worldM2Collision is the collision mesh a model carries: triangles over its
// own vertices, in the model's space.
type worldM2Collision struct {
	vertices [][3]float32
	indices  []uint16
}

// M2 header fields of the bounding triangles and vertices (version 264).
const (
	m2BoundingTrianglesOffset = 0xd8
	m2BoundingVerticesOffset  = 0xe0
)

// readM2Collision reads the collision mesh of a model, nil when it has none.
func readM2Collision(data []byte) *worldM2Collision {
	if len(data) < m2BoundingVerticesOffset+8 {
		return nil
	}
	triangleCount := int(binary.LittleEndian.Uint32(data[m2BoundingTrianglesOffset:]))
	triangleOffset := int(binary.LittleEndian.Uint32(data[m2BoundingTrianglesOffset+4:]))
	vertexCount := int(binary.LittleEndian.Uint32(data[m2BoundingVerticesOffset:]))
	vertexOffset := int(binary.LittleEndian.Uint32(data[m2BoundingVerticesOffset+4:]))
	if triangleCount < 3 || vertexCount < 3 || triangleOffset <= 0 || vertexOffset <= 0 ||
		triangleOffset+triangleCount*2 > len(data) || vertexOffset+vertexCount*12 > len(data) {
		return nil
	}
	collision := &worldM2Collision{vertices: make([][3]float32, vertexCount), indices: make([]uint16, triangleCount)}
	for index := range collision.vertices {
		at := vertexOffset + index*12
		collision.vertices[index] = [3]float32{
			math.Float32frombits(binary.LittleEndian.Uint32(data[at:])),
			math.Float32frombits(binary.LittleEndian.Uint32(data[at+4:])),
			math.Float32frombits(binary.LittleEndian.Uint32(data[at+8:])),
		}
	}
	for index := range collision.indices {
		collision.indices[index] = binary.LittleEndian.Uint16(data[triangleOffset+index*2:])
		if int(collision.indices[index]) >= vertexCount {
			return nil
		}
	}
	return collision
}

// m2CollisionOf reads a model's collision mesh once.
func (cache *worldSceneAssetCache) m2CollisionOf(loader *ui.Loader, path string) *worldM2Collision {
	cache.collisionMu.Lock()
	defer cache.collisionMu.Unlock()
	if collision, ok := cache.m2Collision[path]; ok {
		return collision
	}
	var collision *worldM2Collision
	if data, err := loader.ReadFile(normalizeModelPath(path)); err == nil {
		collision = readM2Collision(data)
	}
	cache.m2Collision[path] = collision
	return collision
}

// m2CollisionTriangles places a model's collision mesh in the world.
func (cache *worldSceneAssetCache) m2CollisionTriangles(loader *ui.Loader, path string, origin [3]float32, rotation *math32.Quaternion, scale float32) []worldCollisionTriangle {
	collision := cache.m2CollisionOf(loader, path)
	if collision == nil {
		return nil
	}
	world := make([][3]float32, len(collision.vertices))
	for index, vertex := range collision.vertices {
		point := rotateWorldM2Vector(rotation, vertex)
		world[index] = [3]float32{point[0]*scale + origin[0], point[1]*scale + origin[1], point[2]*scale + origin[2]}
	}
	triangles := make([]worldCollisionTriangle, 0, len(collision.indices)/3)
	for index := 0; index+2 < len(collision.indices); index += 3 {
		if triangle, ok := worldCollisionTriangleFromPoints(world[collision.indices[index]], world[collision.indices[index+1]], world[collision.indices[index+2]], false); ok {
			triangles = append(triangles, triangle)
		}
	}
	return triangles
}
