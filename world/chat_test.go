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
