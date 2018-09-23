package mp

// Represents a group of clients in a race.
type Session struct {
	ID          uint32          // The eventSessionID from the main server
	MaxClients  uint            // The number of clients that need to be in the session
	ClientCount uint            // The current number of clients
	SyncCounter uint16          // Used for sync packet generation
	SyncCount   uint            // Used to determine when the sync broadcast should occur
	Clients     map[int]*Client // Client map - key is slot, value is Client instance
	Running     bool
}

func CreateSession(sessionID uint32, clientCount int) *Session {
	session := Session{
		ID:          sessionID,
		MaxClients:  uint(clientCount),
		ClientCount: 0,
		Clients:     make(map[int]*Client, clientCount),
		SyncCount:   0,
		SyncCounter: 1,
		Running:     false,
	}

	for i := 0; i < clientCount; i++ {
		session.Clients[i] = nil
	}

	return &session
}

func (s *Session) AddClient(client *Client, slot int) {
	s.Clients[slot] = client

	if s.ClientCount+1 >= s.MaxClients && !s.Running {
		s.BroadcastSyncPackets()
		s.Running = true
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
	for _, c := range s.Clients {
		c.SendSyncPacket()
	}
}

func (s *Session) BroadcastPlayerPacket(data []SubPacket, fullData []byte, client *Client) {
	for _, c := range s.Clients {
		if c.SessionSlot != client.SessionSlot {
			c.SendPlayerData(data, fullData, client)
		}
	}
}
