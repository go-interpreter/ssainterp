// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package interp defines an interpreter for the SSA
// representation of Go programs.
//
// This interpreter is provided as an adjunct for testing the SSA
// construction algorithm.  Its purpose is to provide a minimal
// metacircular implementation of the dynamic semantics of each SSA
// instruction.  It is not, and will never be, a production-quality Go
// interpreter.
//
// The following is a partial list of Go features that are currently
// unsupported or incomplete in the interpreter.
//
// * Unsafe operations, including all uses of unsafe.Pointer, are
// impossible to support given the "boxed" Ivalue representation we
// have chosen.
//
// * The reflect package is only partially implemented.
//
// * "sync/atomic" operations are not currently atomic due to the
// "boxed" Ivalue representation: it is not possible to read, modify
// and write an interface Ivalue atomically.  As a consequence, Mutexes
// are currently broken.  TODO(adonovan): provide a metacircular
// implementation of Mutex avoiding the broken atomic primitives.
//
// * recover is only partially implemented.  Also, the interpreter
// makes no attempt to distinguish target panics from interpreter
// crashes.
//
// * map iteration is asymptotically inefficient.
//
// * the sizes of the int, uint and uintptr types in the target
// program are assumed to be the same as those of the interpreter
// itself.
//
// * all Ivalues occupy space, even those of types defined by the spec
// to have zero size, e.g. struct{}.  This can cause asymptotic
// performance degradation.
//
// * os.Exit is implemented using panic, causing deferred functions to
// run.
package interp

// was: import "golang.org/x/tools/go/ssa/interp"

import (
	"fmt"
	"go/token"
	"os"
	"reflect"
	"runtime"
	"sync"
	"bytes"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types"
)

type continuation int

const (
	kNext continuation = iota
	kReturn
	kJump
)

// Mode is a bitmask of options affecting the interpreter.
type Mode uint

const (
	DisableRecover Mode = 1 << iota // Disable recover() in target programs; show interpreter crash instead.
	EnableTracing                   // Print a trace of all instructions as they are interpreted.
)

type methodSet map[string]*ssa.Function

// State shared between all interpreted goroutines.
type interpreter struct {
	osArgs             []Ivalue              // the Ivalue of os.Args
	prog               *ssa.Program          // the SSA program
	globals            map[ssa.Value]*Ivalue // addresses of global variables (immutable)
	mode               Mode                  // interpreter options
	reflectPackage     *ssa.Package          // the fake reflect package
	errorMethods       methodSet             // the method set of reflect.error, which implements the error interface.
	rtypeMethods       methodSet             // the method set of rtype, which implements the reflect.Type interface.
	runtimeErrorString types.Type            // the runtime.errorString type
	sizes              types.Sizes           // the effective type-sizing function

	externals *Externals // per-interpreter externals (immutable)
	/*
		constants        map[*ssa.Const]Ivalue // constant cache, set during preprocess()
		constantsRWMutex sync.RWMutex
		zeroes           typeutil.Map // zero value cache, early tests show no benefit, TODO retest
		zeroesRWMutex    sync.RWMutex
	*/
	functions        map[*ssa.Function]functionInfo
	functionsRWMutex sync.RWMutex
	panicSource      *frame

	runtimeFunc map[uintptr]*ssa.Function // required to avoid using the unsafe package

// CapturedOutput is non-nil, all writes by the interpreted program
// to file descriptors 1 and 2 will also be written to CapturedOutput.
//
// (The $GOROOT/test system requires that the test be considered a
// failure if "BUG" appears in the combined stdout/stderr output, even
// if it exits zero.  This is a global variable shared by all
// interpreters in the same process.)
//
CapturedOutput *bytes.Buffer
capturedOutputMu sync.Mutex

}

type deferred struct {
	fn    Ivalue
	args  []Ivalue
	instr *ssa.Defer
	tail  *deferred
}

type frame struct {
	i                *interpreter
	caller           *frame
	fn               *ssa.Function
	block, prevBlock *ssa.BasicBlock
	iNum             int // the latest instruction number in the block
	//env              map[ssa.Value]Ivalue // dynamic Ivalues of SSA variables
	env       []Ivalue // dynamic Ivalues of SSA variables
	locals    []Ivalue
	defers    *deferred
	result    Ivalue
	panicking bool
	panic     interface{}
}

func (fr *frame) get(key ssa.Value) Ivalue {
	switch key := key.(type) {
	case nil:
		// Hack; simplifies handling of optional attributes
		// such as ssa.Slice.{Low,High}.
		return nil
	case *ssa.Function, *ssa.Builtin:
		return key
	case *ssa.Const:
		return fr.i.constIvalue(key)
	case *ssa.Global:
		if r, ok := fr.i.globals[key]; ok {
			return r
		}
	}
	return fr.env[envEnt(fr, key)]
	/*
		if r, ok := fr.env[key]; ok {
			return r
		}
		panic(fmt.Sprintf("get: no Ivalue for %T: %v", key, key.Name()))
	*/

}

// runDefer runs a deferred call d.
// It always returns normally, but may set or clear fr.panic.
//
func (fr *frame) runDefer(d *deferred) {
	if fr.i.mode&EnableTracing != 0 {
		fmt.Fprintf(os.Stderr, "%s: invoking deferred function call\n",
			fr.i.prog.Fset.Position(d.instr.Pos()))
	}
	var ok bool
	defer func() {
		if !ok {
			// Deferred call created a new state of panic.
			fr.panicking = true
			fr.panic = recover()
			if fr.i.panicSource == nil {
				fr = fr.i.panicSource
			}
		}
	}()
	call(fr.i, fr, d.instr.Pos(), d.fn, d.args)
	ok = true
}

// runDefers executes fr's deferred function calls in LIFO order.
//
// On entry, fr.panicking indicates a state of panic; if
// true, fr.panic contains the panic Ivalue.
//
// On completion, if a deferred call started a panic, or if no
// deferred call recovered from a previous state of panic, then
// runDefers itself panics after the last deferred call has run.
//
// If there was no initial state of panic, or it was recovered from,
// runDefers returns normally.
//
func (fr *frame) runDefers() {
	for d := fr.defers; d != nil; d = d.tail {
		fr.runDefer(d)
	}
	fr.defers = nil
	if fr.panicking {
		if fr.i.panicSource == nil {
			fr.i.panicSource = fr
		}
		panic(fr.panic) // new panic, or still panicking
	}
}

// lookupMethod returns the method set for type typ, which may be one
// of the interpreter's fake types.
func lookupMethod(i *interpreter, typ types.Type, meth *types.Func) *ssa.Function {
	switch typ {
	case rtypeType:
		return i.rtypeMethods[meth.Id()]
	case errorType:
		return i.errorMethods[meth.Id()]
	}
	return i.prog.LookupMethod(typ, meth.Pkg(), meth.Name())
}

// visitInstr interprets a single ssa.Instruction within the activation
// record frame.  It returns a continuation Ivalue indicating where to
// read the next instruction from.
func visitInstr(fr *frame, instr ssa.Instruction) continuation {

	if debugStats {
		debugMapMutex.Lock()
		debugMap[fmt.Sprintf("%T", instr)]++
		debugMapMutex.Unlock()
	}

	switch instr := instr.(type) {
	case *ssa.DebugRef:
		// no-op

	case *ssa.UnOp:
		fr.env[envEnt(fr, instr)] = fr.i.unop(instr, fr.get(instr.X))

	case *ssa.BinOp:
		fr.env[envEnt(fr, instr)] = binop(instr.Op, instr.X.Type(), fr.get(instr.X), fr.get(instr.Y))

	case *ssa.Call:
		fn, args := prepareCall(fr, &instr.Call)
		fr.env[envEnt(fr, instr)] = call(fr.i, fr, instr.Pos(), fn, args)

	case *ssa.ChangeInterface:
		fr.env[envEnt(fr, instr)] = fr.get(instr.X)

	case *ssa.ChangeType:
		fr.env[envEnt(fr, instr)] = fr.get(instr.X) // (can't fail)

	case *ssa.Convert:
		fr.env[envEnt(fr, instr)] = fr.i.conv(instr.Type(), instr.X.Type(), fr.get(instr.X))

	case *ssa.MakeInterface:
		fr.env[envEnt(fr, instr)] = iface{t: instr.X.Type(), v: fr.get(instr.X)}

	case *ssa.Extract:
		fr.env[envEnt(fr, instr)] = fr.get(instr.Tuple).(tuple)[instr.Index]

	case *ssa.Slice:
		fr.env[envEnt(fr, instr)] = slice(fr.get(instr.X), fr.get(instr.Low), fr.get(instr.High), fr.get(instr.Max))

	case *ssa.Return:
		switch len(instr.Results) {
		case 0:
		case 1:
			fr.result = fr.get(instr.Results[0])
		default:
			var res []Ivalue
			for _, r := range instr.Results {
				res = append(res, fr.get(r))
			}
			fr.result = tuple(res)
		}
		fr.block = nil
		return kReturn

	case *ssa.RunDefers:
		fr.runDefers()

	case *ssa.Panic:
		if fr.i.panicSource == nil {
			fr.i.panicSource = fr
		}
		panic(targetPanic{fr.get(instr.X)})

	case *ssa.Send:
		fr.get(instr.Chan).(chan Ivalue) <- fr.get(instr.X)

	case *ssa.Store:
		store(deref(instr.Addr.Type()), fr.get(instr.Addr).(*Ivalue), fr.get(instr.Val))

	case *ssa.If:
		succ := 1
		if fr.get(instr.Cond).(bool) {
			succ = 0
		}
		fr.prevBlock, fr.block = fr.block, fr.block.Succs[succ]
		return kJump

	case *ssa.Jump:
		fr.prevBlock, fr.block = fr.block, fr.block.Succs[0]
		return kJump

	case *ssa.Defer:
		fn, args := prepareCall(fr, &instr.Call)
		fr.defers = &deferred{
			fn:    fn,
			args:  args,
			instr: instr,
			tail:  fr.defers,
		}

	case *ssa.Go:
		fn, args := prepareCall(fr, &instr.Call)
		go func() {
			defer func() {
				fr.i.userStackIfPanic()
			}()
			call(fr.i, nil, instr.Pos(), fn, args)
		}()

	case *ssa.MakeChan:
		fr.env[envEnt(fr, instr)] = make(chan Ivalue, asInt(fr.get(instr.Size)))

	case *ssa.Alloc:
		var addr *Ivalue
		if instr.Heap {
			// new
			addr = new(Ivalue)
			fr.env[envEnt(fr, instr)] = addr
		} else {
			// local
			addr = fr.env[envEnt(fr, instr)].(*Ivalue)
		}
		*addr = fr.i.zero(deref(instr.Type()))

	case *ssa.MakeSlice:
		slice := make([]Ivalue, asInt(fr.get(instr.Cap)))
		tElt := instr.Type().Underlying().(*types.Slice).Elem()
		for i := range slice {
			slice[i] = fr.i.zero(tElt)
		}
		fr.env[envEnt(fr, instr)] = slice[:asInt(fr.get(instr.Len))]

	case *ssa.MakeMap:
		reserve := 0
		if instr.Reserve != nil {
			reserve = asInt(fr.get(instr.Reserve))
		}
		fr.env[envEnt(fr, instr)] = makeMap(instr.Type().Underlying().(*types.Map).Key(), reserve)

	case *ssa.Range:
		fr.env[envEnt(fr, instr)] = rangeIter(fr.get(instr.X), instr.X.Type())

	case *ssa.Next:
		fr.env[envEnt(fr, instr)] = fr.get(instr.Iter).(iter).next()

	case *ssa.FieldAddr:
		x := fr.get(instr.X)
		// FIXME wrong!  &global.f must not change if we do *global = zero!
		fr.env[envEnt(fr, instr)] = &(*x.(*Ivalue)).(structure)[instr.Field]

	case *ssa.Field:
		fr.env[envEnt(fr, instr)] = fr.get(instr.X).(structure)[instr.Field]

	case *ssa.IndexAddr:
		x := fr.get(instr.X)
		idx := fr.get(instr.Index)
		switch x := x.(type) {
		case []Ivalue:
			fr.env[envEnt(fr, instr)] = &x[asInt(idx)]
		case *Ivalue: // *array
			fr.env[envEnt(fr, instr)] = &(*x).(array)[asInt(idx)]
		default:
			panic(fmt.Sprintf("unexpected x type in IndexAddr: %T", x))
		}

	case *ssa.Index:
		fr.env[envEnt(fr, instr)] = fr.get(instr.X).(array)[asInt(fr.get(instr.Index))]

	case *ssa.Lookup:
		fr.env[envEnt(fr, instr)] = fr.i.lookup(instr, fr.get(instr.X), fr.get(instr.Index))

	case *ssa.MapUpdate:
		m := fr.get(instr.Map)
		key := fr.get(instr.Key)
		v := fr.get(instr.Value)
		switch m := m.(type) {
		case map[Ivalue]Ivalue:
			m[key] = v
		case *hashmap:
			m.insert(key.(hashable), v)
		default:
			panic(fmt.Sprintf("illegal map type: %T", m))
		}

	case *ssa.TypeAssert:
		fr.env[envEnt(fr, instr)] = typeAssert(fr.i, instr, fr.get(instr.X).(iface))

	case *ssa.MakeClosure:
		var bindings []Ivalue
		for _, binding := range instr.Bindings {
			bindings = append(bindings, fr.get(binding))
		}
		fr.env[envEnt(fr, instr)] = &closure{instr.Fn.(*ssa.Function), bindings}

	case *ssa.Phi:
		for i, pred := range instr.Block().Preds {
			if fr.prevBlock == pred {
				fr.env[envEnt(fr, instr)] = fr.get(instr.Edges[i])
				break
			}
		}

	case *ssa.Select:
		var cases []reflect.SelectCase
		if !instr.Blocking {
			cases = append(cases, reflect.SelectCase{
				Dir: reflect.SelectDefault,
			})
		}
		for _, state := range instr.States {
			var dir reflect.SelectDir
			if state.Dir == types.RecvOnly {
				dir = reflect.SelectRecv
			} else {
				dir = reflect.SelectSend
			}
			var send reflect.Value
			if state.Send != nil {
				send = reflect.ValueOf(fr.get(state.Send))
			}
			cases = append(cases, reflect.SelectCase{
				Dir:  dir,
				Chan: reflect.ValueOf(fr.get(state.Chan)),
				Send: send,
			})
		}
		chosen, recv, recvOk := reflect.Select(cases)
		if !instr.Blocking {
			chosen-- // default case should have index -1.
		}
		r := tuple{chosen, recvOk}
		for i, st := range instr.States {
			if st.Dir == types.RecvOnly {
				var v Ivalue
				if i == chosen && recvOk {
					// No need to copy since send makes an unaliased copy.
					v = recv.Interface().(Ivalue)
				} else {
					v = fr.i.zero(st.Chan.Type().Underlying().(*types.Chan).Elem())
				}
				r = append(r, v)
			}
		}
		fr.env[envEnt(fr, instr)] = r

	default:
		panic(fmt.Sprintf("unexpected instruction: %T", instr))
	}

	// if val, ok := instr.(ssa.Value); ok {
	// 	fmt.Println(toString(fr.env[val])) // debugging
	// }

	return kNext
}

// prepareCall determines the function Ivalue and argument Ivalues for a
// function call in a Call, Go or Defer instruction, performing
// interface method lookup if needed.
//
func prepareCall(fr *frame, call *ssa.CallCommon) (fn Ivalue, args []Ivalue) {
	v := fr.get(call.Value)
	if call.Method == nil {
		// Function call.
		fn = v
	} else {
		// Interface method invocation.
		recv := v.(iface)
		if recv.t == nil {
			panic("method invoked on nil interface")
		}
		if f := lookupMethod(fr.i, recv.t, call.Method); f == nil {
			// Unreachable in well-typed programs.
			panic(fmt.Sprintf("method set for dynamic type %v does not contain %s", recv.t, call.Method))
		} else {
			fn = f
		}
		args = append(args, recv.v)
	}
	for _, arg := range call.Args {
		args = append(args, fr.get(arg))
	}
	return
}

// call interprets a call to a function (function, builtin or closure)
// fn with arguments args, returning its result.
// callpos is the position of the callsite.
//
func call(i *interpreter, caller *frame, callpos token.Pos, fn Ivalue, args []Ivalue) Ivalue {
	switch fn := fn.(type) {
	case *ssa.Function:
		if fn == nil {
			panic("call of nil function") // nil of func type
		}
		return callSSA(i, caller, callpos, fn, args, nil)
	case *closure:
		return callSSA(i, caller, callpos, fn.Fn, args, fn.Env)
	case *ssa.Builtin:
		return callBuiltin(caller, callpos, fn, args)
	}
	panic(fmt.Sprintf("cannot call %T", fn))
}

func loc(fset *token.FileSet, pos token.Pos) string {
	if pos == token.NoPos {
		return ""
	}
	return " at " + fset.Position(pos).String()
}

// callSSA interprets a call to function fn with arguments args,
// and lexical environment env, returning its result.
// callpos is the position of the callsite.
//
func callSSA(i *interpreter, caller *frame, callpos token.Pos, fn *ssa.Function, args []Ivalue, env []Ivalue) Ivalue {
	if i.mode&EnableTracing != 0 {
		fset := fn.Prog.Fset
		// TODO(adonovan): fix: loc() lies for external functions.
		fmt.Fprintf(os.Stderr, "Entering %s%s.\n", fn, loc(fset, fn.Pos()))
		suffix := ""
		if caller != nil {
			suffix = ", resuming " + caller.fn.String() + loc(fset, callpos)
		}
		defer fmt.Fprintf(os.Stderr, "Leaving %s%s.\n", fn, suffix)
	}
	fr := &frame{
		i:      i,
		caller: caller, // for panic/recover
		fn:     fn,
	}
	fr.getFnInf() // pre-process the function if required

	if fn.Parent() == nil {
		name := fn.String()
		if ext := i.externals.funcs[name]; ext != nil {
			if i.mode&EnableTracing != 0 {
				fmt.Fprintln(os.Stderr, "\t(user-defined external)")
			}
			return ext(fr, args)
		}
		if ext := externals[name]; ext != nil {
			if i.mode&EnableTracing != 0 {
				fmt.Fprintln(os.Stderr, "\t(interpreter external)")
			}
			return ext(fr, args)
		}
		if fn.Blocks == nil {
			fr.i.panicSource = fr
			panic("no code for function: " + name)
		}
	}
	fr.env = make([]Ivalue, len(i.functions[fr.fn].envEntries)) // make(map[ssa.Value]Ivalue)
	fr.block = fn.Blocks[0]
	fr.locals = make([]Ivalue, len(fn.Locals))
	for i, l := range fn.Locals { // allocate space for locals (used in alloc)
		fr.locals[i] = fr.i.zero(deref(l.Type()))
		fr.env[envEnt(fr, l)] = &fr.locals[i]
	}
	for i, p := range fn.Params {
		fr.env[envEnt(fr, p)] = args[i]
	}
	for i, fv := range fn.FreeVars {
		fr.env[envEnt(fr, fv)] = env[i]
	}
	for fr.block != nil {
		runFrame(fr)
	}
	// Destroy the locals to avoid accidental use after return.
	for i := range fn.Locals {
		fr.locals[i] = bad{}
	}
	return fr.result
}

// runFrame executes SSA instructions starting at fr.block and
// continuing until a return, a panic, or a recovered panic.
//
// After a panic, runFrame panics.
//
// After a normal return, fr.result contains the result of the call
// and fr.block is nil.
//
// A recovered panic in a function without named return parameters
// (NRPs) becomes a normal return of the zero Ivalue of the function's
// result type.
//
// After a recovered panic in a function with NRPs, fr.result is
// undefined and fr.block contains the block at which to resume
// control.
//
func runFrame(fr *frame) {
	defer func() {
		if fr.block == nil {
			return // normal return
		}
		if fr.i.mode&DisableRecover != 0 {
			return // let interpreter crash
		}
		fr.panicking = true
		fr.panic = recover()
		if fr.i.panicSource == nil {
			fr.i.panicSource = fr
		}
		if fr.i.mode&EnableTracing != 0 {
			fmt.Fprintf(os.Stderr, "Panicking: %T %v.\n", fr.panic, fr.panic)
		}
		fr.runDefers()
		fr.block = fr.fn.Recover
	}()

	fnInf := fr.getFnInf()

	fnInstrs := fnInf.instrs
	blkInstrs := fnInstrs[fr.block.Index]
	trace := fr.i.mode&EnableTracing != 0
	var instr ssa.Instruction
	for {
		if trace {
			fmt.Fprintf(os.Stderr, ".%s:\n", fr.block)
		}
	block:
		for fr.iNum, instr = range fr.block.Instrs {
			if trace {
				if v, ok := instr.(ssa.Value); ok {
					fmt.Fprintln(os.Stderr, "\t", v.Name(), "=", instr)
				} else {
					fmt.Fprintln(os.Stderr, "\t", instr)
				}
			}
			if fun := blkInstrs[fr.iNum]; fun != nil {
				fun(fr)
			} else {
				switch visitInstr(fr, instr) {
				case kReturn:
					return
				case kNext:
					// no-op
				case kJump:
					blkInstrs = fnInstrs[fr.block.Index]
					break block
				}
			}
		}
	}
}

// doRecover implements the recover() built-in.
func doRecover(caller *frame) Ivalue {
	// recover() must be exactly one level beneath the deferred
	// function (two levels beneath the panicking function) to
	// have any effect.  Thus we ignore both "defer recover()" and
	// "defer f() -> g() -> recover()".
	if caller.i.mode&DisableRecover == 0 &&
		caller != nil && !caller.panicking &&
		caller.caller != nil && caller.caller.panicking {
		caller.caller.panicking = false
		caller.i.panicSource = caller.caller // Elliott added
		p := caller.caller.panic
		caller.caller.panic = nil
		switch p := p.(type) {
		case targetPanic:
			// The target program explicitly called panic().
			return p.v
		case runtime.Error:
			// The interpreter encountered a runtime error.
			return iface{caller.i.runtimeErrorString, p.Error()}
		case string:
			// The interpreter explicitly called panic().
			return iface{caller.i.runtimeErrorString, p}
		default:
			panic(fmt.Sprintf("unexpected panic type %T in target call to recover()", p))
		}
	}
	return iface{}
}

// setGlobal sets the Ivalue of a system-initialized global variable.
func setGlobal(i *interpreter, pkg *ssa.Package, name string, v Ivalue) {
	if g, ok := i.globals[pkg.Var(name)]; ok {
		*g = v
		return
	}
	panic("no global variable: " + pkg.Pkg.Path() + "." + name)
}

var environ []Ivalue

func init() {
	for _, s := range os.Environ() {
		environ = append(environ, s)
	}
	environ = append(environ, "GOSSAINTERP=1")
	environ = append(environ, "GOARCH="+runtime.GOARCH)
}

// deleteBodies delete the bodies of all standalone functions except the
// specified ones.  A missing intrinsic leads to a clear runtime error.
func deleteBodies(pkg *ssa.Package, except ...string) {
	keep := make(map[string]bool)
	for _, e := range except {
		keep[e] = true
	}
	for _, mem := range pkg.Members {
		if fn, ok := mem.(*ssa.Function); ok && !keep[fn.Name()] {
			fn.Blocks = nil
		}
	}
}

// Context provides the execution context for subsequent calls
type Context struct {
	Interp *interpreter
}

// Interpret interprets the Go program whose main package is mainpkg.
// mode specifies various interpreter options.  filename and args are
// the initial Ivalues of os.Args for the target program.  sizes is the
// effective type-sizing function for this program.
//
// Interpret returns the exit code of the program: 2 for panic (like
// gc does), or the argument to os.Exit for normal termination.
//
// The SSA program must include the "runtime" package.
//
func Interpret(mainpkg *ssa.Package, mode Mode, sizes types.Sizes, filename string, args []string, ext *Externals, output *bytes.Buffer) (context *Context, exitCode int) {
	if ext == nil {
		ext = NewExternals()
	}
	i := &interpreter{
		prog:    mainpkg.Prog,
		globals: make(map[ssa.Value]*Ivalue),
		mode:    mode,
		sizes:   sizes,

		//constants: make(map[*ssa.Const]Ivalue),
		externals:   ext,
		functions:   make(map[*ssa.Function]functionInfo),
		runtimeFunc: make(map[uintptr]*ssa.Function),

		CapturedOutput: output,
	}
	i.preprocess()
	context = &Context{
		Interp: i,
	}
	runtimePkg := i.prog.ImportedPackage("runtime")
	if runtimePkg == nil {
		panic("ssa.Program doesn't include runtime package")
	}
	i.runtimeErrorString = runtimePkg.Type("errorString").Object().Type()

	initReflect(i)

	i.osArgs = append(i.osArgs, filename)
	for _, arg := range args {
		i.osArgs = append(i.osArgs, arg)
	}

	for _, pkg := range i.prog.AllPackages() {
		// Initialize global storage.
		for _, m := range pkg.Members {
			switch v := m.(type) {
			case *ssa.Global:
				//if gp, ok := i.externals.globVals[v]; ok {
				//	i.globals[v] = gp.addr
				//} else {
				cell := i.zero(deref(v.Type()))
				i.globals[v] = &cell
				//}
			}
		}

		// Ad-hoc initialization for magic system variables.
		switch pkg.Pkg.Path() {
		case "syscall":
			setGlobal(i, pkg, "envs", environ)

		case "reflect":
			deleteBodies(pkg, "DeepEqual", "deepValueEqual")

		case "runtime":
			sz := sizes.Sizeof(pkg.Pkg.Scope().Lookup("MemStats").Type())
			setGlobal(i, pkg, "sizeof_C_MStats", uintptr(sz))
			deleteBodies(pkg, "GOROOT", "gogetenv")
		}
	}

	// Top-level error handler.
	exitCode = 2
	defer func() {
		if exitCode != 2 || i.mode&DisableRecover != 0 {
			return
		}
		rec := recover()
		switch p:=rec.(type) {
		case exitPanic:
			exitCode = int(p)
			return
		case targetPanic:
			fmt.Fprintln(os.Stderr, "panic:", toString(p.v))
		case runtime.Error:
			fmt.Fprintln(os.Stderr, "panic:", p.Error())
		case string:
			fmt.Fprintln(os.Stderr, "panic:", p)
		default:
			fmt.Fprintf(os.Stderr, "panic: unexpected type: %T: %v\n", p, p)
		}

		i.userStackIfPanic()

		fmt.Fprintln(os.Stderr, "INTERPRETER STACK:")
		buf := make([]byte, 0x10000)
		runtime.Stack(buf, false)
		fmt.Fprintln(os.Stderr, string(buf))

		panic(rec) // to report the panic back
	}()

	// Run!
	call(i, nil, token.NoPos, mainpkg.Func("init"), nil)
	if mainFn := mainpkg.Func("main"); mainFn != nil {
		call(i, nil, token.NoPos, mainFn, nil)
		exitCode = 0
	} else {
		fmt.Fprintln(os.Stderr, "No main function.")
		exitCode = 1
	}
	return
}

func (i *interpreter) userStackIfPanic() {
	if i.panicSource != nil {
		fmt.Fprintln(os.Stderr, "USER STACK:")
		fr := i.panicSource
		for fr != nil {
			inst := fr.block.Instrs[fr.iNum].(ssa.Instruction)
			where := "-"
			for ii := fr.iNum; ii >= 0 && where == "-"; ii-- {
				where =
					fr.i.prog.Fset.Position(fr.block.Instrs[ii].(ssa.Instruction).Pos()).String()
			}
			fmt.Fprintln(os.Stderr, fr.fn.String(), where, inst.String())
			fr = fr.caller
		}
	}
}

// Call a function in the interpreter's execution context
func (cont *Context) Call(name string, args []Ivalue) Ivalue {
	// TODO implement a faster way than itterating over the packages/members
	fr := &frame{i: cont.Interp}
	pkgs := cont.Interp.prog.AllPackages()
	for _, pkg := range pkgs {
		for _, mem := range pkg.Members {
			targetFn, isFn := mem.(*ssa.Function)
			if isFn {
				if targetFn.String() == name {
					return call(cont.Interp, fr, token.NoPos, targetFn, args)
				}
			}
		}
	}
	panic("No interpreter function: " + name)
}

// deref returns a pointer's element type; otherwise it returns typ.
// TODO(adonovan): Import from ssa?
func deref(typ types.Type) types.Type {
	if p, ok := typ.Underlying().(*types.Pointer); ok {
		return p.Elem()
	}
	return typ
}
