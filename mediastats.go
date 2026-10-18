package meowcaller

import (
	"time"

	"github.com/purpshell/meowcaller/rtp"
)

// MediaStats is the network quality of a call's audio: what arrives here from the
// peer, and what the peer's device reports about the audio we send. It refreshes
// with every RTCP interval (about 1.5 s) once media is up; the zero value means
// media never started.
type MediaStats struct {
	UpdatedAt time.Time

	// Peer to us, measured here. In a group call these add up every participant's
	// stream (lost) or take the worst one (fraction lost, jitter).
	PacketsReceived uint64
	PacketsLost     int64   // cumulative: expected minus received (RFC 3550; may be negative with duplicates)
	FractionLost    float64 // over the last interval, 0 to 1
	JitterMs        float64

	// Us to the peer.
	PacketsSent uint32
	BytesSent   uint32

	// As reported by the peer's device (RTCP reception report about our stream).
	// HasRemote is false until the first report arrives, and stays false if the peer
	// never sends one.
	HasRemote          bool
	RemoteUpdatedAt    time.Time
	RemotePacketsLost  int64
	RemoteFractionLost float64
	RemoteJitterMs     float64

	// Round trip between this server and the peer's device, through the WhatsApp
	// relay, from the report's LSR/DLSR. HasRTT is false while unknown.
	HasRTT bool
	RTTMs  float64
}

// MediaStats returns the latest network quality of the call's audio.
func (c *Call) MediaStats() MediaStats {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	return c.stats
}

// updateLocalMediaStats records one RTCP interval: the reports we are about to send
// on our reception, and our own sender counters.
func (c *Call) updateLocalMediaStats(now time.Time, reports []*rtp.RtcpReceptionReport, received uint64, sent rtp.RtcpSenderStats) {
	if c == nil {
		return
	}
	var lost int64
	var fraction, jitter float64
	for _, r := range reports {
		if r == nil {
			continue
		}
		lost += int64(r.CumulativeLost)
		fraction = max(fraction, float64(r.FractionLost)/256)
		jitter = max(jitter, rtpUnitsToMs(r.Jitter))
	}
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.stats.UpdatedAt = now
	c.stats.PacketsReceived = received
	c.stats.PacketsLost = lost
	c.stats.FractionLost = fraction
	c.stats.JitterMs = jitter
	c.stats.PacketsSent = sent.PacketsSent
	c.stats.BytesSent = sent.OctetsSent
}

// updateRemoteMediaStats records the peer's reception report about our audio, which
// arrived at now.
func (c *Call) updateRemoteMediaStats(now time.Time, r rtp.RtcpReceptionReport) {
	if c == nil {
		return
	}
	rtt, hasRTT := rtp.RoundTripMs(uint64(now.UnixMilli()), r.LastSenderReport, r.DelaySinceLastSenderReport)
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.stats.HasRemote = true
	c.stats.RemoteUpdatedAt = now
	c.stats.RemotePacketsLost = int64(r.CumulativeLost)
	c.stats.RemoteFractionLost = float64(r.FractionLost) / 256
	c.stats.RemoteJitterMs = rtpUnitsToMs(r.Jitter)
	if hasRTT {
		c.stats.HasRTT = true
		c.stats.RTTMs = rtt
	}
}

// rtpUnitsToMs converts an RTCP jitter (in RTP timestamp units of the audio clock) to milliseconds.
func rtpUnitsToMs(units uint32) float64 {
	return float64(units) * 1000 / SampleRate
}
