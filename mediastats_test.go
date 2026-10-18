package meowcaller

import (
	"math"
	"testing"
	"time"

	"github.com/purpshell/meowcaller/rtp"
)

func TestMediaStatsAggregatesReceptionReports(t *testing.T) {
	c := &Call{}
	if got := c.MediaStats(); got != (MediaStats{}) {
		t.Fatalf("before media: %+v, want zero", got)
	}
	now := time.Unix(1_760_000_000, 0)
	reports := []*rtp.RtcpReceptionReport{
		{Ssrc: 1, FractionLost: 64, CumulativeLost: 10, Jitter: 160}, // 25%, 10 ms
		nil,
		{Ssrc: 2, FractionLost: 128, CumulativeLost: 5, Jitter: 80}, // 50%, 5 ms
	}
	c.updateLocalMediaStats(now, reports, 900, rtp.RtcpSenderStats{PacketsSent: 800, OctetsSent: 64_000})

	got := c.MediaStats()
	if !got.UpdatedAt.Equal(now) || got.PacketsReceived != 900 || got.PacketsLost != 15 ||
		got.FractionLost != 0.5 || got.JitterMs != 10 || got.PacketsSent != 800 || got.BytesSent != 64_000 {
		t.Fatalf("stats = %+v", got)
	}
	if got.HasRemote || got.HasRTT {
		t.Fatalf("no peer report yet: %+v", got)
	}
}

func TestMediaStatsRecordsThePeersReportAndRoundTrip(t *testing.T) {
	c := &Call{}
	sent := time.UnixMilli(1_760_000_000_000)
	arrival := sent.Add(330 * time.Millisecond)
	c.updateRemoteMediaStats(arrival, rtp.RtcpReceptionReport{
		FractionLost: 26, CumulativeLost: 42, Jitter: 320,
		LastSenderReport:           rtp.CompactNTP(uint64(sent.UnixMilli())),
		DelaySinceLastSenderReport: 250 * 65_536 / 1000,
	})
	got := c.MediaStats()
	if !got.HasRemote || got.RemotePacketsLost != 42 || math.Abs(got.RemoteFractionLost-26.0/256) > 1e-9 || got.RemoteJitterMs != 20 {
		t.Fatalf("remote = %+v", got)
	}
	if !got.HasRTT || math.Abs(got.RTTMs-80) > 1 {
		t.Fatalf("rtt = %.2f (has %v), want about 80", got.RTTMs, got.HasRTT)
	}

	// A later report without LSR keeps the last known round trip.
	c.updateRemoteMediaStats(arrival.Add(time.Second), rtp.RtcpReceptionReport{CumulativeLost: 43})
	if got := c.MediaStats(); !got.HasRTT || math.Abs(got.RTTMs-80) > 1 || got.RemotePacketsLost != 43 {
		t.Fatalf("after a report without LSR: %+v", got)
	}
}

func TestMediaStatsNilCallIsSafe(t *testing.T) {
	var c *Call
	c.updateLocalMediaStats(time.Now(), nil, 0, rtp.RtcpSenderStats{})
	c.updateRemoteMediaStats(time.Now(), rtp.RtcpReceptionReport{})
}
