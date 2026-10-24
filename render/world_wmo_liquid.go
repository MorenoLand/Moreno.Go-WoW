package render

import (
	"encoding/binary"
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
)

// worldWMOLiquid is the liquid a map object group holds (MLIQ): a grid of
// surface heights and a flag per tile.
type worldWMOLiquid struct {
	xVerts, yVerts int
	xTiles, yTiles int
	corner         [3]float32
	heights        []float32
	tiles          []byte
}

type worldWMOGroupLiquid struct {
	liquid     *worldWMOLiquid
	liquidType uint32
}

const worldWMOLiquidHeader = 30

func parseWorldWMOLiquid(payload []byte) *worldWMOLiquid {
	if len(payload) < worldWMOLiquidHeader {
		return nil
	}
	liquid := &worldWMOLiquid{
		xVerts: int(binary.LittleEndian.Uint32(payload[0:4])), yVerts: int(binary.LittleEndian.Uint32(payload[4:8])),
		xTiles: int(binary.LittleEndian.Uint32(payload[8:12])), yTiles: int(binary.LittleEndian.Uint32(payload[12:16])),
	}
	for axis := range liquid.corner {
		liquid.corner[axis] = math.Float32frombits(binary.LittleEndian.Uint32(payload[16+axis*4:]))
	}
	vertices := liquid.xVerts * liquid.yVerts
	tiles := liquid.xTiles * liquid.yTiles
	if liquid.xVerts < 2 || liquid.yVerts < 2 || liquid.xTiles != liquid.xVerts-1 || liquid.yTiles != liquid.yVerts-1 || worldWMOLiquidHeader+vertices*8+tiles > len(payload) {
		return nil
	}
	for index := 0; index < vertices; index++ {
		liquid.heights = append(liquid.heights, math.Float32frombits(binary.LittleEndian.Uint32(payload[worldWMOLiquidHeader+index*8+4:])))
	}
	liquid.tiles = payload[worldWMOLiquidHeader+vertices*8 : worldWMOLiquidHeader+vertices*8+tiles]
	return liquid
}

// buildWorldWMOLiquids makes a mesh per liquid type from the groups' liquids,
// in world space. A type the LiquidType table lacks is water, as in the client.
func buildWorldWMOLiquids(loader *ui.Loader, groups []worldWMOGroupLiquid, transform worldWMOTransform, animator *worldLiquidAnimator) []*graphic.Mesh {
	if animator == nil || len(groups) == 0 {
		return nil
	}
	builders := make(map[uint16]*worldLiquidBuilder)
	var order []uint16
	for _, group := range groups {
		liquid := group.liquid
		kind := uint16(group.liquidType)
		animator.mu.Lock()
		if !animator.loaded {
			animator.types, animator.loaded = readWorldLiquidTypes(loader), true
		}
		_, known := animator.types[uint32(kind)]
		animator.mu.Unlock()
		if !known {
			kind = 1
		}
		builder := builders[kind]
		if builder == nil {
			animator.mu.Lock()
			art := animator.artFor(loader, kind)
			animator.mu.Unlock()
			if len(art.frames) == 0 {
				continue
			}
			builder = &worldLiquidBuilder{art: art}
			builders[kind] = builder
			order = append(order, kind)
		}
		base := uint32(len(builder.positions) / 3)
		mode := float32(0)
		if builder.art.procedural {
			mode = 2
		}
		repeat := float32(4 * worldUnitSize)
		for j := 0; j < liquid.yVerts; j++ {
			for i := 0; i < liquid.xVerts; i++ {
				local := [3]float32{liquid.corner[0] + float32(i)*worldUnitSize, liquid.corner[1] + float32(j)*worldUnitSize, liquid.corner[2] + liquid.heights[j*liquid.xVerts+i]}
				at := transform.point(local)
				builder.positions.Append(at[0], at[1], at[2])
				builder.uvs.Append(-at[1]/repeat, -at[0]/repeat)
				builder.colors.Append(builder.art.look.tint[0], builder.art.look.tint[1], builder.art.look.tint[2])
				value := builder.art.look.alpha
				if builder.art.procedural {
					value = 0.5
				}
				builder.extra.Append(value, mode)
			}
		}
		for j := 0; j < liquid.yTiles; j++ {
			for i := 0; i < liquid.xTiles; i++ {
				if liquid.tiles[j*liquid.xTiles+i]&0x0F == 0x0F {
					continue
				}
				a := base + uint32(j*liquid.xVerts+i)
				c := base + uint32((j+1)*liquid.xVerts+i)
				builder.indices.Append(a, a+1, c, a+1, c+1, c)
			}
		}
	}
	var meshes []*graphic.Mesh
	for _, kind := range order {
		builder := builders[kind]
		if len(builder.indices) == 0 {
			continue
		}
		geom := geometry.NewGeometry()
		geom.SetIndices(builder.indices)
		geom.AddVBO(gls.NewVBO(builder.positions).AddAttrib(gls.VertexPosition))
		geom.AddVBO(gls.NewVBO(builder.uvs).AddAttrib(gls.VertexTexcoord))
		geom.AddVBO(gls.NewVBO(builder.colors).AddAttrib(gls.VertexColor))
		geom.AddVBO(gls.NewVBO(builder.extra).AddCustomAttrib("VertexTexcoord2", 2))
		mat := newWorldMaterial()
		mat.water = builder.art.water
		mat.SetShader("morenowow_world_liquid")
		mat.SetShaderUnique(false)
		mat.SetSide(material.SideDouble)
		mat.SetUseLights(material.UseLightNone)
		mat.SetTransparent(true)
		mat.SetDepthTest(true)
		mat.SetDepthMask(false)
		mat.SetBlending(material.BlendNormal)
		mat.AddTexture(builder.art.frames[0])
		mesh := graphic.NewMesh(geom, mat)
		mesh.SetRenderOrder(-35)
		animator.mu.Lock()
		animator.meshes = append(animator.meshes, &worldLiquidMesh{mesh: mesh, mat: mat, art: builder.art})
		animator.mu.Unlock()
		meshes = append(meshes, mesh)
	}
	return meshes
}
