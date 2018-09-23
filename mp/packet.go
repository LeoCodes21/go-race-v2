package mp

// This file consists of structures that are used for packet processing.
// These can be passed around, but should not be considered user-friendly.

type PacketType uint

const (
	Unknown   PacketType = 0
	SyncStart PacketType = 1
	Sync      PacketType = 2
	KeepAlive PacketType = 3
	Player    PacketType = 4
)

type ClientHello struct {
	TypeMP       byte   // 0
	Counter      uint16 // 0 on first attempt, increments on every consecutive attempt - although it really should always be 0
	TypeSRV      byte   // 6
	CryptoTicket [32]byte
	CryptoIV     [32]byte
	Unknown1     byte
	HelloTime    uint16 // increases by 5000 on every attempt. unsure what initial value is
	Checksum     uint32 // CRC32 checksum, currently unknown poly+init calculation
}

type ServerHello struct {
	TypeMP    byte   // 0
	Counter   uint16 // should always be zero
	TypeSRV   byte
	Time      uint16 // NOT HELLO TIME! timeDiff
	HelloTime uint16
	Checksum  uint32 // 0x01010101
}

type ClientSyncStartPayload struct {
	Type       byte
	Size       byte
	Unknown    byte
	SessionID  uint32
	PlayerSlot byte // max players = & 0x0f >> 1, slot = >> 5
	PacketEnd  byte
}

type ClientSyncStart struct {
	TypeMP         byte
	Counter        uint16
	TypeSRV        byte
	TypeSRV2       byte
	Time           uint16
	HelloTime      uint16
	UnknownCounter uint16
	HandshakeSync  uint16
	Payload        ClientSyncStartPayload
	Checksum       uint32
}

type ServerSyncStartPayload struct {
	Type      byte
	Size      byte
	GridIndex byte
	SessionID uint32
	SyncSlots byte // bit set for each slot
	PacketEnd byte
}

type ServerSyncStart struct {
	TypeMP         byte
	Counter        uint16
	TypeSRV        byte
	Time           uint16
	HelloTime      uint16
	UnknownCounter uint16
	HandshakeSync  uint16
	Payload        ServerSyncStartPayload
	Checksum       uint32
}

type ServerSyncPayload struct {
	Type      byte
	Size      byte
	Unknown   [3]byte
	PacketEnd byte
}

type ServerSync struct {
	TypeMP         byte
	Counter        uint16
	TypeSRV        byte
	Time           uint16
	HelloTime      uint16
	UnknownCounter uint16
	HandshakeSync  uint16
	Payload        ServerSyncPayload
	Checksum       uint32
}

type ServerKeepAlive struct {
	TypeMP         byte
	Counter        uint16
	TypeSRV        byte
	Time           uint16
	HelloTime      uint16
	UnknownCounter uint16
	HandshakeSync  uint16
	PacketEnd      byte
	Checksum       uint32
}

type SubPacket struct {
	Type byte
	Body []byte
}

func (p *SubPacket) Bytes() []byte {
	out := make([]byte, len(p.Body))
	out[0] = byte(p.Type)
	out[1] = byte(len(p.Body))
	copy(out[2:], p.Body)
	return out
}

func clone(a []byte) []byte {
	out := make([]byte, len(a))
	copy(out, a)
	return out
}

func DetectPacketType(data []byte) PacketType {
	switch len(data) {
	case 26:
		return SyncStart
	case 22:
		return Sync
	case 18:
		return KeepAlive
	default:
		break
	}

	if len(data) >= 16 {
		if data[0] == 0x01 {
			return Player
		}
	}

	return Unknown
}
