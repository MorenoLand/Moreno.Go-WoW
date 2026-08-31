package render

import "github.com/g3n/engine/renderer"

const m2VertexShader = `#include <attributes>
uniform mat4 MVP;
in vec2 VertexTexcoord2;
out vec2 FragTexcoord;
out vec2 FragTexcoord2;
void main() {
    FragTexcoord = vec2(VertexTexcoord.x, 1.0 - VertexTexcoord.y);
    FragTexcoord2 = vec2(VertexTexcoord2.x, 1.0 - VertexTexcoord2.y);
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

func installM2Shaders(r *renderer.Renderer) {
	r.Shaman.AddShader("morenowow_m2_vertex", m2VertexShader)
	r.Shaman.AddShader("morenowow_m2_fragment", m2FragmentShader)
	r.Shaman.AddShader("morenowow_m2_alpha_key_fragment", m2AlphaKeyFragmentShader)
	r.Shaman.AddProgram("morenowow_m2", "morenowow_m2_vertex", "morenowow_m2_fragment")
	r.Shaman.AddProgram("morenowow_m2_alpha_key", "morenowow_m2_vertex", "morenowow_m2_alpha_key_fragment")
}
