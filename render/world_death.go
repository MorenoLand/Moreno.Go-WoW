package render

import (
	"time"

	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// handleDeathPacket applies the packets about dying and coming back; it reports
// whether it was one.
func handleDeathPacket(rt *ui.Runtime, names *worldNames, packet world.Packet) bool {
	switch packet.Opcode {
	case world.CorpseReclaimDelay:
		if delay, err := world.ParseCorpseReclaimDelay(packet.Body); err == nil {
			rt.SetCorpseReclaimDelay(time.Duration(delay) * time.Millisecond)
		}
	case world.ResurrectRequest:
		offer, err := world.ParseResurrectRequest(packet.Body)
		if err != nil {
			return true
		}
		if offer.Name == "" {
			offer.Name = names.players[offer.GUID].Name
		}
		rt.OfferResurrection(offer.GUID, offer.Name, offer.Sickness, offer.Timer)
	default:
		return false
	}
	return true
}

func (h *clientHost) RepopRequest() error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientRepopRequest, []byte{0})
}

func (h *clientHost) ReclaimCorpse() error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	var guid uint64
	if h.corpseGUID != nil {
		guid = h.corpseGUID()
	}
	return session.SendRaw(world.ClientReclaimCorpse, world.BuildLootGUID(guid))
}

func (h *clientHost) ResurrectResponse(guid uint64, accept bool) error {
	session, err := h.worldSession()
	if err != nil {
		return err
	}
	return session.SendRaw(world.ClientResurrectResponse, world.BuildResurrectResponse(guid, accept))
}

// corpseReclaimRadius is how near its corpse a ghost must stand to reclaim it.
const corpseReclaimRadius = 39

// updateCorpseNear offers the corpse back while the ghost stands within reach.
func updateCorpseNear(rt *ui.Runtime, entities map[uint64]*worldEntity, ownGUID uint64, position [3]float32) {
	if !rt.PlayerIsGhost() {
		rt.SetCorpseNear(false)
		return
	}
	for _, entity := range entities {
		if entity.objectType != world.ObjectTypeCorpse || !entity.hasPosition {
			continue
		}
		if uint64(entity.fields[6])|uint64(entity.fields[7])<<32 != ownGUID {
			continue
		}
		dx, dy := entity.movement.Position.X-position[0], entity.movement.Position.Y-position[1]
		rt.SetCorpseNear(dx*dx+dy*dy <= corpseReclaimRadius*corpseReclaimRadius)
		return
	}
	rt.SetCorpseNear(false)
}
