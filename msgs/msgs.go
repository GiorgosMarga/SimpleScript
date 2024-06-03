package msgs

import "net"

type Msg struct {
	To   int
	Val  []byte
	Conn net.Conn `gob:"-"`
}
