package environment

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/GiorgosMarga/simplescript/interpreter"
	"github.com/GiorgosMarga/simplescript/server"
)

type Prog struct {
	name        string
	status      string
	killChan    chan struct{}
	interpreter *interpreter.Interpreter
	id          int
}

type Environment struct {
	s              *server.Server
	threadsRunning map[int]*Prog
	groupsRunning  map[int][]*Prog
	scanner        *bufio.Scanner
	id             int
}

func NewEnvironment(ipAddr, port string) *Environment {
	return &Environment{
		s:              server.NewServer(ipAddr, port),
		threadsRunning: make(map[int]*Prog, 100),
		groupsRunning:  make(map[int][]*Prog, 100),
		scanner:        bufio.NewScanner(os.Stdin),
		id:             0,
	}
}

func (e *Environment) handleGroupExecution(cmd string) {
	cmds := strings.Split(cmd[4:], " || ")
	groupId := e.id
	wg := &sync.WaitGroup{}
	e.groupsRunning[groupId] = make([]*Prog, len(cmds))
	groupChans := make(map[int]chan []byte, len(cmds))
	for i := 0; i < len(cmds); i++ {
		groupChans[i] = make(chan []byte)
	}
	for n, cmd := range cmds {
		t := strings.Split(cmd, " ")
		args := make([]string, 0, 10)
		for i := 1; i < len(t); i++ {
			args = append(args, t[i])
		}
		p := &Prog{
			name:     t[0],
			status:   "RUNNING",
			killChan: make(chan struct{}),
			id:       n,
		}
		e.groupsRunning[groupId][n] = p

		wg.Add(1)
		go func(p *Prog, args []string, killChan chan struct{}, groupChan map[int]chan []byte) {
			f, err := os.Open(p.name)
			if err != nil {
				fmt.Println(err)
				return
			}
			defer func() {
				f.Close()
				wg.Done()
			}()
			inter := interpreter.NewInterpreter(p.id, p.name, killChan, groupChan, 0)
			inter.ReadInstructions(f, args)
			p.interpreter = inter
			if err := inter.Run(); err != nil {
				if errors.Is(err, interpreter.ErrSignalKilled) {
					e.groupsRunning[groupId][p.id].status = "KILLED"
					return
				}
				// unexpected error, should terminate all the other memebers of the group
				e.groupsRunning[groupId][p.id].status = "ERROR:" + err.Error()
				if err := e.killGroup(groupId); err != nil {
					fmt.Println(err)
				}
				return
			}
			e.groupsRunning[groupId][p.id].status = "FINISHED"

		}(p, args, p.killChan, groupChans)
	}
	wg.Wait()
}
func (e *Environment) killGroup(id int) error {
	ps, ok := e.groupsRunning[id]
	if !ok {
		return fmt.Errorf("group (%d) doesn't exist", id)
	}

	for _, p := range ps {
		p.killChan <- struct{}{}
	}
	return nil
}
func (e *Environment) Start() error {
scanLoop:
	for e.scanner.Scan() {
		if strings.Contains(e.scanner.Text(), "||") {
			e.id++

			go e.handleGroupExecution(e.scanner.Text())
			continue
		}
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
			e.s.Migrate(ipAddr, port, p.name, p.interpreter)
			e.groupsRunning[gid][tid].status = "MIGRATED TO" + ipAddr + port
		case t[0] == "list":
			for id, p := range e.threadsRunning {
				fmt.Printf("> threadID: %d\tprogname: %s\tstatus: %s\n", id, p.name, p.status)
			}
			for id, progs := range e.groupsRunning {
				fmt.Printf("> Groupid: %d\n", id)
				for n, p := range progs {
					fmt.Printf("\tthreadID: %d\tprogname: %s\tstatus: %s\n", n, p.name, p.status)
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
				p.killChan <- struct{}{}
				continue
			}
			_, ok = e.groupsRunning[id]
			if ok {
				e.killGroup(id)
				continue
			}
			fmt.Printf("Thread: [%d] doesn't exist or is not running\n", id)
		case t[0] == "run":
			args := make([]string, 0, 10)
			for i := 2; i < len(t); i++ {
				args = append(args, t[i])
			}
			p := &Prog{
				name:   t[1],
				status: "RUNNING",
			}
			e.threadsRunning[e.id] = p
			p.killChan = make(chan struct{})
			go func(id int, progname string, args []string, killChan chan struct{}) {
				f, err := os.Open(progname)
				if err != nil {
					fmt.Println(err)
					return
				}
				defer f.Close()
				inter := interpreter.NewInterpreter(id, progname, killChan, nil, 0)
				inter.ReadInstructions(f, args)
				if err := inter.Run(); err != nil {
					if errors.Is(err, interpreter.ErrSignalKilled) {
						e.threadsRunning[id].status = "KILLED"
						return
					}
					e.threadsRunning[id].status = "ERROR:" + err.Error()
					return
				}
				e.threadsRunning[id].status = "FINISHED"
			}(e.id, t[1], args, p.killChan)
			e.id++
		default:
			fmt.Println("Unknown command:", e.scanner.Text())
		}
	}
	if err := e.scanner.Err(); err != nil {
		return err
	}
	return nil
}
