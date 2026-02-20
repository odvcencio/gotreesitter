package gotreesitter

import (
	"encoding/binary"
	"fmt"
	"unicode"
)

// ExternalVMOp is an opcode for the native-Go external scanner VM.
type ExternalVMOp uint8

const (
	ExternalVMOpFail ExternalVMOp = iota
	ExternalVMOpJump
	ExternalVMOpRequireValid
	ExternalVMOpRequireStateEq
	ExternalVMOpSetState
	ExternalVMOpIfRuneEq
	ExternalVMOpIfRuneInRange
	ExternalVMOpIfRuneClass
	ExternalVMOpAdvance
	ExternalVMOpMarkEnd
	ExternalVMOpEmit
)

// ExternalVMRuneClass is a character class used by ExternalVMOpIfRuneClass.
type ExternalVMRuneClass uint8

const (
	ExternalVMRuneClassWhitespace ExternalVMRuneClass = iota
	ExternalVMRuneClassDigit
	ExternalVMRuneClassLetter
	ExternalVMRuneClassWord
	ExternalVMRuneClassNewline
)

// ExternalVMInstr is one instruction in an external scanner VM program.
//
// Operands:
//   - A: primary operand (opcode-specific)
//   - B: secondary operand (used by range checks)
//   - Alt: alternate program counter when a condition fails
type ExternalVMInstr struct {
	Op  ExternalVMOp
	A   int32
	B   int32
	Alt int32
}

// ExternalVMProgram is a small bytecode program interpreted by ExternalVMScanner.
type ExternalVMProgram struct {
	Code     []ExternalVMInstr
	MaxSteps int // <=0 uses a safe default based on program size
}

// ExternalVMScanner executes an ExternalVMProgram and implements ExternalScanner.
type ExternalVMScanner struct {
	program ExternalVMProgram
}

type externalVMPayload struct {
	State uint32
}

// NewExternalVMScanner validates and constructs an ExternalVMScanner.
func NewExternalVMScanner(program ExternalVMProgram) (*ExternalVMScanner, error) {
	if err := validateExternalVMProgram(program); err != nil {
		return nil, err
	}
	return &ExternalVMScanner{program: program}, nil
}

// MustNewExternalVMScanner is like NewExternalVMScanner but panics on error.
func MustNewExternalVMScanner(program ExternalVMProgram) *ExternalVMScanner {
	s, err := NewExternalVMScanner(program)
	if err != nil {
		panic(err)
	}
	return s
}

func validateExternalVMProgram(program ExternalVMProgram) error {
	if len(program.Code) == 0 {
		return fmt.Errorf("external vm: empty program")
	}
	if program.MaxSteps < 0 {
		return fmt.Errorf("external vm: max steps must be >= 0")
	}

	codeLen := len(program.Code)
	for i, ins := range program.Code {
		switch ins.Op {
		case ExternalVMOpFail, ExternalVMOpMarkEnd, ExternalVMOpAdvance, ExternalVMOpEmit:
			// No control-flow validation needed.
		case ExternalVMOpJump:
			if err := validateExternalVMTarget(i, ins.A, codeLen, "A"); err != nil {
				return err
			}
		case ExternalVMOpRequireValid:
			if ins.A < 0 {
				return fmt.Errorf("external vm: instruction %d invalid valid-symbol index %d", i, ins.A)
			}
			if err := validateExternalVMTarget(i, ins.Alt, codeLen, "Alt"); err != nil {
				return err
			}
		case ExternalVMOpRequireStateEq:
			if ins.A < 0 {
				return fmt.Errorf("external vm: instruction %d invalid state value %d", i, ins.A)
			}
			if err := validateExternalVMTarget(i, ins.Alt, codeLen, "Alt"); err != nil {
				return err
			}
		case ExternalVMOpSetState:
			if ins.A < 0 {
				return fmt.Errorf("external vm: instruction %d invalid state value %d", i, ins.A)
			}
		case ExternalVMOpIfRuneEq:
			if err := validateExternalVMTarget(i, ins.Alt, codeLen, "Alt"); err != nil {
				return err
			}
		case ExternalVMOpIfRuneInRange:
			if ins.B < ins.A {
				return fmt.Errorf("external vm: instruction %d invalid rune range [%d,%d]", i, ins.A, ins.B)
			}
			if err := validateExternalVMTarget(i, ins.Alt, codeLen, "Alt"); err != nil {
				return err
			}
		case ExternalVMOpIfRuneClass:
			if ins.A < 0 || ins.A > int32(ExternalVMRuneClassNewline) {
				return fmt.Errorf("external vm: instruction %d invalid rune class %d", i, ins.A)
			}
			if err := validateExternalVMTarget(i, ins.Alt, codeLen, "Alt"); err != nil {
				return err
			}
		default:
			return fmt.Errorf("external vm: instruction %d unknown opcode %d", i, ins.Op)
		}
	}

	return nil
}

func validateExternalVMTarget(instrIndex int, target int32, codeLen int, operand string) error {
	if target < 0 || int(target) >= codeLen {
		return fmt.Errorf("external vm: instruction %d invalid %s target %d (code len %d)", instrIndex, operand, target, codeLen)
	}
	return nil
}

func defaultExternalVMMaxSteps(codeLen int) int {
	steps := codeLen * 16
	if steps < 64 {
		return 64
	}
	return steps
}

func vmPayload(payload any) (*externalVMPayload, bool) {
	p, ok := payload.(*externalVMPayload)
	if !ok || p == nil {
		return nil, false
	}
	return p, true
}

// Create allocates scanner payload (currently a single uint32 state slot).
func (s *ExternalVMScanner) Create() any {
	return &externalVMPayload{}
}

// Destroy releases scanner payload resources.
func (s *ExternalVMScanner) Destroy(payload any) {}

// Serialize writes payload state into buf.
func (s *ExternalVMScanner) Serialize(payload any, buf []byte) int {
	if len(buf) < 4 {
		return 0
	}
	state := uint32(0)
	if p, ok := vmPayload(payload); ok {
		state = p.State
	}
	binary.LittleEndian.PutUint32(buf[:4], state)
	return 4
}

// Deserialize restores payload state from buf.
func (s *ExternalVMScanner) Deserialize(payload any, buf []byte) {
	p, ok := vmPayload(payload)
	if !ok {
		return
	}
	if len(buf) < 4 {
		p.State = 0
		return
	}
	p.State = binary.LittleEndian.Uint32(buf[:4])
}

func matchesExternalVMRuneClass(r rune, class ExternalVMRuneClass) bool {
	switch class {
	case ExternalVMRuneClassWhitespace:
		return unicode.IsSpace(r)
	case ExternalVMRuneClassDigit:
		return unicode.IsDigit(r)
	case ExternalVMRuneClassLetter:
		return unicode.IsLetter(r)
	case ExternalVMRuneClassWord:
		return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
	case ExternalVMRuneClassNewline:
		return r == '\n'
	default:
		return false
	}
}

// Scan executes the scanner program against the current lexer position.
func (s *ExternalVMScanner) Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool {
	if s == nil || lexer == nil || len(s.program.Code) == 0 {
		return false
	}

	state := uint32(0)
	if p, ok := vmPayload(payload); ok {
		state = p.State
		defer func() {
			p.State = state
		}()
	}

	maxSteps := s.program.MaxSteps
	if maxSteps <= 0 {
		maxSteps = defaultExternalVMMaxSteps(len(s.program.Code))
	}

	pc := 0
	for steps := 0; steps < maxSteps; steps++ {
		if pc < 0 || pc >= len(s.program.Code) {
			return false
		}

		ins := s.program.Code[pc]
		switch ins.Op {
		case ExternalVMOpFail:
			return false
		case ExternalVMOpJump:
			pc = int(ins.A)
		case ExternalVMOpRequireValid:
			idx := int(ins.A)
			if idx < len(validSymbols) && validSymbols[idx] {
				pc++
			} else {
				pc = int(ins.Alt)
			}
		case ExternalVMOpRequireStateEq:
			if state == uint32(ins.A) {
				pc++
			} else {
				pc = int(ins.Alt)
			}
		case ExternalVMOpSetState:
			state = uint32(ins.A)
			pc++
		case ExternalVMOpIfRuneEq:
			if lexer.Lookahead() == rune(ins.A) {
				pc++
			} else {
				pc = int(ins.Alt)
			}
		case ExternalVMOpIfRuneInRange:
			r := lexer.Lookahead()
			if r >= rune(ins.A) && r <= rune(ins.B) {
				pc++
			} else {
				pc = int(ins.Alt)
			}
		case ExternalVMOpIfRuneClass:
			if matchesExternalVMRuneClass(lexer.Lookahead(), ExternalVMRuneClass(ins.A)) {
				pc++
			} else {
				pc = int(ins.Alt)
			}
		case ExternalVMOpAdvance:
			lexer.Advance(ins.A != 0)
			pc++
		case ExternalVMOpMarkEnd:
			lexer.MarkEnd()
			pc++
		case ExternalVMOpEmit:
			lexer.SetResultSymbol(Symbol(ins.A))
			return true
		default:
			return false
		}
	}

	// Step limit hit: treat as failed scan to avoid infinite loops.
	return false
}

func VMFail() ExternalVMInstr { return ExternalVMInstr{Op: ExternalVMOpFail} }

func VMJump(target int) ExternalVMInstr {
	return ExternalVMInstr{Op: ExternalVMOpJump, A: int32(target)}
}

func VMRequireValid(validSymbolIndex, alt int) ExternalVMInstr {
	return ExternalVMInstr{Op: ExternalVMOpRequireValid, A: int32(validSymbolIndex), Alt: int32(alt)}
}

func VMRequireStateEq(state uint32, alt int) ExternalVMInstr {
	return ExternalVMInstr{Op: ExternalVMOpRequireStateEq, A: int32(state), Alt: int32(alt)}
}

func VMSetState(state uint32) ExternalVMInstr {
	return ExternalVMInstr{Op: ExternalVMOpSetState, A: int32(state)}
}

func VMIfRuneEq(r rune, alt int) ExternalVMInstr {
	return ExternalVMInstr{Op: ExternalVMOpIfRuneEq, A: int32(r), Alt: int32(alt)}
}

func VMIfRuneInRange(start, end rune, alt int) ExternalVMInstr {
	return ExternalVMInstr{
		Op:  ExternalVMOpIfRuneInRange,
		A:   int32(start),
		B:   int32(end),
		Alt: int32(alt),
	}
}

func VMIfRuneClass(class ExternalVMRuneClass, alt int) ExternalVMInstr {
	return ExternalVMInstr{Op: ExternalVMOpIfRuneClass, A: int32(class), Alt: int32(alt)}
}

func VMAdvance(skip bool) ExternalVMInstr {
	if skip {
		return ExternalVMInstr{Op: ExternalVMOpAdvance, A: 1}
	}
	return ExternalVMInstr{Op: ExternalVMOpAdvance}
}

func VMMarkEnd() ExternalVMInstr { return ExternalVMInstr{Op: ExternalVMOpMarkEnd} }

func VMEmit(sym Symbol) ExternalVMInstr {
	return ExternalVMInstr{Op: ExternalVMOpEmit, A: int32(sym)}
}
