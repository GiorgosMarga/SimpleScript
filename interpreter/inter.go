package interpreter

// TODO: add Groupid
import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/GiorgosMarga/simplescript/msgs"
)

const (
	Set = "SET"
	Add = "ADD"
	Sub = "SUB"
	Mul = "MUL"
	Div = "DIV"
	Mod = "MOD"
	Bgt = "BGT"
	Bge = "BGE"
	Blt = "BLT"
	Ble = "BLE"
	Beq = "BEQ"
	Bra = "BRA"
	Snd = "SND"
	Rcv = "RCV"
	Slp = "SLP"
	Prn = "PRN"
	Ret = "RET"
)

type Prog struct {
	Name     string
	Status   string
	killChan chan struct{}
	ThreadId int
	GroupId  int
}
type Interpreter struct {
	Instructions [][]byte
	Ctr          int
	Variables    map[string]string
	Argc         int
	NumOfInstr   int
	groupChans   map[int]chan []byte
	Program      *Prog
	SleepUntil   time.Time
	MsgChan      chan *msgs.Msg
	MigratedFrom string
	Conn         net.Conn
}

func NewProg(name string, threadid int) *Prog {
	return &Prog{
		Name:     name,
		Status:   "RUNNING",
		ThreadId: threadid,
		killChan: make(chan struct{}),
	}
}

func (p *Prog) KillProg(status string) {
	if p.Status != "RUNNING" {
		return
	}
	fmt.Printf("Sending termination signal to %s (%d)\n", p.Name, p.ThreadId)
	p.killChan <- struct{}{}
	p.Status = status
}
func NewInterpreter(p *Prog, groupChans map[int]chan []byte, Ctr int, msgChan chan *msgs.Msg) *Interpreter {
	return &Interpreter{
		Ctr:        Ctr,
		Variables:  make(map[string]string),
		Program:    p,
		groupChans: groupChans,
		SleepUntil: time.Now(),
		MsgChan:    msgChan,
	}
}

func (i *Interpreter) ReadInstructions(f io.Reader, args []string) error {
	instr, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	i.Argc = len(args)
	for n, arg := range args {
		argName := fmt.Sprintf("$argv%d", n)
		i.Variables[argName] = arg
	}

	splitted := bytes.Split(instr, []byte{'\n'})
	i.Variables["$argc"] = strconv.Itoa(len(args))
	i.Instructions = splitted
	for ctr, line := range splitted {
		line := bytes.TrimSpace(line)
		i.NumOfInstr++
		if len(line) == 0 {
			continue
		}
		if line[0] == '#' {
			i.Variables[string(line)] = strconv.Itoa(ctr)
		}
	}
	return nil
}
func (i *Interpreter) Run() error {
	for i.Ctr < i.NumOfInstr {
		select {
		case <-i.Program.killChan:
			fmt.Printf("[Groupid: %d, threadID: %d, ProgName: %s] > Received termination signal\n", i.Program.GroupId, i.Program.ThreadId, i.Program.Name)
			return ErrSignalKilled
		default:
			if err := i.readLine(); err != nil {
				if !errors.Is(err, ErrSignalKilled) {
					fmt.Printf("[Groupid: %d, threadID: %d, ProgName: %s] > Error: %s\n", i.Program.GroupId, i.Program.ThreadId, i.Program.Name, err.Error())
				}
				return err
			}
		}
	}
	return nil
}

func (i *Interpreter) readLine() error {
	line := i.Instructions[i.Ctr]
	line = bytes.TrimSpace(line)

	tkns := bytes.Split(line, []byte{' '})
	// fmt.Printf("[threadID: %d, ProgName: %s] > %s\n", i.ThreadId, i.ProgName, string(line))

	switch {
	case bytes.Equal(tkns[0], []byte(Set)):
		i.Ctr++
		return i.HandleSet(tkns)
	case bytes.Equal(tkns[0], []byte(Add)) || bytes.Equal(tkns[0], []byte(Sub)) || bytes.Equal(tkns[0], []byte(Mul)) || bytes.Equal(tkns[0], []byte(Div)) || bytes.Equal(tkns[0], []byte(Mod)):
		i.Ctr++
		return i.HandleMathOperation(tkns)
	case bytes.Equal(tkns[0], []byte(Bgt)) || bytes.Equal(tkns[0], []byte(Bge)) || bytes.Equal(tkns[0], []byte(Blt)) || bytes.Equal(tkns[0], []byte(Ble)) || bytes.Equal(tkns[0], []byte(Beq)):
		return i.HandleBranchOperations(tkns)
	case bytes.Equal(tkns[0], []byte(Bra)):
		return i.HandleBranch(tkns)
	case bytes.Equal(tkns[0], []byte(Slp)):
		i.Ctr++
		return i.HandleSleep(tkns)
	case bytes.Equal(tkns[0], []byte(Prn)):
		i.Ctr++
		return i.HandlePrint(tkns)
	case bytes.Equal(tkns[0], []byte(Snd)):
		i.Ctr++
		return i.HandleSnd(tkns)
	case bytes.Equal(tkns[0], []byte(Rcv)):
		i.Ctr++
		return i.HandleRcv(tkns)
	case bytes.Equal(tkns[0], []byte(Ret)):
		i.Ctr = i.NumOfInstr // for loop in run() stops
	default:
		i.Ctr++
	}
	return nil
}

func (i *Interpreter) HandleRcv(tokens [][]byte) error {
	// Split in the correct way, so strings can contain spaces
	jnd := bytes.Join(tokens, []byte{' '})
	tokens = bytes.SplitN(jnd, []byte{' '}, 3)
	if len(tokens) != 3 {
		return fmt.Errorf("%w line: %d: expected RCV VarVal {VarVal}\nGot %s", ErrInvalidInstruction, i.Ctr, tokens)
	}
	if !isVarVal(tokens[1]) {
		return fmt.Errorf("%w line: %d: expected RCV VarVal {VarVal}\nGot %s", ErrInvalidVal, i.Ctr, tokens)
	}
	if !isVarVal(tokens[2]) {
		return fmt.Errorf("%w line: %d: expected RCV VarVal {VarVal}\nGot %s", ErrInvalidVal, i.Ctr, tokens)
	}
	var (
		val1 = string(tokens[1])
		val2 = string(tokens[2])
		ok   bool
	)
	if isVarName(tokens[1]) {
		val1, ok = i.Variables[string(tokens[1])]
		if !ok {
			return fmt.Errorf("%w line: %d: undeclared variable: %s", ErrUndeclaredVariable, i.Ctr, string(tokens[1]))
		}
		if !isIntVal([]byte(val1)) {
			return fmt.Errorf("%w line: %d: invalid value: %s", ErrInvalidIntVal, i.Ctr, val1)
		}
	}
	id, _ := strconv.Atoi(val1)
	for {
		select {
		case data := <-i.groupChans[id]:
			i.Variables[val2] = string(data)
			return nil
		case <-i.Program.killChan:
			return ErrSignalKilled
		}
	}
}
func (i *Interpreter) HandleSnd(tokens [][]byte) error {
	// Split in the correct way, so strings can contain spaces
	jnd := bytes.Join(tokens, []byte{' '})
	tokens = bytes.SplitN(jnd, []byte{' '}, 3)
	if len(tokens) != 3 {
		return fmt.Errorf("%w line: %d: expected SND VarVal {VarVal}\nGot %s", ErrInvalidInstruction, i.Ctr, tokens)
	}
	if !isVarVal(tokens[1]) {
		return fmt.Errorf("%w line: %d: expected SND VarVal {VarVal}\nGot %s", ErrInvalidVal, i.Ctr, tokens)
	}
	if !isVarVal(tokens[2]) {
		return fmt.Errorf("%w line: %d: expected SND VarVal {VarVal}\nGot %s", ErrInvalidVal, i.Ctr, tokens)
	}
	var (
		val1 = string(tokens[1])
		val2 = string(tokens[2])
		ok   bool
	)
	if isVarName(tokens[1]) {
		val1, ok = i.Variables[string(tokens[1])]
		if !ok {
			return fmt.Errorf("%w line: %d: undeclared variable: %s", ErrUndeclaredVariable, i.Ctr, string(tokens[1]))
		}
		if !isIntVal([]byte(val1)) {
			return fmt.Errorf("%w line: %d: invalid value: %s", ErrInvalidIntVal, i.Ctr, val1)
		}
	}

	if isVarName(tokens[2]) {
		val2, ok = i.Variables[string(tokens[2])]
		if !ok {
			return fmt.Errorf("%w line: %d: undeclared variable: %s", ErrUndeclaredVariable, i.Ctr, string(tokens[2]))
		}
	}
	id, _ := strconv.Atoi(val1)
	if len(i.MigratedFrom) > 0 {
		m := &msgs.Msg{
			To:   id,
			Val:  []byte(val2),
			Conn: i.Conn,
		}
		i.MsgChan <- m
		return nil
	}
	i.groupChans[id] <- []byte(val2)

	return nil
}
func (i *Interpreter) HandleSleep(tokens [][]byte) error {
	if len(tokens) != 2 {
		return fmt.Errorf("%w line: %d: expected SLP ValVarI\nGot %s", ErrInvalidInstruction, i.Ctr, tokens)
	}
	if !isVarValI(tokens[1]) {
		return fmt.Errorf("%w line: %d: expected SLP ValVarI\nGot %s", ErrInvalidIntVal, i.Ctr, tokens)
	}
	sleepVal := string(tokens[1])
	if isVarName(tokens[1]) {
		val, ok := i.Variables[string(tokens[1])]
		if !ok {
			return fmt.Errorf("%w line: %d: undeclared variable: %s", ErrUndeclaredVariable, i.Ctr, string(tokens[1]))
		}
		if !isIntVal([]byte(val)) {
			return fmt.Errorf("%w line: %d: invalid value: %s", ErrInvalidIntVal, i.Ctr, val)
		}
		sleepVal = val
	}
	d, _ := strconv.Atoi(sleepVal)
	var timer *time.Timer
	if time.Now().Before(i.SleepUntil) {
		timer = time.NewTimer(time.Duration(time.Until(i.SleepUntil).Seconds()) * time.Second)
		i.SleepUntil = time.Now().Add(time.Duration(time.Until(i.SleepUntil).Seconds()) * time.Second)
	} else {
		timer = time.NewTimer(time.Duration(d) * time.Second)
		i.SleepUntil = time.Now().Add(time.Duration(d) * time.Second)
	}
	fmt.Println("Sleeping for")
	for {
		select {
		case <-timer.C:
			return nil
		case <-i.Program.killChan:
			return ErrSignalKilled
		}
	}
}

func parseString(b []byte) (int, string) {
	var (
		i   = 0
		res string
	)
	// first character is "
	for i = 1; i < len(b); i++ {
		if b[i] == '"' {
			break
		}
		res += string(b[i])
	}
	return i + 1, res
}

func parseVar(b []byte) (int, string) {
	var (
		ch  byte
		i   = 0
		res string
	)
	for i, ch = range b {
		if ch == ' ' {
			break
		}
		res += string(ch)
	}
	return i, res
}
func (i *Interpreter) HandlePrint(tokens [][]byte) error {
	jnd := bytes.Join(tokens, []byte{' '})
	tokens = bytes.SplitN(jnd, []byte{' '}, 2)

	if len(tokens) != 2 {
		return fmt.Errorf("%w line: %d: expected PRN VarVal\nGot %s", ErrInvalidInstruction, i.Ctr, tokens)
	}
	var (
		varName string
		ok      bool
		str     string
		n       int
	)

	strBldr := &strings.Builder{}
	for j := 0; j < len(tokens[1]); {
		if tokens[1][j] == '"' {
			n, str = parseString(tokens[1][j:])
		} else if tokens[1][j] == '$' {
			n, varName = parseVar(tokens[1][j:])
			str, ok = i.Variables[varName]
			if !ok {
				return fmt.Errorf("%w line: %d: undeclared variable: %s", ErrUndeclaredVariable, i.Ctr, str)
			}
		} else {
			j++
			continue
		}
		strBldr.WriteString(str)
		j += n
	}
	fmt.Printf("[threadID: %d, ProgName: %s] > %s\n", i.Program.ThreadId, i.Program.Name, strBldr.String())
	return nil
}
func (i *Interpreter) HandleBranch(tokens [][]byte) error {
	if len(tokens) != 2 {
		return fmt.Errorf("%w line: %d: expected BRA Label\nGot %s", ErrInvalidInstruction, i.Ctr, tokens)
	}
	if !isLabel(tokens[1]) {
		return fmt.Errorf("%w line: %d: expected BRA Label\nGot %s", ErrInvalidLabel, i.Ctr, tokens)
	}
	val, ok := i.Variables[string(tokens[1])]
	if !ok {
		return fmt.Errorf("%w: Undeclared label: %s", ErrUndeclaredVariable, string(tokens[3]))
	}
	i.Ctr, _ = strconv.Atoi(val)
	return nil
}
func (i *Interpreter) HandleMathOperation(tokens [][]byte) error {
	if len(tokens) != 4 {
		return fmt.Errorf("%w line: %d: expected OPERATION VarName VarVal1 VarVal2\nGot %s", ErrInvalidInstruction, i.Ctr, tokens)
	}
	var (
		varName = tokens[1]
		varVal1 = string(tokens[2])
		varVal2 = string(tokens[3])
		res     string
		val1    int
		val2    int
		err     error
	)

	if !isVarName(varName) {
		return fmt.Errorf("%w line [%d]: expected '$'{Letter | Digit}\nGot %s", ErrInvalidVarName, i.Ctr, string(varName))
	}

	if !isVarValI(tokens[2]) || !isVarValI(tokens[3]) {
		return fmt.Errorf("%w line [%d]: expected VarValI | VarValS\nGot %sand %s", ErrInvalidVal, i.Ctr, string(tokens[2]), string(tokens[3]))
	}

	if isVarName(tokens[2]) {
		val, ok := i.Variables[string(tokens[2])]
		if !ok {
			return fmt.Errorf("%w line [%d]: undeclared variable: %s", ErrUndeclaredVariable, i.Ctr, string(tokens[2]))
		}
		varVal1 = val
	}
	if isVarName(tokens[3]) {
		val, ok := i.Variables[string(tokens[3])]
		if !ok {
			return fmt.Errorf("%w line [%d]: undeclared variable: %s", ErrUndeclaredVariable, i.Ctr, string(tokens[3]))
		}
		varVal2 = val
	}
	if val1, err = strconv.Atoi(varVal1); err != nil {
		return fmt.Errorf("%w line [%d]: not an integer: %s", ErrInvalidIntVal, i.Ctr, varVal1)
	}
	if val2, err = strconv.Atoi(varVal2); err != nil {
		return fmt.Errorf("%w line [%d]: not an integer: %s", ErrInvalidIntVal, i.Ctr, varVal1)
	}

	switch {
	case bytes.Equal(tokens[0], []byte(Add)):
		res = strconv.Itoa(val1 + val2)
	case bytes.Equal(tokens[0], []byte(Sub)):
		res = strconv.Itoa(val1 - val2)
	case bytes.Equal(tokens[0], []byte(Mul)):
		res = strconv.Itoa(val1 * val2)
	case bytes.Equal(tokens[0], []byte(Div)):
		if val2 == 0 {
			return fmt.Errorf("%w line [%d]: cant devide with 0: %s", ErrInvalidIntVal, i.Ctr, tokens[:])
		}
		res = strconv.Itoa(val1 / val2)
	case bytes.Equal(tokens[0], []byte(Mod)):
		if val2 == 0 {
			return fmt.Errorf("%w line [%d]: cant mod with 0: %s", ErrInvalidIntVal, i.Ctr, tokens[:])
		}
		res = strconv.Itoa(val1 % val2)
	}
	i.Variables[string(varName)] = res
	return nil
}

func (i *Interpreter) HandleBranchOperations(tokens [][]byte) error {
	if len(tokens) != 4 {
		return fmt.Errorf("%w line: %d: expected OPERATION VarValI1 VarValI2 Label\nGot %s", ErrInvalidInstruction, i.Ctr, tokens)
	}
	var (
		varVal1 = string(tokens[1])
		varVal2 = string(tokens[2])
		val1    int
		val2    int
		err     error
		branch  = false
	)

	if !isLabel(tokens[3]) {
		return fmt.Errorf("%w line: %d: expected OPERATION VarValI1 VarValI2 Label\nGot %s", ErrInvalidLabel, i.Ctr, tokens)
	}

	if !isVarValI(tokens[1]) || !isVarValI(tokens[2]) {
		return fmt.Errorf("%w line [%d]: expected VarValI | VarValS\nGot %sand %s", ErrInvalidVal, i.Ctr, string(tokens[1]), string(tokens[2]))
	}

	if isVarName(tokens[1]) {
		val, ok := i.Variables[string(tokens[1])]
		if !ok {
			return fmt.Errorf("%w line [%d]: undeclared variable: %s", ErrUndeclaredVariable, i.Ctr, string(tokens[1]))
		}
		varVal1 = val
	}
	if isVarName(tokens[2]) {
		val, ok := i.Variables[string(tokens[2])]
		if !ok {
			return fmt.Errorf("%w line [%d]: undeclared variable: %s", ErrUndeclaredVariable, i.Ctr, string(tokens[2]))
		}
		varVal2 = val
	}
	if val1, err = strconv.Atoi(varVal1); err != nil {
		return fmt.Errorf("%w line [%d]: not an integer: %s", ErrInvalidIntVal, i.Ctr, varVal1)
	}
	if val2, err = strconv.Atoi(varVal2); err != nil {
		return fmt.Errorf("%w line [%d]: not an integer: %s", ErrInvalidIntVal, i.Ctr, varVal1)
	}

	switch {
	case bytes.Equal(tokens[0], []byte(Bgt)):
		branch = val1 > val2
	case bytes.Equal(tokens[0], []byte(Bge)):
		branch = val1 >= val2
	case bytes.Equal(tokens[0], []byte(Blt)):
		branch = val1 < val2
	case bytes.Equal(tokens[0], []byte(Ble)):
		branch = val1 <= val2
	case bytes.Equal(tokens[0], []byte(Beq)):
		branch = val1 == val2
	}
	if !branch {
		i.Ctr++
		return nil
	}
	val, ok := i.Variables[string(tokens[3])]
	if !ok {
		return fmt.Errorf("%w: Undeclared label: %s", ErrUndeclaredVariable, string(tokens[3]))
	}
	i.Ctr, _ = strconv.Atoi(val)
	return nil
}
func (i *Interpreter) HandleSet(tokens [][]byte) error {
	if len(tokens) != 3 {
		return fmt.Errorf("%w line: %d: expected SET VarName VarVal\nGot %s", ErrInvalidInstruction, i.Ctr, tokens)
	}
	var (
		varName = tokens[1]
		varVal  = tokens[2]
	)

	if !isVarName(varName) {
		return fmt.Errorf("%w line [%d]: expected '$'{Letter | Digit}\nGot %s", ErrInvalidVarName, i.Ctr, string(varName))
	}

	if !isVarVal(varVal) {
		return fmt.Errorf("%w line [%d]: expected VarValI | VarValS\nGot %s", ErrInvalidVal, i.Ctr, string(varVal))
	}

	if isVarName(varVal) {
		val, ok := i.Variables[string(varVal)]
		if !ok {
			return fmt.Errorf("line [%d]: undeclared variable %s", i.Ctr, string(varVal))
		}
		i.Variables[string(varName)] = val
		return nil
	}
	i.Variables[string(varName)] = string(varVal)
	return nil
}

func (i *Interpreter) GetPos() int {
	return i.Ctr
}

func isVarName(v []byte) bool {
	return v[0] == '$'
}

func isVarVal(v []byte) bool {
	return isVarValI(v) || isVarValS(v)
}

func isLabel(v []byte) bool {
	return v[0] == '#'
}
func isVarValS(v []byte) bool {
	return isVarName(v) || isString(v)
}
func isString(v []byte) bool {
	return v[0] == '"' && v[len(v)-1] == '"'
}
func isVarValI(v []byte) bool {
	return isVarName(v) || isIntVal(v)
}
func isIntVal(v []byte) bool {
	_, err := strconv.Atoi(string(v))
	return err == nil
}
