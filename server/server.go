package server

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/GiorgosMarga/simplescript/interpreter"
)

const StreamByte = byte(0)

type Server struct {
	IpAddr  string
	Port    string
	ln      net.Listener
	peers   []net.Conn
	EnvChan chan *interpreter.Prog
}

func NewServer(ipAddr, port string) *Server {
	return &Server{
		IpAddr:  ipAddr,
		Port:    port,
		EnvChan: make(chan *interpreter.Prog),
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
func (s *Server) Migrate(ipAddr, port string, p *interpreter.Prog) error {

	fmt.Println("Migrating to", ipAddr+port)
	conn, err := net.Dial("tcp", ipAddr+port)
	if err != nil {
		fmt.Println(err)
		return err
	}
	if err := gob.NewEncoder(conn).Encode(p); err != nil {
		return err
	}
	go s.HandleConn(conn)
	return nil
}

func (s *Server) HandleConn(conn net.Conn) error {
	defer conn.Close()
	for {
		p := &interpreter.Prog{}
		if err := gob.NewDecoder(conn).Decode(p); err != nil {
			if errors.Is(err, io.EOF) {
				return err
			}
			fmt.Printf("Wrong: %s", err)
			continue
		}
		s.EnvChan <- p
	}
}
