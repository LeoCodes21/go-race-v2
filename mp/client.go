package mp

import (
	"net"
	"time"
	"bytes"
	"encoding/binary"
	"fmt"
	"encoding/hex"
)

// Represents a player connected to the server.
type Client struct {
	Address    *net.UDPAddr
	Connection *net.UDPConn

	HelloTime       uint16
	Ping            uint
	LastPacketTime  time.Time
	CreationTime    time.Time
	ControlSequence uint16
	RaceSequence    uint16

	sessionSlot   int
	session       *Session
	sessionClient *SessionClient
	instance      *Instance
}

func CreateClient(addr *net.UDPAddr, connection *net.UDPConn, helloTime uint16, creationTime time.Time, instance *Instance) *Client {
	return &Client{
		Address:      addr,
		HelloTime:    helloTime,
		CreationTime: creationTime,
		Connection:   connection,
		instance:     instance,
	}
}

func (c *Client) GetControlSequence() uint16 {
	out := c.ControlSequence

	c.ControlSequence++

	return out
}

func (c *Client) GetRaceSequence() uint16 {
	out := c.RaceSequence

	c.RaceSequence++

	return out
}

func (c *Client) GetTimeDiff() uint16 {
	return uint16(time.Now().Sub(c.CreationTime).Seconds() * 1000)
}

// Processes an incoming packet.
func (c *Client) ProcessPacket(data []byte) {
	fmt.Printf("DEBUG: Client %d is processing packet (%d bytes):\n", c.Address.Port, len(data))
	fmt.Println(hex.Dump(data))
	packetType := DetectPacketType(data)
	fmt.Printf("DEBUG: type = %d\n", packetType)

	c.Ping = uint(time.Now().Sub(c.LastPacketTime).Seconds() * 1000)
	c.LastPacketTime = time.Now()

	switch packetType {
	case SyncStart:
		c.HandleSyncStart(data)
		break
	case Sync:
		c.HandleSync(data)
		break
	case Player:
		c.HandlePlayerData(data)
		break
	case KeepAlive:
		sendKeepAlive(c)
		c.session.IncrementSyncCount()
		break
	default:
		break
	}
}

func (c *Client) HandlePlayerData(packet []byte) {
	data := packet[10 : len(packet)-5]
	reader := bytes.NewReader(data)
	sbs := make([]SubPacket, 0)
	for {
		ptype, err := reader.ReadByte()
		if err != nil {
			break
		}
		plen, _ := reader.ReadByte()
		innerData := make([]byte, plen)
		reader.Read(innerData)
		sbs = append(sbs, SubPacket{
			Type: ptype,
			Body: innerData,
		})
	}

	c.sessionClient.BufferPacket(BufferedPacket{
		Data:           sbs,
		SequenceNumber: binary.BigEndian.Uint16(data[6:8]),
	})

	c.session.SetClientActive(c.sessionClient.Slot)
}

func (c *Client) SendPlayerData(packet BufferedPacket, client *SessionClient) {
	buf := new(bytes.Buffer)
	buf.WriteByte(0x01)                                      // 0
	buf.WriteByte(byte(client.Slot))                         // 1
	binary.Write(buf, binary.BigEndian, c.GetRaceSequence()) // 2, 3
	buf.Write([]byte{0xff, 0xff, 0xff, 0xff})

	for _, p := range packet.Data {
		body := clone(p.Body)
		if p.Type == 0x12 && len(body) >= 26 {
			td := c.GetTimeDiff()
			body[0] = byte(td >> 8)
			body[1] = byte(td & 0xff)
		}
		buf.WriteByte(p.Type)
		buf.WriteByte(byte(len(body)))
		buf.Write(body)
	}
	buf.Write([]byte{0xff, 0x01, 0x01, 0x01, 0x01})
	c.SendBuffer(buf)
}

func (c *Client) HandleSync(data []byte) {
	sc := c.session.Clients[c.sessionClient.Slot]

	if sc.SyncState != SyncStateNone {
		return
	}

	sc.SyncState = SyncStateSync

	c.session.IncrementSyncCount()
}

func (c *Client) HandleSyncStart(data []byte) {
	syncStart := ClientSyncStart{}
	reader := bytes.NewReader(data)

	binary.Read(reader, binary.BigEndian, &syncStart)

	session, ok := c.instance.Sessions[syncStart.Payload.SessionID]

	if !ok {
		session = CreateSession(syncStart.Payload.SessionID, int(syncStart.Payload.PlayerSlot&0x0f>>1))
		c.instance.Sessions[syncStart.Payload.SessionID] = session
	}

	c.session = session

	if !session.Running {
		slot := int(syncStart.Payload.PlayerSlot >> 5)
		sc, _ := session.Clients[slot]

		c.sessionSlot = slot

		if sc.State != StateActive {
			c.sessionClient = session.AddClient(c, c.sessionSlot)
		}
	} else {
		session.IncrementSyncCount()
	}
}

func (c *Client) SendSyncPacket() {
	sc := c.session.Clients[c.sessionSlot]

	switch sc.SyncState {
	case SyncStateStart:
		sendSyncStart(c)
		break
	case SyncStateSync:
		sendSync(c)
		break
	default:
		fmt.Printf("unhandled state: %d\n", sc.SyncState)
		break
	}

	sc.SyncState = SyncStateNone
}

func sendSyncStart(client *Client) {
	packet := ServerSyncStart{}

	packet.Counter = client.GetControlSequence()
	packet.TypeSRV = 0x02
	packet.Time = client.GetTimeDiff()
	packet.HelloTime = client.HelloTime - 5

	if client.session.SyncCounter == 0 {
		packet.UnknownCounter = 0
		packet.HandshakeSync = 0xFFFF
	} else {
		packet.UnknownCounter = client.session.SyncCounter
		packet.HandshakeSync = 0xFFFF &^ (1 << (16 - packet.UnknownCounter))
	}

	packet.Payload = ServerSyncStartPayload{}
	packet.Payload.SessionID = client.session.ID
	packet.Payload.Size = 0x06
	packet.Payload.GridIndex = byte(client.sessionClient.Slot)

	var syncSlots byte

	for i := uint(0); i < client.session.MaxClients; i++ {
		syncSlots |= 1 << i
	}

	packet.Payload.SyncSlots = syncSlots

	packet.Payload.PacketEnd = 0xff

	packet.Checksum = 0x01010101

	client.Send(packet)
}

func sendSync(client *Client) {
	packet := ServerSync{}

	packet.Counter = client.GetControlSequence()
	packet.TypeSRV = 0x02
	packet.Time = client.GetTimeDiff()
	packet.HelloTime = client.HelloTime

	if client.session.SyncCounter == 0 {
		packet.UnknownCounter = 0xFFFF
		packet.HandshakeSync = 0xFFFF
	} else {
		packet.UnknownCounter = client.session.SyncCounter
		packet.HandshakeSync = 0xFFFF &^ (1 << (16 - packet.UnknownCounter))
	}

	packet.Payload = ServerSyncPayload{}
	packet.Payload.Type = 0x01
	packet.Payload.Size = 0x03
	packet.Payload.Unknown = [3]byte{0x00, 0x40, 0xb7}
	packet.Payload.PacketEnd = 0xff

	packet.Checksum = 0x01010101

	client.Send(packet)
}

func sendKeepAlive(client *Client) {
	packet := ServerKeepAlive{}

	packet.Counter = client.GetControlSequence()
	packet.TypeSRV = 0x02
	packet.Time = client.GetTimeDiff()
	packet.HelloTime = client.HelloTime
	if client.session.SyncCounter == 0 {
		packet.UnknownCounter = 0xFFFF
		packet.HandshakeSync = 0xFFFF
	} else {
		packet.UnknownCounter = client.session.SyncCounter
		packet.HandshakeSync = 0xFFFF &^ (1 << (16 - packet.UnknownCounter))
	}
	packet.PacketEnd = 0xff
	packet.Checksum = 0x01010101

	client.Send(packet)
}

func (c *Client) SendBuffer(buffer *bytes.Buffer) {
	c.Connection.WriteToUDP(buffer.Bytes(), c.Address)

	fmt.Printf("DEBUG: Sent packet to client %d:\n", c.Address.Port)
	fmt.Println(hex.Dump(buffer.Bytes()))
}

// Sends a data packet to the client.
func (c *Client) Send(data interface{}) {
	buffer := &bytes.Buffer{}

	binary.Write(buffer, binary.BigEndian, data)

	c.Connection.WriteToUDP(buffer.Bytes(), c.Address)

	fmt.Printf("DEBUG: Sent packet to client %d:\n", c.Address.Port)
	fmt.Println(hex.Dump(buffer.Bytes()))
}
