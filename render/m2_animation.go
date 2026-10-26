package render

import (
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/math32"
)

// m2MeshGroup is a mesh several parts share: they each pose a window of its
// arrays, and the mesh's buffers are sent whole.
type m2MeshGroup struct {
	positionVBO, normalVBO, uvVBO, uv2VBO, colorVBO, alphaVBO *gls.VBO
	positions, normals, uvs, uvs2, colors, alphas             math32.ArrayF32
}

func (m *m2AnimatedMesh) uploadPositions() {
	if g := m.group; g != nil {
		g.positionVBO.SetBuffer(g.positions)
		g.normalVBO.SetBuffer(g.normals)
		return
	}
	if m.positionVBO != nil {
		m.positionVBO.SetBuffer(m.part.positions)
	}
	if m.normalVBO != nil {
		m.normalVBO.SetBuffer(m.part.normals)
	}
}

func (m *m2AnimatedMesh) uploadTint() {
	if g := m.group; g != nil {
		g.colorVBO.SetBuffer(g.colors)
		g.alphaVBO.SetBuffer(g.alphas)
		return
	}
	if m.colorVBO != nil {
		m.colorVBO.SetBuffer(m.part.colors)
	}
	if m.alphaVBO != nil {
		m.alphaVBO.SetBuffer(m.part.alphas)
	}
}

func (m *m2AnimatedMesh) uploadUVs(second bool) {
	if g := m.group; g != nil {
		if second {
			if g.uv2VBO != nil {
				g.uv2VBO.SetBuffer(g.uvs2)
			}
		} else {
			g.uvVBO.SetBuffer(g.uvs)
		}
		return
	}
	if second {
		if m.uv2VBO != nil {
			m.uv2VBO.SetBuffer(m.part.uvs2)
		}
	} else if m.uvVBO != nil {
		m.uvVBO.SetBuffer(m.part.uvs)
	}
}

type m2AnimatedMesh struct {
	// group is set when the part's mesh is shared with others.
	group       *m2MeshGroup
	part        *m2Part
	positionVBO *gls.VBO
	normalVBO   *gls.VBO
	uvVBO       *gls.VBO
	uv2VBO      *gls.VBO
	colorVBO    *gls.VBO
	alphaVBO    *gls.VBO
	light       m2SceneLight
	// tintKnown is set once the colour and alpha buffers hold the part's tint.
	tintKnown               bool
	baseUVs                 math32.ArrayF32
	baseUVs2                math32.ArrayF32
	textureTransformIndices []int
}

type m2Animation struct {
	// skipSkin leaves the vertices, tints and texture coordinates as they are
	// while the bones still advance, for a model nobody can see this frame.
	skipSkin         bool
	boneMatrices     []m2BoneMatrix
	model            *parsedM2
	skin             parsedSkin
	meshes           []*m2AnimatedMesh
	sequence         int
	motionID         uint16
	clock            float64
	globalClock      float64
	idle             []int
	idleBase         int
	variationTimer   float64
	playingVariation bool
	variationEnabled bool
	random           uint32
	poseBuffer       []m2Bone
}

func buildM2Animation(model *parsedM2, skin parsedSkin, meshes []*m2AnimatedMesh, modelPath string) *m2Animation {
	if model == nil || len(model.sequences) == 0 || len(meshes) == 0 || (!modelHasAnimation(model) && !modelHasTextureAnimation(model) && !modelHasColorAnimation(model) && !modelHasParticleAnimation(model)) {
		return nil
	}
	sequence := defaultM2Sequence(model)
	idle := idleM2Sequences(model, model.sequences[sequence].id)
	if len(idle) == 0 {
		idle = []int{sequence}
	}
	seed := uint32(len(model.data)) + 0x9e3779b9
	for index := range modelPath {
		seed = seed*16777619 ^ uint32(modelPath[index])
	}
	animation := &m2Animation{model: model, skin: skin, meshes: meshes, sequence: sequence, motionID: model.sequences[sequence].id, idle: idle, idleBase: sequence, variationEnabled: model.sequences[sequence].id == 0, random: seed}
	animation.variationTimer = m2VariationTimer(animation, 3000, 11000)
	return animation
}

func (animation *m2Animation) SetMotion(id uint16) {
	if animation == nil || animation.model == nil {
		return
	}
	if animation.motionID == id && animation.sequence >= 0 && animation.sequence < len(animation.model.sequences) {
		return
	}
	sequence, id := m2SequenceWithFallback(animation.model, id)
	if sequence < 0 || sequence == animation.sequence && animation.motionID == id {
		return
	}
	animation.sequence = sequence
	animation.motionID = id
	animation.clock = 0
	animation.playingVariation = false
	animation.idle = idleM2Sequences(animation.model, id)
	if len(animation.idle) == 0 {
		animation.idle = []int{sequence}
	}
	animation.idleBase = sequence
	animation.variationEnabled = id == 0
	animation.variationTimer = m2VariationTimer(animation, 3000, 11000)
}

func (animation *m2Animation) SetSequence(index int) {
	if animation == nil || animation.model == nil || index < 0 || index >= len(animation.model.sequences) {
		return
	}
	animation.sequence = index
	animation.motionID = animation.model.sequences[index].id
	animation.clock = 0
	animation.idle = []int{index}
	animation.idleBase = index
	animation.variationEnabled = false
	animation.playingVariation = false
}

// SetAnimationID plays the animation with the given animation id, which is
// what a Model frame's SetSequence takes (ids from AnimationData, not an index
// into the model's sequence list). An id the model has no animation for leaves
// the current animation playing.
func (animation *m2Animation) SetAnimationID(id int) {
	if animation == nil || animation.model == nil || id < 0 || id > 0xffff {
		return
	}
	if index := m2SequenceIndex(animation.model, uint16(id)); index >= 0 {
		animation.SetSequence(index)
	}
}

func defaultM2Sequence(model *parsedM2) int {
	if sequence := m2SequenceIndex(model, 0); sequence >= 0 {
		return sequence
	}
	if sequence := m2SequenceIndex(model, 0x93); sequence >= 0 {
		return sequence
	}
	return 0
}

func m2SequenceIndex(model *parsedM2, id uint16) int {
	if model == nil {
		return -1
	}
	first := -1
	for index, sequence := range model.sequences {
		if sequence.id != id || sequence.duration == 0 {
			continue
		}
		if first < 0 {
			first = index
		}
		if sequence.variation == 0 {
			return index
		}
	}
	return first
}

func idleM2Sequences(model *parsedM2, id uint16) []int {
	result := make([]int, 0)
	for index, sequence := range model.sequences {
		if sequence.id == id && sequence.duration > 0 {
			result = append(result, index)
		}
	}
	return result
}

func nextM2Random(animation *m2Animation) uint32 {
	animation.random = animation.random*1664525 + 1013904223
	return animation.random
}

func m2VariationTimer(animation *m2Animation, low, high float64) float64 {
	if high <= low {
		return low
	}
	return low + float64(nextM2Random(animation)%uint32(high-low+1))
}

func modelHasAnimation(model *parsedM2) bool {
	for _, bone := range model.bones {
		if trackVec3HasKeys(bone.translationTrack) || trackQuatHasKeys(bone.rotationTrack) || trackVec3HasKeys(bone.scaleTrack) {
			return true
		}
	}
	return false
}

func modelHasTextureAnimation(model *parsedM2) bool {
	if model == nil {
		return false
	}
	for _, transform := range model.textureTransforms {
		if trackVec3HasKeys(transform.translation) || trackQuatHasKeys(transform.rotation) || trackVec3HasKeys(transform.scale) {
			return true
		}
	}
	return false
}

func modelHasColorAnimation(model *parsedM2) bool {
	if model == nil {
		return false
	}
	for _, color := range model.colors {
		if trackVec3HasKeys(color.colorTrack) || trackScalarHasKeys(color.alphaTrack) {
			return true
		}
	}
	for _, weight := range model.textureWeights {
		if trackScalarHasKeys(weight.weightTrack) {
			return true
		}
	}
	return false
}

func modelHasParticleAnimation(model *parsedM2) bool {
	if model == nil {
		return false
	}
	for _, emitter := range model.particles {
		if trackScalarHasKeys(emitter.rateTrack) {
			return true
		}
	}
	return false
}

func trackVec3HasKeys(track m2TrackVec3) bool {
	for _, sequence := range track.sequences {
		if len(sequence.values) > 1 {
			return true
		}
	}
	return false
}

func trackQuatHasKeys(track m2TrackQuat) bool {
	for _, sequence := range track.sequences {
		if len(sequence.values) > 1 {
			return true
		}
	}
	return false
}

func trackScalarHasKeys(track m2TrackScalar) bool {
	for _, sequence := range track.sequences {
		if len(sequence.values) > 1 {
			return true
		}
	}
	return false
}

func (animation *m2Animation) Update(elapsed float64) []uint32 {
	if animation == nil || animation.model == nil || animation.sequence < 0 || animation.sequence >= len(animation.model.sequences) || elapsed <= 0 {
		return nil
	}
	if elapsed > 0.25 {
		elapsed = 0.25
	}
	sequence := animation.model.sequences[animation.sequence]
	if sequence.duration == 0 {
		return nil
	}
	previous := uint32(animation.clock)
	animation.globalClock += elapsed * 1000
	animation.clock += elapsed * 1000
	wrapped := false
	if animation.variationEnabled && !animation.playingVariation {
		animation.variationTimer -= elapsed * 1000
	}
	if animation.clock >= float64(sequence.duration) {
		if animation.playingVariation {
			animation.playingVariation = false
			animation.sequence = animation.idleBase
			animation.clock = 0
			animation.variationTimer = m2VariationTimer(animation, 4000, 10000)
			sequence = animation.model.sequences[animation.sequence]
		} else {
			animation.clock = animation.clock - float64(sequence.duration)*float64(uint64(animation.clock)/uint64(sequence.duration))
			wrapped = true
		}
	}
	if animation.variationEnabled && !animation.playingVariation && animation.variationTimer <= 0 && len(animation.idle) > 1 {
		choice := animation.idle[int(nextM2Random(animation)%uint32(len(animation.idle)))]
		if choice != animation.sequence {
			animation.playingVariation = true
			animation.sequence = choice
			animation.clock = 0
			sequence = animation.model.sequences[animation.sequence]
		} else {
			animation.variationTimer = m2VariationTimer(animation, 2000, 6000)
		}
	}
	current := uint32(animation.clock)
	bones := animation.poseBonesAtReusable(uint32(animation.clock), uint32(animation.globalClock))
	for index, bone := range bones {
		animation.model.bones[index].translation = bone.translation
		animation.model.bones[index].rotation = bone.rotation
		animation.model.bones[index].scale = bone.scale
	}
	updateM2AnimatedValues(animation.model, animation.sequence, uint32(animation.clock), uint32(animation.globalClock))
	animation.boneMatrices = buildM2BoneMatrices(bones, animation.boneMatrices)
	if animation.skipSkin {
		return animation.crossedSounds(animation.sequence, previous, current, sequence.duration, wrapped)
	}
	for _, animatedMesh := range animation.meshes {
		if animatedMesh == nil || animatedMesh.part == nil || len(animatedMesh.part.vertexRefs) != len(animatedMesh.part.positions)/3 {
			continue
		}
		for index, ref := range animatedMesh.part.vertexRefs {
			posed := poseM2VertexWithMatrices(ref.vertex, animation.boneMatrices)
			position := modelVector(posed.position)
			normal := modelVector(posed.normal)
			animatedMesh.part.positions.Set(index*3, position[0], position[1], position[2])
			animatedMesh.part.normals.Set(index*3, normal[0], normal[1], normal[2])
		}
		animatedMesh.uploadPositions()
	}
	for _, animatedMesh := range animation.meshes {
		if animatedMesh == nil || animatedMesh.part == nil {
			continue
		}
		part := animatedMesh.part
		color, alpha := m2PartTintAt(animation.model, part.colorIndex, part.textureWeightIndex, animation.sequence, uint32(animation.clock), uint32(animation.globalClock))
		// A tint that has not changed leaves the buffers as they are and the GPU copy alone.
		if animatedMesh.tintKnown && !animatedMesh.light.enabled && color == part.color && alpha == part.alpha {
			continue
		}
		part.color, part.alpha = color, alpha
		animatedMesh.tintKnown = true
		for index := 0; index+2 < len(part.colors); index += 3 {
			part.colors[index] = part.color[0]
			part.colors[index+1] = part.color[1]
			part.colors[index+2] = part.color[2]
		}
		applyM2SceneLight(part, animatedMesh.light)
		for index := range part.alphas {
			part.alphas[index] = part.alpha
		}
		animatedMesh.uploadTint()
	}
	animation.updateTextureCoordinatesAt(uint32(animation.clock), uint32(animation.globalClock))
	return animation.crossedSounds(animation.sequence, previous, current, sequence.duration, wrapped)
}

func (animation *m2Animation) updateTextureCoordinates(timeMS uint32) {
	animation.updateTextureCoordinatesAt(timeMS, timeMS)
}

func (animation *m2Animation) updateTextureCoordinatesAt(timeMS, globalTimeMS uint32) {
	if animation == nil || animation.model == nil {
		return
	}
	for _, animatedMesh := range animation.meshes {
		if animatedMesh == nil || animatedMesh.part == nil || len(animatedMesh.textureTransformIndices) == 0 {
			continue
		}
		if len(animatedMesh.baseUVs) == len(animatedMesh.part.uvs) {
			copy(animatedMesh.part.uvs, animatedMesh.baseUVs)
			if index := animatedMesh.textureTransformIndices[0]; index >= 0 && index < len(animation.model.textureTransforms) {
				applyM2TextureTransform(animatedMesh.part.uvs, animatedMesh.baseUVs, animation.model.textureTransforms[index], animation.sequence, timeMS, globalTimeMS, animation.model.globalLoops)
			}
			animatedMesh.uploadUVs(false)
		}
		if len(animatedMesh.baseUVs2) == len(animatedMesh.part.uvs2) && len(animatedMesh.textureTransformIndices) > 1 {
			copy(animatedMesh.part.uvs2, animatedMesh.baseUVs2)
			if index := animatedMesh.textureTransformIndices[1]; index >= 0 && index < len(animation.model.textureTransforms) {
				applyM2TextureTransform(animatedMesh.part.uvs2, animatedMesh.baseUVs2, animation.model.textureTransforms[index], animation.sequence, timeMS, globalTimeMS, animation.model.globalLoops)
			}
			animatedMesh.uploadUVs(true)
		}
	}
}

func applyM2TextureTransform(dst, base math32.ArrayF32, transform m2TextureTransform, sequence int, timeMS, globalTimeMS uint32, globalLoops []uint32) {
	translation := transform.translation.value(sequence, m2TrackTime(transform.translation.globalSequence, timeMS, globalTimeMS), globalLoops, [3]float32{})
	rotation := transform.rotation.value(sequence, m2TrackTime(transform.rotation.globalSequence, timeMS, globalTimeMS), globalLoops, [4]float32{0, 0, 0, 1})
	scale := transform.scale.value(sequence, m2TrackTime(transform.scale.globalSequence, timeMS, globalTimeMS), globalLoops, [3]float32{1, 1, 1})
	angle := float32(2 * math.Atan2(float64(rotation[2]), float64(rotation[3])))
	cosine, sine := float32(math.Cos(float64(angle))), float32(math.Sin(float64(angle)))
	for index := 0; index+1 < len(base) && index+1 < len(dst); index += 2 {
		x, y := (base[index]-0.5)*scale[0], (base[index+1]-0.5)*scale[1]
		dst[index] = x*cosine - y*sine + 0.5 + translation[0]
		dst[index+1] = x*sine + y*cosine + 0.5 + translation[1]
	}
}

func (animation *m2Animation) crossedSounds(sequence int, previous, current, duration uint32, wrapped bool) []uint32 {
	if animation == nil || animation.model == nil || len(animation.model.events) == 0 {
		return nil
	}
	sounds := make([]uint32, 0)
	for _, event := range animation.model.events {
		if !isM2SoundEvent(event.identifier) || sequence < 0 || sequence >= len(event.times) {
			continue
		}
		for _, timestamp := range event.times[sequence] {
			if (!wrapped && timestamp > previous && timestamp <= current) || (wrapped && ((timestamp > previous && timestamp <= duration) || timestamp <= current)) {
				sounds = append(sounds, event.data)
			}
		}
	}
	return sounds
}

func isM2SoundEvent(identifier [4]byte) bool {
	return identifier == [4]byte{'$', 'C', 'S', 'D'} || identifier == [4]byte{'$', 'D', 'S', 'O'}
}

func m2TrackTime(globalSequence uint16, sequenceTime, globalTime uint32) uint32 {
	if globalSequence != 0xffff {
		return globalTime
	}
	return sequenceTime
}

func (animation *m2Animation) poseBones(timeMS uint32) []m2Bone {
	return animation.poseBonesAt(timeMS, timeMS)
}

func (animation *m2Animation) poseBonesAt(timeMS, globalTimeMS uint32) []m2Bone {
	bones := make([]m2Bone, len(animation.model.bones))
	return animation.fillPoseBones(bones, timeMS, globalTimeMS)
}

func (animation *m2Animation) poseBonesAtReusable(timeMS, globalTimeMS uint32) []m2Bone {
	bones := animation.model.bones
	if len(animation.poseBuffer) != len(bones) {
		// The fixed part of each bone is copied once; a pose only sets the moving part.
		animation.poseBuffer = make([]m2Bone, len(bones))
		copy(animation.poseBuffer, bones)
	}
	loops := animation.model.globalLoops
	for index := range bones {
		source, bone := &bones[index], &animation.poseBuffer[index]
		bone.translation = source.translationTrack.value(animation.sequence, m2TrackTime(source.translationTrack.globalSequence, timeMS, globalTimeMS), loops, source.translation)
		bone.rotation = source.rotationTrack.value(animation.sequence, m2TrackTime(source.rotationTrack.globalSequence, timeMS, globalTimeMS), loops, source.rotation)
		bone.scale = source.scaleTrack.value(animation.sequence, m2TrackTime(source.scaleTrack.globalSequence, timeMS, globalTimeMS), loops, source.scale)
	}
	return animation.poseBuffer
}

func (animation *m2Animation) fillPoseBones(bones []m2Bone, timeMS, globalTimeMS uint32) []m2Bone {
	for index, source := range animation.model.bones {
		bone := source
		bone.translation = bone.translationTrack.value(animation.sequence, m2TrackTime(bone.translationTrack.globalSequence, timeMS, globalTimeMS), animation.model.globalLoops, source.translation)
		bone.rotation = bone.rotationTrack.value(animation.sequence, m2TrackTime(bone.rotationTrack.globalSequence, timeMS, globalTimeMS), animation.model.globalLoops, source.rotation)
		bone.scale = bone.scaleTrack.value(animation.sequence, m2TrackTime(bone.scaleTrack.globalSequence, timeMS, globalTimeMS), animation.model.globalLoops, source.scale)
		bones[index] = bone
	}
	return bones
}

// m2AnimationFallbacks maps an animation id to the one AnimationData.dbc says
// to play when a model has no sequence for it (flyers have no "fly forward",
// say, and fall back to run).
var m2AnimationFallbacks map[uint16]uint16

func readAnimationFallbacks(loader *ui.Loader) map[uint16]uint16 {
	result := make(map[uint16]uint16)
	readWorldDBCRows(loader, `DBFilesClient\AnimationData.dbc`, 6, func(row []uint32) {
		if row[5] != 0 && row[5] != row[0] {
			result[uint16(row[0])] = uint16(row[5])
		}
	})
	return result
}

// m2SequenceWithFallback finds the sequence for an animation id, following the
// fallback chain, and the id actually used.
func m2SequenceWithFallback(model *parsedM2, id uint16) (int, uint16) {
	for steps := 0; steps < 8; steps++ {
		if index := m2SequenceIndex(model, id); index >= 0 {
			return index, id
		}
		next, ok := m2AnimationFallbacks[id]
		if !ok {
			break
		}
		id = next
	}
	return m2SequenceIndex(model, 0), 0
}
