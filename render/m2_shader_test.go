package render

import (
	"strings"
	"testing"
)

func TestM2FragmentShadersUseMaterialInclude(t *testing.T) {
	for name, source := range map[string]string{"m2": m2FragmentShader, "particle": m2ParticleFragmentShader, "alpha-key": m2AlphaKeyFragmentShader} {
		if !strings.Contains(source, "#include <material>") {
			t.Fatalf("%s fragment shader does not include material", name)
		}
		if strings.Contains(source, "uniform sampler2D MatTexture[MAT_TEXTURES]") {
			t.Fatalf("%s fragment shader redeclares material textures", name)
		}
	}
}
