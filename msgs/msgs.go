package msgs

import "net"

type Msg struct {
	From    int
	ChanId  int
	GroupId int
	Val     []byte
	Conn    net.Conn `gob:"-"`
}

type MsgForEncoding struct {
	GroupId int
	From    int
	Val     []byte
	ChanId  int
}

type InterProcessMsg struct {
	Val  []byte
	From int
}
