package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRealmlistWTF(t *testing.T) {
	host, ok, err := ParseRealmlistWTF("set realmlist 127.0.0.1")
	if err != nil || !ok || host != "127.0.0.1" {
		t.Fatalf("basic: host=%q ok=%v err=%v", host, ok, err)
	}
	host, ok, err = ParseRealmlistWTF("# comment\nSET realmlist \"logon.example.com\"\n")
	if err != nil || !ok || host != "logon.example.com" {
		t.Fatalf("quoted: host=%q ok=%v err=%v", host, ok, err)
	}
	host, ok, err = ParseRealmlistWTF("set realmlist first\nset realmlist second.host")
	if err != nil || !ok || host != "second.host" {
		t.Fatalf("last wins: host=%q ok=%v err=%v", host, ok, err)
	}
	host, ok, err = ParseRealmlistWTF("// only comments\n")
	if err != nil || ok || host != "" {
		t.Fatalf("empty: host=%q ok=%v err=%v", host, ok, err)
	}
	if _, _, err := ParseRealmlistWTF("set realmlist"); err == nil {
		t.Fatal("expected missing-host error")
	}
}

func TestParseConfigWTFRealmList(t *testing.T) {
	host, ok, err := ParseConfigWTFRealmList(`SET portal "us.logon.worldofwarcraft.com"
SET realmList "127.0.0.1"
SET gxApi "D3D9"`)
	if err != nil || !ok || host != "127.0.0.1" {
		t.Fatalf("config: host=%q ok=%v err=%v", host, ok, err)
	}
}

func TestRealmlistPathAndConfigWTFPath(t *testing.T) {
	got := RealmlistPath(`F:\Games\Wrath of the Lich King\Data`, "enUS")
	want := filepath.Join(`F:\Games\Wrath of the Lich King\Data`, "enUS", "realmlist.wtf")
	if got != want {
		t.Fatalf("RealmlistPath=%q want %q", got, want)
	}
	got = ConfigWTFPath(`F:\Games\Wrath of the Lich King\Data`)
	want = filepath.Join(`F:\Games\Wrath of the Lich King`, "WTF", "Config.wtf")
	if got != want {
		t.Fatalf("ConfigWTFPath=%q want %q", got, want)
	}
	if RealmlistPath("", "enUS") != "" || ConfigWTFPath("") != "" {
		t.Fatal("empty dataPath should yield empty paths")
	}
}

func TestLoadRealmlistAuthAddressPrefersLooseFile(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "Data")
	locale := filepath.Join(data, "enUS")
	if err := os.MkdirAll(locale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "WTF"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "WTF", "Config.wtf"), []byte("SET realmList \"from-config\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locale, "realmlist.wtf"), []byte("set realmlist from-loose\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	host, ok, err := LoadRealmlistAuthAddress(data, "enUS")
	if err != nil || !ok || host != "from-loose" {
		t.Fatalf("prefer loose: host=%q ok=%v err=%v", host, ok, err)
	}
}

func TestLoadRealmlistAuthAddressFallsBackToConfigWTF(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "Data")
	if err := os.MkdirAll(filepath.Join(data, "enUS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "WTF"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "WTF", "Config.wtf"), []byte("SET realmList \"config-only\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	host, ok, err := LoadRealmlistAuthAddress(data, "enUS")
	if err != nil || !ok || host != "config-only" {
		t.Fatalf("config fallback: host=%q ok=%v err=%v", host, ok, err)
	}
}

func TestLoadRealmlistAuthAddressReadsNativeInstall(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	host, ok, err := LoadRealmlistAuthAddress(dataPath, "enUS")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected native Data\\enUS\\realmlist.wtf to set a host")
	}
	if host != "127.0.0.1" {
		t.Fatalf("native realmlist host=%q want 127.0.0.1", host)
	}
	path := RealmlistPath(dataPath, "enUS")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, ok, err := ParseRealmlistWTF(string(raw))
	if err != nil || !ok || parsed != host {
		t.Fatalf("round-trip parse=%q ok=%v err=%v host=%q", parsed, ok, err, host)
	}
}

func TestLoadRealmlistAuthAddressFallsBackToBareFile(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "Data")
	if err := os.MkdirAll(filepath.Join(data, "enUS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "realmlist.wtf"), []byte("set realmlist bare-host\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	host, ok, err := LoadRealmlistAuthAddress(data, "enUS")
	if err != nil || !ok || host != "bare-host" {
		t.Fatalf("bare fallback: host=%q ok=%v err=%v", host, ok, err)
	}
}

func TestLoadRealmlistAuthAddressFallsBackToWTFRealmlist(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "Data")
	if err := os.MkdirAll(filepath.Join(data, "enUS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "WTF"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "WTF", "realmlist.wtf"), []byte("set realmlist wtf-dir-host\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	host, ok, err := LoadRealmlistAuthAddress(data, "enUS")
	if err != nil || !ok || host != "wtf-dir-host" {
		t.Fatalf("WTF\\realmlist fallback: host=%q ok=%v err=%v", host, ok, err)
	}
}
