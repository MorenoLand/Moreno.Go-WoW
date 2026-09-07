package world

import (
	"encoding/binary"
	"testing"
)

func TestParseMessageChatSay(t *testing.T) {
	body := []byte{byte(ChatSay)}
	var word [8]byte
	binary.LittleEndian.PutUint32(word[:4], 1)
	body = append(body, word[:4]...)
	binary.LittleEndian.PutUint64(word[:], 42)
	body = append(body, word[:]...)
	body = append(body, 0, 0, 0, 0)
	binary.LittleEndian.PutUint64(word[:], 7)
	body = append(body, word[:]...)
	binary.LittleEndian.PutUint32(word[:4], 6)
	body = append(body, word[:4]...)
	body = append(body, []byte("hello\x00")...)
	body = append(body, 0)
	message, err := ParseMessageChat(body)
	if err != nil {
		t.Fatal(err)
	}
	if message.Type != ChatSay || message.Language != 1 || message.Sender != 42 || message.Target != 7 || message.Text != "hello" || message.Tag != 0 {
		t.Fatalf("message=%+v", message)
	}
}

func TestParseMessageChatRejectsTrailingBytes(t *testing.T) {
	body := []byte{byte(ChatSay), 0, 0, 0, 0}
	body = append(body, make([]byte, 8)...)
	body = append(body, make([]byte, 4)...)
	body = append(body, make([]byte, 8)...)
	body = append(body, 6, 0, 0, 0)
	body = append(body, []byte("hello\x00")...)
	body = append(body, 0, 1)
	if _, err := ParseMessageChat(body); err == nil {
		t.Fatal("trailing chat data was accepted")
	}
}

func TestBuildMessageChatSayAndWhisper(t *testing.T) {
	say, err := BuildMessageChat(ChatSay, 7, "", "hello")
	if err != nil {
		t.Fatal(err)
	}
	expectedSay := []byte{1, 0, 0, 0, 7, 0, 0, 0, 'h', 'e', 'l', 'l', 'o', 0}
	if string(say) != string(expectedSay) {
		t.Fatalf("say=%v expected=%v", say, expectedSay)
	}
	whisper, err := BuildMessageChat(ChatWhisper, 7, "Alice", "Hi")
	if err != nil {
		t.Fatal(err)
	}
	expectedWhisper := []byte{7, 0, 0, 0, 7, 0, 0, 0, 'A', 'l', 'i', 'c', 'e', 0, 'H', 'i', 0}
	if string(whisper) != string(expectedWhisper) {
		t.Fatalf("whisper=%v expected=%v", whisper, expectedWhisper)
	}
}

func TestBuildMessageChatRejectsInvalidInputs(t *testing.T) {
	if _, err := BuildMessageChat(ChatSystem, 7, "", "hello"); err == nil {
		t.Fatal("system chat was accepted for sending")
	}
	if _, err := BuildMessageChat(ChatWhisper, 7, "", "hello"); err == nil {
		t.Fatal("whisper without target was accepted")
	}
	if _, err := BuildMessageChat(ChatSay, 5, "", "hello"); err == nil {
		t.Fatal("invalid language was accepted")
	}
}
