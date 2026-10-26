package render

import (
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
)

// worldMaterial is a standard material that also feeds the frame's atmosphere
// (sun-lit surface light, fog, water colours, sky gradient) to the world
// shaders each time it is drawn.
type worldMaterial struct {
	*material.Standard
	// noFog leaves the surface unfogged: additive blends would brighten.
	noFog bool
	// water selects the river (0) or ocean (1) colours; -1 for other surfaces.
	water int
	// sky draws the sky gradient.
	sky bool
	// lit lights the surface with the frame's ambient and directional light.
	lit bool

	// local lights the surface from inside a WMO instead of by the sun and sky.
	local *worldLocalLight

	// wmoMode is how a map object surface is lit (wmoUnlit, wmoExterior,
	// wmoInterior) and instance the WMO placement it belongs to.
	wmoMode float32
	// envMask has a bit for each texture of an M2 surface drawn with the
	// reflected environment coordinates.
	envMask        float32
	envMaskUniform gls.Uniform
	// highlight is the colour a model under the mouse is lit up by.
	highlight         [3]float32
	highlightUniform  gls.Uniform
	instance          int
	wmoParams, wmoAdd gls.Uniform

	indoorAmbient, pointPositions, pointColours      gls.Uniform
	ambient, diffuse, direction, fogColor, fogParams gls.Uniform
	waterShallow, waterDeep, waterAlpha, skyColors   gls.Uniform
}

func newWorldMaterial() *worldMaterial {
	m := &worldMaterial{Standard: material.NewStandard(&math32.Color{R: 1, G: 1, B: 1}), water: -1}
	m.envMaskUniform.Init("M2EnvMask")
	m.highlightUniform.Init("M2Highlight")
	m.wmoParams.Init("WmoParams")
	m.wmoAdd.Init("WmoAdd")
	m.indoorAmbient.Init("IndoorAmbient")
	m.pointPositions.Init("PointLightPos")
	m.pointColours.Init("PointLightColor")
	m.ambient.Init("LightAmbient")
	m.diffuse.Init("LightDiffuse")
	m.direction.Init("LightDirection")
	m.fogColor.Init("FogColor")
	m.fogParams.Init("FogParams")
	m.waterShallow.Init("WaterShallow")
	m.waterDeep.Init("WaterDeep")
	m.waterAlpha.Init("WaterAlpha")
	m.skyColors.Init("SkyColors")
	return m
}

// worldProgramKey identifies the shader program a material draws with.
type worldProgramKey struct {
	shader   string
	textures int
}

// worldProgramUniforms is what the plain atmosphere uniforms of a program were
// last set to; a program keeps its uniforms, so a draw that would set the same
// values again skips the calls.
type worldProgramUniforms struct {
	set                              bool
	ambient, diffuse, direction, fog [3]float32
	fogStart, fogEnd                 float32
	valid, lit, noFog                bool
	wmoMode                          float32
	envMask                          float32
	wmoAdd                           [3]float32
	highlight                        [3]float32
}

// Wow.exe's three ways of lighting a map object surface (FUN_007a8b10).
const (
	wmoUnlit    = 0
	wmoExterior = 1
	// wmoExteriorUnified is an exterior surface of a map object whose header says
	// it uses the unified path (MOHD flag 0x2): the MapObjU programs add the
	// baked colour to half the light instead of multiplying it.
	wmoExteriorUnified = 2
	wmoInterior        = 3
)

// worldInsideInstance is the WMO placement the player stands in.
var worldInsideInstance int

// wmoAmbientAdd is what the light's ambient adds to the surfaces of the map
// object the player stands in (the engine sets the vertex program's constant
// 29 from it, halved, and the pixel stage doubles the result).
func (m *worldMaterial) wmoAmbientAdd() [3]float32 {
	if m.wmoMode == wmoUnlit || m.instance == 0 || m.instance != worldInsideInstance || !worldAtmo.valid {
		return [3]float32{}
	}
	// Interior surfaces take half of it: their baked colour carries more of the
	// light than a street's does.
	share := float32(0.5)
	if m.wmoMode == wmoInterior {
		share = 0.25
	}
	var add [3]float32
	for index := range add {
		value := worldAtmo.ambient[index]
		if value > 1 {
			value = 1
		}
		add[index] = value * share
	}
	return add
}

var worldUniformCache = make(map[worldProgramKey]*worldProgramUniforms)

// RenderSetup sets the standard material state and then the atmosphere uniforms.
func (m *worldMaterial) RenderSetup(gs *gls.GLS) {
	m.Standard.RenderSetup(gs)
	a := &worldAtmo
	key := worldProgramKey{m.Shader(), m.TextureCount()}
	cached := worldUniformCache[key]
	if cached == nil {
		cached = &worldProgramUniforms{}
		worldUniformCache[key] = cached
	}
	if m.local == nil && m.water < 0 && !m.sky {
		current := worldProgramUniforms{set: true, ambient: a.ambient, diffuse: a.diffuse, direction: a.direction, fog: a.fog, fogStart: a.fogStart, fogEnd: a.fogEnd, valid: a.valid, lit: m.lit, noFog: m.noFog, wmoMode: m.wmoMode, envMask: m.envMask, wmoAdd: m.wmoAmbientAdd(), highlight: m.highlight}
		if *cached == current {
			return
		}
		*cached = current
	} else {
		cached.set = false
	}
	enabled := float32(0)
	if a.valid {
		enabled = 1
	}
	lit := float32(0)
	if a.valid && m.lit && !m.noFog {
		lit = 1
	}
	var positions, colours [worldLocalLightCount * 4]float32
	indoor := float32(0)
	var indoorAmbient [3]float32
	if m.local != nil && lit > 0 {
		indoor, indoorAmbient = 1, m.local.ambient
		if m.local.exterior {
			indoor = 2
		}
		for index := 0; index < m.local.count; index++ {
			light := m.local.lights[index]
			copy(positions[index*4:], []float32{light.position[0], light.position[1], light.position[2], light.end})
			copy(colours[index*4:], []float32{light.colour[0], light.colour[1], light.colour[2], light.start})
		}
	}
	add := m.wmoAmbientAdd()
	gs.Uniform1f(m.envMaskUniform.Location(gs), m.envMask)
	gs.Uniform3f(m.highlightUniform.Location(gs), m.highlight[0], m.highlight[1], m.highlight[2])
	gs.Uniform4f(m.wmoParams.Location(gs), m.wmoMode, 0, 0, 0)
	gs.Uniform3f(m.wmoAdd.Location(gs), add[0], add[1], add[2])
	gs.Uniform4f(m.indoorAmbient.Location(gs), indoorAmbient[0], indoorAmbient[1], indoorAmbient[2], indoor)
	gs.Uniform4fv(m.pointPositions.Location(gs), worldLocalLightCount, &positions[0])
	gs.Uniform4fv(m.pointColours.Location(gs), worldLocalLightCount, &colours[0])
	gs.Uniform4f(m.ambient.Location(gs), a.ambient[0], a.ambient[1], a.ambient[2], lit)
	gs.Uniform3f(m.diffuse.Location(gs), a.diffuse[0], a.diffuse[1], a.diffuse[2])
	gs.Uniform3f(m.direction.Location(gs), a.direction[0], a.direction[1], a.direction[2])
	gs.Uniform3f(m.fogColor.Location(gs), a.fog[0], a.fog[1], a.fog[2])
	fogStart, fogEnd := a.fogStart, a.fogEnd
	if !a.valid || m.noFog {
		fogStart, fogEnd = 0, 0
	}
	gs.Uniform4f(m.fogParams.Location(gs), fogStart, fogEnd, 0, 0)
	if m.water >= 0 {
		shallow, deep, alpha := a.waterShallow[m.water], a.waterDeep[m.water], a.waterAlpha[m.water]
		gs.Uniform3f(m.waterShallow.Location(gs), shallow[0], shallow[1], shallow[2])
		gs.Uniform3f(m.waterDeep.Location(gs), deep[0], deep[1], deep[2])
		gs.Uniform4f(m.waterAlpha.Location(gs), alpha[0], alpha[1], enabled, 0)
	}
	if m.sky {
		colours := make([]float32, 0, 18)
		for _, colour := range a.sky {
			colours = append(colours, colour[0], colour[1], colour[2])
		}
		gs.Uniform3fv(m.skyColors.Location(gs), 6, &append(colours, a.fog[0], a.fog[1], a.fog[2])[0])
	}
}

// Shared GLSL for the fog every world shader applies: the blend runs from
// FogParams.x to FogParams.y by distance from the eye, towards the fog colour.
// An unset FogParams (y <= x) draws unfogged.
const (
	worldFogVertexDeclarations = "uniform mat4 ModelViewMatrix;\nuniform vec4 FogParams;\nout float FragFog;\n"
	worldFogVertexBody         = "    float fogDistance = length((ModelViewMatrix * vec4(VertexPosition, 1.0)).xyz);\n    FragFog = FogParams.y > FogParams.x ? clamp((fogDistance - FogParams.x) / (FogParams.y - FogParams.x), 0.0, 1.0) : 0.0;\n"
	worldFogFragmentDecl       = "uniform vec3 FogColor;\nin float FragFog;\n"
	worldFogApply              = "    FragColor.rgb = mix(FragColor.rgb, FogColor, FragFog);\n"
)

// worldFoggedBlending reports whether a blend mode can be fogged towards the
// fog colour: additive and multiplicative surfaces would brighten or darken
// wrongly.
func worldFoggedBlending(blending material.Blending) bool {
	return blending == material.BlendNone || blending == material.BlendNormal
}
