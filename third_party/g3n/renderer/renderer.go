// Copyright 2016 The G3N Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package renderer implements the scene renderer.
package renderer

import (
	"sort"
	"unsafe"

	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/light"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/util/logger"
)

// Package logger
var log = logger.New("RENDERER", logger.Default)

// Renderer renders a scene containing 3D objects and/or 2D GUI elements.
type Renderer struct {
	Shaman                      // Embedded shader manager
	gs          *gls.GLS        // Reference to OpenGL state
	rinfo       core.RenderInfo // Preallocated Render info
	specs       ShaderSpecs     // Preallocated Shader specs
	sortObjects bool            // Flag indicating whether objects should be sorted before rendering
	stats       Stats           // Renderer statistics

	// Populated each frame
	ambLights    []*light.Ambient           // Ambient lights in the scene
	dirLights    []*light.Directional       // Directional lights in the scene
	pointLights  []*light.Point             // Point lights in the scene
	spotLights   []*light.Spot              // Spot lights in the scene
	others       []core.INode               // Other nodes (audio, players, etc)
	graphics     []*graphic.Graphic         // Graphics to be rendered
	grmatsOpaque []*graphic.GraphicMaterial // Opaque graphic materials to be rendered
	grmatsTransp []*graphic.GraphicMaterial // Transparent graphic materials to be rendered
	zLayers      map[int][]gui.IPanel       // All IPanels to be rendered organized by Z-layer
	zLayerKeys   []int                      // Z-layers being used (initially in no particular order, sorted later)

	lastProgram      programKey // program the previous draw of this Render picked
	lastProgramValid bool

	sortScratch []sortEntry
	sortRest    []*graphic.GraphicMaterial
	programIDs  map[programKey]uint64
}

// Stats describes how many objects of each type are being rendered.
// It is cleared at the start of each render.
type Stats struct {
	GraphicMats int // Number of graphic materials rendered
	Lights      int // Number of lights rendered
	Panels      int // Number of GUI panels rendered
	Others      int // Number of other objects rendered
}

// NewRenderer creates and returns a pointer to a new Renderer.
func NewRenderer(gs *gls.GLS) *Renderer {

	r := new(Renderer)
	r.gs = gs
	r.Shaman.Init(gs)
	r.sortObjects = true

	r.ambLights = make([]*light.Ambient, 0)
	r.dirLights = make([]*light.Directional, 0)
	r.pointLights = make([]*light.Point, 0)
	r.spotLights = make([]*light.Spot, 0)
	r.others = make([]core.INode, 0)
	r.graphics = make([]*graphic.Graphic, 0)
	r.grmatsOpaque = make([]*graphic.GraphicMaterial, 0)
	r.grmatsTransp = make([]*graphic.GraphicMaterial, 0)
	r.zLayers = make(map[int][]gui.IPanel)
	r.zLayers[0] = make([]gui.IPanel, 0)
	r.zLayerKeys = append(r.zLayerKeys, 0)

	return r
}

// Stats returns a copy of the statistics for the last frame.
// Should be called after the frame was rendered.
func (r *Renderer) Stats() Stats {

	return r.stats
}

// SetObjectSorting sets whether objects will be sorted before rendering.
func (r *Renderer) SetObjectSorting(sort bool) {

	r.sortObjects = sort
}

// ObjectSorting returns whether objects will be sorted before rendering.
func (r *Renderer) ObjectSorting() bool {

	return r.sortObjects
}

// Render renders the specified scene using the specified camera. Returns an an error.
func (r *Renderer) Render(scene core.INode, cam camera.ICamera) error {

	// Updates world matrices of all scene nodes
	scene.UpdateMatrixWorld()

	// Build RenderInfo
	cam.ViewMatrix(&r.rinfo.ViewMatrix)
	cam.ProjMatrix(&r.rinfo.ProjMatrix)

	// Clear stats and scene arrays
	r.stats = Stats{}
	r.lastProgramValid = false
	r.ambLights = r.ambLights[0:0]
	r.dirLights = r.dirLights[0:0]
	r.pointLights = r.pointLights[0:0]
	r.spotLights = r.spotLights[0:0]
	r.others = r.others[0:0]
	r.graphics = r.graphics[0:0]
	r.grmatsOpaque = r.grmatsOpaque[0:0]
	r.grmatsTransp = r.grmatsTransp[0:0]
	r.zLayers = make(map[int][]gui.IPanel)
	r.zLayers[0] = make([]gui.IPanel, 0)
	r.zLayerKeys = r.zLayerKeys[0:1]
	r.zLayerKeys[0] = 0

	// Prepare for frustum culling
	var proj math32.Matrix4
	proj.MultiplyMatrices(&r.rinfo.ProjMatrix, &r.rinfo.ViewMatrix)
	frustum := math32.NewFrustumFromMatrix(&proj)

	// Classify scene and all scene nodes, culling renderable IGraphics which are fully outside of the camera frustum
	r.classifyAndCull(scene, frustum, 0)

	// Set light counts in shader specs
	r.specs.AmbientLightsMax = len(r.ambLights)
	r.specs.DirLightsMax = len(r.dirLights)
	r.specs.PointLightsMax = len(r.pointLights)
	r.specs.SpotLightsMax = len(r.spotLights)

	// Pre-calculate MV and MVP matrices and compile initial lists of opaque and transparent graphic materials
	for _, gr := range r.graphics {
		// Calculate MV and MVP matrices for all non-GUI graphics to be rendered
		gr.CalculateMatrices(r.gs, &r.rinfo)
		// Append all graphic materials of this graphic to lists of graphic materials to be rendered
		materials := gr.Materials()
		for i := range materials {
			r.stats.GraphicMats++
			if materials[i].IMaterial().GetMaterial().Transparent() {
				r.grmatsTransp = append(r.grmatsTransp, &materials[i])
			} else {
				r.grmatsOpaque = append(r.grmatsOpaque, &materials[i])
			}
		}
	}

	// TODO: If both GraphicMaterials belong to same Graphic we might want to keep their relative order...
	// Z-sort graphic materials back to front
	if r.sortObjects {
		r.stateSort(r.grmatsOpaque)
		zSort(r.grmatsTransp)
	}

	// Sort zLayers back to front
	sort.Ints(r.zLayerKeys)

	// Iterate over all panels from back to front, setting Z and adding graphic materials to grmatsTransp/grmatsOpaque
	const deltaZ = 0.00001
	panZ := float32(-1 + float32(r.stats.Panels)*deltaZ)
	for _, k := range r.zLayerKeys {
		for _, ipan := range r.zLayers[k] {
			// Set panel Z
			ipan.SetPositionZ(panZ)
			panZ -= deltaZ
			// Append the panel's graphic material to lists of graphic materials to be rendered
			mat := ipan.GetGraphic().Materials()[0]
			if mat.IMaterial().GetMaterial().Transparent() {
				r.grmatsTransp = append(r.grmatsTransp, &mat)
			} else {
				r.grmatsOpaque = append(r.grmatsOpaque, &mat)
			}
		}
	}

	// Render opaque objects front to back
	for i := len(r.grmatsOpaque) - 1; i >= 0; i-- {
		err := r.renderGraphicMaterial(r.grmatsOpaque[i])
		if err != nil {
			return err
		}
	}

	// Render transparent objects back to front
	for _, grmat := range r.grmatsTransp {
		err := r.renderGraphicMaterial(grmat)
		if err != nil {
			return err
		}
	}

	// Render other nodes (audio players, etc)
	for _, inode := range r.others {
		inode.Render(r.gs)
	}

	// Enable depth mask so that clearing the depth buffer works
	r.gs.DepthMask(true)
	// TODO enable color mask, stencil mask?
	// TODO clear the buffers for the user, and set the appropriate masks to true before clearing

	return nil
}

// classifyAndCull classifies the provided INode and all of its descendents.
// It ignores (culls) renderable IGraphics which are fully outside of the specified frustum.
// sortEntry is a graphic material with what it is ordered by, worked out once.
type sortEntry struct {
	key   uint64
	depth float32
	index int // where it was submitted, to keep that order among equals
	grmat *graphic.GraphicMaterial
}

// stateSort orders opaque materials. Those of graphics that allow it (units,
// which nothing lies flush against) go last in draw order, grouped by render
// order, program and texture so that what shares state is drawn one after the
// other; the rest keep the back to front order they always had, which some of
// them (the ground, map objects) depend on for coplanar surfaces. The draw loop
// goes from the end of the list.
func (r *Renderer) stateSort(grmats []*graphic.GraphicMaterial) {

	zSort(grmats)
	if r.programIDs == nil {
		r.programIDs = make(map[programKey]uint64)
	}
	grouped := r.sortScratch[:0]
	rest := r.sortRest[:0]
	for _, grmat := range grmats {
		gr := grmat.IGraphic().GetGraphic()
		if !gr.StateSort() {
			rest = append(rest, grmat)
			continue
		}
		mat := grmat.IMaterial().GetMaterial()
		program := programKey{mat.Shader(), mat.ShaderUnique(), mat.UseLights(), mat.TextureCount()}
		programID, ok := r.programIDs[program]
		if !ok {
			programID = uint64(len(r.programIDs) + 1)
			r.programIDs[program] = programID
		}
		// Textures group by where they live: the same texture is the same address.
		var textureID uint64
		if tex := mat.FirstTexture(); tex != nil {
			textureID = uint64(uintptr(unsafe.Pointer(tex)) >> 4)
		}
		position := gr.Position()
		position.ApplyMatrix4(gr.ModelViewMatrix())
		order := int64(gr.RenderOrder())
		if order < -32768 {
			order = -32768
		} else if order > 32767 {
			order = 32767
		}
		grouped = append(grouped, sortEntry{key: uint64(order+32768)<<48 | (programID&0xFFF)<<36 | textureID&0xFFFFFFFFF, depth: position.Z, index: len(grouped), grmat: grmat})
	}
	if len(grouped) > 0 {
		sort.Slice(grouped, func(i, j int) bool {
			if grouped[i].key != grouped[j].key {
				return grouped[i].key < grouped[j].key
			}
			if grouped[i].depth != grouped[j].depth {
				return grouped[i].depth < grouped[j].depth
			}
			return grouped[i].index > grouped[j].index
		})
		for index := range grouped {
			grmats[index] = grouped[index].grmat
			grouped[index].grmat = nil
		}
		copy(grmats[len(grouped):], rest)
	}
	for index := range rest {
		rest[index] = nil
	}
	r.sortScratch = grouped[:0]
	r.sortRest = rest[:0]
}

// Drawn returns the graphics of the last Render that passed culling, in the
// order they were found; the slice is reused by the next Render.
func (r *Renderer) Drawn() []*graphic.Graphic { return r.graphics }

// programKey is what picks a shader program for a material that adds no defines.
type programKey struct {
	name   string
	unique bool
	lights material.UseLights
	count  int
}

// The classes classifyAndCull sorts nodes into, kept on the node.
const (
	classPanel uint8 = iota + 1
	classGraphic
	classLight
	classOther
)

func (r *Renderer) classifyAndCull(inode core.INode, frustum *math32.Frustum, zLayer int) {

	// Ignore invisible nodes and their descendants
	if !inode.Visible() {
		return
	}
	node := inode.GetNode()
	kind := node.ClassKind()
	if kind == 0 {
		switch inode.(type) {
		case gui.IPanel:
			kind = classPanel
		case graphic.IGraphic:
			kind = classGraphic
		case light.ILight:
			kind = classLight
		default:
			kind = classOther
		}
		node.SetClassKind(kind)
	}
	var ipan gui.IPanel
	var igr graphic.IGraphic
	switch kind {
	case classPanel:
		ipan = inode.(gui.IPanel)
	case classGraphic:
		igr = inode.(graphic.IGraphic)
	}
	// If node is an IPanel append it to appropriate list
	if ipan != nil {
		zLayer += ipan.ZLayerDelta()
		if ipan.Renderable() {
			// TODO cull panels
			_, ok := r.zLayers[zLayer]
			if !ok {
				r.zLayerKeys = append(r.zLayerKeys, zLayer)
				r.zLayers[zLayer] = make([]gui.IPanel, 0)
			}
			r.zLayers[zLayer] = append(r.zLayers[zLayer], ipan)
			r.stats.Panels++
		}
		// Check if node is an IGraphic
	} else if igr != nil {
		if igr.Renderable() {
			gr := igr.GetGraphic()
			// Frustum culling
			if igr.Cullable() {
				bb := gr.WorldBoundingBox(igr.GetGeometry().BoundingBox())
				if frustum.IntersectsBox(&bb) {
					// Append graphic to list of graphics to be rendered
					r.graphics = append(r.graphics, gr)
				}
			} else {
				// Append graphic to list of graphics to be rendered
				r.graphics = append(r.graphics, gr)
			}
		}
		// Node is not a Graphic
	} else {
		// Check if node is a Light
		if kind == classLight {
			il := inode.(light.ILight)
			switch l := il.(type) {
			case *light.Ambient:
				r.ambLights = append(r.ambLights, l)
			case *light.Directional:
				r.dirLights = append(r.dirLights, l)
			case *light.Point:
				r.pointLights = append(r.pointLights, l)
			case *light.Spot:
				r.spotLights = append(r.spotLights, l)
			default:
				panic("Invalid light type")
			}
			// Other nodes
		} else {
			r.others = append(r.others, inode)
			r.stats.Others++
		}
	}
	// Classify children
	for _, ichild := range inode.Children() {
		r.classifyAndCull(ichild, frustum, zLayer)
	}
}

// zSort sorts a list of graphic materials based on the user-specified render order
// then based on their Z position relative to the camera, back to front.
func zSort(grmats []*graphic.GraphicMaterial) {

	sort.Slice(grmats, func(i, j int) bool {
		gr1 := grmats[i].IGraphic().GetGraphic()
		gr2 := grmats[j].IGraphic().GetGraphic()
		// Check for user-supplied render order
		rO1 := gr1.RenderOrder()
		rO2 := gr2.RenderOrder()
		if rO1 != rO2 {
			return rO1 < rO2
		}
		mvm1 := gr1.ModelViewMatrix()
		mvm2 := gr2.ModelViewMatrix()
		g1pos := gr1.Position()
		g2pos := gr2.Position()
		g1pos.ApplyMatrix4(mvm1)
		g2pos.ApplyMatrix4(mvm2)
		return g1pos.Z < g2pos.Z
	})
}

// renderGraphicMaterial renders the specified graphic material.
func (r *Renderer) renderGraphicMaterial(grmat *graphic.GraphicMaterial) error {

	mat := grmat.IMaterial().GetMaterial()
	geom := grmat.IGraphic().GetGeometry()
	gr := grmat.IGraphic().GetGraphic()

	// Most materials define nothing extra; when one asks for the program the
	// previous draw used, nothing is built or compared again.
	plain := len(mat.ShaderDefines) == 0 && len(geom.ShaderDefines) == 0 && len(gr.ShaderDefines) == 0
	key := programKey{mat.Shader(), mat.ShaderUnique(), mat.UseLights(), mat.TextureCount()}
	if !(plain && r.lastProgramValid && r.lastProgram == key) {
		// Add defines from material, geometry and graphic
		r.specs.Defines = *gls.NewShaderDefines()
		r.specs.Defines.Add(&mat.ShaderDefines)
		r.specs.Defines.Add(&geom.ShaderDefines)
		r.specs.Defines.Add(&gr.ShaderDefines)

		// Set the shader specs for this material and set shader program
		r.specs.Name = mat.Shader()
		r.specs.ShaderUnique = mat.ShaderUnique()
		r.specs.UseLights = mat.UseLights()
		r.specs.MatTexturesMax = mat.TextureCount()

		// Set active program and apply shader specs
		_, err := r.Shaman.SetProgram(&r.specs)
		if err != nil {
			return err
		}
		r.lastProgram, r.lastProgramValid = key, plain
	}

	// Set up lights (transfer lights' uniforms)
	if r.specs.UseLights != material.UseLightNone {
		if r.specs.UseLights&material.UseLightAmbient != 0 {
			for idx, l := range r.ambLights {
				l.RenderSetup(r.gs, &r.rinfo, idx)
				r.stats.Lights++
			}
		}
		if r.specs.UseLights&material.UseLightDirectional != 0 {
			for idx, l := range r.dirLights {
				l.RenderSetup(r.gs, &r.rinfo, idx)
				r.stats.Lights++
			}
		}
		if r.specs.UseLights&material.UseLightPoint != 0 {
			for idx, l := range r.pointLights {
				l.RenderSetup(r.gs, &r.rinfo, idx)
				r.stats.Lights++
			}
		}
		if r.specs.UseLights&material.UseLightSpot != 0 {
			for idx, l := range r.spotLights {
				l.RenderSetup(r.gs, &r.rinfo, idx)
				r.stats.Lights++
			}
		}
	}

	// Render this graphic material
	grmat.Render(r.gs, &r.rinfo)

	return nil
}
