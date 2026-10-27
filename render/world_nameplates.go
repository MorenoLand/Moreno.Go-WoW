package render

import (
	"fmt"
	"math"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/texture"
)

const (
	nameplateRange        = 60.0
	nameplateMaxNewPerRun = 12
	playerFlagsField      = 150 // PLAYER_FLAGS
	playerGuildField      = 151 // PLAYER_GUILDID
	playerFlagGM          = 0x08
)

// worldNameplate is the name drawn over one unit: a quad the size of the text
// that always faces the camera, anchored over the unit's head in the world and
// depth tested, so whatever stands in front of that point covers it.
type worldNameplate struct {
	mesh *graphic.Mesh
	mat  *nameplateMaterial
	tex  *texture.Texture2D
	key  string
	// pixels is the picture's size on screen.
	pixels [2]float32
	// bucket is the pixel size the picture was drawn at.
	bucket int
}

const (
	nameplateVertexShader = `#include <attributes>
uniform mat4 MVP;
uniform mat4 ModelViewMatrix;
uniform vec2 PlateSize;
out vec2 FragTexcoord;
void main() {
    vec3 cameraRight = normalize(vec3(ModelViewMatrix[0][0], ModelViewMatrix[1][0], ModelViewMatrix[2][0]));
    vec3 cameraUp = normalize(vec3(ModelViewMatrix[0][1], ModelViewMatrix[1][1], ModelViewMatrix[2][1]));
    vec3 vertex = cameraRight * (VertexPosition.x * PlateSize.x) + cameraUp * (VertexPosition.y * PlateSize.y);
    gl_Position = MVP * vec4(vertex, 1.0);
    FragTexcoord = VertexTexcoord;
}`
	nameplateFragmentShader = `precision highp float;
#include <material>
in vec2 FragTexcoord;
out vec4 FragColor;
void main() {
    vec4 result = vec4(1.0);
#if MAT_TEXTURES > 0
    result = texture(MatTexture[0], FragTexcoord * MatTexRepeat(0) + MatTexOffset(0));
#endif
    FragColor = result;
}`
)

// nameplateMaterial feeds a plate's size in world units to its shader.
// worldPlateRenderOrder draws names after every blended surface.
const worldPlateRenderOrder = 1 << 30

type nameplateMaterial struct {
	*material.Standard
	size    [2]float32
	sizeLoc gls.Uniform
}

func newNameplateMaterial() *nameplateMaterial {
	m := &nameplateMaterial{Standard: material.NewStandard(&math32.Color{R: 1, G: 1, B: 1})}
	m.sizeLoc.Init("PlateSize")
	return m
}

func (m *nameplateMaterial) RenderSetup(gs *gls.GLS) {
	m.Standard.RenderSetup(gs)
	gs.Uniform2f(m.sizeLoc.Location(gs), m.size[0], m.size[1])
}

// newNameplateMesh is a unit quad centred on its origin, the text picture on it.
func newNameplateMesh(tex *texture.Texture2D) (*graphic.Mesh, *nameplateMaterial) {
	positions := math32.NewArrayF32(0, 20)
	positions.Append(
		-0.5, -0.5, 0, 0, 1,
		0.5, -0.5, 0, 1, 1,
		0.5, 0.5, 0, 1, 0,
		-0.5, 0.5, 0, 0, 0,
	)
	indices := math32.NewArrayU32(0, 6)
	indices.Append(0, 1, 2, 0, 2, 3)
	geom := geometry.NewGeometry()
	geom.SetIndices(indices)
	geom.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition).AddAttrib(gls.VertexTexcoord))
	mat := newNameplateMaterial()
	mat.SetShader("morenowow_nameplate")
	mat.SetShaderUnique(true)
	mat.SetUseLights(material.UseLightNone)
	mat.SetSide(material.SideDouble)
	// A name sits in the world at the unit's head: whatever stands nearer, a
	// wall, a hill or an object, covers the part of it behind, and the rest
	// stays visible.
	mat.SetDepthTest(true)
	mat.SetDepthMask(false)
	mat.SetTransparent(true)
	mat.SetBlending(material.BlendNormal)
	mat.AddTexture(tex)
	mesh := graphic.NewMesh(geom, mat)
	mesh.SetRenderOrder(worldPlateRenderOrder)
	mesh.SetCullable(false)
	return mesh, mat
}

// worldNameplates draws the names (and guild or title lines) over the units
// near the player.
type worldNameplates struct {
	plates map[uint64]*worldNameplate
	// modelReads is how many model files may still be read this frame to find
	// attachment points: a crowd of new models would otherwise stall the frame.
	modelReads int
	// pictures is how many name pictures may still be drawn this frame.
	pictures int
	// tops is the height the text of each unit reaches, for what hangs above it.
	tops map[uint64]float32
}

func newWorldNameplates() *worldNameplates {
	return &worldNameplates{plates: make(map[uint64]*worldNameplate), tops: make(map[uint64]float32)}
}

// nameplateInputs is what a frame of nameplates needs from the world.
type nameplateInputs struct {
	scene         *core.Node
	engine        *ui.UIEngine
	cam           *camera.Camera
	width         float32
	height        float32
	entities      map[uint64]*worldEntity
	ownGUID       uint64
	playerNode    *core.Node
	cameraAt      [3]float32
	names         *worldNames
	factions      *worldFactions
	send          func(world.ClientOpcode, []byte) error
	playerFaction uint32
	// ownModelPath is the model file of the player's own character.
	ownModelPath string
	// collision answers whether a wall hides a unit.
	collision *worldCollisionHolder
}

// nameplateColour is the colour of a unit's name from how it relates to the
// player: hostile red, neutral yellow, friendly green for creatures; players
// of the player's side are drawn blue.
func nameplateColour(player, hostile, friendly bool) (uint8, uint8, uint8) {
	switch {
	case hostile:
		return 255, 0, 0
	case player && friendly:
		return 102, 102, 255
	case friendly:
		return 0, 255, 0
	}
	return 255, 255, 0
}

// lines are the text lines over an entity, or nil while its name is unknown.
func (in *nameplateInputs) lines(entity *worldEntity) []ui.NameplateLine {
	hostile, friendly := false, true
	if entity.guid != in.ownGUID && in.factions != nil {
		hostile, friendly = in.factions.reaction(in.playerFaction, entity.fields[unitFactionField])
	}
	switch entity.objectType {
	case world.ObjectTypePlayer:
		name := in.names.players[entity.guid].Name
		if name == "" {
			return nil
		}
		if entity.fields[playerFlagsField]&playerFlagGM != 0 {
			name = "<GM>" + name
		}
		r, g, b := nameplateColour(true, hostile, friendly)
		lines := []ui.NameplateLine{{Text: name, R: r, G: g, B: b}}
		if guildID := entity.fields[playerGuildField]; guildID != 0 {
			in.names.requestGuild(guildID, in.send)
			if guild := in.names.guilds[guildID]; guild != "" {
				lines = append(lines, ui.NameplateLine{Text: "<" + guild + ">", R: r, G: g, B: b})
			}
		}
		return lines
	case world.ObjectTypeUnit:
		creature, ok := in.names.creatures[entity.fields[objectEntryField]]
		if !ok || creature.Name == "" {
			return nil
		}
		r, g, b := nameplateColour(false, hostile, friendly)
		lines := []ui.NameplateLine{{Text: creature.Name, R: r, G: g, B: b}}
		if creature.SubName != "" {
			lines = append(lines, ui.NameplateLine{Text: "<" + creature.SubName + ">", R: r, G: g, B: b})
		}
		return lines
	}
	return nil
}

func nameplateKey(lines []ui.NameplateLine, size int) string {
	key := fmt.Sprintf("%d", size)
	for _, line := range lines {
		key += fmt.Sprintf("|%s/%d.%d.%d", line.Text, line.R, line.G, line.B)
	}
	return key
}

// update places a frame's nameplates.
func (n *worldNameplates) update(in nameplateInputs) {
	if in.engine == nil || in.cam == nil || in.names == nil {
		return
	}
	in.cam.UpdateMatrixWorld()
	n.modelReads = 2
	n.pictures = 6
	seen := make(map[uint64]bool, len(n.plates))
	asked := 0
	for guid, entity := range in.entities {
		node := entity.node
		if guid == in.ownGUID {
			node = in.playerNode
		}
		if node == nil || !node.Visible() || (entity.objectType != world.ObjectTypeUnit && entity.objectType != world.ObjectTypePlayer) {
			continue
		}
		if !in.names.asked[entity.guid] && asked < nameplateMaxNewPerRun {
			asked++
			in.names.request(entity, in.send)
		}
		lines := in.lines(entity)
		if len(lines) == 0 {
			continue
		}
		modelPath := entity.modelPath
		if guid == in.ownGUID {
			modelPath = in.ownModelPath
		}
		head := nameplateAnchor(in.engine.AssetLoader, node, modelPath, &n.modelReads)
		// Wow.exe (PlayerName.cpp FUN_007e5640) sets the name as text in the
		// world: its line height is 0.2 yards, more for units taller than four
		// yards (FUN_007e5420), and its middle sits a line height per line above
		// the overhead point.
		lineHeight := float32(0.2)
		if height := entity.geoHeight * node.Scale().Y; height > 4 {
			lineHeight *= height * 0.25 * 1.5
		}
		anchor := math32.NewVector3(head.X, head.Y, head.Z+float32(len(lines))*lineHeight)
		n.tops[guid] = anchor.Z + float32(len(lines))*lineHeight*0.5
		_, visible := worldToScreen(in.cam, *anchor, in.width, in.height)
		if !visible {
			continue
		}
		// Depth along the view direction.
		var view math32.Matrix4
		in.cam.ViewMatrix(&view)
		depth := -float64(view[2]*anchor.X + view[6]*anchor.Y + view[10]*anchor.Z + view[14])
		if depth < 0.1 {
			continue
		}
		unitsPerPixel := 2 * float32(depth) * float32(math.Tan(float64(in.cam.Fov())*math.Pi/360)) / in.height
		// The line height on screen; the picture is drawn at about that size.
		onScreen := float64(lineHeight / unitsPerPixel)
		if onScreen < 3 {
			continue
		}
		bucket := int(math.Round(onScreen/2)) * 2
		if bucket < 6 {
			bucket = 6
		}
		if bucket > 64 {
			bucket = 64
		}
		key := nameplateKey(lines, bucket)
		plate := n.plates[guid]
		if plate == nil {
			plate = &worldNameplate{}
			n.plates[guid] = plate
		}
		if plate.key != key && plate.mesh != nil && n.pictures <= 0 {
			// The picture is drawn again a few frames on; this one stays meanwhile.
			bucket = plate.bucket
		} else if plate.key != key {
			if n.pictures <= 0 {
				continue
			}
			n.pictures--
			picture := in.engine.RenderNameplate(lines, float64(bucket))
			if picture == nil {
				continue
			}
			if plate.mesh == nil {
				plate.tex = texture.NewTexture2DFromRGBA(picture)
				plate.mesh, plate.mat = newNameplateMesh(plate.tex)
				in.scene.Add(plate.mesh)
			} else {
				plate.tex.SetFromRGBA(picture)
			}
			plate.pixels = [2]float32{float32(picture.Bounds().Dx()), float32(picture.Bounds().Dy())}
			plate.key = key
			plate.bucket = bucket
		}
		// The picture is bucket pixels to the line height.
		scale := lineHeight / float32(bucket)
		plate.mat.size = [2]float32{plate.pixels[0] * scale, plate.pixels[1] * scale}
		plate.mesh.SetPositionVec(anchor)
		plate.mesh.SetVisible(true)
		seen[guid] = true
	}
	for guid, plate := range n.plates {
		if seen[guid] {
			continue
		}
		if plate.mesh != nil {
			plate.mesh.SetVisible(false)
		}
		if _, alive := in.entities[guid]; !alive {
			if plate.mesh != nil {
				in.scene.Remove(plate.mesh)
				plate.mesh.Dispose()
			}
			delete(n.plates, guid)
			delete(n.tops, guid)
		}
	}
}

// hide takes every nameplate off the screen.
func (n *worldNameplates) hide() {
	for _, plate := range n.plates {
		if plate.mesh != nil {
			plate.mesh.SetVisible(false)
		}
	}
}

// nameplateAnchor is the point a unit's name hangs from, in the world: where
// Wow.exe's overhead position (FUN_0071fef0) puts it, the model's attachment 18
// (0x12). A model without one gets its top.
func nameplateAnchor(loader *ui.Loader, node *core.Node, modelPath string, modelReads *int) math32.Vector3 {
	position := node.Position()
	info, hasInfo := node.UserData().(glueModelInfo)
	if loader != nil && modelPath != "" && hasInfo && info.animation != nil {
		attachments, known := info.attachments, len(info.attachments) > 0
		if !known {
			attachments, known = cachedModelAttachments(modelPath)
		}
		if !known && *modelReads > 0 {
			*modelReads--
			attachments, known = modelAttachments(loader, modelPath), true
		}
		for _, attachment := range attachments {
			if attachment.id != 18 || int(attachment.bone) >= len(info.animation.boneMatrices) {
				continue
			}
			matrix := info.animation.boneMatrices[attachment.bone].point
			var local [3]float32
			for row := 0; row < 3; row++ {
				local[row] = matrix[row*4]*attachment.position[0] + matrix[row*4+1]*attachment.position[1] + matrix[row*4+2]*attachment.position[2] + matrix[row*4+3]
			}
			if children := node.Children(); len(children) > 0 {
				offset := children[0].GetNode().Position()
				local[0] += offset.X
				local[1] += offset.Y
				local[2] += offset.Z
			}
			scale := node.Scale()
			point := math32.NewVector3(local[0]*scale.X, local[1]*scale.Y, local[2]*scale.Z)
			rotation := node.Quaternion()
			point.ApplyQuaternion(&rotation)
			return *math32.NewVector3(position.X+point.X, position.Y+point.Y, position.Z+point.Z)
		}
	}
	// No attachment: the top of the model.
	top := float32(1.8)
	if hasInfo && info.modelTop > 0 {
		top = info.modelTop * node.Scale().Y
	}
	return *math32.NewVector3(position.X, position.Y, position.Z+top)
}
