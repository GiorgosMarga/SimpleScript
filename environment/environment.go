package environment

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
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

const MaxID = 100 // math.MaxInt

type Environment struct {
	s                   *server.Server
	threadsRunning      map[int]*interpreter.Interpreter
	groupsRunning       map[int][]*interpreter.Interpreter
	groupInfos          map[int]*GroupInfo
	scanner             *bufio.Scanner
	msgChan             chan *msgs.Msg
	addressToNumThreads map[string]int
	numOfThreads        int
	mtx                 *sync.Mutex
}

type GroupInfo struct {
	groupChans   map[int]chan msgs.InterProcessMsg
	threadsState map[int]int
	threadsAddr  map[int]string
}

var stateToStatus = map[int]string{
	interpreter.Running:  "RUNNING",
	interpreter.Killed:   "KILLED",
	interpreter.Migrated: "MIGRATED",
	interpreter.Finished: "FINISHED",
	interpreter.Error:    "ERROR",
	interpreter.Idle:     "IDLE",
}

func NewEnvironment(ipAddr, port string) *Environment {
	return &Environment{
		s:                   server.NewServer(ipAddr, port),
		threadsRunning:      make(map[int]*interpreter.Interpreter, 100),
		groupsRunning:       make(map[int][]*interpreter.Interpreter, 100),
		scanner:             bufio.NewScanner(os.Stdin),
		groupInfos:          make(map[int]*GroupInfo),
		msgChan:             make(chan *msgs.Msg),
		addressToNumThreads: make(map[string]int),
		mtx:                 &sync.Mutex{},
	}
}

func (e *Environment) loadBalance(inter *interpreter.Interpreter) (bool, error) {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if len(e.s.Peers) == 0 {
		return false, nil
	}
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
	if minVal < e.numOfThreads {
		if err := e.migrate(minIndex, inter); err != nil {
			return false, err
		}
		e.addressToNumThreads[minIndex]++
		return true, nil

	}
	return false, nil
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
	}
	e.groupInfos[groupId] = gi
	threadToAddr := make(map[int]string)
	for i := range len(cmds) {
		threadToAddr[i] = e.s.Address()
	}
	e.numOfThreads += len(cmds)
	for n, cmd := range cmds {
		t := strings.Split(cmd, " ")
		wg.Add(1)
		go func(id int, threadsState map[int]int) {
			if err := e.runScript(t, 0, id, groupId, groupChans, threadsState, threadToAddr); err != nil {
				if !errors.Is(err, interpreter.ErrSignalKilled) {
					fmt.Println(err, "Killing all members....")
					time.Sleep(100 * time.Millisecond)
					e.killGroup(groupId, true)
				}
			}
			wg.Done()
		}(n, gi.threadsState)
	}

	wg.Wait()

}

func (e *Environment) runScript(t []string, startingPos, id, grpid int, grpChans map[int]chan msgs.InterProcessMsg, groupInfo map[int]int, threadsAddr map[int]string) error {

	args := make([]string, 0, 10)
	for i := 0; i < len(t); i++ {
		args = append(args, t[i])
	}
	p := interpreter.NewProg(t[0], id, grpid)
	f, err := os.Open(p.Name)
	if err != nil {
		return err
	}
	defer f.Close()
	inter := interpreter.NewInterpreter(p, grpChans, startingPos, e.msgChan, groupInfo, threadsAddr)
	e.mtx.Lock()
	if grpid > -1 {
		e.groupsRunning[grpid][id] = inter
	} else {
		e.threadsRunning[id] = inter
	}
	e.mtx.Unlock()

	inter.ReadInstructions(f, args)
	ok, err := e.loadBalance(inter)
	if err != nil {
		fmt.Println(err)
		return err
	}
	if ok {
		return nil
	}
	e.s.LoadBalance(1, []net.Conn{})
	if _, ok := e.groupInfos[p.GroupId]; ok {
		e.groupInfos[p.GroupId].threadsState[id] = interpreter.Running
	}
	inter.Program.Status = interpreter.Running
	err = inter.Run()
	if err != nil {
		if errors.Is(err, interpreter.ErrSignalKilled) {
			return err
		}
		inter.Program.Status = interpreter.Error
	}
	inter.Program.Status = interpreter.Finished
	e.mtx.Lock()
	e.numOfThreads--
	e.mtx.Unlock()
	e.s.LoadBalance(-1, []net.Conn{})
	return err
}
func (e *Environment) killGroup(id int, notify bool) error {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	inters, ok := e.groupsRunning[id]
	if !ok {
		return fmt.Errorf("group (%d) doesn't exist", id)
	}
	if notify {
		if err := e.s.KillGroupMsg(id); err != nil {
			return err
		}
	}
	gi := e.groupInfos[id]
	for n, inter := range inters {
		inter.Program.KillProg(interpreter.Killed)
		gi.threadsState[n] = interpreter.Killed
		e.numOfThreads--
	}
	e.s.LoadBalance(-len(inters), []net.Conn{})
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
	ignoreConn := e.s.Peers[inter.MigratedFrom]
	e.s.LoadBalance(1, []net.Conn{ignoreConn})
	fmt.Printf("Resuming %s execution\n", inter.Program.Name)
	inter.MsgChan = e.msgChan
	gi, ok := e.groupInfos[inter.Program.GroupId]
	if !ok {
		gi = &GroupInfo{
			groupChans:   make(map[int]chan msgs.InterProcessMsg),
			threadsState: make(map[int]int),
			threadsAddr:  inter.ThreadsAddr,
		}
		e.groupInfos[inter.Program.GroupId] = gi
	}
	gi.groupChans[inter.Program.ThreadId] = make(chan msgs.InterProcessMsg)
	gi.threadsState[inter.Program.ThreadId] = interpreter.Running
	e.groupInfos[inter.Program.GroupId] = gi
	inter.Update(gi.groupChans, gi.threadsState, gi.threadsAddr)
	inter.Program.Status = interpreter.Running
	go func() {
		if err := inter.Run(); err != nil {
			if !errors.Is(err, interpreter.ErrSignalKilled) {
				inter.Program.Status = interpreter.Error
			}
		} else {
			inter.Program.Status = interpreter.Finished
		}
		e.s.LoadBalance(-1, []net.Conn{})
	}()
	return nil
}

func (e *Environment) handleMsgs() error {
	for {
		select {
		case m := <-e.msgChan:
			if err := e.s.SendSndMsg(m); err != nil {
				fmt.Println(err)
				return err
			}
		case m := <-e.s.MsgChan:
			switch msg := m.(type) {
			case *interpreter.Interpreter:
				if err := e.handleMigrationMsg(msg); err != nil {
					return err
				}
			case *msgs.Msg:
				gi := e.groupInfos[msg.GroupId]
				interMsg := msgs.InterProcessMsg{
					Val:  msg.Val,
					From: msg.From,
				}
				if gi.threadsState[msg.ChanId] == interpreter.Migrated {
					threads := e.groupsRunning[msg.GroupId]
					msg.To = threads[msg.ChanId].ThreadsAddr[msg.ChanId]
					e.s.SendSndMsg(msg)
					continue
				}
				gi.groupChans[msg.ChanId] <- interMsg
			case *msgs.LoadBalanceMsg:
				e.addressToNumThreads[msg.From] += msg.Val
			case *msgs.MigrationInfoMsg:
				threads, ok := e.groupsRunning[msg.GroupId]
				if !ok {
					continue
				}
				for _, thread := range threads {
					thread.ThreadsAddr[msg.ThreadId] = msg.MigratedTo
				}
			case *msgs.NewConnectionMsg:
				e.addressToNumThreads[msg.Address] = 0
			case *msgs.KillGroupMsg:
				if _, ok := e.groupsRunning[msg.GroupId]; ok {
					if err := e.killGroup(msg.GroupId, false); err != nil {
						fmt.Println(err)
					}
				}
			case *msgs.NewGroupMsg:
				e.mtx.Lock()
				e.groupsRunning[msg.Groupid] = make([]*interpreter.Interpreter, 0)
				e.mtx.Unlock()
			case *msgs.DisconnectMsg:
				e.mtx.Lock()
				delete(e.addressToNumThreads, msg.Address)
				e.mtx.Unlock()
			}
		}
	}
}

func (e *Environment) migrate(addr string, inter *interpreter.Interpreter) error {
	var (
		gid = inter.Program.GroupId
		tid = inter.Program.ThreadId
	)
	if !e.s.IsConnected(addr) {
		return fmt.Errorf("peer isn't connected: %s", addr)
	}

	// inter.Lock()
	// defer inter.Unlock()

	inter.ThreadsAddr[tid] = addr
	inter.MigratedFrom = e.s.Address()
	if err := inter.Program.KillProg(interpreter.Migrated); err != nil {
		return err
	}
	if err := e.s.Migrate(addr, inter); err != nil {
		return err
	}
	e.groupInfos[gid].threadsState[tid] = interpreter.Migrated
	// fmt.Println(e.groupInfos[gid].threadsState)
	inter.Program.Status = interpreter.Migrated
	e.numOfThreads--
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
			if len(t) != 5 {
				fmt.Println("migrate ip port groupid threadid")
				continue
			}
			var (
				grpID    = t[1]
				threadId = t[2]
				ipAddr   = t[3]
				port     = t[4]
				group    []*interpreter.Interpreter
				ok       bool
			)
			gid, _ := strconv.Atoi(grpID)
			tid, _ := strconv.Atoi(threadId)
			if group, ok = e.groupsRunning[gid]; !ok {
				fmt.Printf("group: (%d) doesn't exist\n", gid)
				continue
			}
			var inter *interpreter.Interpreter
			for i := 0; i < len(group); i++ {
				if group[i].Program.ThreadId == tid {
					inter = group[i]
					break
				}
			}
			if inter == nil {
				fmt.Printf("process: (%d) doesn't exist\n", tid)
				continue
			}

			fmt.Println("Starting migration....")
			go func() {
				if err := e.migrate(ipAddr+port, inter); err != nil {
					fmt.Println(err)
				}
			}()
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
			if len(t) != 2 {
				fmt.Println("kill <id>")
				continue
			}
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

				e.killGroup(id, true)
				continue
			}
			fmt.Printf("Thread: [%d] doesn't exist or is not running\n", id)
		case t[0] == "run":
			fmt.Println("Running")
			id := e.generateID()
			if err := e.s.SendNewGroupMsg(id); err != nil {
				fmt.Println(err)
				continue
			}
			for i := 1; i < len(t); i++ {
				if t[i] == "||" {
					go e.handleGroupExecution(strings.Join(t, " "), id)
					continue scanLoop
				}
			}
			go func() {
				if err := e.runScript(t[1:], 0, id, -1, nil, nil, nil); err != nil {
					fmt.Println(err)
				}
			}()
		case t[0] == "shutdown":
			peers := make([]string, 0)
			for address := range e.addressToNumThreads {
				peers = append(peers, address)
			}
			if len(peers) == 0 {
				fmt.Println("No other environments running...")
				fmt.Println("Shutting down...")
			}
			ctr := 0
			for _, g := range e.groupsRunning {
				for _, i := range g {
					if i.Program.Status != interpreter.Running {
						continue
					}
					go func() {
						if err := e.migrate(peers[ctr], i); err != nil {
							fmt.Println(err)
						}
					}()
					ctr = (ctr + 1) % len(peers)
				}
			}
		default:
			fmt.Println("Unknown command:", e.scanner.Text())
		}
	}
	if err := e.scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (e *Environment) generateID() int {
	newid := rand.IntN(MaxID)
	for _, ok := e.groupsRunning[newid]; ok; {
		newid = rand.IntN(MaxID)
	}
	return newid
}
