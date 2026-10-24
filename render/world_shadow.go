package render

import (
	"image"
	"image/draw"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/texture"
)

// Unit shadows follow Wow.exe's blob path (FUN_007e4480, used while the
// extShadowQuality cvar is below 1, its default of 0): Textures\ShadowBlob.blp
// laid over the ground across the model's box, half its x and y extents on
// each side of the origin, and multiplied into what is below. It only draws
// where the ground lies inside a volume reaching 5/3 of the box half height
// below the origin and one half height above it.
const (
	worldShadowGrid   = 5
	worldShadowBelow  = 1.6666666
	worldShadowAbove  = 1.0
	worldShadowLift   = 0.1
	worldShadowPeriod = 100 * time.Millisecond
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
    vec3 shade = vec3(1.0);
#if MAT_TEXTURES > 0
    vec4 texel = texture(MatTexture[0], FragTexcoord);
    shade = mix(vec3(1.0), texel.rgb, texel.a);
#endif
    FragColor = vec4(mix(shade, vec3(1.0), FragFog), 1.0);
}`
)

var worldShadowTexture *texture.Texture2D
var worldShadowMaterial *material.Standard

// worldShadowState is one unit's shadow mesh and where it was last laid.
type worldShadowState struct {
	mesh         *graphic.Mesh
	vbo          *gls.VBO
	checkedAt    time.Time
	x, y, z, yaw float32
	built        bool
}

// modelShadowHalfExtents is half the model's extent on x and y.
func modelShadowHalfExtents(vertices []m2Vertex) [2]float32 {
	if len(vertices) == 0 {
		return [2]float32{}
	}
	low := [2]float32{math.MaxFloat32, math.MaxFloat32}
	high := [2]float32{-math.MaxFloat32, -math.MaxFloat32}
	for _, vertex := range vertices {
		for axis := 0; axis < 2; axis++ {
			low[axis] = float32(math.Min(float64(low[axis]), float64(vertex.position[axis])))
			high[axis] = float32(math.Max(float64(high[axis]), float64(vertex.position[axis])))
		}
	}
	return [2]float32{(high[0] - low[0]) * 0.5, (high[1] - low[1]) * 0.5}
}

func loadWorldShadowTexture(loader *ui.Loader) {
	if worldShadowTexture != nil || loader == nil {
		return
	}
	data, err := loader.ReadAsset(`Textures\ShadowBlob.blp`)
	if err != nil {
		return
	}
	img, err := ui.DecodeBLP(data)
	if err != nil {
		return
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	// Texels the file leaves out are black, which filtering would pull into the
	// edge of the shadow as a dark ring; they multiply as no shadow instead.
	for i := 0; i < len(rgba.Pix); i += 4 {
		if rgba.Pix[i+3] == 0 {
			rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2], rgba.Pix[i+3] = 255, 255, 255, 255
		}
	}
	worldShadowTexture = texture.NewTexture2DFromRGBA(rgba)
	worldShadowTexture.SetWrapS(gls.CLAMP_TO_EDGE)
	worldShadowTexture.SetWrapT(gls.CLAMP_TO_EDGE)
	worldShadowMaterial = material.NewStandard(&math32.Color{R: 1, G: 1, B: 1})
	worldShadowMaterial.SetShader("morenowow_blob_shadow")
	worldShadowMaterial.SetShaderUnique(true)
	worldShadowMaterial.SetUseLights(material.UseLightNone)
	worldShadowMaterial.SetSide(material.SideDouble)
	worldShadowMaterial.SetDepthTest(true)
	worldShadowMaterial.SetDepthMask(false)
	worldShadowMaterial.SetTransparent(true)
	worldShadowMaterial.SetBlending(material.BlendMultiply)
	worldShadowMaterial.SetPolygonOffset(-2, -2)
	worldShadowMaterial.AddTexture(worldShadowTexture)
}

func newWorldShadowMesh() (*graphic.Mesh, *gls.VBO) {
	count := worldShadowGrid * worldShadowGrid
	positions := math32.NewArrayF32(count*5, count*5)
	vbo := gls.NewVBO(positions).AddAttrib(gls.VertexPosition).AddAttrib(gls.VertexTexcoord)
	vbo.SetUsage(gls.DYNAMIC_DRAW)
	indices := math32.NewArrayU32(0, (worldShadowGrid-1)*(worldShadowGrid-1)*6)
	for row := 0; row < worldShadowGrid-1; row++ {
		for column := 0; column < worldShadowGrid-1; column++ {
			a := uint32(row*worldShadowGrid + column)
			b := a + 1
			c := a + worldShadowGrid
			d := c + 1
			indices.Append(a, b, d, a, d, c)
		}
	}
	geom := geometry.NewGeometry()
	geom.SetIndices(indices)
	geom.AddVBO(vbo)
	mesh := graphic.NewMesh(geom, worldShadowMaterial)
	mesh.SetRenderOrder(-60)
	mesh.SetCullable(false)
	return mesh, vbo
}

// updateWorldShadow lays a unit's shadow over the ground under it. The mesh
// is a small grid whose heights follow the ground, rebuilt when the unit
// moves or turns.
func updateWorldShadow(node *core.Node, state *worldShadowState, collision *worldCollisionHolder, loader *ui.Loader, now time.Time) {
	if node == nil || collision == nil || now.Sub(state.checkedAt) < worldShadowPeriod {
		return
	}
	info, ok := node.UserData().(glueModelInfo)
	if !ok || info.shadowHalf[0] <= 0 || info.shadowHalf[1] <= 0 || info.rawHeight <= 0 {
		return
	}
	loadWorldShadowTexture(loader)
	if worldShadowMaterial == nil {
		return
	}
	state.checkedAt = now
	if state.mesh == nil || state.mesh.Parent() != core.INode(node) {
		state.mesh, state.vbo = newWorldShadowMesh()
		state.built = false
		node.Add(state.mesh)
	}
	position := node.Position()
	rotation := node.Rotation()
	yaw := rotation.Z
	if state.built && math.Abs(float64(position.X-state.x)) < 0.05 && math.Abs(float64(position.Y-state.y)) < 0.05 && math.Abs(float64(position.Z-state.z)) < 0.05 && yaw == state.yaw {
		return
	}
	state.x, state.y, state.z, state.yaw, state.built = position.X, position.Y, position.Z, yaw, true
	scale := node.Scale()
	size := scale.X
	if size <= 0 {
		size = 1
	}
	height := info.rawHeight * 0.5
	sin, cos := float32(math.Sin(float64(yaw))), float32(math.Cos(float64(yaw)))
	buffer := state.vbo.Buffer()
	covered := false
	for row := 0; row < worldShadowGrid; row++ {
		for column := 0; column < worldShadowGrid; column++ {
			u := float32(column) / (worldShadowGrid - 1)
			v := float32(row) / (worldShadowGrid - 1)
			lx := (u*2 - 1) * info.shadowHalf[0]
			ly := (v*2 - 1) * info.shadowHalf[1]
			wx := position.X + (lx*cos-ly*sin)*size
			wy := position.Y + (lx*sin+ly*cos)*size
			z := float32(0)
			if ground, found := collision.floor(wx, wy, position.Z+height*size); found {
				z = ground - position.Z
				if z >= -worldShadowBelow*height*size && z <= worldShadowAbove*height*size {
					covered = true
				}
			}
			index := (row*worldShadowGrid + column) * 5
			buffer.Set(index, lx)
			buffer.Set(index+1, ly)
			buffer.Set(index+2, z/size+worldShadowLift)
			buffer.Set(index+3, u)
			buffer.Set(index+4, v)
		}
	}
	state.vbo.Update()
	state.mesh.SetVisible(covered)
}
