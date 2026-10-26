package render

import (
	"fmt"
	"math"
	"sort"
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

// npcExtraItemSlots maps CreatureDisplayInfoExtra item columns to inventory types.
var npcExtraItemSlots = [11]uint8{1, 3, 4, 5, 6, 7, 8, 9, 10, 19, 16}

type worldCreatureExtra struct {
	id         uint32
	race       uint8
	gender     uint8
	skin       uint8
	face       uint8
	hairStyle  uint8
	hairColor  uint8
	facialHair uint8
	items      [11]uint32
	bake       string
	ok         bool
}

func loadWorldCreatureExtra(tables *worldCreatureTables, displayRecord int) worldCreatureExtra {
	var extra worldCreatureExtra
	if tables.extra.fields < 21 || displayRecord < 0 {
		return extra
	}
	extraID := tables.displays.value(displayRecord, 3)
	if extraID == 0 {
		return extra
	}
	record := characterDBCRecordByID(tables.extra, extraID)
	if record < 0 {
		return extra
	}
	extra.ok = true
	extra.id = extraID
	extra.race = uint8(tables.extra.value(record, 1))
	extra.gender = uint8(tables.extra.value(record, 2))
	extra.skin = uint8(tables.extra.value(record, 3))
	extra.face = uint8(tables.extra.value(record, 4))
	extra.hairStyle = uint8(tables.extra.value(record, 5))
	extra.hairColor = uint8(tables.extra.value(record, 6))
	extra.facialHair = uint8(tables.extra.value(record, 7))
	for index := 0; index < 11; index++ {
		extra.items[index] = tables.extra.value(record, 8+index)
	}
	extra.bake = tables.extra.string(record, 20)
	return extra
}

func worldCreatureDisplayScales(tables *worldCreatureTables, displayRecord, modelRecord int) (displayScale, modelScale float32) {
	displayScale = tables.displays.valueFloat(displayRecord, 4)
	modelScale = tables.models.valueFloat(modelRecord, 4)
	if displayScale <= 0 {
		displayScale = 1
	}
	if modelScale <= 0 {
		modelScale = 1
	}
	return displayScale, modelScale
}

// worldEntityScale combines OBJECT_FIELD_SCALE_X with CreatureModelData.modelScale.
// When the scale field is absent, CreatureDisplayInfo.creatureModelScale is used.
func worldEntityScale(entity *worldEntity, displayScale, modelScale float32) float32 {
	if displayScale <= 0 {
		displayScale = 1
	}
	if modelScale <= 0 {
		modelScale = 1
	}
	objectScale := displayScale
	if bits, ok := entity.fields[world.ObjectScaleField]; ok {
		if value := math.Float32frombits(bits); value > 0 {
			objectScale = value
		}
	}
	return objectScale * modelScale
}

func resolveWorldCreatureExtraGeosets(loader *ui.Loader, extra worldCreatureExtra, skin parsedSkin) (map[uint16]bool, string) {
	active, sections := resolveWorldCreatureExtraAppearance(loader, extra, skin)
	return active, sections.cape
}

func resolveWorldCreatureExtraAppearance(loader *ui.Loader, extra worldCreatureExtra, skin parsedSkin) (map[uint16]bool, characterSectionTextures) {
	character := world.Character{
		Race:       extra.race,
		Gender:     extra.gender,
		Skin:       extra.skin,
		Face:       extra.face,
		HairStyle:  extra.hairStyle,
		HairColor:  extra.hairColor,
		FacialHair: extra.facialHair,
	}
	finger, trinket := 10, 12
	for index, displayID := range extra.items {
		if displayID == 0 {
			continue
		}
		inv := npcExtraItemSlots[index]
		slot := equipmentSlotForInventoryType(inv, &finger, &trinket)
		if slot < 0 || slot >= world.EquipmentSlots {
			continue
		}
		character.Equipment[slot] = world.Equipment{DisplayID: displayID, InventoryType: inv}
	}
	sections, _ := resolveCharacterSections(loader, character)
	active, err := resolveCharacterEquipment(loader, character, &sections, skin)
	if err != nil {
		active = map[uint16]bool{0: true}
	}
	return active, sections
}

// buildWorldUnitModel builds an M2 for world placement at native vertex scale.
// Unlike buildGlueModel, it does not run the UI modelTransform fit (3/maxDimension).
func buildWorldUnitModel(loader *ui.Loader, modelPath string, model parsedM2, skin parsedSkin, textureOverrides map[int]string, activeGeosets map[uint16]bool, preloaded map[string]*texture.Texture2D) (*core.Node, error) {
	parts := buildM2PartsWithFilters(model, skin, textureOverrides, activeGeosets)
	if len(parts) == 0 {
		return nil, fmt.Errorf("%s: no renderable skin batches", modelPath)
	}
	root := core.NewNode()
	textures := make(map[string]*texture.Texture2D)
	texturePaths := make(map[string]struct{})
	stats := glueModelStats{}
	animatedMeshes := make([]*m2AnimatedMesh, 0, len(parts))
	// Parts that are drawn the same way (one texture, one blend) share a mesh:
	// a unit was a draw for every piece of it, and most pieces share a texture.
	for _, members := range groupWorldParts(parts) {
		part := members[0]
		stats.parts += len(members)
		for _, member := range members {
			stats.vertices += len(member.positions) / 3
			stats.triangles += len(member.indices) / 3
		}
		if part.material.blend >= 2 {
			stats.transparentBatches++
		} else {
			stats.opaqueBatches++
		}
		for _, texturePath := range part.texturePaths {
			texturePaths[texturePath] = struct{}{}
		}
		merged := mergeWorldParts(members)
		geom := geometry.NewGeometry()
		geom.SetIndices(merged.indices)
		positionVBO := gls.NewVBO(merged.positions).AddAttrib(gls.VertexPosition)
		normalVBO := gls.NewVBO(merged.normals).AddAttrib(gls.VertexNormal)
		geom.AddVBO(positionVBO)
		geom.AddVBO(normalVBO)
		uvVBO := gls.NewVBO(merged.uvs).AddAttrib(gls.VertexTexcoord)
		geom.AddVBO(uvVBO)
		var uv2VBO *gls.VBO
		if len(part.uvSets) > 1 {
			uv2VBO = gls.NewVBO(merged.uvs2).AddCustomAttrib("VertexTexcoord2", 2)
			geom.AddVBO(uv2VBO)
		}
		colorVBO := gls.NewVBO(merged.colors).AddAttrib(gls.VertexColor)
		alphaVBO := gls.NewVBO(merged.alphas).AddCustomAttrib("VertexM2Alpha", 1)
		combinerVBO := gls.NewVBO(merged.combiners).AddCustomAttrib("VertexM2Combiner", 2)
		geom.AddVBO(colorVBO)
		geom.AddVBO(alphaVBO)
		geom.AddVBO(combinerVBO)
		mat := newWorldMaterial()
		mat.lit = true
		mat.envMask = float32(part.envMask)
		mat.noFog = !worldFoggedBlending(m2MaterialBlending(part.material.blend))
		if part.material.blend == 1 {
			mat.SetShader("morenowow_world_m2_alpha_key")
		} else {
			mat.SetShader("morenowow_world_m2")
		}
		mat.SetShaderUnique(false)
		if part.material.flags&0x04 != 0 {
			mat.SetSide(material.SideDouble)
		} else {
			mat.SetSide(material.SideFront)
		}
		mat.SetUseLights(material.UseLightNone)
		mat.SetDepthTest(part.material.flags&0x08 == 0)
		mat.SetDepthMask(part.material.flags&0x10 == 0)
		if m2MaterialBlending(part.material.blend) == material.BlendNone {
			mat.SetTransparent(false)
			mat.SetBlending(material.BlendNone)
		} else {
			mat.SetTransparent(true)
			mat.SetBlending(m2MaterialBlending(part.material.blend))
		}
		for textureIndex, texturePath := range part.texturePaths {
			tex := preloaded[texturePath]
			shared := false
			if tex == nil {
				tex = textures[texturePath]
			}
			if tex == nil {
				tex = loadSharedModelTexture(loader, texturePath)
				if tex != nil {
					textures[texturePath] = tex
				}
			}
			if tex != nil && preloaded[texturePath] == nil {
				shared = true
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
				if shared {
					// The cache keeps one reference; each material holds its own.
					tex.Incref()
				}
			}
		}
		mesh := graphic.NewMesh(geom, mat)
		mesh.SetRenderOrder(m2RenderOrder(part))
		mesh.SetStateSort(true)
		root.Add(mesh)
		var group *m2MeshGroup
		if len(members) > 1 {
			group = &m2MeshGroup{positionVBO: positionVBO, normalVBO: normalVBO, uvVBO: uvVBO, uv2VBO: uv2VBO, colorVBO: colorVBO, alphaVBO: alphaVBO, positions: merged.positions, normals: merged.normals, uvs: merged.uvs, uvs2: merged.uvs2, colors: merged.colors, alphas: merged.alphas}
		}
		for _, member := range members {
			animated := &m2AnimatedMesh{part: member, group: group, baseUVs: append(math32.ArrayF32(nil), member.uvs...), baseUVs2: append(math32.ArrayF32(nil), member.uvs2...), textureTransformIndices: append([]int(nil), member.textureTransformIndices...)}
			if group == nil {
				animated.positionVBO, animated.normalVBO, animated.uvVBO, animated.uv2VBO, animated.colorVBO, animated.alphaVBO = positionVBO, normalVBO, uvVBO, uv2VBO, colorVBO, alphaVBO
			}
			animatedMeshes = append(animatedMeshes, animated)
		}
	}
	if len(root.Children()) == 0 {
		return nil, fmt.Errorf("%s: model textures or geometry unavailable", modelPath)
	}
	scale := float32(1)
	modelBottom := float32(math.MaxFloat32)
	modelTop := float32(-math.MaxFloat32)
	for _, part := range parts {
		for index := 1; index < len(part.positions); index += 3 {
			if top := part.positions[index]; top > modelTop {
				modelTop = top
			}
			if bottom := part.positions[index]; bottom < modelBottom {
				modelBottom = bottom
			}
		}
	}
	stats.textures = len(texturePaths)
	particles := buildM2ParticleSystem(loader, &model, root, scale, textures, 1)
	if particles != nil {
		stats.particleEmitters = particles.emitterCount
		stats.particlePoints = particles.pointCount
	}
	animation := buildM2Animation(&model, skin, animatedMeshes, modelPath)
	info := glueModelInfo{stats: stats, particles: particles, animation: animation, modelScale: scale, modelBottom: modelBottom, modelTop: modelTop, attachments: model.attachments}
	if len(model.vertices) > 0 {
		low, high := float32(math.MaxFloat32), float32(-math.MaxFloat32)
		for _, vertex := range model.vertices {
			low, high = float32(math.Min(float64(low), float64(vertex.position[2]))), float32(math.Max(float64(high), float64(vertex.position[2])))
		}
		info.rawHeight = high - low
		info.shadowHalf = modelShadowHalfExtents(model.vertices)
	}
	if len(model.cameras) > 0 {
		info.portrait, info.hasPortrait = model.cameras[0], true
	}
	root.SetUserData(info)
	return root, nil
}

func buildWorldCreatureModelFromParts(loader *ui.Loader, path string, variations [3]string, bake string, extra worldCreatureExtra) (*core.Node, error) {
	model, err := loadSharedM2(loader, path)
	if err != nil {
		return nil, fmt.Errorf("read creature model %s: %w", path, err)
	}
	skinData, err := loader.ReadFile(worldM2SkinPath(path))
	if err != nil {
		return nil, fmt.Errorf("read creature skin %s: %w", worldM2SkinPath(path), err)
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		return nil, err
	}
	overrides := make(map[int]string)
	preloaded := make(map[string]*texture.Texture2D)
	bakePath := ""
	if bake != "" {
		bakePath = `Textures\BakedNpcTextures\` + bake
		if !strings.Contains(strings.ToLower(bakePath), ".blp") {
			bakePath += ".blp"
		}
	}
	capePath := ""
	var activeGeosets map[uint16]bool
	sections := characterSectionTextures{}
	if extra.ok {
		activeGeosets, sections = resolveWorldCreatureExtraAppearance(loader, extra, skin)
		capePath = sections.cape
	}
	bodyPath := sections.bodySkin
	if extra.ok && bakePath == "" && bodyPath != "" && (sections.faceLower != "" || sections.faceUpper != "" || sections.facialHairLower != "" || sections.facialHairUpper != "" || sections.scalpLower != "" || sections.scalpUpper != "" || len(sections.underwear) > 0 || len(sections.regions) > 0) {
		composite, compositeErr := composeCharacterSkin(loader, sections)
		if compositeErr == nil {
			bodyPath = fmt.Sprintf("__world_character_skin_%d", extra.id)
			preloaded[bodyPath] = composite
		}
	}
	for index, textureType := range model.textureTypes {
		texPath := ""
		switch textureType {
		case 1:
			if bakePath != "" {
				texPath = bakePath
			} else {
				texPath = bodyPath
			}
		case 2:
			texPath = capePath
		case 6:
			texPath = sections.hair
			if texPath == "" && extra.ok {
				texPath = fmt.Sprintf(`Character\%s\Hair00_00.blp`, characterRaceFolder(extra.race))
			}
		case 8:
			texPath = sections.skinExtra
			if texPath == "" && len(sections.underwear) > 0 {
				texPath = sections.underwear[0].path
			}
			if texPath == "" {
				texPath = bodyPath
			}
		}
		if texPath == "" {
			slot := -1
			switch textureType {
			case 1, 2, 11:
				slot = 0
			case 12:
				slot = 1
			case 13:
				slot = 2
			}
			if slot >= 0 && texPath == "" {
				texPath = worldCreatureVariationPath(path, variations[slot])
			}
		}
		if texPath != "" {
			overrides[index] = texPath
		}
	}
	return buildWorldUnitModel(loader, path, model, skin, overrides, activeGeosets, preloaded)
}

// groupWorldParts sorts the parts that can share a mesh together, in the order
// each kind first appears. Parts with nothing to draw are left out.
func groupWorldParts(parts map[string]*m2Part) [][]*m2Part {
	type key struct {
		order       int
		blend       uint16
		flags       uint16
		envMask     uint32
		textures    string
		textureFlag string
		secondUV    bool
	}
	index := make(map[key]int)
	var groups [][]*m2Part
	keys := make([]string, 0, len(parts))
	for name := range parts {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		part := parts[name]
		if len(part.texturePaths) == 0 || len(part.indices) == 0 {
			continue
		}
		// Opaque pieces are drawn by depth, so the order a model lists them in does not
		// matter; blended ones are layered in that order and keep it.
		order := m2RenderOrder(part)
		if part.material.blend < 2 {
			order = 0
		}
		k := key{order: order, blend: uint16(part.material.blend), flags: uint16(part.material.flags), envMask: part.envMask, textures: strings.Join(part.texturePaths, "\x00"), textureFlag: fmt.Sprint(part.textureFlags), secondUV: len(part.uvSets) > 1}
		if at, ok := index[k]; ok {
			groups[at] = append(groups[at], part)
			continue
		}
		index[k] = len(groups)
		groups = append(groups, []*m2Part{part})
	}
	return groups
}

// mergedWorldParts are the arrays of a mesh made of several parts.
type mergedWorldParts struct {
	positions, normals, uvs, uvs2, colors, alphas, combiners math32.ArrayF32
	indices                                                  math32.ArrayU32
}

// mergeWorldParts joins the arrays of parts into one set, and makes each
// part's own arrays windows onto it, so that posing a part writes into the
// shared buffers.
func mergeWorldParts(members []*m2Part) mergedWorldParts {
	if len(members) == 1 {
		part := members[0]
		return mergedWorldParts{part.positions, part.normals, part.uvs, part.uvs2, part.colors, part.alphas, part.combiners, part.indices}
	}
	var total, totalIndices int
	for _, part := range members {
		total += len(part.positions) / 3
		totalIndices += len(part.indices)
	}
	merged := mergedWorldParts{
		positions: make(math32.ArrayF32, 0, total*3), normals: make(math32.ArrayF32, 0, total*3),
		uvs: make(math32.ArrayF32, 0, total*2), uvs2: make(math32.ArrayF32, 0, total*2),
		colors: make(math32.ArrayF32, 0, total*3), alphas: make(math32.ArrayF32, 0, total),
		combiners: make(math32.ArrayF32, 0, total*2), indices: make(math32.ArrayU32, 0, totalIndices),
	}
	for _, part := range members {
		base := uint32(len(merged.positions) / 3)
		merged.positions = append(merged.positions, part.positions...)
		merged.normals = append(merged.normals, part.normals...)
		merged.uvs = append(merged.uvs, part.uvs...)
		merged.uvs2 = append(merged.uvs2, part.uvs2...)
		merged.colors = append(merged.colors, part.colors...)
		merged.alphas = append(merged.alphas, part.alphas...)
		merged.combiners = append(merged.combiners, part.combiners...)
		for _, index := range part.indices {
			merged.indices = append(merged.indices, base+index)
		}
	}
	// Each part now works in its own window of the shared arrays.
	at := 0
	for _, part := range members {
		count := len(part.positions) / 3
		window := func(source, shared math32.ArrayF32, width int) math32.ArrayF32 {
			if len(source) == 0 {
				return source
			}
			return shared[at*width : (at+count)*width : (at+count)*width]
		}
		part.positions = window(part.positions, merged.positions, 3)
		part.normals = window(part.normals, merged.normals, 3)
		part.uvs = window(part.uvs, merged.uvs, 2)
		part.uvs2 = window(part.uvs2, merged.uvs2, 2)
		part.colors = window(part.colors, merged.colors, 3)
		part.alphas = window(part.alphas, merged.alphas, 1)
		part.combiners = window(part.combiners, merged.combiners, 2)
		at += count
	}
	return merged
}
