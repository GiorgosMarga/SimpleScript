package interpreter

// TODO: add groupid
import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"time"
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

type Interpreter struct {
	killChan     chan struct{}
	instructions [][]byte
	ctr          int
	variables    map[string]string
	argc         int
	threadId     int
	progName     string
	numOfInstr   int
	groupid      int
	groupChans   map[int]chan []byte
	fileSize     int
}

func NewInterpreter(threadId int, progName string, killChan chan struct{}, groupChans map[int]chan []byte, ctr int) *Interpreter {
	return &Interpreter{
		ctr:        ctr,
		variables:  make(map[string]string),
		threadId:   threadId,
		progName:   progName,
		killChan:   killChan,
		groupChans: groupChans,
	}
}

func (i *Interpreter) ReadInstructions(f io.Reader, args []string) error {
	instr, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	i.argc = len(args)
	for n, arg := range args {
		argName := fmt.Sprintf("$arg%d", n+1)
		i.variables[argName] = arg
	}

	splitted := bytes.Split(instr, []byte{'\n'})

	i.instructions = splitted
	for ctr, line := range splitted {
		i.numOfInstr++
		i.fileSize += len(line)
		if len(line) == 0 {
			continue
		}
		if line[0] == '#' {
			labelName := line[:len(line)-1] // ex. #Label1: (ignore ':')
			i.variables[string(labelName)] = strconv.Itoa(ctr)
		}
	}
	return nil
}
func (i *Interpreter) Run() error {
	for i.ctr < i.numOfInstr {
		select {
		case <-i.killChan:
			fmt.Printf("[groupid: %d, threadID: %d, progName: %s] > Received termination signal\n", i.groupid, i.threadId, i.progName)
			return ErrSignalKilled
		default:
			if err := i.readLine(); err != nil {
				fmt.Printf("[groupid: %d, threadID: %d, progName: %s] > Error: %s\n", i.groupid, i.threadId, i.progName, err.Error())
				return err
			}
		}
	}
	return nil
}

func (i *Interpreter) readLine() error {
	line := i.instructions[i.ctr]
	line = bytes.TrimSpace(line)

	tkns := bytes.Split(line, []byte{' '})
	// fmt.Printf("[threadID: %d, progName: %s] > %s\n", i.threadId, i.progName, string(line))

	switch {
	case bytes.Equal(tkns[0], []byte(Set)):
		i.ctr++
		return i.HandleSet(tkns)
	case bytes.Equal(tkns[0], []byte(Add)) || bytes.Equal(tkns[0], []byte(Sub)) || bytes.Equal(tkns[0], []byte(Mul)) || bytes.Equal(tkns[0], []byte(Div)) || bytes.Equal(tkns[0], []byte(Mod)):
		i.ctr++
		return i.HandleMathOperation(tkns)
	case bytes.Equal(tkns[0], []byte(Bgt)) || bytes.Equal(tkns[0], []byte(Bge)) || bytes.Equal(tkns[0], []byte(Blt)) || bytes.Equal(tkns[0], []byte(Ble)) || bytes.Equal(tkns[0], []byte(Beq)):
		return i.HandleBranchOperations(tkns)
	case bytes.Equal(tkns[0], []byte(Bra)):
		return i.HandleBranch(tkns)
	case bytes.Equal(tkns[0], []byte(Slp)):
		i.ctr++
		return i.HandleSleep(tkns)
	case bytes.Equal(tkns[0], []byte(Prn)):
		i.ctr++
		return i.HandlePrint(tkns)
	case bytes.Equal(tkns[0], []byte(Snd)) || bytes.Equal(tkns[0], []byte(Rcv)):
		i.ctr++
		return i.HandleSndRcv(tkns)
	default:
		i.ctr++
	}
	return nil
}
func (i *Interpreter) HandleSndRcv(tokens [][]byte) error {
	// Split in the correct way, so strings can contain spaces
	jnd := bytes.Join(tokens, []byte{' '})
	tokens = bytes.SplitN(jnd, []byte{' '}, 3)
	if len(tokens) != 3 {
		return fmt.Errorf("%w line: %d: expected SND VarVal {VarVal}\nGot %s", ErrInvalidInstruction, i.ctr, tokens)
	}
	if !isVarVal(tokens[1]) {
		return fmt.Errorf("%w line: %d: expected SND VarVal {VarVal}\nGot %s", ErrInvalidVal, i.ctr, tokens)
	}
	if !isVarVal(tokens[2]) {
		return fmt.Errorf("%w line: %d: expected SND VarVal {VarVal}\nGot %s", ErrInvalidVal, i.ctr, tokens)
	}
	var (
		val1 = string(tokens[1])
		val2 = string(tokens[2])
		ok   bool
	)
	if isVarName(tokens[1]) {
		val1, ok = i.variables[string(tokens[1])]
		if !ok {
			return fmt.Errorf("%w line: %d: undeclared variable: %s", ErrUndeclaredVariable, i.ctr, string(tokens[1]))
		}
		if !isIntVal([]byte(val1)) {
			return fmt.Errorf("%w line: %d: invalid value: %s", ErrInvalidIntVal, i.ctr, val1)
		}
	}

	if isVarName(tokens[2]) {
		val2, ok = i.variables[string(tokens[2])]
		if !ok {
			return fmt.Errorf("%w line: %d: undeclared variable: %s", ErrUndeclaredVariable, i.ctr, string(tokens[2]))
		}
		if !isIntVal([]byte(val2)) {
			return fmt.Errorf("%w line: %d: invalid value: %s", ErrInvalidIntVal, i.ctr, val2)
		}
	}

	id, _ := strconv.Atoi(val1)
	if bytes.Equal(tokens[0], []byte(Snd)) {
		i.groupChans[id] <- []byte(val2)
	} else {
		<-i.groupChans[id]
	}

	return nil
}
func (i *Interpreter) HandleSleep(tokens [][]byte) error {
	if len(tokens) != 2 {
		return fmt.Errorf("%w line: %d: expected SLP ValVarI\nGot %s", ErrInvalidInstruction, i.ctr, tokens)
	}
	if !isVarValI(tokens[1]) {
		return fmt.Errorf("%w line: %d: expected SLP ValVarI\nGot %s", ErrInvalidIntVal, i.ctr, tokens)
	}
	sleepVal := string(tokens[1])
	if isVarName(tokens[1]) {
		val, ok := i.variables[string(tokens[1])]
		if !ok {
			return fmt.Errorf("%w line: %d: undeclared variable: %s", ErrUndeclaredVariable, i.ctr, string(tokens[1]))
		}
		if !isIntVal([]byte(val)) {
			return fmt.Errorf("%w line: %d: invalid value: %s", ErrInvalidIntVal, i.ctr, val)
		}
		sleepVal = val
	}
	d, _ := strconv.Atoi(sleepVal)
	time.Sleep(time.Duration(d) * time.Second)
	return nil
}
func (i *Interpreter) HandlePrint(tokens [][]byte) error {
	jnd := bytes.Join(tokens, []byte{' '})
	tokens = bytes.SplitN(jnd, []byte{' '}, 2)

	if len(tokens) != 2 {
		return fmt.Errorf("%w line: %d: expected PRN VarVal\nGot %s", ErrInvalidInstruction, i.ctr, tokens)
	}
	if !isVarVal(tokens[1]) {
		return fmt.Errorf("%w line: %d: expected PRN VarVal\nGot %s", ErrInvalidVal, i.ctr, tokens)
	}
	var (
		val = string(tokens[1])
		ok  bool
	)
	if isVarName(tokens[1]) {
		val, ok = i.variables[string(tokens[1])]
		if !ok {
			return fmt.Errorf("%w line: %d: undeclared variable: %s", ErrUndeclaredVariable, i.ctr, string(tokens[1]))
		}
	}
	fmt.Printf("[threadID: %d, progName: %s] > %s\n", i.threadId, i.progName, val)
	return nil
}
func (i *Interpreter) HandleBranch(tokens [][]byte) error {
	if len(tokens) != 2 {
		return fmt.Errorf("%w line: %d: expected BRA Label\nGot %s", ErrInvalidInstruction, i.ctr, tokens)
	}
	if !isLabel(tokens[1]) {
		return fmt.Errorf("%w line: %d: expected BRA Label\nGot %s", ErrInvalidLabel, i.ctr, tokens)
	}
	val, ok := i.variables[string(tokens[1])]
	if !ok {
		return fmt.Errorf("%w: Undeclared label: %s", ErrUndeclaredVariable, string(tokens[3]))
	}
	i.ctr, _ = strconv.Atoi(val)
	return nil
}
func (i *Interpreter) HandleMathOperation(tokens [][]byte) error {
	if len(tokens) != 4 {
		return fmt.Errorf("%w line: %d: expected OPERATION VarName VarVal1 VarVal2\nGot %s", ErrInvalidInstruction, i.ctr, tokens)
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
		return fmt.Errorf("%w line [%d]: expected '$'{Letter | Digit}\nGot %s", ErrInvalidVarName, i.ctr, string(varName))
	}

	if !isVarValI(tokens[2]) || !isVarValI(tokens[3]) {
		return fmt.Errorf("%w line [%d]: expected VarValI | VarValS\nGot %sand %s", ErrInvalidVal, i.ctr, string(tokens[2]), string(tokens[3]))
	}

	if isVarName(tokens[2]) {
		val, ok := i.variables[string(tokens[2])]
		if !ok {
			return fmt.Errorf("%w line [%d]: undeclared variable: %s", ErrUndeclaredVariable, i.ctr, string(tokens[2]))
		}
		varVal1 = val
	}
	if isVarName(tokens[3]) {
		val, ok := i.variables[string(tokens[3])]
		if !ok {
			return fmt.Errorf("%w line [%d]: undeclared variable: %s", ErrUndeclaredVariable, i.ctr, string(tokens[3]))
		}
		varVal2 = val
	}
	if val1, err = strconv.Atoi(varVal1); err != nil {
		return fmt.Errorf("%w line [%d]: not an integer: %s", ErrInvalidIntVal, i.ctr, varVal1)
	}
	if val2, err = strconv.Atoi(varVal2); err != nil {
		return fmt.Errorf("%w line [%d]: not an integer: %s", ErrInvalidIntVal, i.ctr, varVal1)
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
			return fmt.Errorf("%w line [%d]: cant devide with 0: %s", ErrInvalidIntVal, i.ctr, tokens[:])
		}
		res = strconv.Itoa(val1 / val2)
	case bytes.Equal(tokens[0], []byte(Mod)):
		if val2 == 0 {
			return fmt.Errorf("%w line [%d]: cant mod with 0: %s", ErrInvalidIntVal, i.ctr, tokens[:])
		}
		res = strconv.Itoa(val1 % val2)
	}
	i.variables[string(varName)] = res
	return nil
}

func (i *Interpreter) HandleBranchOperations(tokens [][]byte) error {
	if len(tokens) != 4 {
		return fmt.Errorf("%w line: %d: expected OPERATION VarValI1 VarValI2 Label\nGot %s", ErrInvalidInstruction, i.ctr, tokens)
	}
	var (
		varVal1 = string(tokens[2])
		varVal2 = string(tokens[3])
		val1    int
		val2    int
		err     error
		branch  = false
	)

	if !isLabel(tokens[3]) {
		return fmt.Errorf("%w line: %d: expected OPERATION VarValI1 VarValI2 Label\nGot %s", ErrInvalidLabel, i.ctr, tokens)
	}

	if !isVarValI(tokens[1]) || !isVarValI(tokens[2]) {
		return fmt.Errorf("%w line [%d]: expected VarValI | VarValS\nGot %sand %s", ErrInvalidVal, i.ctr, string(tokens[1]), string(tokens[2]))
	}

	if isVarName(tokens[1]) {
		val, ok := i.variables[string(tokens[1])]
		if !ok {
			return fmt.Errorf("%w line [%d]: undeclared variable: %s", ErrUndeclaredVariable, i.ctr, string(tokens[2]))
		}
		varVal1 = val
	}
	if isVarName(tokens[2]) {
		val, ok := i.variables[string(tokens[2])]
		if !ok {
			return fmt.Errorf("%w line [%d]: undeclared variable: %s", ErrUndeclaredVariable, i.ctr, string(tokens[2]))
		}
		varVal2 = val
	}

	if val1, err = strconv.Atoi(varVal1); err != nil {
		return fmt.Errorf("%w line [%d]: not an integer: %s", ErrInvalidIntVal, i.ctr, varVal1)
	}
	if val2, err = strconv.Atoi(varVal2); err != nil {
		return fmt.Errorf("%w line [%d]: not an integer: %s", ErrInvalidIntVal, i.ctr, varVal1)
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
		i.ctr++
		return nil
	}
	val, ok := i.variables[string(tokens[3])]
	if !ok {
		return fmt.Errorf("%w: Undeclared label: %s", ErrUndeclaredVariable, string(tokens[3]))
	}
	i.ctr, _ = strconv.Atoi(val)
	return nil
}
func (i *Interpreter) HandleSet(tokens [][]byte) error {
	if len(tokens) != 3 {
		return fmt.Errorf("%w line: %d: expected SET VarName VarVal\nGot %s", ErrInvalidInstruction, i.ctr, tokens)
	}
	var (
		varName = tokens[1]
		varVal  = tokens[2]
	)

	if !isVarName(varName) {
		return fmt.Errorf("%w line [%d]: expected '$'{Letter | Digit}\nGot %s", ErrInvalidVarName, i.ctr, string(varName))
	}

	if !isVarVal(varVal) {
		return fmt.Errorf("%w line [%d]: expected VarValI | VarValS\nGot %s", ErrInvalidVal, i.ctr, string(varVal))
	}

	if isVarName(varVal) {
		val, ok := i.variables[string(varVal)]
		if !ok {
			return fmt.Errorf("line [%d]: undeclared variable %s", i.ctr, string(varVal))
		}
		i.variables[string(varName)] = val
		return nil
	}
	i.variables[string(varName)] = string(varVal)
	return nil
}

func (i *Interpreter) GetPos() int {
	return i.ctr
}

func (i *Interpreter) FileSize() int {
	return i.fileSize
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
