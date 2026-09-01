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
