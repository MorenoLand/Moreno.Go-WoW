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
	colorIndex      uint16
	blend           uint8
	emitterType     uint8
	rows            uint16
	cols            uint16
	textureRotation int16
	rate            float32
	life            float32
	rateTrack       m2TrackScalar
	speed           float32
	speedVariation  float32
	verticalRange   float32
	horizontalRange float32
	gravity         float32
	areaLength      float32
	areaWidth       float32
	lifespanVary    float32
	rateVary        float32
	twinkleSpeed    float32
	twinklePercent  float32
	twinkleScale    [2]float32
	baseSpin        float32
	baseSpinVary    float32
	spin            float32
	spinVary        float32
	drag            float32
	color           [3]float32
	alpha           float32
	scale           [2]float32
	colorTrack      m2ParticleTrack
	alphaTrack      m2ParticleTrack
	scaleTrack      m2ParticleTrack
	headCellTrack   m2ParticleTrack
}

type m2ParticleTrack struct {
	times      []float32
	values     []float32
	components int
}

type m2Particle struct {
	origin   [3]float32
	velocity [3]float32
	gravity  [3]float32
	age      float32
	life     float32
	seed     uint32
	rotation float32
	spin     float32
	drag     float32
}

type m2ParticleGroup struct {
	emitter     m2ParticleEmitter
	model       *parsedM2
	bone        int
	base        [3]float32
	right       [3]float32
	up          [3]float32
	particles   []m2Particle
	positions   math32.ArrayF32
	colors      math32.ArrayF32
	params      math32.ArrayF32
	alphas      math32.ArrayF32
	rotations   math32.ArrayF32
	corners     math32.ArrayF32
	positionVBO *gls.VBO
	colorVBO    *gls.VBO
	paramsVBO   *gls.VBO
	alphaVBO    *gls.VBO
	rotationVBO *gls.VBO
	cornerVBO   *gls.VBO
	mesh        *graphic.Mesh
	particleTex *texture.Texture2D
	rootScale   float32
	activeCount int
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
		flags:           binaryU32(data, base+0x04),
		position:        [3]float32{readF32(data, base+0x08), readF32(data, base+0x0c), readF32(data, base+0x10)},
		bone:            binaryU16(data, base+0x14),
		texture:         binaryU16(data, base+0x16),
		colorIndex:      binaryU16(data, base+0x2a),
		blend:           data[base+0x28],
		emitterType:     data[base+0x29],
		textureRotation: int16(binaryU16(data, base+0x2e)),
		rows:            binaryU16(data, base+0x30),
		cols:            binaryU16(data, base+0x32),
		color:           [3]float32{255, 255, 255},
		alpha:           1,
		scale:           [2]float32{1, 1},
	}
	if emitter.rows == 0 {
		emitter.rows = 1
	}
	if emitter.cols == 0 {
		emitter.cols = 1
	}
	emitter.speed = readM2TrackFloat(data, base+0x34)
	emitter.speedVariation = readM2TrackFloat(data, base+0x48)
	emitter.verticalRange = readM2TrackFloat(data, base+0x5c)
	emitter.horizontalRange = readM2TrackFloat(data, base+0x70)
	emitter.gravity = readM2TrackFloat(data, base+0x84)
	emitter.life = readM2TrackFloat(data, base+0x98)
	emitter.lifespanVary = readF32(data, base+0xac)
	emitter.rate = readM2TrackFloat(data, base+0xb0)
	emitter.rateVary = readF32(data, base+0xc4)
	emitter.areaLength = readM2TrackFloat(data, base+0xc8)
	emitter.areaWidth = readM2TrackFloat(data, base+0xdc)
	emitter.lifespanVary = finiteParticleValue(emitter.lifespanVary)
	emitter.rateVary = finiteParticleValue(emitter.rateVary)
	emitter.twinkleSpeed = finiteParticleValue(readF32(data, base+0x160))
	emitter.twinklePercent = clampParticleValue(finiteParticleValue(readF32(data, base+0x164)), 0, 1)
	emitter.twinkleScale = [2]float32{finiteParticleValue(readF32(data, base+0x168)), finiteParticleValue(readF32(data, base+0x16c))}
	emitter.baseSpin = finiteParticleValue(readF32(data, base+0x178))
	emitter.baseSpinVary = finiteParticleValue(readF32(data, base+0x17c))
	emitter.spin = finiteParticleValue(readF32(data, base+0x180))
	emitter.spinVary = finiteParticleValue(readF32(data, base+0x184))
	emitter.drag = finiteParticleValue(readF32(data, base+0x174))
	if key, ok := readM2FBlockKey(data, base+0x104, 12); ok {
		emitter.color = [3]float32{readF32(data, key), readF32(data, key+4), readF32(data, key+8)}
	}
	if key, ok := readM2FBlockKey(data, base+0x114, 2); ok {
		emitter.alpha = float32(binaryU16(data, key)) / 32767
	}
	if key, ok := readM2FBlockKey(data, base+0x124, 8); ok {
		emitter.scale = [2]float32{readF32(data, key), readF32(data, key+4)}
	}
	emitter.colorTrack = readM2ParticleTrack(data, base+260, 3, 4, false)
	emitter.alphaTrack = readM2ParticleTrack(data, base+276, 1, 2, true)
	emitter.scaleTrack = readM2ParticleTrack(data, base+292, 2, 4, false)
	emitter.headCellTrack = readM2ParticleTrack(data, base+316, 1, 2, false)
	emitter.rate = finiteParticleValue(emitter.rate)
	emitter.life = finiteParticleValue(emitter.life)
	emitter.speed = finiteParticleValue(emitter.speed)
	emitter.speedVariation = finiteParticleValue(emitter.speedVariation)
	emitter.verticalRange = finiteParticleValue(emitter.verticalRange)
	emitter.horizontalRange = finiteParticleValue(emitter.horizontalRange)
	emitter.gravity = finiteParticleValue(emitter.gravity)
	emitter.areaLength = finiteParticleValue(emitter.areaLength)
	emitter.areaWidth = finiteParticleValue(emitter.areaWidth)
	for index := range emitter.color {
		emitter.color[index] = clampParticleValue(emitter.color[index], 0, 255)
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

func readM2ParticleTrack(data []byte, base, components, componentSize int, fixed16 bool) m2ParticleTrack {
	if base < 0 || base+16 > len(data) || components < 1 || componentSize < 1 {
		return m2ParticleTrack{}
	}
	timeCount := int(binary.LittleEndian.Uint32(data[base : base+4]))
	timeOffset := int(binary.LittleEndian.Uint32(data[base+4 : base+8]))
	valueCount := int(binary.LittleEndian.Uint32(data[base+8 : base+12]))
	valueOffset := int(binary.LittleEndian.Uint32(data[base+12 : base+16]))
	count := timeCount
	if valueCount < count {
		count = valueCount
	}
	if count <= 0 || timeOffset < 0 || valueOffset < 0 || count > (len(data)-timeOffset)/2 || count > (len(data)-valueOffset)/(components*componentSize) {
		return m2ParticleTrack{}
	}
	track := m2ParticleTrack{times: make([]float32, count), values: make([]float32, count*components), components: components}
	for index := 0; index < count; index++ {
		track.times[index] = float32(binary.LittleEndian.Uint16(data[timeOffset+index*2:])) / 32767
		for component := 0; component < components; component++ {
			at := valueOffset + (index*components+component)*componentSize
			if componentSize == 2 {
				value := float32(binary.LittleEndian.Uint16(data[at:]))
				if fixed16 {
					value /= 32767
				}
				track.values[index*components+component] = value
			} else {
				track.values[index*components+component] = readF32(data, at)
			}
		}
	}
	return track
}

func (track m2ParticleTrack) value(age float32, component int, fallback float32) float32 {
	if component < 0 || component >= track.components || len(track.times) == 0 || len(track.values) < track.components {
		return fallback
	}
	if len(track.times) == 1 || len(track.values) < len(track.times)*track.components {
		return track.values[component]
	}
	next := 0
	for next < len(track.times) && track.times[next] <= age {
		next++
	}
	if next == 0 {
		return track.values[component]
	}
	if next >= len(track.times) {
		return track.values[(len(track.times)-1)*track.components+component]
	}
	left := next - 1
	right := next
	start, end := track.times[left], track.times[right]
	if end <= start {
		return track.values[left*track.components+component]
	}
	fraction := (age - start) / (end - start)
	v0 := track.values[left*track.components+component]
	v1 := track.values[right*track.components+component]
	return v0 + (v1-v0)*fraction
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

func buildM2ParticleSystem(loader *ui.Loader, model *parsedM2, root *core.Node, rootScale float32, textures map[string]*texture.Texture2D) *m2ParticleSystem {
	if model == nil || len(model.particles) == 0 {
		return nil
	}
	right, up := particleBasis(*model)
	particleTextures := make(map[particleTextureKey]*texture.Texture2D)
	system := &m2ParticleSystem{}
	for index, emitter := range model.particles {
		maxRate := m2ParticleMaxRate(model, emitter)
		if maxRate <= 0 || emitter.life <= 0 || int(emitter.texture) >= len(model.textures) {
			continue
		}
		path := model.textures[emitter.texture]
		if path == "" {
			continue
		}
		key := particleTextureKey{path: path, rows: emitter.rows, cols: emitter.cols}
		tex := particleTextures[key]
		if tex == nil {
			// Always load a particle-owned texture so SetRepeat/wrap cannot
			// mutate mesh materials that share the same BLP path.
			tex = loadModelTexture(loader, path)
			if tex == nil {
				continue
			}
			if emitter.rows > 1 || emitter.cols > 1 {
				tex.SetWrapS(gls.REPEAT)
				tex.SetWrapT(gls.REPEAT)
			} else {
				tex.SetWrapS(gls.CLAMP_TO_EDGE)
				tex.SetWrapT(gls.CLAMP_TO_EDGE)
			}
			tex.SetRepeat(1/float32(emitter.cols), 1/float32(emitter.rows))
			tex.SetOffset(0, 0)
			particleTextures[key] = tex
		}
		count := int(math.Ceil(float64(maxRate * emitter.life)))
		if count < 1 {
			count = 1
		}
		if count > 64 {
			count = 64
		}
		base := modelVector(transformM2Point(int(emitter.bone), emitter.position, model.bones, 0))
		activeCount := m2ParticleCount(m2ParticleEmissionRate(model, emitter), emitter.life, count)
		vertexCount := count * 4
		group := &m2ParticleGroup{emitter: emitter, model: model, bone: int(emitter.bone), base: base, right: right, up: up, particleTex: tex, rootScale: rootScale, activeCount: activeCount, particles: make([]m2Particle, count), positions: math32.NewArrayF32(0, vertexCount*3), colors: math32.NewArrayF32(0, vertexCount*3), params: math32.NewArrayF32(0, vertexCount*4), alphas: math32.NewArrayF32(0, vertexCount), rotations: math32.NewArrayF32(0, vertexCount), corners: math32.NewArrayF32(0, vertexCount*2)}
		for particleIndex := range group.particles {
			seed := uint32(index+1)*0x9e3779b9 + uint32(particleIndex+1)*0x85ebca6b
			age := float32(0)
			if particleIndex < activeCount {
				age = emitter.life * float32(particleIndex) / float32(activeCount)
			}
			group.particles[particleIndex] = spawnM2Particle(group, seed, age)
			position := particlePosition(group.particles[particleIndex])
			color, size, alpha, cell := particleAppearance(group.emitter, group.particles[particleIndex])
			modelColor, modelAlpha := m2ParticleColor(model, emitter.colorIndex)
			color[0] *= modelColor[0]
			color[1] *= modelColor[1]
			color[2] *= modelColor[2]
			alpha *= modelAlpha
			if particleIndex >= activeCount {
				alpha = 0
			}
			for _, corner := range particleCorners {
				group.positions.Append(position[0], position[1], position[2])
				group.colors.Append(color[0], color[1], color[2])
				group.params.Append(size[0]*0.5, size[1]*0.5, cell[0], cell[1])
				group.alphas.Append(alpha)
				group.rotations.Append(group.particles[particleIndex].rotation)
				group.corners.Append(corner[0], corner[1])
			}
		}
		geom := geometry.NewGeometry()
		indices := math32.NewArrayU32(0, count*6)
		for particleIndex := 0; particleIndex < count; particleIndex++ {
			baseIndex := uint32(particleIndex * 4)
			indices.Append(baseIndex, baseIndex+1, baseIndex+2, baseIndex, baseIndex+2, baseIndex+3)
		}
		geom.SetIndices(indices)
		geom.AddVBO(gls.NewVBO(group.positions).AddAttrib(gls.VertexPosition))
		group.colorVBO = gls.NewVBO(group.colors).AddAttrib(gls.VertexColor)
		group.paramsVBO = gls.NewVBO(group.params).AddCustomAttrib("VertexParticleParams", 4)
		group.alphaVBO = gls.NewVBO(group.alphas).AddCustomAttrib("VertexParticleAlpha", 1)
		group.rotationVBO = gls.NewVBO(group.rotations).AddCustomAttrib("VertexParticleRotation", 1)
		group.cornerVBO = gls.NewVBO(group.corners).AddCustomAttrib("VertexParticleCorner", 2)
		geom.AddVBO(group.colorVBO)
		geom.AddVBO(group.paramsVBO)
		geom.AddVBO(group.alphaVBO)
		geom.AddVBO(group.rotationVBO)
		geom.AddVBO(group.cornerVBO)
		mat := material.NewStandard(&math32.Color{R: 1, G: 1, B: 1})
		mat.SetShader("morenowow_particle")
		mat.SetShaderUnique(true)
		mat.SetEmissiveColor(&math32.Color{R: 1, G: 1, B: 1})
		mat.SetOpacity(1)
		mat.SetSide(material.SideDouble)
		mat.SetUseLights(material.UseLightNone)
		mat.SetDepthTest(true)
		mat.SetDepthMask(false)
		mat.SetShaderUnique(true)
		blending := m2ParticleBlending(emitter.blend)
		mat.SetTransparent(blending != material.BlendNone)
		mat.SetBlending(blending)
		mat.AddTexture(tex)
		mesh := graphic.NewMesh(geom, mat)
		mesh.SetRenderOrder(10)
		mesh.SetCullable(false)
		group.positionVBO = geom.VBO(gls.VertexPosition)
		group.mesh = mesh
		root.Add(mesh)
		system.groups = append(system.groups, group)
		system.emitterCount++
		system.pointCount += count
	}
	if len(system.groups) == 0 {
		return nil
	}
	return system
}

func m2ParticleBlending(raw uint8) material.Blending {
	// WotLK particle blendingType maps through s_gxBlend to EGxBlend:
	// 0 Opaque, 1 AlphaKey, 2 Alpha, 3 NoAlphaAdd, 4 Add, 5 Mod, 6 Mod2x.
	// g3n BlendAdditive is SRC_ALPHA,ONE (GxBlend_Add). NoAlphaAdd (ONE,ONE)
	// is approximated the same way; soft RGB falloff still modulates intensity
	// when texture alpha is opaque.
	switch raw {
	case 0, 1:
		return material.BlendNone
	case 3, 4:
		return material.BlendAdditive
	case 5, 6:
		return material.BlendMultiply
	default:
		return material.BlendNormal
	}
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
	polar := random() * emitter.verticalRange
	azimuth := randomSigned() * emitter.horizontalRange * 0.5
	sine := float32(math.Sin(float64(polar)))
	direction := [3]float32{float32(math.Cos(float64(azimuth))) * sine, float32(math.Sin(float64(azimuth))) * sine, float32(math.Cos(float64(polar)))}
	if emitter.emitterType == 2 {
		radius := emitter.areaLength + (emitter.areaWidth-emitter.areaLength)*random()
		offset := scaleParticleVector(direction, radius)
		offset = modelVector(transformM2Direction(group.bone, offset, group.model.bones, 0))
		return newM2Particle(group, seed, age, offset, direction, random)
	}
	offset := [3]float32{randomSigned() * emitter.areaLength * 0.5, randomSigned() * emitter.areaWidth * 0.5, 0}
	offset = modelVector(transformM2Direction(group.bone, offset, group.model.bones, 0))
	return newM2Particle(group, seed, age, offset, direction, random)
}

func newM2Particle(group *m2ParticleGroup, seed uint32, age float32, offset, direction [3]float32, random func() float32) m2Particle {
	emitter := group.emitter
	life := emitter.life + emitter.lifespanVary*(random()*2-1)
	if life <= 0 {
		life = emitter.life
	}
	speed := emitter.speed * (1 + emitter.speedVariation*(random()*2-1))
	velocity := scaleParticleVector(modelVector(transformM2Direction(group.bone, direction, group.model.bones, 0)), speed)
	gravity := modelVector(transformM2Direction(group.bone, [3]float32{0, 0, -emitter.gravity}, group.model.bones, 0))
	drag := emitter.drag
	if math.Abs(float64(speed)) < 0.01 {
		drift := [3]float32{random()*2 - 1, random()*2 - 1, -random() * 0.5}
		velocity = modelVector(transformM2Direction(group.bone, drift, group.model.bones, 0))
		if particleVectorLength(gravity) < 0.001 {
			gravity = modelVector(transformM2Direction(group.bone, [3]float32{0, 0, -1.5}, group.model.bones, 0))
		}
		drag = 0
	}
	rotation := float32(emitter.textureRotation)*math.Pi/180 + random()*2*math.Pi
	spin := emitter.baseSpin + emitter.baseSpinVary*(random()*2-1) + emitter.spin + emitter.spinVary*(random()*2-1)
	return m2Particle{origin: addParticleVector(group.base, offset), velocity: velocity, gravity: gravity, age: age, life: life, seed: seed, rotation: rotation, spin: spin, drag: drag}
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
		if group.model != nil {
			group.base = modelVector(transformM2Point(group.bone, group.emitter.position, group.model.bones, 0))
		}
		activeCount := m2ParticleCount(m2ParticleEmissionRate(group.model, group.emitter), group.emitter.life, len(group.particles))
		if activeCount > group.activeCount {
			for index := group.activeCount; index < activeCount; index++ {
				seed := uint32(index+1)*0x85ebca6b + uint32(len(group.particles))*0x9e3779b9
				group.particles[index] = spawnM2Particle(group, seed, group.emitter.life*float32(index)/float32(activeCount))
			}
		}
		group.activeCount = activeCount
		for index := range group.particles {
			if index >= activeCount {
				for vertex := 0; vertex < 4; vertex++ {
					group.alphas.Set(index*4+vertex, 0)
				}
				continue
			}
			particle := &group.particles[index]
			particle.age += delta
			particle.rotation += particle.spin * delta
			if particle.drag > 0 {
				particle.velocity = scaleParticleVector(particle.velocity, float32(math.Max(0, 1-float64(particle.drag*delta))))
			}
			if particle.age >= particle.life {
				particle.seed += 0x9e3779b9
				*particle = spawnM2Particle(group, particle.seed, 0)
			}
			position := particlePosition(*particle)
			color, size, alpha, cell := particleAppearance(group.emitter, *particle)
			modelColor, modelAlpha := m2ParticleColor(group.model, group.emitter.colorIndex)
			color[0] *= modelColor[0]
			color[1] *= modelColor[1]
			color[2] *= modelColor[2]
			alpha *= modelAlpha
			for vertex := 0; vertex < 4; vertex++ {
				positionIndex := (index*4 + vertex) * 3
				group.positions.Set(positionIndex, position[0], position[1], position[2])
				group.colors.Set(positionIndex, color[0], color[1], color[2])
				paramsIndex := (index*4 + vertex) * 4
				group.params.Set(paramsIndex, size[0]*0.5, size[1]*0.5, cell[0], cell[1])
				group.alphas.Set(index*4+vertex, alpha)
				group.rotations.Set(index*4+vertex, particle.rotation)
			}
		}
		if group.positionVBO != nil {
			group.positionVBO.SetBuffer(group.positions)
		}
		if group.colorVBO != nil {
			group.colorVBO.SetBuffer(group.colors)
		}
		if group.paramsVBO != nil {
			group.paramsVBO.SetBuffer(group.params)
		}
		if group.alphaVBO != nil {
			group.alphaVBO.SetBuffer(group.alphas)
		}
		if group.rotationVBO != nil {
			group.rotationVBO.SetBuffer(group.rotations)
		}
		// Corner attributes are static billboard offsets; re-uploading them
		// every frame forced a useless GPU BufferData for every emitter.
	}
}

func m2ParticleMaxRate(model *parsedM2, emitter m2ParticleEmitter) float32 {
	maxRate := emitter.rate
	for _, sequence := range emitter.rateTrack.sequences {
		for _, value := range sequence.values {
			if value > maxRate {
				maxRate = value
			}
		}
	}
	if maxRate < 0 || math.IsNaN(float64(maxRate)) || math.IsInf(float64(maxRate), 0) {
		return 0
	}
	return maxRate
}

func m2ParticleEmissionRate(model *parsedM2, emitter m2ParticleEmitter) float32 {
	rate := emitter.rate
	if model != nil && trackScalarHasKeys(emitter.rateTrack) {
		rate = emitter.rateTrack.value(model.animationSequence, m2TrackTime(emitter.rateTrack.globalSequence, model.animationTime, model.animationGlobalTime), model.globalLoops, rate)
	}
	if rate < 0 || math.IsNaN(float64(rate)) || math.IsInf(float64(rate), 0) {
		return 0
	}
	return rate
}

func m2ParticleCount(rate, life float32, capacity int) int {
	if rate <= 0 || life <= 0 || capacity <= 0 {
		return 0
	}
	count := int(math.Ceil(float64(rate * life)))
	if count < 1 {
		count = 1
	}
	if count > capacity {
		count = capacity
	}
	return count
}

func m2ParticleColor(model *parsedM2, index uint16) ([3]float32, float32) {
	color := [3]float32{1, 1, 1}
	alpha := float32(1)
	if model == nil || index == 0xffff || int(index) >= len(model.colors) {
		return color, alpha
	}
	entry := model.colors[index]
	color = entry.current
	if entry.currentAlpha > 0.001 {
		alpha = entry.currentAlpha
	}
	return color, alpha
}

var particleCorners = [][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}}

func particleAppearance(emitter m2ParticleEmitter, particle m2Particle) ([3]float32, [2]float32, float32, [2]float32) {
	age := float32(0)
	if particle.life > 0 {
		age = particle.age / particle.life
	}
	if age < 0 {
		age = 0
	}
	if age > 1 {
		age = 1
	}
	color := [3]float32{emitter.color[0], emitter.color[1], emitter.color[2]}
	for component := range color {
		color[component] = emitter.colorTrack.value(age, component, color[component])
		color[component] = clampParticleValue(color[component]/255, 0, 1)
	}
	alpha := clampParticleValue(emitter.alphaTrack.value(age, 0, emitter.alpha), 0, 1)
	sizeX := finiteParticleValue(emitter.scaleTrack.value(age, 0, emitter.scale[0]))
	sizeY := finiteParticleValue(emitter.scaleTrack.value(age, 1, emitter.scale[1]))
	if sizeX <= 0 {
		sizeX = emitter.scale[0]
	}
	if sizeY <= 0 {
		sizeY = emitter.scale[1]
	}
	cell := int(math.Round(float64(emitter.headCellTrack.value(age, 0, 0))))
	cells := int(emitter.rows) * int(emitter.cols)
	if cells < 1 {
		cells = 1
	}
	cell %= cells
	if cell < 0 {
		cell += cells
	}
	columns := int(emitter.cols)
	if columns < 1 {
		columns = 1
	}
	return color, [2]float32{sizeX, sizeY}, alpha, [2]float32{float32(cell % columns), float32(cell / columns)}
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
