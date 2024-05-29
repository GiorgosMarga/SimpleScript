package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/GiorgosMarga/simplescript/interpreter"
)

const StreamByte = byte(0)

type Server struct {
	ipAddr string
	port   string
	ln     net.Listener
	peers  []net.Conn
}

func NewServer(ipAddr, port string) *Server {
	return &Server{
		ipAddr: ipAddr,
		port:   port,
	}
}

func (s *Server) ListenAndAccept() error {
	var err error
	s.ln, err = net.Listen("tcp", s.port)
	if err != nil {
		return err
	}

	fmt.Printf("Environment server listening (%s%s)\n", s.ipAddr, s.port)
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}
		go s.handleConn(conn)
	}
}
func (s *Server) Migrate(ipAddr, port, filename string, inter *interpreter.Interpreter) error {
	conn, err := net.Dial("tcp", port)
	if err != nil {
		fmt.Println(err)
		return err
	}
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	st, _ := f.Stat()
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, int64(len(filename)))
	binary.Write(buf, binary.LittleEndian, []byte(filename))
	binary.Write(buf, binary.LittleEndian, st.Size())
	binary.Write(buf, binary.LittleEndian, b)
	binary.Write(buf, binary.LittleEndian, int64(inter.GetPos()))
	n, err := conn.Write(buf.Bytes())
	if err != nil {
		return nil
	}
	fmt.Printf("[%s]: wrote (%d) bytes to %s\n", s.port, n, conn.RemoteAddr())
	go s.handleConn(conn)
	return nil
}

func (s *Server) handleConn(conn net.Conn) error {
	var (
		size        int64
		filenameLen int64
		interPos    int64
		err         error
	)
	for {
		binary.Read(conn, binary.LittleEndian, &filenameLen)
		fname := make([]byte, filenameLen)
		binary.Read(conn, binary.LittleEndian, fname)
		fmt.Println("Filename is", fname)
		binary.Read(conn, binary.LittleEndian, &size)
		b := make([]byte, size)
		err = binary.Read(conn, binary.LittleEndian, b)
		if err != nil {
			fmt.Println(err)
			return err
		}
		err = binary.Read(conn, binary.LittleEndian, &interPos)
		if err != nil {
			fmt.Println(err)
			return err
		}
		fmt.Println(string(b))
		fmt.Println(interPos)
	}
}
