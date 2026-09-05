package render

import "github.com/g3n/engine/renderer"

const m2VertexShader = `#include <attributes>
uniform mat4 MVP;
in vec2 VertexTexcoord2;
out vec2 FragTexcoord;
out vec2 FragTexcoord2;
void main() {
    FragTexcoord = VertexTexcoord;
    FragTexcoord2 = VertexTexcoord2;
    gl_Position = MVP * vec4(VertexPosition, 1.0);
}`

const m2FragmentShader = `precision highp float;
#if MAT_TEXTURES > 0
uniform sampler2D MatTexture[MAT_TEXTURES];
#endif
in vec2 FragTexcoord;
in vec2 FragTexcoord2;
out vec4 FragColor;
void main() {
    vec4 result = vec4(1.0);
#if MAT_TEXTURES > 0
    result = texture(MatTexture[0], FragTexcoord);
#if MAT_TEXTURES > 1
    for (int index = 1; index < MAT_TEXTURES; index++) {
        vec2 coord = FragTexcoord;
        if (index == 1) {
            coord = FragTexcoord2;
        }
        vec4 layer = texture(MatTexture[index], coord);
        result.rgb = layer.rgb * layer.a + result.rgb * (1.0 - layer.a);
        result.a = layer.a + result.a * (1.0 - layer.a);
    }
#endif
#endif
    FragColor = result;
}`

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
    vec2 sprite = ParticleCorner * 0.5 + vec2(0.5);
    vec2 coord = sprite * repeat + ParticleCell * repeat + MatTexOffset(0);
    result = texture(MatTexture[0], coord);
#endif
    FragColor = vec4(result.rgb * ParticleColor, result.a * ParticleAlpha);
}`

const m2AlphaKeyFragmentShader = `precision highp float;
#if MAT_TEXTURES > 0
uniform sampler2D MatTexture[MAT_TEXTURES];
#endif
in vec2 FragTexcoord;
in vec2 FragTexcoord2;
out vec4 FragColor;
void main() {
    vec4 result = vec4(1.0);
#if MAT_TEXTURES > 0
    result = texture(MatTexture[0], FragTexcoord);
#if MAT_TEXTURES > 1
    for (int index = 1; index < MAT_TEXTURES; index++) {
        vec2 coord = FragTexcoord;
        if (index == 1) {
            coord = FragTexcoord2;
        }
        vec4 layer = texture(MatTexture[index], coord);
        result.rgb = layer.rgb * layer.a + result.rgb * (1.0 - layer.a);
        result.a = layer.a + result.a * (1.0 - layer.a);
    }
#endif
#endif
    if (result.a < 0.5) {
        discard;
    }
    FragColor = vec4(result.rgb, 1.0);
}`

const worldTerrainVertexShader = `#include <attributes>
uniform mat4 MVP;
out vec2 FragTexcoord;
void main() {
    FragTexcoord = VertexTexcoord;
    gl_Position = MVP * vec4(VertexPosition, 1.0);
}`

const worldTerrainFragmentShader = `precision highp float;
uniform sampler2D MatTexture[MAT_TEXTURES];
in vec2 FragTexcoord;
out vec4 FragColor;
void main() {
    vec2 tiled = FragTexcoord * 8.0;
    vec4 color = texture(MatTexture[0], tiled);
#if MAT_TEXTURES > 4
    color.rgb = mix(color.rgb, texture(MatTexture[1], tiled).rgb, texture(MatTexture[4], FragTexcoord).r);
#endif
#if MAT_TEXTURES > 5
    color.rgb = mix(color.rgb, texture(MatTexture[2], tiled).rgb, texture(MatTexture[5], FragTexcoord).r);
#endif
#if MAT_TEXTURES > 6
    color.rgb = mix(color.rgb, texture(MatTexture[3], tiled).rgb, texture(MatTexture[6], FragTexcoord).r);
#endif
    FragColor = vec4(color.rgb, 1.0);
}`

const worldTerrainAlphaKeyFragmentShader = `precision highp float;
uniform sampler2D MatTexture[MAT_TEXTURES];
in vec2 FragTexcoord;
out vec4 FragColor;
void main() {
    vec4 color = texture(MatTexture[0], FragTexcoord);
    if (color.a < 0.5) {
        discard;
    }
    FragColor = vec4(color.rgb, 1.0);
}`

const worldWMOVertexShader = `#include <attributes>
uniform mat4 MVP;
out vec2 FragTexcoord;
out vec3 FragVertexColor;
void main() {
    FragTexcoord = VertexTexcoord;
    FragVertexColor = VertexColor;
    gl_Position = MVP * vec4(VertexPosition, 1.0);
}`

const worldWMOFragmentShader = `precision highp float;
uniform sampler2D MatTexture[MAT_TEXTURES];
in vec2 FragTexcoord;
in vec3 FragVertexColor;
out vec4 FragColor;
void main() {
    vec4 color = texture(MatTexture[0], FragTexcoord);
    FragColor = vec4(color.rgb * FragVertexColor, color.a);
}`

const worldWMOAlphaKeyFragmentShader = `precision highp float;
uniform sampler2D MatTexture[MAT_TEXTURES];
in vec2 FragTexcoord;
in vec3 FragVertexColor;
out vec4 FragColor;
void main() {
    vec4 color = texture(MatTexture[0], FragTexcoord);
    if (color.a < 0.5) {
        discard;
    }
    FragColor = vec4(color.rgb * FragVertexColor, 1.0);
}`

const worldSkyVertexShader = `#include <attributes>
uniform mat4 ModelMatrix;
uniform mat4 MVP;
out float FragSkyHeight;
void main() {
    FragSkyHeight = normalize(mat3(ModelMatrix) * VertexNormal).z;
    gl_Position = MVP * vec4(VertexPosition, 1.0);
}`

const worldSkyFragmentShader = `precision highp float;
in float FragSkyHeight;
out vec4 FragColor;
void main() {
    float height = smoothstep(-0.08, 0.72, FragSkyHeight);
    vec3 horizon = vec3(0.52, 0.62, 0.72);
    vec3 zenith = vec3(0.20, 0.34, 0.60);
    FragColor = vec4(mix(horizon, zenith, height), 1.0);
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
	r.Shaman.AddShader("morenowow_world_sky_fragment", worldSkyFragmentShader)
	r.Shaman.AddProgram("morenowow_m2", "morenowow_m2_vertex", "morenowow_m2_fragment")
	r.Shaman.AddProgram("morenowow_particle", "morenowow_particle_vertex", "morenowow_particle_fragment")
	r.Shaman.AddProgram("morenowow_m2_alpha_key", "morenowow_m2_vertex", "morenowow_m2_alpha_key_fragment")
	r.Shaman.AddProgram("morenowow_world_terrain", "morenowow_world_terrain_vertex", "morenowow_world_terrain_fragment")
	r.Shaman.AddProgram("morenowow_world_terrain_alpha_key", "morenowow_world_terrain_vertex", "morenowow_world_terrain_alpha_key_fragment")
	r.Shaman.AddProgram("morenowow_world_wmo", "morenowow_world_wmo_vertex", "morenowow_world_wmo_fragment")
	r.Shaman.AddProgram("morenowow_world_wmo_alpha_key", "morenowow_world_wmo_vertex", "morenowow_world_wmo_alpha_key_fragment")
	r.Shaman.AddProgram("morenowow_world_sky", "morenowow_world_sky_vertex", "morenowow_world_sky_fragment")
}
