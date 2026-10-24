package render

import (
	"reflect"
	"strings"
	"time"
	"unsafe"

	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/texture"
)

// A mesh or texture the graphics card has been given still has its CPU copy
// kept by the engine, and a city's worth of those is gigabytes. These helpers
// let the copies go once what is static has been drawn.

func unexportedField(object interface{}, name string) reflect.Value {
	value := reflect.ValueOf(object).Elem().FieldByName(name)
	return reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem()
}

// textureUploaded reports whether a texture has been sent to the card.
func textureUploaded(tex *texture.Texture2D) bool {
	return !unexportedField(tex, "gs").IsNil() && !unexportedField(tex, "updateData").Bool()
}

// dropTextureData lets go of a texture's pixels once the card has them.
func dropTextureData(tex *texture.Texture2D) {
	if tex == nil || !textureUploaded(tex) {
		return
	}
	data := unexportedField(tex, "data")
	if data.IsNil() {
		return
	}
	data.Set(reflect.Zero(data.Type()))
}

// geometryUploaded reports whether a geometry has been drawn at least once.
func geometryUploaded(geom *geometry.Geometry) bool {
	return !unexportedField(geom, "gs").IsNil()
}

// dropGeometryData lets go of the vertex arrays of a static geometry once the
// card has them; its bounds are worked out first, as culling needs them.
func dropGeometryData(geom *geometry.Geometry) {
	geom.BoundingSphere()
	for _, vbo := range geom.VBOs() {
		vbo.SetBuffer(nil)
	}
}

// worldStaticShader reports whether a shader draws geometry that is never
// changed after it is built.
func worldStaticShader(name string) bool {
	return strings.HasPrefix(name, "morenowow_world_terrain") || strings.HasPrefix(name, "morenowow_world_wmo")
}

// compact frees the CPU copies of what the loaded tiles have already uploaded.
func (s *worldTerrainStreamer) compact() {
	done := make(map[*geometry.Geometry]bool)
	var walk func(node core.INode)
	walk = func(node core.INode) {
		if mesh, ok := node.(*graphic.Mesh); ok {
			if mat, ok := mesh.GetMaterial(0).(*worldMaterial); ok && worldStaticShader(mat.Shader()) {
				geom := mesh.GetGeometry()
				if !done[geom] && geometryUploaded(geom) && !s.compacted[geom] {
					dropGeometryData(geom)
					s.compacted[geom] = true
				}
				done[geom] = true
			}
		}
		for _, child := range node.GetNode().Children() {
			walk(child)
		}
	}
	for _, tile := range s.tiles {
		walk(tile.node)
		for _, tex := range tile.collision.owned {
			dropTextureData(tex)
		}
	}
	for _, tex := range s.cache.textures {
		dropTextureData(tex)
	}
}

var _ = time.Second

// materialTextures are the textures a material draws with.
func materialTextures(mat material.IMaterial) []*texture.Texture2D {
	if mat == nil {
		return nil
	}
	field := unexportedField(mat.GetMaterial(), "textures")
	textures, _ := field.Interface().([]*texture.Texture2D)
	return textures
}

// compactEntityTextures lets go of the pixels of the textures the units and
// objects of the scene have been drawn with.
func compactEntityTextures(entities map[uint64]*worldEntity) {
	var walk func(node core.INode)
	walk = func(node core.INode) {
		if mesh, ok := node.(*graphic.Mesh); ok {
			for _, tex := range materialTextures(mesh.GetMaterial(0)) {
				dropTextureData(tex)
			}
		}
		for _, child := range node.GetNode().Children() {
			walk(child)
		}
	}
	for _, entity := range entities {
		if entity.node != nil {
			walk(entity.node)
		}
	}
	dropSharedTextureData()
}
