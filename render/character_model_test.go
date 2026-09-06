package render

import (
	"os"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

func TestReplaceCharacterGeosetGroup(t *testing.T) {
	active := map[uint16]bool{0: true, 401: true, 402: true, 501: true}
	available := map[uint16]bool{401: true, 402: true, 403: true, 501: true, 502: true}
	replaceCharacterGeosetGroup(active, 4, 403, available)
	if active[401] || active[402] || !active[403] {
		t.Fatalf("group 4 was not replaced: %v", active)
	}
	replaceCharacterGeosetGroup(active, 5, 502, available)
	if active[501] || !active[502] {
		t.Fatalf("group 5 was not replaced: %v", active)
	}
}

func TestResolveCharacterGeosetDoesNotSubstituteForNone(t *testing.T) {
	if got := resolveCharacterGeoset(801, map[uint16]bool{802: true}); got != 0 {
		t.Fatalf("none fallback=%d", got)
	}
}

func TestNativeDefaultGeosetGroup9IsBareNotKneepad(t *testing.T) {
	// FUN_004DFDA0 stores 0x385 (901) for group 9. 902/903 are equipment variants
	// present on stock models; preferring them draws boot-cuff/kneepad geometry on
	// unequipped characters and flares the calves.
	available := map[uint16]bool{401: true, 501: true, 702: true, 902: true, 903: true, 1301: true}
	if got := resolveCharacterGeoset(901, available); got != 0 {
		t.Fatalf("bare group-9 fallback=%d want 0 when 901 absent", got)
	}
	if got := resolveCharacterGeoset(902, available); got != 902 {
		t.Fatalf("equipment kneepad resolve=%d", got)
	}
}

func TestResolveCharacterGeosetDoesNotPromoteKneepadsFromBareDefault(t *testing.T) {
	available := map[uint16]bool{902: true, 903: true}
	active := map[uint16]bool{0: true, 501: true}
	replaceCharacterGeosetGroup(active, 9, 901, available)
	if active[901] || active[902] || active[903] {
		t.Fatalf("group 9 should stay off for bare default when 901 missing: %v", active)
	}
}

func TestEquipmentSlotForInventoryType(t *testing.T) {
	finger, trinket := 10, 12
	cases := []struct {
		inv  uint8
		want int
	}{
		{4, 3}, {5, 4}, {20, 4}, {7, 6}, {8, 7}, {16, 14}, {17, 15}, {22, 16}, {19, 18}, {0, -1},
	}
	for _, tc := range cases {
		if got := equipmentSlotForInventoryType(tc.inv, &finger, &trinket); got != tc.want {
			t.Fatalf("inv=%d slot=%d want=%d", tc.inv, got, tc.want)
		}
	}
	if got := equipmentSlotForInventoryType(11, &finger, &trinket); got != 10 {
		t.Fatalf("first finger slot=%d", got)
	}
	if got := equipmentSlotForInventoryType(11, &finger, &trinket); got != 11 {
		t.Fatalf("second finger slot=%d", got)
	}
}

func TestLiveCharStartOutfitHumanWarrior(t *testing.T) {
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

	equipment, ok, err := resolveCharStartOutfit(loader, 1, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("human warrior male CharStartOutfit row missing")
	}
	want := map[uint8]uint32{4: 9891, 7: 9892, 8: 10141}
	for inv, display := range want {
		found := false
		for _, item := range equipment {
			if item.InventoryType == inv && item.DisplayID == display {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing create armor inv=%d display=%d equipment=%v", inv, display, equipment)
		}
	}
}

func TestLiveCharacterCreateAppliesStarterArmorTextures(t *testing.T) {
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

	naked := world.Character{Race: 1, Gender: 0}
	create := world.Character{Race: 1, Class: 1, Gender: 0}
	if err := applyCharStartOutfit(loader, &create); err != nil {
		t.Fatal(err)
	}
	if create.Equipment[3].DisplayID == 0 {
		t.Fatal("create warrior missing shirt display from CharStartOutfit")
	}

	skinData, err := loader.ReadFile(`Character\Human\Male\HumanMale00.skin`)
	if err != nil {
		t.Fatal(err)
	}
	skin, err := parseSkin(skinData)
	if err != nil {
		t.Fatal(err)
	}

	nakedSections := characterSectionTextures{}
	if _, err := resolveCharacterEquipment(loader, naked, &nakedSections, skin); err != nil {
		t.Fatal(err)
	}
	createSections := characterSectionTextures{}
	active, err := resolveCharacterEquipment(loader, create, &createSections, skin)
	if err != nil {
		t.Fatal(err)
	}
	if len(createSections.regions) == 0 {
		t.Fatalf("create warrior produced no armor texture regions; equipment=%v", create.Equipment)
	}
	if len(nakedSections.regions) != 0 {
		t.Fatalf("unequipped human unexpectedly gained armor regions=%v", nakedSections.regions)
	}
	if !active[702] {
		t.Fatalf("create warrior lost ear geoset 702: %v", active)
	}
	for id := range active {
		if id/100 == 9 {
			t.Fatalf("create warrior unexpectedly enabled group-9 geoset %d: %v", id, active)
		}
	}
	t.Logf("create warrior armor regions=%d equipment shirt=%d pants=%d boots=%d", len(createSections.regions), create.Equipment[3].DisplayID, create.Equipment[6].DisplayID, create.Equipment[7].DisplayID)
}

func TestLiveCharacterCreateModelLoadsWithStarterArmor(t *testing.T) {
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

	model, err := loadGlueCharacterModel(loader, world.Character{Race: 1, Class: 1, Gender: 0})
	if err != nil {
		t.Fatal(err)
	}
	if model == nil || len(model.Children()) == 0 {
		t.Fatal("create warrior model has no drawable children")
	}
	model.Dispose()
}
