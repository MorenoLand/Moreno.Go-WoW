package render

import (
	"math"
	"os"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

func TestWorldEntityScaleUsesObjectScaleAsDisplayComponent(t *testing.T) {
	entity := &worldEntity{fields: map[uint16]uint32{world.ObjectScaleField: math.Float32bits(1.3)}}
	got := worldEntityScale(entity, 1.3, 1)
	if math.Abs(float64(got-1.3)) > 1e-6 {
		t.Fatalf("scale=%v want 1.3", got)
	}
	entity.fields[world.ObjectScaleField] = math.Float32bits(2)
	got = worldEntityScale(entity, 1.3, 1.5)
	if math.Abs(float64(got-3)) > 1e-6 {
		t.Fatalf("scale=%v want 3", got)
	}
	entity = &worldEntity{fields: map[uint16]uint32{}}
	got = worldEntityScale(entity, 1.3, 2)
	if math.Abs(float64(got-2.6)) > 1e-6 {
		t.Fatalf("missing object scale fallback=%v want 2.6", got)
	}
}

func TestLiveWorldCreatureKeepsNativeScaleAndExtraGeosets(t *testing.T) {
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

	var tables worldCreatureTables
	def, err := tables.definition(loader, 2575, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !def.extra.ok {
		t.Fatal("display 2575 should resolve CreatureDisplayInfoExtra")
	}
	if def.bake == "" {
		t.Fatal("display 2575 missing bake name")
	}
	var hasItem bool
	for _, id := range def.extra.items {
		if id != 0 {
			hasItem = true
			break
		}
	}
	if !hasItem {
		t.Fatal("display 2575 Extra row has no item displays")
	}

	model, err := buildWorldCreatureModel(loader, def)
	if err != nil {
		t.Fatal(err)
	}
	info, ok := model.UserData().(glueModelInfo)
	if !ok {
		t.Fatal("missing glueModelInfo")
	}
	if math.Abs(float64(info.modelScale-1)) > 1e-5 {
		t.Fatalf("world creature normalized to UI scale=%v; want native 1", info.modelScale)
	}

	skinData, err := loader.ReadFile(worldM2SkinPath(def.path))
	if err != nil {
		t.Fatal(err)
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		t.Fatal(err)
	}
	active, cape := resolveWorldCreatureExtraGeosets(loader, def.extra, skin)
	if !active[0] {
		t.Fatalf("extra geosets missing body: %v", active)
	}
	if def.extra.race == 4 && !active[702] {
		t.Fatalf("night elf NPC missing ear geoset 702: %v", active)
	}
	t.Logf("display 2575 active=%v cape=%q displayScale=%v modelScale=%v", active, cape, def.displayScale, def.modelScale)
}

func TestLiveWorldCreatureStagUsesDisplayScaleWithoutNormalize(t *testing.T) {
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
	var tables worldCreatureTables
	def, err := tables.definition(loader, 2161, 0)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(def.displayScale-1.3)) > 1e-4 {
		t.Fatalf("stag displayScale=%v want 1.3", def.displayScale)
	}
	model, err := buildWorldCreatureModel(loader, def)
	if err != nil {
		t.Fatal(err)
	}
	info := model.UserData().(glueModelInfo)
	if math.Abs(float64(info.modelScale-1)) > 1e-5 {
		t.Fatalf("stag UI-normalized scale=%v", info.modelScale)
	}
	entity := &worldEntity{fields: map[uint16]uint32{}}
	if got := worldEntityScale(entity, def.displayScale, def.modelScale); math.Abs(float64(got-1.3)) > 1e-4 {
		t.Fatalf("stag entity scale=%v", got)
	}
}
