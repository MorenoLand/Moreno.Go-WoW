package ui

import (
	"os"
	"testing"
)

func TestLiveGlueStatusDialogContract(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	dialog := engine.Rt.widgets["GlueDialog"]
	if dialog == nil {
		t.Fatal("GlueDialog is missing")
	}
	for _, event := range []string{"OPEN_STATUS_DIALOG", "UPDATE_STATUS_DIALOG", "CLOSE_STATUS_DIALOG"} {
		if !dialog.events[event] {
			t.Fatalf("GlueDialog is not registered for %s", event)
		}
	}
	engine.SetStatusKey("AUTH_FAILED")
	if !dialog.shown {
		t.Fatal("OPEN_STATUS_DIALOG did not show GlueDialog")
	}
	if !engine.Rt.Execute("assert(GlueDialog.which == 'OKAY')", "@status-dialog-test.lua") {
		t.Fatalf("dialog type after failure: %v", engine.Rt.ScriptErrors())
	}
	button := engine.Rt.widgets["GlueDialogButton1"]
	if button == nil {
		t.Fatal("GlueDialogButton1 is missing")
	}
	if button.normalTexture == nil || button.normalTexture.textureFile == "" || engine.loadBLP(button.normalTexture.textureFile) == nil {
		t.Fatalf("auth button normal texture unavailable: %+v", button.normalTexture)
	}
	engine.Update(1.0 / 60)
	if button.normalTexture == nil || button.pushedTexture == nil || button.highlightTexture == nil || button.normalTexture.textureFile != `Interface\Glues\Common\Glue-Panel-Button-Up-Blue` || button.pushedTexture.textureFile != `Interface\Glues\Common\Glue-Panel-Button-Down-Blue` || button.highlightTexture.textureFile != `Interface\Glues\Common\Glue-Panel-Button-Highlight-Blue` {
		t.Fatalf("auth button artwork normal=%v pushed=%v highlight=%v", button.normalTexture, button.pushedTexture, button.highlightTexture)
	}
	engine.SetStatusKey("GAME_SERVER_LOGIN")
	if text := engine.Rt.widgets["GlueDialogText"]; text == nil || text.text != engine.resolveText("GAME_SERVER_LOGIN") {
		t.Fatalf("dialog text=%v", text)
	}
	if !engine.Rt.Execute("assert(GlueDialog.which == 'CANCEL')", "@status-dialog-test.lua") {
		t.Fatalf("dialog type during login: %v", engine.Rt.ScriptErrors())
	}
	engine.SetStatusText("raw error")
	if text := engine.Rt.widgets["GlueDialogText"]; text == nil || text.text != "raw error" {
		t.Fatalf("raw dialog text=%v", text)
	}
	engine.SetStatusText("")
	if dialog.shown {
		t.Fatal("CLOSE_STATUS_DIALOG did not hide GlueDialog")
	}
}
