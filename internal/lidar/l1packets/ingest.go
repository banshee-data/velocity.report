package l1packets

import (
	"github.com/banshee-data/velocity.report/internal/lidar/network"
	"github.com/banshee-data/velocity.report/internal/lidar/parse"
)

// Type aliases re-export packet ingestion and parsing types from
// the network/ and parse/ subpackages. These aliases enable callers
// to import from l1packets while the implementation remains in
// dedicated subpackages.

// Ingestion types (from network/).

// UDPListener receives LiDAR packets over UDP.
type UDPListener = network.UDPListener

// UDPListenerConfig configures the UDP listener.
type UDPListenerConfig = network.UDPListenerConfig

// PCAPPacket represents a single captured network packet.
type PCAPPacket = network.PCAPPacket

// PCAPReader provides sequential access to packets in a PCAP file.
type PCAPReader = network.PCAPReader

// PCAPReaderFactory creates PCAPReader instances.
type PCAPReaderFactory = network.PCAPReaderFactory

// Constructor re-exports.

// NewUDPListener creates a configured UDP packet listener.
var NewUDPListener = network.NewUDPListener

// Parsing types (from parse/).

// Pandar40PParser parses Hesai Pandar40P LiDAR packets.
type Pandar40PParser = parse.Pandar40PParser

// Pandar40PConfig configures the Pandar40P packet parser.
type Pandar40PConfig = parse.Pandar40PConfig

// Constructor re-exports.

// NewPandar40PParser creates a parser for Pandar40P packets.
var NewPandar40PParser = parse.NewPandar40PParser
