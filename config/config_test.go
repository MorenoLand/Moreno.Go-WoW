package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepairDataPathResolvesUniqueTruncatedDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Wrath of the Lich King", "Data"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := repairDataPath(filepath.Join(root, "Wrath"))
	want := filepath.Join(root, "Wrath of the Lich King", "Data")
	if got != want {
		t.Fatalf("repairDataPath=%q want %q", got, want)
	}
}

func TestRepairDataPathLeavesAmbiguousPrefixUntouched(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Wrath One", "Wrath Two"} {
		if err := os.MkdirAll(filepath.Join(root, name, "Data"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "Wrath")
	if got := repairDataPath(path); got != path {
		t.Fatalf("ambiguous repairDataPath=%q want %q", got, path)
	}
}

func TestAudioOptionsPersistCVars(t *testing.T) {
	options := DefaultAudioOptions()
	if !options.SetCVar("Sound_EnableMusic", "0") || options.EnableMusic {
		t.Fatalf("music=%v", options.EnableMusic)
	}
	if !options.SetCVar("Sound_SFXVolume", "0.25") || options.SFXVolume != 0.25 {
		t.Fatalf("sfx volume=%v", options.SFXVolume)
	}
	if options.SetCVar("not-a-sound-cvar", "0") {
		t.Fatal("unknown cvar was persisted")
	}
	if got := options.CVars()["Sound_EnableMusic"]; got != "0" {
		t.Fatalf("music cvar=%q", got)
	}
}

func TestLoadKeepsAudioDefaultsForLegacyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"auth_address":"localhost:3724"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	options, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !options.Audio.EnableMusic || options.Audio.MusicVolume != 1 {
		t.Fatalf("audio defaults=%+v", options.Audio)
	}
}
