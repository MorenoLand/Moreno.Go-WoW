package network

import (
	"testing"

	"github.com/MorenoLand/Moreno.WoW/auth"
)

func TestSelectRealmMatchesNameAddressAndID(t *testing.T) {
	realms := []auth.Realm{{Name: "Offline", Address: "offline:8085", Flags: 2, ID: 1}, {Name: "Test Realm", Address: "127.0.0.1:8085", ID: 7}}
	for _, selector := range []string{"Test Realm", "127.0.0.1:8085", "7"} {
		realm, err := selectRealm(realms, selector)
		if err != nil {
			t.Fatal(err)
		}
		if realm.ID != 7 {
			t.Fatalf("selector %q selected realm %#v", selector, realm)
		}
	}
}

func TestSelectRealmPrefersAvailableRealm(t *testing.T) {
	realms := []auth.Realm{{Name: "Locked", Flags: 0, Locked: true, ID: 1}, {Name: "Available", ID: 2}}
	realm, err := selectRealm(realms, "")
	if err != nil {
		t.Fatal(err)
	}
	if realm.ID != 2 {
		t.Fatalf("selected realm %#v", realm)
	}
}

func TestSplitEndpointRejectsMalformedPorts(t *testing.T) {
	if host, port, err := splitEndpoint("127.0.0.1", 3724); err != nil || host != "127.0.0.1" || port != 3724 {
		t.Fatalf("default endpoint: %q %d %v", host, port, err)
	}
	if _, _, err := splitEndpoint("127.0.0.1:not-a-port", 3724); err == nil {
		t.Fatal("accepted malformed auth port")
	}
	if host, port, err := splitEndpoint("[::1]:3724", 0); err != nil || host != "::1" || port != 3724 {
		t.Fatalf("IPv6 endpoint: %q %d %v", host, port, err)
	}
}
