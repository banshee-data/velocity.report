package l9endpoints

// FixtureReturnIdentity gives a hand-built point cloud a return identity, for
// tests and fixtures that record a VRLOG without a sensor behind it. It is
// not how a recording gets one: the adapter derives that from the packets the
// points were decoded from.
//
// The points become returns 0, 1, 2, … of packets keyed firstKey, firstKey+1,
// …, laid out as a Pandar40P's (400 returns to a packet), and the frame range
// covers those packets whole. Frames meant to come from different packets
// need firstKey values far enough apart that their keys do not meet.
func FixtureReturnIdentity(pc *PointCloudFrame, firstKey uint64) {
	const per = returnBlocksPandar40P * returnChannelsPandar40P
	n := len(pc.X)
	packets := (n + per - 1) / per
	if packets == 0 {
		packets = 1
	}
	pc.PacketKeys = make([]uint64, packets)
	for i := range pc.PacketKeys {
		pc.PacketKeys[i] = firstKey + uint64(i)
	}
	pc.ReturnRefs = make([]uint32, n)
	for i := range pc.ReturnRefs {
		pc.ReturnRefs[i] = uint32(i)
	}
	pc.ReturnStart, pc.ReturnEnd = 0, uint32(packets*per)
	pc.ReturnModel = returnModelPandar40P
	pc.ReturnBlocks, pc.ReturnChannels = returnBlocksPandar40P, returnChannelsPandar40P
}
