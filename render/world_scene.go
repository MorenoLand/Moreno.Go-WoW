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
	worldTileSize    = 1600.0 / 3.0
	worldChunkSize   = worldTileSize / 16.0
	worldUnitSize    = worldChunkSize / 8.0
	worldHeightCount = 145
)

type worldADT struct {
	version       uint32
	textures      []string
	wmoNames      []string
	wmoPlacements []worldWMOPlacement
	chunks        []worldADTChunk
}

type worldADTChunk struct {
	gridX    int
	gridY    int
	flags    uint32
	holes    uint16
	position [3]float32
	heights  [worldHeightCount]float32
	texture  int
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

type worldWMOMaterial struct {
	flags   uint32
	blend   uint32
	texture string
}

type worldWMORoot struct {
	groupCount int
	materials  []worldWMOMaterial
}

type worldWMOBatch struct {
	start    int
	count    int
	flags    uint8
	material uint8
}

type worldWMOGroup struct {
	vertices []float32
	normals  []float32
	uvs      []float32
	indices  []uint16
	batches  []worldWMOBatch
}

type worldSceneInfo struct {
	mapName   string
	tileX     int
	tileY     int
	chunks    int
	vertices  int
	triangles int
	textures  int
	wmoMeshes int
}

func loadWorldTerrain(loader *ui.Loader, position world.WorldPosition) (*core.Node, worldSceneInfo, error) {
	if loader == nil {
		return nil, worldSceneInfo{}, fmt.Errorf("world terrain has no asset loader")
	}
	mapName := loadMapName(loader, position.Map)
	if mapName == "" {
		return nil, worldSceneInfo{}, fmt.Errorf("map %d has no Map.dbc name", position.Map)
	}
	tileX, tileY := worldTileAt(position.X, position.Y)
	if tileX < 0 || tileX >= 64 || tileY < 0 || tileY >= 64 {
		return nil, worldSceneInfo{}, fmt.Errorf("world position %.3f,%.3f maps outside tile grid at %d,%d", position.X, position.Y, tileX, tileY)
	}
	path := fmt.Sprintf(`World\Maps\%s\%s_%d_%d.adt`, mapName, mapName, tileX, tileY)
	data, err := loader.ReadFile(path)
	if err != nil {
		return nil, worldSceneInfo{}, fmt.Errorf("read %s: %w", path, err)
	}
	adt, err := parseWorldADT(data)
	if err != nil {
		return nil, worldSceneInfo{}, fmt.Errorf("parse %s: %w", path, err)
	}
	root, info, err := buildWorldTerrain(loader, adt, position)
	if err != nil {
		return nil, worldSceneInfo{}, fmt.Errorf("build %s: %w", path, err)
	}
	info.mapName, info.tileX, info.tileY = mapName, tileX, tileY
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

func worldTileAt(x, y float32) (int, int) {
	return int(math.Floor(float64(32 - y/worldTileSize))), int(math.Floor(float64(32 - x/worldTileSize)))
}

func parseWorldADT(data []byte) (worldADT, error) {
	result := worldADT{}
	var main []byte
	var mwmo, mwid, modf []byte
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
	result.wmoNames = parseWorldOffsetNames(mwmo, mwid)
	result.wmoPlacements = parseWorldWMOPlacements(modf, result.wmoNames)
	for index := 0; index < 256; index++ {
		base := index * 16
		chunkOffset := int(binary.LittleEndian.Uint32(main[base : base+4]))
		if chunkOffset == 0 {
			continue
		}
		chunk, ok := parseWorldMCNK(data, chunkOffset, index%16, index/16, result.textures)
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

func parseWorldWMORoot(data []byte) (worldWMORoot, error) {
	var version uint32
	var mohd, motx, momt []byte
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
		case "MOVI":
			for index := 0; index+2 <= len(payload); index += 2 {
				group.indices = append(group.indices, binary.LittleEndian.Uint16(payload[index:index+2]))
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

func buildWorldWMOInstances(loader *ui.Loader, adt worldADT, position world.WorldPosition, textures map[string]*texture.Texture2D, placeholder *texture.Texture2D) (*core.Node, int) {
	root := core.NewNode()
	modelCache := make(map[string]worldWMORoot)
	meshCount := 0
	for _, placement := range adt.wmoPlacements {
		origin := worldWMOPosition(placement.position)
		dx, dy := origin[0]-position.X, origin[1]-position.Y
		if dx*dx+dy*dy > (worldTileSize*2)*(worldTileSize*2) {
			continue
		}
		model, ok := modelCache[placement.path]
		if !ok {
			data, err := loader.ReadFile(placement.path)
			if err != nil {
				continue
			}
			model, err = parseWorldWMORoot(data)
			if err != nil {
				continue
			}
			modelCache[placement.path] = model
		}
		for groupIndex := 0; groupIndex < model.groupCount; groupIndex++ {
			groupPath := worldWMOGroupPath(placement.path, groupIndex)
			data, err := loader.ReadFile(groupPath)
			if err != nil {
				continue
			}
			group, err := parseWorldWMOGroup(data)
			if err != nil {
				continue
			}
			batches := group.batches
			if len(batches) == 0 {
				batches = []worldWMOBatch{{start: 0, count: len(group.indices)}}
			}
			for _, batch := range batches {
				mesh := buildWorldWMOBatch(loader, group, batch, model.materials, placement, textures, placeholder)
				if mesh != nil {
					root.Add(mesh)
					meshCount++
				}
			}
		}
	}
	if meshCount == 0 {
		return nil, 0
	}
	return root, meshCount
}

func worldWMOGroupPath(path string, index int) string {
	stem := path
	if strings.HasSuffix(strings.ToLower(stem), ".wmo") {
		stem = stem[:len(stem)-4]
	}
	return fmt.Sprintf("%s_%03d.wmo", stem, index)
}

func buildWorldWMOBatch(loader *ui.Loader, group worldWMOGroup, batch worldWMOBatch, materials []worldWMOMaterial, placement worldWMOPlacement, textures map[string]*texture.Texture2D, placeholder *texture.Texture2D) *graphic.Mesh {
	start, end := batch.start, batch.start+batch.count
	if start < 0 || end > len(group.indices) || start >= end {
		return nil
	}
	positions := math32.NewArrayF32(0, len(group.vertices))
	uvs := math32.NewArrayF32(0, len(group.vertices)/3*2)
	for index := 0; index+2 < len(group.vertices); index += 3 {
		point := transformWorldWMOPoint([3]float32{group.vertices[index], group.vertices[index+1], group.vertices[index+2]}, placement)
		positions.Append(point[0], point[1], point[2])
		uvIndex := index / 3 * 2
		if uvIndex+1 < len(group.uvs) {
			uvs.Append(group.uvs[uvIndex], group.uvs[uvIndex+1])
		} else {
			uvs.Append(0, 0)
		}
	}
	indices := math32.NewArrayU32(0, end-start)
	for _, index := range group.indices[start:end] {
		if int(index)*3+2 < len(group.vertices) {
			indices.Append(uint32(index))
		}
	}
	if len(indices) < 3 {
		return nil
	}
	materialIndex := int(batch.material)
	materialInfo := worldWMOMaterial{}
	if materialIndex >= 0 && materialIndex < len(materials) {
		materialInfo = materials[materialIndex]
	}
	path := materialInfo.texture
	tex := placeholder
	if path != "" {
		if cached, ok := textures[path]; ok {
			tex = cached
		} else if loaded := loadModelTexture(loader, path); loaded != nil {
			textures[path] = loaded
			tex = loaded
		}
	}
	geom := geometry.NewGeometry()
	geom.SetIndices(indices)
	geom.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition))
	geom.AddVBO(gls.NewVBO(uvs).AddAttrib(gls.VertexTexcoord))
	mat := material.NewStandard(&math32.Color{R: 1, G: 1, B: 1})
	shader := "morenowow_world_terrain"
	if materialInfo.blend == 1 {
		shader = "morenowow_world_terrain_alpha_key"
	}
	mat.SetShader(shader)
	mat.SetShaderUnique(true)
	if materialInfo.flags&0x04 != 0 {
		mat.SetSide(material.SideDouble)
	} else {
		mat.SetSide(material.SideFront)
	}
	mat.SetUseLights(material.UseLightNone)
	mat.AddTexture(tex)
	if materialInfo.blend >= 2 {
		mat.SetTransparent(true)
		mat.SetDepthMask(false)
		if materialInfo.blend == 4 {
			mat.SetBlending(material.BlendAdditive)
		} else {
			mat.SetBlending(material.BlendNormal)
		}
	}
	mesh := graphic.NewMesh(geom, mat)
	mesh.SetRenderOrder(-40 + int(materialInfo.blend))
	return mesh
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

func parseWorldMCNK(data []byte, offset, fallbackX, fallbackY int, textures []string) (worldADTChunk, bool) {
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
	if layers > 0 {
		mcly, found := worldSubChunk(data, offset, int(binary.LittleEndian.Uint32(payload[0x1c:0x20])), "MCLY")
		if found && len(mcly) >= 16 {
			texture := int(binary.LittleEndian.Uint32(mcly[:4]))
			if texture >= 0 && texture < len(textures) {
				chunk.texture = texture
			}
		}
	}
	return chunk, true
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
	root := core.NewNode()
	textures := make(map[string]*texture.Texture2D)
	placeholder := texture.NewTexture2DFromRGBA(worldPlaceholderTexture())
	info := worldSceneInfo{textures: len(adt.textures)}
	for _, chunk := range adt.chunks {
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
		tex := placeholder
		if chunk.texture >= 0 && chunk.texture < len(adt.textures) {
			path := adt.textures[chunk.texture]
			if cached, ok := textures[path]; ok {
				tex = cached
			} else if loaded := loadModelTexture(loader, path); loaded != nil {
				textures[path] = loaded
				tex = loaded
			}
		}
		mat := material.NewStandard(&math32.Color{R: 1, G: 1, B: 1})
		mat.SetShader("morenowow_world_terrain")
		mat.SetShaderUnique(true)
		mat.SetSide(material.SideDouble)
		mat.SetUseLights(material.UseLightNone)
		mat.AddTexture(tex)
		mesh := graphic.NewMesh(geom, mat)
		mesh.SetRenderOrder(-90)
		root.Add(mesh)
		info.chunks++
		info.vertices += len(positions) / 3
		info.triangles += len(indices) / 3
	}
	if info.chunks == 0 {
		return nil, worldSceneInfo{}, fmt.Errorf("ADT produced no drawable chunks")
	}
	wmoRoot, wmoCount := buildWorldWMOInstances(loader, adt, position, textures, placeholder)
	if wmoRoot != nil {
		root.Add(wmoRoot)
	}
	info.wmoMeshes = wmoCount
	return root, info, nil
}

func worldPlaceholderTexture() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 80, G: 105, B: 70, A: 255})
	return img
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
