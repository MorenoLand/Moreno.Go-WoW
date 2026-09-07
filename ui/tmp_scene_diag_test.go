package ui

import (
	"image/png"
	"os"
	"strings"
	"testing"
)

func TestTmpSceneDiag(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	for _, archive := range engine.AssetLoader.mpq.archives {
		t.Logf("archive %s", archive.path)
	}
	if list, listErr := engine.AssetLoader.ReadFile("(listfile)"); listErr == nil {
		for _, line := range strings.Split(string(list), "\n") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "loginscene") || strings.Contains(lower, "loginscreen") || strings.Contains(lower, "accountlogin") {
				t.Logf("list %s", strings.TrimSpace(line))
			}
		}
	}
	engine.SetInitialCredentials("denveous", "", false)
	login := engine.Rt.widgets["AccountLogin"]
	t.Logf("screen=%v modelPath=%q login=%v shown=%v modelFile=%q errors=%v", mustCVarTmp(engine, "currentGlueScreen"), engine.CurrentModelPath(), login != nil, login != nil && login.shown, loginModelTmp(login), engine.Rt.ScriptErrors())
	if engine.Rt.Host != nil {
		width, height := engine.Rt.Host.ScreenSize()
		t.Logf("host size=%gx%g", width, height)
	}
	if engine.Rt.Execute("local w,h=GlueParent:GetSize(); _G.GlueWidth=tostring(w); _G.GlueHeight=tostring(h)", "@size-probe.lua") {
		t.Logf("lua GlueParent size=%s,%s", engine.Rt.L.GetGlobal("GlueWidth"), engine.Rt.L.GetGlobal("GlueHeight"))
	}
	for _, name := range []string{"GlueParent", "AccountLogin", "LoginScene", "LoginScreenBackground", "LoginScreenBlend"} {
		if widget := engine.Rt.widgets[name]; widget != nil {
			t.Logf("geometry name=%s size=%gx%g points=%v shown=%t alpha=%g texture=%q", name, widget.width, widget.height, widget.points, widget.shown, widget.alpha, widget.textureFile)
		}
	}
	if root := engine.Rt.widgets["GlueParent"]; root != nil {
		t.Logf("glue-root shown=%t children=%d", root.shown, len(root.children))
		for index, child := range root.children {
			if child.name == "AccountLogin" || child.name == "LoginScene" || strings.Contains(child.name, "Character") {
				t.Logf("glue-child=%d name=%q shown=%t", index, child.name, child.shown)
			}
		}
	}
	if login != nil {
		for name := range login.scripts {
			t.Logf("login script=%s", name)
		}
	}
	for _, path := range []string{`Interface\Glues\Models\UI_Login\UI_Login.m2`, `Interface\Glues\Models\UI_CharacterSelect\UI_CharacterSelect.m2`, `Interface\Glues\Models\UI_NightElf\UI_NightElf.m2`, `Interface\Glues\Models\UI_Human\UI_Human.m2`, `Interface\Glues\Models\UI_Alliance\UI_Alliance.m2`} {
		data, readErr := engine.AssetLoader.ReadFile(path)
		t.Logf("model=%s bytes=%d err=%v", path, len(data), readErr)
	}
	for _, path := range []string{`Interface\GlueXML\GlueXML.toc`, `Interface\GlueXML\LoginScene.lua`, `Interface\GlueXML\AccountLogin.lua`, `Interface\GlueXML\AccountLogin.xml`} {
		data, readErr := engine.AssetLoader.ReadFile(path)
		if readErr != nil {
			t.Logf("source missing %s: %v", path, readErr)
			continue
		}
		t.Logf("source %s bytes=%d", path, len(data))
		for index, line := range strings.Split(string(data), "\n") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "model") || strings.Contains(lower, "scene") || strings.Contains(lower, "background") || strings.Contains(lower, "accountloginoptions") || strings.Contains(lower, "optionsbutton") || strings.Contains(lower, "accountloginaccountedit") || strings.Contains(lower, "accountloginpasswordedit") || (path == `Interface\GlueXML\LoginScene.lua` && index >= 70 && index <= 140) || (path == `Interface\GlueXML\LoginScene.lua` && index >= 158 && index <= 275) || (path == `Interface\GlueXML\LoginScene.lua` && index >= 440 && index <= 480) || (path == `Interface\GlueXML\AccountLogin.xml` && index >= 140 && index <= 320) {
				t.Logf("source-line %s:%d %s", path, index+1, strings.TrimSpace(line))
			}
		}
	}
	for name, widget := range engine.Rt.widgets {
		if widget.kind == kindModel || widget.kind == kindModelFFX {
			t.Logf("model-widget=%s shown=%t file=%q parent=%s", name, widget.shown, widget.modelFile, widgetParentForTmp(widget))
		}
	}
	if scene := engine.Rt.widgets["LoginScene"]; scene != nil {
		t.Logf("login-scene shown=%t parent=%s children=%d", scene.shown, widgetParentForTmp(scene), len(scene.children))
		for name := range scene.scripts {
			t.Logf("login-scene script=%s", name)
		}
		for index, child := range scene.children {
			t.Logf("login-child=%d kind=%s name=%q shown=%t file=%q children=%d", index, child.kind.objectType(), child.name, child.shown, child.modelFile, len(child.children))
		}
	}
	for index := 0; index < 5; index++ {
		engine.Update(1.0 / 60)
		t.Logf("after-update=%d current=%q", index+1, engine.CurrentModelPath())
	}
	engine.Update(1.0)
	engine.SetSceneBackground(true)
	frame := engine.Render(1920, 1080)
	if background := engine.Rt.widgets["LoginScreenBackground"]; background != nil {
		image := engine.loadBLP(background.textureFile)
		t.Logf("login background shown=%t texture=%q rect=%v image=%v", background.shown, background.textureFile, background.renderRect, image != nil)
		if image != nil {
			if file, createErr := os.Create(`..\bin\tmp-login-background.png`); createErr == nil {
				if encodeErr := png.Encode(file, image); encodeErr != nil {
					t.Logf("login background encode error: %v", encodeErr)
				}
				file.Close()
			}
		}
	}
	if file, createErr := os.Create(`..\bin\tmp-login-ui.png`); createErr == nil {
		if encodeErr := png.Encode(file, frame); encodeErr != nil {
			t.Logf("login UI capture encode error: %v", encodeErr)
		}
		file.Close()
	} else {
		t.Logf("login UI capture create error: %v", createErr)
	}
	if !engine.Rt.Execute(`_G.SceneLoaded = tostring(ModelList.loaded); _G.SceneTimed = tostring(timed_update); _G.SceneM1 = tostring(M[1] and M[1].parent and M[1].parent:IsShown())`, "@scene-probe.lua") {
		t.Fatal(engine.Rt.ScriptErrors())
	}
	t.Logf("scene probe loaded=%s timed=%s m1=%s", engine.Rt.L.GetGlobal("SceneLoaded"), engine.Rt.L.GetGlobal("SceneTimed"), engine.Rt.L.GetGlobal("SceneM1"))
	if !engine.Rt.Execute(`_G.BlendStartProbe = tostring(blend_start); _G.BlendDurationProbe = tostring(ModelList.blend_start_duration); _G.BlendAlphaProbe = tostring(LoginScreenBlend:GetAlpha())`, "@blend-probe.lua") {
		t.Fatal(engine.Rt.ScriptErrors())
	}
	t.Logf("blend probe start=%s duration=%s alpha=%s", engine.Rt.L.GetGlobal("BlendStartProbe"), engine.Rt.L.GetGlobal("BlendDurationProbe"), engine.Rt.L.GetGlobal("BlendAlphaProbe"))
	for _, invocation := range engine.Rt.TraceInvocations() {
		if strings.Contains(invocation, "AccountLogin|OnUpdate") || strings.Contains(invocation, "LoginScene") {
			t.Logf("scene invoke=%s", invocation)
		}
	}
	if scene := engine.Rt.widgets["LoginScene"]; scene != nil {
		for index, child := range scene.children {
			t.Logf("after-update-child=%d shown=%t children=%d", index, child.shown, len(child.children))
		}
	}
	if !engine.Rt.Execute(`LoginScreen_OnUpdate(AccountLogin, 1); _G.ManualCurrent = tostring(M[1] and M[1].parent and M[1].parent:IsShown())`, "@manual-scene-update.lua") {
		t.Fatal(engine.Rt.ScriptErrors())
	}
	t.Logf("manual scene current=%s", engine.Rt.L.GetGlobal("ManualCurrent"))
	if scene := engine.Rt.widgets["LoginScene"]; scene != nil {
		var visit func(*widget, int)
		visit = func(parent *widget, depth int) {
			for _, child := range parent.children {
				if child.kind == kindModel || child.kind == kindModelFFX {
					t.Logf("login-model depth=%d shown=%t file=%q position=%v facing=%v scale=%v alpha=%v", depth, child.shown, child.modelFile, child.modelPosition, child.modelFacing, child.modelScale, child.alpha)
				}
				visit(child, depth+1)
			}
		}
		visit(scene, 0)
	}
}

func mustCVarTmp(engine *UIEngine, name string) string {
	value, _ := engine.Rt.GetCVar(name)
	return value
}

func loginModelTmp(w *widget) string {
	if w == nil {
		return ""
	}
	return w.modelFile
}
