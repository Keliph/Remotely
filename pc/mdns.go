package main

// A tiny Multicast DNS (Bonjour) responder that answers one question:
// "where is blanketremote-xxxxxx.local?". iPhones (Safari) and Macs resolve .local names
// natively, so a home-screen link saved with this name keeps working when the PC's IP changes.

import (
	"encoding/binary"
	"log"
	"net"
	"strings"
	"time"
)

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

const (
	dnsTypeA   = 1
	dnsTypeANY = 255
)

type mdnsQuestion struct {
	name    string // lower case, no trailing dot
	qtype   uint16
	unicast bool // the "QU" bit: asker wants a direct reply
}

// mdnsResponder runs forever. It keeps one listener per network card and re-checks the cards every
// 30 seconds (Wi-Fi reconnects, cable plugged in...).
func mdnsResponder(name string) {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	active := map[string]*net.UDPConn{}
	failed := map[string]bool{}
	for {
		want := map[string]net.Interface{}
		ifaces, _ := net.Interfaces()
		for _, ifi := range ifaces {
			if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 || ifi.Flags&net.FlagMulticast == 0 {
				continue
			}
			if ips := ipv4Nets(ifi); len(ips) > 0 {
				want[ifaceKey(ifi, ips)] = ifi
			}
		}
		for k, c := range active {
			if _, ok := want[k]; !ok {
				c.Close()
				delete(active, k)
			}
		}
		for k, ifi := range want {
			if _, ok := active[k]; ok {
				continue
			}
			ifi := ifi
			c, err := net.ListenMulticastUDP("udp4", &ifi, mdnsGroup)
			if err != nil {
				if !failed[k] {
					log.Printf("mdns: %s: %v", ifi.Name, err)
					failed[k] = true
				}
				continue
			}
			active[k] = c
			go serveMDNS(c, ifi, name)
		}
		time.Sleep(30 * time.Second)
	}
}

func ipv4Nets(ifi net.Interface) []*net.IPNet {
	var out []*net.IPNet
	addrs, _ := ifi.Addrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
			out = append(out, n)
		}
	}
	return out
}

func ifaceKey(ifi net.Interface, nets []*net.IPNet) string {
	parts := []string{ifi.Name}
	for _, n := range nets {
		parts = append(parts, n.String())
	}
	return strings.Join(parts, "|")
}

func serveMDNS(c *net.UDPConn, ifi net.Interface, name string) {
	buf := make([]byte, 9000)
	for {
		n, src, err := c.ReadFromUDP(buf)
		if err != nil {
			return // listener closed
		}
		id, qs, ok := parseMDNSQuery(buf[:n])
		if !ok {
			continue
		}
		var ask *mdnsQuestion
		for i := range qs {
			if qs[i].name == name && (qs[i].qtype == dnsTypeA || qs[i].qtype == dnsTypeANY) {
				ask = &qs[i]
				break
			}
		}
		if ask == nil {
			continue
		}
		// A "legacy" asker sends from a random port and expects a classic DNS reply straight back.
		legacy := src.Port != mdnsGroup.Port
		ip, thisCard := answerIP(src.IP, ifi)
		if ip == nil || (!legacy && !thisCard) {
			continue // the listener on the asker's network card answers instead
		}
		resp := buildMDNSAnswer(name, ip, id, legacy)
		if legacy || ask.unicast {
			_, _ = c.WriteToUDP(resp, src)
		} else {
			_, _ = c.WriteToUDP(resp, mdnsGroup)
		}
	}
}

// answerIP picks this PC's address on the asker's network. thisCard reports whether that address
// belongs to the card this listener is on.
func answerIP(asker net.IP, ifi net.Interface) (net.IP, bool) {
	ifaces, _ := net.Interfaces()
	for _, other := range ifaces {
		if other.Flags&net.FlagUp == 0 {
			continue
		}
		for _, n := range ipv4Nets(other) {
			if n.Contains(asker) {
				return n.IP.To4(), other.Name == ifi.Name
			}
		}
	}
	if nets := ipv4Nets(ifi); len(nets) > 0 {
		return nets[0].IP.To4(), true
	}
	return nil, false
}

func parseMDNSQuery(b []byte) (uint16, []mdnsQuestion, bool) {
	if len(b) < 12 {
		return 0, nil, false
	}
	id := binary.BigEndian.Uint16(b[0:])
	if binary.BigEndian.Uint16(b[2:])&0x8000 != 0 {
		return 0, nil, false // that's an answer, not a question
	}
	count := int(binary.BigEndian.Uint16(b[4:]))
	off := 12
	var qs []mdnsQuestion
	for i := 0; i < count && i < 64; i++ {
		name, next, ok := readDNSName(b, off)
		if !ok || next+4 > len(b) {
			return 0, nil, false
		}
		qs = append(qs, mdnsQuestion{
			name:    strings.ToLower(name),
			qtype:   binary.BigEndian.Uint16(b[next:]),
			unicast: binary.BigEndian.Uint16(b[next+2:])&0x8000 != 0,
		})
		off = next + 4
	}
	return id, qs, true
}

// readDNSName reads a (possibly compressed) DNS name and returns it plus the offset just after it.
func readDNSName(b []byte, off int) (string, int, bool) {
	var labels []string
	next := -1
	for jumps := 0; jumps < 20 && len(labels) < 128; {
		if off >= len(b) {
			return "", 0, false
		}
		l := int(b[off])
		switch {
		case l == 0:
			if next < 0 {
				next = off + 1
			}
			return strings.Join(labels, "."), next, true
		case l&0xC0 == 0xC0: // pointer to a name earlier in the packet
			if off+1 >= len(b) {
				return "", 0, false
			}
			if next < 0 {
				next = off + 2
			}
			off = int(binary.BigEndian.Uint16(b[off:]) & 0x3FFF)
			jumps++
		case l&0xC0 != 0:
			return "", 0, false
		default:
			if off+1+l > len(b) {
				return "", 0, false
			}
			labels = append(labels, string(b[off+1:off+1+l]))
			off += 1 + l
		}
	}
	return "", 0, false
}

func encodeDNSName(name string) []byte {
	var out []byte
	for _, l := range strings.Split(name, ".") {
		if l == "" || len(l) > 63 {
			continue
		}
		out = append(out, byte(len(l)))
		out = append(out, l...)
	}
	return append(out, 0)
}

func buildMDNSAnswer(name string, ip net.IP, id uint16, legacy bool) []byte {
	enc := encodeDNSName(name)
	b := make([]byte, 12, 12+2*len(enc)+20)
	questions := uint16(0)
	if legacy {
		questions = 1 // classic DNS replies repeat the question
	} else {
		id = 0 // multicast replies use id 0
	}
	binary.BigEndian.PutUint16(b[0:], id)
	binary.BigEndian.PutUint16(b[2:], 0x8400) // response, authoritative
	binary.BigEndian.PutUint16(b[4:], questions)
	binary.BigEndian.PutUint16(b[6:], 1) // one answer
	if legacy {
		b = append(b, enc...)
		b = binary.BigEndian.AppendUint16(b, dnsTypeA)
		b = binary.BigEndian.AppendUint16(b, 1) // class IN
	}
	class, ttl := uint16(0x8001), uint32(120) // IN + "cache flush": this is the only right answer
	if legacy {
		class, ttl = 1, 10
	}
	b = append(b, enc...)
	b = binary.BigEndian.AppendUint16(b, dnsTypeA)
	b = binary.BigEndian.AppendUint16(b, class)
	b = binary.BigEndian.AppendUint32(b, ttl)
	b = binary.BigEndian.AppendUint16(b, 4)
	return append(b, ip.To4()...)
}
