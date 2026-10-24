package render

import (
	"image"
	"image/color"
	"math"

	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/texture"
)

const (
	blobShadowVertexShader = `#include <attributes>
uniform mat4 MVP;
` + worldFogVertexDeclarations + `out vec2 FragTexcoord;
void main() {
    FragTexcoord = VertexTexcoord;
` + worldFogVertexBody + `    gl_Position = MVP * vec4(VertexPosition, 1.0);
}`
	blobShadowFragmentShader = `precision highp float;
#include <material>
in vec2 FragTexcoord;
in float FragFog;
out vec4 FragColor;
void main() {
    float shade = 0.0;
#if MAT_TEXTURES > 0
    shade = texture(MatTexture[0], FragTexcoord).a;
#endif
    FragColor = vec4(0.0, 0.0, 0.0, shade * 0.5 * (1.0 - FragFog));
}`
)

var (
	blobShadowGeometry *geometry.Geometry
	blobShadowTexture  *texture.Texture2D
	blobShadowMaterial *material.Standard
)

// blobShadowTextureImage is a soft round shadow, dark in the middle and fading
// to nothing at the edge.
func blobShadowTextureImage() *image.RGBA {
	const size = 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := (float64(x)+0.5)/size*2-1, (float64(y)+0.5)/size*2-1
			distance := math.Sqrt(dx*dx + dy*dy)
			alpha := 1 - distance
			if alpha < 0 {
				alpha = 0
			}
			alpha = alpha * alpha * (3 - 2*alpha)
			img.SetRGBA(x, y, color.RGBA{A: uint8(alpha * 255)})
		}
	}
	return img
}

// addBlobShadow puts a soft shadow on the ground under a unit or object. radius
// is in the node's own units, so it grows with the node's scale.
func addBlobShadow(node *core.Node, radius float32) {
	if node == nil || radius <= 0 {
		return
	}
	if blobShadowGeometry == nil {
		positions := math32.NewArrayF32(0, 20)
		positions.Append(
			-1, -1, 0, 0, 0,
			1, -1, 0, 1, 0,
			1, 1, 0, 1, 1,
			-1, 1, 0, 0, 1,
		)
		indices := math32.NewArrayU32(0, 6)
		indices.Append(0, 1, 2, 0, 2, 3)
		blobShadowGeometry = geometry.NewGeometry()
		blobShadowGeometry.SetIndices(indices)
		blobShadowGeometry.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition).AddAttrib(gls.VertexTexcoord))
		blobShadowTexture = texture.NewTexture2DFromRGBA(blobShadowTextureImage())
		blobShadowTexture.SetWrapS(gls.CLAMP_TO_EDGE)
		blobShadowTexture.SetWrapT(gls.CLAMP_TO_EDGE)
		blobShadowMaterial = material.NewStandard(&math32.Color{R: 0, G: 0, B: 0})
		blobShadowMaterial.SetShader("morenowow_blob_shadow")
		blobShadowMaterial.SetShaderUnique(true)
		blobShadowMaterial.SetUseLights(material.UseLightNone)
		blobShadowMaterial.SetSide(material.SideDouble)
		blobShadowMaterial.SetDepthTest(true)
		blobShadowMaterial.SetDepthMask(false)
		blobShadowMaterial.SetTransparent(true)
		blobShadowMaterial.SetBlending(material.BlendNormal)
		blobShadowMaterial.SetPolygonOffset(-3, -3)
		blobShadowMaterial.AddTexture(blobShadowTexture)
	}
	blobShadowGeometry.Incref()
	mesh := graphic.NewMesh(blobShadowGeometry, blobShadowMaterial)
	mesh.SetRenderOrder(-60)
	mesh.SetCullable(false)
	mesh.SetPosition(0, 0, 0.04)
	mesh.SetScale(radius, radius, 1)
	node.Add(mesh)
}
