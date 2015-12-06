// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package interp

// Emulated functions that we cannot interpret because they are
// external or because they use "unsafe" or "reflect" operations.

import (
	"fmt"
	"math"
	"os"
	"reflect"
	"runtime"
	"sync"
	"syscall"
	"time"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types"
)

type externalFn func(fr *frame, args []Ivalue) Ivalue

// TODO(adonovan): fix: reflect.Value abstracts an lIvalue or an
// rIvalue; Set() causes mutations that can be observed via aliases.
// We have not captured that correctly here.

// Key strings are from Function.String().
var externals map[string]externalFn

func init() {
	// That little dot ۰ is an Arabic zero numeral (U+06F0), categories [Nd].
	externals = map[string]externalFn{
		"(*sync.Pool).Get":                 ext۰sync۰Pool۰Get,
		"(*sync.Pool).Put":                 ext۰sync۰Pool۰Put,
		"(reflect.Value).Bool":             ext۰reflect۰Value۰Bool,
		"(reflect.Value).CanAddr":          ext۰reflect۰Value۰CanAddr,
		"(reflect.Value).CanInterface":     ext۰reflect۰Value۰CanInterface,
		"(reflect.Value).Elem":             ext۰reflect۰Value۰Elem,
		"(reflect.Value).Field":            ext۰reflect۰Value۰Field,
		"(reflect.Value).Float":            ext۰reflect۰Value۰Float,
		"(reflect.Value).Index":            ext۰reflect۰Value۰Index,
		"(reflect.Value).Int":              ext۰reflect۰Value۰Int,
		"(reflect.Value).Interface":        ext۰reflect۰Value۰Interface,
		"(reflect.Value).IsNil":            ext۰reflect۰Value۰IsNil,
		"(reflect.Value).IsValid":          ext۰reflect۰Value۰IsValid,
		"(reflect.Value).Kind":             ext۰reflect۰Value۰Kind,
		"(reflect.Value).Len":              ext۰reflect۰Value۰Len,
		"(reflect.Value).MapIndex":         ext۰reflect۰Value۰MapIndex,
		"(reflect.Value).MapKeys":          ext۰reflect۰Value۰MapKeys,
		"(reflect.Value).NumField":         ext۰reflect۰Value۰NumField,
		"(reflect.Value).NumMethod":        ext۰reflect۰Value۰NumMethod,
		"(reflect.Value).Pointer":          ext۰reflect۰Value۰Pointer,
		"(reflect.Value).Set":              ext۰reflect۰Value۰Set,
		"(reflect.Value).String":           ext۰reflect۰Value۰String,
		"(reflect.Value).Type":             ext۰reflect۰Value۰Type,
		"(reflect.Value).Uint":             ext۰reflect۰Value۰Uint,
		"(reflect.error).Error":            ext۰reflect۰error۰Error,
		"(reflect.rtype).Bits":             ext۰reflect۰rtype۰Bits,
		"(reflect.rtype).Elem":             ext۰reflect۰rtype۰Elem,
		"(reflect.rtype).Field":            ext۰reflect۰rtype۰Field,
		"(reflect.rtype).In":               ext۰reflect۰rtype۰In,
		"(reflect.rtype).Kind":             ext۰reflect۰rtype۰Kind,
		"(reflect.rtype).NumField":         ext۰reflect۰rtype۰NumField,
		"(reflect.rtype).NumIn":            ext۰reflect۰rtype۰NumIn,
		"(reflect.rtype).NumMethod":        ext۰reflect۰rtype۰NumMethod,
		"(reflect.rtype).NumOut":           ext۰reflect۰rtype۰NumOut,
		"(reflect.rtype).Out":              ext۰reflect۰rtype۰Out,
		"(reflect.rtype).Size":             ext۰reflect۰rtype۰Size,
		"(reflect.rtype).String":           ext۰reflect۰rtype۰String,
		"bytes.Equal":                      ext۰bytes۰Equal,
		"bytes.Compare":                    ext۰bytes۰Compare,
		"bytes.IndexByte":                  ext۰bytes۰IndexByte,
		"hash/crc32.haveSSE42":             ext۰crc32۰haveSSE42,
		"math.Abs":                         ext۰math۰Abs,
		"math.Exp":                         ext۰math۰Exp,
		"math.Float32bits":                 ext۰math۰Float32bits,
		"math.Float32frombits":             ext۰math۰Float32frombits,
		"math.Float64bits":                 ext۰math۰Float64bits,
		"math.Float64frombits":             ext۰math۰Float64frombits,
		"math.Ldexp":                       ext۰math۰Ldexp,
		"math.Log":                         ext۰math۰Log,
		"math.Min":                         ext۰math۰Min,
		"math.Sqrt":                        ext۰math۰Sqrt, // Elliott
		"os.runtime_args":                  ext۰os۰runtime_args,
		"os.runtime_beforeExit":            ext۰os۰runtime_beforeExit,
		"reflect.New":                      ext۰reflect۰New,
		"reflect.SliceOf":                  ext۰reflect۰SliceOf,
		"reflect.TypeOf":                   ext۰reflect۰TypeOf,
		"reflect.ValueOf":                  ext۰reflect۰ValueOf,
		"reflect.Zero":                     ext۰reflect۰Zero,
		"reflect.init":                     ext۰reflect۰Init,
		"reflect.valueInterface":           ext۰reflect۰valueInterface,
		"runtime.Breakpoint":               ext۰runtime۰Breakpoint,
		"runtime.Caller":                   ext۰runtime۰Caller,
		"runtime.Callers":                  ext۰runtime۰Callers,
		"runtime.FuncForPC":                ext۰runtime۰FuncForPC,
		"runtime.GC":                       ext۰runtime۰GC,
		"runtime.GOMAXPROCS":               ext۰runtime۰GOMAXPROCS,
		"runtime.Goexit":                   ext۰runtime۰Goexit,
		"runtime.Gosched":                  ext۰runtime۰Gosched,
		"runtime.init":                     ext۰runtime۰init,
		"runtime.NumCPU":                   ext۰runtime۰NumCPU,
		"runtime.ReadMemStats":             ext۰runtime۰ReadMemStats,
		"runtime.SetFinalizer":             ext۰runtime۰SetFinalizer,
		"(*runtime.Func).Entry":            ext۰runtime۰Func۰Entry,
		"(*runtime.Func).FileLine":         ext۰runtime۰Func۰FileLine,
		"(*runtime.Func).Name":             ext۰runtime۰Func۰Name,
		"runtime.environ":                  ext۰runtime۰environ,
		"runtime.getgoroot":                ext۰runtime۰getgoroot,
		"strings.IndexByte":                ext۰strings۰IndexByte,
		"sync.runtime_Semacquire":          ext۰sync۰runtime_Semacquire,
		"sync.runtime_Semrelease":          ext۰sync۰runtime_Semrelease,
		"sync.runtime_Syncsemcheck":        ext۰sync۰runtime_Syncsemcheck,
		"sync.runtime_canSpin":             sync۰runtime۰canSpin, // Elliott
		"sync.runtime_doSpin":              sync۰runtime۰doSpin,  // Elliott
		"sync.runtime_registerPoolCleanup": ext۰sync۰runtime_registerPoolCleanup,
		"sync/atomic.AddInt32":             ext۰atomic۰AddInt32,
		"sync/atomic.AddUint32":            ext۰atomic۰AddUint32,
		"sync/atomic.AddUint64":            ext۰atomic۰AddUint64,
		"sync/atomic.AddInt64":             ext۰atomic۰AddInt64, // Elliott
		"sync/atomic.CompareAndSwapInt32":  ext۰atomic۰CompareAndSwapInt32,
		"sync/atomic.CompareAndSwapUint64": ext۰atomic۰CompareAndSwapUint64, // Elliott
		"sync/atomic.LoadInt32":            ext۰atomic۰LoadInt32,
		"sync/atomic.LoadInt64":            ext۰atomic۰LoadInt64, // Elliott
		"sync/atomic.LoadUint32":           ext۰atomic۰LoadUint32,
		"sync/atomic.LoadUint64":           ext۰atomic۰LoadUint64, // Elliott
		"sync/atomic.StoreInt32":           ext۰atomic۰StoreInt32,
		"sync/atomic.StoreUint32":          ext۰atomic۰StoreUint32,
		"syscall.Close":                    ext۰syscall۰Close,
		"syscall.Exit":                     ext۰syscall۰Exit,
		"syscall.Fstat":                    ext۰syscall۰Fstat,
		"syscall.Getpid":                   ext۰syscall۰Getpid,
		"syscall.Getwd":                    ext۰syscall۰Getwd,
		"syscall.Kill":                     ext۰syscall۰Kill,
		"syscall.Lstat":                    ext۰syscall۰Lstat,
		"syscall.Open":                     ext۰syscall۰Open,
		"syscall.ParseDirent":              ext۰syscall۰ParseDirent,
		"syscall.RawSyscall":               ext۰syscall۰RawSyscall,
		"syscall.Read":                     ext۰syscall۰Read,
		"syscall.ReadDirent":               ext۰syscall۰ReadDirent,
		"syscall.Stat":                     ext۰syscall۰Stat,
		"syscall.Write":                    ext۰syscall۰Write,
		"syscall.Pipe":                     ext۰syscall۰Pipe,
		"syscall.Syscall":                  ext۰syscall۰Syscall,
		"syscall.CloseOnExec":              ext۰syscall۰CloseOnExec,
		"syscall.runtime_envs":             ext۰runtime۰environ,
		"syscall.now":                      ext۰time۰now,
		"time.Sleep":                       ext۰time۰Sleep,
		"time.now":                         ext۰time۰now,
		"time.runtimeNano":                 ext۰time۰runtimeNano,
	}
}

// wrapError returns an interpreted 'error' interface Ivalue for err.
func wrapError(err error) Ivalue {
	if err == nil {
		return iface{}
	}
	return iface{t: errorType, v: err.Error()}
}

func sync۰runtime۰canSpin(fr *frame, args []Ivalue) Ivalue { //i int) bool { // Elliott
	return true
}
func sync۰runtime۰doSpin(fr *frame, args []Ivalue) Ivalue { //) { // Elliott
	runtime.Gosched()
	return nil
}

func ext۰sync۰Pool۰Get(fr *frame, args []Ivalue) Ivalue {
	Pool := fr.i.prog.ImportedPackage("sync").Type("Pool").Object()
	_, newIndex, _ := types.LookupFieldOrMethod(Pool.Type(), false, Pool.Pkg(), "New")

	if New := (*args[0].(*Ivalue)).(structure)[newIndex[0]]; New != nil {
		return call(fr.i, fr, 0, New, nil)
	}
	return nil
}

func ext۰sync۰Pool۰Put(fr *frame, args []Ivalue) Ivalue {
	return nil
}

func ext۰bytes۰Equal(fr *frame, args []Ivalue) Ivalue {
	// func Equal(a, b []byte) bool
	a := args[0].([]Ivalue)
	b := args[1].([]Ivalue)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func ext۰bytes۰IndexByte(fr *frame, args []Ivalue) Ivalue {
	// func IndexByte(s []byte, c byte) int
	s := args[0].([]Ivalue)
	c := args[1].(byte)
	for i, b := range s {
		if b.(byte) == c {
			return i
		}
	}
	return -1
}

// Compare returns an integer comparing two byte slices lexicographically.
// The result will be 0 if a==b, -1 if a < b, and +1 if a > b.
// A nil argument is equivalent to an empty slice.
func ext۰bytes۰Compare(fr *frame, args []Ivalue) Ivalue {
	// func Compare(a, b []byte) int {
	a := args[0].([]Ivalue)
	b := args[1].([]Ivalue)
	if a == nil {
		if b == nil {
			return 0
		}
		if len(b) == 0 {
			return 0
		}
		return -1
	}
	if b == nil {
		if len(a) == 0 {
			return 0
		}
		return 1
	}
	i := 0
	for (i < len(a)) && (i < len(b)) {
		if a[i].(byte) < b[i].(byte) {
			return -1
		}
		if a[i].(byte) > b[i].(byte) {
			return +1
		}
		i++
	}
	if len(a) == len(b) {
		return 0
	}
	if len(a) < len(b) {
		return -1
	}
	return +1
}

func ext۰crc32۰haveSSE42(fr *frame, args []Ivalue) Ivalue {
	return false
}

func ext۰math۰Float64frombits(fr *frame, args []Ivalue) Ivalue {
	return math.Float64frombits(args[0].(uint64))
}

func ext۰math۰Float64bits(fr *frame, args []Ivalue) Ivalue {
	return math.Float64bits(args[0].(float64))
}

func ext۰math۰Float32frombits(fr *frame, args []Ivalue) Ivalue {
	return math.Float32frombits(args[0].(uint32))
}

func ext۰math۰Abs(fr *frame, args []Ivalue) Ivalue {
	return math.Abs(args[0].(float64))
}

func ext۰math۰Exp(fr *frame, args []Ivalue) Ivalue {
	return math.Exp(args[0].(float64))
}

func ext۰math۰Float32bits(fr *frame, args []Ivalue) Ivalue {
	return math.Float32bits(args[0].(float32))
}

func ext۰math۰Min(fr *frame, args []Ivalue) Ivalue {
	return math.Min(args[0].(float64), args[1].(float64))
}

func ext۰math۰Ldexp(fr *frame, args []Ivalue) Ivalue {
	return math.Ldexp(args[0].(float64), args[1].(int))
}

func ext۰math۰Log(fr *frame, args []Ivalue) Ivalue {
	return math.Log(args[0].(float64))
}

func ext۰math۰Sqrt(fr *frame, args []Ivalue) Ivalue { // Elliott
	return math.Sqrt(args[0].(float64))
}

func ext۰os۰runtime_args(fr *frame, args []Ivalue) Ivalue {
	return fr.i.osArgs
}

func ext۰os۰runtime_beforeExit(fr *frame, args []Ivalue) Ivalue {
	return nil
}

func ext۰runtime۰Breakpoint(fr *frame, args []Ivalue) Ivalue {
	runtime.Breakpoint()
	return nil
}

func ext۰runtime۰Caller(fr *frame, args []Ivalue) Ivalue {
	// func Caller(skip int) (pc uintptr, file string, line int, ok bool)
	skip := 1 + args[0].(int)
	for i := 0; i < skip; i++ {
		if fr != nil {
			fr = fr.caller
		}
	}
	var pc uintptr
	var file string
	var line int
	var ok bool
	if fr != nil {
		fn := fr.fn
		// TODO(adonovan): use pc/posn of current instruction, not start of fn.
		pc = reflect.ValueOf(fn).Pointer() // uintptr(unsafe.Pointer(fn))
		posn := fn.Prog.Fset.Position(fn.Pos())
		file = posn.Filename
		line = posn.Line
		ok = true
	}
	return tuple{pc, file, line, ok}
}

func ext۰runtime۰Callers(fr *frame, args []Ivalue) Ivalue {
	// Callers(skip int, pc []uintptr) int
	skip := args[0].(int)
	pc := args[1].([]Ivalue)
	for i := 0; i < skip; i++ {
		if fr != nil {
			fr = fr.caller
		}
	}
	i := 0
	for fr != nil {
		pc[i] = reflect.ValueOf(fr.fn).Pointer() // uintptr(unsafe.Pointer(fr.fn))
		i++
		fr = fr.caller
	}
	return i
}

func ext۰runtime۰FuncForPC(fr *frame, args []Ivalue) Ivalue {
	// FuncForPC(pc uintptr) *Func
	pc := args[0].(uintptr)
	var fn *ssa.Function
	if pc != 0 {
		//fn = (*ssa.Function)(unsafe.Pointer(pc)) // indeed unsafe!
		fn = fr.i.runtimeFunc[pc]
	}
	var Func Ivalue
	Func = structure{fn} // a runtime.Func
	return &Func
}

func ext۰runtime۰environ(fr *frame, args []Ivalue) Ivalue {
	// This function also implements syscall.runtime_envs.
	return environ
}

func ext۰runtime۰getgoroot(fr *frame, args []Ivalue) Ivalue {
	return os.Getenv("GOROOT")
}

func ext۰strings۰IndexByte(fr *frame, args []Ivalue) Ivalue {
	// func IndexByte(s string, c byte) int
	s := args[0].(string)
	c := args[1].(byte)
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func ext۰sync۰runtime_Syncsemcheck(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): fix: implement.
	return nil
}

func ext۰sync۰runtime_registerPoolCleanup(fr *frame, args []Ivalue) Ivalue {
	return nil
}

func ext۰sync۰runtime_Semacquire(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): fix: implement.
	return nil
}

func ext۰sync۰runtime_Semrelease(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): fix: implement.
	return nil
}

func ext۰runtime۰GOMAXPROCS(fr *frame, args []Ivalue) Ivalue {
	// Ignore args[0]; don't let the interpreted program
	// set the interpreter's GOMAXPROCS!
	return runtime.GOMAXPROCS(0)
}

func ext۰runtime۰Goexit(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): don't kill the interpreter's main goroutine.
	runtime.Goexit()
	return nil
}

func ext۰runtime۰GC(fr *frame, args []Ivalue) Ivalue {
	runtime.GC()
	return nil
}

func ext۰runtime۰Gosched(fr *frame, args []Ivalue) Ivalue {
	runtime.Gosched()
	return nil
}

func ext۰runtime۰init(fr *frame, args []Ivalue) Ivalue {
	return nil
}

func ext۰runtime۰NumCPU(fr *frame, args []Ivalue) Ivalue {
	return runtime.NumCPU()
}

func ext۰runtime۰ReadMemStats(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): populate args[0].(Struct)
	return nil
}

func ext۰atomic۰LoadUint32(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): fix: not atomic!

	// Elliott
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()

	return (*args[0].(*Ivalue)).(uint32)
}

func ext۰atomic۰StoreUint32(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): fix: not atomic!

	// Elliott
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()

	*args[0].(*Ivalue) = args[1].(uint32)
	return nil
}

func ext۰atomic۰LoadInt32(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): fix: not atomic!

	// Elliott
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()

	return (*args[0].(*Ivalue)).(int32)
}

func ext۰atomic۰LoadInt64(fr *frame, args []Ivalue) Ivalue { // Elliott
	// TODO(adonovan): fix: not atomic!

	// Elliott
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()

	return (*args[0].(*Ivalue)).(int64)
}

func ext۰atomic۰StoreInt32(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): fix: not atomic!
	// Elliott
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()

	*args[0].(*Ivalue) = args[1].(int32)
	return nil
}

func ext۰atomic۰CompareAndSwapInt32(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): fix: not atomic!
	// Elliott
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()

	p := args[0].(*Ivalue)
	if (*p).(int32) == args[1].(int32) {
		*p = args[2].(int32)
		return true
	}
	return false
}

func ext۰atomic۰AddInt32(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): fix: not atomic!
	// Elliott
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()

	p := args[0].(*Ivalue)
	newv := (*p).(int32) + args[1].(int32)
	*p = newv
	return newv
}

func ext۰atomic۰AddUint32(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): fix: not atomic!
	// Elliott
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()

	p := args[0].(*Ivalue)
	newv := (*p).(uint32) + args[1].(uint32)
	*p = newv
	return newv
}

func ext۰atomic۰LoadUint64(fr *frame, args []Ivalue) Ivalue {
	//	func LoadUint64(addr *uint64) (val uint64)
	// Elliott
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()
	p := args[0].(*Ivalue)
	//fmt.Printf("DEBUG LoadUint64 %v:%T\n", *p, *p)
	if _, isArray := (*p).(array); isArray {
		p = &((*p).(array)[0])
	}
	if ui64, isUint64 := (*p).(uint64); isUint64 {
		return ui64
	}
	return uint64(0) // TODO correct ???
}

func ext۰atomic۰AddUint64(fr *frame, args []Ivalue) Ivalue {
	// TODO(adonovan): fix: not atomic!
	// Elliott
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()

	p := args[0].(*Ivalue)
	//fmt.Printf("DEBUG AddUint64 %v:%T %v:%T\n", *p, *p, args[1], args[1])
	if _, isArray := (*p).(array); isArray {
		p = &((*p).(array)[0])
		if _, isUint64 := (*p).(uint64); !isUint64 {
			*p = uint64(0) // TODO check this works...
		}
	}
	newv := (*p).(uint64) + args[1].(uint64)
	*p = newv
	return newv
}

func ext۰atomic۰CompareAndSwapUint64(fr *frame, args []Ivalue) Ivalue {
	// func CompareAndSwapUint64(addr *uint64, old, new uint64) (swapped bool)
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()
	p := args[0].(*Ivalue)
	//fmt.Printf("DEBUG CompareAndSwapUint64 %v:%T %v:%T %v:%T\n",
	//	*p, *p, args[1], args[1], args[2], args[2])
	if _, isArray := (*p).(array); isArray {
		//fmt.Println("DEBUG isArray")
		p = &((*p).(array)[0])
		if _, isUint64 := (*p).(uint64); !isUint64 {
			//fmt.Println("DEBUG !isUint64")
			*p = uint64(0) // TODO check this works...
		}
	}
	if (*p).(uint64) == args[1].(uint64) {
		*p = args[2]
		return true
	}
	return false
}

var atomic_mutex sync.Mutex // Elliott

func ext۰atomic۰AddInt64(fr *frame, args []Ivalue) Ivalue { // Elliott

	// Elliott
	atomic_mutex.Lock()
	defer atomic_mutex.Unlock()
	p := args[0].(*Ivalue)
	newv := (*p).(int64) + args[1].(int64)
	*p = newv
	return newv
}

func ext۰runtime۰SetFinalizer(fr *frame, args []Ivalue) Ivalue {
	return nil // ignore
}

// Pretend: type runtime.Func struct { entry *ssa.Function }

func ext۰runtime۰Func۰FileLine(fr *frame, args []Ivalue) Ivalue {
	// func (*runtime.Func) FileLine(uintptr) (string, int)
	f, _ := (*args[0].(*Ivalue)).(structure)[0].(*ssa.Function)
	pc := args[1].(uintptr)
	_ = pc
	if f != nil {
		// TODO(adonovan): use position of current instruction, not fn.
		posn := f.Prog.Fset.Position(f.Pos())
		return tuple{posn.Filename, posn.Line}
	}
	return tuple{"", 0}
}

func ext۰runtime۰Func۰Name(fr *frame, args []Ivalue) Ivalue {
	// func (*runtime.Func) Name() string
	f, _ := (*args[0].(*Ivalue)).(structure)[0].(*ssa.Function)
	if f != nil {
		return f.String()
	}
	return ""
}

func ext۰runtime۰Func۰Entry(fr *frame, args []Ivalue) Ivalue {
	// func (*runtime.Func) Entry() uintptr
	f, _ := (*args[0].(*Ivalue)).(structure)[0].(*ssa.Function)
	return reflect.ValueOf(f).Pointer() // uintptr(unsafe.Pointer(f))
}

func ext۰time۰now(fr *frame, args []Ivalue) Ivalue {
	nano := time.Now().UnixNano()
	return tuple{int64(nano / 1e9), int32(nano % 1e9)}
}

func ext۰time۰runtimeNano(fr *frame, args []Ivalue) Ivalue {
	return time.Now().UnixNano()
}

var timers = make(map[*Ivalue]time.Timer)

func ext۰time۰startTimer(fr *frame, args []Ivalue) Ivalue {
	// TODO
	fmt.Println("DEBUG time.startTimer TODO")
	return nil
}
func ext۰time۰stopTimer(fr *frame, args []Ivalue) Ivalue {
	// TODO
	fmt.Println("DEBUG time.stopTimer TODO")
	return false
}
func ext۰time۰Sleep(fr *frame, args []Ivalue) Ivalue {
	time.Sleep(time.Duration(args[0].(int64)))
	return nil
}

func ext۰syscall۰Exit(fr *frame, args []Ivalue) Ivalue {
	panic(exitPanic(args[0].(int)))
}

func ext۰syscall۰Getwd(fr *frame, args []Ivalue) Ivalue {
	s, err := syscall.Getwd()
	return tuple{s, wrapError(err)}
}

func ext۰syscall۰Getpid(fr *frame, args []Ivalue) Ivalue {
	return syscall.Getpid()
}

// IvalueToBytes converts an Ivalue containing the interpreter
// internal representation of "[]byte" into a []byte.
func IvalueToBytes(v Ivalue) []byte {
	in := v.([]Ivalue)
	b := make([]byte, len(in))
	for i := range in {
		b[i] = in[i].(byte)
	}
	return b
}
