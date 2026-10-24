package render

import (
	"image"
	"image/draw"
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

// The ring under a target is Wow.exe's ObjectSelectionCircle: Textures\UnitSelectTexture.blp
// laid on the ground over a square of half side r around the unit (FUN_00725980,
// FUN_007441d0), tinted by how the unit stands to the player. r is worked out
// from the model's x and y extents (FUN_00720330): the square root of their
// half diagonal times the unit's scale, plus a quadratic term above 5, and at
// most 10.
const (
	worldRingGrid     = 9
	worldRingLift     = 0.05
	worldRingMaxSize  = 10
	worldRingKnee     = 5
	worldRingKneeRate = 0.06
	worldRingDefault  = 1.2
)

const (
	selectRingFragmentShader = `precision highp float;
#include <material>
in vec2 FragTexcoord;
in float FragFog;
out vec4 FragColor;
void main() {
    vec4 texel = vec4(1.0);
#if MAT_TEXTURES > 0
    texel = texture(MatTexture[0], FragTexcoord);
#endif
    FragColor = vec4(texel.rgb * MatEmissiveColor, texel.a * MatOpacity * (1.0 - FragFog));
}`
)

// worldSelectRing is the one ring of the target.
type worldSelectRing struct {
	mesh     *graphic.Mesh
	vbo      *gls.VBO
	material *material.Standard
	texture  *texture.Texture2D
	shown    *core.Node
	lastX    float32
	lastY    float32
	lastZ    float32
	lastSize float32
	built    bool
}

func (ring *worldSelectRing) load(loader *ui.Loader) bool {
	if ring.material != nil {
		return true
	}
	data, err := loader.ReadAsset(`Textures\UnitSelectTexture.blp`)
	if err != nil {
		return false
	}
	img, err := ui.DecodeBLP(data)
	if err != nil {
		return false
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	ring.texture = texture.NewTexture2DFromRGBA(rgba)
	ring.texture.SetWrapS(gls.CLAMP_TO_EDGE)
	ring.texture.SetWrapT(gls.CLAMP_TO_EDGE)
	ring.material = material.NewStandard(&math32.Color{R: 1, G: 1, B: 1})
	ring.material.SetShader("morenowow_select_ring")
	ring.material.SetShaderUnique(true)
	ring.material.SetUseLights(material.UseLightNone)
	ring.material.SetSide(material.SideDouble)
	ring.material.SetDepthTest(true)
	ring.material.SetDepthMask(false)
	ring.material.SetTransparent(true)
	ring.material.SetBlending(material.BlendNormal)
	ring.material.SetPolygonOffset(-2, -2)
	ring.material.AddTexture(ring.texture)
	count := worldRingGrid * worldRingGrid
	positions := math32.NewArrayF32(count*5, count*5)
	ring.vbo = gls.NewVBO(positions).AddAttrib(gls.VertexPosition).AddAttrib(gls.VertexTexcoord)
	ring.vbo.SetUsage(gls.DYNAMIC_DRAW)
	indices := math32.NewArrayU32(0, (worldRingGrid-1)*(worldRingGrid-1)*6)
	for row := 0; row < worldRingGrid-1; row++ {
		for column := 0; column < worldRingGrid-1; column++ {
			a := uint32(row*worldRingGrid + column)
			indices.Append(a, a+1, a+worldRingGrid+1, a, a+worldRingGrid+1, a+worldRingGrid)
		}
	}
	geom := geometry.NewGeometry()
	geom.SetIndices(indices)
	geom.AddVBO(ring.vbo)
	ring.mesh = graphic.NewMesh(geom, ring.material)
	ring.mesh.SetRenderOrder(-59)
	ring.mesh.SetCullable(false)
	return true
}

// worldSelectRingSize is the half side of the ring square for a unit.
func worldSelectRingSize(info glueModelInfo, scale float32) float32 {
	half := math.Hypot(float64(info.shadowHalf[0]), float64(info.shadowHalf[1]))
	if half < 1.0/1048576 {
		return worldRingDefault
	}
	product := half * float64(scale)
	size := math.Sqrt(product)
	if product > worldRingKnee {
		excess := product - worldRingKnee
		size += excess * excess * worldRingKneeRate
	}
	if size > worldRingMaxSize {
		size = worldRingMaxSize
	}
	return float32(size)
}

// update puts the ring under the target's entity, or removes it when there is none.
func (ring *worldSelectRing) update(scene *core.Node, target *core.Node, colour [3]float32, collision *worldCollisionHolder, loader *ui.Loader) {
	if target == nil || collision == nil {
		ring.hide()
		return
	}
	info, ok := target.UserData().(glueModelInfo)
	if !ok || !ring.load(loader) {
		ring.hide()
		return
	}
	if ring.shown != target || ring.mesh.Parent() == nil {
		ring.hide()
		scene.Add(ring.mesh)
		ring.shown = target
		ring.built = false
	}
	ring.material.SetEmissiveColor(&math32.Color{R: colour[0], G: colour[1], B: colour[2]})
	position := target.Position()
	scale := target.Scale().X
	if scale <= 0 {
		scale = 1
	}
	size := worldSelectRingSize(info, scale)
	if ring.built && math.Abs(float64(position.X-ring.lastX)) < 0.02 && math.Abs(float64(position.Y-ring.lastY)) < 0.02 && math.Abs(float64(position.Z-ring.lastZ)) < 0.02 && size == ring.lastSize {
		return
	}
	ring.lastX, ring.lastY, ring.lastZ, ring.lastSize, ring.built = position.X, position.Y, position.Z, size, true
	buffer := ring.vbo.Buffer()
	for row := 0; row < worldRingGrid; row++ {
		for column := 0; column < worldRingGrid; column++ {
			u := float32(column) / (worldRingGrid - 1)
			v := float32(row) / (worldRingGrid - 1)
			x := position.X + (u*2-1)*size
			y := position.Y + (v*2-1)*size
			z := position.Z
			if ground, found := collision.floor(x, y, position.Z+size); found {
				z = ground
			}
			index := (row*worldRingGrid + column) * 5
			buffer.Set(index, x)
			buffer.Set(index+1, y)
			buffer.Set(index+2, z+worldRingLift)
			buffer.Set(index+3, u)
			buffer.Set(index+4, 1-v)
		}
	}
	ring.vbo.Update()
	// The ring is projected into a cube of half side size around the unit
	// (FUN_00725980): ground above or below that is not marked, so a cell whose
	// ground lies outside it is left out.
	indices := math32.NewArrayU32(0, (worldRingGrid-1)*(worldRingGrid-1)*6)
	for row := 0; row < worldRingGrid-1; row++ {
		for column := 0; column < worldRingGrid-1; column++ {
			a := row*worldRingGrid + column
			inside := true
			for _, corner := range []int{a, a + 1, a + worldRingGrid, a + worldRingGrid + 1} {
				if math.Abs(float64((*buffer)[corner*5+2]-worldRingLift-position.Z)) > float64(size) {
					inside = false
				}
			}
			if inside {
				first := uint32(a)
				indices.Append(first, first+1, first+worldRingGrid+1, first, first+worldRingGrid+1, first+worldRingGrid)
			}
		}
	}
	ring.mesh.GetGeometry().SetIndices(indices)
}

func (ring *worldSelectRing) hide() {
	if ring.mesh != nil && ring.mesh.Parent() != nil {
		ring.mesh.Parent().GetNode().Remove(ring.mesh)
	}
	ring.shown = nil
}
