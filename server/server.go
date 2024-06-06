package server

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/GiorgosMarga/simplescript/interpreter"
	"github.com/GiorgosMarga/simplescript/msgs"
)

func init() {
	gob.Register(msgs.InterProcessMsg{})
	gob.Register(msgs.LoadBalanceMsg{})
	gob.Register(msgs.MsgForEncoding{})
	gob.Register(msgs.MigrationInfoMsg{})
	gob.Register(msgs.Msg{})
}

// const StreamByte = byte(0)
const (
	MigrationMsg   = 0x0
	SendMsg        = 0x1
	LoadBalanceMsg = 0x2
	MigrateInfoMsg = 0x3
)

type Server struct {
	IpAddr        string
	Port          string
	ln            net.Listener
	MigrationChan chan *interpreter.Interpreter
	MsgChan       chan any
	Peers         map[string]net.Conn
}

func NewServer(ipAddr, port string) *Server {
	return &Server{
		IpAddr:        ipAddr,
		Port:          port,
		MigrationChan: make(chan *interpreter.Interpreter),
		MsgChan:       make(chan any),
		Peers:         make(map[string]net.Conn),
	}
}

func (s *Server) ListenAndAccept() error {
	var err error
	s.ln, err = net.Listen("tcp", s.Port)
	if err != nil {
		return err
	}
	defer s.ln.Close()
	fmt.Printf("Environment server listening (%s)\n", s.Address())
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}
		go s.HandleConn(conn, false)
	}
}
func (s *Server) Migrate(ipAddr string, i *interpreter.Interpreter) (net.Conn, error) {
	conn, ok := s.Peers[ipAddr]
	if !ok {
		fmt.Println("Peers isn't connected:", ipAddr)
		return nil, fmt.Errorf("peer isn't connected: %s", ipAddr)
	}
	if _, err := conn.Write([]byte{MigrationMsg}); err != nil {
		return nil, err
	}
	if err := gob.NewEncoder(conn).Encode(i); err != nil {
		return nil, err
	}

	infoMsg := msgs.MigrationInfoMsg{
		GroupId:    i.Program.GroupId,
		ThreadId:   i.Program.ThreadId,
		MigratedTo: ipAddr,
	}

	for _, c := range s.Peers {
		if c == conn {
			continue
		}
		if _, err := c.Write([]byte{MigrateInfoMsg}); err != nil {
			return nil, err
		}
		if err := gob.NewEncoder(c).Encode(infoMsg); err != nil {
			return nil, err
		}
	}

	return conn, nil
}

func (s *Server) HandleConn(conn net.Conn, isInbound bool) error {
	defer func() {
		fmt.Println("Conn closed")
		conn.Close()
	}()
	var (
		b   = make([]byte, 1)
		err error
	)

	if !isInbound {
		a := make([]byte, 1024)
		n, err := conn.Read(a)
		if err != nil {
			fmt.Println(err)
			return err
		}
		adrs := string(a[:n])
		fmt.Printf("[%s] incoming connection from: (%s)\n", s.Address(), adrs)
		s.Peers[adrs] = conn
	} else {
		if _, err := conn.Write([]byte(s.Address())); err != nil {
			return err
		}
	}

	for {
		conn.Read(b)
		switch b[0] {
		case MigrationMsg:
			err = s.handleMigrationMsg(conn)
		case SendMsg:
			err = s.handleSndMsg(conn)
		case LoadBalanceMsg:
			err = s.handleLoadBalanceMsg(conn)
		case MigrateInfoMsg:
			err = s.handleMigrateInfoMsg(conn)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			log.Fatal(err)
			return err
		}
	}
}
func (s *Server) handleMigrateInfoMsg(conn net.Conn) error {
	m := &msgs.MigrationInfoMsg{}
	if err := gob.NewDecoder(conn).Decode(m); err != nil {
		return err
	}
	s.MsgChan <- m
	return nil
}
func (s *Server) handleMigrationMsg(conn net.Conn) error {
	inter := &interpreter.Interpreter{}
	if err := gob.NewDecoder(conn).Decode(inter); err != nil {
		return err
	}
	inter.MigratedFrom = conn.RemoteAddr().Network()
	s.MigrationChan <- inter
	return nil
}
func (s *Server) SendSndMsg(m *msgs.Msg) error {
	var ok bool
	m.Conn, ok = s.Peers[m.To]
	if !ok {
		return nil
	}
	if _, err := m.Conn.Write([]byte{SendMsg}); err != nil {
		return err
	}
	msgForEncoding := msgs.MsgForEncoding{
		GroupId: m.GroupId,
		Val:     m.Val,
		ChanId:  m.ChanId,
		From:    m.From,
	}
	if err := gob.NewEncoder(m.Conn).Encode(msgForEncoding); err != nil {
		return err
	}
	return nil
}
func (s *Server) handleSndMsg(conn net.Conn) error {
	msg := &msgs.Msg{}
	if err := gob.NewDecoder(conn).Decode(msg); err != nil {
		return err
	}
	s.MsgChan <- msg
	return nil
}

func (s *Server) Connect(Peers string) error {

	if len(Peers) == 0 {
		return nil
	}
	addresses := strings.Split(Peers, ",")
	for _, p := range addresses {
		conn, err := net.Dial("tcp", p)
		if err != nil {
			return err
		}
		s.Peers[p] = conn
		fmt.Printf("[%s] connected with (%s)\n", s.Address(), conn.RemoteAddr().String())
		go s.HandleConn(conn, true)
	}
	return nil
}
func (s *Server) LoadBalance(val int) {
	temp := fmt.Sprintf("%d", val)
	s.broadcastMessage(LoadBalanceMsg, []byte(temp))
}
func (s *Server) handleLoadBalanceMsg(conn net.Conn) error {
	b := make([]byte, 1024)
	n, err := conn.Read(b)
	if err != nil {
		return err
	}
	val, err := strconv.Atoi(string(b[:n]))
	if err != nil {
		return err
	}
	lbMsg := msgs.LoadBalanceMsg{
		From: conn.RemoteAddr().String(),
		Val:  val,
	}
	s.MsgChan <- lbMsg
	return nil
}
func (s *Server) broadcastMessage(msgB byte, msg []byte) error {
	for _, p := range s.Peers {
		p.Write([]byte{msgB})
		p.Write(msg)
	}
	return nil
}

func (s *Server) Address() string {
	return s.IpAddr + s.Port
}
