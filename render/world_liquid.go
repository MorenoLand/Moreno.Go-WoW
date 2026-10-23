package render

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/texture"
)

// The MH2O chunk of a 3.3.5a ADT states the liquid of the whole tile once: 256
// fixed headers (one per map chunk, in MCIN order) each naming that chunk's
// liquid instances. Every offset is measured from the start of the MH2O
// payload. An instance is a sub-rectangle of the chunk's 8x8 cell grid with
// (width+1)*(height+1) vertices.
const (
	worldLiquidHeaderSize   = 12
	worldLiquidInstanceSize = 24
	worldLiquidFrameRate    = 18.0
	worldLiquidMaxFrames    = 60
	// worldLiquidFullDepth is the depth byte at which a surface is fully opaque;
	// the file stores depth, not alpha, and the deepest river bytes stay small.
	worldLiquidFullDepth = 12.0
	// worldLiquidMaterialProcedural is the LiquidType.dbc material of water drawn by
	// the procedural water shader: its frames are height maps, not colour.
	worldLiquidMaterialProcedural = 3
)

// Vertex formats of an MH2O instance.
const (
	worldLiquidHeightDepth   = 0
	worldLiquidHeightUV      = 1
	worldLiquidDepthOnly     = 2
	worldLiquidHeightUVDepth = 3
)

// LiquidType.dbc category column: what the liquid is rather than what it is called.
const (
	worldLiquidWater = 0
	worldLiquidOcean = 1
	worldLiquidMagma = 2
	worldLiquidSlime = 3
)

type worldLiquidSheet struct {
	liquidType uint16
	format     uint16
	minHeight  float32
	maxHeight  float32
	xOffset    int
	yOffset    int
	width      int
	height     int
	heights    []float32
	exists     []bool
	depths     []byte
}

func worldLiquidBitmapLen(width, height int) int {
	return (width*height + 7) / 8
}

func parseWorldLiquids(payload []byte) [256][]worldLiquidSheet {
	var result [256][]worldLiquidSheet
	u32 := func(at int) int {
		if at < 0 || at+4 > len(payload) {
			return 0
		}
		return int(binary.LittleEndian.Uint32(payload[at : at+4]))
	}
	for index := 0; index < 256; index++ {
		header := index * worldLiquidHeaderSize
		if header+worldLiquidHeaderSize > len(payload) {
			break
		}
		offset, count := u32(header), u32(header+4)
		if count <= 0 || count > 16 {
			continue
		}
		for layer := 0; layer < count; layer++ {
			at := offset + layer*worldLiquidInstanceSize
			if at < 0 || at+worldLiquidInstanceSize > len(payload) {
				break
			}
			if sheet, ok := parseWorldLiquidSheet(payload, at); ok {
				result[index] = append(result[index], sheet)
			}
		}
	}
	return result
}

func parseWorldLiquidSheet(payload []byte, at int) (worldLiquidSheet, bool) {
	sheet := worldLiquidSheet{
		liquidType: binary.LittleEndian.Uint16(payload[at:]),
		format:     binary.LittleEndian.Uint16(payload[at+2:]),
		minHeight:  math.Float32frombits(binary.LittleEndian.Uint32(payload[at+4:])),
		maxHeight:  math.Float32frombits(binary.LittleEndian.Uint32(payload[at+8:])),
		xOffset:    int(payload[at+12]),
		yOffset:    int(payload[at+13]),
		width:      int(payload[at+14]),
		height:     int(payload[at+15]),
	}
	existsOffset := int(binary.LittleEndian.Uint32(payload[at+16:]))
	vertexOffset := int(binary.LittleEndian.Uint32(payload[at+20:]))
	if sheet.width > 8 || sheet.height > 8 || sheet.xOffset+sheet.width > 8 || sheet.yOffset+sheet.height > 8 {
		return worldLiquidSheet{}, false
	}
	cells := sheet.width * sheet.height
	if existsOffset != 0 && cells > 0 {
		end := existsOffset + worldLiquidBitmapLen(sheet.width, sheet.height)
		if existsOffset < 0 || end > len(payload) {
			return worldLiquidSheet{}, false
		}
		bits := payload[existsOffset:end]
		sheet.exists = make([]bool, cells)
		for cell := range sheet.exists {
			sheet.exists[cell] = bits[cell/8]&(1<<uint(cell%8)) != 0
		}
	}
	vertices := (sheet.width + 1) * (sheet.height + 1)
	hasHeights := sheet.format == worldLiquidHeightDepth || sheet.format == worldLiquidHeightUV || sheet.format == worldLiquidHeightUVDepth
	if vertexOffset != 0 && hasHeights {
		end := vertexOffset + vertices*4
		if vertexOffset < 0 || end > len(payload) {
			return worldLiquidSheet{}, false
		}
		sheet.heights = make([]float32, vertices)
		for v := range sheet.heights {
			sheet.heights[v] = math.Float32frombits(binary.LittleEndian.Uint32(payload[vertexOffset+v*4:]))
		}
	}
	if vertexOffset != 0 {
		before := -1
		switch sheet.format {
		case worldLiquidHeightDepth:
			before = 4
		case worldLiquidDepthOnly:
			before = 0
		case worldLiquidHeightUVDepth:
			before = 8
		}
		if before >= 0 {
			start := vertexOffset + before*vertices
			if start >= 0 && start+vertices <= len(payload) {
				sheet.depths = append([]byte(nil), payload[start:start+vertices]...)
			}
		}
	}
	return sheet, true
}

func (s worldLiquidSheet) cellExists(i, j int) bool {
	if i < 0 || j < 0 || i >= s.width || j >= s.height {
		return false
	}
	if len(s.exists) == 0 {
		return true
	}
	return s.exists[j*s.width+i]
}

func (s worldLiquidSheet) vertexHeight(i, j int) float32 {
	if len(s.heights) == 0 {
		return s.minHeight
	}
	index := j*(s.width+1) + i
	if index < 0 || index >= len(s.heights) {
		return s.minHeight
	}
	return s.heights[index]
}

// vertexAlphaScale is how opaque the sheet is at a vertex: the file stores a
// depth that is 0 at the waterline, and a sheet without depths is opaque.
func (s worldLiquidSheet) vertexAlphaScale(i, j int) float32 {
	if len(s.depths) == 0 {
		return 1
	}
	index := j*(s.width+1) + i
	if index < 0 || index >= len(s.depths) {
		return 1
	}
	scale := float32(s.depths[index]) / worldLiquidFullDepth
	if scale > 1 {
		scale = 1
	}
	return scale
}

// heightAt is the surface height at a world position, or false where the
// sheet does not reach. Both axes run inwards (negative) from the chunk
// origin like the terrain's; xOffset indexes the world-y axis and yOffset the
// world-x axis.
func (s worldLiquidSheet) heightAt(origin [3]float32, x, y float32) (float32, bool) {
	const tolerance = 0.01
	major := (origin[0] - x) / worldUnitSize
	minor := (origin[1] - y) / worldUnitSize
	if major < -tolerance || major > 8+tolerance || minor < -tolerance || minor > 8+tolerance {
		return 0, false
	}
	localMajor := clampFloat32(major, 0, 8) - float32(s.yOffset)
	localMinor := clampFloat32(minor, 0, 8) - float32(s.xOffset)
	if localMajor < -tolerance || localMinor < -tolerance || localMajor > float32(s.height)+tolerance || localMinor > float32(s.width)+tolerance {
		return 0, false
	}
	localMajor = clampFloat32(localMajor, 0, float32(s.height))
	localMinor = clampFloat32(localMinor, 0, float32(s.width))
	cellJ := int(math.Floor(float64(localMajor)))
	if cellJ > s.height-1 {
		cellJ = s.height - 1
	}
	cellI := int(math.Floor(float64(localMinor)))
	if cellI > s.width-1 {
		cellI = s.width - 1
	}
	if !s.cellExists(cellI, cellJ) {
		return 0, false
	}
	sv, tv := localMajor-float32(cellJ), localMinor-float32(cellI)
	h00, h10 := s.vertexHeight(cellI, cellJ), s.vertexHeight(cellI, cellJ+1)
	h01, h11 := s.vertexHeight(cellI+1, cellJ), s.vertexHeight(cellI+1, cellJ+1)
	top := h00 + (h01-h00)*tv
	bottom := h10 + (h11-h10)*tv
	return top + (bottom-top)*sv, true
}

func clampFloat32(value, low, high float32) float32 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// liquidAt is the water surface at a position for the swim test: the lowest
// surface at or above the feet, else the highest one below them.
func (collision worldSceneCollision) liquidAt(x, y, z float32) (float32, bool) {
	cellX := int(math.Floor(float64(x / float32(worldChunkSize))))
	cellY := int(math.Floor(float64(y / float32(worldChunkSize))))
	best, found := float32(0), false
	above := false
	for offsetY := -1; offsetY <= 1; offsetY++ {
		for offsetX := -1; offsetX <= 1; offsetX++ {
			for _, chunkIndex := range collision.terrainCells[[2]int{cellX + offsetX, cellY + offsetY}] {
				chunk := collision.chunks[chunkIndex]
				for _, sheet := range chunk.liquids {
					surface, ok := sheet.heightAt(chunk.position, x, y)
					if !ok {
						continue
					}
					isAbove := surface >= z-0.5
					switch {
					case !found:
					case isAbove && !above:
					case isAbove == above && isAbove && surface < best:
					case isAbove == above && !isAbove && surface > best:
					default:
						continue
					}
					best, found, above = surface, true, isAbove
				}
			}
		}
	}
	return best, found
}

// worldLiquidType is the LiquidType.dbc row a sheet draws with.
type worldLiquidType struct {
	category uint32
	material uint32
	// texture is the first LiquidType texture that names an animation (it holds
	// a "%d" for the frame number).
	texture string
}

func readWorldLiquidTypes(loader *ui.Loader) map[uint32]worldLiquidType {
	result := make(map[uint32]worldLiquidType)
	data, err := loader.ReadFile(`DBFilesClient\LiquidType.dbc`)
	if err != nil || len(data) < 20 || string(data[:4]) != "WDBC" {
		return result
	}
	records := int(binary.LittleEndian.Uint32(data[4:8]))
	fields := int(binary.LittleEndian.Uint32(data[8:12]))
	stride := int(binary.LittleEndian.Uint32(data[12:16]))
	stringSize := int(binary.LittleEndian.Uint32(data[16:20]))
	stringStart := 20 + records*stride
	if fields < 16 || stride < fields*4 || stringStart+stringSize > len(data) {
		return result
	}
	readString := func(offset uint32) string {
		start := int(offset)
		if start <= 0 || start >= stringSize {
			return ""
		}
		end := start
		for end < stringSize && data[stringStart+end] != 0 {
			end++
		}
		return string(data[stringStart+start : stringStart+end])
	}
	for record := 0; record < records; record++ {
		base := 20 + record*stride
		field := func(column int) uint32 { return binary.LittleEndian.Uint32(data[base+column*4:]) }
		row := worldLiquidType{category: field(3), material: field(14)}
		for column := 15; column < 21; column++ {
			if name := readString(field(column)); strings.Contains(name, "%d") {
				row.texture = strings.ReplaceAll(name, "/", `\`)
				break
			}
		}
		result[field(0)] = row
	}
	return result
}

// worldLiquidLook is how a category is tinted. Alpha-keyed art (water, ocean)
// stores no colour of its own, so the tint supplies it.
type worldLiquidLook struct {
	tint  [3]float32
	alpha float32
}

func worldLiquidLookFor(category uint32) worldLiquidLook {
	switch category {
	case worldLiquidOcean:
		return worldLiquidLook{tint: [3]float32{0.42, 0.62, 0.76}, alpha: 0.86}
	case worldLiquidMagma:
		return worldLiquidLook{tint: [3]float32{1, 1, 1}, alpha: 1}
	case worldLiquidSlime:
		return worldLiquidLook{tint: [3]float32{0.85, 1, 0.7}, alpha: 1}
	}
	return worldLiquidLook{tint: [3]float32{0.62, 0.78, 0.92}, alpha: 0.72}
}

// worldLiquidArt is the animation of one liquid type: the frames of its
// texture pattern, played at a fixed rate.
type worldLiquidArt struct {
	frames     []*texture.Texture2D
	look       worldLiquidLook
	procedural bool
	// water is 0 for river and 1 for ocean colours, -1 for surfaces that are not
	// procedural water.
	water int
}

type worldLiquidMesh struct {
	mesh  *graphic.Mesh
	mat   *worldMaterial
	art   *worldLiquidArt
	frame int
}

// worldLiquidAnimator owns the liquid art and steps every liquid mesh to the
// frame the clock names; frames are played, not blended.
type worldLiquidAnimator struct {
	mu     sync.Mutex
	types  map[uint32]worldLiquidType
	art    map[uint16]*worldLiquidArt
	meshes []*worldLiquidMesh
	loaded bool
}

func newWorldLiquidAnimator() *worldLiquidAnimator {
	return &worldLiquidAnimator{art: make(map[uint16]*worldLiquidArt)}
}

func (a *worldLiquidAnimator) artFor(loader *ui.Loader, liquidType uint16) *worldLiquidArt {
	if art, ok := a.art[liquidType]; ok {
		return art
	}
	if !a.loaded {
		a.types, a.loaded = readWorldLiquidTypes(loader), true
	}
	row, ok := a.types[uint32(liquidType)]
	art := &worldLiquidArt{look: worldLiquidLookFor(row.category), procedural: ok && row.material == worldLiquidMaterialProcedural}
	art.water = -1
	if art.procedural {
		art.water = 0
		if row.category == worldLiquidOcean {
			art.water = 1
		}
	}
	if ok && row.texture != "" {
		for frame := 1; frame <= worldLiquidMaxFrames; frame++ {
			path := strings.ReplaceAll(row.texture, "%d", strconv.Itoa(frame))
			tex := loadModelTexture(loader, path)
			if tex == nil {
				break
			}
			tex.SetWrapS(gls.REPEAT)
			tex.SetWrapT(gls.REPEAT)
			art.frames = append(art.frames, tex)
			if !strings.Contains(row.texture, "%d") {
				break
			}
		}
	}
	a.art[liquidType] = art
	return art
}

// update shows the frame for a clock in seconds on every live liquid mesh.
func (a *worldLiquidAnimator) update(seconds float64) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	step := int(math.Max(0, seconds) * worldLiquidFrameRate)
	live := a.meshes[:0]
	for _, entry := range a.meshes {
		if entry.mesh.Parent() == nil {
			continue
		}
		live = append(live, entry)
		if len(entry.art.frames) == 0 {
			continue
		}
		frame := step % len(entry.art.frames)
		if frame == entry.frame {
			continue
		}
		entry.mat.RemoveTexture(entry.art.frames[entry.frame])
		entry.mat.AddTexture(entry.art.frames[frame])
		entry.frame = frame
	}
	a.meshes = live
}

type worldLiquidBuilder struct {
	art       *worldLiquidArt
	positions math32.ArrayF32
	uvs       math32.ArrayF32
	colors    math32.ArrayF32
	extra     math32.ArrayF32
	indices   math32.ArrayU32
}

// buildWorldLiquids makes one mesh per liquid type for a tile.
func buildWorldLiquids(loader *ui.Loader, chunks []worldADTChunk, animator *worldLiquidAnimator) []*graphic.Mesh {
	builders := make(map[uint16]*worldLiquidBuilder)
	var order []uint16
	for _, chunk := range chunks {
		for _, sheet := range chunk.liquids {
			builder := builders[sheet.liquidType]
			if builder == nil {
				art := animator.artFor(loader, sheet.liquidType)
				if len(art.frames) == 0 {
					continue
				}
				builder = &worldLiquidBuilder{art: art}
				builders[sheet.liquidType] = builder
				order = append(order, sheet.liquidType)
			}
			builder.addSheet(chunk, sheet)
		}
	}
	var meshes []*graphic.Mesh
	for _, liquidType := range order {
		builder := builders[liquidType]
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

func (b *worldLiquidBuilder) addSheet(chunk worldADTChunk, sheet worldLiquidSheet) {
	base := uint32(len(b.positions) / 3)
	stride := sheet.width + 1
	mode := float32(0)
	if b.art.procedural {
		mode = 2
	}
	// Texture coordinates come from the world position so neighbouring chunks
	// tile seamlessly; the pattern repeats every four cells.
	repeat := float32(4 * worldUnitSize)
	for j := 0; j <= sheet.height; j++ {
		for i := 0; i <= sheet.width; i++ {
			x := chunk.position[0] - float32(sheet.yOffset+j)*worldUnitSize
			y := chunk.position[1] - float32(sheet.xOffset+i)*worldUnitSize
			b.positions.Append(x, y, sheet.vertexHeight(i, j))
			b.uvs.Append(-y/repeat, -x/repeat)
			b.colors.Append(b.art.look.tint[0], b.art.look.tint[1], b.art.look.tint[2])
			value := b.art.look.alpha
			if b.art.procedural {
				value = sheet.vertexAlphaScale(i, j)
			}
			b.extra.Append(value, mode)
		}
	}
	for j := 0; j < sheet.height; j++ {
		for i := 0; i < sheet.width; i++ {
			if !sheet.cellExists(i, j) {
				continue
			}
			a := base + uint32(j*stride+i)
			c := base + uint32((j+1)*stride+i)
			b.indices.Append(a, a+1, c, a+1, c+1, c)
		}
	}
}

const worldLiquidVertexShader = `#include <attributes>
in vec2 VertexTexcoord2;
uniform mat4 MVP;
` + worldFogVertexDeclarations + `out vec2 FragTexcoord;
out vec3 FragTint;
out vec2 FragLiquid;
out vec3 FragViewPosition;
void main() {
    FragTexcoord = VertexTexcoord;
    FragTint = VertexColor;
    FragLiquid = VertexTexcoord2;
    FragViewPosition = (ModelViewMatrix * vec4(VertexPosition, 1.0)).xyz;
` + worldFogVertexBody + `    gl_Position = MVP * vec4(VertexPosition, 1.0);
}`

// Lava and slime are ordinary colour textures. Procedural water (LiquidType
// material 3) animates a height map: its slope tilts the surface normal and
// the angle to the viewer blends the water colour toward the haze. The colour
// runs from the light's shallow to its deep water colour with depth
// (FragLiquid.x), and so does the opacity.
const worldLiquidFragmentShader = `precision highp float;
uniform sampler2D MatTexture[MAT_TEXTURES];
uniform mat4 ModelViewMatrix;
uniform vec3 WaterShallow;
uniform vec3 WaterDeep;
uniform vec4 WaterAlpha;
` + worldFogFragmentDecl + `in vec2 FragTexcoord;
in vec3 FragTint;
in vec2 FragLiquid;
in vec3 FragViewPosition;
out vec4 FragColor;
void main() {
    if (FragLiquid.y < 1.5) {
        vec4 surface = texture(MatTexture[0], FragTexcoord);
        FragColor = vec4(surface.rgb * FragTint, FragLiquid.x);
` + worldFogApply + `        return;
    }
    const float pixel = 1.0 / 256.0;
    float h = texture(MatTexture[0], FragTexcoord).r;
    float du = texture(MatTexture[0], FragTexcoord + vec2(pixel, 0.0)).r - h;
    float dv = texture(MatTexture[0], FragTexcoord + vec2(0.0, pixel)).r - h;
    // u runs along -y and v along -x, so the world gradient is (-dh/dv, -dh/du)
    // and the normal leans against it.
    vec3 normal = normalize(mat3(ModelViewMatrix) * normalize(vec3(dv * 14.0, du * 14.0, 1.0)));
    vec3 view = normalize(-FragViewPosition);
    float fresnel = pow(1.0 - max(dot(normal, view), 0.0), 3.0);
    float depth = FragLiquid.x;
    vec3 base = mix(WaterShallow, WaterDeep, depth);
    float opacity = mix(WaterAlpha.x, WaterAlpha.y, depth);
    if (WaterAlpha.z < 0.5) {
        base = FragTint * 0.45;
        opacity = 0.72;
    }
    vec3 haze = WaterAlpha.z > 0.5 ? FogColor : vec3(0.52, 0.62, 0.72);
    vec3 color = mix(base, haze, clamp(0.1 + 0.9 * fresnel, 0.0, 1.0));
    FragColor = vec4(color, mix(opacity, 1.0, fresnel * 0.6));
` + worldFogApply + `}`

// drop forgets the liquid meshes of a tile that has been released.
func (a *worldLiquidAnimator) drop(released map[*graphic.Mesh]bool) {
	if a == nil || len(released) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	live := a.meshes[:0]
	for _, entry := range a.meshes {
		if !released[entry.mesh] {
			live = append(live, entry)
		}
	}
	a.meshes = live
}
