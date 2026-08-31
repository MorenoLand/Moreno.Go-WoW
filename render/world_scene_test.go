package render

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
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
	if root == nil || info.chunks != 256 || info.triangles == 0 || info.wmoMeshes == 0 || info.m2Meshes == 0 {
		t.Fatalf("root=%v chunks=%d triangles=%d wmoMeshes=%d m2Meshes=%d", root != nil, info.chunks, info.triangles, info.wmoMeshes, info.m2Meshes)
	}
	t.Logf("world scene: %+v", info)
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
}
