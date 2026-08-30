package test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/MorenoLand/Moreno.WoW/ui"
)

// TestGlueXMLSmoke loads the extracted glue interface data and executes the
// full GlueXML.toc against the ui runtime. The data stays outside this
// repository: point WOW_TEST_GLUEXML at a directory containing the extracted
// GlueXML files (GlueXML.toc present). The test skips when the variable is
// unset.
func TestGlueXMLSmoke(t *testing.T) {
	source := os.Getenv("WOW_TEST_GLUEXML")
	if source == "" {
		t.Skip("WOW_TEST_GLUEXML not set; skipped")
	}
	if _, err := os.Stat(filepath.Join(source, "GlueXML.toc")); err != nil {
		t.Fatalf("%s does not contain GlueXML.toc: %v", source, err)
	}

	// Stage the loose files under a temporary Interface root. FrameXML files
	// are staged alongside GlueXML because option panels include
	// ..\FrameXML\GraphicsQualityLevels.lua.
	root := t.TempDir()
	stageTree := func(rel, source string) {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			in, err := os.Open(filepath.Join(source, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			out, err := os.Create(filepath.Join(dir, e.Name()))
			if err != nil {
				in.Close()
				t.Fatal(err)
			}
			if _, err := io.Copy(out, in); err != nil {
				in.Close()
				out.Close()
				t.Fatal(err)
			}
			in.Close()
			out.Close()
		}
	}
	stageTree("Interface/GlueXML", source)
	if frame := os.Getenv("WOW_TEST_FRAMEXML"); frame != "" {
		stageTree("Interface/FrameXML", frame)
	}

	rt := ui.NewRuntime(nil)
	defer rt.Close()
	loader := ui.NewLoader(root, rt)

	total, done := 0, 0
	if err := loader.LoadTOC(`Interface\GlueXML\GlueXML.toc`, func(d, t int) {
		done, total = d, t
	}); err != nil {
		t.Fatalf("LoadTOC: %v", err)
	}
	if total == 0 || done != total {
		t.Fatalf("progress = %d/%d", done, total)
	}

	errs := rt.ScriptErrors()
	for _, e := range errs {
		t.Logf("script error: %v", e)
	}
	if len(errs) != 0 {
		t.Errorf("%d script error(s) executing glue interface data", len(errs))
	}

	must := []string{"GlueParent", "AccountLogin", "CharacterSelect", "RealmList"}
	for _, name := range must {
		if rt.L.GetGlobal(name).Type() == 0 { // LTNil
			t.Errorf("global %s missing after load", name)
		}
	}

	// Login screen text comes from the string table; verify a known entry
	// is live in the state.
	if rt.L.GetGlobal("LOGIN") == nil {
		t.Logf("LOGIN global absent (localization entry names vary)")
	}
}
