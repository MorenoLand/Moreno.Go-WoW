package render

import (
	"time"

	"github.com/MorenoLand/Moreno.WoW/network"
	"github.com/MorenoLand/Moreno.WoW/ui"
	"github.com/MorenoLand/Moreno.WoW/world"
)

// worldPingInterval is how often the client pings the server (the client sends
// CMSG_PING every 30 seconds).
const worldPingInterval = 30 * time.Second

// worldNetStats measures the round trip to the server and the rate it sends at,
// for GetNetStats.
type worldNetStats struct {
	pingSequence uint32
	pingSent     time.Time
	lastPing     time.Time
	latency      int

	received    int
	bandwidth   float64
	windowStart time.Time
	windowBytes int
}

// update pings the server when it is time and refreshes the bandwidth figure
// once a second.
func (n *worldNetStats) update(now time.Time, session *network.Session, rt *ui.Runtime) {
	if session == nil {
		return
	}
	if n.windowStart.IsZero() {
		n.windowStart, n.windowBytes = now, n.received
	}
	if elapsed := now.Sub(n.windowStart); elapsed >= time.Second {
		n.bandwidth = float64(n.received-n.windowBytes) / 1024 / elapsed.Seconds()
		n.windowStart, n.windowBytes = now, n.received
		rt.SetNetStats(n.latency, n.bandwidth)
	}
	if n.lastPing.IsZero() || now.Sub(n.lastPing) >= worldPingInterval {
		n.lastPing = now
		n.pingSequence++
		n.pingSent = now
		_ = session.SendRaw(world.PingOpcode, world.BuildPing(n.pingSequence, uint32(n.latency)))
	}
}
