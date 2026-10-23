package render

import "github.com/g3n/engine/renderer"

const m2VertexShader = `#include <attributes>
uniform mat4 MVP;
uniform mat4 ModelMatrix;
` + worldFogVertexDeclarations + `in vec2 VertexTexcoord2;
out vec3 FragNormal;
in float VertexM2Alpha;
in vec2 VertexM2Combiner;
out vec2 FragTexcoord;
out vec2 FragTexcoord2;
out vec3 FragVertexColor;
out float FragM2Alpha;
out vec2 FragM2Combiner;
void main() {
    FragTexcoord = VertexTexcoord;
    FragTexcoord2 = VertexTexcoord2;
    FragVertexColor = VertexColor;
    FragM2Alpha = VertexM2Alpha;
    FragM2Combiner = VertexM2Combiner;
    FragNormal = mat3(ModelMatrix) * VertexNormal;
` + worldFogVertexBody + `    gl_Position = MVP * vec4(VertexPosition, 1.0);
}`

const m2FragmentShader = `precision highp float;
#include <material>
in vec2 FragTexcoord;
in vec2 FragTexcoord2;
in vec3 FragVertexColor;
in float FragM2Alpha;
in vec2 FragM2Combiner;
in vec3 FragNormal;
uniform vec4 LightAmbient;
uniform vec3 LightDiffuse;
uniform vec3 LightDirection;
` + worldFogFragmentDecl + `out vec4 FragColor;
vec3 m2Light() {
    if (LightAmbient.w < 0.5) {
        return vec3(1.0);
    }
    vec3 n = normalize(FragNormal);
    if (!gl_FrontFacing) {
        n = -n;
    }
    return clamp(LightAmbient.rgb + LightDiffuse * max(dot(n, LightDirection), 0.0), 0.0, 1.0);
}
vec4 m2Combine(vec4 base, vec4 layer, float operation) {
    int op = int(operation + 0.5);
    if (op == 0) {
        return layer;
    }
    if (op == 1) {
        return vec4(base.rgb * layer.rgb, base.a * layer.a);
    }
    if (op == 2) {
        return vec4(mix(base.rgb, layer.rgb, layer.a), base.a);
    }
    if (op == 3) {
        return vec4(base.rgb + layer.rgb, base.a);
    }
    if (op == 4) {
        return vec4(base.rgb * layer.rgb * 2.0, base.a * layer.a);
    }
    if (op == 5) {
        return vec4(mix(base.rgb, layer.rgb, layer.a), layer.a);
    }
    if (op == 6) {
        return vec4(base.rgb * layer.rgb * 2.0, base.a);
    }
    if (op == 7) {
        return vec4(base.rgb + layer.rgb, base.a);
    }
    return vec4(base.rgb * layer.rgb, base.a * layer.a);
}
void main() {
    vec4 result = vec4(1.0);
#if MAT_TEXTURES > 0
    result = m2Combine(result, texture(MatTexture[0], FragTexcoord), FragM2Combiner.x);
#if MAT_TEXTURES > 1
    for (int index = 1; index < MAT_TEXTURES; index++) {
        vec2 coord = FragTexcoord;
        if (index == 1) {
            coord = FragTexcoord2;
        }
        vec4 layer = texture(MatTexture[index], coord);
        result = m2Combine(result, layer, index == 1 ? FragM2Combiner.y : float(1));
    }
#endif
#endif
    FragColor = vec4(result.rgb * FragVertexColor * m2Light(), result.a * FragM2Alpha * MatOpacity);
` + worldFogApply + `}`

const m2ParticleVertexShader = `#include <attributes>
uniform mat4 MVP;
uniform mat4 ModelViewMatrix;
in vec4 VertexParticleParams;
in float VertexParticleAlpha;
in float VertexParticleRotation;
in vec2 VertexParticleCorner;
out vec3 ParticleColor;
out float ParticleAlpha;
out vec2 ParticleCell;
out vec2 ParticleCorner;
void main() {
    float c = cos(VertexParticleRotation);
    float s = sin(VertexParticleRotation);
    vec2 turned = vec2(VertexParticleCorner.x * c - VertexParticleCorner.y * s, VertexParticleCorner.x * s + VertexParticleCorner.y * c);
    vec3 cameraRight = normalize(vec3(ModelViewMatrix[0][0], ModelViewMatrix[1][0], ModelViewMatrix[2][0]));
    vec3 cameraUp = normalize(vec3(ModelViewMatrix[0][1], ModelViewMatrix[1][1], ModelViewMatrix[2][1]));
    vec3 vertex = VertexPosition + cameraRight * (turned.x * VertexParticleParams.x) + cameraUp * (turned.y * VertexParticleParams.y);
    gl_Position = MVP * vec4(vertex, 1.0);
    ParticleColor = VertexColor;
    ParticleAlpha = VertexParticleAlpha;
    ParticleCell = VertexParticleParams.zw;
    ParticleCorner = VertexParticleCorner;
}`

const m2ParticleFragmentShader = `precision highp float;
#include <material>
in vec3 ParticleColor;
in float ParticleAlpha;
in vec2 ParticleCell;
in vec2 ParticleCorner;
out vec4 FragColor;
void main() {
    vec4 result = vec4(1.0);
#if MAT_TEXTURES > 0
    vec2 repeat = MatTexRepeat(0);
    vec2 sprite = vec2(ParticleCorner.x * 0.5 + 0.5, 0.5 - ParticleCorner.y * 0.5);
    vec2 coord = sprite * repeat + ParticleCell * repeat + MatTexOffset(0);
    result = texture(MatTexture[0], coord);
#endif
    FragColor = vec4(result.rgb * ParticleColor, result.a * ParticleAlpha * MatOpacity);
}`

const m2AlphaKeyFragmentShader = `precision highp float;
#include <material>
in vec2 FragTexcoord;
in vec2 FragTexcoord2;
in vec3 FragVertexColor;
in float FragM2Alpha;
in vec2 FragM2Combiner;
in vec3 FragNormal;
uniform vec4 LightAmbient;
uniform vec3 LightDiffuse;
uniform vec3 LightDirection;
` + worldFogFragmentDecl + `out vec4 FragColor;
vec3 m2Light() {
    if (LightAmbient.w < 0.5) {
        return vec3(1.0);
    }
    vec3 n = normalize(FragNormal);
    if (!gl_FrontFacing) {
        n = -n;
    }
    return clamp(LightAmbient.rgb + LightDiffuse * max(dot(n, LightDirection), 0.0), 0.0, 1.0);
}
vec4 m2Combine(vec4 base, vec4 layer, float operation) {
    int op = int(operation + 0.5);
    if (op == 0) {
        return layer;
    }
    if (op == 1) {
        return vec4(base.rgb * layer.rgb, base.a * layer.a);
    }
    if (op == 2) {
        return vec4(mix(base.rgb, layer.rgb, layer.a), base.a);
    }
    if (op == 3) {
        return vec4(base.rgb + layer.rgb, base.a);
    }
    if (op == 4) {
        return vec4(base.rgb * layer.rgb * 2.0, base.a * layer.a);
    }
    if (op == 5) {
        return vec4(mix(base.rgb, layer.rgb, layer.a), layer.a);
    }
    if (op == 6) {
        return vec4(base.rgb * layer.rgb * 2.0, base.a);
    }
    if (op == 7) {
        return vec4(base.rgb + layer.rgb, base.a);
    }
    return vec4(base.rgb * layer.rgb, base.a * layer.a);
}
void main() {
    vec4 result = vec4(1.0);
#if MAT_TEXTURES > 0
    result = m2Combine(result, texture(MatTexture[0], FragTexcoord), FragM2Combiner.x);
#if MAT_TEXTURES > 1
    for (int index = 1; index < MAT_TEXTURES; index++) {
        vec2 coord = FragTexcoord;
        if (index == 1) {
            coord = FragTexcoord2;
        }
        vec4 layer = texture(MatTexture[index], coord);
        result = m2Combine(result, layer, index == 1 ? FragM2Combiner.y : float(1));
    }
#endif
#endif
    if (result.a * FragM2Alpha * MatOpacity < 0.5) {
        discard;
    }
    FragColor = vec4(result.rgb * FragVertexColor * m2Light(), 1.0);
` + worldFogApply + `}`

const worldTerrainVertexShader = `#include <attributes>
uniform mat4 MVP;
` + worldFogVertexDeclarations + `out vec2 FragTexcoord;
out vec3 FragNormal;
void main() {
    FragTexcoord = VertexTexcoord;
    FragNormal = VertexNormal;
` + worldFogVertexBody + `    gl_Position = MVP * vec4(VertexPosition, 1.0);
}`

const worldTerrainFragmentShader = `precision highp float;
uniform sampler2D MatTexture[MAT_TEXTURES];
uniform vec4 LightAmbient;
uniform vec3 LightDiffuse;
uniform vec3 LightDirection;
` + worldFogFragmentDecl + `in vec2 FragTexcoord;
in vec3 FragNormal;
out vec4 FragColor;
void main() {
	vec2 tiled = FragTexcoord * 8.0;
	vec4 color = texture(MatTexture[0], tiled);
#if MAT_TEXTURES > 1
	color.rgb = mix(color.rgb, texture(MatTexture[1], tiled).rgb, texture(MatTexture[2], FragTexcoord).r);
#endif
#if MAT_TEXTURES > 3
	color.rgb = mix(color.rgb, texture(MatTexture[3], tiled).rgb, texture(MatTexture[4], FragTexcoord).r);
#endif
#if MAT_TEXTURES > 5
	color.rgb = mix(color.rgb, texture(MatTexture[5], tiled).rgb, texture(MatTexture[6], FragTexcoord).r);
#endif
    vec3 lit = color.rgb;
    if (LightAmbient.w > 0.5) {
        lit *= clamp(LightAmbient.rgb + LightDiffuse * max(dot(normalize(FragNormal), LightDirection), 0.0), 0.0, 1.0);
    }
    FragColor = vec4(lit, 1.0);
` + worldFogApply + `}`

const worldTerrainAlphaKeyFragmentShader = `precision highp float;
uniform sampler2D MatTexture[MAT_TEXTURES];
` + worldFogFragmentDecl + `in vec2 FragTexcoord;
out vec4 FragColor;
void main() {
    vec4 color = texture(MatTexture[0], FragTexcoord);
    if (color.a < 0.5) {
        discard;
    }
    FragColor = vec4(color.rgb, 1.0);
` + worldFogApply + `}`

const worldWMOVertexShader = `#include <attributes>
uniform mat4 MVP;
` + worldFogVertexDeclarations + `out vec2 FragTexcoord;
out vec3 FragVertexColor;
out vec3 FragNormal;
void main() {
    FragTexcoord = VertexTexcoord;
    FragVertexColor = VertexColor;
    FragNormal = VertexNormal;
` + worldFogVertexBody + `    gl_Position = MVP * vec4(VertexPosition, 1.0);
}`

// Outdoor groups are lit by the sun and sky (LightAmbient.w is set for them);
// indoor groups keep the colours baked into the model.
const worldWMOLight = `uniform vec4 LightAmbient;
uniform vec3 LightDiffuse;
uniform vec3 LightDirection;
in vec3 FragNormal;
vec3 wmoLight() {
    if (LightAmbient.w < 0.5) {
        return vec3(1.0);
    }
    vec3 n = normalize(FragNormal);
    if (!gl_FrontFacing) {
        n = -n;
    }
    return clamp(LightAmbient.rgb + LightDiffuse * max(dot(n, LightDirection), 0.0), 0.0, 1.0);
}
`

const worldWMOFragmentShader = `precision highp float;
uniform sampler2D MatTexture[MAT_TEXTURES];
` + worldWMOLight + worldFogFragmentDecl + `in vec2 FragTexcoord;
in vec3 FragVertexColor;
out vec4 FragColor;
void main() {
    vec4 color = texture(MatTexture[0], FragTexcoord);
    FragColor = vec4(color.rgb * FragVertexColor * wmoLight(), color.a);
` + worldFogApply + `}`

const worldWMOAlphaKeyFragmentShader = `precision highp float;
uniform sampler2D MatTexture[MAT_TEXTURES];
` + worldWMOLight + worldFogFragmentDecl + `in vec2 FragTexcoord;
in vec3 FragVertexColor;
out vec4 FragColor;
void main() {
    vec4 color = texture(MatTexture[0], FragTexcoord);
    if (color.a < 0.5) {
        discard;
    }
    FragColor = vec4(color.rgb * FragVertexColor * wmoLight(), 1.0);
` + worldFogApply + `}`

const worldSkyVertexShader = `#include <attributes>
uniform mat4 ModelMatrix;
uniform mat4 MVP;
out float FragSkyHeight;
void main() {
    FragSkyHeight = normalize(mat3(ModelMatrix) * VertexNormal).z;
    gl_Position = MVP * vec4(VertexPosition, 1.0);
}`

const worldSkyFragmentShader = `precision highp float;
uniform vec4 LightAmbient;
uniform vec3 SkyColors[6];
in float FragSkyHeight;
out vec4 FragColor;
void main() {
    if (LightAmbient.w < 0.5) {
        float height = smoothstep(-0.08, 0.72, FragSkyHeight);
        FragColor = vec4(mix(vec3(0.52, 0.62, 0.72), vec3(0.20, 0.34, 0.60), height), 1.0);
        return;
    }
    // SkyColors: top, middle, band 1, band 2, smog (horizon), then the fog colour.
    float e = FragSkyHeight;
    vec3 color = SkyColors[4];
    color = mix(color, SkyColors[3], smoothstep(0.0, 0.10, e));
    color = mix(color, SkyColors[2], smoothstep(0.10, 0.25, e));
    color = mix(color, SkyColors[1], smoothstep(0.25, 0.50, e));
    color = mix(color, SkyColors[0], smoothstep(0.50, 0.85, e));
    color = mix(SkyColors[5], color, smoothstep(-0.12, 0.0, e));
    FragColor = vec4(color, 1.0);
}`

func installM2Shaders(r *renderer.Renderer) {
	r.Shaman.AddShader("morenowow_m2_vertex", m2VertexShader)
	r.Shaman.AddShader("morenowow_m2_fragment", m2FragmentShader)
	r.Shaman.AddShader("morenowow_particle_vertex", m2ParticleVertexShader)
	r.Shaman.AddShader("morenowow_particle_fragment", m2ParticleFragmentShader)
	r.Shaman.AddShader("morenowow_m2_alpha_key_fragment", m2AlphaKeyFragmentShader)
	r.Shaman.AddShader("morenowow_world_terrain_vertex", worldTerrainVertexShader)
	r.Shaman.AddShader("morenowow_world_terrain_fragment", worldTerrainFragmentShader)
	r.Shaman.AddShader("morenowow_world_terrain_alpha_key_fragment", worldTerrainAlphaKeyFragmentShader)
	r.Shaman.AddShader("morenowow_world_wmo_vertex", worldWMOVertexShader)
	r.Shaman.AddShader("morenowow_world_wmo_fragment", worldWMOFragmentShader)
	r.Shaman.AddShader("morenowow_world_wmo_alpha_key_fragment", worldWMOAlphaKeyFragmentShader)
	r.Shaman.AddShader("morenowow_world_sky_vertex", worldSkyVertexShader)
	r.Shaman.AddShader("morenowow_world_liquid_vertex", worldLiquidVertexShader)
	r.Shaman.AddShader("morenowow_world_liquid_fragment", worldLiquidFragmentShader)
	r.Shaman.AddShader("morenowow_world_sky_fragment", worldSkyFragmentShader)
	r.Shaman.AddProgram("morenowow_m2", "morenowow_m2_vertex", "morenowow_m2_fragment")
	r.Shaman.AddProgram("morenowow_particle", "morenowow_particle_vertex", "morenowow_particle_fragment")
	r.Shaman.AddProgram("morenowow_m2_alpha_key", "morenowow_m2_vertex", "morenowow_m2_alpha_key_fragment")
	// World models use their own programs: uniforms belong to a program, so the
	// sun-lit, fogged variants must not share state with the menu scenes.
	r.Shaman.AddProgram("morenowow_world_m2", "morenowow_m2_vertex", "morenowow_m2_fragment")
	r.Shaman.AddProgram("morenowow_world_m2_alpha_key", "morenowow_m2_vertex", "morenowow_m2_alpha_key_fragment")
	r.Shaman.AddProgram("morenowow_world_terrain", "morenowow_world_terrain_vertex", "morenowow_world_terrain_fragment")
	r.Shaman.AddProgram("morenowow_world_terrain_alpha_key", "morenowow_world_terrain_vertex", "morenowow_world_terrain_alpha_key_fragment")
	r.Shaman.AddProgram("morenowow_world_wmo", "morenowow_world_wmo_vertex", "morenowow_world_wmo_fragment")
	r.Shaman.AddProgram("morenowow_world_wmo_alpha_key", "morenowow_world_wmo_vertex", "morenowow_world_wmo_alpha_key_fragment")
	r.Shaman.AddProgram("morenowow_world_sky", "morenowow_world_sky_vertex", "morenowow_world_sky_fragment")
	r.Shaman.AddProgram("morenowow_world_liquid", "morenowow_world_liquid_vertex", "morenowow_world_liquid_fragment")
}
