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
	IpAddr string
	Port   string
	ln     net.Listener
	peers  []net.Conn
}

func NewServer(ipAddr, port string) *Server {
	return &Server{
		IpAddr: ipAddr,
		Port:   port,
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
	conn, err := net.Dial("tcp", port)
	if err != nil {
		fmt.Println(err)
		return err
	}
	fmt.Println(p.Interpreter)
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
				fmt.Println("Connection Closed")
				return err
			}
			fmt.Printf("Wrong: %s", err)
			continue
		}
		fmt.Printf("%+v\n", p.Interpreter)
	}
}
