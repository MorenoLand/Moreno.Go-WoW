package ui

import (
	"os"
	"testing"
)

type chatSendProbeHost struct {
	hostScreen
	count    int
	message  string
	chatType string
	language string
	target   string
}

func (h *chatSendProbeHost) SendChatMessage(message, chatType, language, target string) error {
	h.count++
	h.message = message
	h.chatType = chatType
	h.language = language
	h.target = target
	return nil
}

func TestLiveWorldChatEditBoxSendsThroughHost(t *testing.T) {
	dataPath := os.Getenv("WOW_TEST_DATA")
	if dataPath == "" {
		t.Skip("WOW_TEST_DATA not set")
	}
	engine, err := LoadUIEngineFromMPQ(dataPath, "enUS", "")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	host := &chatSendProbeHost{hostScreen: hostScreen{w: 960, h: 640}}
	engine.Rt.Host = host
	if err := engine.LoadWorldUI(); err != nil {
		t.Fatal(err)
	}
	if !engine.Rt.Execute(`
ChatFrame1EditBox:SetAttribute("chatType", "SAY");
ChatFrame1EditBox.language = "Common";
ChatFrame1EditBox:SetText("hello");
ChatEdit_SendText(ChatFrame1EditBox, 1);`, "@world-chat-send.lua") {
		t.Fatalf("send failed: %v", engine.Rt.ScriptErrors())
	}
	if host.count != 1 || host.message != "hello" || host.chatType != "SAY" || host.language != "Common" || host.target != "" {
		t.Fatalf("chat send=%+v", host)
	}
	if len(engine.Rt.ScriptErrors()) != 0 {
		t.Fatalf("chat send script errors=%v", engine.Rt.ScriptErrors())
	}
}
