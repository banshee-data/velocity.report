package labelset

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// PacketKeyScheme names how a packet key is derived, so a reader can refuse a
// table made some other way.
const PacketKeyScheme = "sha256-64:udp-payload"

// PacketKey identifies one sensor packet by its content: the first 64 bits of
// SHA-256 over the UDP payload, read big-endian. It is independent of the file
// the packet is in, its position there and its capture timestamp.
type PacketKey uint64

// KeyOf returns the key of a UDP payload.
func KeyOf(payload []byte) PacketKey {
	sum := sha256.Sum256(payload)
	return PacketKey(binary.BigEndian.Uint64(sum[:8]))
}

// String renders the key as 16 lower-case hex digits.
func (k PacketKey) String() string {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(k))
	return hex.EncodeToString(b[:])
}

// MarshalJSON writes the key as a hex string, since JSON numbers lose
// precision above 2^53.
func (k PacketKey) MarshalJSON() ([]byte, error) { return json.Marshal(k.String()) }

// UnmarshalJSON reads the hex form written by MarshalJSON.
func (k *PacketKey) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := ParsePacketKey(s)
	if err != nil {
		return err
	}
	*k = parsed
	return nil
}

// ParsePacketKey reads 16 hex digits.
func ParsePacketKey(s string) (PacketKey, error) {
	if len(s) != 16 {
		return 0, fmt.Errorf("packet key %q is not 16 hex digits", s)
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return 0, fmt.Errorf("packet key %q: %w", s, err)
	}
	return PacketKey(binary.BigEndian.Uint64(b)), nil
}

// Layout is how a sensor's packet divides into returns. A return number is
// packet_index × Blocks × Channels + block × Channels + (channel − 1), with
// block counted from 0 and channel from 1, as the Pandar40P parser numbers
// them.
type Layout struct {
	Model    string `json:"model"`
	Blocks   int    `json:"blocks"`
	Channels int    `json:"channels"`
}

// Pandar40P is the layout of a Hesai Pandar40P data packet: ten blocks of
// forty channels.
var Pandar40P = Layout{Model: "hesai-pandar40p", Blocks: 10, Channels: 40}

// PerPacket is the number of returns one packet can carry.
func (l Layout) PerPacket() uint64 { return uint64(l.Blocks) * uint64(l.Channels) }

// Validate refuses a layout that cannot number returns.
func (l Layout) Validate() error {
	if l.Model == "" {
		return fmt.Errorf("layout has no sensor model")
	}
	if l.Blocks < 1 || l.Channels < 1 || l.Blocks > 1<<10 || l.Channels > 1<<10 {
		return fmt.Errorf("layout %s: %d blocks of %d channels", l.Model, l.Blocks, l.Channels)
	}
	return nil
}

// Return numbers a return within a packet table.
func (l Layout) Return(packetIndex, block, channel int) (uint64, error) {
	if packetIndex < 0 || block < 0 || block >= l.Blocks || channel < 1 || channel > l.Channels {
		return 0, fmt.Errorf("return (packet %d, block %d, channel %d) is outside layout %s", packetIndex, block, channel, l.Model)
	}
	return uint64(packetIndex)*l.PerPacket() + uint64(block)*uint64(l.Channels) + uint64(channel-1), nil
}

// Locate inverts Return.
func (l Layout) Locate(r uint64) (packetIndex, block, channel int) {
	per := l.PerPacket()
	within := r % per
	return int(r / per), int(within / uint64(l.Channels)), int(within%uint64(l.Channels)) + 1
}

// ReturnRef is a return identified independently of any table.
type ReturnRef struct {
	Key     PacketKey
	Block   int
	Channel int
}
