package server

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/GiorgosMarga/simplescript/interpreter"
	"github.com/GiorgosMarga/simplescript/msgs"
)

// const StreamByte = byte(0)
const (
	MigrationMsg = 0x0
	SendMsg      = 0x1
)

type Server struct {
	IpAddr        string
	Port          string
	ln            net.Listener
	MigrationChan chan *interpreter.Interpreter
	MsgChan       chan *msgs.Msg
}

func NewServer(ipAddr, port string) *Server {
	return &Server{
		IpAddr:        ipAddr,
		Port:          port,
		MigrationChan: make(chan *interpreter.Interpreter),
		MsgChan:       make(chan *msgs.Msg),
	}
}

func (s *Server) ListenAndAccept() error {
	var err error
	s.ln, err = net.Listen("tcp", s.Port)
	if err != nil {
		return err
	}
	defer s.ln.Close()
	fmt.Printf("Environment server listening (%s%s)\n", s.IpAddr, s.Port)
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}
		go s.HandleConn(conn)
	}
}
func (s *Server) Migrate(ipAddr, port string, p *interpreter.Interpreter) (net.Conn, error) {
	conn, err := net.Dial("tcp", ipAddr+port)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	conn.Write([]byte{MigrationMsg})
	if err := gob.NewEncoder(conn).Encode(p); err != nil {
		return nil, err
	}
	go s.HandleConn(conn)
	return conn, nil
}

func (s *Server) HandleConn(conn net.Conn) error {
	defer conn.Close()
	var (
		b   = make([]byte, 1)
		err error
	)
	for {
		conn.Read(b)
		switch b[0] {
		case MigrationMsg:
			err = s.handleMigrationMsg(conn)
		case SendMsg:
			err = s.handleSndMsg(conn)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func (s *Server) handleMigrationMsg(conn net.Conn) error {
	inter := &interpreter.Interpreter{}
	if err := gob.NewDecoder(conn).Decode(inter); err != nil {
		if errors.Is(err, io.EOF) {
			return err
		}
		fmt.Printf("Wrong: %s\n", err)
	}
	inter.MigratedFrom = conn.RemoteAddr().Network()
	inter.Conn = conn
	s.MigrationChan <- inter
	return nil
}
func (s *Server) SendSndMsg(m *msgs.Msg) error {
	m.Conn.Write([]byte{0x1})
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
		if errors.Is(err, io.EOF) {
			return err
		}
		fmt.Printf("Wrong: %s\n", err)
	}
	s.MsgChan <- msg
	return nil
}
