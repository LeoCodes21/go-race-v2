package mp

import (
	"fmt"
	"time"
	"sync"
	"sort"
)

type ClientState uint
type SyncState uint

// Represents the different states a SessionClient can be in.
// StateNoClient is the initial state; it signifies that there is no client associated.
// StateWaitingForData is the default state for initialized clients; it is active ...
// until the client has started sending car data packets.
// The client is then switched to StateActive.
const (
	StateNoClient       ClientState = 0
	StateWaitingForData ClientState = 1
	StateActive         ClientState = 2
)

const (
	SyncStateNone  SyncState = 0
	SyncStateStart SyncState = 1
	SyncStateSync  SyncState = 2
)

type BufferedPacket struct {
	SequenceNumber uint16
	Data           []SubPacket
}

type SessionClient struct {
	Session   *Session
	Client    *Client
	Buffer    []BufferedPacket
	State     ClientState
	SyncState SyncState
	Slot      int
}

// Represents a group of clients in a race.
type Session struct {
	sync.Mutex

	ID          uint32                 // The eventSessionID from the main server
	MaxClients  uint                   // The number of clients that need to be in the session
	ClientCount uint                   // The current number of clients
	SyncCounter uint16                 // Used for sync packet generation
	SyncCount   uint                   // Used to determine when the sync broadcast should occur
	Clients     map[int]*SessionClient // Client map - key is slot, value is Client instance
	ActiveCount uint

	Running bool
}

func CreateSession(sessionID uint32, clientCount int) *Session {
	session := Session{
		ID:          sessionID,
		MaxClients:  uint(clientCount),
		ClientCount: 0,
		Clients:     make(map[int]*SessionClient, clientCount),
		SyncCount:   0,
		ActiveCount: 0,
		SyncCounter: 1,
		Running:     false,
	}

	for i := 0; i < clientCount; i++ {
		session.Clients[i] = &SessionClient{
			Client:    nil,
			Session:   &session,
			Buffer:    make([]BufferedPacket, 0),
			State:     StateNoClient,
			SyncState: SyncStateNone,
		}
	}

	fmt.Printf("DEBUG: created session %d with %d slots\n", sessionID, clientCount)

	return &session
}

func (s *Session) AddClient(client *Client, slot int) *SessionClient {
	if s.Clients[slot].Client != client {
		fmt.Printf("DEBUG: added client [%d] to session [%d]\n", client.Address.Port, s.ID)
		s.ClientCount++
	}

	s.Clients[slot].Client = client
	s.Clients[slot].Slot = slot
	s.Clients[slot].State = StateWaitingForData
	s.Clients[slot].SyncState = SyncStateStart

	client.sessionClient = s.Clients[slot]

	s.Clients[slot].Client.SendSyncPacket()

	return s.Clients[slot]
}

func (s *Session) SetClientActive(slot int) {
	if s.Clients[slot].State != StateActive {
		s.Clients[slot].State = StateActive
		s.ActiveCount++

		if s.ActiveCount == s.MaxClients {
			fmt.Printf("DEBUG: all clients in session [%d] are active\n", s.ID)

			if !s.Running {
				s.Running = true

				for key, _ := range s.Clients {
					s.Clients[key].SyncState = SyncStateStart
				}

				s.BroadcastSyncPackets()
				go s.StartBroadcasting()
			}
		}
	}
}

func (s *Session) IncrementSyncCount() {
	s.SyncCount++

	if s.SyncCount == s.MaxClients {
		s.SyncCount = 0
		s.BroadcastSyncPackets()
		s.SyncCounter++
	}
}

func (s *Session) BroadcastSyncPackets() {
	for _, sc := range s.Clients {
		if sc.Client != nil {
			sc.Client.SendSyncPacket()
		}
	}
}

func (s *Session) StartBroadcasting() {
	timer := time.Tick(40 * time.Millisecond)

	for {
		s.Lock()

		if !s.Running {
			break
		}

		for key, _ := range s.Clients {
			sc := s.Clients[key]

			if sc.State == StateNoClient {
				continue
			}

			buffer := make([]BufferedPacket, len(sc.Buffer))

			copy(buffer, sc.Buffer)

			sort.Slice(buffer[:], func(i, j int) bool {
				return buffer[i].SequenceNumber <= buffer[j].SequenceNumber
			})

			for _, sc2 := range s.Clients {
				if sc2.State == StateNoClient {
					continue
				}

				if sc2.Client.Address.Port != sc.Client.Address.Port {
					for _, pkt := range buffer {
						sc2.Client.SendPlayerData(pkt, sc)
					}
				}
			}

			buffer = nil
			sc.Buffer = nil
			sc.Buffer = make([]BufferedPacket, 0)
		}

		s.Unlock()

		<-timer
	}
}

func (sc *SessionClient) BufferPacket(packet BufferedPacket) {
	if len(sc.Buffer) > 32 {
		panic("Packet buffer overflow")
	}

	sc.Buffer = append(sc.Buffer, packet)
}
