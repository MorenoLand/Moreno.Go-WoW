package ui

import "testing"

func TestGlueAPIMatchesOptionalAmbienceAndAddonSecurity(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	if !rt.Execute("PlayGlueAmbience()", "@api-test.lua") {
		t.Fatalf("optional ambience failed: %v", rt.ScriptErrors())
	}
	rt.Glue.AddOns = []AddonInfo{{Name: "Example"}}
	if !rt.Execute("local _, _, _, _, _, _, security = GetAddOnInfo(1); addonSecurity = security", "@api-test.lua") {
		t.Fatalf("addon info failed: %v", rt.ScriptErrors())
	}
	if got := rt.L.GetGlobal("addonSecurity").String(); got != "INSECURE" {
		t.Fatalf("security=%q", got)
	}
}

func TestCreateLocalizedValueUsesNativeNamesWhenGlobalsAreAbsent(t *testing.T) {
	rt := NewRuntime(nil)
	defer rt.Close()
	for key, want := range map[string]string{"RACE_HUMAN": "Human", "RACE_NIGHTELF": "Night Elf", "RACE_SCOURGE": "Undead", "DEATHKNIGHT": "Death Knight"} {
		if got := createLocalizedValue(rt.L, key); got != want {
			t.Fatalf("%s=%q, want %q", key, got, want)
		}
	}
}
