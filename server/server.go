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
	IpAddr  string
	Port    string
	ln      net.Listener
	EnvChan chan *interpreter.Interpreter
}

func NewServer(ipAddr, port string) *Server {
	return &Server{
		IpAddr:  ipAddr,
		Port:    port,
		EnvChan: make(chan *interpreter.Interpreter),
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
func (s *Server) Migrate(ipAddr, port string, p *interpreter.Interpreter) error {
	conn, err := net.Dial("tcp", ipAddr+port)
	if err != nil {
		fmt.Println(err)
		return err
	}
	conn.Write([]byte{MigrationMsg})
	if err := gob.NewEncoder(conn).Encode(p); err != nil {
		return err
	}
	go s.HandleConn(conn)
	return nil
}

func (s *Server) HandleConn(conn net.Conn) error {
	defer conn.Close()
	b := make([]byte, 1)
	for {
		conn.Read(b)
		switch b[0] {
		case MigrationMsg:
			s.handleMigrationMsg(conn)
		case SendMsg:
			s.handleSndMsg(conn)
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
	s.EnvChan <- inter
	return nil
}

func (s *Server) handleSndMsg(conn net.Conn) error {
	fmt.Println("Handling send msg")
	msg := &msgs.Msg{}
	if err := gob.NewDecoder(conn).Decode(msg); err != nil {
		if errors.Is(err, io.EOF) {
			return err
		}
		fmt.Printf("Wrong: %s\n", err)
	}
	fmt.Println(msg)
	return nil
}
