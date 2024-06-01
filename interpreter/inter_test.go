package interpreter

import (
	"fmt"
	"os"
	"testing"
)

// func TestSet(t *testing.T) {
// 	tests := []struct {
// 		name  string
// 		input io.Reader
// 	}{
// 		{"Working SET", bytes.NewReader([]byte("SET $val1 10"))},
// 		{"Invalid VarName", bytes.NewReader([]byte("SET val1 10"))},
// 		{"Invalid VarVal", bytes.NewReader([]byte("SET $val1 #10"))},
// 		{"Undeclared Variable", bytes.NewReader([]byte("SET $val1 $val2"))},
// 	}

// 	for n, tt := range tests {
// 		i := NewIntrepreter(n, tt.name)
// 		i.ReadInstructions(tt.input, []string{})
// 		t.Run(tt.name, func(t *testing.T) {
// 			err := i.ReadLine()
// 			fmt.Println(err)
// 		})
// 	}
// }

// func TestAdd(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		input    io.Reader
// 		expected error
// 	}{
// 		{"Working", bytes.NewReader([]byte("ADD $val1 10 10")), nil},
// 		{"Invalid Instruction", bytes.NewReader([]byte("ADD $val1 10")), ErrInvalidInstruction},
// 		{"Invalid VarName", bytes.NewReader([]byte("ADD val1 10 10")), ErrInvalidVarName},
// 		{"Invalid VarVal1", bytes.NewReader([]byte("ADD $val1 #10 10")), ErrInvalidVal},
// 		{"Invalid VarVal2", bytes.NewReader([]byte("ADD $val1 #10 10")), ErrInvalidVal},
// 		{"Undeclared Variable", bytes.NewReader([]byte("ADD $val1 $val2 10")), ErrUndeclaredVariable},
// 	}

// 	for n, tt := range tests {
// 		i := NewIntrepreter(n, tt.name)
// 		i.ReadInstructions(tt.input, []string{})
// 		t.Run(tt.name, func(t *testing.T) {
// 			err := i.ReadLine()
// 			if !errors.Is(err, tt.expected) {
// 				t.FailNow()
// 			}
// 		})
// 	}
// }

// func TestSub(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		input    io.Reader
// 		expected error
// 	}{
// 		{"Working", bytes.NewReader([]byte("SUB $val1 10 10")), nil},
// 		{"Invalid Instruction", bytes.NewReader([]byte("SUB $val1 10")), ErrInvalidInstruction},
// 		{"Invalid VarName", bytes.NewReader([]byte("SUB val1 10 10")), ErrInvalidVarName},
// 		{"Invalid VarVal1", bytes.NewReader([]byte("SUB $val1 #10 10")), ErrInvalidVal},
// 		{"Invalid VarVal2", bytes.NewReader([]byte("SUB $val1 #10 10")), ErrInvalidVal},
// 		{"Undeclared Variable", bytes.NewReader([]byte("SUB $val1 $val2 10")), ErrUndeclaredVariable},
// 	}

// 	for n, tt := range tests {
// 		i := NewIntrepreter(n, tt.name)
// 		i.ReadInstructions(tt.input, []string{})
// 		t.Run(tt.name, func(t *testing.T) {
// 			err := i.ReadLine()
// 			if !errors.Is(err, tt.expected) {
// 				t.FailNow()
// 			}
// 		})
// 	}
// }

// func TestMul(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		input    io.Reader
// 		expected error
// 	}{
// 		{"Working", bytes.NewReader([]byte("MUL $val1 10 10")), nil},
// 		{"Invalid Instruction", bytes.NewReader([]byte("MUL $val1 10")), ErrInvalidInstruction},
// 		{"Invalid VarName", bytes.NewReader([]byte("MUL val1 10 10")), ErrInvalidVarName},
// 		{"Invalid VarVal1", bytes.NewReader([]byte("MUL $val1 #10 10")), ErrInvalidVal},
// 		{"Invalid VarVal2", bytes.NewReader([]byte("MUL $val1 #10 10")), ErrInvalidVal},
// 		{"Undeclared Variable", bytes.NewReader([]byte("MUL $val1 $val2 10")), ErrUndeclaredVariable},
// 	}

// 	for n, tt := range tests {
// 		i := NewIntrepreter(n, tt.name)
// 		i.ReadInstructions(tt.input, []string{})
// 		t.Run(tt.name, func(t *testing.T) {
// 			err := i.ReadLine()
// 			if !errors.Is(err, tt.expected) {
// 				t.FailNow()
// 			}
// 		})
// 	}
// }

// func TestDiv(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		input    io.Reader
// 		expected error
// 	}{
// 		{"Working", bytes.NewReader([]byte("DIV $val1 10 10")), nil},
// 		{"Invalid Instruction", bytes.NewReader([]byte("DIV $val1 10")), ErrInvalidInstruction},
// 		{"Invalid VarName", bytes.NewReader([]byte("DIV val1 10 10")), ErrInvalidVarName},
// 		{"Invalid VarVal1", bytes.NewReader([]byte("DIV $val1 #10 10")), ErrInvalidVal},
// 		{"Invalid VarVal2", bytes.NewReader([]byte("DIV $val1 #10 10")), ErrInvalidVal},
// 		{"Undeclared Variable", bytes.NewReader([]byte("DIV $val1 $val2 10")), ErrUndeclaredVariable},
// 		{"Div with 0", bytes.NewReader([]byte("DIV $val1 10 0")), ErrInvalidIntVal},
// 	}

// 	for n, tt := range tests {
// 		i := NewIntrepreter(n, tt.name)
// 		i.ReadInstructions(tt.input, []string{})
// 		t.Run(tt.name, func(t *testing.T) {
// 			err := i.ReadLine()
// 			if !errors.Is(err, tt.expected) {
// 				t.FailNow()
// 			}
// 		})
// 	}
// }

// func TestMod(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		input    io.Reader
// 		expected error
// 	}{
// 		{"Working", bytes.NewReader([]byte("MOD $val1 10 10")), nil},
// 		{"Invalid Instruction", bytes.NewReader([]byte("MOD $val1 10")), ErrInvalidInstruction},
// 		{"Invalid VarName", bytes.NewReader([]byte("MOD val1 10 10")), ErrInvalidVarName},
// 		{"Invalid VarVal1", bytes.NewReader([]byte("MOD $val1 #10 10")), ErrInvalidVal},
// 		{"Invalid VarVal2", bytes.NewReader([]byte("MOD $val1 #10 10")), ErrInvalidVal},
// 		{"Undeclared Variable", bytes.NewReader([]byte("MOD $val1 $val2 10")), ErrUndeclaredVariable},
// 		{"Div with 0", bytes.NewReader([]byte("MOD $val1 10 0")), ErrInvalidIntVal},
// 	}

// 	for n, tt := range tests {
// 		i := NewInterpreter(n, tt.name)
// 		i.ReadInstructions(tt.input, []string{})
// 		t.Run(tt.name, func(t *testing.T) {
// 			err := i.ReadLine()
// 			if !errors.Is(err, tt.expected) {
// 				t.FailNow()
// 			}
// 		})
// 	}
// }

func TestPrint(t *testing.T) {
	p := &Prog{
		Name: "TEST_PRINT",
		Id:   0,
	}
	i := NewInterpreter(p, nil, 0)
	// instructions := bytes.NewReader([]byte("SET $val1 10\nPRN \"this is \" $val1"))

	f, err := os.Open("../printTest.ss")
	if err != nil {
		fmt.Println(err)
		t.FailNow()
	}
	defer f.Close()

	i.ReadInstructions(f, []string{})

	i.Run()
}
