package environment

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GiorgosMarga/simplescript/interpreter"
	"github.com/GiorgosMarga/simplescript/msgs"
	"github.com/GiorgosMarga/simplescript/server"
)

type Environment struct {
	s              *server.Server
	threadsRunning map[int]*interpreter.Interpreter
	groupsRunning  map[int][]*interpreter.Interpreter
	scanner        *bufio.Scanner
	id             int
	msgChan        chan *msgs.Msg
}

func NewEnvironment(ipAddr, port string) *Environment {
	return &Environment{
		s:              server.NewServer(ipAddr, port),
		threadsRunning: make(map[int]*interpreter.Interpreter, 100),
		groupsRunning:  make(map[int][]*interpreter.Interpreter, 100),
		scanner:        bufio.NewScanner(os.Stdin),
		id:             0,
		msgChan:        make(chan *msgs.Msg),
	}
}

func (e *Environment) handleGroupExecution(cmd string, groupId int) {
	fmt.Println("Group execution for group", groupId)
	cmds := strings.Split(cmd[4:], " || ") // skip 'run'
	wg := &sync.WaitGroup{}
	e.groupsRunning[groupId] = make([]*interpreter.Interpreter, len(cmds))
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
// TODO: handle list "killed", who should add this environment or script
func (e *Environment) runScript(t []string, startingPos, id, grpid int, grpChans map[int]chan []byte) error {
	args := make([]string, 0, 10)
	for i := 0; i < len(t); i++ {
		args = append(args, t[i])
	}
	p := interpreter.NewProg(t[0], id)
	f, err := os.Open(p.Name)
	if err != nil {
		return err
	}
	defer f.Close()
	inter := interpreter.NewInterpreter(p, grpChans, startingPos, e.msgChan)
	if grpid > -1 {
		e.groupsRunning[grpid][id] = inter
	} else {
		e.threadsRunning[id] = inter
	}
	inter.ReadInstructions(f, args)
	if err := inter.Run(); err != nil {
		if errors.Is(err, interpreter.ErrSignalKilled) {
			return err
		}
		e.threadsRunning[p.ThreadId].Program.Status = "ERROR:" + err.Error()
		return err
	}
	if grpid > -1 {
		e.groupsRunning[p.GroupId][p.ThreadId].Program.Status = "FINISHED"
	} else {
		e.threadsRunning[p.ThreadId].Program.Status = "FINISHED"
	}
	return nil
}
func (e *Environment) killGroup(id int) error {
	inters, ok := e.groupsRunning[id]
	if !ok {
		return fmt.Errorf("group (%d) doesn't exist", id)
	}

	for _, inter := range inters {
		inter.Program.KillProg("KILLED")
	}
	return nil
}
func (e *Environment) handleMigrations() error {
	for inter := range e.s.EnvChan {
		if inter.Program.GroupId > -1 {
			fmt.Println(e.groupsRunning[inter.Program.GroupId])
		}
		fmt.Printf("Resuming %s execution\n", inter.Program.Name)
		inter.MsgChan = e.msgChan
		go inter.Run()
	}
	return nil
}

type MsgForEncoding struct {
	To  int
	Val []byte
}

func (e *Environment) handleSndMsg() error {
	for m := range e.msgChan {
		m.Conn.Write([]byte{0x1})
		msgForEncoding := MsgForEncoding{
			To:  m.To,
			Val: m.Val,
		}
		if err := gob.NewEncoder(m.Conn).Encode(msgForEncoding); err != nil {
			fmt.Println("Sending sndMsg:", err)
			continue
		}
	}
	return nil
}
func init() {
	gob.Register(msgs.Msg{})
}
func (e *Environment) Start() error {
	go func() {
		log.Fatal(e.s.ListenAndAccept())
	}()
	go e.handleMigrations()
	go e.handleSndMsg()

	dirName := fmt.Sprintf(".%s%s", e.s.IpAddr, e.s.Port)
	if _, err := os.Stat(dirName); os.IsNotExist(err) {
		if err := os.Mkdir(dirName, os.ModePerm); err != nil {
			log.Fatal(err)
		}
	}

scanLoop:
	for e.scanner.Scan() {
		t := strings.Split(strings.TrimSpace(e.scanner.Text()), " ")
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
			if err := e.s.Migrate(ipAddr, port, p); err != nil {
				fmt.Println(err)
				continue
			}
			p.Program.KillProg("MIGRATED TO " + ipAddr + port)
		case t[0] == "list":
			for id, p := range e.threadsRunning {
				fmt.Printf("> threadID: %d\tName: %s\tstatus: %s\n", id, p.Program.Name, p.Program.Status)
			}
			for id, interpreters := range e.groupsRunning {
				fmt.Printf("> Groupid: %d\n", id)
				for n, p := range interpreters {
					fmt.Printf("\tthreadID: %d\tName: %s\tstatus: %s\n", n, p.Program.Name, p.Program.Status)
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
			if ok && p.Program.Status == "RUNNING" {
				p.Program.KillProg("KILLED")
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
