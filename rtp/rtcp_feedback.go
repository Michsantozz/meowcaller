package rtp

import "encoding/binary"

// RtcpPtRr is the RFC 3550 Receiver Report packet type.
const RtcpPtRr uint8 = 201

// ParseReceptionReportFor finds, in a compound RTCP packet, the reception report
// block the peer wrote about mediaSsrc (our stream): how much of it was lost and
// how much it jittered on arrival, plus the LSR/DLSR pair for the round trip.
//
// Only the first block of each SR/RR is read: WhatsApp appends transport
// statistics (24 bytes in 1:1 calls, 8 in group calls) after each block, so the
// RFC 3550 24-byte stride does not hold past the first one.
func ParseReceptionReportFor(data []byte, mediaSsrc uint32) (RtcpReceptionReport, bool) {
	for offset := 0; offset < len(data); {
		if offset+4 > len(data) {
			return RtcpReceptionReport{}, false
		}
		packetLen := (int(binary.BigEndian.Uint16(data[offset+2:offset+4])) + 1) * 4
		if packetLen < 4 || offset+packetLen > len(data) {
			return RtcpReceptionReport{}, false
		}
		packet := data[offset : offset+packetLen]
		offset += packetLen
		if packet[0]&0x1f == 0 {
			continue
		}
		var block int
		switch packet[1] {
		case RtcpPtSr:
			block = 28
		case RtcpPtRr:
			block = 8
		default:
			continue
		}
		if len(packet) < block+24 || binary.BigEndian.Uint32(packet[block:block+4]) != mediaSsrc {
			continue
		}
		b := packet[block : block+24]
		lost := int32(uint32(b[5])<<16 | uint32(b[6])<<8 | uint32(b[7]))
		if lost&0x80_0000 != 0 {
			lost -= 1 << 24 // 24-bit two's complement
		}
		return RtcpReceptionReport{
			Ssrc:                       mediaSsrc,
			FractionLost:               b[4],
			CumulativeLost:             lost,
			ExtendedHighestSequence:    binary.BigEndian.Uint32(b[8:12]),
			Jitter:                     binary.BigEndian.Uint32(b[12:16]),
			LastSenderReport:           binary.BigEndian.Uint32(b[16:20]),
			DelaySinceLastSenderReport: binary.BigEndian.Uint32(b[20:24]),
		}, true
	}
	return RtcpReceptionReport{}, false
}

// CompactNTP is the middle 32 bits of the NTP timestamp BuildSenderReport writes
// for nowMs: the value a peer echoes back as LSR.
func CompactNTP(nowMs uint64) uint32 {
	ntpSec := uint32((nowMs / 1000) + ntpUnixOffsetSecs)
	ntpFrac := uint32(float64(nowMs%1000) / 1000.0 * 4294967296.0)
	return ntpSec<<16 | ntpFrac>>16
}

// maxPlausibleRTT bounds RoundTripMs: anything longer is a clock or parsing
// artifact, not a network round trip.
const maxPlausibleRTT = 10 * 65_536 // 10 s in 1/65536 s units

// RoundTripMs is the RFC 3550 round trip from a reception report about our
// stream that arrived at arrivalMs: arrival - LSR - DLSR. ok is false when the
// peer has not received any of our Sender Reports yet (LSR 0) or the result is
// not plausible.
func RoundTripMs(arrivalMs uint64, lsr, dlsr uint32) (float64, bool) {
	if lsr == 0 {
		return 0, false
	}
	rtt := CompactNTP(arrivalMs) - lsr - dlsr // wraps negative into a huge value
	if rtt > maxPlausibleRTT {
		return 0, false
	}
	return float64(rtt) * 1000 / 65_536, true
}
