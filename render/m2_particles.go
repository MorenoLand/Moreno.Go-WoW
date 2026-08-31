package render

import (
	"encoding/binary"
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/texture"
)

type m2ParticleEmitter struct {
	flags           uint32
	position        [3]float32
	bone            uint16
	texture         uint16
	blend           uint8
	emitterType     uint8
	rows            uint16
	cols            uint16
	rate            float32
	life            float32
	speed           float32
	verticalRange   float32
	horizontalRange float32
	gravity         float32
	areaLength      float32
	areaWidth       float32
	color           [3]float32
	alpha           float32
	scale           [2]float32
}

type m2Particle struct {
	origin   [3]float32
	velocity [3]float32
	gravity  [3]float32
	age      float32
	life     float32
	seed     uint32
}

type m2ParticleGroup struct {
	emitter     m2ParticleEmitter
	model       parsedM2
	base        [3]float32
	right       [3]float32
	up          [3]float32
	particles   []m2Particle
	positions   math32.ArrayF32
	positionVBO *gls.VBO
	points      *graphic.Points
	particleTex *texture.Texture2D
}

type m2ParticleSystem struct {
	groups       []*m2ParticleGroup
	time         float32
	emitterCount int
	pointCount   int
}

type particleTextureKey struct {
	path string
	rows uint16
	cols uint16
}

func parseM2ParticleEmitter(data []byte, base int) m2ParticleEmitter {
	emitter := m2ParticleEmitter{
		flags:       binaryU32(data, base+0x04),
		position:    [3]float32{readF32(data, base+0x08), readF32(data, base+0x0c), readF32(data, base+0x10)},
		bone:        binaryU16(data, base+0x14),
		texture:     binaryU16(data, base+0x16),
		blend:       data[base+0x28],
		emitterType: data[base+0x29],
		rows:        binaryU16(data, base+0x30),
		cols:        binaryU16(data, base+0x32),
		color:       [3]float32{1, 1, 1},
		alpha:       1,
		scale:       [2]float32{1, 1},
	}
	if emitter.rows == 0 {
		emitter.rows = 1
	}
	if emitter.cols == 0 {
		emitter.cols = 1
	}
	emitter.speed = readM2TrackFloat(data, base+0x34)
	emitter.verticalRange = readM2TrackFloat(data, base+0x5c)
	emitter.horizontalRange = readM2TrackFloat(data, base+0x70)
	emitter.gravity = readM2TrackFloat(data, base+0x84)
	emitter.life = readM2TrackFloat(data, base+0x98)
	emitter.rate = readM2TrackFloat(data, base+0xb0)
	emitter.areaLength = readM2TrackFloat(data, base+0xc8)
	emitter.areaWidth = readM2TrackFloat(data, base+0xdc)
	if key, ok := readM2FBlockKey(data, base+0x104, 12); ok {
		emitter.color = [3]float32{readF32(data, key) / 255, readF32(data, key+4) / 255, readF32(data, key+8) / 255}
	}
	if key, ok := readM2FBlockKey(data, base+0x114, 2); ok {
		emitter.alpha = float32(binaryU16(data, key)) / 32767
	}
	if key, ok := readM2FBlockKey(data, base+0x124, 8); ok {
		emitter.scale = [2]float32{readF32(data, key), readF32(data, key+4)}
	}
	emitter.rate = finiteParticleValue(emitter.rate)
	emitter.life = finiteParticleValue(emitter.life)
	emitter.speed = finiteParticleValue(emitter.speed)
	emitter.verticalRange = finiteParticleValue(emitter.verticalRange)
	emitter.horizontalRange = finiteParticleValue(emitter.horizontalRange)
	emitter.gravity = finiteParticleValue(emitter.gravity)
	emitter.areaLength = finiteParticleValue(emitter.areaLength)
	emitter.areaWidth = finiteParticleValue(emitter.areaWidth)
	for index := range emitter.color {
		emitter.color[index] = clampParticleValue(emitter.color[index], 0, 1)
	}
	emitter.alpha = clampParticleValue(emitter.alpha, 0, 1)
	for index := range emitter.scale {
		emitter.scale[index] = finiteParticleValue(emitter.scale[index])
		if emitter.scale[index] <= 0 {
			emitter.scale[index] = 1
		}
	}
	return emitter
}

func readM2TrackFloat(data []byte, offset int) float32 {
	key, ok := readM2TrackKey(data, offset, 4)
	if !ok {
		return 0
	}
	return readF32(data, key)
}

func readM2FBlockKey(data []byte, offset, valueSize int) (int, bool) {
	if offset < 0 || offset+16 > len(data) {
		return 0, false
	}
	count := int(binary.LittleEndian.Uint32(data[offset+8 : offset+12]))
	keyOffset := int(binary.LittleEndian.Uint32(data[offset+12 : offset+16]))
	if count <= 0 || keyOffset < 0 || keyOffset+valueSize > len(data) {
		return 0, false
	}
	return keyOffset, true
}

func binaryU16(data []byte, offset int) uint16 {
	return binary.LittleEndian.Uint16(data[offset : offset+2])
}

func binaryU32(data []byte, offset int) uint32 {
	return binary.LittleEndian.Uint32(data[offset : offset+4])
}

func finiteParticleValue(value float32) float32 {
	if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
		return 0
	}
	return value
}

func clampParticleValue(value, low, high float32) float32 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func buildM2ParticleSystem(loader *ui.Loader, model parsedM2, root *core.Node, rootScale float32, textures map[string]*texture.Texture2D) *m2ParticleSystem {
	if len(model.particles) == 0 {
		return nil
	}
	right, up := particleBasis(model)
	particleTextures := make(map[particleTextureKey]*texture.Texture2D)
	system := &m2ParticleSystem{}
	for index, emitter := range model.particles {
		if emitter.rate <= 0 || emitter.life <= 0 || int(emitter.texture) >= len(model.textures) {
			continue
		}
		path := model.textures[emitter.texture]
		if path == "" {
			continue
		}
		key := particleTextureKey{path: path, rows: emitter.rows, cols: emitter.cols}
		tex := particleTextures[key]
		if tex == nil {
			if len(particleTextures) == 0 {
				tex = textures[path]
			}
			if tex == nil {
				tex = loadModelTexture(loader, path)
			}
			if tex == nil {
				continue
			}
			tex.SetWrapS(gls.REPEAT)
			tex.SetWrapT(gls.REPEAT)
			tex.SetRepeat(1/float32(emitter.cols), 1/float32(emitter.rows))
			tex.SetOffset(0, 0)
			particleTextures[key] = tex
		}
		count := int(math.Ceil(float64(emitter.rate * emitter.life)))
		if count < 1 {
			count = 1
		}
		if count > 64 {
			count = 64
		}
		base := modelVector(transformM2Point(int(emitter.bone), emitter.position, model.bones, 0))
		group := &m2ParticleGroup{emitter: emitter, model: model, base: base, right: right, up: up, particleTex: tex, particles: make([]m2Particle, count), positions: math32.NewArrayF32(0, count*3)}
		for particleIndex := range group.particles {
			seed := uint32(index+1)*0x9e3779b9 + uint32(particleIndex+1)*0x85ebca6b
			group.particles[particleIndex] = spawnM2Particle(group, seed, emitter.life*float32(particleIndex)/float32(count))
			position := particlePosition(group.particles[particleIndex])
			group.positions.Append(position[0], position[1], position[2])
		}
		geom := geometry.NewGeometry()
		geom.AddVBO(gls.NewVBO(group.positions).AddAttrib(gls.VertexPosition))
		mat := material.NewPoint(&math32.Color{R: emitter.color[0], G: emitter.color[1], B: emitter.color[2]})
		mat.SetEmissiveColor(&math32.Color{R: emitter.color[0], G: emitter.color[1], B: emitter.color[2]})
		pointSize := emitter.scale[0] * rootScale * 100
		if pointSize < 2 {
			pointSize = 2
		}
		if pointSize > 16 {
			pointSize = 16
		}
		mat.SetSize(pointSize)
		mat.SetOpacity(emitter.alpha)
		mat.SetSide(material.SideDouble)
		mat.SetUseLights(material.UseLightNone)
		mat.SetDepthTest(true)
		mat.SetDepthMask(false)
		mat.SetShaderUnique(true)
		switch emitter.blend {
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
		mat.AddTexture(tex)
		points := graphic.NewPoints(geom, mat)
		points.SetRenderOrder(-10)
		points.SetCullable(false)
		group.positionVBO = geom.VBO(gls.VertexPosition)
		group.points = points
		root.Add(points)
		system.groups = append(system.groups, group)
		system.emitterCount++
		system.pointCount += count
	}
	if len(system.groups) == 0 {
		return nil
	}
	return system
}

func particleBasis(model parsedM2) ([3]float32, [3]float32) {
	up := [3]float32{0, 1, 0}
	direction := [3]float32{-1, 0, 0}
	if model.camera != nil {
		position := modelVector(model.camera.position)
		target := modelVector(model.camera.target)
		direction = subParticleVector(target, position)
	}
	right := crossParticleVector(direction, up)
	length := particleVectorLength(right)
	if length < 0.001 {
		right = [3]float32{0, 0, 1}
	} else {
		right = scaleParticleVector(right, 1/length)
	}
	return right, up
}

func spawnM2Particle(group *m2ParticleGroup, seed uint32, age float32) m2Particle {
	emitter := group.emitter
	random := func() float32 {
		seed = seed*1664525 + 1013904223
		return float32(seed>>8) / float32(1<<24)
	}
	randomSigned := func() float32 { return random()*2 - 1 }
	offset := [3]float32{randomSigned() * emitter.areaWidth * 0.5, randomSigned() * emitter.areaLength * 0.5, 0}
	offset = modelVector(transformM2Direction(int(emitter.bone), offset, group.model.bones, 0))
	velocity := [3]float32{}
	if math.Abs(float64(emitter.speed)) >= 0.01 {
		direction := [3]float32{randomSigned() * emitter.horizontalRange, randomSigned() * emitter.horizontalRange, 1 + randomSigned()*emitter.verticalRange}
		length := particleVectorLength(direction)
		if length > 0.001 {
			direction = scaleParticleVector(direction, 1/length)
		}
		velocity = scaleParticleVector(modelVector(transformM2Direction(int(emitter.bone), direction, group.model.bones, 0)), emitter.speed)
	} else {
		velocity = modelVector(transformM2Direction(int(emitter.bone), [3]float32{randomSigned(), randomSigned(), -random() * 0.5}, group.model.bones, 0))
	}
	return m2Particle{origin: addParticleVector(group.base, offset), velocity: velocity, gravity: [3]float32{0, -emitter.gravity, 0}, age: age, life: emitter.life, seed: seed}
}

func (system *m2ParticleSystem) Update(elapsed float64) {
	if system == nil || elapsed <= 0 {
		return
	}
	delta := float32(elapsed)
	if delta > 0.25 {
		delta = 0.25
	}
	system.time += delta
	for _, group := range system.groups {
		for index := range group.particles {
			particle := &group.particles[index]
			particle.age += delta
			if particle.age >= particle.life {
				particle.seed += 0x9e3779b9
				*particle = spawnM2Particle(group, particle.seed, 0)
			}
			position := particlePosition(*particle)
			group.positions.Set(index*3, position[0], position[1], position[2])
		}
		group.positionVBO.SetBuffer(group.positions)
		cells := int(group.emitter.rows) * int(group.emitter.cols)
		if cells > 1 {
			frame := int(math.Floor(float64(system.time*8))) % cells
			row := frame / int(group.emitter.cols)
			col := frame % int(group.emitter.cols)
			group.particleTex.SetOffset(float32(col)/float32(group.emitter.cols), float32(row)/float32(group.emitter.rows))
		}
	}
}

func particlePosition(p m2Particle) [3]float32 {
	return addParticleVector(p.origin, addParticleVector(scaleParticleVector(p.velocity, p.age), scaleParticleVector(p.gravity, 0.5*p.age*p.age)))
}

func transformM2Direction(index int, value [3]float32, bones []m2Bone, depth int) [3]float32 {
	if index < 0 || index >= len(bones) || depth > len(bones) {
		return value
	}
	value = rotateM2Vector(bones[index].rotation, value)
	if bones[index].parent >= 0 {
		return transformM2Direction(int(bones[index].parent), value, bones, depth+1)
	}
	return value
}

func addParticleVector(left, right [3]float32) [3]float32 {
	return [3]float32{left[0] + right[0], left[1] + right[1], left[2] + right[2]}
}

func subParticleVector(left, right [3]float32) [3]float32 {
	return [3]float32{left[0] - right[0], left[1] - right[1], left[2] - right[2]}
}

func scaleParticleVector(value [3]float32, scale float32) [3]float32 {
	return [3]float32{value[0] * scale, value[1] * scale, value[2] * scale}
}

func crossParticleVector(left, right [3]float32) [3]float32 {
	return [3]float32{left[1]*right[2] - left[2]*right[1], left[2]*right[0] - left[0]*right[2], left[0]*right[1] - left[1]*right[0]}
}

func particleVectorLength(value [3]float32) float32 {
	return float32(math.Sqrt(float64(value[0]*value[0] + value[1]*value[1] + value[2]*value[2])))
}
