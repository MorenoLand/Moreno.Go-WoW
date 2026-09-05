package render

import (
	"encoding/binary"
	"fmt"
	"image"
	"math"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/math32"
)

func TestM2RenderOrderUsesMaterialPriority(t *testing.T) {
	background := &m2Part{renderOrder: 27, priorityPlane: 10, material: m2RenderFlag{blend: 2}}
	dragon := &m2Part{renderOrder: 4, priorityPlane: 11, material: m2RenderFlag{blend: 2}}
	if m2RenderOrder(background) >= m2RenderOrder(dragon) {
		t.Fatalf("background order=%d dragon order=%d", m2RenderOrder(background), m2RenderOrder(dragon))
	}
	layered := &m2Part{renderOrder: 3, priorityPlane: 11, materialLayer: 1, material: m2RenderFlag{blend: 2}}
	if m2RenderOrder(dragon) >= m2RenderOrder(layered) {
		t.Fatalf("base order=%d layered order=%d", m2RenderOrder(dragon), m2RenderOrder(layered))
	}
	opaque := &m2Part{renderOrder: 4, priorityPlane: 11, material: m2RenderFlag{blend: 0}}
	if m2RenderOrder(opaque) != -96 {
		t.Fatalf("opaque order=%d", m2RenderOrder(opaque))
	}
}

func TestM2TextureTransformUsesGlobalClock(t *testing.T) {
	model := parsedM2{globalLoops: []uint32{1000}, textureTransforms: []m2TextureTransform{{translation: m2TrackVec3{interpolation: 1, globalSequence: 0, sequences: []m2Vec3Keys{{times: []uint32{0, 1000}, values: [][3]float32{{0, 0, 0}, {0.5, 0, 0}}}}}}}}
	part := &m2Part{uvs: math32.ArrayF32{0.25, 0.25}}
	mesh := &m2AnimatedMesh{part: part, baseUVs: append(math32.ArrayF32(nil), part.uvs...), textureTransformIndices: []int{0}}
	animation := &m2Animation{model: &model, sequence: 0, meshes: []*m2AnimatedMesh{mesh}}
	animation.updateTextureCoordinatesAt(0, 0)
	start := part.uvs[0]
	animation.updateTextureCoordinatesAt(0, 500)
	if math.Abs(float64(part.uvs[0]-start)) < 0.1 {
		t.Fatalf("global texture transform did not advance: start=%f current=%f", start, part.uvs[0])
	}
}

func TestPoseM2VertexUsesSkinBonePalette(t *testing.T) {
	model := parsedM2{bones: []m2Bone{{parent: -1, rotation: [4]float32{0, 0, 0, 1}, scale: [3]float32{1, 1, 1}}, {flags: 0x200, parent: -1, translation: [3]float32{2, 0, 0}, rotation: [4]float32{0, 0, 0, 1}, scale: [3]float32{1, 1, 1}}}, boneCombos: []uint16{0, 1}}
	skin := parsedSkin{bones: [][4]uint8{{1, 0, 0, 0}}}
	vertex := m2Vertex{weights: [4]uint8{255, 0, 0, 0}, bones: [4]uint8{0, 0, 0, 0}}
	posed := poseM2Vertex(model, skin, 0, vertex, 0)
	if posed.position[0] != 2 {
		t.Fatalf("posed x=%f", posed.position[0])
	}
}

func TestNormalizeModelPathResolvesLegacyMDX(t *testing.T) {
	if got := normalizeModelPath(`WORLD/GENERIC/HUMAN/DOODAD.MDX`); got != `WORLD\GENERIC\HUMAN\DOODAD.m2` {
		t.Fatalf("normalized path=%q", got)
	}
}

func TestWorldM2BasisConversionKeepsStandingAxis(t *testing.T) {
	part := &m2Part{positions: []float32{1, 2, 3}, normals: []float32{4, 5, 6}}
	convertWorldM2Parts(map[string]*m2Part{"test": part})
	if got, want := []float32(part.positions), []float32{1, -3, 2}; !equalFloatSlice(got, want) {
		t.Fatalf("positions=%v want %v", got, want)
	}
	if got, want := []float32(part.normals), []float32{4, -6, 5}; !equalFloatSlice(got, want) {
		t.Fatalf("normals=%v want %v", got, want)
	}
}

func TestCharacterTextureRegionsKeepUnderwearOnThePelvis(t *testing.T) {
	underwear, ok := characterRegionRectangle(5, image.Rect(0, 0, 512, 512))
	if !ok || underwear != image.Rect(256, 192, 512, 320) {
		t.Fatalf("underwear region=%v ok=%t", underwear, ok)
	}
	hand, ok := characterRegionRectangle(2, image.Rect(0, 0, 512, 512))
	if !ok || hand != image.Rect(0, 256, 256, 320) {
		t.Fatalf("hand region=%v ok=%t", hand, ok)
	}
	for path, want := range map[string]int{"NakedPelvisSkin00_00.blp": 5, "NakedTorsoSkin00_00.blp": 3} {
		if got, ok := characterUnderwearRegion(path); !ok || got != want {
			t.Fatalf("underwear path=%q region=%d ok=%t want=%d", path, got, ok, want)
		}
	}
	if _, ok := characterUnderwearRegion("Character\\Human\\Male\\Skin00_00.blp"); ok {
		t.Fatal("body skin was accepted as underwear")
	}
}

func TestM2SequenceSelectionPrefersPrimaryVariation(t *testing.T) {
	model := parsedM2{sequences: []m2Sequence{{id: 0, variation: 3, duration: 100}, {id: 0, variation: 0, duration: 100}, {id: 4, variation: 2, duration: 100}, {id: 4, variation: 0, duration: 100}}}
	if got := defaultM2Sequence(&model); got != 1 {
		t.Fatalf("default sequence=%d want primary stand index 1", got)
	}
	animation := &m2Animation{model: &model, sequence: 2, motionID: 4}
	animation.SetMotion(0)
	if animation.sequence != 1 || animation.idleBase != 1 {
		t.Fatalf("motion sequence=%d idleBase=%d want 1", animation.sequence, animation.idleBase)
	}
}

func equalFloatSlice(left, right []float32) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestLiveCharacterModelBounds(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	for _, path := range []string{`Character\Human\Male\HumanMale.m2`, `Character\NightElf\Female\NightElfFemale.m2`, `Interface\Glues\Models\UI_Human\UI_Human.m2`, `Interface\Glues\Models\UI_NightElf\UI_NightElf.m2`} {
		data, readErr := loader.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		model, parseErr := parseM2(data)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		rawMin, rawMax := model.vertices[0].position, model.vertices[0].position
		min, max := modelVector(model.vertices[0].position), modelVector(model.vertices[0].position)
		for _, vertex := range model.vertices[1:] {
			for axis := 0; axis < 3; axis++ {
				rawMin[axis] = float32(math.Min(float64(rawMin[axis]), float64(vertex.position[axis])))
				rawMax[axis] = float32(math.Max(float64(rawMax[axis]), float64(vertex.position[axis])))
			}
			point := modelVector(vertex.position)
			for axis := 0; axis < 3; axis++ {
				min[axis] = float32(math.Min(float64(min[axis]), float64(point[axis])))
				max[axis] = float32(math.Max(float64(max[axis]), float64(point[axis])))
			}
		}
		t.Logf("%s rawBounds=%v..%v bounds min=%v max=%v size=%v", path, rawMin, rawMax, min, max, [3]float32{max[0] - min[0], max[1] - min[1], max[2] - min[2]})
	}
}

func TestLiveCharacterModelRenderable(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	for _, path := range []string{`Character\NightElf\Female\NightElfFemale.m2`, `Character\Human\Male\HumanMale.m2`} {
		model, loadErr := loadGlueModel(loader, path)
		if loadErr != nil {
			t.Fatalf("%s: %v", path, loadErr)
		}
		if model == nil || len(model.Children()) == 0 {
			t.Fatalf("%s: no drawable children", path)
		}
		t.Logf("%s drawable children=%d", path, len(model.Children()))
		model.Dispose()
	}
	backdrop, loadErr := loadGlueModel(loader, `Interface\Glues\Models\UI_NightElf\UI_NightElf.m2`)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	defer backdrop.Dispose()
	info, ok := backdrop.UserData().(glueModelInfo)
	if !ok || !info.hasStand {
		t.Fatal("UI_NightElf backdrop has no authored attachment 0")
	}
	t.Logf("UI_NightElf stand=(%v,%v,%v)", info.standPosition.X, info.standPosition.Y, info.standPosition.Z)
	t.Logf("UI_NightElf camera position=(%v,%v,%v) target=(%v,%v,%v) fov=%v scale=%v", info.position.X, info.position.Y, info.position.Z, info.target.X, info.target.Y, info.target.Z, info.fov, info.modelScale)
	character, loadErr := loadGlueModel(loader, `Character\NightElf\Female\NightElfFemale.m2`)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	defer character.Dispose()
	characterInfo, ok := character.UserData().(glueModelInfo)
	if !ok {
		t.Fatal("character model has no transform info")
	}
	t.Logf("NightElf character model scale=%v", characterInfo.modelScale)
	selected, loadErr := loadGlueCharacterModel(loader, world.Character{GUID: 1, Race: 4, Gender: 1, Skin: 0, Face: 0, HairStyle: 0, HairColor: 0})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	defer selected.Dispose()
	if len(selected.Children()) == 0 {
		t.Fatal("textured character model has no drawable children")
	}
	if math.Abs(float64(selected.Scale().X-characterInfo.modelScale)) > 0.0001 {
		t.Fatalf("textured character scale=%v want own model scale=%v", selected.Scale(), characterInfo.modelScale)
	}
	selectedInfo, ok := selected.UserData().(glueModelInfo)
	if !ok || selectedInfo.animation == nil || selectedInfo.animation.variationEnabled {
		t.Fatal("character preview did not stay on explicit primary idle variation")
	}
	t.Logf("textured NightElf character drawable children=%d", len(selected.Children()))
	skinData, readErr := loader.ReadFile(`Character\NightElf\Female\NightElfFemale00.skin`)
	if readErr != nil {
		t.Fatal(readErr)
	}
	parsedSkin, parseErr := parseSkin(skinData)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	active, equipmentErr := resolveCharacterEquipment(loader, world.Character{Race: 4, Gender: 1, Skin: 0, Face: 0, HairStyle: 0, HairColor: 0}, &characterSectionTextures{bodySkin: "Character\\NightElf\\Female\\NightElfFemaleSkin00_00.blp"}, parsedSkin)
	if equipmentErr != nil {
		t.Fatal(equipmentErr)
	}
	ids := make([]uint16, 0, len(active))
	for id := range active {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	t.Logf("NightElf active geosets=%v", ids)
	available := make([]uint16, 0, len(parsedSkin.submeshes))
	for _, submesh := range parsedSkin.submeshes {
		available = append(available, submesh.submeshID)
	}
	sort.Slice(available, func(left, right int) bool { return available[left] < available[right] })
	t.Logf("NightElf available geosets=%v", available)
}

func TestLiveCharacterSectionsResolve(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	for _, character := range []world.Character{{Race: 4, Gender: 1, Skin: 0, Face: 0, HairStyle: 0, HairColor: 0}, {Race: 11, Gender: 0, Skin: 0, Face: 0, HairStyle: 0, HairColor: 0}} {
		sections, resolveErr := resolveCharacterSections(loader, character)
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		if sections.bodySkin == "" {
			t.Fatalf("race=%d gender=%d skin did not resolve", character.Race, character.Gender)
		}
		if len(sections.underwear) == 0 {
			t.Fatalf("race=%d gender=%d underwear did not resolve", character.Race, character.Gender)
		}
		t.Logf("race=%d gender=%d skin=%s face=(%s,%s) hair=%s underwear=%v", character.Race, character.Gender, sections.bodySkin, sections.faceLower, sections.faceUpper, sections.hair, sections.underwear)
		for _, layer := range sections.underwear {
			region, ok := characterUnderwearRegion(layer.path)
			if !ok || layer.region != region {
				t.Fatalf("underwear layer=%+v resolved region=%d ok=%t", layer, region, ok)
			}
		}
		for _, path := range []string{sections.bodySkin, sections.faceLower, sections.faceUpper, sections.hair} {
			if image, imageErr := loadCharacterImage(loader, path); imageErr == nil {
				t.Logf("character texture %s bounds=%v", path, image.Bounds())
			}
		}
	}
}

func TestLiveMainMenuParticleEmitters(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	data, err := loader.ReadFile(`Interface\Glues\Models\UI_MainMenu_Northrend\UI_MainMenu_Northrend.m2`)
	if err != nil {
		t.Fatal(err)
	}
	model, err := parseM2(data)
	if err != nil {
		t.Fatal(err)
	}
	center, scale := modelTransform(model.vertices)
	t.Logf("main menu model center=%v scale=%v", center, scale)
	if model.camera != nil {
		position := modelPoint(model.camera.position, center, scale)
		target := modelPoint(model.camera.target, center, scale)
		delta := [3]float32{position[0] - target[0], position[1] - target[1], position[2] - target[2]}
		t.Logf("main menu camera position=%v target=%v distance=%v", position, target, particleVectorLength(delta))
	}
	if len(model.particles) == 0 {
		t.Fatal("Northrend main menu has no particle emitters")
	}
	for index, emitter := range model.particles {
		position := modelPoint(transformM2Point(int(emitter.bone), emitter.position, model.bones, 0), center, scale)
		t.Logf("main menu emitter %d position=%v bone=%d texture=%d path=%s blend=%d type=%d rate=%v life=%v speed=%v range=(%v,%v) scale=%v alpha=%v color=%v", index, position, emitter.bone, emitter.texture, model.textures[emitter.texture], emitter.blend, emitter.emitterType, emitter.rate, emitter.life, emitter.speed, emitter.horizontalRange, emitter.verticalRange, emitter.scale, emitter.alpha, emitter.color)
		if strings.HasSuffix(model.textures[emitter.texture], `PARTICLES\SNOW1.BLP`) {
			particles, _ := readM2Array(data, 0x128, m2ParticleSize)
			base := particles.offset + index*m2ParticleSize
			trackValues := func(offset int) []float32 {
				_, _, _, valuesOuter, ok := readM2TrackHeader(data, base+offset)
				if !ok {
					return nil
				}
				values, ok := readM2TrackArray(data, valuesOuter.offset)
				if !ok || values.offset > len(data) || values.count > (len(data)-values.offset)/4 {
					return nil
				}
				result := make([]float32, values.count)
				for key := range result {
					result[key] = readF32(data, values.offset+key*4)
				}
				return result
			}
			t.Logf("snow emitter %d rows=%d cols=%d lifespanVary=%v rateVary=%v speedKeys=%v verticalKeys=%v horizontalKeys=%v tracks color=%d alpha=%d scale=%d head=%d tail=%d extra tail=%v twinkle=(%v,%v) scale=(%v,%v) burst=%v drag=%v spin=(%v,%v,%v,%v) wind=(%v,%v,%v,%v)", index, binary.LittleEndian.Uint16(data[base+48:]), binary.LittleEndian.Uint16(data[base+50:]), readF32(data, base+172), readF32(data, base+196), trackValues(0x34), trackValues(0x5c), trackValues(0x70), binary.LittleEndian.Uint32(data[base+260:]), binary.LittleEndian.Uint32(data[base+276:]), binary.LittleEndian.Uint32(data[base+292:]), binary.LittleEndian.Uint32(data[base+316:]), binary.LittleEndian.Uint32(data[base+332:]), readF32(data, base+348), readF32(data, base+352), readF32(data, base+356), readF32(data, base+360), readF32(data, base+364), readF32(data, base+368), readF32(data, base+372), readF32(data, base+376), readF32(data, base+380), readF32(data, base+384), readF32(data, base+388), readF32(data, base+416), readF32(data, base+420), readF32(data, base+424), readF32(data, base+428))
		}
	}
}

func TestLiveMainMenuSnowMotion(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	data, err := loader.ReadFile(`Interface\Glues\Models\UI_MainMenu_Northrend\UI_MainMenu_Northrend.m2`)
	if err != nil {
		t.Fatal(err)
	}
	model, err := parseM2(data)
	if err != nil {
		t.Fatal(err)
	}
	_, scale := modelTransform(model.vertices)
	found := false
	for index, emitter := range model.particles {
		if int(emitter.texture) >= len(model.textures) || !strings.HasSuffix(model.textures[emitter.texture], `PARTICLES\SNOW1.BLP`) || emitter.rate <= 0 || emitter.life <= 0 {
			continue
		}
		base := modelVector(transformM2Point(int(emitter.bone), emitter.position, model.bones, 0))
		group := &m2ParticleGroup{emitter: emitter, model: &model, bone: int(emitter.bone), base: base, rootScale: scale, positions: math32.NewArrayF32(0, 3), colors: math32.NewArrayF32(0, 3), params: math32.NewArrayF32(0, 4), rotations: math32.NewArrayF32(0, 1)}
		group.particles = []m2Particle{spawnM2Particle(group, uint32(index+1), emitter.life/2)}
		before := particlePosition(group.particles[0])
		group.positions.Append(before[0], before[1], before[2])
		group.colors.Append(1, 1, 1)
		group.params.Append(1, 1, 0, 0)
		group.rotations.Append(0)
		(&m2ParticleSystem{groups: []*m2ParticleGroup{group}}).Update(0.5)
		after := [3]float32{group.positions[0], group.positions[1], group.positions[2]}
		if before == after {
			t.Fatalf("snow emitter %d did not move: %v", index, before)
		}
		found = true
	}
	if !found {
		t.Fatal("main menu has no active snow emitter")
	}
}

func TestLiveMainMenuSnowBatches(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	modelData, err := loader.ReadFile(`Interface\Glues\Models\UI_MainMenu_Northrend\UI_MainMenu_Northrend.m2`)
	if err != nil {
		t.Fatal(err)
	}
	model, err := parseM2(modelData)
	if err != nil {
		t.Fatal(err)
	}
	skinData, err := loader.ReadFile(`Interface\Glues\Models\UI_MainMenu_Northrend\UI_MainMenu_Northrend00.skin`)
	if err != nil {
		t.Fatal(err)
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		t.Fatal(err)
	}
	parts := buildM2Parts(model, skin)
	count := 0
	for _, part := range parts {
		for _, path := range part.texturePaths {
			if strings.HasSuffix(path, `PARTICLES\SNOW1.BLP`) {
				count++
				t.Logf("static snow batch submesh=%d vertices=%d indices=%d", part.submeshID, len(part.positions)/3, len(part.indices))
			}
		}
	}
	if count != 0 {
		t.Fatalf("main menu has %d static snow batches", count)
	}
}

func TestLiveMainMenuTextureAnimation(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	modelData, err := loader.ReadFile(`Interface\Glues\Models\UI_MainMenu_Northrend\UI_MainMenu_Northrend.m2`)
	if err != nil {
		t.Fatal(err)
	}
	model, err := parseM2(modelData)
	if err != nil {
		t.Fatal(err)
	}
	loadM2AnimationTracks(loader, `Interface\Glues\Models\UI_MainMenu_Northrend\UI_MainMenu_Northrend.m2`, &model)
	if len(model.textureTransforms) == 0 || !modelHasTextureAnimation(&model) {
		t.Fatalf("main menu has no animated texture transforms: %d", len(model.textureTransforms))
	}
	skinData, err := loader.ReadFile(`Interface\Glues\Models\UI_MainMenu_Northrend\UI_MainMenu_Northrend00.skin`)
	if err != nil {
		t.Fatal(err)
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		t.Fatal(err)
	}
	parts := buildM2Parts(model, skin)
	animated := 0
	for _, part := range parts {
		for _, index := range part.textureTransformIndices {
			if index >= 0 {
				animated++
				break
			}
		}
	}
	if animated == 0 {
		t.Fatal("main menu texture transforms are not referenced by any skin batch")
	}
	changed := false
	duration := model.sequences[0].duration
	for _, part := range parts {
		transform := -1
		for _, index := range part.textureTransformIndices {
			if index >= 0 {
				transform = index
				break
			}
		}
		if transform < 0 || len(part.uvs) < 2 {
			continue
		}
		mesh := &m2AnimatedMesh{part: part, baseUVs: append(math32.ArrayF32(nil), part.uvs...), textureTransformIndices: []int{transform}}
		animation := &m2Animation{model: &model, sequence: 0, meshes: []*m2AnimatedMesh{mesh}}
		animation.updateTextureCoordinates(0)
		start := append(math32.ArrayF32(nil), part.uvs...)
		animation.updateTextureCoordinates(duration / 2)
		for index := range start {
			if math.Abs(float64(start[index]-part.uvs[index])) > 0.00001 {
				changed = true
				break
			}
		}
		if changed {
			break
		}
	}
	if !changed {
		t.Fatal("main menu texture transforms do not change UV coordinates")
	}
	t.Logf("main menu texture transforms=%d animated batches=%d", len(model.textureTransforms), animated)
}

func TestLiveNightElfTextureAnimation(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	path := `Interface\Glues\Models\UI_NightElf\UI_NightElf.m2`
	modelData, err := loader.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	model, err := parseM2(modelData)
	if err != nil {
		t.Fatal(err)
	}
	loadM2AnimationTracks(loader, path, &model)
	skinData, err := loader.ReadFile(`Interface\Glues\Models\UI_NightElf\UI_NightElf00.skin`)
	if err != nil {
		t.Fatal(err)
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		t.Fatal(err)
	}
	parts := buildM2Parts(model, skin)
	animated := 0
	for index, transform := range model.textureTransforms {
		if !trackVec3HasKeys(transform.translation) && !trackQuatHasKeys(transform.rotation) && !trackVec3HasKeys(transform.scale) {
			continue
		}
		animated++
		if len(transform.translation.sequences) > 0 {
			t.Logf("NightElf transform %d global=%d translation times=%v values=%v", index, transform.translation.globalSequence, transform.translation.sequences[0].times, transform.translation.sequences[0].values)
		}
		if len(transform.rotation.sequences) > 0 {
			t.Logf("NightElf transform %d rotation=%v", index, transform.rotation.sequences[0].values)
		}
		if len(transform.scale.sequences) > 0 {
			t.Logf("NightElf transform %d scale=%v", index, transform.scale.sequences[0].values)
		}
	}
	t.Logf("NightElf sequences=%v texture transforms=%d animated=%d combos=%d", model.sequences, len(model.textureTransforms), animated, len(model.textureTransformCombos))
	for index, path := range model.textures {
		if strings.Contains(strings.ToLower(path), "sky") || strings.Contains(strings.ToLower(path), "cloud") || strings.Contains(strings.ToLower(path), "night") {
			t.Logf("NightElf texture %d flags=%#x path=%s", index, model.textureFlags[index], path)
		}
	}
	for index, combo := range model.textureTransformCombos {
		t.Logf("NightElf transform combo %d=%d", index, combo)
	}
	for key, part := range parts {
		t.Logf("NightElf part %s textures=%v transforms=%v uvSets=%d", key, part.texturePaths, part.textureTransformIndices, len(part.uvSets))
	}
	if animated == 0 {
		t.Fatal("Night Elf backdrop has no animated texture transforms")
	}
}

func TestLiveM2AnimationTracks(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	for _, path := range []string{`Interface\Glues\Models\UI_MainMenu_Northrend\UI_MainMenu_Northrend.m2`, `Interface\Glues\Models\UI_NightElf\UI_NightElf.m2`, `Character\NightElf\Female\NightElfFemale.m2`} {
		data, readErr := loader.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		model, parseErr := parseM2(data)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		loadM2AnimationTracks(loader, path, &model)
		if len(model.sequences) == 0 || !modelHasAnimation(&model) {
			t.Fatalf("%s has no decoded animation sequences", path)
		}
		sequence := defaultM2Sequence(&model)
		for index, bone := range model.bones {
			if got, want := bone.rotation, bone.rotationTrack.value(sequence, 0, model.globalLoops, [4]float32{0, 0, 0, 1}); got != want {
				t.Fatalf("%s bone=%d initial rotation=%v want primary sequence=%v", path, index, got, want)
			}
		}
		animation := &m2Animation{model: &model, sequence: sequence}
		mid := uint32(0)
		if model.sequences[0].duration > 1 {
			mid = model.sequences[0].duration / 2
		}
		start, middle := animation.poseBones(0), animation.poseBones(mid)
		changed := 0
		for index := range start {
			if start[index].translation != middle[index].translation || start[index].rotation != middle[index].rotation || start[index].scale != middle[index].scale {
				changed++
			}
		}
		ids := make([]string, 0, minM2TestInt(len(model.sequences), 24))
		for index, sequence := range model.sequences {
			if index >= 24 {
				break
			}
			ids = append(ids, fmt.Sprintf("%d:%d:%d", sequence.id, sequence.variation, sequence.duration))
		}
		idle := make([]string, 0)
		for index, sequence := range model.sequences {
			if sequence.id == 0 && sequence.duration > 0 {
				idle = append(idle, fmt.Sprintf("%d:%d:%d:%d", index, sequence.variation, sequence.duration, sequence.variationNext))
			}
		}
		t.Logf("%s sequences=%d first=(id=%d duration=%d flags=%#x) movingBones=%d firstSequences=%v idle=%v", path, len(model.sequences), model.sequences[0].id, model.sequences[0].duration, model.sequences[0].flags, changed, ids, idle)
	}
}

func minM2TestInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func TestLiveMainMenuSoundEvents(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	path := `Interface\Glues\Models\UI_MainMenu_Northrend\UI_MainMenu_Northrend.m2`
	data, err := loader.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	model, err := parseM2(data)
	if err != nil {
		t.Fatal(err)
	}
	loadM2AnimationTracks(loader, path, &model)
	soundData, err := loader.ReadFile(`DBFilesClient\SoundEntries.dbc`)
	if err != nil {
		t.Fatal(err)
	}
	sounds := parseSoundEntries(soundData)
	audio := &audioManager{loader: loader, catalog: sounds, pcm: make(map[string][]byte), failed: make(map[string]struct{})}
	count := 0
	for _, event := range model.events {
		name := string(event.identifier[:])
		if len(event.times) > 0 {
			count += len(event.times[0])
			entry := soundEntry{}
			for _, candidate := range sounds {
				if candidate.id == event.data {
					entry = candidate
					break
				}
			}
			if entry.id == 0 {
				t.Fatalf("sound event %d is absent from SoundEntries.dbc", event.data)
			}
			if _, _, soundErr := audio.pcmLocked(entryName(sounds, entry.id)); soundErr != nil {
				t.Fatalf("sound event %d: %v", event.data, soundErr)
			}
			t.Logf("main menu event name=%q data=%d files=%v directory=%s times=%v", name, event.data, entry.files, entry.directory, event.times[0])
		}
	}
	if count == 0 {
		t.Fatal("main menu has no decoded sound events")
	}
}

func TestLiveHumanCreateGeosets(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	path := `Character\Human\Male\HumanMale.m2`
	modelData, err := loader.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	model, err := parseM2(modelData)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Human model bones=%d", len(model.bones))
	skinData, err := loader.ReadFile(`Character\Human\Male\HumanMale00.skin`)
	if err != nil {
		t.Fatal(err)
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		t.Fatal(err)
	}
	active, err := resolveCharacterEquipment(loader, world.Character{Race: 1, Gender: 0}, &characterSectionTextures{}, skin)
	if err != nil {
		t.Fatal(err)
	}
	facial100, facial200, facial300 := resolveCharacterFacialHair(loader, world.Character{Race: 1, Gender: 0})
	t.Logf("Human default facial geosets=%d,%d,%d", facial100, facial200, facial300)
	ids := make([]uint16, 0, len(active))
	for id := range active {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	t.Logf("Human create active geosets=%v", ids)
}

func TestLiveBloodElfCharacterModel(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	sections, sectionErr := resolveCharacterSections(loader, world.Character{Race: 10, Gender: 1})
	if sectionErr != nil {
		t.Fatal(sectionErr)
	}
	t.Logf("Blood Elf sections=%+v", sections)
	model, err := loadGlueCharacterModel(loader, world.Character{GUID: 1, Race: 10, Gender: 1})
	if err != nil {
		t.Fatal(err)
	}
	if model == nil || len(model.Children()) == 0 {
		t.Fatal("Blood Elf character model has no drawable children")
	}
	model.Dispose()
}

func entryName(entries map[string]soundEntry, id uint32) string {
	for name, entry := range entries {
		if entry.id == id {
			return name
		}
	}
	return ""
}
