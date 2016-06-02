package mesh

// surrogateGossiper ignores unicasts and relays broadcasts and gossips.
type surrogateGossiper struct{}

var _ Gossiper = &surrogateGossiper{}

// OnGossipUnicast implements Gossiper.
func (*surrogateGossiper) OnGossipUnicast(sender PeerName, msg []byte) error {
	return nil
}

// OnGossipBroadcast implements Gossiper.
func (*surrogateGossiper) OnGossipBroadcast(_ PeerName, update []byte) (GossipData, error) {
	return newSurrogateGossipData(update), nil
}

// Gossip implements Gossiper.
func (*surrogateGossiper) Gossip() GossipData {
	return nil
}

// OnGossip implements Gossiper.
func (*surrogateGossiper) OnGossip(update []byte) (GossipData, error) {
	return newSurrogateGossipData(update), nil
}

// surrogateGossipData is a simple in-memory GossipData.
type surrogateGossipData struct {
	messages [][]byte
}

var _ GossipData = &surrogateGossipData{}

func newSurrogateGossipData(msg []byte) *surrogateGossipData {
	return &surrogateGossipData{messages: [][]byte{msg}}
}

// Encode implements GossipData.
func (d *surrogateGossipData) Encode() [][]byte {
	return d.messages
}

// Merge implements GossipData.
func (d *surrogateGossipData) Merge(other GossipData) GossipData {
	o := other.(*surrogateGossipData)
	messages := make([][]byte, 0, len(d.messages)+len(o.messages))
	messages = append(messages, d.messages...)
	messages = append(messages, o.messages...)
	return &surrogateGossipData{messages: messages}
}
