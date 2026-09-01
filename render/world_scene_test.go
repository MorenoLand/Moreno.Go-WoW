package render

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
	"github.com/g3n/engine/math32"
)

func TestWorldTileAtUsesSwappedInvertedAxes(t *testing.T) {
	if x, y := worldTileAt(0, 0); x != 32 || y != 32 {
		t.Fatalf("origin tile=%d,%d", x, y)
	}
	if x, y := worldTileAt(-8897.333, -178.333); x != 32 || y != 48 {
		t.Fatalf("Northshire tile=%d,%d", x, y)
	}
}

func TestLiveWorldSceneBuild(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	root, info, err := loadWorldTerrain(loader, world.WorldPosition{Map: 0, X: -8897.0, Y: -178.3, Z: 80})
	if err != nil {
		t.Fatal(err)
	}
	if root == nil || info.tiles < 1 || info.chunks < 256 || info.triangles == 0 || info.wmoMeshes == 0 || info.m2Meshes == 0 {
		t.Fatalf("root=%v tiles=%d chunks=%d triangles=%d wmoMeshes=%d m2Meshes=%d", root != nil, info.tiles, info.chunks, info.triangles, info.wmoMeshes, info.m2Meshes)
	}
	t.Logf("world scene: %+v", info)
	loadingPath := worldLoadingScreenPath(loader, 0)
	if loadingPath == "" {
		t.Fatal("map 0 has no loading screen")
	}
	if _, err := loader.ReadAsset(loadingPath); err != nil {
		t.Fatalf("loading screen %s: %v", loadingPath, err)
	}
	t.Logf("loading screen: %s", loadingPath)
}

func TestLiveWorldPlayerBuild(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	player, err := buildWorldPlayer(loader, world.Character{Race: 4, Gender: 1}, world.WorldPosition{X: -9098, Y: 413, Z: 94})
	if err != nil {
		t.Fatal(err)
	}
	if player == nil {
		t.Fatal("world player is nil")
	}
	if len(player.Children()) == 0 {
		t.Fatal("world player has no drawable children")
	}
	t.Logf("world player children=%d position=%v", len(player.Children()), player.Position())
	parts, err := loadWorldM2Parts(loader, `Character\NightElf\Female\NightElfFemale.m2`)
	if err != nil {
		t.Fatal(err)
	}
	for key, part := range parts {
		t.Logf("world player part %s textures=%v blend=%d vertices=%d", key, part.texturePaths, part.material.blend, len(part.positions)/3)
	}
	player.Dispose()
}

func TestLiveAzerothADT(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	mapData, err := loader.ReadFile(`DBFilesClient\Map.dbc`)
	if err != nil {
		t.Fatal(err)
	}
	if got := parseMapNames(mapData)[0]; got != "Azeroth" {
		t.Fatalf("map 0=%q", got)
	}
	data, err := loader.ReadFile(`World\Maps\Azeroth\Azeroth_32_48.adt`)
	if err != nil {
		t.Fatal(err)
	}
	adt, err := parseWorldADT(data)
	if err != nil {
		t.Fatal(err)
	}
	if adt.version != 18 || len(adt.chunks) != 256 || len(adt.textures) == 0 {
		t.Fatalf("version=%d chunks=%d textures=%d", adt.version, len(adt.chunks), len(adt.textures))
	}
	if len(adt.m2Names) == 0 || len(adt.m2Placements) == 0 {
		t.Fatalf("M2 names=%d placements=%d", len(adt.m2Names), len(adt.m2Placements))
	}
	if len(adt.wmoNames) == 0 || len(adt.wmoPlacements) == 0 {
		t.Fatalf("WMO names=%d placements=%d", len(adt.wmoNames), len(adt.wmoPlacements))
	}
	t.Logf("Azeroth placements M2[0]=%v WMO[0]=%v", adt.m2Placements[0].position, adt.wmoPlacements[0].position)
	layeredChunks, alphaMaps := 0, 0
	for _, chunk := range adt.chunks {
		if len(chunk.layers) > 1 {
			layeredChunks++
		}
		for _, alpha := range chunk.alphaMaps {
			if len(alpha) == 64*64 {
				alphaMaps++
			}
		}
	}
	if layeredChunks == 0 || alphaMaps == 0 {
		t.Fatalf("layered chunks=%d alpha maps=%d", layeredChunks, alphaMaps)
	}
	foundWMO := false
	for _, placement := range adt.wmoPlacements {
		if !strings.Contains(placement.path, `NSABBEY\NSABBEY.WMO`) {
			continue
		}
		rootData, err := loader.ReadFile(placement.path)
		if err != nil {
			t.Fatal(err)
		}
		root, err := parseWorldWMORoot(rootData)
		if err != nil {
			t.Fatal(err)
		}
		if root.groupCount == 0 || len(root.materials) == 0 {
			t.Fatalf("WMO groups=%d materials=%d", root.groupCount, len(root.materials))
		}
		t.Logf("NSABBEY doodad sets=%d doodads=%d", len(root.doodadSets), len(root.doodads))
		missingMaterials := 0
		blendCounts := make(map[uint32]int)
		for _, material := range root.materials {
			blendCounts[material.blend]++
			if material.texture == "" {
				continue
			}
			if _, readErr := loader.ReadAsset(material.texture); readErr != nil {
				missingMaterials++
				t.Logf("NSABBEY missing material texture %s: %v", material.texture, readErr)
			}
		}
		t.Logf("NSABBEY materials=%d missingTextures=%d blendModes=%v", len(root.materials), missingMaterials, blendCounts)
		foundGroup := false
		for index := 0; index < root.groupCount; index++ {
			groupData, readErr := loader.ReadFile(worldWMOGroupPath(placement.path, index))
			if readErr != nil {
				continue
			}
			group, parseErr := parseWorldWMOGroup(groupData)
			if parseErr == nil && len(group.vertices) > 0 && len(group.indices) > 0 {
				foundGroup = true
				break
			}
		}
		if !foundGroup {
			t.Fatal("NSABBEY has no renderable group")
		}
		foundWMO = true
		break
	}
	if !foundWMO {
		t.Fatal("Azeroth test tile has no NSABBEY placement")
	}
	point := worldHeightPoint(adt.chunks[0], 0)
	if math.IsNaN(float64(point[2])) || math.IsInf(float64(point[2]), 0) {
		t.Fatalf("first terrain height is not finite at %v", point)
	}
	t.Logf("Azeroth tile chunk0 grid=%d,%d rawPosition=%v worldPoint0=%v", adt.chunks[0].gridX, adt.chunks[0].gridY, adt.chunks[0].position, point)
}

func TestLiveAzerothNeighborWMOInventory(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	data, err := loader.ReadFile(`World\Maps\Azeroth\Azeroth_31_49.adt`)
	if err != nil {
		t.Fatal(err)
	}
	adt, err := parseWorldADT(data)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, placement := range adt.wmoPlacements {
		if strings.Contains(strings.ToUpper(placement.path), `STORMWIND.WMO`) {
			origin := worldWMOPosition(placement.position)
			t.Logf("Stormwind placement raw=%v world=%v set=%d distance=%.1f", placement.position, origin, placement.doodadSet, math.Hypot(float64(origin[0]+9098), float64(origin[1]-413)))
		}
		if seen[placement.path] {
			continue
		}
		seen[placement.path] = true
		rootData, readErr := loader.ReadFile(placement.path)
		if readErr != nil {
			continue
		}
		root, parseErr := parseWorldWMORoot(rootData)
		if parseErr != nil {
			continue
		}
		blendModes := make(map[uint32]int)
		for _, material := range root.materials {
			blendModes[material.blend]++
		}
		if strings.Contains(strings.ToUpper(placement.path), `STORMWIND.WMO`) {
			for index := 0; index < len(root.doodads) && index < 10; index++ {
				t.Logf("Stormwind doodad %d path=%s scale=%f", index, root.doodads[index].path, root.doodads[index].scale)
			}
		}
		t.Logf("neighbor WMO %s groups=%d materials=%d sets=%d doodads=%d blendModes=%v", placement.path, root.groupCount, len(root.materials), len(root.doodadSets), len(root.doodads), blendModes)
	}
}

func TestLiveAzerothM2AxisInventory(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader, err := ui.NewMPQLoader(dataPath, "enUS", rt)
	if err != nil {
		t.Fatal(err)
	}
	defer loader.Close()
	data, err := loader.ReadFile(`World\Maps\Azeroth\Azeroth_31_49.adt`)
	if err != nil {
		t.Fatal(err)
	}
	adt, err := parseWorldADT(data)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	count := 0
	for _, placement := range adt.m2Placements {
		if strings.Contains(strings.ToUpper(placement.path), "FENCE") {
			t.Logf("M2 placement fence path=%s position=%v rotation=%v scale=%f", placement.path, placement.position, placement.rotation, placement.scale)
			parts, loadErr := loadWorldM2Parts(loader, placement.path)
			if loadErr == nil {
				min, max := math32.NewVector3(math.MaxFloat32, math.MaxFloat32, math.MaxFloat32), math32.NewVector3(-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32)
				rotation := worldM2Rotation(placement.rotation)
				for _, part := range parts {
					for index := 0; index+2 < len(part.positions); index += 3 {
						point := math32.NewVector3(part.positions[index], part.positions[index+1], part.positions[index+2]).ApplyQuaternion(rotation)
						min.Min(point)
						max.Max(point)
					}
				}
				t.Logf("M2 transformed fence bounds=%v..%v", min, max)
			}
			break
		}
	}
	for _, path := range adt.m2Names {
		upper := strings.ToUpper(path)
		if seen[path] || (!strings.Contains(upper, "TREE") && !strings.Contains(upper, "FENCE")) {
			continue
		}
		seen[path] = true
		modelData, readErr := loader.ReadFile(normalizeModelPath(path))
		if readErr != nil {
			continue
		}
		model, parseErr := parseM2(modelData)
		if parseErr != nil || len(model.vertices) == 0 {
			continue
		}
		min, max := model.vertices[0].position, model.vertices[0].position
		for _, vertex := range model.vertices[1:] {
			for axis := 0; axis < 3; axis++ {
				min[axis] = float32(math.Min(float64(min[axis]), float64(vertex.position[axis])))
				max[axis] = float32(math.Max(float64(max[axis]), float64(vertex.position[axis])))
			}
		}
		t.Logf("M2 axis %s rawBounds=%v..%v", path, min, max)
		count++
		if count == 8 {
			break
		}
	}
	if count == 0 {
		t.Fatal("tile has no readable tree/fence M2")
	}
}

func TestWorldM2RotationPreservesVerticalAxis(t *testing.T) {
	convertedUp := math32.NewVector3(0, 0, 1)
	convertedUp.ApplyQuaternion(worldM2Rotation([3]float32{1, 121.5, 4}))
	if math.Abs(float64(convertedUp.X)) > 0.1 || math.Abs(float64(convertedUp.Y)) > 0.1 || convertedUp.Z < 0.9 {
		t.Fatalf("fence up axis transformed to %v", convertedUp)
	}
}
