package mp

import (
	"net"
	"sync"
	"fmt"
	"bytes"
	"encoding/binary"
	"time"
)

// Start the UDP server and begin listening for packets
func Start(addrStr string) (*Instance, error) {
	addr, err := net.ResolveUDPAddr("udp", addrStr)
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	i := NewInstance(listener)
	go i.RunPacketRead()
	go i.GarbageCollector()
	return i, nil
}

func NewInstance(listener *net.UDPConn) *Instance {
	return &Instance{
		listener: listener,
		buffer:   make([]byte, 1024),
		Clients:  make(map[int]*Client),
		Sessions: make(map[uint32]*Session),
	}
}

type Instance struct {
	sync.Mutex
	listener *net.UDPConn
	buffer   []byte
	Clients  map[int]*Client
	Sessions map[uint32]*Session
}

func (i *Instance) GarbageCollector() {
	timer := time.Tick(time.Second * 10)

	for {
		i.Lock()

		for key, client := range i.Clients {
			if time.Now().Sub(client.LastPacketTime).Seconds() > 5 {
				if client.session != nil {
					delete(client.session.Clients, client.SessionSlot)
					client.session.ClientCount--
				}

				delete(i.Clients, key)

				fmt.Printf("Removed dead client [%d]\n", key)
			}
		}

		for key, session := range i.Sessions {
			if session.ClientCount == 0 && session.Running {
				delete(i.Sessions, key)
				fmt.Printf("Removed session [%d]\n", key)
			}
		}

		i.Unlock()
		<-timer
	}
}

func (i *Instance) RunPacketRead() {
	for {
		addr, data := i.readPacket()
		i.Lock()

		// Check for existing client
		client, exists := i.Clients[addr.Port]

		if !exists {
			// Only handle HELLO
			if len(data) == 75 && data[0] == 0x00 && data[3] == 0x06 {
				creationTime := time.Now()
				reader := bytes.NewReader(data)
				cliHello := ClientHello{}

				binary.Read(reader, binary.BigEndian, &cliHello)

				client = CreateClient(addr, i.listener, cliHello.HelloTime, creationTime, i)

				srvHello := ServerHello{}
				srvHello.TypeSRV = 0x01
				srvHello.Counter = client.GetControlSequence()
				srvHello.Time = client.GetTimeDiff() + 10
				srvHello.HelloTime = cliHello.HelloTime
				srvHello.Checksum = 0x01010101

				client.Send(srvHello)

				i.Clients[addr.Port] = client
			} else {
				fmt.Printf("WARN: Received non-hello packet from unknown client [%d]\n", addr.Port)
			}
		} else {
			client.ProcessPacket(data)
		}

		i.Unlock()
	}
}

func (i *Instance) readPacket() (*net.UDPAddr, []byte) {
	pktLen, addr, _ := i.listener.ReadFromUDP(i.buffer)
	return addr, i.buffer[:pktLen]
}
