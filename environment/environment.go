package environment

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GiorgosMarga/simplescript/interpreter"
	"github.com/GiorgosMarga/simplescript/server"
)

type Environment struct {
	s              *server.Server
	threadsRunning map[int]*interpreter.Prog
	groupsRunning  map[int][]*interpreter.Prog
	scanner        *bufio.Scanner
	id             int
}

func NewEnvironment(ipAddr, port string) *Environment {
	return &Environment{
		s:              server.NewServer(ipAddr, port),
		threadsRunning: make(map[int]*interpreter.Prog, 100),
		groupsRunning:  make(map[int][]*interpreter.Prog, 100),
		scanner:        bufio.NewScanner(os.Stdin),
		id:             0,
	}
}

func (e *Environment) handleGroupExecution(cmd string, groupId int) {
	fmt.Println("Group execution for group", groupId)
	cmds := strings.Split(cmd[4:], " || ") // skip 'run'
	wg := &sync.WaitGroup{}
	e.groupsRunning[groupId] = make([]*interpreter.Prog, len(cmds))
	groupChans := make(map[int]chan []byte, len(cmds))
	for i := 0; i < len(cmds); i++ {
		groupChans[i] = make(chan []byte)
	}
	for n, cmd := range cmds {
		t := strings.Split(cmd, " ")
		wg.Add(1)
		go func(grpid, id int) {
			if err := e.runScript(t, 0, id, grpid, groupChans); err != nil {
				if !errors.Is(err, interpreter.ErrSignalKilled) {
					fmt.Println(err, "Killing all members....")
					time.Sleep(100 * time.Millisecond)
					e.killGroup(groupId)
				}
			}
			wg.Done()
		}(groupId, n)
	}
	wg.Wait()
}

// TODO: add mtx
func (e *Environment) runScript(t []string, startingPos, id, grpid int, grpChans map[int]chan []byte) error {
	args := make([]string, 0, 10)
	for i := 0; i < len(t); i++ {
		args = append(args, t[i])
	}
	p := &interpreter.Prog{
		Name:     t[0],
		Status:   "RUNNING",
		Id:       id,
		KillChan: make(chan struct{}),
	}
	if grpid > -1 {
		e.groupsRunning[grpid][id] = p
	} else {
		e.threadsRunning[id] = p
	}
	f, err := os.Open(p.Name)
	if err != nil {
		return err
	}
	defer f.Close()
	inter := interpreter.NewInterpreter(p, grpChans, startingPos)
	inter.ReadInstructions(f, args)
	p.Interpreter = inter
	if err := inter.Run(); err != nil {
		if errors.Is(err, interpreter.ErrSignalKilled) {
			e.threadsRunning[p.Id].Status = "KILLED"
			return err
		}
		e.threadsRunning[p.Id].Status = "ERROR:" + err.Error()
		return err
	}
	if grpid > -1 {
		e.groupsRunning[grpid][id].Status = "FINISHED"
	} else {
		e.threadsRunning[p.Id].Status = "FINISHED"
	}
	return nil
}
func (e *Environment) killGroup(id int) error {
	ps, ok := e.groupsRunning[id]
	if !ok {
		return fmt.Errorf("group (%d) doesn't exist", id)
	}

	for _, p := range ps {
		p.KillChan <- struct{}{}
	}
	return nil
}
func (e *Environment) handleMigrations() error {
	for p := range e.s.EnvChan {
		inter := p.Interpreter
		fmt.Printf("Resuming %s execution\n", p.Name)
		go inter.Run()
	}
	return nil
}
func (e *Environment) Start() error {
	go func() {
		log.Fatal(e.s.ListenAndAccept())
	}()
	go e.handleMigrations()

	dirName := fmt.Sprintf(".%s%s", e.s.IpAddr, e.s.Port)
	if _, err := os.Stat(dirName); os.IsNotExist(err) {
		if err := os.Mkdir(dirName, os.ModePerm); err != nil {
			log.Fatal(err)
		}
	}

scanLoop:
	for e.scanner.Scan() {
		t := strings.Split(e.scanner.Text(), " ")
		switch {
		case t[0] == "exit":
			break scanLoop
		case t[0] == "migrate":
			var (
				grpID    = t[1]
				threadId = t[2]
				ipAddr   = t[3]
				port     = t[4]
			)
			gid, _ := strconv.Atoi(grpID)
			tid, _ := strconv.Atoi(threadId)
			p := e.groupsRunning[gid][tid]
			e.s.Migrate(ipAddr, port, p)
			e.groupsRunning[gid][tid].Status = "MIGRATED TO" + ipAddr + port
		case t[0] == "list":
			for id, p := range e.threadsRunning {
				fmt.Printf("> threadID: %d\tName: %s\tstatus: %s\n", id, p.Name, p.Status)
			}
			for id, interpreters := range e.groupsRunning {
				fmt.Printf("> Groupid: %d\n", id)
				for n, p := range interpreters {
					fmt.Printf("\tthreadID: %d\tName: %s\tstatus: %s\n", n, p.Name, p.Status)
				}
			}
			fmt.Println()
		case t[0] == "kill":
			id, err := strconv.Atoi(t[1])
			if err != nil {
				fmt.Println(err.Error())
				continue
			}
			p, ok := e.threadsRunning[id]
			if ok {
				p.KillChan <- struct{}{}
				continue
			}
			_, ok = e.groupsRunning[id]
			if ok {
				e.killGroup(id)
				continue
			}
			fmt.Printf("Thread: [%d] doesn't exist or is not running\n", id)
		case t[0] == "run":
			for i := 1; i < len(t); i++ {
				if t[i] == "||" {
					go e.handleGroupExecution(strings.Join(t, " "), e.id)
					e.id++
					continue scanLoop
				}
			}
			go func() {
				if err := e.runScript(t[1:], 0, e.id, -1, nil); err != nil {
					fmt.Println(err)
				}
			}()
		default:
			fmt.Println("Unknown command:", e.scanner.Text())
		}
	}
	if err := e.scanner.Err(); err != nil {
		return err
	}
	return nil
}

// TODO: ipaddr+port
func (e *Environment) Migrate(ipAddr, port string, p *interpreter.Prog) error {
	conn, err := net.Dial("tcp", port)
	if err != nil {
		fmt.Println(err)
		return err
	}
	f, err := os.Open(p.Name)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	st, _ := f.Stat()
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, int64(len(p.Name)))
	binary.Write(buf, binary.LittleEndian, []byte(p.Name))
	binary.Write(buf, binary.LittleEndian, st.Size())
	binary.Write(buf, binary.LittleEndian, b)
	binary.Write(buf, binary.LittleEndian, int64(p.Interpreter.GetPos()))

	binary.Write(buf, binary.LittleEndian, int64(len(p.Interpreter.Variables)))
	_, err = conn.Write(buf.Bytes())
	buf.Reset()
	for varName, varVal := range p.Interpreter.Variables {
		binary.Write(buf, binary.LittleEndian, int64(len(varName)))
		binary.Write(buf, binary.LittleEndian, []byte(varName))
		binary.Write(buf, binary.LittleEndian, int64(len(varVal)))
		binary.Write(buf, binary.LittleEndian, []byte(varVal))
	}
	_, err = conn.Write(buf.Bytes())

	if err != nil {
		return err
	}
	// fmt.Printf("[%s]: wrote (%d) bytes to %s\n", s.port, n, conn.RemoteAddr())
	go e.s.HandleConn(conn)
	return nil

}
