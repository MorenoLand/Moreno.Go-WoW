package world

import (
	"encoding/binary"
	"fmt"
	"math"
)

// GameTime is the server's SMSG_LOGIN_SETTIMESPEED: the game clock at the
// moment of the packet and how fast it runs.
type GameTime struct {
	// MinuteOfDay is the game clock as minutes since midnight.
	MinuteOfDay int
	// Speed is game minutes elapsed per real second.
	Speed float32
}

// ParseLoginSetTimeSpeed reads a packed game time (minute in bits 0-5, hour in
// bits 6-10) followed by the clock speed.
func ParseLoginSetTimeSpeed(body []byte) (GameTime, error) {
	if len(body) < 8 {
		return GameTime{}, fmt.Errorf("login set time speed packet is %d bytes", len(body))
	}
	packed := binary.LittleEndian.Uint32(body[0:4])
	speed := math.Float32frombits(binary.LittleEndian.Uint32(body[4:8]))
	minute, hour := int(packed&0x3f), int(packed>>6)&0x1f
	return GameTime{MinuteOfDay: hour*60 + minute, Speed: speed}, nil
}
