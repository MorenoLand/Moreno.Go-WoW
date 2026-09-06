package render

import (
	"math"

	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/math32"
)

type m2AnimatedMesh struct {
	part                    *m2Part
	positionVBO             *gls.VBO
	normalVBO               *gls.VBO
	uvVBO                   *gls.VBO
	uv2VBO                  *gls.VBO
	colorVBO                *gls.VBO
	alphaVBO                *gls.VBO
	baseUVs                 math32.ArrayF32
	baseUVs2                math32.ArrayF32
	textureTransformIndices []int
}

type m2Animation struct {
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
	sequence := m2SequenceIndex(animation.model, id)
	if sequence < 0 {
		id = 0
		sequence = m2SequenceIndex(animation.model, id)
	}
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
	for _, animatedMesh := range animation.meshes {
		if animatedMesh == nil || animatedMesh.part == nil || len(animatedMesh.part.vertexRefs) != len(animatedMesh.part.positions)/3 {
			continue
		}
		for index, ref := range animatedMesh.part.vertexRefs {
			posed := poseM2VertexWithBones(animation.model, animation.skin, ref.local, ref.vertex, ref.boneComboIndex, bones)
			position := modelVector(posed.position)
			normal := modelVector(posed.normal)
			animatedMesh.part.positions.Set(index*3, position[0], position[1], position[2])
			animatedMesh.part.normals.Set(index*3, normal[0], normal[1], normal[2])
		}
		if animatedMesh.positionVBO != nil {
			animatedMesh.positionVBO.SetBuffer(animatedMesh.part.positions)
		}
		if animatedMesh.normalVBO != nil {
			animatedMesh.normalVBO.SetBuffer(animatedMesh.part.normals)
		}
	}
	for _, animatedMesh := range animation.meshes {
		if animatedMesh == nil || animatedMesh.part == nil {
			continue
		}
		part := animatedMesh.part
		part.color, part.alpha = m2PartTintAt(animation.model, part.colorIndex, part.textureWeightIndex, animation.sequence, uint32(animation.clock), uint32(animation.globalClock))
		for index := 0; index+2 < len(part.colors); index += 3 {
			part.colors[index] = part.color[0]
			part.colors[index+1] = part.color[1]
			part.colors[index+2] = part.color[2]
		}
		for index := range part.alphas {
			part.alphas[index] = part.alpha
		}
		if animatedMesh.colorVBO != nil {
			animatedMesh.colorVBO.SetBuffer(part.colors)
		}
		if animatedMesh.alphaVBO != nil {
			animatedMesh.alphaVBO.SetBuffer(part.alphas)
		}
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
			if animatedMesh.uvVBO != nil {
				animatedMesh.uvVBO.SetBuffer(animatedMesh.part.uvs)
			}
		}
		if len(animatedMesh.baseUVs2) == len(animatedMesh.part.uvs2) && len(animatedMesh.textureTransformIndices) > 1 {
			copy(animatedMesh.part.uvs2, animatedMesh.baseUVs2)
			if index := animatedMesh.textureTransformIndices[1]; index >= 0 && index < len(animation.model.textureTransforms) {
				applyM2TextureTransform(animatedMesh.part.uvs2, animatedMesh.baseUVs2, animation.model.textureTransforms[index], animation.sequence, timeMS, globalTimeMS, animation.model.globalLoops)
			}
			if animatedMesh.uv2VBO != nil {
				animatedMesh.uv2VBO.SetBuffer(animatedMesh.part.uvs2)
			}
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
	if cap(animation.poseBuffer) < len(animation.model.bones) {
		animation.poseBuffer = make([]m2Bone, len(animation.model.bones))
	} else {
		animation.poseBuffer = animation.poseBuffer[:len(animation.model.bones)]
	}
	return animation.fillPoseBones(animation.poseBuffer, timeMS, globalTimeMS)
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
