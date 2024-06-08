package msgs

type Msg struct {
	From    int
	ChanId  int
	GroupId int
	Val     []byte
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

type NewConnectionMsg struct {
	Address string
	GroupId int
}

type KillGroupMsg struct {
	GroupId int
}
type NewGroupMsg struct {
	Groupid int
}
type MsgWrapper struct {
	Type byte
	Val  any
}
