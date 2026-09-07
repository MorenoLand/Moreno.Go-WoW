package world

import (
	"encoding/binary"
	"testing"
)

func TestParseAuraUpdate(t *testing.T) {
	body := []byte{0x01, 1, 3}
	spell := make([]byte, 4)
	binary.LittleEndian.PutUint32(spell, 21562)
	body = append(body, spell...)
	body = append(body, 0x21, 21, 2, 0x0F, 0x44, 0x33, 0x22, 0x11, 0xE8, 0x03, 0x00, 0x00, 0xF4, 0x01, 0x00, 0x00)
	update, err := ParseAuraUpdate(body, false)
	if err != nil {
		t.Fatal(err)
	}
	if update.GUID != 1 || len(update.Updates) != 1 {
		t.Fatalf("update=%+v", update)
	}
	aura := update.Updates[0]
	if aura.Slot != 3 || aura.Aura.SpellID != 21562 || aura.Aura.Flags != 0x21 || aura.Aura.Level != 21 || aura.Aura.Charges != 2 || aura.Aura.CasterGUID != 0x11223344 || aura.Aura.MaxDuration != 1000 || aura.Aura.Duration != 500 {
		t.Fatalf("aura=%+v", aura)
	}
}

func TestParseAuraUpdateAllHandlesRemovalAndEffectAmounts(t *testing.T) {
	body := []byte{0x01, 2, 0, 0, 0, 0, 0, 4, 4, 0, 0, 0, 0x49, 10, 1, 7, 0, 0, 0}
	update, err := ParseAuraUpdate(body, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(update.Updates) != 2 || !update.Updates[0].Aura.Empty() || update.Updates[1].Aura.SpellID != 4 {
		t.Fatalf("updates=%+v", update.Updates)
	}
}

func TestParseAuraUpdateRejectsTruncatedDuration(t *testing.T) {
	body := []byte{0x01, 0x01, 0, 1, 0, 0, 0, 0x20, 1, 1, 0x01, 0}
	if _, err := ParseAuraUpdate(body, false); err == nil {
		t.Fatal("truncated duration was accepted")
	}
}
