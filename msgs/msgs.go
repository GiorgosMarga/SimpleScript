package msgs

import "net"

type Msg struct {
	From    int
	ChanId  int
	GroupId int
	Val     []byte
	To      string
	Conn    net.Conn `gob:"-"`
}

type MsgForEncoding struct {
	GroupId int
	From    int
	Val     []byte
	ChanId  int
	To      string
}

type InterProcessMsg struct {
	Val  []byte
	From int
}
type MigrationInfoMsg struct {
	GroupId    int
	ThreadId   int
	MigratedTo string
}
type LoadBalanceMsg struct {
	Val  int
	From string
}
