package render

import "testing"

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
