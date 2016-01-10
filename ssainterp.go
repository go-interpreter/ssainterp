// Parts of this code are:
// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssainterp

import (
	"bytes"
	"fmt"
	"go/build"

	"github.com/go-interpreter/ssainterp/interp"
	//"./interp"

	"go/types"

	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Func is the signature of a normal go function
// that can be called from the interpreter.
// TODO: currently the parameters can only reliably contain simple values.
type Func func([]interp.Ivalue) interp.Ivalue

// ExtFunc defines an external go function callable from the interpreter.
type ExtFunc struct {
	Nam string // e.g. "main.Foo"
	Fun Func
}

// Interpreter defines the datastructure to run an interpreter.
type Interpreter struct {
	Context *interp.Context
	Panic   interface{}
	MainPkg *ssa.Package
	exts    *interp.Externals
}

// Run the interpreter, given some code.
func Run(code string, extFns []ExtFunc, args []string, output *bytes.Buffer) (interp *Interpreter, exitCode int, error error) {
	ssai := new(Interpreter)
	exitCode, err := ssai.run(code, extFns, args, output)
	if ssai.Panic != nil {
		return nil, -1, fmt.Errorf("panic in Run: %v", ssai.Panic)
	}
	return ssai, exitCode, err
}

func (ssai *Interpreter) run(code string, extFns []ExtFunc, args []string, output *bytes.Buffer) (int, error) {

	defer func() {
		if r := recover(); r != nil {
			ssai.Panic = r
		}
	}()

	conf := loader.Config{
		Build: &build.Default,
	}

	// Parse the input file.
	file, err := conf.ParseFile("main.go", code)
	if err != nil {
		return -1, err // parse error
	}

	// Create single-file main package.
	conf.CreateFromFiles("main", file)
	conf.Import("runtime") // always need this for ssa/interp

	// Load the main package and its dependencies.
	iprog, err := conf.Load()
	if err != nil {
		return -1, err // type error in some package
	}

	// Create SSA-form program representation.
	prog := ssautil.CreateProgram(iprog, ssa.SanityCheckFunctions)

	var mainPkg *ssa.Package
	for _, pkg := range prog.AllPackages() {
		if pkg.Pkg.Name() == "main" {
			mainPkg = pkg
			if mainPkg.Func("main") == nil {
				return -1, fmt.Errorf("no func main() in main package")
			}
			break
		}
	}
	if mainPkg == nil {
		return -1, fmt.Errorf("no main package")
	}

	// Build SSA code for bodies for whole program
	prog.Build()

	//mainPkg.Func("main").WriteTo(os.Stdout) // list the main func in SSA form for DEBUG

	ssai.exts = interp.NewExternals()
	// add the callable external functions
	for _, ef := range extFns {
		ssai.exts.AddExtFunc(ef.Nam, ef.Fun)
	}

	context, exitCode := interp.Interpret(mainPkg, 0,
		&types.StdSizes{
			WordSize: 8, // word size in bytes - must be >= 4 (32bits)
			MaxAlign: 8, // maximum alignment in bytes - must be >= 1
		}, "main.go", args, ssai.exts, output)
	if context == nil {
		return -1, fmt.Errorf("nil context returned")
	}
	ssai.Context = context
	ssai.MainPkg = mainPkg
	return exitCode, nil
}

// Call a function in an already created interpreter.
func (ssai *Interpreter) Call(name string, args []interp.Ivalue) (interp.Ivalue, error) {
	if ssai == nil {
		return nil, fmt.Errorf("nil *Interpreter")
	}
	if ssai.Panic != nil {
		return nil, fmt.Errorf("prior panic in Interpreter: %v", ssai.Panic)
	}
	result := ssai.call(name, args)
	if ssai.Panic != nil {
		return nil, fmt.Errorf("panic in Call: %v", ssai.Panic)
	}
	return result, nil
}

func (ssai *Interpreter) call(name string, args []interp.Ivalue) interp.Ivalue {
	defer func() {
		if r := recover(); r != nil {
			ssai.Panic = r
		}
	}()
	return ssai.Context.Call(name, args)
}
