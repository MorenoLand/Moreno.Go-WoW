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

	ambient, diffuse, direction, fogColor, fogParams gls.Uniform
	waterShallow, waterDeep, waterAlpha, skyColors   gls.Uniform
}

func newWorldMaterial() *worldMaterial {
	m := &worldMaterial{Standard: material.NewStandard(&math32.Color{R: 1, G: 1, B: 1}), water: -1}
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

// RenderSetup sets the standard material state and then the atmosphere uniforms.
func (m *worldMaterial) RenderSetup(gs *gls.GLS) {
	m.Standard.RenderSetup(gs)
	a := &worldAtmo
	enabled := float32(0)
	if a.valid {
		enabled = 1
	}
	lit := float32(0)
	if a.valid && m.lit && !m.noFog {
		lit = 1
	}
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
