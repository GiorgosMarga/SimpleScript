package server

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	"github.com/GiorgosMarga/simplescript/interpreter"
	"github.com/GiorgosMarga/simplescript/msgs"
)

func init() {
	gob.Register(&msgs.MsgWrapper{})
	gob.Register(&msgs.LoadBalanceMsg{})
	gob.Register(&msgs.MigrationInfoMsg{})
	gob.Register(&interpreter.Interpreter{})
	gob.Register(&msgs.NewConnectionMsg{})
	gob.Register(&msgs.KillGroupMsg{})
	gob.Register(&msgs.NewGroupMsg{})
	gob.Register(&msgs.Msg{})
}

// const StreamByte = byte(0)
const (
	MigrationMsg   = 0x0
	SendMsg        = 0x1
	LoadBalanceMsg = 0x2
	MigrateInfoMsg = 0x3
	ConnectionMsg  = 0x4
	KillMsg        = 0x5
	NewGroupMsg    = 0x6
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

func (s *Server) Migrate(ipAddr string, i *interpreter.Interpreter) error {
	conn, ok := s.Peers[ipAddr]
	if !ok {
		fmt.Println("Peers isn't connected:", ipAddr)
		return fmt.Errorf("peer isn't connected: %s", ipAddr)
	}
	msg := msgs.MsgWrapper{
		Type: MigrationMsg,
		Val:  i,
	}
	s.sendMsg(msg, conn)
	infoMsg := &msgs.MigrationInfoMsg{
		GroupId:    i.Program.GroupId,
		ThreadId:   i.Program.ThreadId,
		MigratedTo: ipAddr,
	}
	msg = msgs.MsgWrapper{
		Type: MigrateInfoMsg,
		Val:  infoMsg,
	}
	for _, c := range s.Peers {
		if c == conn {
			continue
		}
		s.sendMsg(msg, c)
	}
	return nil
}

func (s *Server) sendMsg(msg msgs.MsgWrapper, conn net.Conn) error {
	buffer := new(bytes.Buffer)
	if err := gob.NewEncoder(buffer).Encode(msg); err != nil {
		fmt.Println(err)
		return err
	}
	msgBytes := buffer.Bytes()
	length := uint32(len(msgBytes))
	if err := binary.Write(conn, binary.BigEndian, length); err != nil {
		return err
	}
	_, err := conn.Write(msgBytes)
	return err
}
func (s *Server) receiveMessage(conn net.Conn) (msgs.MsgWrapper, error) {
	var length uint32
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return msgs.MsgWrapper{}, err
	}

	msgBytes := make([]byte, length)
	if _, err := conn.Read(msgBytes); err != nil {
		return msgs.MsgWrapper{}, err
	}

	buffer := bytes.NewBuffer(msgBytes)
	decoder := gob.NewDecoder(buffer)

	var wrapper msgs.MsgWrapper
	if err := decoder.Decode(&wrapper); err != nil {
		return msgs.MsgWrapper{}, err
	}
	return wrapper, nil
}

func (s *Server) HandleConn(conn net.Conn, isInbound bool) error {
	defer func() {
		fmt.Println("Conn closed")
		conn.Close()
	}()

	if !isInbound {
		msg, err := s.receiveMessage(conn)
		if err != nil {
			log.Fatal(err)
		}
		if msg.Type != ConnectionMsg {
			log.Fatalf("protocol error: received wrong message type: %d", msg.Type)
		}
		m := msg.Val.(*msgs.NewConnectionMsg)
		fmt.Printf("[%s] incoming connection from: (%s)\n", s.Address(), m.Address)
		s.Peers[m.Address] = conn
		s.MsgChan <- m
	} else {
		msg := msgs.MsgWrapper{
			Val: &msgs.NewConnectionMsg{
				Address: s.Address(),
				GroupId: 0,
			},
			Type: ConnectionMsg,
		}
		if err := s.sendMsg(msg, conn); err != nil {
			return err
		}
	}
	for {
		msg, err := s.receiveMessage(conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				delete(s.Peers, s.getAddress(conn))
				msg := &msgs.DisconnectMsg{Address: s.getAddress(conn)}
				s.MsgChan <- msg
				return nil
			}
			log.Fatal(err)
		}
		switch msg.Type {
		case MigrationMsg:
			err = s.handleMigrationMsg(msg.Val.(*interpreter.Interpreter))
		case SendMsg:
			err = s.handleSndMsg(msg.Val.(*msgs.Msg))
		case LoadBalanceMsg:
			err = s.handleLoadBalanceMsg(msg.Val.(*msgs.LoadBalanceMsg))
		case MigrateInfoMsg:
			err = s.handleMigrateInfoMsg(msg.Val.(*msgs.MigrationInfoMsg))
		case KillMsg:
			err = s.handleKillMsg(msg.Val.(*msgs.KillGroupMsg))
		case NewGroupMsg:
			err = s.handleNewGroupMsg(msg.Val.(*msgs.NewGroupMsg))
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
func (s *Server) SendNewGroupMsg(grpid int) error {
	msg := &msgs.NewGroupMsg{Groupid: grpid}
	if err := s.broadcastMessage([]net.Conn{}, NewGroupMsg, msg); err != nil {
		return err
	}
	return nil
}
func (s *Server) handleNewGroupMsg(msg *msgs.NewGroupMsg) error {
	s.MsgChan <- msg
	return nil
}
func (s *Server) handleMigrateInfoMsg(msg *msgs.MigrationInfoMsg) error {
	s.MsgChan <- msg
	return nil
}
func (s *Server) handleKillMsg(msg *msgs.KillGroupMsg) error {
	s.MsgChan <- msg
	return nil
}
func (s *Server) handleMigrationMsg(inter *interpreter.Interpreter) error {
	s.MsgChan <- inter
	return nil
}
func (s *Server) SendSndMsg(m *msgs.Msg) error {
	conn, ok := s.Peers[m.To]
	if !ok {
		return nil
	}
	msg := msgs.MsgWrapper{
		Type: SendMsg,
		Val:  m,
	}
	s.sendMsg(msg, conn)
	return nil
}

func (s *Server) KillGroupMsg(grpid int) error {
	msg := &msgs.KillGroupMsg{
		GroupId: grpid,
	}
	if err := s.broadcastMessage([]net.Conn{}, KillMsg, msg); err != nil {
		return err
	}
	return nil
}
func (s *Server) handleSndMsg(msg *msgs.Msg) error {
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
func (s *Server) LoadBalance(val int, ignore []net.Conn) error {
	lbMsg := &msgs.LoadBalanceMsg{
		Val:  val,
		From: s.Address(),
	}
	if err := s.broadcastMessage(ignore, LoadBalanceMsg, lbMsg); err != nil {
		return err
	}
	return nil
}
func (s *Server) handleLoadBalanceMsg(msg *msgs.LoadBalanceMsg) error {
	s.MsgChan <- msg
	return nil
}
func (s *Server) broadcastMessage(ignore []net.Conn, msgB byte, msg any) error {

	msgW := msgs.MsgWrapper{
		Type: msgB,
		Val:  msg,
	}

	for _, conn := range s.Peers {
		if exists(conn, ignore) {
			continue
		}
		s.sendMsg(msgW, conn)
	}

	return nil
}
func (s *Server) Address() string {
	return s.IpAddr + s.Port
}
func (s *Server) getAddress(conn net.Conn) string {
	for a, c := range s.Peers {
		if c == conn {
			return a
		}
	}
	return ""
}
func exists(conn net.Conn, conns []net.Conn) bool {
	for _, c := range conns {
		if c == conn {
			return true
		}
	}
	return false
}

func (s *Server) IsConnected(addr string) bool {
	_, ok := s.Peers[addr]
	return ok
}
