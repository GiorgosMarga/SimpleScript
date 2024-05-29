package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
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

func killGroup(groups map[int][]*Prog, id int) error {
	ps, ok := groups[id]
	if !ok {
		return fmt.Errorf("group (%d) doesn't exist", id)
	}

	for _, p := range ps {
		p.killChan <- struct{}{}

	}
	return nil
}
func handleGroupExecution(cmd string, groups map[int][]*Prog, groupId int) error {
	// progs := 0
	cmds := strings.Split(cmd[4:], " || ")
	wg := &sync.WaitGroup{}
	groups[groupId] = make([]*Prog, len(cmds))
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
		groups[groupId][n] = p

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
					groups[groupId][p.id].status = "KILLED"
					return
				}
				groups[groupId][p.id].status = "ERROR:" + err.Error()
				killGroup(groups, groupId) // unexpected error, should terminate all the other memebers of the group
				return
			}
			groups[groupId][p.id].status = "FINISHED"

		}(p, args, p.killChan, groupChans)
	}
	wg.Wait()
	return nil
}

func main() {

	var (
		ipAddr string
		port   string
	)

	flag.StringVar(&ipAddr, "ip", "127.0.0.0", "The ip address of the environment. Used is case of a migration")
	flag.StringVar(&port, "p", ":3000", "The port of the environment. Used is case of a migration")
	flag.Parse()

	s := server.NewServer(ipAddr, port)
	go func() {
		log.Fatal(s.ListenAndAccept())
	}()

	threadsRunning := make(map[int]*Prog, 100)
	groups := make(map[int][]*Prog, 100)
	fmt.Println("SimpleScript Environment")
	scanner := bufio.NewScanner(os.Stdin)
	id := 0
scanLoop:
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "||") {
			go handleGroupExecution(scanner.Text(), groups, id)
			id++
			continue
		}
		t := strings.Split(scanner.Text(), " ")
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
			p := groups[gid][id]
			s.Migrate(ipAddr, port, p.name, p.interpreter)
			groups[gid][tid].status = "MIGRATED TO" + ipAddr + port
		case t[0] == "list":
			for id, p := range threadsRunning {
				fmt.Printf("> threadID: %d\tprogname: %s\tstatus: %s\n", id, p.name, p.status)
			}
			for id, progs := range groups {
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
			p, ok := threadsRunning[id]
			if ok {
				p.killChan <- struct{}{}
				continue
			}
			_, ok = groups[id]
			if ok {
				killGroup(groups, id)
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
			threadsRunning[id] = p
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
						threadsRunning[id].status = "KILLED"
						return
					}
					threadsRunning[id].status = "ERROR:" + err.Error()
					return
				}
				threadsRunning[id].status = "FINISHED"
			}(id, t[1], args, p.killChan)
			id++
		default:
			fmt.Println("Unknown command:", scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err.Error())
	}
}
