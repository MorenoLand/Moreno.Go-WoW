package render

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"math"
	"strings"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/texture"
)

const (
	m2Version264     = 264
	m2VertexSize     = 48
	m2CameraSize     = 100
	m2ParticleSize   = 476
	m2RenderFlagSize = 4
	skinSubmeshSize  = 48
	skinBatchSize    = 24
)

type m2Array struct {
	count  int
	offset int
}

type m2Vertex struct {
	position [3]float32
	weights  [4]uint8
	bones    [4]uint8
	normal   [3]float32
	uv       [2]float32
	uv2      [2]float32
}

type m2RenderFlag struct {
	flags uint16
	blend uint16
}

type m2Bone struct {
	flags       uint32
	parent      int16
	pivot       [3]float32
	translation [3]float32
	rotation    [4]float32
	scale       [3]float32
}

type skinSubmesh struct {
	vertexStart    uint32
	indexStart     uint32
	indexCount     uint32
	boneCount      uint32
	boneComboIndex uint32
	boneInfluences uint32
}

type skinBatch struct {
	priorityPlane     int8
	submeshIndex      uint16
	materialIndex     uint16
	materialLayer     uint16
	textureCount      uint16
	textureComboIndex uint16
	textureCoordIndex uint16
}

type m2Part struct {
	texturePaths  []string
	textureFlags  []uint32
	uvSets        []int
	positions     math32.ArrayF32
	normals       math32.ArrayF32
	uvs           math32.ArrayF32
	uvs2          math32.ArrayF32
	indices       math32.ArrayU32
	renderOrder   int
	priorityPlane int8
	materialLayer uint16
	uvSet         int
	material      m2RenderFlag
}

type posedM2Vertex struct {
	position [3]float32
	normal   [3]float32
}

type m2Camera struct {
	fov      float32
	farClip  float32
	nearClip float32
	position [3]float32
	target   [3]float32
}

type glueModelInfo struct {
	position  math32.Vector3
	target    math32.Vector3
	fov       float32
	far       float32
	near      float32
	stats     glueModelStats
	particles *m2ParticleSystem
}

type glueModelStats struct {
	parts              int
	vertices           int
	triangles          int
	textures           int
	opaqueBatches      int
	transparentBatches int
	particleEmitters   int
	particlePoints     int
}

func loadGlueModel(loader *ui.Loader, modelPath string) (*core.Node, error) {
	modelPath = normalizeModelPath(modelPath)
	if modelPath == "" {
		return nil, nil
	}
	modelData, err := loader.ReadFile(modelPath)
	if err != nil {
		return nil, err
	}
	model, err := parseM2(modelData)
	if err != nil {
		return nil, err
	}
	skinPath := strings.TrimSuffix(modelPath, ".m2") + "00.skin"
	skinData, err := loader.ReadFile(skinPath)
	if err != nil {
		return nil, err
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		return nil, err
	}
	parts := buildM2Parts(model, skin)
	if len(parts) == 0 {
		return nil, fmt.Errorf("%s: no renderable skin batches", modelPath)
	}
	root := core.NewNode()
	textures := make(map[string]*texture.Texture2D)
	texturePaths := make(map[string]struct{})
	stats := glueModelStats{}
	for _, part := range parts {
		if len(part.texturePaths) == 0 || len(part.indices) == 0 {
			continue
		}
		stats.parts++
		stats.vertices += len(part.positions) / 3
		stats.triangles += len(part.indices) / 3
		if part.material.blend >= 2 {
			stats.transparentBatches++
		} else {
			stats.opaqueBatches++
		}
		for _, texturePath := range part.texturePaths {
			texturePaths[texturePath] = struct{}{}
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
		case 0:
			mat.SetTransparent(false)
			mat.SetBlending(material.BlendNone)
		case 1:
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
				textures[texturePath] = tex
			}
			if tex != nil {
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
		}
		mesh := graphic.NewMesh(geom, mat)
		mesh.SetRenderOrder(m2RenderOrder(part))
		root.Add(mesh)
	}
	if len(root.Children()) == 0 {
		return nil, fmt.Errorf("%s: model textures or geometry unavailable", modelPath)
	}
	center, scale := modelTransform(model.vertices)
	root.SetPosition(-center[0]*scale, -center[1]*scale, -center[2]*scale)
	root.SetScale(scale, scale, scale)
	stats.textures = len(texturePaths)
	particles := buildM2ParticleSystem(loader, model, root, scale, textures)
	if particles != nil {
		stats.particleEmitters = particles.emitterCount
		stats.particlePoints = particles.pointCount
	}
	info := glueModelInfo{stats: stats, particles: particles}
	if model.camera != nil {
		position := modelPoint(model.camera.position, center, scale)
		target := modelPoint(model.camera.target, center, scale)
		info.position = *math32.NewVector3(position[0], position[1], position[2])
		info.target = *math32.NewVector3(target[0], target[1], target[2])
		info.fov = model.camera.fov
		info.far = model.camera.farClip * scale
		info.near = model.camera.nearClip * scale
	}
	root.SetUserData(info)
	return root, nil
}

type parsedM2 struct {
	vertices      []m2Vertex
	bones         []m2Bone
	boneCombos    []uint16
	textures      []string
	textureCombos []uint16
	textureFlags  []uint32
	textureCoords []uint16
	renderFlags   []m2RenderFlag
	camera        *m2Camera
	particles     []m2ParticleEmitter
}

func parseM2(data []byte) (parsedM2, error) {
	if len(data) < 0xA0 || string(data[:4]) != "MD20" {
		return parsedM2{}, fmt.Errorf("invalid M2 header")
	}
	if binary.LittleEndian.Uint32(data[4:8]) != m2Version264 {
		return parsedM2{}, fmt.Errorf("unsupported M2 version %d", binary.LittleEndian.Uint32(data[4:8]))
	}
	vertices, err := readM2Array(data, 0x3c, m2VertexSize)
	if err != nil {
		return parsedM2{}, err
	}
	textures, err := readM2Array(data, 0x50, 16)
	if err != nil {
		return parsedM2{}, err
	}
	combos, err := readM2Array(data, 0x80, 2)
	if err != nil {
		return parsedM2{}, err
	}
	textureCoords, err := readM2Array(data, 0x88, 2)
	if err != nil {
		return parsedM2{}, err
	}
	renderFlags, err := readM2Array(data, 0x70, m2RenderFlagSize)
	if err != nil {
		return parsedM2{}, err
	}
	cameras, err := readM2Array(data, 0x110, m2CameraSize)
	if err != nil {
		return parsedM2{}, err
	}
	particles, err := readM2Array(data, 0x128, m2ParticleSize)
	if err != nil {
		return parsedM2{}, err
	}
	boneCombos, err := readM2Array(data, 0x78, 2)
	if err != nil {
		return parsedM2{}, err
	}
	bones, err := readM2Array(data, 0x2c, 88)
	if err != nil {
		return parsedM2{}, err
	}
	result := parsedM2{vertices: make([]m2Vertex, vertices.count), bones: make([]m2Bone, bones.count), boneCombos: make([]uint16, boneCombos.count), textures: make([]string, textures.count), textureFlags: make([]uint32, textures.count), textureCombos: make([]uint16, combos.count), textureCoords: make([]uint16, textureCoords.count), renderFlags: make([]m2RenderFlag, renderFlags.count), particles: make([]m2ParticleEmitter, particles.count)}
	for index := range result.vertices {
		base := vertices.offset + index*m2VertexSize
		result.vertices[index] = m2Vertex{position: [3]float32{readF32(data, base), readF32(data, base+4), readF32(data, base+8)}, weights: [4]uint8{data[base+12], data[base+13], data[base+14], data[base+15]}, bones: [4]uint8{data[base+16], data[base+17], data[base+18], data[base+19]}, normal: [3]float32{readF32(data, base+20), readF32(data, base+24), readF32(data, base+28)}, uv: [2]float32{readF32(data, base+32), readF32(data, base+36)}, uv2: [2]float32{readF32(data, base+40), readF32(data, base+44)}}
	}
	for index := range result.boneCombos {
		result.boneCombos[index] = binary.LittleEndian.Uint16(data[boneCombos.offset+index*2:])
	}
	for index := range result.bones {
		base := bones.offset + index*88
		result.bones[index] = m2Bone{flags: binary.LittleEndian.Uint32(data[base+4 : base+8]), parent: int16(binary.LittleEndian.Uint16(data[base+8 : base+10])), pivot: [3]float32{readF32(data, base+76), readF32(data, base+80), readF32(data, base+84)}, translation: readM2TrackVector(data, base+16, [3]float32{}), rotation: readM2TrackQuaternion(data, base+36), scale: readM2TrackVector(data, base+56, [3]float32{1, 1, 1})}
	}
	for index := range result.textures {
		base := textures.offset + index*16
		result.textureFlags[index] = binary.LittleEndian.Uint32(data[base+4 : base+8])
		count := int(binary.LittleEndian.Uint32(data[base+8 : base+12]))
		offset := int(binary.LittleEndian.Uint32(data[base+12 : base+16]))
		if count > 0 && offset >= 0 && offset+count <= len(data) {
			result.textures[index] = normalizeModelPath(strings.TrimRight(string(data[offset:offset+count]), "\x00"))
		}
	}
	for index := range result.textureCombos {
		result.textureCombos[index] = binary.LittleEndian.Uint16(data[combos.offset+index*2:])
	}
	for index := range result.textureCoords {
		result.textureCoords[index] = binary.LittleEndian.Uint16(data[textureCoords.offset+index*2:])
	}
	for index := range result.renderFlags {
		base := renderFlags.offset + index*m2RenderFlagSize
		result.renderFlags[index] = m2RenderFlag{flags: binary.LittleEndian.Uint16(data[base : base+2]), blend: binary.LittleEndian.Uint16(data[base+2 : base+4])}
	}
	for index := range result.particles {
		result.particles[index] = parseM2ParticleEmitter(data, particles.offset+index*m2ParticleSize)
	}
	if cameras.count > 0 {
		base := cameras.offset
		result.camera = &m2Camera{fov: readF32(data, base+4), farClip: readF32(data, base+8), nearClip: readF32(data, base+12), position: [3]float32{readF32(data, base+36), readF32(data, base+40), readF32(data, base+44)}, target: [3]float32{readF32(data, base+68), readF32(data, base+72), readF32(data, base+76)}}
	}
	return result, nil
}

type parsedSkin struct {
	vertices  []uint16
	indices   []uint16
	bones     [][4]uint8
	submeshes []skinSubmesh
	batches   []skinBatch
}

func parseSkin(data []byte) (parsedSkin, error) {
	if len(data) < 0x30 || string(data[:4]) != "SKIN" {
		return parsedSkin{}, fmt.Errorf("invalid SKIN header")
	}
	vertices, err := readSkinArray(data, 0x04, 2)
	if err != nil {
		return parsedSkin{}, err
	}
	indices, err := readSkinArray(data, 0x0c, 2)
	if err != nil {
		return parsedSkin{}, err
	}
	bones, err := readSkinArray(data, 0x14, 4)
	if err != nil {
		return parsedSkin{}, err
	}
	submeshes, err := readSkinArray(data, 0x1c, skinSubmeshSize)
	if err != nil {
		return parsedSkin{}, err
	}
	batches, err := readSkinArray(data, 0x24, skinBatchSize)
	if err != nil {
		return parsedSkin{}, err
	}
	result := parsedSkin{vertices: make([]uint16, vertices.count), indices: make([]uint16, indices.count), bones: make([][4]uint8, bones.count), submeshes: make([]skinSubmesh, submeshes.count), batches: make([]skinBatch, batches.count)}
	for index := range result.vertices {
		result.vertices[index] = binary.LittleEndian.Uint16(data[vertices.offset+index*2:])
	}
	for index := range result.indices {
		result.indices[index] = binary.LittleEndian.Uint16(data[indices.offset+index*2:])
	}
	for index := range result.bones {
		base := bones.offset + index*4
		result.bones[index] = [4]uint8{data[base], data[base+1], data[base+2], data[base+3]}
	}
	for index := range result.submeshes {
		base := submeshes.offset + index*skinSubmeshSize
		result.submeshes[index] = skinSubmesh{vertexStart: uint32(binary.LittleEndian.Uint16(data[base+4 : base+6])), indexStart: uint32(binary.LittleEndian.Uint16(data[base+8 : base+10])), indexCount: uint32(binary.LittleEndian.Uint16(data[base+10 : base+12])), boneCount: uint32(binary.LittleEndian.Uint16(data[base+12 : base+14])), boneComboIndex: uint32(binary.LittleEndian.Uint16(data[base+14 : base+16])), boneInfluences: uint32(binary.LittleEndian.Uint16(data[base+16 : base+18]))}
	}
	for index := range result.batches {
		base := batches.offset + index*skinBatchSize
		result.batches[index] = skinBatch{priorityPlane: int8(data[base+1]), submeshIndex: binary.LittleEndian.Uint16(data[base+4 : base+6]), materialLayer: binary.LittleEndian.Uint16(data[base+12 : base+14]), textureCount: binary.LittleEndian.Uint16(data[base+14 : base+16]), textureComboIndex: binary.LittleEndian.Uint16(data[base+16 : base+18])}
		result.batches[index].materialIndex = binary.LittleEndian.Uint16(data[base+10 : base+12])
		result.batches[index].textureCoordIndex = binary.LittleEndian.Uint16(data[base+18 : base+20])
	}
	return result, nil
}

func buildM2Parts(model parsedM2, skin parsedSkin) map[string]*m2Part {
	parts := make(map[string]*m2Part)
	type poseKey struct {
		local          int
		boneComboIndex uint32
	}
	posed := make(map[poseKey]posedM2Vertex)
	for batchIndex, batch := range skin.batches {
		if int(batch.submeshIndex) >= len(skin.submeshes) {
			continue
		}
		submesh := skin.submeshes[batch.submeshIndex]
		start := int(submesh.indexStart)
		end := start + int(submesh.indexCount)
		if start < 0 || end > len(skin.indices) || start >= end || end-start < 3 {
			continue
		}
		texturePaths := make([]string, 0, batch.textureCount)
		textureFlags := make([]uint32, 0, batch.textureCount)
		uvSet := 0
		if int(batch.textureCoordIndex) < len(model.textureCoords) {
			uvSet = int(model.textureCoords[batch.textureCoordIndex])
			if uvSet > 1 {
				uvSet = 0
			}
		}
		for textureIndex := 0; textureIndex < int(batch.textureCount); textureIndex++ {
			comboIndex := int(batch.textureComboIndex) + textureIndex
			if comboIndex >= len(model.textureCombos) {
				break
			}
			modelTextureIndex := model.textureCombos[comboIndex]
			if int(modelTextureIndex) >= len(model.textures) || model.textures[modelTextureIndex] == "" {
				continue
			}
			texturePaths = append(texturePaths, model.textures[modelTextureIndex])
			textureFlags = append(textureFlags, model.textureFlags[modelTextureIndex])
		}
		if len(texturePaths) == 0 {
			continue
		}
		materialInfo := m2RenderFlag{}
		if int(batch.materialIndex) < len(model.renderFlags) {
			materialInfo = model.renderFlags[batch.materialIndex]
		}
		uvSets := make([]int, len(texturePaths))
		for textureIndex := range texturePaths {
			coordIndex := int(batch.textureCoordIndex) + textureIndex
			if coordIndex < len(model.textureCoords) {
				uvSets[textureIndex] = int(model.textureCoords[coordIndex])
				if uvSets[textureIndex] > 1 {
					uvSets[textureIndex] = 0
				}
			}
		}
		if len(uvSets) > 0 {
			uvSet = uvSets[0]
		}
		key := fmt.Sprintf("%d\x00%d\x00%d\x00%d\x00%s", batchIndex, uvSet, materialInfo.flags, materialInfo.blend, strings.Join(texturePaths, "\x00"))
		part := parts[key]
		if part == nil {
			part = &m2Part{texturePaths: texturePaths, textureFlags: textureFlags, uvSets: uvSets, renderOrder: batchIndex, priorityPlane: batch.priorityPlane, materialLayer: batch.materialLayer, uvSet: uvSet, material: materialInfo}
			parts[key] = part
		}
		for index := start; index+2 < end; index += 3 {
			for corner := 0; corner < 3; corner++ {
				local := int(skin.indices[index+corner])
				if local < 0 || local >= len(skin.vertices) {
					continue
				}
				vertexIndex := int(skin.vertices[local])
				if vertexIndex < 0 || vertexIndex >= len(model.vertices) {
					continue
				}
				vertex := model.vertices[vertexIndex]
				key := poseKey{local: local, boneComboIndex: submesh.boneComboIndex}
				transformed, ok := posed[key]
				if !ok {
					transformed = poseM2Vertex(model, skin, local, vertex, int(submesh.boneComboIndex))
					posed[key] = transformed
				}
				position := modelVector(transformed.position)
				normal := modelVector(transformed.normal)
				part.positions.Append(position[0], position[1], position[2])
				part.normals.Append(normal[0], normal[1], normal[2])
				uv := vertex.uv
				if part.uvSet == 1 {
					uv = vertex.uv2
				}
				part.uvs.Append(uv[0], uv[1])
				if len(part.uvSets) > 1 {
					uv2 := vertex.uv
					if part.uvSets[1] == 1 {
						uv2 = vertex.uv2
					}
					part.uvs2.Append(uv2[0], uv2[1])
				}
				part.indices.Append(uint32(len(part.positions)/3 - 1))
			}
		}
	}
	return parts
}

func m2RenderOrder(part *m2Part) int {
	if part.material.blend < 2 {
		return -100 + part.renderOrder
	}
	return int(part.priorityPlane)*1000000 + int(part.materialLayer)*10000 + int(part.material.blend)*100 + part.renderOrder
}

func loadModelTexture(loader *ui.Loader, path string) *texture.Texture2D {
	data, err := loader.ReadAsset(path)
	if err != nil {
		return nil
	}
	img, err := ui.DecodeBLP(data)
	if err != nil {
		return nil
	}
	bounds := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	if src, ok := img.(*image.NRGBA); ok {
		for y := 0; y < bounds.Dy(); y++ {
			srcStart := (bounds.Min.Y+y)*src.Stride + bounds.Min.X*4
			dstStart := y * rgba.Stride
			copy(rgba.Pix[dstStart:dstStart+rgba.Stride], src.Pix[srcStart:srcStart+rgba.Stride])
		}
	} else {
		draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)
	}
	return texture.NewTexture2DFromRGBA(rgba)
}

func readM2Array(data []byte, offset, stride int) (m2Array, error) {
	if offset < 0 || offset+8 > len(data) {
		return m2Array{}, fmt.Errorf("M2 array header out of range at %#x", offset)
	}
	array := m2Array{count: int(binary.LittleEndian.Uint32(data[offset : offset+4])), offset: int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))}
	if array.count < 0 || array.count > 1<<24 || array.offset < 0 || array.count > (len(data)-array.offset)/stride {
		return m2Array{}, fmt.Errorf("M2 array out of range at %#x count=%d offset=%#x", offset, array.count, array.offset)
	}
	return array, nil
}

func readSkinArray(data []byte, offset, stride int) (m2Array, error) {
	if offset < 0 || offset+8 > len(data) {
		return m2Array{}, fmt.Errorf("SKIN array header out of range at %#x", offset)
	}
	array := m2Array{count: int(binary.LittleEndian.Uint32(data[offset : offset+4])), offset: int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))}
	if array.count < 0 || array.count > 1<<24 || array.offset < 0 || array.count > (len(data)-array.offset)/stride {
		return m2Array{}, fmt.Errorf("SKIN array out of range at %#x count=%d offset=%#x", offset, array.count, array.offset)
	}
	return array, nil
}

func readF32(data []byte, offset int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
}

func readM2TrackKey(data []byte, offset, valueSize int) (int, bool) {
	if offset < 0 || offset+20 > len(data) {
		return 0, false
	}
	valueCount := int(binary.LittleEndian.Uint32(data[offset+12 : offset+16]))
	valueHeaders := int(binary.LittleEndian.Uint32(data[offset+16 : offset+20]))
	if valueCount <= 0 || valueHeaders < 0 || valueHeaders+8 > len(data) {
		return 0, false
	}
	count := int(binary.LittleEndian.Uint32(data[valueHeaders : valueHeaders+4]))
	keyOffset := int(binary.LittleEndian.Uint32(data[valueHeaders+4 : valueHeaders+8]))
	if count <= 0 || keyOffset < 0 || valueSize < 0 || keyOffset+valueSize > len(data) {
		return 0, false
	}
	return keyOffset, true
}

func readM2TrackVector(data []byte, offset int, fallback [3]float32) [3]float32 {
	key, ok := readM2TrackKey(data, offset, 12)
	if !ok {
		return fallback
	}
	return [3]float32{readF32(data, key), readF32(data, key+4), readF32(data, key+8)}
}

func readM2TrackQuaternion(data []byte, offset int) [4]float32 {
	key, ok := readM2TrackKey(data, offset, 8)
	if !ok {
		return [4]float32{0, 0, 0, 1}
	}
	quaternion := [4]float32{decodeM2Quaternion(binary.LittleEndian.Uint16(data[key : key+2])), decodeM2Quaternion(binary.LittleEndian.Uint16(data[key+2 : key+4])), decodeM2Quaternion(binary.LittleEndian.Uint16(data[key+4 : key+6])), decodeM2Quaternion(binary.LittleEndian.Uint16(data[key+6 : key+8]))}
	length := float32(math.Sqrt(float64(quaternion[0]*quaternion[0] + quaternion[1]*quaternion[1] + quaternion[2]*quaternion[2] + quaternion[3]*quaternion[3])))
	if length < 0.0001 {
		return [4]float32{0, 0, 0, 1}
	}
	return [4]float32{quaternion[0] / length, quaternion[1] / length, quaternion[2] / length, quaternion[3] / length}
}

func decodeM2Quaternion(value uint16) float32 {
	decoded := int32(int16(value))
	if decoded < 0 {
		return float32(decoded+32768) / 32767
	}
	return float32(decoded-32767) / 32767
}

func poseM2Vertex(model parsedM2, skin parsedSkin, local int, vertex m2Vertex, boneComboIndex int) posedM2Vertex {
	weights := vertex.weights
	bones := vertex.bones
	if local >= 0 && local < len(skin.bones) {
		bones = skin.bones[local]
	}
	var position, normal [3]float32
	weightTotal := 0
	for slot, weight := range weights {
		if weight == 0 {
			continue
		}
		boneIndex := int(bones[slot])
		if boneComboIndex >= 0 && boneComboIndex+boneIndex < len(model.boneCombos) {
			boneIndex = int(model.boneCombos[boneComboIndex+boneIndex])
		}
		if boneIndex >= len(model.bones) {
			continue
		}
		weightTotal += int(weight)
		bonePosition := transformM2Point(boneIndex, vertex.position, model.bones, 0)
		boneNormal := transformM2Normal(boneIndex, vertex.normal, model.bones, 0)
		factor := float32(weight) / 255
		for axis := 0; axis < 3; axis++ {
			position[axis] += bonePosition[axis] * factor
			normal[axis] += boneNormal[axis] * factor
		}
	}
	if weightTotal == 0 {
		return posedM2Vertex{position: vertex.position, normal: vertex.normal}
	}
	if weightTotal != 255 {
		factor := 255 / float32(weightTotal)
		for axis := 0; axis < 3; axis++ {
			position[axis] *= factor
			normal[axis] *= factor
		}
	}
	return posedM2Vertex{position: position, normal: normal}
}

func transformM2Point(index int, point [3]float32, bones []m2Bone, depth int) [3]float32 {
	if index < 0 || index >= len(bones) || depth > len(bones) {
		return point
	}
	bone := bones[index]
	point[0] -= bone.pivot[0]
	point[1] -= bone.pivot[1]
	point[2] -= bone.pivot[2]
	point = rotateM2Vector(bone.rotation, point)
	point[0] *= bone.scale[0]
	point[1] *= bone.scale[1]
	point[2] *= bone.scale[2]
	point[0] += bone.pivot[0] + bone.translation[0]
	point[1] += bone.pivot[1] + bone.translation[1]
	point[2] += bone.pivot[2] + bone.translation[2]
	if bone.parent >= 0 {
		return transformM2Point(int(bone.parent), point, bones, depth+1)
	}
	return point
}

func transformM2Normal(index int, normal [3]float32, bones []m2Bone, depth int) [3]float32 {
	if index < 0 || index >= len(bones) || depth > len(bones) {
		return normal
	}
	bone := bones[index]
	normal = rotateM2Vector(bone.rotation, normal)
	if bone.parent >= 0 {
		return transformM2Normal(int(bone.parent), normal, bones, depth+1)
	}
	return normal
}

func rotateM2Vector(quaternion [4]float32, value [3]float32) [3]float32 {
	qx, qy, qz, qw := quaternion[0], quaternion[1], quaternion[2], quaternion[3]
	ux := qy*value[2] - qz*value[1]
	uy := qz*value[0] - qx*value[2]
	uz := qx*value[1] - qy*value[0]
	uux := qy*uz - qz*uy
	uuy := qz*ux - qx*uz
	uuz := qx*uy - qy*ux
	return [3]float32{value[0] + 2*(qw*ux+uux), value[1] + 2*(qw*uy+uuy), value[2] + 2*(qw*uz+uuz)}
}

func modelVector(value [3]float32) [3]float32 {
	return [3]float32{value[0], value[2], -value[1]}
}

func modelPoint(value [3]float32, center [3]float32, scale float32) [3]float32 {
	point := modelVector(value)
	return [3]float32{(point[0] - center[0]) * scale, (point[1] - center[1]) * scale, (point[2] - center[2]) * scale}
}

func modelTransform(vertices []m2Vertex) ([3]float32, float32) {
	var min, max [3]float32
	if len(vertices) == 0 {
		return [3]float32{}, 1
	}
	first := modelVector(vertices[0].position)
	min, max = first, first
	for _, vertex := range vertices[1:] {
		position := modelVector(vertex.position)
		for axis := 0; axis < 3; axis++ {
			if position[axis] < min[axis] {
				min[axis] = position[axis]
			}
			if position[axis] > max[axis] {
				max[axis] = position[axis]
			}
		}
	}
	center := [3]float32{(min[0] + max[0]) / 2, (min[1] + max[1]) / 2, (min[2] + max[2]) / 2}
	maxDimension := float32(0)
	for axis := 0; axis < 3; axis++ {
		if span := max[axis] - min[axis]; span > maxDimension {
			maxDimension = span
		}
	}
	if maxDimension == 0 {
		return center, 1
	}
	return center, 3 / maxDimension
}

func normalizeModelPath(path string) string {
	path = strings.ReplaceAll(strings.TrimSpace(strings.TrimRight(path, "\x00")), "/", "\\")
	return strings.TrimPrefix(path, "\\")
}
