package ui

import (
	lua "github.com/yuin/gopher-lua"
	"os"
	"path/filepath"
	"testing"
)

// writeTree creates a loader fixture tree from a path -> content map.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLoadTOCOrderAndSkipRules(t *testing.T) {
	root := writeTree(t, map[string]string{
		"Interface/GlueXML/GlueXML.toc": "# comment\r\n## Interface: 30300\r\nFirst.lua\r\n\r\nSecond.xml\r\n",
		"Interface/GlueXML/First.lua":   "loadedFirst = true\nfirstOrder = (order or 0) + 1\n",
		"Interface/GlueXML/Second.xml":  "<Ui><Script file=\"Third.lua\"/></Ui>",
		"Interface/GlueXML/Third.lua":   "loadedThird = true\nthirdOrder = (order or 0) + 2\n",
	})
	rt := NewRuntime(nil)
	defer rt.Close()
	loader := NewLoader(root, rt)
	var order []int
	if err := loader.LoadTOC("Interface\\GlueXML\\GlueXML.toc", func(done, total int) {
		order = append(order, done)
		if total != 2 {
			t.Fatalf("total = %d, want 2", total)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("progress order = %v", order)
	}
	for _, name := range []string{"loadedFirst", "loadedThird"} {
		if rt.L.GetGlobal(name) != lua.LTrue {
			t.Errorf("%s not set", name)
		}
	}
	if errs := rt.ScriptErrors(); len(errs) != 0 {
		t.Fatalf("script errors: %v", errs)
	}
}

func TestResolveRelativeAndDotDot(t *testing.T) {
	root := writeTree(t, map[string]string{
		"Interface/GlueXML/Frame.xml": "<Ui><Include file=\"..\\Shared\\Common.xml\"/></Ui>",
		"Interface/Shared/Common.xml": "<Ui><Script file=\"Common.lua\"/></Ui>",
		"Interface/Shared/Common.lua": "commonLoaded = true\n",
	})
	rt := NewRuntime(nil)
	defer rt.Close()
	loader := NewLoader(root, rt)
	if err := loader.LoadInterfaceFile("Interface\\GlueXML\\Frame.xml"); err != nil {
		t.Fatal(err)
	}
	if rt.L.GetGlobal("commonLoaded") != lua.LTrue {
		t.Fatal("commonLoaded not set; relative include or .. normalization failed")
	}
}

func TestVirtualTemplateAndParentName(t *testing.T) {
	root := writeTree(t, map[string]string{
		"Interface/GlueXML/T.xml": `<Ui>
<Button name="MenuTemplate" virtual="true">
	<Size><AbsDimension x="128" y="32"/></Size>
	<Scripts><OnLoad>templateLoadRan = true</OnLoad></Scripts>
</Button>
<Button name="Panel" inherits="MenuTemplate">
	<Frames>
		<Button name="$parentClose">
			<Scripts><OnLoad>closeName = self:GetName()</OnLoad></Scripts>
		</Button>
	</Frames>
</Button>
</Ui>`,
	})
	rt := NewRuntime(nil)
	defer rt.Close()
	loader := NewLoader(root, rt)
	if err := loader.LoadInterfaceFile("Interface\\GlueXML\\T.xml"); err != nil {
		t.Fatal(err)
	}
	panel := rt.lookup("Panel")
	if panel == nil {
		t.Fatal("Panel widget missing")
	}
	if panel.width != 128 || panel.height != 32 {
		t.Fatalf("inherited size = %v x %v, want 128 x 32", panel.width, panel.height)
	}
	if rt.L.GetGlobal("templateLoadRan") != lua.LTrue {
		t.Fatal("inherited OnLoad did not run")
	}
	if got := rt.L.GetGlobal("closeName"); got != lua.LString("PanelClose") {
		t.Fatalf("$parent substitution: closeName = %v, want PanelClose", got)
	}
	if rt.lookup("PanelClose") == nil {
		t.Fatal("PanelClose not registered")
	}
}

func TestScriptParamsAndEvents(t *testing.T) {
	root := writeTree(t, map[string]string{
		"Interface/GlueXML/E.xml": `<Ui>
<Frame name="Watcher">
	<Scripts>
		<OnLoad>seenLoad = self:GetName()</OnLoad>
		<OnEvent>seenEvent = event; seenArg = (...)</OnEvent>
	</Scripts>
</Frame>
</Ui>`,
	})
	rt := NewRuntime(nil)
	defer rt.Close()
	loader := NewLoader(root, rt)
	if err := loader.LoadInterfaceFile("Interface\\GlueXML\\E.xml"); err != nil {
		t.Fatal(err)
	}
	if got := rt.L.GetGlobal("seenLoad"); got != lua.LString("Watcher") {
		t.Fatalf("OnLoad self = %v", got)
	}
	rt.L.SetGlobal("RegisterTest", lua.LString("done"))
	watcher := rt.lookup("Watcher")
	if watcher == nil {
		t.Fatal("Watcher missing")
	}
	rt.registerEventWidget("OPEN_REALM_LIST", watcher)
	rt.FireEvent("OPEN_REALM_LIST", lua.LString("payload"))
	if got := rt.L.GetGlobal("seenEvent"); got != lua.LString("OPEN_REALM_LIST") {
		t.Fatalf("OnEvent event = %v", got)
	}
	if got := rt.L.GetGlobal("seenArg"); got != lua.LString("payload") {
		t.Fatalf("OnEvent payload = %v", got)
	}
}

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		`Interface\GlueXML\..\Shared\F.xml`: `Interface\Shared\F.xml`,
		`Interface\A\B\..\..\C.xml`:         `Interface\C.xml`,
		`Interface\Plain\File.xml`:          `Interface\Plain\File.xml`,
	}
	for in, want := range cases {
		if got := normalizePath(in); got != want {
			t.Errorf("normalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}
