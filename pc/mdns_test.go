package main

import (
	"encoding/binary"
	"net"
	"testing"
)

// A query like iPhones send: two questions, the second one using a compression pointer.
func sampleQuery() []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b[4:], 2)
	b = append(b, encodeDNSName("BlanketRemote-1F0798.local")...)
	b = binary.BigEndian.AppendUint16(b, 28)     // AAAA
	b = binary.BigEndian.AppendUint16(b, 0x8001) // IN + QU
	b = append(b, 0xC0, 12)                      // same name again, compressed
	b = binary.BigEndian.AppendUint16(b, dnsTypeA)
	b = binary.BigEndian.AppendUint16(b, 1)
	return b
}

func TestParseQuery(t *testing.T) {
	_, qs, ok := parseMDNSQuery(sampleQuery())
	if !ok || len(qs) != 2 {
		t.Fatalf("parse failed: %v %v", ok, qs)
	}
	if qs[0].name != "blanketremote-1f0798.local" || qs[0].qtype != 28 || !qs[0].unicast {
		t.Fatalf("q0 wrong: %+v", qs[0])
	}
	if qs[1].name != "blanketremote-1f0798.local" || qs[1].qtype != dnsTypeA || qs[1].unicast {
		t.Fatalf("q1 wrong: %+v", qs[1])
	}
}

func TestRejectsAnswersAndJunk(t *testing.T) {
	ans := buildMDNSAnswer("x.local", net.IPv4(1, 2, 3, 4), 0, false)
	if _, _, ok := parseMDNSQuery(ans); ok {
		t.Fatal("answers must not be treated as questions")
	}
	loop := []byte{0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0xC0, 12} // pointer to itself
	if _, _, ok := parseMDNSQuery(loop); ok {
		t.Fatal("pointer loop must fail")
	}
	if _, _, ok := parseMDNSQuery([]byte{1, 2, 3}); ok {
		t.Fatal("short packet must fail")
	}
}

func TestAnswer(t *testing.T) {
	ip := net.IPv4(192, 168, 1, 23)
	m := buildMDNSAnswer("blanketremote-1f0798.local", ip, 777, false)
	if binary.BigEndian.Uint16(m[0:]) != 0 || binary.BigEndian.Uint16(m[4:]) != 0 || binary.BigEndian.Uint16(m[6:]) != 1 {
		t.Fatalf("multicast header wrong: % x", m[:12])
	}
	if got := net.IP(m[len(m)-4:]); !got.Equal(ip) {
		t.Fatalf("ip = %v", got)
	}
	l := buildMDNSAnswer("blanketremote-1f0798.local", ip, 777, true)
	if binary.BigEndian.Uint16(l[0:]) != 777 || binary.BigEndian.Uint16(l[4:]) != 1 {
		t.Fatalf("legacy header wrong: % x", l[:12])
	}
}
