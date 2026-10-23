package render

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"

	"github.com/MorenoLand/Moreno.WoW/network"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// worldMovementNet answers the server's movement instructions: forced speed
// and flag changes, knock-backs and teleports. Each one changes the local
// controller and is acknowledged with the controller's own movement state, the
// way the original client does.
type worldMovementNet struct {
	guid     uint64
	started  time.Time
	session  func() *network.Session
	debug    bool
	counters map[uint16]uint32
}

func newWorldMovementNet(guid uint64, started time.Time, session func() *network.Session, debug bool) *worldMovementNet {
	return &worldMovementNet{guid: guid, started: started, session: session, debug: debug, counters: make(map[uint16]uint32)}
}

func (n *worldMovementNet) sendAck(opcode world.ClientOpcode, counter uint32, info world.MovementInfo, extra ...float32) {
	session := n.session()
	if session == nil {
		return
	}
	body := world.BuildMovementAck(n.guid, counter, info, extra...)
	if err := session.SendRaw(opcode, body); err != nil && n.debug {
		log.Printf("world movement ack %#x: %v", uint32(opcode), err)
	}
}

// handle applies a movement packet and reports whether it was one.
func (n *worldMovementNet) handle(packet world.Packet, c *worldCameraController, reporter *worldMovementReporter, now time.Time) bool {
	if c == nil || reporter == nil {
		return false
	}
	if _, ok := world.ForcedChangeFor(packet.Opcode); ok {
		guid, counter, change, err := world.ParseForcedChange(packet.Opcode, packet.Body)
		if err != nil {
			if n.debug {
				log.Printf("world forced movement change %#x: %v", packet.Opcode, err)
			}
			return true
		}
		if guid != n.guid {
			return true
		}
		c.applyForcedChange(change)
		info := reporter.info(now, c, c.flags())
		if change.AckSpeed {
			n.sendAck(change.Ack, counter, info, change.Value)
		} else {
			n.sendAck(change.Ack, counter, info)
		}
		return true
	}
	switch packet.Opcode {
	case world.MoveKnockBack:
		knock, err := world.ParseKnockBack(packet.Body)
		if err != nil || knock.GUID != n.guid {
			return true
		}
		c.knockBack(knock)
		n.sendAck(world.AckMoveKnockBack, knock.Counter, reporter.info(now, c, c.flags()|world.MovementFlagFalling))
		return true
	case world.MoveTeleportAck:
		tele, err := world.ParseTeleport(packet.Body)
		if err != nil {
			if n.debug {
				log.Printf("world teleport: %v", err)
			}
			return true
		}
		if tele.GUID != n.guid {
			return true
		}
		c.teleport(tele.Movement.Position)
		session := n.session()
		if session == nil {
			return true
		}
		body := world.AppendPackedGUID(nil, n.guid)
		body = binary.LittleEndian.AppendUint32(body, tele.Counter)
		body = binary.LittleEndian.AppendUint32(body, uint32(now.Sub(n.started).Milliseconds()))
		if err := session.SendRaw(world.MoveTeleportAckOpcode, body); err != nil && n.debug {
			log.Printf("world teleport ack: %v", err)
		}
		return true
	}
	return false
}

var _ = fmt.Sprintf
