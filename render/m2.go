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
	m2Version264           = 264
	m2VertexSize           = 48
	m2CameraSize           = 100
	m2ParticleSize         = 476
	m2ColorSize            = 40
	m2TextureWeightSize    = 20
	m2TextureTransformSize = 60
	m2RenderFlagSize       = 4
	skinSubmeshSize        = 48
	skinBatchSize          = 24
)

const (
	m2CombinerOpaque uint16 = iota
	m2CombinerMod
	m2CombinerDecal
	m2CombinerAdd
	m2CombinerMod2X
	m2CombinerFade
	m2CombinerMod2XNA
	m2CombinerAddNA
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
	flags            uint32
	parent           int16
	pivot            [3]float32
	translation      [3]float32
	rotation         [4]float32
	scale            [3]float32
	translationTrack m2TrackVec3
	rotationTrack    m2TrackQuat
	scaleTrack       m2TrackVec3
}

type m2Sequence struct {
	id            uint16
	variation     uint16
	duration      uint32
	flags         uint32
	variationNext int16
	aliasNext     uint16
}

type m2Vec3Keys struct {
	times  []uint32
	values [][3]float32
}

type m2QuatKeys struct {
	times  []uint32
	values [][4]float32
}

type m2TrackVec3 struct {
	interpolation  uint16
	globalSequence uint16
	sequences      []m2Vec3Keys
}

type m2TrackQuat struct {
	interpolation  uint16
	globalSequence uint16
	sequences      []m2QuatKeys
}

type m2ScalarKeys struct {
	times  []uint32
	values []float32
}

type m2TrackScalar struct {
	interpolation  uint16
	globalSequence uint16
	sequences      []m2ScalarKeys
}

type m2Color struct {
	colorTrack   m2TrackVec3
	alphaTrack   m2TrackScalar
	current      [3]float32
	currentAlpha float32
}

type m2TextureWeight struct {
	weightTrack m2TrackScalar
	current     float32
}

type m2TextureTransform struct {
	translation m2TrackVec3
	rotation    m2TrackQuat
	scale       m2TrackVec3
}

type m2Event struct {
	identifier [4]byte
	data       uint32
	bone       uint32
	position   [3]float32
	times      [][]uint32
}

type skinSubmesh struct {
	submeshID      uint16
	submeshLevel   uint16
	vertexStart    uint32
	indexStart     uint32
	indexCount     uint32
	boneCount      uint32
	boneComboIndex uint32
	boneInfluences uint32
}

type skinBatch struct {
	flags                 uint8
	priorityPlane         int8
	shader                uint16
	submeshIndex          uint16
	colorIndex            uint16
	materialIndex         uint16
	materialLayer         uint16
	textureCount          uint16
	textureComboIndex     uint16
	textureCoordIndex     uint16
	textureWeightIndex    uint16
	textureTransformIndex uint16
}

type m2Part struct {
	texturePaths            []string
	textureFlags            []uint32
	uvSets                  []int
	positions               math32.ArrayF32
	normals                 math32.ArrayF32
	uvs                     math32.ArrayF32
	uvs2                    math32.ArrayF32
	indices                 math32.ArrayU32
	renderOrder             int
	priorityPlane           int8
	materialLayer           uint16
	submeshID               uint16
	uvSet                   int
	material                m2RenderFlag
	colorIndex              int
	textureWeightIndex      int
	color                   [3]float32
	alpha                   float32
	colors                  math32.ArrayF32
	alphas                  math32.ArrayF32
	textureCombiners        [2]float32
	combiners               math32.ArrayF32
	vertexRefs              []m2VertexRef
	textureTransformIndices []int
}

type m2VertexRef struct {
	local          int
	vertex         m2Vertex
	boneComboIndex int
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

type m2Attachment struct {
	id       uint32
	bone     uint16
	position [3]float32
}

type glueModelInfo struct {
	position      math32.Vector3
	target        math32.Vector3
	standPosition math32.Vector3
	modelScale    float32
	fov           float32
	far           float32
	near          float32
	hasStand      bool
	modelBottom   float32
	stats         glueModelStats
	particles     *m2ParticleSystem
	animation     *m2Animation
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
	loadM2AnimationTracks(loader, modelPath, &model)
	skinPath := strings.TrimSuffix(modelPath, ".m2") + "00.skin"
	skinData, err := loader.ReadFile(skinPath)
	if err != nil {
		return nil, err
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		return nil, err
	}
	return buildGlueModel(loader, modelPath, model, skin, nil, nil, nil)
}

func buildGlueModel(loader *ui.Loader, modelPath string, model parsedM2, skin parsedSkin, textureOverrides map[int]string, preloaded map[string]*texture.Texture2D, activeGeosets map[uint16]bool) (*core.Node, error) {
	parts := buildM2PartsWithFilters(model, skin, textureOverrides, activeGeosets)
	if len(parts) == 0 {
		return nil, fmt.Errorf("%s: no renderable skin batches", modelPath)
	}
	root := core.NewNode()
	textures := make(map[string]*texture.Texture2D)
	texturePaths := make(map[string]struct{})
	stats := glueModelStats{}
	animatedMeshes := make([]*m2AnimatedMesh, 0, len(parts))
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
		positionVBO := gls.NewVBO(part.positions).AddAttrib(gls.VertexPosition)
		normalVBO := gls.NewVBO(part.normals).AddAttrib(gls.VertexNormal)
		geom.AddVBO(positionVBO)
		geom.AddVBO(normalVBO)
		uvVBO := gls.NewVBO(part.uvs).AddAttrib(gls.VertexTexcoord)
		geom.AddVBO(uvVBO)
		var uv2VBO *gls.VBO
		if len(part.uvSets) > 1 {
			uv2VBO = gls.NewVBO(part.uvs2).AddCustomAttrib("VertexTexcoord2", 2)
			geom.AddVBO(uv2VBO)
		}
		colorVBO := gls.NewVBO(part.colors).AddAttrib(gls.VertexColor)
		alphaVBO := gls.NewVBO(part.alphas).AddCustomAttrib("VertexM2Alpha", 1)
		combinerVBO := gls.NewVBO(part.combiners).AddCustomAttrib("VertexM2Combiner", 2)
		geom.AddVBO(colorVBO)
		geom.AddVBO(alphaVBO)
		geom.AddVBO(combinerVBO)
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
			tex := preloaded[texturePath]
			if tex == nil {
				tex = textures[texturePath]
			}
			if tex == nil {
				tex = loadModelTexture(loader, texturePath)
				if tex != nil {
					textures[texturePath] = tex
				}
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
		animatedMeshes = append(animatedMeshes, &m2AnimatedMesh{part: part, positionVBO: positionVBO, normalVBO: normalVBO, uvVBO: uvVBO, uv2VBO: uv2VBO, colorVBO: colorVBO, alphaVBO: alphaVBO, baseUVs: append(math32.ArrayF32(nil), part.uvs...), baseUVs2: append(math32.ArrayF32(nil), part.uvs2...), textureTransformIndices: append([]int(nil), part.textureTransformIndices...)})
	}
	if len(root.Children()) == 0 {
		return nil, fmt.Errorf("%s: model textures or geometry unavailable", modelPath)
	}
	center, scale := modelTransform(model.vertices)
	root.SetPosition(-center[0]*scale, -center[1]*scale, -center[2]*scale)
	root.SetScale(scale, scale, scale)
	modelBottom := float32(math.MaxFloat32)
	for _, part := range parts {
		for index := 1; index < len(part.positions); index += 3 {
			if bottom := (part.positions[index] - center[1]) * scale; bottom < modelBottom {
				modelBottom = bottom
			}
		}
	}
	stats.textures = len(texturePaths)
	particles := buildM2ParticleSystem(loader, &model, root, scale, textures)
	if particles != nil {
		stats.particleEmitters = particles.emitterCount
		stats.particlePoints = particles.pointCount
	}
	animation := buildM2Animation(&model, skin, animatedMeshes, modelPath)
	info := glueModelInfo{stats: stats, particles: particles, animation: animation, modelScale: scale, modelBottom: modelBottom}
	for _, attachment := range model.attachments {
		if attachment.id == 0 {
			point := modelPoint(attachment.position, center, scale)
			info.standPosition = *math32.NewVector3(point[0], point[1], point[2])
			info.hasStand = true
			break
		}
	}
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
	data                   []byte
	flags                  uint32
	animationSequence      int
	animationTime          uint32
	animationGlobalTime    uint32
	boneOffset             int
	colorOffset            int
	textureWeightOffset    int
	textureTransformOffset int
	vertices               []m2Vertex
	bones                  []m2Bone
	colors                 []m2Color
	sequences              []m2Sequence
	globalLoops            []uint32
	boneCombos             []uint16
	textures               []string
	textureTypes           []uint32
	textureCombos          []uint16
	textureCombinerCombos  []uint16
	textureFlags           []uint32
	textureCoords          []uint16
	textureWeightCombos    []uint16
	textureWeights         []m2TextureWeight
	textureTransforms      []m2TextureTransform
	textureTransformCombos []uint16
	renderFlags            []m2RenderFlag
	attachments            []m2Attachment
	camera                 *m2Camera
	particles              []m2ParticleEmitter
	events                 []m2Event
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
	colors, err := readM2Array(data, 0x48, m2ColorSize)
	if err != nil {
		return parsedM2{}, err
	}
	textureWeights, err := readM2Array(data, 0x58, m2TextureWeightSize)
	if err != nil {
		return parsedM2{}, err
	}
	globalLoops, err := readM2Array(data, 0x14, 4)
	if err != nil {
		return parsedM2{}, err
	}
	sequences, err := readM2Array(data, 0x1c, 64)
	if err != nil {
		return parsedM2{}, err
	}
	events, err := readM2Array(data, 0x100, 36)
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
	textureWeightCombos, err := readM2Array(data, 0x90, 2)
	if err != nil {
		return parsedM2{}, err
	}
	flags := binary.LittleEndian.Uint32(data[0x10:0x14])
	textureCombinerCombos := m2Array{}
	if flags&0x08 != 0 {
		textureCombinerCombos, err = readM2Array(data, 0x130, 2)
		if err != nil {
			return parsedM2{}, err
		}
	}
	renderFlags, err := readM2Array(data, 0x70, m2RenderFlagSize)
	if err != nil {
		return parsedM2{}, err
	}
	textureTransforms, err := readM2Array(data, 0x60, m2TextureTransformSize)
	if err != nil {
		return parsedM2{}, err
	}
	textureTransformCombos, err := readM2Array(data, 0x98, 2)
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
	attachments, err := readM2Array(data, 0xf0, 40)
	if err != nil {
		return parsedM2{}, err
	}
	result := parsedM2{data: data, flags: flags, boneOffset: bones.offset, colorOffset: colors.offset, textureWeightOffset: textureWeights.offset, textureTransformOffset: textureTransforms.offset, vertices: make([]m2Vertex, vertices.count), bones: make([]m2Bone, bones.count), colors: make([]m2Color, colors.count), sequences: make([]m2Sequence, sequences.count), globalLoops: make([]uint32, globalLoops.count), boneCombos: make([]uint16, boneCombos.count), textures: make([]string, textures.count), textureTypes: make([]uint32, textures.count), textureFlags: make([]uint32, textures.count), textureCombos: make([]uint16, combos.count), textureCombinerCombos: make([]uint16, textureCombinerCombos.count), textureCoords: make([]uint16, textureCoords.count), textureWeightCombos: make([]uint16, textureWeightCombos.count), textureWeights: make([]m2TextureWeight, textureWeights.count), textureTransforms: make([]m2TextureTransform, textureTransforms.count), textureTransformCombos: make([]uint16, textureTransformCombos.count), renderFlags: make([]m2RenderFlag, renderFlags.count), attachments: make([]m2Attachment, attachments.count), particles: make([]m2ParticleEmitter, particles.count), events: make([]m2Event, events.count)}
	for index := range result.globalLoops {
		result.globalLoops[index] = binary.LittleEndian.Uint32(data[globalLoops.offset+index*4:])
	}
	for index := range result.sequences {
		base := sequences.offset + index*64
		result.sequences[index] = m2Sequence{id: binary.LittleEndian.Uint16(data[base : base+2]), variation: binary.LittleEndian.Uint16(data[base+2 : base+4]), duration: binary.LittleEndian.Uint32(data[base+4 : base+8]), flags: binary.LittleEndian.Uint32(data[base+12 : base+16]), variationNext: int16(binary.LittleEndian.Uint16(data[base+60 : base+62])), aliasNext: binary.LittleEndian.Uint16(data[base+62 : base+64])}
	}
	inline := make([]bool, len(result.sequences))
	for index, sequence := range result.sequences {
		inline[index] = sequence.flags&0x20 != 0
	}
	for index := range result.colors {
		base := colors.offset + index*m2ColorSize
		result.colors[index] = m2Color{colorTrack: readM2TrackVec3(data, base, len(result.sequences), nil, inline), alphaTrack: readM2TrackScalar(data, base+20, len(result.sequences), nil, inline)}
	}
	for index := range result.textureWeights {
		base := textureWeights.offset + index*m2TextureWeightSize
		result.textureWeights[index] = m2TextureWeight{weightTrack: readM2TrackScalar(data, base, len(result.sequences), nil, inline)}
	}
	for index := range result.textureTransforms {
		base := textureTransforms.offset + index*m2TextureTransformSize
		result.textureTransforms[index] = m2TextureTransform{translation: readM2TrackVec3(data, base, len(result.sequences), nil, inline), rotation: readM2TrackQuat(data, base+20, len(result.sequences), nil, inline), scale: readM2TrackVec3(data, base+40, len(result.sequences), nil, inline)}
	}
	for index := range result.vertices {
		base := vertices.offset + index*m2VertexSize
		result.vertices[index] = m2Vertex{position: [3]float32{readF32(data, base), readF32(data, base+4), readF32(data, base+8)}, weights: [4]uint8{data[base+12], data[base+13], data[base+14], data[base+15]}, bones: [4]uint8{data[base+16], data[base+17], data[base+18], data[base+19]}, normal: [3]float32{readF32(data, base+20), readF32(data, base+24), readF32(data, base+28)}, uv: [2]float32{readF32(data, base+32), readF32(data, base+36)}, uv2: [2]float32{readF32(data, base+40), readF32(data, base+44)}}
	}
	for index := range result.boneCombos {
		result.boneCombos[index] = binary.LittleEndian.Uint16(data[boneCombos.offset+index*2:])
	}
	for index := range result.bones {
		base := bones.offset + index*88
		translationTrack := readM2TrackVec3(data, base+16, len(result.sequences), nil, inline)
		rotationTrack := readM2TrackQuat(data, base+36, len(result.sequences), nil, inline)
		scaleTrack := readM2TrackVec3(data, base+56, len(result.sequences), nil, inline)
		result.bones[index] = m2Bone{flags: binary.LittleEndian.Uint32(data[base+4 : base+8]), parent: int16(binary.LittleEndian.Uint16(data[base+8 : base+10])), pivot: [3]float32{readF32(data, base+76), readF32(data, base+80), readF32(data, base+84)}, translation: translationTrack.value(0, 0, result.globalLoops, [3]float32{}), rotation: rotationTrack.value(0, 0, result.globalLoops, [4]float32{0, 0, 0, 1}), scale: scaleTrack.value(0, 0, result.globalLoops, [3]float32{1, 1, 1}), translationTrack: translationTrack, rotationTrack: rotationTrack, scaleTrack: scaleTrack}
	}
	for index := range result.textures {
		base := textures.offset + index*16
		result.textureTypes[index] = binary.LittleEndian.Uint32(data[base : base+4])
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
	for index := range result.textureCombinerCombos {
		result.textureCombinerCombos[index] = binary.LittleEndian.Uint16(data[textureCombinerCombos.offset+index*2:])
	}
	for index := range result.textureCoords {
		result.textureCoords[index] = binary.LittleEndian.Uint16(data[textureCoords.offset+index*2:])
	}
	for index := range result.textureWeightCombos {
		result.textureWeightCombos[index] = binary.LittleEndian.Uint16(data[textureWeightCombos.offset+index*2:])
	}
	for index := range result.textureTransformCombos {
		result.textureTransformCombos[index] = binary.LittleEndian.Uint16(data[textureTransformCombos.offset+index*2:])
	}
	for index := range result.renderFlags {
		base := renderFlags.offset + index*m2RenderFlagSize
		result.renderFlags[index] = m2RenderFlag{flags: binary.LittleEndian.Uint16(data[base : base+2]), blend: binary.LittleEndian.Uint16(data[base+2 : base+4])}
	}
	for index := range result.attachments {
		base := attachments.offset + index*40
		result.attachments[index] = m2Attachment{id: binary.LittleEndian.Uint32(data[base : base+4]), bone: binary.LittleEndian.Uint16(data[base+4 : base+6]), position: [3]float32{readF32(data, base+8), readF32(data, base+12), readF32(data, base+16)}}
	}
	for index := range result.particles {
		base := particles.offset + index*m2ParticleSize
		emitter := parseM2ParticleEmitter(data, base)
		emitter.rateTrack = readM2TrackFloatTrack(data, base+0xb0, len(result.sequences), nil, inline)
		result.particles[index] = emitter
	}
	result.events = readM2Events(data, events, len(result.sequences), nil, inline)
	if cameras.count > 0 {
		base := cameras.offset
		result.camera = &m2Camera{fov: readF32(data, base+4), farClip: readF32(data, base+8), nearClip: readF32(data, base+12), position: [3]float32{readF32(data, base+36), readF32(data, base+40), readF32(data, base+44)}, target: [3]float32{readF32(data, base+68), readF32(data, base+72), readF32(data, base+76)}}
	}
	updateM2AnimatedValues(&result, defaultM2Sequence(&result), 0, 0)
	return result, nil
}

type parsedSkin struct {
	vertices  []uint16
	indices   []uint16
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
	submeshes, err := readSkinArray(data, 0x1c, skinSubmeshSize)
	if err != nil {
		return parsedSkin{}, err
	}
	batches, err := readSkinArray(data, 0x24, skinBatchSize)
	if err != nil {
		return parsedSkin{}, err
	}
	result := parsedSkin{vertices: make([]uint16, vertices.count), indices: make([]uint16, indices.count), submeshes: make([]skinSubmesh, submeshes.count), batches: make([]skinBatch, batches.count)}
	for index := range result.vertices {
		result.vertices[index] = binary.LittleEndian.Uint16(data[vertices.offset+index*2:])
	}
	for index := range result.indices {
		result.indices[index] = binary.LittleEndian.Uint16(data[indices.offset+index*2:])
	}
	for index := range result.submeshes {
		base := submeshes.offset + index*skinSubmeshSize
		result.submeshes[index] = skinSubmesh{submeshID: binary.LittleEndian.Uint16(data[base : base+2]), submeshLevel: binary.LittleEndian.Uint16(data[base+2 : base+4]), vertexStart: uint32(binary.LittleEndian.Uint16(data[base+4 : base+6])), indexStart: uint32(binary.LittleEndian.Uint16(data[base+8 : base+10])), indexCount: uint32(binary.LittleEndian.Uint16(data[base+10 : base+12])), boneCount: uint32(binary.LittleEndian.Uint16(data[base+12 : base+14])), boneComboIndex: uint32(binary.LittleEndian.Uint16(data[base+14 : base+16])), boneInfluences: uint32(binary.LittleEndian.Uint16(data[base+16 : base+18]))}
	}
	for index := range result.batches {
		base := batches.offset + index*skinBatchSize
		result.batches[index] = skinBatch{flags: data[base], priorityPlane: int8(data[base+1]), shader: binary.LittleEndian.Uint16(data[base+2 : base+4]), submeshIndex: binary.LittleEndian.Uint16(data[base+4 : base+6]), colorIndex: binary.LittleEndian.Uint16(data[base+8 : base+10]), materialIndex: binary.LittleEndian.Uint16(data[base+10 : base+12]), materialLayer: binary.LittleEndian.Uint16(data[base+12 : base+14]), textureCount: binary.LittleEndian.Uint16(data[base+14 : base+16]), textureComboIndex: binary.LittleEndian.Uint16(data[base+16 : base+18]), textureCoordIndex: binary.LittleEndian.Uint16(data[base+18 : base+20]), textureWeightIndex: binary.LittleEndian.Uint16(data[base+20 : base+22]), textureTransformIndex: binary.LittleEndian.Uint16(data[base+22 : base+24])}
	}
	return result, nil
}

func buildM2Parts(model parsedM2, skin parsedSkin) map[string]*m2Part {
	return buildM2PartsWithFilters(model, skin, nil, nil)
}

func buildM2PartsWithOverrides(model parsedM2, skin parsedSkin, textureOverrides map[int]string) map[string]*m2Part {
	return buildM2PartsWithFilters(model, skin, textureOverrides, nil)
}

func buildM2PartsWithFilters(model parsedM2, skin parsedSkin, textureOverrides map[int]string, activeGeosets map[uint16]bool) map[string]*m2Part {
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
		if activeGeosets != nil && !activeGeosets[submesh.submeshID] {
			continue
		}
		start := int(submesh.indexStart)
		end := start + int(submesh.indexCount)
		if start < 0 || end > len(skin.indices) || start >= end || end-start < 3 {
			continue
		}
		texturePaths := make([]string, 0, batch.textureCount)
		textureFlags := make([]uint32, 0, batch.textureCount)
		textureTransformIndices := make([]int, 0, batch.textureCount)
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
			if int(modelTextureIndex) >= len(model.textures) {
				continue
			}
			texturePath := model.textures[modelTextureIndex]
			if texturePath == "" && textureOverrides != nil {
				texturePath = textureOverrides[int(modelTextureIndex)]
			}
			if texturePath == "" {
				continue
			}
			texturePaths = append(texturePaths, texturePath)
			textureFlags = append(textureFlags, model.textureFlags[modelTextureIndex])
			transformIndex := -1
			transformComboIndex := int(batch.textureTransformIndex) + textureIndex
			if transformComboIndex < len(model.textureTransformCombos) {
				value := model.textureTransformCombos[transformComboIndex]
				if value != 0xffff && int(value) < len(model.textureTransforms) {
					transformIndex = int(value)
				}
			}
			textureTransformIndices = append(textureTransformIndices, transformIndex)
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
			colorIndex := -1
			if batch.colorIndex != 0xffff && int(batch.colorIndex) < len(model.colors) {
				colorIndex = int(batch.colorIndex)
			}
			textureWeightIndex := -1
			if batch.textureCount > 0 && int(batch.textureWeightIndex) < len(model.textureWeightCombos) {
				value := model.textureWeightCombos[batch.textureWeightIndex]
				if value != 0xffff && int(value) < len(model.textureWeights) {
					textureWeightIndex = int(value)
				}
			}
			color, alpha := m2PartTintAt(&model, colorIndex, textureWeightIndex, 0, 0, 0)
			textureCombiners := m2TextureCombiners(model, batch, materialInfo, len(texturePaths))
			part = &m2Part{texturePaths: texturePaths, textureFlags: textureFlags, uvSets: uvSets, renderOrder: batchIndex, priorityPlane: batch.priorityPlane, materialLayer: batch.materialLayer, submeshID: submesh.submeshID, uvSet: uvSet, material: materialInfo, colorIndex: colorIndex, textureWeightIndex: textureWeightIndex, color: color, alpha: alpha, textureCombiners: textureCombiners, textureTransformIndices: textureTransformIndices}
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
				part.colors.Append(part.color[0], part.color[1], part.color[2])
				part.alphas.Append(part.alpha)
				part.combiners.Append(part.textureCombiners[0], part.textureCombiners[1])
				part.vertexRefs = append(part.vertexRefs, m2VertexRef{local: local, vertex: vertex, boneComboIndex: int(submesh.boneComboIndex)})
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

func readM2TrackHeader(data []byte, offset int) (uint16, uint16, m2Array, m2Array, bool) {
	if offset < 0 || offset+20 > len(data) {
		return 0, 0, m2Array{}, m2Array{}, false
	}
	return binary.LittleEndian.Uint16(data[offset : offset+2]), binary.LittleEndian.Uint16(data[offset+2 : offset+4]), m2Array{count: int(binary.LittleEndian.Uint32(data[offset+4 : offset+8])), offset: int(binary.LittleEndian.Uint32(data[offset+8 : offset+12]))}, m2Array{count: int(binary.LittleEndian.Uint32(data[offset+12 : offset+16])), offset: int(binary.LittleEndian.Uint32(data[offset+16 : offset+20]))}, true
}

func readM2TrackArray(data []byte, offset int) (m2Array, bool) {
	if offset < 0 || offset+8 > len(data) {
		return m2Array{}, false
	}
	array := m2Array{count: int(binary.LittleEndian.Uint32(data[offset : offset+4])), offset: int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))}
	if array.count < 0 || array.count > 1<<24 || array.offset < 0 {
		return m2Array{}, false
	}
	return array, true
}

func readM2TrackVec3(data []byte, offset, sequenceCount int, external map[int][]byte, inline []bool) m2TrackVec3 {
	interpolation, globalSequence, timesOuter, valuesOuter, ok := readM2TrackHeader(data, offset)
	if !ok || sequenceCount < 0 {
		return m2TrackVec3{}
	}
	track := m2TrackVec3{interpolation: interpolation, globalSequence: globalSequence, sequences: make([]m2Vec3Keys, sequenceCount)}
	count := sequenceCount
	if timesOuter.count < count {
		count = timesOuter.count
	}
	if valuesOuter.count < count {
		count = valuesOuter.count
	}
	for index := 0; index < count; index++ {
		if !inline[index] {
			if source, exists := external[index]; exists {
				track.sequences[index] = readM2Vec3Keys(data, timesOuter, valuesOuter, index, source)
			}
			continue
		}
		track.sequences[index] = readM2Vec3Keys(data, timesOuter, valuesOuter, index, data)
	}
	return track
}

func m2PartTintAt(model *parsedM2, colorIndex, textureWeightIndex int, sequence int, timeMS, globalTimeMS uint32) ([3]float32, float32) {
	color := [3]float32{1, 1, 1}
	alpha := float32(1)
	if model != nil && colorIndex >= 0 && colorIndex < len(model.colors) {
		entry := model.colors[colorIndex]
		color = entry.colorTrack.value(sequence, m2TrackTime(entry.colorTrack.globalSequence, timeMS, globalTimeMS), model.globalLoops, color)
		if value := entry.alphaTrack.value(sequence, m2TrackTime(entry.alphaTrack.globalSequence, timeMS, globalTimeMS), model.globalLoops, alpha); value > 0.001 {
			alpha = value
		}
	}
	if model != nil && textureWeightIndex >= 0 && textureWeightIndex < len(model.textureWeights) {
		entry := model.textureWeights[textureWeightIndex]
		if value := entry.weightTrack.value(sequence, m2TrackTime(entry.weightTrack.globalSequence, timeMS, globalTimeMS), model.globalLoops, 1); value > 0.001 {
			alpha *= value
		}
	}
	for index := range color {
		color[index] = clampM2Color(color[index])
	}
	return color, clampM2Color(alpha)
}

func m2TextureCombiners(model parsedM2, batch skinBatch, material m2RenderFlag, textureCount int) [2]float32 {
	result := [2]float32{float32(m2CombinerMod), float32(m2CombinerMod)}
	if textureCount > 2 {
		textureCount = 2
	}
	for index := 0; index < textureCount; index++ {
		combiner := m2CombinerMod
		if model.flags&0x08 != 0 && int(batch.shader)+index < len(model.textureCombinerCombos) {
			combiner = model.textureCombinerCombos[int(batch.shader)+index] & 0x07
		} else if material.blend == 0 {
			combiner = m2CombinerOpaque
		}
		if index == 0 && material.blend == 0 {
			combiner = m2CombinerOpaque
		}
		result[index] = float32(combiner)
	}
	return result
}

func readM2TrackScalar(data []byte, offset, sequenceCount int, external map[int][]byte, inline []bool) m2TrackScalar {
	interpolation, globalSequence, timesOuter, valuesOuter, ok := readM2TrackHeader(data, offset)
	if !ok || sequenceCount < 0 {
		return m2TrackScalar{}
	}
	track := m2TrackScalar{interpolation: interpolation, globalSequence: globalSequence, sequences: make([]m2ScalarKeys, sequenceCount)}
	count := sequenceCount
	if timesOuter.count < count {
		count = timesOuter.count
	}
	if valuesOuter.count < count {
		count = valuesOuter.count
	}
	for index := 0; index < count; index++ {
		if !inline[index] {
			if source, exists := external[index]; exists {
				track.sequences[index] = readM2ScalarKeys(data, timesOuter, valuesOuter, index, source)
			}
			continue
		}
		track.sequences[index] = readM2ScalarKeys(data, timesOuter, valuesOuter, index, data)
	}
	return track
}

func readM2TrackFloatTrack(data []byte, offset, sequenceCount int, external map[int][]byte, inline []bool) m2TrackScalar {
	interpolation, globalSequence, timesOuter, valuesOuter, ok := readM2TrackHeader(data, offset)
	if !ok || sequenceCount < 0 {
		return m2TrackScalar{}
	}
	track := m2TrackScalar{interpolation: interpolation, globalSequence: globalSequence, sequences: make([]m2ScalarKeys, sequenceCount)}
	count := sequenceCount
	if timesOuter.count < count {
		count = timesOuter.count
	}
	if valuesOuter.count < count {
		count = valuesOuter.count
	}
	for index := 0; index < count; index++ {
		if !inline[index] {
			if source, exists := external[index]; exists {
				track.sequences[index] = readM2ScalarKeysFloat(data, timesOuter, valuesOuter, index, source)
			}
			continue
		}
		track.sequences[index] = readM2ScalarKeysFloat(data, timesOuter, valuesOuter, index, data)
	}
	return track
}

func readM2Vec3Keys(data []byte, timesOuter, valuesOuter m2Array, index int, source []byte) m2Vec3Keys {
	times, timesOK := readM2TrackArray(data, timesOuter.offset+index*8)
	values, valuesOK := readM2TrackArray(data, valuesOuter.offset+index*8)
	if !timesOK || !valuesOK {
		return m2Vec3Keys{}
	}
	count := times.count
	if values.count < count {
		count = values.count
	}
	if count <= 0 || times.offset < 0 || values.offset < 0 || times.offset > len(source) || values.offset > len(source) || count > (len(source)-times.offset)/4 || count > (len(source)-values.offset)/12 {
		return m2Vec3Keys{}
	}
	keys := m2Vec3Keys{times: make([]uint32, count), values: make([][3]float32, count)}
	for key := 0; key < count; key++ {
		keys.times[key] = binary.LittleEndian.Uint32(source[times.offset+key*4:])
		base := values.offset + key*12
		keys.values[key] = [3]float32{readF32(source, base), readF32(source, base+4), readF32(source, base+8)}
	}
	return keys
}

func readM2ScalarKeys(data []byte, timesOuter, valuesOuter m2Array, index int, source []byte) m2ScalarKeys {
	times, timesOK := readM2TrackArray(data, timesOuter.offset+index*8)
	values, valuesOK := readM2TrackArray(data, valuesOuter.offset+index*8)
	if !timesOK || !valuesOK {
		return m2ScalarKeys{}
	}
	count := times.count
	if values.count < count {
		count = values.count
	}
	if count <= 0 || times.offset < 0 || values.offset < 0 || times.offset > len(data) || values.offset > len(source) || count > (len(data)-times.offset)/4 || count > (len(source)-values.offset)/2 {
		return m2ScalarKeys{}
	}
	keys := m2ScalarKeys{times: make([]uint32, count), values: make([]float32, count)}
	for key := 0; key < count; key++ {
		keys.times[key] = binary.LittleEndian.Uint32(data[times.offset+key*4:])
		keys.values[key] = float32(binary.LittleEndian.Uint16(source[values.offset+key*2:])) / 32767
	}
	return keys
}

func readM2ScalarKeysFloat(data []byte, timesOuter, valuesOuter m2Array, index int, source []byte) m2ScalarKeys {
	times, timesOK := readM2TrackArray(data, timesOuter.offset+index*8)
	values, valuesOK := readM2TrackArray(data, valuesOuter.offset+index*8)
	if !timesOK || !valuesOK {
		return m2ScalarKeys{}
	}
	count := times.count
	if values.count < count {
		count = values.count
	}
	if count <= 0 || times.offset < 0 || values.offset < 0 || times.offset > len(data) || values.offset > len(source) || count > (len(data)-times.offset)/4 || count > (len(source)-values.offset)/4 {
		return m2ScalarKeys{}
	}
	keys := m2ScalarKeys{times: make([]uint32, count), values: make([]float32, count)}
	for key := 0; key < count; key++ {
		keys.times[key] = binary.LittleEndian.Uint32(data[times.offset+key*4:])
		keys.values[key] = readF32(source, values.offset+key*4)
	}
	return keys
}

func readM2TrackQuat(data []byte, offset, sequenceCount int, external map[int][]byte, inline []bool) m2TrackQuat {
	interpolation, globalSequence, timesOuter, valuesOuter, ok := readM2TrackHeader(data, offset)
	if !ok || sequenceCount < 0 {
		return m2TrackQuat{}
	}
	track := m2TrackQuat{interpolation: interpolation, globalSequence: globalSequence, sequences: make([]m2QuatKeys, sequenceCount)}
	count := sequenceCount
	if timesOuter.count < count {
		count = timesOuter.count
	}
	if valuesOuter.count < count {
		count = valuesOuter.count
	}
	for index := 0; index < count; index++ {
		if !inline[index] {
			if source, exists := external[index]; exists {
				track.sequences[index] = readM2QuatKeys(data, timesOuter, valuesOuter, index, source)
			}
			continue
		}
		track.sequences[index] = readM2QuatKeys(data, timesOuter, valuesOuter, index, data)
	}
	return track
}

func readM2Events(data []byte, array m2Array, sequenceCount int, external map[int][]byte, inline []bool) []m2Event {
	if array.count <= 0 || sequenceCount < 0 {
		return nil
	}
	events := make([]m2Event, array.count)
	for index := range events {
		base := array.offset + index*36
		events[index] = m2Event{identifier: [4]byte{data[base], data[base+1], data[base+2], data[base+3]}, data: binary.LittleEndian.Uint32(data[base+4 : base+8]), bone: binary.LittleEndian.Uint32(data[base+8 : base+12]), position: [3]float32{readF32(data, base+12), readF32(data, base+16), readF32(data, base+20)}, times: make([][]uint32, sequenceCount)}
		outer, ok := readM2TrackArray(data, base+28)
		if !ok {
			continue
		}
		count := sequenceCount
		if outer.count < count {
			count = outer.count
		}
		for sequence := 0; sequence < count; sequence++ {
			if !inline[sequence] {
				if _, exists := external[sequence]; !exists {
					continue
				}
			}
			inner, ok := readM2TrackArray(data, outer.offset+sequence*8)
			if !ok || inner.count <= 0 || inner.offset < 0 {
				continue
			}
			source := data
			if externalData, exists := external[sequence]; exists {
				source = externalData
			}
			if inner.count > (len(source)-inner.offset)/4 {
				continue
			}
			events[index].times[sequence] = make([]uint32, inner.count)
			for key := range events[index].times[sequence] {
				events[index].times[sequence][key] = binary.LittleEndian.Uint32(source[inner.offset+key*4:])
			}
		}
	}
	return events
}

func readM2QuatKeys(data []byte, timesOuter, valuesOuter m2Array, index int, source []byte) m2QuatKeys {
	times, timesOK := readM2TrackArray(data, timesOuter.offset+index*8)
	values, valuesOK := readM2TrackArray(data, valuesOuter.offset+index*8)
	if !timesOK || !valuesOK {
		return m2QuatKeys{}
	}
	count := times.count
	if values.count < count {
		count = values.count
	}
	if count <= 0 || times.offset < 0 || values.offset < 0 || times.offset > len(source) || values.offset > len(source) || count > (len(source)-times.offset)/4 || count > (len(source)-values.offset)/8 {
		return m2QuatKeys{}
	}
	keys := m2QuatKeys{times: make([]uint32, count), values: make([][4]float32, count)}
	for key := 0; key < count; key++ {
		keys.times[key] = binary.LittleEndian.Uint32(source[times.offset+key*4:])
		base := values.offset + key*8
		quaternion := [4]float32{decodeM2Quaternion(binary.LittleEndian.Uint16(source[base:])), decodeM2Quaternion(binary.LittleEndian.Uint16(source[base+2:])), decodeM2Quaternion(binary.LittleEndian.Uint16(source[base+4:])), decodeM2Quaternion(binary.LittleEndian.Uint16(source[base+6:]))}
		keys.values[key] = normalizeM2Quaternion(quaternion)
	}
	return keys
}

func (track m2TrackVec3) value(sequence int, time uint32, globalLoops []uint32, fallback [3]float32) [3]float32 {
	if track.globalSequence != 0xffff {
		if index := int(track.globalSequence); index < len(globalLoops) {
			duration := globalLoops[index]
			if duration > 0 {
				time %= duration
			}
		}
		sequence = 0
	}
	if sequence < 0 || sequence >= len(track.sequences) {
		return fallback
	}
	keys := track.sequences[sequence]
	count := len(keys.times)
	if len(keys.values) < count {
		count = len(keys.values)
	}
	if count == 0 {
		return fallback
	}
	if count == 1 {
		return keys.values[0]
	}
	next := 0
	for next < count && keys.times[next] <= time {
		next++
	}
	if next == 0 {
		return keys.values[0]
	}
	if next >= count {
		return keys.values[count-1]
	}
	left := next - 1
	start, end := keys.times[left], keys.times[next]
	if track.interpolation == 0 || end <= start {
		return keys.values[left]
	}
	fraction := float32(time-start) / float32(end-start)
	return lerpM2Vector(keys.values[left], keys.values[next], fraction)
}

func (track m2TrackScalar) value(sequence int, time uint32, globalLoops []uint32, fallback float32) float32 {
	if track.globalSequence != 0xffff {
		if index := int(track.globalSequence); index < len(globalLoops) {
			duration := globalLoops[index]
			if duration > 0 {
				time %= duration
			}
		}
		sequence = 0
	}
	if sequence < 0 || sequence >= len(track.sequences) {
		return fallback
	}
	keys := track.sequences[sequence]
	count := len(keys.times)
	if len(keys.values) < count {
		count = len(keys.values)
	}
	if count == 0 {
		return fallback
	}
	if count == 1 {
		return keys.values[0]
	}
	next := 0
	for next < count && keys.times[next] <= time {
		next++
	}
	if next == 0 {
		return keys.values[0]
	}
	if next >= count {
		return keys.values[count-1]
	}
	left := next - 1
	start, end := keys.times[left], keys.times[next]
	if track.interpolation == 0 || end <= start {
		return keys.values[left]
	}
	fraction := float32(time-start) / float32(end-start)
	return keys.values[left] + (keys.values[next]-keys.values[left])*fraction
}

func (track m2TrackQuat) value(sequence int, time uint32, globalLoops []uint32, fallback [4]float32) [4]float32 {
	if track.globalSequence != 0xffff {
		if index := int(track.globalSequence); index < len(globalLoops) {
			duration := globalLoops[index]
			if duration > 0 {
				time %= duration
			}
		}
		sequence = 0
	}
	if sequence < 0 || sequence >= len(track.sequences) {
		return fallback
	}
	keys := track.sequences[sequence]
	count := len(keys.times)
	if len(keys.values) < count {
		count = len(keys.values)
	}
	if count == 0 {
		return fallback
	}
	if count == 1 {
		return keys.values[0]
	}
	next := 0
	for next < count && keys.times[next] <= time {
		next++
	}
	if next == 0 {
		return keys.values[0]
	}
	if next >= count {
		return keys.values[count-1]
	}
	left := next - 1
	start, end := keys.times[left], keys.times[next]
	if track.interpolation == 0 || end <= start {
		return keys.values[left]
	}
	fraction := float32(time-start) / float32(end-start)
	return slerpM2Quaternion(keys.values[left], keys.values[next], fraction)
}

func lerpM2Vector(left, right [3]float32, fraction float32) [3]float32 {
	return [3]float32{left[0] + (right[0]-left[0])*fraction, left[1] + (right[1]-left[1])*fraction, left[2] + (right[2]-left[2])*fraction}
}

func normalizeM2Quaternion(value [4]float32) [4]float32 {
	length := float32(math.Sqrt(float64(value[0]*value[0] + value[1]*value[1] + value[2]*value[2] + value[3]*value[3])))
	if length < 0.0001 {
		return [4]float32{0, 0, 0, 1}
	}
	return [4]float32{value[0] / length, value[1] / length, value[2] / length, value[3] / length}
}

func slerpM2Quaternion(left, right [4]float32, fraction float32) [4]float32 {
	dot := left[0]*right[0] + left[1]*right[1] + left[2]*right[2] + left[3]*right[3]
	if dot < 0 {
		right = [4]float32{-right[0], -right[1], -right[2], -right[3]}
		dot = -dot
	}
	if dot > 0.9995 {
		return normalizeM2Quaternion([4]float32{left[0] + (right[0]-left[0])*fraction, left[1] + (right[1]-left[1])*fraction, left[2] + (right[2]-left[2])*fraction, left[3] + (right[3]-left[3])*fraction})
	}
	angle := float32(math.Acos(math.Max(-1, math.Min(1, float64(dot)))))
	sine := float32(math.Sin(float64(angle)))
	if math.Abs(float64(sine)) < 0.0001 {
		return left
	}
	first := float32(math.Sin(float64((1-fraction)*angle))) / sine
	second := float32(math.Sin(float64(fraction*angle))) / sine
	return normalizeM2Quaternion([4]float32{left[0]*first + right[0]*second, left[1]*first + right[1]*second, left[2]*first + right[2]*second, left[3]*first + right[3]*second})
}

func loadM2AnimationTracks(loader *ui.Loader, modelPath string, model *parsedM2) {
	if loader == nil || model == nil || len(model.sequences) == 0 {
		return
	}
	external := make(map[int][]byte)
	inline := make([]bool, len(model.sequences))
	for index, sequence := range model.sequences {
		inline[index] = sequence.flags&0x20 != 0
		if inline[index] {
			continue
		}
		if data, err := loader.ReadFile(m2ExternalAnimationPath(modelPath, sequence)); err == nil {
			external[index] = data
		}
	}
	for index := range model.bones {
		base := model.boneOffset + index*88
		model.bones[index].translationTrack = readM2TrackVec3(model.data, base+16, len(model.sequences), external, inline)
		model.bones[index].rotationTrack = readM2TrackQuat(model.data, base+36, len(model.sequences), external, inline)
		model.bones[index].scaleTrack = readM2TrackVec3(model.data, base+56, len(model.sequences), external, inline)
	}
	for index := range model.textureTransforms {
		base := model.textureTransformOffset + index*m2TextureTransformSize
		model.textureTransforms[index].translation = readM2TrackVec3(model.data, base, len(model.sequences), external, inline)
		model.textureTransforms[index].rotation = readM2TrackQuat(model.data, base+20, len(model.sequences), external, inline)
		model.textureTransforms[index].scale = readM2TrackVec3(model.data, base+40, len(model.sequences), external, inline)
	}
	for index := range model.colors {
		base := model.colorOffset + index*m2ColorSize
		model.colors[index].colorTrack = readM2TrackVec3(model.data, base, len(model.sequences), external, inline)
		model.colors[index].alphaTrack = readM2TrackScalar(model.data, base+20, len(model.sequences), external, inline)
	}
	for index := range model.textureWeights {
		base := model.textureWeightOffset + index*m2TextureWeightSize
		model.textureWeights[index].weightTrack = readM2TrackScalar(model.data, base, len(model.sequences), external, inline)
	}
	particles, _ := readM2Array(model.data, 0x128, m2ParticleSize)
	for index := range model.particles {
		base := particles.offset + index*m2ParticleSize
		model.particles[index].rateTrack = readM2TrackFloatTrack(model.data, base+0xb0, len(model.sequences), external, inline)
	}
	events, _ := readM2Array(model.data, 0x100, 36)
	model.events = readM2Events(model.data, events, len(model.sequences), external, inline)
	resolveM2TrackAliases(model)
	sequence := defaultM2Sequence(model)
	for index := range model.bones {
		model.bones[index].translation = model.bones[index].translationTrack.value(sequence, 0, model.globalLoops, [3]float32{})
		model.bones[index].rotation = model.bones[index].rotationTrack.value(sequence, 0, model.globalLoops, [4]float32{0, 0, 0, 1})
		model.bones[index].scale = model.bones[index].scaleTrack.value(sequence, 0, model.globalLoops, [3]float32{1, 1, 1})
	}
	updateM2AnimatedValues(model, sequence, 0, 0)
}

func updateM2AnimatedValues(model *parsedM2, sequence int, timeMS, globalTimeMS uint32) {
	if model == nil {
		return
	}
	model.animationSequence = sequence
	model.animationTime = timeMS
	model.animationGlobalTime = globalTimeMS
	for index := range model.colors {
		color := &model.colors[index]
		color.current = color.colorTrack.value(sequence, m2TrackTime(color.colorTrack.globalSequence, timeMS, globalTimeMS), model.globalLoops, [3]float32{1, 1, 1})
		color.currentAlpha = color.alphaTrack.value(sequence, m2TrackTime(color.alphaTrack.globalSequence, timeMS, globalTimeMS), model.globalLoops, 1)
		for component := range color.current {
			color.current[component] = clampM2Color(color.current[component])
		}
		color.currentAlpha = clampM2Color(color.currentAlpha)
	}
	for index := range model.textureWeights {
		weight := &model.textureWeights[index]
		weight.current = clampM2Color(weight.weightTrack.value(sequence, m2TrackTime(weight.weightTrack.globalSequence, timeMS, globalTimeMS), model.globalLoops, 1))
	}
}

func clampM2Color(value float32) float32 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func m2ExternalAnimationPath(modelPath string, sequence m2Sequence) string {
	stem := modelPath
	if strings.HasSuffix(strings.ToLower(stem), ".m2") {
		stem = stem[:len(stem)-3]
	}
	return fmt.Sprintf("%s%04d-%02d.anim", stem, sequence.id, sequence.variation)
}

func resolveM2TrackAliases(model *parsedM2) {
	if model == nil {
		return
	}
	target := func(start int) int {
		index := start
		for step := 0; step < len(model.sequences); step++ {
			if index < 0 || index >= len(model.sequences) {
				return -1
			}
			sequence := model.sequences[index]
			if sequence.flags&0x40 == 0 {
				return index
			}
			next := int(sequence.aliasNext)
			if next == index {
				return -1
			}
			index = next
		}
		return -1
	}
	for boneIndex := range model.bones {
		for index, sequence := range model.sequences {
			if sequence.flags&0x40 == 0 {
				continue
			}
			to := target(index)
			if to < 0 || to == index {
				continue
			}
			if len(model.bones[boneIndex].translationTrack.sequences[index].values) == 0 {
				model.bones[boneIndex].translationTrack.sequences[index] = model.bones[boneIndex].translationTrack.sequences[to]
			}
			if len(model.bones[boneIndex].rotationTrack.sequences[index].values) == 0 {
				model.bones[boneIndex].rotationTrack.sequences[index] = model.bones[boneIndex].rotationTrack.sequences[to]
			}
			if len(model.bones[boneIndex].scaleTrack.sequences[index].values) == 0 {
				model.bones[boneIndex].scaleTrack.sequences[index] = model.bones[boneIndex].scaleTrack.sequences[to]
			}
		}
	}
	for transformIndex := range model.textureTransforms {
		for index, sequence := range model.sequences {
			if sequence.flags&0x40 == 0 {
				continue
			}
			to := target(index)
			if to < 0 || to == index {
				continue
			}
			transform := &model.textureTransforms[transformIndex]
			if len(transform.translation.sequences[index].values) == 0 {
				transform.translation.sequences[index] = transform.translation.sequences[to]
			}
			if len(transform.rotation.sequences[index].values) == 0 {
				transform.rotation.sequences[index] = transform.rotation.sequences[to]
			}
			if len(transform.scale.sequences[index].values) == 0 {
				transform.scale.sequences[index] = transform.scale.sequences[to]
			}
		}
	}
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
	return poseM2VertexWithBones(&model, skin, local, vertex, boneComboIndex, model.bones)
}

func poseM2VertexWithBones(_ *parsedM2, _ parsedSkin, _ int, vertex m2Vertex, _ int, bonesModel []m2Bone) posedM2Vertex {
	weights := vertex.weights
	var position, normal [3]float32
	weightTotal := 0
	for slot, weight := range weights {
		if weight == 0 {
			continue
		}
		boneIndex := int(vertex.bones[slot])
		if boneIndex >= len(bonesModel) {
			continue
		}
		weightTotal += int(weight)
		bonePosition := transformM2Point(boneIndex, vertex.position, bonesModel, 0)
		boneNormal := transformM2Normal(boneIndex, vertex.normal, bonesModel, 0)
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
	if bone.flags&(0x80|0x200) != 0 {
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
	}
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
	if bone.flags&(0x80|0x200) != 0 {
		normal = rotateM2Vector(bone.rotation, normal)
	}
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
	path = strings.TrimPrefix(path, "\\")
	if strings.HasSuffix(strings.ToLower(path), ".mdx") {
		path = path[:len(path)-4] + ".m2"
	}
	return path
}
