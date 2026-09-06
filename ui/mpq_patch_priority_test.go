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
	dataRoot, err := findMPQDataRoot(dataPath)
	if err != nil {
		t.Fatal(err)
	}
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
	loginPath := set.files[normalizeMPQPath(`Interface\GlueXML\AccountLogin.xml`)].archive.path
	relative, err := filepath.Rel(dataRoot, loginPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatalf("AccountLogin.xml came from outside Data: %s", loginPath)
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
	nested := filepath.Join(data, "staged")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	touch(filepath.Join(nested, "patch-5.MPQ"))
	paths, err := discoverMPQArchives(data, "enUS")
	if err != nil {
		t.Fatal(err)
	}
	bases := make([]string, 0, len(paths))
	for _, p := range paths {
		bases = append(bases, filepath.Base(p))
	}
	enUS3, patch4, patch5 := -1, -1, -1
	for i, b := range bases {
		if strings.EqualFold(b, "patch-enUS-3.MPQ") {
			enUS3 = i
		}
		if strings.EqualFold(b, "patch-4.MPQ") && patch4 < 0 {
			patch4 = i
		}
		if strings.EqualFold(b, "patch-5.MPQ") && patch5 < 0 {
			patch5 = i
		}
	}
	if enUS3 < 0 || patch4 < 0 || patch5 < 0 {
		t.Fatalf("missing patches in %v", bases)
	}
	if !strings.EqualFold(filepath.Dir(paths[patch4]), data) {
		t.Fatalf("install-root patch-4 was loaded: %s", paths[patch4])
	}
	if patch4 < enUS3 {
		t.Fatalf("patch-4 at %d before patch-enUS-3 at %d", patch4, enUS3)
	}
	if patch5 < patch4 {
		t.Fatalf("nested patch-5 at %d before patch-4 at %d", patch5, patch4)
	}
}

func TestFindMPQDataRootUsesOnlyDataDirectory(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "Data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "patch-4.MPQ"), []byte("MPQ"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "common.MPQ"), []byte("MPQ"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := findMPQDataRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(filepath.Clean(got), filepath.Clean(data)) {
		t.Fatalf("data root=%s want %s", got, data)
	}
	rootOnly := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootOnly, "patch-4.MPQ"), []byte("MPQ"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := findMPQDataRoot(rootOnly); err == nil {
		t.Fatal("install-root MPQ was accepted without a Data directory")
	}
}
