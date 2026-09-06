package render

import (
	"fmt"
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
	sections := characterSectionTextures{}
	active, err := resolveCharacterEquipment(loader, character, &sections, skin)
	if err != nil {
		active = map[uint16]bool{0: true}
	}
	return active, sections.cape
}

// buildWorldUnitModel builds an M2 for world placement at native vertex scale.
// Unlike buildGlueModel, it does not run the UI modelTransform fit (3/maxDimension).
func buildWorldUnitModel(loader *ui.Loader, modelPath string, model parsedM2, skin parsedSkin, textureOverrides map[int]string, activeGeosets map[uint16]bool) (*core.Node, error) {
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
		animatedMeshes = append(animatedMeshes, &m2AnimatedMesh{part: part, positionVBO: positionVBO, normalVBO: normalVBO, uvVBO: uvVBO, uv2VBO: uv2VBO, baseUVs: append(math32.ArrayF32(nil), part.uvs...), baseUVs2: append(math32.ArrayF32(nil), part.uvs2...), textureTransformIndices: append([]int(nil), part.textureTransformIndices...)})
	}
	if len(root.Children()) == 0 {
		return nil, fmt.Errorf("%s: model textures or geometry unavailable", modelPath)
	}
	scale := float32(1)
	modelBottom := float32(math.MaxFloat32)
	for _, part := range parts {
		for index := 1; index < len(part.positions); index += 3 {
			if bottom := part.positions[index]; bottom < modelBottom {
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
	root.SetUserData(info)
	return root, nil
}

func buildWorldCreatureModelFromParts(loader *ui.Loader, path string, variations [3]string, bake string, extra worldCreatureExtra) (*core.Node, error) {
	modelData, err := loader.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read creature model %s: %w", path, err)
	}
	model, err := parseM2(modelData)
	if err != nil {
		return nil, err
	}
	loadM2AnimationTracks(loader, path, &model)
	skinData, err := loader.ReadFile(worldM2SkinPath(path))
	if err != nil {
		return nil, fmt.Errorf("read creature skin %s: %w", worldM2SkinPath(path), err)
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		return nil, err
	}
	overrides := make(map[int]string)
	bakePath := ""
	if bake != "" {
		bakePath = `Textures\BakedNpcTextures\` + bake
		if !strings.Contains(strings.ToLower(bakePath), ".blp") {
			bakePath += ".blp"
		}
	}
	capePath := ""
	var activeGeosets map[uint16]bool
	if extra.ok {
		activeGeosets, capePath = resolveWorldCreatureExtraGeosets(loader, extra, skin)
	}
	for index, textureType := range model.textureTypes {
		texPath := ""
		if textureType == 1 && bakePath != "" {
			texPath = bakePath
		} else if textureType == 2 && capePath != "" {
			texPath = capePath
		} else {
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
	return buildWorldUnitModel(loader, path, model, skin, overrides, activeGeosets)
}
