package discovery

import "net"

// Inquiry represents an inquiry type in discovery requests and responses.
type Inquiry byte

// These constants define types of discovery inquiries.
const (
	Dnop  Inquiry = iota // no operation which indicates a simple dummy message
	Dping                // a ping request
)

// the map of firendly discovery inquiry names
var ops = map[Inquiry]string{
	Dnop:  "NOP",
	Dping: "PING",
}

// Request represents an inquiry to other nodes.
type Request struct {
	inq     Inquiry      // a type of inquiry
	addr    *net.UDPAddr // client address
	payload []byte       // request payload
}

// Response represents a reply for inquiry from other nodes.
type Response struct {
	inq     Inquiry      // a type of inquiry
	addr    *net.UDPAddr // client address
	payload []byte       // request payload
}
