package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLiveGlueAccountLoginOpens(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	set, err := openMPQSet(dataPath, "enUS")
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	for _, name := range []string{`Interface\GlueXML\GlueXML.toc`, `Interface\GlueXML\AccountLogin.xml`} {
		data, err := set.ReadFile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("%s empty", name)
		}
		src := filepath.Base(set.files[normalizeMPQPath(name)].archive.path)
		t.Logf("%s ok len=%d from=%s", name, len(data), src)
	}
	loginSrc := filepath.Base(set.files[normalizeMPQPath(`Interface\GlueXML\AccountLogin.xml`)].archive.path)
	if !strings.EqualFold(loginSrc, "patch-4.MPQ") {
		t.Fatalf("AccountLogin.xml from %s, want patch-4.MPQ", loginSrc)
	}
}

func TestClassifyPatchArchiveGenerations(t *testing.T) {
	number, localePatch, ok := classifyPatchArchive("patch-4.MPQ", "enUS")
	if !ok || number != 4 || localePatch {
		t.Fatalf("patch-4 => (%d,%v,%v)", number, localePatch, ok)
	}
	number, localePatch, ok = classifyPatchArchive("patch-enUS-3.MPQ", "enUS")
	if !ok || number != 3 || !localePatch {
		t.Fatalf("patch-enUS-3 => (%d,%v,%v)", number, localePatch, ok)
	}
}

func TestMPQFileKeyUsesBasename(t *testing.T) {
	base := hashString("AccountLogin.xml", hashTypeFileKey)
	full := hashString(normalizeMPQPath(`Interface\GlueXML\AccountLogin.xml`), hashTypeFileKey)
	if base == full {
		t.Fatal("basename and full-path keys must differ")
	}
}

func TestDiscoverMPQPatchPriorityOrder(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "Data")
	locale := filepath.Join(data, "enUS")
	if err := os.MkdirAll(locale, 0o755); err != nil {
		t.Fatal(err)
	}
	touch := func(path string) {
		if err := os.WriteFile(path, []byte("MPQ"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"common.MPQ", "patch.MPQ", "patch-3.MPQ", "patch-4.MPQ"} {
		touch(filepath.Join(data, name))
	}
	for _, name := range []string{"locale-enUS.MPQ", "patch-enUS-3.MPQ"} {
		touch(filepath.Join(locale, name))
	}
	touch(filepath.Join(root, "patch-4.MPQ"))
	paths, err := discoverMPQArchives(data, "enUS")
	if err != nil {
		t.Fatal(err)
	}
	bases := make([]string, 0, len(paths))
	for _, p := range paths {
		bases = append(bases, filepath.Base(p))
	}
	enUS3, patch4 := -1, -1
	for i, b := range bases {
		if strings.EqualFold(b, "patch-enUS-3.MPQ") {
			enUS3 = i
		}
		if strings.EqualFold(b, "patch-4.MPQ") && patch4 < 0 {
			patch4 = i
		}
	}
	if enUS3 < 0 || patch4 < 0 {
		t.Fatalf("missing patches in %v", bases)
	}
	if patch4 < enUS3 {
		t.Fatalf("patch-4 at %d before patch-enUS-3 at %d", patch4, enUS3)
	}
}
