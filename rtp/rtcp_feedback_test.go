package rtp

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestParseReceptionReportForReadsThePeersSenderReportBlock(t *testing.T) {
	const ours = uint32(0x0A0B0C0D)
	want := RtcpReceptionReport{
		Ssrc: ours, FractionLost: 64, CumulativeLost: 1234, ExtendedHighestSequence: 0x0001_0203,
		Jitter: 480, LastSenderReport: 0x11223344, DelaySinceLastSenderReport: 0x00008000,
	}
	var cname [WhatsappRtcpCnameLen]byte
	packet := BuildSenderReportWithSdesAndReception(0x99887766, &RtcpSenderStats{PacketsSent: 10}, 1_700_000_000_000, &cname, &want, true)

	got, ok := ParseReceptionReportFor(packet, ours)
	if !ok {
		t.Fatal("report about our stream not found")
	}
	if got != want {
		t.Fatalf("report = %+v, want %+v", got, want)
	}
	if _, ok := ParseReceptionReportFor(packet, 0xDEADBEEF); ok {
		t.Fatal("a report about another stream must not match")
	}
}

func TestParseReceptionReportForReadsGroupAndReceiverReports(t *testing.T) {
	const ours = uint32(0x01020304)
	report := RtcpReceptionReport{Ssrc: ours, FractionLost: 1, CumulativeLost: -3, Jitter: 7}
	group := BuildGroupSenderReport(0x55555555, nil, 1_000, &report, RTCPGroupReportExtension{1, 2, 3})
	got, ok := ParseReceptionReportFor(group, ours)
	if !ok || got.CumulativeLost != -3 || got.Jitter != 7 {
		t.Fatalf("group SR: report = %+v, ok = %v", got, ok)
	}

	rr := make([]byte, 0, 32)
	rr = append(rr, 0x81, RtcpPtRr, 0, 7)
	rr = binary.BigEndian.AppendUint32(rr, 0x66666666)
	rr = appendReceptionReport(rr, &report)
	got, ok = ParseReceptionReportFor(rr, ours)
	if !ok || got.FractionLost != 1 || got.CumulativeLost != -3 {
		t.Fatalf("RR: report = %+v, ok = %v", got, ok)
	}
}

func TestParseReceptionReportForRejectsTruncatedPackets(t *testing.T) {
	report := RtcpReceptionReport{Ssrc: 1}
	var cname [WhatsappRtcpCnameLen]byte
	packet := BuildSenderReportWithSdesAndReception(2, &RtcpSenderStats{}, 1, &cname, &report, false)
	for n := range 52 {
		if _, ok := ParseReceptionReportFor(packet[:n], 1); ok {
			t.Fatalf("matched a packet cut at %d bytes", n)
		}
	}
	sr := BuildSenderReport(2, &RtcpSenderStats{}, 1)
	if _, ok := ParseReceptionReportFor(sr[:], 2); ok {
		t.Fatal("an SR without blocks has no report")
	}
}

func TestCompactNTPMatchesTheSenderReportTimestamp(t *testing.T) {
	const nowMs = 1_760_000_123_456
	sr := BuildSenderReport(1, &RtcpSenderStats{}, nowMs)
	want := binary.BigEndian.Uint32(sr[8:12])<<16 | binary.BigEndian.Uint32(sr[12:16])>>16
	if got := CompactNTP(nowMs); got != want {
		t.Fatalf("CompactNTP = %#x, want %#x (SR NTP middle bits)", got, want)
	}
}

func TestRoundTripMs(t *testing.T) {
	const sentMs = 1_760_000_000_000
	lsr := CompactNTP(sentMs)
	dlsr := uint32(250 * 65_536 / 1000) // the peer held our SR for 250 ms
	rtt, ok := RoundTripMs(sentMs+250+80, lsr, dlsr)
	if !ok || math.Abs(rtt-80) > 1 {
		t.Fatalf("rtt = %.2f ms (ok %v), want about 80", rtt, ok)
	}
	if _, ok := RoundTripMs(sentMs+300, 0, dlsr); ok {
		t.Fatal("LSR 0 means no SR received yet: no round trip")
	}
	if _, ok := RoundTripMs(sentMs+100, lsr, dlsr); ok {
		t.Fatal("arrival before LSR+DLSR is not a round trip")
	}
	if _, ok := RoundTripMs(sentMs+60_000, lsr, 0); ok {
		t.Fatal("a 60 s round trip is not plausible")
	}
}
