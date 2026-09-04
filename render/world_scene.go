package render

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/texture"
)

const (
	worldTileSize        = 1600.0 / 3.0
	worldChunkSize       = worldTileSize / 16.0
	worldUnitSize        = worldChunkSize / 8.0
	worldHeightCount     = 145
	worldStreamRadius    = 1
	worldCollisionCell   = 32.0
	worldFloorNormalZ    = 0.5
	worldBodyRadius      = 0.55
	worldBodyHeight      = 2.0
	worldStepHeight      = 0.8
	worldMaxGroundSnap   = 5.0
	worldObjectDistance  = worldTileSize * 1.5
	worldWMODoodadBudget = 512
)

type worldADT struct {
	version       uint32
	bigAlpha      bool
	textures      []string
	m2Names       []string
	m2Placements  []worldM2Placement
	wmoNames      []string
	wmoPlacements []worldWMOPlacement
	chunks        []worldADTChunk
}

type worldADTChunk struct {
	gridX     int
	gridY     int
	flags     uint32
	holes     uint16
	position  [3]float32
	heights   [worldHeightCount]float32
	texture   int
	layers    []worldADTLayer
	alphaMaps [][]byte
}

type worldADTLayer struct {
	texture     int
	flags       uint32
	alphaOffset uint32
}

type worldWMOPlacement struct {
	path      string
	position  [3]float32
	rotation  [3]float32
	lower     [3]float32
	upper     [3]float32
	doodadSet uint16
	scale     float32
}

type worldM2Placement struct {
	path     string
	position [3]float32
	rotation [3]float32
	scale    float32
	flags    uint16
}

type worldWMOMaterial struct {
	flags   uint32
	blend   uint32
	texture string
}

type worldWMORoot struct {
	groupCount int
	materials  []worldWMOMaterial
	doodadSets []worldWMODoodadSet
	doodads    []worldWMODoodad
}

type worldWMODoodadSet struct {
	start uint32
	count uint32
}

type worldWMODoodad struct {
	path     string
	position [3]float32
	rotation [4]float32
	scale    float32
}

type worldWMOBatch struct {
	start    int
	count    int
	flags    uint8
	material uint8
}

type worldWMOTriangle struct {
	flags    uint8
	material uint8
}

type worldWMOGroup struct {
	vertices  []float32
	normals   []float32
	uvs       []float32
	colors    []float32
	indices   []uint16
	batches   []worldWMOBatch
	triangles []worldWMOTriangle
}

type worldWMOMeshKey struct {
	material   worldWMOMaterial
	batchFlags uint8
}

type worldWMOMeshBuilder struct {
	positions math32.ArrayF32
	uvs       math32.ArrayF32
	colors    math32.ArrayF32
	indices   math32.ArrayU32
}

type worldSceneInfo struct {
	mapName   string
	tileX     int
	tileY     int
	tiles     int
	chunks    int
	vertices  int
	triangles int
	textures  int
	wmoMeshes int
	m2Meshes  int
}

type worldSceneCollision struct {
	chunks []worldADTChunk
	solids []worldCollisionMesh
	cells  map[[2]int][]worldCollisionRef
}

type worldCollisionRef struct {
	mesh     int
	triangle int
}

type worldCollisionTriangle struct {
	a, b, c  [3]float32
	normal   [3]float32
	min, max [3]float32
	noCamera bool
}

type worldCollisionMesh struct {
	triangles []worldCollisionTriangle
}

func worldCollisionTriangleFromPoints(a, b, c [3]float32, noCamera bool) (worldCollisionTriangle, bool) {
	ab := [3]float32{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	ac := [3]float32{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
	normal := [3]float32{ab[1]*ac[2] - ab[2]*ac[1], ab[2]*ac[0] - ab[0]*ac[2], ab[0]*ac[1] - ab[1]*ac[0]}
	length := float32(math.Sqrt(float64(normal[0]*normal[0] + normal[1]*normal[1] + normal[2]*normal[2])))
	if length < 0.000001 {
		return worldCollisionTriangle{}, false
	}
	normal[0], normal[1], normal[2] = normal[0]/length, normal[1]/length, normal[2]/length
	min := [3]float32{float32(math.Min(float64(a[0]), math.Min(float64(b[0]), float64(c[0])))), float32(math.Min(float64(a[1]), math.Min(float64(b[1]), float64(c[1])))), float32(math.Min(float64(a[2]), math.Min(float64(b[2]), float64(c[2]))))}
	max := [3]float32{float32(math.Max(float64(a[0]), math.Max(float64(b[0]), float64(c[0])))), float32(math.Max(float64(a[1]), math.Max(float64(b[1]), float64(c[1])))), float32(math.Max(float64(a[2]), math.Max(float64(b[2]), float64(c[2]))))}
	return worldCollisionTriangle{a: a, b: b, c: c, normal: normal, min: min, max: max, noCamera: noCamera}, true
}

func newWorldSceneCollision(chunks []worldADTChunk, solids []worldCollisionMesh) worldSceneCollision {
	collision := worldSceneCollision{chunks: chunks, solids: solids, cells: make(map[[2]int][]worldCollisionRef)}
	for meshIndex, mesh := range solids {
		for triangleIndex, triangle := range mesh.triangles {
			minX := int(math.Floor(float64(triangle.min[0] / worldCollisionCell)))
			maxX := int(math.Floor(float64(triangle.max[0] / worldCollisionCell)))
			minY := int(math.Floor(float64(triangle.min[1] / worldCollisionCell)))
			maxY := int(math.Floor(float64(triangle.max[1] / worldCollisionCell)))
			for x := minX; x <= maxX; x++ {
				for y := minY; y <= maxY; y++ {
					key := [2]int{x, y}
					collision.cells[key] = append(collision.cells[key], worldCollisionRef{mesh: meshIndex, triangle: triangleIndex})
				}
			}
		}
	}
	return collision
}

type worldSceneAssetCache struct {
	textures           map[string]*texture.Texture2D
	placeholder        *texture.Texture2D
	terrainPlaceholder *texture.Texture2D
	zeroAlpha          *texture.Texture2D
	opaqueAlpha        *texture.Texture2D
	wmoModels          map[string]worldWMORoot
	m2Parts            map[string]map[string]*m2Part
}

func newWorldSceneAssetCache() *worldSceneAssetCache {
	return &worldSceneAssetCache{
		textures: make(map[string]*texture.Texture2D), placeholder: texture.NewTexture2DFromRGBA(worldPlaceholderTexture()),
		terrainPlaceholder: texture.NewTexture2DFromRGBA(worldSolidTexture(color.RGBA{R: 255, G: 255, B: 255, A: 255})),
		zeroAlpha:          texture.NewTexture2DFromRGBA(worldAlphaTexture(nil, false)), opaqueAlpha: texture.NewTexture2DFromRGBA(worldAlphaTexture(nil, true)),
		wmoModels: make(map[string]worldWMORoot), m2Parts: make(map[string]map[string]*m2Part),
	}
}

func (collision worldSceneCollision) ground(x, y float32) (float32, bool) {
	height, _, ok := collision.terrain(x, y)
	return height, ok
}

func (collision worldSceneCollision) floor(x, y, reference float32) (float32, bool) {
	best, _, found := collision.terrain(x, y)
	if !found || best < reference-worldMaxGroundSnap || best > reference+worldStepHeight {
		found = false
	}
	cell := [2]int{int(math.Floor(float64(x / worldCollisionCell))), int(math.Floor(float64(y / worldCollisionCell)))}
	for _, ref := range collision.cells[cell] {
		triangle := collision.solids[ref.mesh].triangles[ref.triangle]
		if math.Abs(float64(triangle.normal[2])) < worldFloorNormalZ || !worldPointInTriangle2D([2]float32{x, y}, [2]float32{triangle.a[0], triangle.a[1]}, [2]float32{triangle.b[0], triangle.b[1]}, [2]float32{triangle.c[0], triangle.c[1]}) || triangle.normal[2] == 0 {
			continue
		}
		height := triangle.a[2] - (triangle.normal[0]*(x-triangle.a[0])+triangle.normal[1]*(y-triangle.a[1]))/triangle.normal[2]
		if height < reference-worldMaxGroundSnap || height > reference+worldStepHeight || (found && height <= best) {
			continue
		}
		best, found = height, true
	}
	return best, found
}

func (collision worldSceneCollision) terrain(x, y float32) (float32, [3]float32, bool) {
	for _, chunk := range collision.chunks {
		localX := (chunk.position[0] - x) / worldUnitSize
		localY := (chunk.position[1] - y) / worldUnitSize
		if localX < 0 || localY < 0 || localX > 8 || localY > 8 {
			continue
		}
		row, column := int(math.Floor(float64(localX))), int(math.Floor(float64(localY)))
		if row >= 8 {
			row = 7
			localX = 8
		}
		if column >= 8 {
			column = 7
			localY = 8
		}
		if chunk.holes&(1<<uint((row/2)*4+column/2)) != 0 {
			return 0, [3]float32{}, false
		}
		u, v := localX-float32(row), localY-float32(column)
		outer := func(r, c int) float32 { return chunk.heights[r*17+c] }
		h00, h01 := outer(row, column), outer(row, column+1)
		h10, h11 := outer(row+1, column), outer(row+1, column+1)
		height := h00*(1-u)*(1-v) + h10*u*(1-v) + h01*(1-u)*v + h11*u*v
		dx := ((h10-h00)*(1-v) + (h11-h01)*v) / worldUnitSize
		dy := ((h01-h00)*(1-u) + (h11-h10)*u) / worldUnitSize
		normal := [3]float32{-(-dx), -(-dy), 1}
		length := float32(math.Sqrt(float64(normal[0]*normal[0] + normal[1]*normal[1] + normal[2]*normal[2])))
		if length > 0 {
			normal[0], normal[1], normal[2] = normal[0]/length, normal[1]/length, normal[2]/length
		}
		return chunk.position[2] + height, normal, true
	}
	return 0, [3]float32{}, false
}

func (collision worldSceneCollision) move(from, to [3]float32) [3]float32 {
	deltaX, deltaY := to[0]-from[0], to[1]-from[1]
	if deltaX == 0 && deltaY == 0 {
		return to
	}
	if _, normal, ok := collision.terrain(to[0], to[1]); ok && normal[2] < worldFloorNormalZ {
		uphillX, uphillY := -normal[0], -normal[1]
		lengthSquared := uphillX*uphillX + uphillY*uphillY
		if lengthSquared > 0 {
			uphill := (deltaX*uphillX + deltaY*uphillY) / lengthSquared
			if uphill > 0 {
				deltaX -= uphillX * uphill
				deltaY -= uphillY * uphill
				if deltaX*deltaX+deltaY*deltaY < 0.000001 {
					return from
				}
			}
		}
	}
	to[0], to[1] = from[0]+deltaX, from[1]+deltaY
	for pass := 0; pass < 2; pass++ {
		correctionX, correctionY := float32(0), float32(0)
		minX, maxX := to[0]-worldBodyRadius, to[0]+worldBodyRadius
		minY, maxY := to[1]-worldBodyRadius, to[1]+worldBodyRadius
		for cellX := int(math.Floor(float64(minX / worldCollisionCell))); cellX <= int(math.Floor(float64(maxX/worldCollisionCell))); cellX++ {
			for cellY := int(math.Floor(float64(minY / worldCollisionCell))); cellY <= int(math.Floor(float64(maxY/worldCollisionCell))); cellY++ {
				for _, ref := range collision.cells[[2]int{cellX, cellY}] {
					triangle := collision.solids[ref.mesh].triangles[ref.triangle]
					if math.Abs(float64(triangle.normal[2])) >= worldFloorNormalZ || triangle.max[2] < from[2]+worldStepHeight || triangle.min[2] > from[2]+worldBodyHeight {
						continue
					}
					pushX, pushY, distance, ok := worldCollisionPush2D(to[0], to[1], triangle, worldBodyRadius)
					if !ok {
						continue
					}
					if pushX*pushX+pushY*pushY > correctionX*correctionX+correctionY*correctionY {
						correctionX, correctionY = pushX, pushY
					}
					_ = distance
				}
			}
		}
		if correctionX == 0 && correctionY == 0 {
			break
		}
		to[0] += correctionX
		to[1] += correctionY
	}
	return to
}

func worldCollisionPush2D(x, y float32, triangle worldCollisionTriangle, radius float32) (float32, float32, float32, bool) {
	p := [2]float32{x, y}
	a, b, c := [2]float32{triangle.a[0], triangle.a[1]}, [2]float32{triangle.b[0], triangle.b[1]}, [2]float32{triangle.c[0], triangle.c[1]}
	closest := p
	inside := worldPointInTriangle2D(p, a, b, c)
	if !inside {
		best := float32(math.MaxFloat32)
		for _, edge := range [][2][2]float32{{a, b}, {b, c}, {c, a}} {
			point := worldClosestSegmentPoint2D(p, edge[0], edge[1])
			dx, dy := p[0]-point[0], p[1]-point[1]
			distance := float32(math.Sqrt(float64(dx*dx + dy*dy)))
			if distance < best {
				best, closest = distance, point
			}
		}
	}
	deltaX, deltaY := p[0]-closest[0], p[1]-closest[1]
	distance := float32(math.Sqrt(float64(deltaX*deltaX + deltaY*deltaY)))
	if !inside && distance >= radius {
		return 0, 0, distance, false
	}
	if distance < 0.000001 {
		deltaX, deltaY = triangle.normal[0], triangle.normal[1]
		if (p[0]-triangle.a[0])*deltaX+(p[1]-triangle.a[1])*deltaY < 0 {
			deltaX, deltaY = -deltaX, -deltaY
		}
		distance = 0
	} else {
		deltaX, deltaY = deltaX/distance, deltaY/distance
	}
	return deltaX * (radius - distance), deltaY * (radius - distance), distance, true
}

func worldPointInTriangle2D(p, a, b, c [2]float32) bool {
	sign := func(p1, p2, p3 [2]float32) float32 { return (p1[0]-p3[0])*(p2[1]-p3[1]) - (p2[0]-p3[0])*(p1[1]-p3[1]) }
	d1, d2, d3 := sign(p, a, b), sign(p, b, c), sign(p, c, a)
	return !(d1 < 0 && (d2 >= 0 || d3 >= 0) || d1 >= 0 && (d2 < 0 || d3 < 0))
}

func worldClosestSegmentPoint2D(p, a, b [2]float32) [2]float32 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	lengthSquared := dx*dx + dy*dy
	if lengthSquared < 0.000001 {
		return a
	}
	t := ((p[0]-a[0])*dx + (p[1]-a[1])*dy) / lengthSquared
	t = float32(math.Max(0, math.Min(1, float64(t))))
	return [2]float32{a[0] + dx*t, a[1] + dy*t}
}

func worldSegmentTriangleHit(from, to [3]float32, triangle worldCollisionTriangle) (float32, bool) {
	direction := [3]float32{to[0] - from[0], to[1] - from[1], to[2] - from[2]}
	edge1 := [3]float32{triangle.b[0] - triangle.a[0], triangle.b[1] - triangle.a[1], triangle.b[2] - triangle.a[2]}
	edge2 := [3]float32{triangle.c[0] - triangle.a[0], triangle.c[1] - triangle.a[1], triangle.c[2] - triangle.a[2]}
	pvec := [3]float32{direction[1]*edge2[2] - direction[2]*edge2[1], direction[2]*edge2[0] - direction[0]*edge2[2], direction[0]*edge2[1] - direction[1]*edge2[0]}
	determinant := edge1[0]*pvec[0] + edge1[1]*pvec[1] + edge1[2]*pvec[2]
	if math.Abs(float64(determinant)) < 0.000001 {
		return 0, false
	}
	inverse := 1 / determinant
	tvec := [3]float32{from[0] - triangle.a[0], from[1] - triangle.a[1], from[2] - triangle.a[2]}
	u := (tvec[0]*pvec[0] + tvec[1]*pvec[1] + tvec[2]*pvec[2]) * inverse
	if u < 0 || u > 1 {
		return 0, false
	}
	qvec := [3]float32{tvec[1]*edge1[2] - tvec[2]*edge1[1], tvec[2]*edge1[0] - tvec[0]*edge1[2], tvec[0]*edge1[1] - tvec[1]*edge1[0]}
	v := (direction[0]*qvec[0] + direction[1]*qvec[1] + direction[2]*qvec[2]) * inverse
	if v < 0 || u+v > 1 {
		return 0, false
	}
	t := (edge2[0]*qvec[0] + edge2[1]*qvec[1] + edge2[2]*qvec[2]) * inverse
	return t, t >= 0 && t <= 1
}

func (collision worldSceneCollision) cameraPosition(focus, eye math32.Vector3) math32.Vector3 {
	spanX, spanY, spanZ := eye.X-focus.X, eye.Y-focus.Y, eye.Z-focus.Z
	clear := func(fraction float32) bool {
		x, y, z := focus.X+spanX*fraction, focus.Y+spanY*fraction, focus.Z+spanZ*fraction
		ground, _, ok := collision.terrain(x, y)
		return !ok || z >= ground+0.35
	}
	allowed := float32(1)
	for step := 1; step <= 12; step++ {
		fraction := float32(step) / 12
		if clear(fraction) {
			continue
		}
		low, high := float32(step-1)/12, fraction
		for index := 0; index < 6; index++ {
			middle := (low + high) * 0.5
			if clear(middle) {
				low = middle
			} else {
				high = middle
			}
		}
		allowed = low
		break
	}
	spanMinX, spanMaxX := float32(math.Min(float64(focus.X), float64(eye.X))), float32(math.Max(float64(focus.X), float64(eye.X)))
	spanMinY, spanMaxY := float32(math.Min(float64(focus.Y), float64(eye.Y))), float32(math.Max(float64(focus.Y), float64(eye.Y)))
	nearest := float32(1)
	spanX, spanY, spanZ = eye.X-focus.X, eye.Y-focus.Y, eye.Z-focus.Z
	for cellX := int(math.Floor(float64(spanMinX / worldCollisionCell))); cellX <= int(math.Floor(float64(spanMaxX/worldCollisionCell))); cellX++ {
		for cellY := int(math.Floor(float64(spanMinY / worldCollisionCell))); cellY <= int(math.Floor(float64(spanMaxY/worldCollisionCell))); cellY++ {
			for _, ref := range collision.cells[[2]int{cellX, cellY}] {
				triangle := collision.solids[ref.mesh].triangles[ref.triangle]
				if triangle.noCamera || math.Abs(float64(triangle.normal[2])) >= worldFloorNormalZ {
					continue
				}
				if hit, ok := worldSegmentTriangleHit([3]float32{focus.X, focus.Y, focus.Z}, [3]float32{eye.X, eye.Y, eye.Z}, triangle); ok && hit < nearest {
					nearest = hit
				}
			}
		}
	}
	if nearest < allowed {
		allowed = float32(math.Max(0, float64(nearest-0.035)))
	}
	return *math32.NewVector3(focus.X+spanX*allowed, focus.Y+spanY*allowed, focus.Z+spanZ*allowed)
}

func loadWorldTerrain(loader *ui.Loader, position world.WorldPosition) (*core.Node, worldSceneInfo, error) {
	return loadWorldTerrainProgress(loader, position, nil)
}

func loadWorldTerrainProgress(loader *ui.Loader, position world.WorldPosition, progress func(float64)) (*core.Node, worldSceneInfo, error) {
	if loader == nil {
		return nil, worldSceneInfo{}, fmt.Errorf("world terrain has no asset loader")
	}
	if progress != nil {
		progress(0.05)
	}
	mapName := loadMapName(loader, position.Map)
	if mapName == "" {
		return nil, worldSceneInfo{}, fmt.Errorf("map %d has no Map.dbc name", position.Map)
	}
	tileX, tileY := worldTileAt(position.X, position.Y)
	if tileX < 0 || tileX >= 64 || tileY < 0 || tileY >= 64 {
		return nil, worldSceneInfo{}, fmt.Errorf("world position %.3f,%.3f maps outside tile grid at %d,%d", position.X, position.Y, tileX, tileY)
	}
	bigAlpha := worldMapBigAlpha(loader, mapName)
	root := core.NewNode()
	info := worldSceneInfo{mapName: mapName, tileX: tileX, tileY: tileY}
	cache := newWorldSceneAssetCache()
	collisionChunks := make([]worldADTChunk, 0, 256*(worldStreamRadius*2+1)*(worldStreamRadius*2+1))
	collisionSolids := make([]worldCollisionMesh, 0)
	requestedTiles := (worldStreamRadius*2 + 1) * (worldStreamRadius*2 + 1)
	for offsetY := -worldStreamRadius; offsetY <= worldStreamRadius; offsetY++ {
		for offsetX := -worldStreamRadius; offsetX <= worldStreamRadius; offsetX++ {
			neighborX, neighborY := tileX+offsetX, tileY+offsetY
			if neighborX < 0 || neighborX >= 64 || neighborY < 0 || neighborY >= 64 {
				continue
			}
			path := fmt.Sprintf(`World\Maps\%s\%s_%d_%d.adt`, mapName, mapName, neighborX, neighborY)
			data, readErr := loader.ReadFile(path)
			if readErr != nil {
				if neighborX == tileX && neighborY == tileY {
					return nil, worldSceneInfo{}, fmt.Errorf("read %s: %w", path, readErr)
				}
				continue
			}
			adt, parseErr := parseWorldADTWithAlpha(data, bigAlpha)
			if parseErr != nil {
				if neighborX == tileX && neighborY == tileY {
					return nil, worldSceneInfo{}, fmt.Errorf("parse %s: %w", path, parseErr)
				}
				continue
			}
			if progress != nil {
				progress(0.1 + 0.8*float64(info.tiles)/float64(requestedTiles))
			}
			tileRoot, tileInfo, buildErr := buildWorldTerrainProgressWithCache(loader, adt, position, cache, nil)
			if buildErr != nil {
				if neighborX == tileX && neighborY == tileY {
					return nil, worldSceneInfo{}, fmt.Errorf("build %s: %w", path, buildErr)
				}
				continue
			}
			root.Add(tileRoot)
			info.tiles++
			info.chunks += tileInfo.chunks
			info.vertices += tileInfo.vertices
			info.triangles += tileInfo.triangles
			info.textures += tileInfo.textures
			info.wmoMeshes += tileInfo.wmoMeshes
			info.m2Meshes += tileInfo.m2Meshes
			collision, ok := tileRoot.UserData().(worldSceneCollision)
			if ok {
				collisionChunks = append(collisionChunks, collision.chunks...)
				collisionSolids = append(collisionSolids, collision.solids...)
			}
		}
	}
	if info.tiles == 0 || info.chunks == 0 {
		return nil, worldSceneInfo{}, fmt.Errorf("world map %s has no readable tiles near %d,%d", mapName, tileX, tileY)
	}
	if progress != nil {
		progress(1)
	}
	root.SetUserData(newWorldSceneCollision(collisionChunks, collisionSolids))
	return root, info, nil
}

func loadMapName(loader *ui.Loader, mapID uint32) string {
	if data, err := loader.ReadFile(`DBFilesClient\Map.dbc`); err == nil {
		if names := parseMapNames(data); names[mapID] != "" {
			return names[mapID]
		}
	}
	return mapFallbackName(mapID)
}

func mapFallbackName(id uint32) string {
	switch id {
	case 0:
		return "Azeroth"
	case 1:
		return "Kalimdor"
	case 530:
		return "Expansion01"
	case 571:
		return "Northrend"
	default:
		return ""
	}
}

func worldMapBigAlpha(loader *ui.Loader, mapName string) bool {
	if loader == nil || mapName == "" {
		return false
	}
	data, err := loader.ReadFile(fmt.Sprintf(`World\Maps\%s\%s.wdt`, mapName, mapName))
	if err != nil {
		return false
	}
	for offset := 0; offset < len(data); {
		id, _, payload, _, next, ok := worldChunk(data, offset)
		if !ok {
			break
		}
		if id == "MPHD" && len(payload) >= 4 {
			return binary.LittleEndian.Uint32(payload[:4])&0x4 != 0
		}
		offset = next
	}
	return false
}

func parseMapNames(data []byte) map[uint32]string {
	result := make(map[uint32]string)
	if len(data) < 20 || string(data[:4]) != "WDBC" {
		return result
	}
	records := int(binary.LittleEndian.Uint32(data[4:8]))
	fields := int(binary.LittleEndian.Uint32(data[8:12]))
	stride := int(binary.LittleEndian.Uint32(data[12:16]))
	stringSize := int(binary.LittleEndian.Uint32(data[16:20]))
	if records < 0 || fields < 2 || stride < fields*4 || stringSize < 1 || 20+records*stride < 20 || 20+records*stride+stringSize > len(data) {
		return result
	}
	stringStart := 20 + records*stride
	readString := func(offset uint32) string {
		start := int(offset)
		if start < 0 || start >= stringSize {
			return ""
		}
		end := start
		for end < stringSize && data[stringStart+end] != 0 {
			end++
		}
		return string(data[stringStart+start : stringStart+end])
	}
	for record := 0; record < records; record++ {
		base := 20 + record*stride
		name := readString(binary.LittleEndian.Uint32(data[base+4 : base+8]))
		if name != "" {
			result[binary.LittleEndian.Uint32(data[base:base+4])] = name
		}
	}
	return result
}

func worldLoadingScreenPath(loader *ui.Loader, mapID uint32) string {
	if loader != nil {
		if mapData, err := loader.ReadFile(`DBFilesClient\Map.dbc`); err == nil {
			if screenID, ok := worldMapLoadingScreenID(mapData, mapID); ok {
				if screens, readErr := loader.ReadFile(`DBFilesClient\LoadingScreens.dbc`); readErr == nil {
					if path := worldLoadingScreenRecord(screens, screenID); path != "" {
						return path
					}
				}
			}
		}
	}
	switch mapID {
	case 0, 1:
		return `Interface\Glues\LoadingScreens\LoadScreenKalimdor.blp`
	case 530:
		return `Interface\Glues\LoadingScreens\LoadScreenOutland.blp`
	case 571:
		return `Interface\Glues\LoadingScreens\LoadScreenNorthrend.blp`
	default:
		return ""
	}
}

func worldMapLoadingScreenID(data []byte, mapID uint32) (uint32, bool) {
	if len(data) < 20 || string(data[:4]) != "WDBC" {
		return 0, false
	}
	records := int(binary.LittleEndian.Uint32(data[4:8]))
	fields := int(binary.LittleEndian.Uint32(data[8:12]))
	stride := int(binary.LittleEndian.Uint32(data[12:16]))
	stringSize := int(binary.LittleEndian.Uint32(data[16:20]))
	if records < 0 || fields <= 57 || stride < fields*4 || stringSize < 1 || records > (len(data)-20-stringSize)/stride {
		return 0, false
	}
	for record := 0; record < records; record++ {
		base := 20 + record*stride
		if binary.LittleEndian.Uint32(data[base:base+4]) == mapID {
			return binary.LittleEndian.Uint32(data[base+57*4 : base+58*4]), true
		}
	}
	return 0, false
}

func worldLoadingScreenRecord(data []byte, screenID uint32) string {
	if len(data) < 20 || string(data[:4]) != "WDBC" {
		return ""
	}
	records := int(binary.LittleEndian.Uint32(data[4:8]))
	fields := int(binary.LittleEndian.Uint32(data[8:12]))
	stride := int(binary.LittleEndian.Uint32(data[12:16]))
	stringSize := int(binary.LittleEndian.Uint32(data[16:20]))
	if records < 0 || fields < 3 || stride < fields*4 || stringSize < 1 || records > (len(data)-20-stringSize)/stride {
		return ""
	}
	stringStart := 20 + records*stride
	for record := 0; record < records; record++ {
		base := 20 + record*stride
		if binary.LittleEndian.Uint32(data[base:base+4]) != screenID {
			continue
		}
		offset := binary.LittleEndian.Uint32(data[base+8 : base+12])
		path := worldStringAt(data[stringStart:stringStart+stringSize], offset)
		if path == "" {
			return ""
		}
		if !strings.Contains(path, "\\") {
			path = `Interface\Glues\LoadingScreens\` + path
		}
		if !strings.HasSuffix(strings.ToLower(path), ".blp") {
			path += ".blp"
		}
		return path
	}
	return ""
}

func worldTileAt(x, y float32) (int, int) {
	return int(math.Floor(float64(32 - y/worldTileSize))), int(math.Floor(float64(32 - x/worldTileSize)))
}

func parseWorldADT(data []byte) (worldADT, error) {
	return parseWorldADTWithAlpha(data, false)
}

func parseWorldADTWithAlpha(data []byte, bigAlpha bool) (worldADT, error) {
	result := worldADT{bigAlpha: bigAlpha}
	var main []byte
	var mmdx, mmid, mddf, mwmo, mwid, modf []byte
	for offset := 0; offset < len(data); {
		id, _, payload, _, next, ok := worldChunk(data, offset)
		if !ok {
			break
		}
		switch id {
		case "MVER":
			if len(payload) >= 4 {
				result.version = binary.LittleEndian.Uint32(payload[:4])
			}
		case "MTEX":
			result.textures = parseWorldTextureNames(payload)
		case "MMDX":
			mmdx = payload
		case "MMID":
			mmid = payload
		case "MDDF":
			mddf = payload
		case "MCIN":
			main = payload
		case "MWMO":
			mwmo = payload
		case "MWID":
			mwid = payload
		case "MODF":
			modf = payload
		}
		offset = next
	}
	if result.version != 18 {
		return worldADT{}, fmt.Errorf("unsupported version %d", result.version)
	}
	if len(main) < 256*16 {
		return worldADT{}, fmt.Errorf("MCIN is short (%d)", len(main))
	}
	result.m2Names = parseWorldOffsetNames(mmdx, mmid)
	result.m2Placements = parseWorldM2Placements(mddf, result.m2Names)
	result.wmoNames = parseWorldOffsetNames(mwmo, mwid)
	result.wmoPlacements = parseWorldWMOPlacements(modf, result.wmoNames)
	for index := 0; index < 256; index++ {
		base := index * 16
		chunkOffset := int(binary.LittleEndian.Uint32(main[base : base+4]))
		if chunkOffset == 0 {
			continue
		}
		chunk, ok := parseWorldMCNK(data, chunkOffset, index%16, index/16, result.textures, result.bigAlpha)
		if ok {
			result.chunks = append(result.chunks, chunk)
		}
	}
	if len(result.chunks) == 0 {
		return worldADT{}, fmt.Errorf("ADT has no height chunks")
	}
	return result, nil
}

func parseWorldOffsetNames(names, offsets []byte) []string {
	if len(offsets)%4 != 0 {
		return nil
	}
	result := make([]string, len(offsets)/4)
	for index := range result {
		offset := int(binary.LittleEndian.Uint32(offsets[index*4 : index*4+4]))
		if offset < 0 || offset >= len(names) {
			continue
		}
		end := offset
		for end < len(names) && names[end] != 0 {
			end++
		}
		result[index] = strings.ReplaceAll(string(names[offset:end]), "/", "\\")
	}
	return result
}

func parseWorldWMOPlacements(data []byte, names []string) []worldWMOPlacement {
	const size = 64
	result := make([]worldWMOPlacement, 0, len(data)/size)
	for offset := 0; offset+size <= len(data); offset += size {
		nameID := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		if nameID < 0 || nameID >= len(names) || names[nameID] == "" {
			continue
		}
		placement := worldWMOPlacement{path: names[nameID], scale: 1}
		for index := range placement.position {
			placement.position[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset+8+index*4 : offset+12+index*4]))
			placement.rotation[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset+20+index*4 : offset+24+index*4]))
			placement.lower[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset+32+index*4 : offset+36+index*4]))
			placement.upper[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset+44+index*4 : offset+48+index*4]))
		}
		placement.doodadSet = binary.LittleEndian.Uint16(data[offset+58 : offset+60])
		result = append(result, placement)
	}
	return result
}

func parseWorldM2Placements(data []byte, names []string) []worldM2Placement {
	const size = 36
	result := make([]worldM2Placement, 0, len(data)/size)
	for offset := 0; offset+size <= len(data); offset += size {
		nameID := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		if nameID < 0 || nameID >= len(names) || names[nameID] == "" {
			continue
		}
		placement := worldM2Placement{path: names[nameID], scale: float32(binary.LittleEndian.Uint16(data[offset+32:offset+34])) / 1024, flags: binary.LittleEndian.Uint16(data[offset+34 : offset+36])}
		if placement.scale <= 0 {
			placement.scale = 1
		}
		for index := range placement.position {
			placement.position[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset+8+index*4 : offset+12+index*4]))
			placement.rotation[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset+20+index*4 : offset+24+index*4]))
		}
		result = append(result, placement)
	}
	return result
}

func parseWorldWMORoot(data []byte) (worldWMORoot, error) {
	var version uint32
	var mohd, motx, momt, modn, mods, modd []byte
	for offset := 0; offset < len(data); {
		id, _, payload, _, next, ok := worldChunk(data, offset)
		if !ok {
			break
		}
		switch id {
		case "MVER":
			if len(payload) >= 4 {
				version = binary.LittleEndian.Uint32(payload[:4])
			}
		case "MOHD":
			mohd = payload
		case "MOTX":
			motx = payload
		case "MOMT":
			momt = payload
		case "MODN":
			modn = payload
		case "MODS":
			mods = payload
		case "MODD":
			modd = payload
		}
		offset = next
	}
	if version != 17 {
		return worldWMORoot{}, fmt.Errorf("unsupported version %d", version)
	}
	if len(mohd) < 8 {
		return worldWMORoot{}, fmt.Errorf("MOHD is short")
	}
	root := worldWMORoot{groupCount: int(binary.LittleEndian.Uint32(mohd[4:8]))}
	for offset := 0; offset+64 <= len(momt); offset += 64 {
		textureOffset := binary.LittleEndian.Uint32(momt[offset+12 : offset+16])
		root.materials = append(root.materials, worldWMOMaterial{
			flags:   binary.LittleEndian.Uint32(momt[offset : offset+4]),
			blend:   binary.LittleEndian.Uint32(momt[offset+8 : offset+12]),
			texture: worldStringAt(motx, textureOffset),
		})
	}
	for offset := 0; offset+32 <= len(mods); offset += 32 {
		root.doodadSets = append(root.doodadSets, worldWMODoodadSet{start: binary.LittleEndian.Uint32(mods[offset+20 : offset+24]), count: binary.LittleEndian.Uint32(mods[offset+24 : offset+28])})
	}
	for offset := 0; offset+40 <= len(modd); offset += 40 {
		nameOffset := binary.LittleEndian.Uint32(modd[offset:]) & 0x00ffffff
		doodad := worldWMODoodad{path: worldStringAt(modn, nameOffset), scale: math.Float32frombits(binary.LittleEndian.Uint32(modd[offset+32 : offset+36]))}
		for index := range doodad.position {
			doodad.position[index] = math.Float32frombits(binary.LittleEndian.Uint32(modd[offset+4+index*4 : offset+8+index*4]))
		}
		for index := range doodad.rotation {
			doodad.rotation[index] = math.Float32frombits(binary.LittleEndian.Uint32(modd[offset+16+index*4 : offset+20+index*4]))
		}
		if doodad.scale <= 0 || math.IsNaN(float64(doodad.scale)) || math.IsInf(float64(doodad.scale), 0) {
			doodad.scale = 1
		}
		if doodad.path != "" {
			root.doodads = append(root.doodads, doodad)
		}
	}
	if root.groupCount < 1 {
		return worldWMORoot{}, fmt.Errorf("WMO has no groups")
	}
	return root, nil
}

func worldStringAt(data []byte, offset uint32) string {
	start := int(offset)
	if start < 0 || start >= len(data) {
		return ""
	}
	end := start
	for end < len(data) && data[end] != 0 {
		end++
	}
	return strings.ReplaceAll(string(data[start:end]), "/", "\\")
}

func parseWorldWMOGroup(data []byte) (worldWMOGroup, error) {
	var mogp []byte
	for offset := 0; offset < len(data); {
		id, _, payload, _, next, ok := worldChunk(data, offset)
		if !ok {
			break
		}
		if id == "MOGP" {
			mogp = payload
			break
		}
		offset = next
	}
	if len(mogp) < 68 {
		return worldWMOGroup{}, fmt.Errorf("MOGP is short")
	}
	group := worldWMOGroup{}
	for offset := 68; offset < len(mogp); {
		id, _, payload, _, next, ok := worldChunk(mogp, offset)
		if !ok {
			break
		}
		switch id {
		case "MOVT":
			group.vertices = append(group.vertices, worldFloat32s(payload, 3)...)
		case "MONR":
			group.normals = append(group.normals, worldFloat32s(payload, 3)...)
		case "MOTV":
			group.uvs = append(group.uvs, worldFloat32s(payload, 2)...)
		case "MOCV":
			for index := 0; index+3 < len(payload); index += 4 {
				group.colors = append(group.colors, float32(payload[index+2])/255, float32(payload[index+1])/255, float32(payload[index])/255)
			}
		case "MOVI":
			for index := 0; index+2 <= len(payload); index += 2 {
				group.indices = append(group.indices, binary.LittleEndian.Uint16(payload[index:index+2]))
			}
		case "MOPY":
			for index := 0; index+1 < len(payload); index += 2 {
				group.triangles = append(group.triangles, worldWMOTriangle{flags: payload[index], material: payload[index+1]})
			}
		case "MOBA":
			for index := 0; index+24 <= len(payload); index += 24 {
				group.batches = append(group.batches, worldWMOBatch{
					start: int(binary.LittleEndian.Uint32(payload[index+12 : index+16])),
					count: int(binary.LittleEndian.Uint16(payload[index+16 : index+18])),
					flags: payload[index+22], material: payload[index+23],
				})
			}
		}
		offset = next
	}
	if len(group.vertices) < 3 || len(group.indices) < 3 {
		return worldWMOGroup{}, fmt.Errorf("WMO group has no geometry")
	}
	return group, nil
}

func worldFloat32s(data []byte, components int) []float32 {
	count := len(data) / (components * 4)
	result := make([]float32, 0, count*components)
	for index := 0; index < count*components; index++ {
		result = append(result, math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:index*4+4])))
	}
	return result
}

func buildWorldM2Instances(loader *ui.Loader, adt worldADT, position world.WorldPosition, cache *worldSceneAssetCache) (*core.Node, int) {
	root := core.NewNode()
	meshCount := 0
	for _, placement := range adt.m2Placements {
		origin := worldWMOPosition(placement.position)
		dx, dy := origin[0]-position.X, origin[1]-position.Y
		if dx*dx+dy*dy > worldObjectDistance*worldObjectDistance {
			continue
		}
		parts, ok := cache.m2Parts[placement.path]
		if !ok {
			var err error
			parts, err = loadWorldM2Parts(loader, placement.path)
			if err != nil {
				continue
			}
			cache.m2Parts[placement.path] = parts
		}
		instance := buildWorldM2Instance(loader, parts, cache.textures, cache.placeholder)
		if instance == nil {
			continue
		}
		instance.SetPosition(origin[0], origin[1], origin[2])
		instance.SetRotationQuat(worldM2Rotation(placement.rotation))
		instance.SetScale(placement.scale, placement.scale, placement.scale)
		root.Add(instance)
		meshCount += len(instance.Children())
	}
	if meshCount == 0 {
		return nil, 0
	}
	return root, meshCount
}

func loadWorldM2Parts(loader *ui.Loader, modelPath string) (map[string]*m2Part, error) {
	modelPath = normalizeModelPath(modelPath)
	if modelPath == "" {
		return nil, fmt.Errorf("empty M2 path")
	}
	modelData, err := loader.ReadFile(modelPath)
	if err != nil {
		return nil, err
	}
	model, err := parseM2(modelData)
	if err != nil {
		return nil, err
	}
	skinData, err := loader.ReadFile(worldM2SkinPath(modelPath))
	if err != nil {
		return nil, err
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		return nil, err
	}
	parts := buildM2Parts(model, skin)
	convertWorldM2Parts(parts)
	return parts, nil
}

func convertWorldM2Parts(parts map[string]*m2Part) {
	for _, part := range parts {
		for index := 0; index+2 < len(part.positions); index += 3 {
			x, y, z := part.positions[index], part.positions[index+1], part.positions[index+2]
			part.positions.Set(index, x, -z, y)
			x, y, z = part.normals[index], part.normals[index+1], part.normals[index+2]
			part.normals.Set(index, x, -z, y)
		}
	}
}

func buildWorldM2Instance(loader *ui.Loader, parts map[string]*m2Part, textures map[string]*texture.Texture2D, placeholder *texture.Texture2D) *core.Node {
	root := core.NewNode()
	for _, part := range parts {
		if len(part.texturePaths) == 0 || len(part.indices) == 0 {
			continue
		}
		geom := geometry.NewGeometry()
		geom.SetIndices(part.indices)
		geom.AddVBO(gls.NewVBO(part.positions).AddAttrib(gls.VertexPosition))
		geom.AddVBO(gls.NewVBO(part.normals).AddAttrib(gls.VertexNormal))
		geom.AddVBO(gls.NewVBO(part.uvs).AddAttrib(gls.VertexTexcoord))
		if len(part.uvSets) > 1 {
			geom.AddVBO(gls.NewVBO(part.uvs2).AddCustomAttrib("VertexTexcoord2", 2))
		}
		mat := material.NewStandard(&math32.Color{R: 1, G: 1, B: 1})
		if part.material.blend == 1 {
			mat.SetShader("morenowow_m2_alpha_key")
		} else {
			mat.SetShader("morenowow_m2")
		}
		mat.SetShaderUnique(true)
		if part.material.flags&0x04 != 0 {
			mat.SetSide(material.SideDouble)
		} else {
			mat.SetSide(material.SideFront)
		}
		mat.SetUseLights(material.UseLightNone)
		mat.SetDepthTest(part.material.flags&0x08 == 0)
		mat.SetDepthMask(part.material.flags&0x10 == 0)
		switch part.material.blend {
		case 0, 1:
			mat.SetTransparent(false)
			mat.SetBlending(material.BlendNone)
		case 3, 4:
			mat.SetTransparent(true)
			mat.SetBlending(material.BlendAdditive)
		case 5, 6:
			mat.SetTransparent(true)
			mat.SetBlending(material.BlendMultiply)
		default:
			mat.SetTransparent(true)
			mat.SetBlending(material.BlendNormal)
		}
		for textureIndex, texturePath := range part.texturePaths {
			tex := textures[texturePath]
			if tex == nil {
				tex = loadModelTexture(loader, texturePath)
				if tex != nil {
					textures[texturePath] = tex
				}
			}
			if tex == nil {
				tex = placeholder
			}
			if textureIndex < len(part.textureFlags) {
				if part.textureFlags[textureIndex]&0x01 != 0 {
					tex.SetWrapS(gls.REPEAT)
				}
				if part.textureFlags[textureIndex]&0x02 != 0 {
					tex.SetWrapT(gls.REPEAT)
				}
			}
			mat.AddTexture(tex)
		}
		mesh := graphic.NewMesh(geom, mat)
		mesh.SetRenderOrder(m2RenderOrder(part))
		root.Add(mesh)
	}
	if len(root.Children()) == 0 {
		return nil
	}
	return root
}

func worldM2SkinPath(path string) string {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".m2") {
		return path[:len(path)-3] + "00.skin"
	}
	if strings.HasSuffix(lower, ".mdx") {
		return path[:len(path)-4] + "00.skin"
	}
	return path + "00.skin"
}

func worldM2Rotation(rotation [3]float32) *math32.Quaternion {
	qz := math32.NewQuaternion(0, 0, 0, 1).SetFromAxisAngle(math32.NewVector3(0, 0, 1), float32(float64(rotation[1]+180)*math.Pi/180))
	qy := math32.NewQuaternion(0, 0, 0, 1).SetFromAxisAngle(math32.NewVector3(0, 1, 0), float32(float64(rotation[0])*math.Pi/180))
	qx := math32.NewQuaternion(0, 0, 0, 1).SetFromAxisAngle(math32.NewVector3(1, 0, 0), float32(float64(rotation[2])*math.Pi/180))
	return qz.Multiply(qy).Multiply(qx).Normalize()
}

func worldWMORotation(rotation [3]float32) *math32.Quaternion {
	qz := math32.NewQuaternion(0, 0, 0, 1).SetFromAxisAngle(math32.NewVector3(0, 0, 1), float32(float64(rotation[1]+180)*math.Pi/180))
	qy := math32.NewQuaternion(0, 0, 0, 1).SetFromAxisAngle(math32.NewVector3(0, 1, 0), float32(float64(-rotation[0])*math.Pi/180))
	qx := math32.NewQuaternion(0, 0, 0, 1).SetFromAxisAngle(math32.NewVector3(1, 0, 0), float32(float64(rotation[2])*math.Pi/180))
	return qz.Multiply(qy).Multiply(qx).Normalize()
}

func worldWMODoodadRotation(rotation [4]float32, parent [3]float32) *math32.Quaternion {
	local := math32.NewQuaternion(rotation[0], rotation[1], rotation[2], rotation[3]).Normalize()
	return worldWMORotation(parent).Multiply(local).Normalize()
}

func worldCharacterModelPath(character world.Character) string {
	folder := "Human"
	switch character.Race {
	case 2:
		folder = "Orc"
	case 3:
		folder = "Dwarf"
	case 4:
		folder = "NightElf"
	case 5:
		folder = "Scourge"
	case 6:
		folder = "Tauren"
	case 7:
		folder = "Gnome"
	case 8:
		folder = "Troll"
	case 10:
		folder = "BloodElf"
	case 11:
		folder = "Draenei"
	}
	sex := "Male"
	if character.Gender == 1 {
		sex = "Female"
	}
	return fmt.Sprintf(`Character\%s\%s\%s%s.m2`, folder, sex, folder, sex)
}

func buildWorldPlayer(loader *ui.Loader, character world.Character, position world.WorldPosition) (*core.Node, error) {
	model, err := loadGlueCharacterModel(loader, character)
	if err != nil {
		return nil, err
	}
	return wrapWorldModel(model, position, 1), nil
}

func buildWorldWMOInstances(loader *ui.Loader, adt worldADT, position world.WorldPosition, cache *worldSceneAssetCache) (*core.Node, int, []worldCollisionMesh) {
	root := core.NewNode()
	meshCount := 0
	doodadCount := 0
	solids := make([]worldCollisionMesh, 0)
	for _, placement := range adt.wmoPlacements {
		origin := worldWMOPosition(placement.position)
		dx, dy := origin[0]-position.X, origin[1]-position.Y
		if dx*dx+dy*dy > worldObjectDistance*worldObjectDistance {
			continue
		}
		model, ok := cache.wmoModels[placement.path]
		if !ok {
			data, err := loader.ReadFile(placement.path)
			if err != nil {
				continue
			}
			model, err = parseWorldWMORoot(data)
			if err != nil {
				continue
			}
			cache.wmoModels[placement.path] = model
		}
		meshes, placementSolids := buildWorldWMOPlacement(loader, placement, model, cache.textures, cache.placeholder)
		solids = append(solids, placementSolids...)
		for _, mesh := range meshes {
			root.Add(mesh)
			meshCount++
		}
		if setIndex := int(placement.doodadSet); setIndex >= 0 && setIndex < len(model.doodadSets) {
			set := model.doodadSets[setIndex]
			start := minWorldInt(int(set.start), len(model.doodads))
			end := minWorldInt(start+int(set.count), len(model.doodads))
			for _, doodad := range model.doodads[start:end] {
				if doodadCount >= worldWMODoodadBudget {
					break
				}
				parts := cache.m2Parts[doodad.path]
				if parts == nil {
					var err error
					parts, err = loadWorldM2Parts(loader, doodad.path)
					if err != nil {
						continue
					}
					cache.m2Parts[doodad.path] = parts
				}
				instance := buildWorldM2Instance(loader, parts, cache.textures, cache.placeholder)
				if instance == nil {
					continue
				}
				worldPosition := transformWorldWMOPoint(doodad.position, placement)
				instance.SetPosition(worldPosition[0], worldPosition[1], worldPosition[2])
				instance.SetRotationQuat(worldWMODoodadRotation(doodad.rotation, placement.rotation))
				instance.SetScale(placement.scale*doodad.scale, placement.scale*doodad.scale, placement.scale*doodad.scale)
				root.Add(instance)
				meshCount += len(instance.Children())
				doodadCount++
			}
		}
	}
	if meshCount == 0 && len(solids) == 0 {
		return nil, 0, nil
	}
	return root, meshCount, solids
}

func buildWorldWMOPlacement(loader *ui.Loader, placement worldWMOPlacement, model worldWMORoot, textures map[string]*texture.Texture2D, placeholder *texture.Texture2D) ([]*graphic.Mesh, []worldCollisionMesh) {
	builders := make(map[worldWMOMeshKey]*worldWMOMeshBuilder)
	solids := make([]worldCollisionMesh, 0)
	for groupIndex := 0; groupIndex < model.groupCount; groupIndex++ {
		groupData, err := loader.ReadFile(worldWMOGroupPath(placement.path, groupIndex))
		if err != nil {
			continue
		}
		group, err := parseWorldWMOGroup(groupData)
		if err != nil {
			continue
		}
		collisionMesh := worldCollisionMesh{triangles: make([]worldCollisionTriangle, 0, len(group.indices)/3)}
		for triangleIndex := 0; triangleIndex+2 < len(group.indices); triangleIndex += 3 {
			point := func(index uint16) ([3]float32, bool) {
				at := int(index) * 3
				if at+2 >= len(group.vertices) {
					return [3]float32{}, false
				}
				return transformWorldWMOPoint([3]float32{group.vertices[at], group.vertices[at+1], group.vertices[at+2]}, placement), true
			}
			a, aok := point(group.indices[triangleIndex])
			b, bok := point(group.indices[triangleIndex+1])
			c, cok := point(group.indices[triangleIndex+2])
			if !aok || !bok || !cok {
				continue
			}
			noCamera := triangleIndex/3 < len(group.triangles) && group.triangles[triangleIndex/3].flags&0x01 != 0
			if triangle, valid := worldCollisionTriangleFromPoints(a, b, c, noCamera); valid {
				collisionMesh.triangles = append(collisionMesh.triangles, triangle)
			}
		}
		if len(collisionMesh.triangles) > 0 {
			solids = append(solids, collisionMesh)
		}
		batches := group.batches
		if len(batches) == 0 {
			batches = []worldWMOBatch{{start: 0, count: len(group.indices)}}
		}
		bases := make(map[worldWMOMeshKey]uint32)
		for _, batch := range batches {
			if batch.start < 0 || batch.count <= 0 || batch.start+batch.count > len(group.indices) {
				continue
			}
			materialInfo := worldWMOMaterial{}
			if int(batch.material) < len(model.materials) {
				materialInfo = model.materials[batch.material]
			}
			key := worldWMOMeshKey{material: materialInfo, batchFlags: batch.flags}
			builder := builders[key]
			if builder == nil {
				builder = &worldWMOMeshBuilder{positions: math32.NewArrayF32(0, len(group.vertices)), uvs: math32.NewArrayF32(0, len(group.vertices)/3*2), colors: math32.NewArrayF32(0, len(group.vertices)), indices: math32.NewArrayU32(0, batch.count)}
				builders[key] = builder
			}
			base, exists := bases[key]
			if !exists {
				base = uint32(len(builder.positions) / 3)
				bases[key] = base
				for index := 0; index+2 < len(group.vertices); index += 3 {
					point := transformWorldWMOPoint([3]float32{group.vertices[index], group.vertices[index+1], group.vertices[index+2]}, placement)
					builder.positions.Append(point[0], point[1], point[2])
					uvIndex := index / 3 * 2
					if uvIndex+1 < len(group.uvs) {
						builder.uvs.Append(group.uvs[uvIndex], group.uvs[uvIndex+1])
					} else {
						builder.uvs.Append(0, 0)
					}
					colorIndex := index
					if colorIndex+2 < len(group.colors) {
						builder.colors.Append(group.colors[colorIndex], group.colors[colorIndex+1], group.colors[colorIndex+2])
					} else {
						builder.colors.Append(1, 1, 1)
					}
				}
			}
			for _, index := range group.indices[batch.start : batch.start+batch.count] {
				if int(index)*3+2 < len(group.vertices) {
					builder.indices.Append(base + uint32(index))
				}
			}
		}
	}
	meshes := make([]*graphic.Mesh, 0, len(builders))
	for key, builder := range builders {
		if len(builder.indices) < 3 {
			continue
		}
		tex := placeholder
		if key.material.texture != "" {
			if cached, ok := textures[key.material.texture]; ok {
				tex = cached
			} else if loaded := loadModelTexture(loader, key.material.texture); loaded != nil {
				textures[key.material.texture] = loaded
				tex = loaded
			}
		}
		geom := geometry.NewGeometry()
		geom.SetIndices(builder.indices)
		geom.AddVBO(gls.NewVBO(builder.positions).AddAttrib(gls.VertexPosition))
		geom.AddVBO(gls.NewVBO(builder.uvs).AddAttrib(gls.VertexTexcoord))
		geom.AddVBO(gls.NewVBO(builder.colors).AddAttrib(gls.VertexColor))
		mat := material.NewStandard(&math32.Color{R: 1, G: 1, B: 1})
		shader := "morenowow_world_wmo"
		if key.material.blend == 1 {
			shader = "morenowow_world_wmo_alpha_key"
		}
		mat.SetShader(shader)
		mat.SetShaderUnique(true)
		mat.SetDepthTest(true)
		mat.SetDepthMask(true)
		if key.material.flags&0x04 != 0 {
			mat.SetSide(material.SideDouble)
		} else {
			mat.SetSide(material.SideFront)
		}
		mat.SetUseLights(material.UseLightNone)
		mat.AddTexture(tex)
		switch key.material.blend {
		case 0, 1:
			mat.SetTransparent(false)
			mat.SetBlending(material.BlendNone)
		case 3, 4:
			mat.SetTransparent(true)
			mat.SetDepthMask(false)
			mat.SetBlending(material.BlendAdditive)
		default:
			mat.SetTransparent(true)
			mat.SetDepthMask(false)
			mat.SetBlending(material.BlendNormal)
		}
		mesh := graphic.NewMesh(geom, mat)
		mesh.SetRenderOrder(-40 + int(key.material.blend))
		meshes = append(meshes, mesh)
	}
	return meshes, solids
}

func worldWMOGroupPath(path string, index int) string {
	stem := path
	if strings.HasSuffix(strings.ToLower(stem), ".wmo") {
		stem = stem[:len(stem)-4]
	}
	return fmt.Sprintf("%s_%03d.wmo", stem, index)
}

func worldWMOPosition(raw [3]float32) [3]float32 {
	return [3]float32{32*worldTileSize - raw[2], 32*worldTileSize - raw[0], raw[1]}
}

func transformWorldWMOPoint(point [3]float32, placement worldWMOPlacement) [3]float32 {
	point[0] *= placement.scale
	point[1] *= placement.scale
	point[2] *= placement.scale
	roll := float64(placement.rotation[2]) * math.Pi / 180
	pitch := -float64(placement.rotation[0]) * math.Pi / 180
	yaw := (float64(placement.rotation[1]) + 180) * math.Pi / 180
	point = rotateWorldWMOX(point, roll)
	point = rotateWorldWMOY(point, pitch)
	point = rotateWorldWMOZ(point, yaw)
	origin := worldWMOPosition(placement.position)
	return [3]float32{point[0] + origin[0], point[1] + origin[1], point[2] + origin[2]}
}

func rotateWorldWMOX(point [3]float32, angle float64) [3]float32 {
	c, s := float32(math.Cos(angle)), float32(math.Sin(angle))
	return [3]float32{point[0], point[1]*c - point[2]*s, point[1]*s + point[2]*c}
}

func rotateWorldWMOY(point [3]float32, angle float64) [3]float32 {
	c, s := float32(math.Cos(angle)), float32(math.Sin(angle))
	return [3]float32{point[0]*c + point[2]*s, point[1], -point[0]*s + point[2]*c}
}

func rotateWorldWMOZ(point [3]float32, angle float64) [3]float32 {
	c, s := float32(math.Cos(angle)), float32(math.Sin(angle))
	return [3]float32{point[0]*c - point[1]*s, point[0]*s + point[1]*c, point[2]}
}

func parseWorldTextureNames(data []byte) []string {
	var result []string
	for _, part := range strings.Split(string(data), "\x00") {
		part = strings.TrimSpace(strings.ReplaceAll(part, "/", "\\"))
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseWorldMCNK(data []byte, offset, fallbackX, fallbackY int, textures []string, bigAlpha bool) (worldADTChunk, bool) {
	id, size, payload, _, _, ok := worldChunk(data, offset)
	if !ok || id != "MCNK" || len(payload) < 128 || size < 128 {
		return worldADTChunk{}, false
	}
	chunk := worldADTChunk{gridX: fallbackX, gridY: fallbackY, texture: -1}
	chunk.flags = binary.LittleEndian.Uint32(payload[0:4])
	if x := int(binary.LittleEndian.Uint32(payload[4:8])); x < 16 {
		chunk.gridX = x
	}
	if y := int(binary.LittleEndian.Uint32(payload[8:12])); y < 16 {
		chunk.gridY = y
	}
	chunk.holes = binary.LittleEndian.Uint16(payload[0x3c:0x3e])
	for index := range chunk.position {
		chunk.position[index] = math.Float32frombits(binary.LittleEndian.Uint32(payload[0x68+index*4 : 0x6c+index*4]))
	}
	mcvt, ok := worldSubChunk(data, offset, int(binary.LittleEndian.Uint32(payload[0x14:0x18])), "MCVT")
	if !ok || len(mcvt) < worldHeightCount*4 {
		return worldADTChunk{}, false
	}
	for index := range chunk.heights {
		chunk.heights[index] = math.Float32frombits(binary.LittleEndian.Uint32(mcvt[index*4 : index*4+4]))
	}
	layers := int(binary.LittleEndian.Uint32(payload[0x0c:0x10]))
	if layers > 4 {
		layers = 4
	}
	if layers > 0 {
		mcly, found := worldSubChunk(data, offset, int(binary.LittleEndian.Uint32(payload[0x1c:0x20])), "MCLY")
		if found {
			for index := 0; index < layers && index*16+16 <= len(mcly); index++ {
				layer := worldADTLayer{texture: int(binary.LittleEndian.Uint32(mcly[index*16 : index*16+4])), flags: binary.LittleEndian.Uint32(mcly[index*16+4 : index*16+8]), alphaOffset: binary.LittleEndian.Uint32(mcly[index*16+8 : index*16+12])}
				chunk.layers = append(chunk.layers, layer)
				if index == 0 && layer.texture >= 0 && layer.texture < len(textures) {
					chunk.texture = layer.texture
				}
			}
		}
		if len(chunk.layers) > 1 {
			mcal, _ := worldSubChunk(data, offset, int(binary.LittleEndian.Uint32(payload[0x24:0x28])), "MCAL")
			for index := 1; index < len(chunk.layers); index++ {
				layer := chunk.layers[index]
				chunk.alphaMaps = append(chunk.alphaMaps, decodeWorldAlphaMap(mcal, int(layer.alphaOffset), layer.flags&0x200 != 0, bigAlpha, chunk.flags&0x8000 != 0))
			}
		}
	}
	return chunk, true
}

func opaqueWorldAlphaMap() []byte {
	alpha := make([]byte, 64*64)
	for index := range alpha {
		alpha[index] = 255
	}
	return alpha
}

func decodeWorldAlphaMap(data []byte, offset int, compressed, bigAlpha, doNotFix bool) []byte {
	alpha := make([]byte, 64*64)
	if offset < 0 || offset >= len(data) {
		return alpha
	}
	if compressed {
		write := 0
		for read := offset; read < len(data) && write < len(alpha); {
			control := data[read]
			read++
			count := int(control & 0x7f)
			if count == 0 {
				continue
			}
			if control&0x80 != 0 {
				if read >= len(data) {
					break
				}
				value := data[read]
				read++
				for index := 0; index < count && write < len(alpha); index++ {
					alpha[write] = value
					write++
				}
			} else {
				count = minWorldInt(count, len(data)-read)
				count = minWorldInt(count, len(alpha)-write)
				copy(alpha[write:write+count], data[read:read+count])
				read += count
				write += count
			}
		}
		return alpha
	}
	if bigAlpha && offset+4096 <= len(data) {
		copy(alpha, data[offset:offset+4096])
		return alpha
	}
	if !bigAlpha && offset+2048 <= len(data) {
		for index, packed := range data[offset : offset+2048] {
			alpha[index*2] = (packed & 0x0f) * 17
			alpha[index*2+1] = (packed >> 4) * 17
		}
		if !doNotFix {
			for row := 0; row < 64; row++ {
				alpha[row*64+63] = alpha[row*64+62]
			}
			for column := 0; column < 64; column++ {
				alpha[63*64+column] = alpha[62*64+column]
			}
		}
	}
	return alpha
}

func minWorldInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func worldHeightPoint(chunk worldADTChunk, index int) [3]float32 {
	pair, within := index/17, index%17
	row, col := pair*2, within
	inner := false
	if within >= 9 {
		row, col, inner = pair*2+1, within-9, true
	}
	columnOffset := float32(0)
	if inner {
		columnOffset = 0.5
	}
	return [3]float32{chunk.position[0] - float32(row)*0.5*worldUnitSize, chunk.position[1] - (float32(col)+columnOffset)*worldUnitSize, chunk.position[2] + chunk.heights[index]}
}

func buildWorldTerrain(loader *ui.Loader, adt worldADT, position world.WorldPosition) (*core.Node, worldSceneInfo, error) {
	return buildWorldTerrainProgress(loader, adt, position, nil)
}

func buildWorldSky() *core.Node {
	root := core.NewNode()
	root.SetRotation(math.Pi/2, 0, 0)
	geom := geometry.NewSphere(2400, 64, 32)
	mat := material.NewStandard(&math32.Color{R: 1, G: 1, B: 1})
	mat.SetShader("morenowow_world_sky")
	mat.SetShaderUnique(true)
	mat.SetSide(material.SideDouble)
	mat.SetUseLights(material.UseLightNone)
	mat.SetDepthTest(false)
	mat.SetDepthMask(false)
	mat.SetTransparent(false)
	mesh := graphic.NewMesh(geom, mat)
	mesh.SetCullable(false)
	mesh.SetRenderOrder(1000)
	root.Add(mesh)
	return root
}

func buildWorldTerrainProgress(loader *ui.Loader, adt worldADT, position world.WorldPosition, progress func(float64)) (*core.Node, worldSceneInfo, error) {
	return buildWorldTerrainProgressWithCache(loader, adt, position, newWorldSceneAssetCache(), progress)
}

func buildWorldTerrainProgressWithCache(loader *ui.Loader, adt worldADT, position world.WorldPosition, cache *worldSceneAssetCache, progress func(float64)) (*core.Node, worldSceneInfo, error) {
	root := core.NewNode()
	info := worldSceneInfo{textures: len(adt.textures)}
	for chunkIndex, chunk := range adt.chunks {
		positions := math32.NewArrayF32(0, worldHeightCount*3)
		normals := math32.NewArrayF32(0, worldHeightCount*3)
		uvs := math32.NewArrayF32(0, worldHeightCount*2)
		for index := 0; index < worldHeightCount; index++ {
			point := worldHeightPoint(chunk, index)
			positions.Append(point[0], point[1], point[2])
			normals.Append(0, 0, 1)
			pair, within := index/17, index%17
			row, col := pair*2, within
			if within >= 9 {
				row, col = pair*2+1, within-9
			}
			u, v := float32(col)/8, float32(row)*0.5/8
			if within >= 9 {
				u += 0.5 / 8
			}
			uvs.Append(u, v)
		}
		indices := math32.NewArrayU32(0, 8*8*4*3)
		outer := func(row, col int) uint32 { return uint32(row*17 + col) }
		inner := func(row, col int) uint32 { return uint32(row*17 + 9 + col) }
		for cellY := 0; cellY < 8; cellY++ {
			for cellX := 0; cellX < 8; cellX++ {
				if chunk.holes&(1<<uint((cellY/2)*4+cellX/2)) != 0 {
					continue
				}
				center := inner(cellY, cellX)
				topLeft, topRight := outer(cellY, cellX), outer(cellY, cellX+1)
				bottomLeft, bottomRight := outer(cellY+1, cellX), outer(cellY+1, cellX+1)
				indices.Append(center, topLeft, topRight, center, topRight, bottomRight, center, bottomRight, bottomLeft, center, bottomLeft, topLeft)
			}
		}
		if len(indices) == 0 {
			continue
		}
		geom := geometry.NewGeometry()
		geom.SetIndices(indices)
		geom.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition))
		geom.AddVBO(gls.NewVBO(normals).AddAttrib(gls.VertexNormal))
		geom.AddVBO(gls.NewVBO(uvs).AddAttrib(gls.VertexTexcoord))
		layerIndices := [4]int{-1, -1, -1, -1}
		if len(chunk.layers) > 0 {
			for index := 0; index < len(chunk.layers) && index < len(layerIndices); index++ {
				layerIndices[index] = chunk.layers[index].texture
			}
		} else {
			layerIndices[0] = chunk.texture
		}
		mat := material.NewStandard(&math32.Color{R: 1, G: 1, B: 1})
		mat.SetShader("morenowow_world_terrain")
		mat.SetShaderUnique(true)
		mat.SetSide(material.SideDouble)
		mat.SetUseLights(material.UseLightNone)
		for _, textureIndex := range layerIndices {
			tex := cache.terrainPlaceholder
			if textureIndex >= 0 && textureIndex < len(adt.textures) {
				path := adt.textures[textureIndex]
				if cached, ok := cache.textures[path]; ok {
					tex = cached
				} else if loaded := loadModelTexture(loader, path); loaded != nil {
					loaded.SetWrapS(gls.REPEAT)
					loaded.SetWrapT(gls.REPEAT)
					cache.textures[path] = loaded
					tex = loaded
				}
			}
			mat.AddTexture(tex)
		}
		for index := 0; index < 3; index++ {
			alphaTexture := cache.zeroAlpha
			if index < len(chunk.alphaMaps) {
				if len(chunk.alphaMaps[index]) == 0 {
					alphaTexture = cache.zeroAlpha
				} else if isOpaqueWorldAlpha(chunk.alphaMaps[index]) {
					alphaTexture = cache.opaqueAlpha
				} else {
					alphaTexture = texture.NewTexture2DFromRGBA(worldAlphaTexture(chunk.alphaMaps[index], false))
				}
			}
			mat.AddTexture(alphaTexture)
		}
		mesh := graphic.NewMesh(geom, mat)
		mesh.SetRenderOrder(-90)
		root.Add(mesh)
		info.chunks++
		info.vertices += len(positions) / 3
		info.triangles += len(indices) / 3
		if progress != nil {
			progress(0.2 + 0.55*float64(chunkIndex+1)/float64(len(adt.chunks)))
		}
	}
	if info.chunks == 0 {
		return nil, worldSceneInfo{}, fmt.Errorf("ADT produced no drawable chunks")
	}
	wmoRoot, wmoCount, wmoSolids := buildWorldWMOInstances(loader, adt, position, cache)
	if wmoRoot != nil {
		root.Add(wmoRoot)
	}
	m2Root, m2Count := buildWorldM2Instances(loader, adt, position, cache)
	if m2Root != nil {
		root.Add(m2Root)
	}
	info.wmoMeshes = wmoCount
	if progress != nil {
		progress(0.85)
	}
	info.m2Meshes = m2Count
	if progress != nil {
		progress(0.98)
	}
	root.SetUserData(newWorldSceneCollision(adt.chunks, wmoSolids))
	return root, info, nil
}

func worldPlaceholderTexture() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 80, G: 105, B: 70, A: 255})
	return img
}

func worldSolidTexture(c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, c)
	return img
}

func worldAlphaTexture(data []byte, opaque bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for index := 0; index < 64*64; index++ {
		value := byte(0)
		if opaque {
			value = 255
		} else if index < len(data) {
			value = data[index]
		}
		img.SetRGBA(index%64, index/64, color.RGBA{R: value, G: value, B: value, A: 255})
	}
	return img
}

func isOpaqueWorldAlpha(data []byte) bool {
	if len(data) < 64*64 {
		return false
	}
	for _, value := range data[:64*64] {
		if value != 255 {
			return false
		}
	}
	return true
}

func worldChunk(data []byte, offset int) (string, int, []byte, int, int, bool) {
	if offset < 0 || offset+8 > len(data) {
		return "", 0, nil, 0, 0, false
	}
	size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
	end := offset + 8 + size
	if size < 0 || end < offset+8 || end > len(data) {
		return "", 0, nil, 0, 0, false
	}
	next := end
	if next > len(data) {
		return "", 0, nil, 0, 0, false
	}
	magic := string([]byte{data[offset+3], data[offset+2], data[offset+1], data[offset]})
	return magic, size, data[offset+8 : end], end, next, true
}

func worldSubChunk(data []byte, chunkOffset, relative int, expected string) ([]byte, bool) {
	if relative <= 0 {
		return nil, false
	}
	id, _, payload, _, _, ok := worldChunk(data, chunkOffset+relative)
	return payload, ok && id == expected
}
