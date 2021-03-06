package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/calmh/syncthing/discover"
)

type Node struct {
	Addresses []Address
	Updated   time.Time
}

type Address struct {
	IP   []byte
	Port uint16
}

var (
	nodes    = make(map[string]Node)
	lock     sync.Mutex
	queries  = 0
	answered = 0
)

func main() {
	var debug bool
	var listen string
	var timestamp bool

	flag.StringVar(&listen, "listen", ":22025", "Listen address")
	flag.BoolVar(&debug, "debug", false, "Enable debug output")
	flag.BoolVar(&timestamp, "timestamp", true, "Timestamp the log output")
	flag.Parse()

	log.SetOutput(os.Stdout)
	if !timestamp {
		log.SetFlags(0)
	}

	addr, _ := net.ResolveUDPAddr("udp", listen)
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			time.Sleep(600 * time.Second)

			lock.Lock()

			var deleted = 0
			for id, node := range nodes {
				if time.Since(node.Updated) > 60*time.Minute {
					delete(nodes, id)
					deleted++
				}
			}
			log.Printf("Expired %d nodes; %d nodes in registry; %d queries (%d answered)", deleted, len(nodes), queries, answered)
			queries = 0
			answered = 0

			lock.Unlock()
		}
	}()

	var buf = make([]byte, 1024)
	for {
		buf = buf[:cap(buf)]
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			panic(err)
		}
		if n < 4 {
			log.Printf("Received short packet (%d bytes)", n)
			continue
		}

		buf = buf[:n]
		magic := binary.BigEndian.Uint32(buf)

		switch magic {
		case discover.AnnouncementMagicV1:
			var pkt discover.AnnounceV1
			err := pkt.UnmarshalXDR(buf)
			if err != nil {
				log.Println("AnnounceV1 Unmarshal:", err)
				log.Println(hex.Dump(buf))
				continue
			}
			if debug {
				log.Printf("<- %v %#v", addr, pkt)
			}

			ip := addr.IP.To4()
			if ip == nil {
				ip = addr.IP.To16()
			}
			node := Node{
				Addresses: []Address{{
					IP:   ip,
					Port: pkt.Port,
				}},
				Updated: time.Now(),
			}

			lock.Lock()
			nodes[pkt.NodeID] = node
			lock.Unlock()

		case discover.QueryMagicV1:
			var pkt discover.QueryV1
			err := pkt.UnmarshalXDR(buf)
			if err != nil {
				log.Println("QueryV1 Unmarshal:", err)
				log.Println(hex.Dump(buf))
				continue
			}
			if debug {
				log.Printf("<- %v %#v", addr, pkt)
			}

			lock.Lock()
			node, ok := nodes[pkt.NodeID]
			queries++
			lock.Unlock()

			if ok && len(node.Addresses) > 0 {
				pkt := discover.AnnounceV1{
					Magic:  discover.AnnouncementMagicV1,
					NodeID: pkt.NodeID,
					Port:   node.Addresses[0].Port,
					IP:     node.Addresses[0].IP,
				}
				if debug {
					log.Printf("-> %v %#v", addr, pkt)
				}

				tb := pkt.MarshalXDR()
				_, _, err = conn.WriteMsgUDP(tb, nil, addr)
				if err != nil {
					log.Println("QueryV1 response write:", err)
				}

				lock.Lock()
				answered++
				lock.Unlock()
			}

		case discover.AnnouncementMagicV2:
			var pkt discover.AnnounceV2
			err := pkt.UnmarshalXDR(buf)
			if err != nil {
				log.Println("AnnounceV2 Unmarshal:", err)
				log.Println(hex.Dump(buf))
				continue
			}
			if debug {
				log.Printf("<- %v %#v", addr, pkt)
			}

			ip := addr.IP.To4()
			if ip == nil {
				ip = addr.IP.To16()
			}

			var addrs []Address
			for _, addr := range pkt.Addresses {
				tip := addr.IP
				if len(tip) == 0 {
					tip = ip
				}
				addrs = append(addrs, Address{
					IP:   tip,
					Port: addr.Port,
				})
			}

			node := Node{
				Addresses: addrs,
				Updated:   time.Now(),
			}

			lock.Lock()
			nodes[pkt.NodeID] = node
			lock.Unlock()

		case discover.QueryMagicV2:
			var pkt discover.QueryV2
			err := pkt.UnmarshalXDR(buf)
			if err != nil {
				log.Println("QueryV2 Unmarshal:", err)
				log.Println(hex.Dump(buf))
				continue
			}
			if debug {
				log.Printf("<- %v %#v", addr, pkt)
			}

			lock.Lock()
			node, ok := nodes[pkt.NodeID]
			queries++
			lock.Unlock()

			if ok && len(node.Addresses) > 0 {
				pkt := discover.AnnounceV2{
					Magic:  discover.AnnouncementMagicV2,
					NodeID: pkt.NodeID,
				}
				for _, addr := range node.Addresses {
					pkt.Addresses = append(pkt.Addresses, discover.Address{IP: addr.IP, Port: addr.Port})
				}
				if debug {
					log.Printf("-> %v %#v", addr, pkt)
				}

				tb := pkt.MarshalXDR()
				_, _, err = conn.WriteMsgUDP(tb, nil, addr)
				if err != nil {
					log.Println("QueryV2 response write:", err)
				}

				lock.Lock()
				answered++
				lock.Unlock()
			}
		}
	}
}
