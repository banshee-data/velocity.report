package annotation

// FixtureReturns fabricates the returns a hand-built pack needs, for tests
// and synthetic fixtures (internal/lidar/perframeeval/evalfixture) that write
// packs without a capture behind them. It is not how a real pack gets its
// returns: Export takes those from the recording, where they name packets
// that exist.
//
// Each sample is given packets of its own, enough for its points at 400
// returns to a packet (at least one), laid out as a Pandar40P's. Its points
// become consecutive returns from the start of its first packet, and its
// frame range covers its packets whole. FixtureReturns sets each sample's
// ReturnStart and ReturnEnd in place, so pass the same slice to WritePack.
//
// The packet keys are deterministic: the same sample point counts always give
// the same table, so a fixture's pack digest is stable. They are not the keys
// of any real packet.
func FixtureReturns(samples []Sample) PackReturns {
	layout := ReturnLayout{Model: "hesai-pandar40p", Blocks: 10, Channels: 40}
	per := layout.PerPacket()
	r := PackReturns{Layout: layout, Returns: make([][]uint64, len(samples))}
	for i := range samples {
		n := uint64(max(samples[i].PointCount, 0))
		packets := max((n+per-1)/per, 1)
		first := uint64(len(r.Packets))
		for j := uint64(0); j < packets; j++ {
			r.Packets = append(r.Packets, fixturePacketKey(first+j))
		}
		returns := make([]uint64, n)
		for j := range returns {
			returns[j] = first*per + uint64(j)
		}
		r.Returns[i] = returns
		samples[i].ReturnStart = first * per
		samples[i].ReturnEnd = (first + packets) * per
	}
	return r
}

// fixturePacketKey spreads packet indices over the key space. The multiplier
// is odd, so distinct indices give distinct keys, and only index 2^64−1,
// which no fixture reaches, would give zero.
func fixturePacketKey(i uint64) uint64 { return (i + 1) * 0x9e3779b97f4a7c15 }
