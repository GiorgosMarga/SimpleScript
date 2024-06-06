package environment

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"net"
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
	s                   *server.Server
	threadsRunning      map[int]*interpreter.Interpreter
	groupsRunning       map[int][]*interpreter.Interpreter
	groupInfos          map[int]*GroupInfo
	scanner             *bufio.Scanner
	id                  int
	msgChan             chan *msgs.Msg
	addressToNumThreads map[string]int
}

type GroupInfo struct {
	groupChans   map[int]chan msgs.InterProcessMsg
	threadsState map[int]int
	threadToConn map[int]net.Conn
}

var stateToStatus = map[int]string{
	interpreter.Running:  "RUNNING",
	interpreter.Killed:   "KILLED",
	interpreter.Migrated: "MIGRATED",
	interpreter.Finished: "FINISHED",
	interpreter.Error:    "ERROR",
}

func NewEnvironment(ipAddr, port string) *Environment {
	return &Environment{
		s:                   server.NewServer(ipAddr, port),
		threadsRunning:      make(map[int]*interpreter.Interpreter, 100),
		groupsRunning:       make(map[int][]*interpreter.Interpreter, 100),
		scanner:             bufio.NewScanner(os.Stdin),
		groupInfos:          make(map[int]*GroupInfo),
		id:                  0,
		msgChan:             make(chan *msgs.Msg),
		addressToNumThreads: make(map[string]int),
	}
}

func (e *Environment) loadBalance(val int, groupId int) error {
	if len(e.s.Peers) == 0 {
		return nil
	}
	time.Sleep(100 * time.Millisecond)
	for add := range e.s.Peers {
		e.addressToNumThreads[add] = 0
	}
	for range val {
		var (
			minIndex = "" // position
			minVal   = math.MaxInt
		)
		for k, v := range e.addressToNumThreads {
			if v < minVal {
				minVal = v
				minIndex = k
			}
		}
		if minVal < len(e.groupsRunning) {
			for _, thread := range e.groupsRunning[groupId] {
				if thread.Program.Status != interpreter.Running {
					continue
				}
				if err := e.migrate(minIndex, thread); err != nil {
					fmt.Println(err)
					return err
				}
				break
			}
			e.addressToNumThreads[minIndex]++
		}
	}

	e.s.LoadBalance(val)
	return nil
}
func (e *Environment) handleGroupExecution(cmd string, groupId int) {
	fmt.Println("Group execution for group", groupId)
	cmds := strings.Split(cmd[4:], " || ") // skip 'run'
	wg := &sync.WaitGroup{}
	e.groupsRunning[groupId] = make([]*interpreter.Interpreter, len(cmds))
	groupChans := make(map[int]chan msgs.InterProcessMsg, len(cmds))
	for i := 0; i < len(cmds); i++ {
		groupChans[i] = make(chan msgs.InterProcessMsg)
	}

	gi := &GroupInfo{
		groupChans:   groupChans,
		threadsState: make(map[int]int),
		threadToConn: make(map[int]net.Conn),
	}
	e.groupInfos[groupId] = gi
	threadToAddr := make(map[int]string)
	for i := range len(cmds) {
		threadToAddr[i] = e.s.Address()
	}
	for n, cmd := range cmds {
		t := strings.Split(cmd, " ")
		wg.Add(1)
		gi.threadsState[n] = interpreter.Running
		go func(id int, threadsState map[int]int) {
			if err := e.runScript(t, 0, id, groupId, groupChans, threadsState, threadToAddr); err != nil {
				if !errors.Is(err, interpreter.ErrSignalKilled) {
					fmt.Println(err, "Killing all members....")
					time.Sleep(100 * time.Millisecond)
					e.killGroup(groupId)
				}
			}
			wg.Done()
		}(n, gi.threadsState)
	}
	// if err := e.loadBalance(len(cmds), groupId); err != nil {
	// 	fmt.Println(err)
	// }
	wg.Wait()
}

func (e *Environment) runScript(t []string, startingPos, id, grpid int, grpChans map[int]chan msgs.InterProcessMsg, groupInfo map[int]int, threadsAddr map[int]string) error {
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
	inter := interpreter.NewInterpreter(p, grpChans, startingPos, e.msgChan, groupInfo, threadsAddr)
	if grpid > -1 {
		e.groupsRunning[grpid][id] = inter
	} else {
		e.threadsRunning[id] = inter
	}
	inter.ReadInstructions(f, args)
	e.id++

	if err := inter.Run(); err != nil {
		if errors.Is(err, interpreter.ErrSignalKilled) {
			return err
		}
		e.threadsRunning[p.ThreadId].Program.Status = interpreter.Error
		return err
	}
	inter.Program.Status = interpreter.Finished
	return nil
}
func (e *Environment) killGroup(id int) error {
	inters, ok := e.groupsRunning[id]
	if !ok {
		return fmt.Errorf("group (%d) doesn't exist", id)
	}
	gi := e.groupInfos[id]
	for n, inter := range inters {
		inter.Program.KillProg(interpreter.Killed)
		gi.threadsState[n] = interpreter.Killed
	}
	return nil
}
func (e *Environment) handleMigrationMsg(inter *interpreter.Interpreter) error {
	gid := inter.Program.GroupId
	if gid > -1 {
		var (
			g  []*interpreter.Interpreter
			ok bool
		)
		g, ok = e.groupsRunning[gid]
		if !ok {
			g = make([]*interpreter.Interpreter, 0)
		}
		g = append(g, inter)
		e.groupsRunning[gid] = g

	}
	fmt.Printf("Resuming %s execution\n", inter.Program.Name)
	inter.MsgChan = e.msgChan
	gi, ok := e.groupInfos[inter.Program.GroupId]
	if !ok {

		gi = &GroupInfo{
			groupChans:   make(map[int]chan msgs.InterProcessMsg),
			threadsState: make(map[int]int),
		}
		e.groupInfos[inter.Program.GroupId] = gi

	}
	gi.groupChans[inter.Program.ThreadId] = make(chan msgs.InterProcessMsg)
	gi.threadsState[inter.Program.ThreadId] = interpreter.Running
	e.groupInfos[inter.Program.GroupId] = gi
	inter.AddChan(gi.groupChans)
	inter.AddThreadsState(gi.threadsState)
	inter.CreateMtx()
	go func() {
		if err := inter.Run(); err != nil {
			fmt.Println(err)
		}
		inter.Program.Status = interpreter.Finished
	}()
	return nil
}

func (e *Environment) handleMsgs() error {
	for {
		select {
		case inter := <-e.s.MigrationChan:
			if err := e.handleMigrationMsg(inter); err != nil {
				return err
			}
		case m := <-e.msgChan:
			if err := e.s.SendSndMsg(m); err != nil {
				fmt.Println(err)
				return err
			}
		case m := <-e.s.MsgChan:
			switch msg := m.(type) {
			case *msgs.Msg:
				gi := e.groupInfos[msg.GroupId]
				interMsg := msgs.InterProcessMsg{
					Val:  msg.Val,
					From: msg.From,
				}
				gi.groupChans[msg.ChanId] <- interMsg
			case msgs.LoadBalanceMsg:
				e.addressToNumThreads[msg.From] += msg.Val
			case *msgs.MigrationInfoMsg:
				threads, ok := e.groupsRunning[msg.GroupId]
				if !ok {
					fmt.Println("Group doesnt exist")
					continue
				}
				for _, thread := range threads {
					thread.ThreadsAddr[msg.ThreadId] = msg.MigratedTo
				}
			}
		}
	}
}

func (e *Environment) migrate(addr string, inter *interpreter.Interpreter) error {
	var (
		gid = inter.Program.GroupId
		tid = inter.Program.ThreadId
	)
	fmt.Println(tid)
	inter.Lock()
	defer inter.Unlock()
	inter.ThreadsAddr[tid] = addr
	conn, err := e.s.Migrate(addr, inter)
	if err != nil {
		return err
	}
	e.groupInfos[gid].threadsState[tid] = interpreter.Migrated
	e.groupInfos[gid].threadToConn[tid] = conn
	inter.Program.KillProg(interpreter.Migrated)
	return nil
}

func (e *Environment) Start(peers string) error {

	if err := e.s.Connect(peers); err != nil {
		return err
	}

	go e.s.ListenAndAccept()
	go e.handleMsgs()

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
			inter := e.groupsRunning[gid][tid]
			if err := e.migrate(ipAddr+port, inter); err != nil {
				fmt.Println(err)
				continue
			}
		case t[0] == "list":
			for id, p := range e.threadsRunning {
				fmt.Printf("> threadID: %d\tName: %s\tstatus: %s\n", id, p.Program.Name, stateToStatus[p.Program.Status])
			}
			for id, interpreters := range e.groupsRunning {
				fmt.Printf("> Groupid: %d\n", id)
				for n, p := range interpreters {
					fmt.Printf("\tthreadID: %d\tName: %s\tstatus: %s\n", n, p.Program.Name, stateToStatus[p.Program.Status])
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
			if ok && p.Program.Status == interpreter.Running {
				p.Program.KillProg(interpreter.Killed)
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
					continue scanLoop
				}
			}
			go func() {
				if err := e.runScript(t[1:], 0, e.id, -1, nil, nil, nil); err != nil {
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
