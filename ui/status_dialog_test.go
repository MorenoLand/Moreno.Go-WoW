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
