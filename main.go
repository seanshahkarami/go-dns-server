package main

import (
	"log"
	"net"

	"golang.org/x/net/dns/dnsmessage"
)

func mustResolveUDPAddr(network string, address string) *net.UDPAddr {
	addr, err := net.ResolveUDPAddr(network, address)

	if err != nil {
		panic(err)
	}

	return addr
}

var upstreams = []*net.UDPAddr{
	mustResolveUDPAddr("udp", "8.8.8.8:53"),
}

var aresources = map[dnsmessage.Name]*dnsmessage.AResource{
	dnsmessage.MustNewName("google.com."):   &dnsmessage.AResource{A: [4]byte{172, 217, 1, 46}},
	dnsmessage.MustNewName("terraria.org."): &dnsmessage.AResource{A: [4]byte{104, 22, 6, 117}},
}

func handleMessage(conn net.PacketConn, data []byte, from net.Addr) error {
	var msg dnsmessage.Message

	if err := msg.Unpack(data); err != nil {
		return err
	}

	if msg.Header.Response {
		log.Printf("response from %v\n", from)

		for _, a := range msg.Answers {
			switch r := a.Body.(type) {
			case *dnsmessage.AResource:
				aresources[a.Header.Name] = r
			default:
				log.Printf("unexpected answer %v\n", a)
			}
		}
	} else {
		log.Printf("request from %v\n", from)

		for _, q := range msg.Questions {
			switch q.Type {
			case dnsmessage.TypeA:
				handleResourceA(conn, msg.Header, q, from)
			default:
				handleNotImplemented(conn, msg.Header, q, from)
			}
		}
	}

	return nil
}

func handleResourceA(conn net.PacketConn, h dnsmessage.Header, q dnsmessage.Question, from net.Addr) {
	if r, ok := aresources[q.Name]; ok {
		log.Printf("aresource %s %v.\n", q.Name, r.A)

		msg := dnsmessage.Message{
			Header: dnsmessage.Header{
				ID:       h.ID,
				Response: true,
				RCode:    dnsmessage.RCodeSuccess,
			},
			Answers: []dnsmessage.Resource{
				{
					Header: dnsmessage.ResourceHeader{
						Name:  q.Name,
						Type:  q.Type,
						Class: q.Class,
					},
					Body: r,
				},
			},
		}

		if data, err := msg.Pack(); err == nil {
			conn.WriteTo(data, from)
		}
	} else {
		log.Printf("aresource %s none. checking upstreams.\n", q.Name)

		msg := dnsmessage.Message{
			Header: dnsmessage.Header{
				ID: h.ID,
			},
			Questions: []dnsmessage.Question{q},
		}

		if data, err := msg.Pack(); err == nil {
			for _, upstream := range upstreams {
				conn.WriteTo(data, upstream)
			}
		}
	}
}

func handleNotImplemented(conn net.PacketConn, h dnsmessage.Header, q dnsmessage.Question, from net.Addr) {
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:       h.ID,
			Response: true,
			RCode:    dnsmessage.RCodeNotImplemented,
		},
	}

	if data, err := msg.Pack(); err == nil {
		conn.WriteTo(data, from)
	}
}

func main() {
	var buf [512]byte

	conn, err := net.ListenPacket("udp", ":53")

	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close()

	for {
		n, from, err := conn.ReadFrom(buf[:])

		if err != nil {
			log.Fatal(err)
		}

		if err := handleMessage(conn, buf[:n], from); err != nil {
			log.Printf("error %s\n", err)
		}
	}
}
