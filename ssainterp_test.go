package ssainterp_test

import (
	"bytes"
	"testing"

	"github.com/go-interpreter/ssainterp"
	"github.com/go-interpreter/ssainterp/interp"
)

func TestOutputCapture(t *testing.T) {
	var out bytes.Buffer

	_, _, err := ssainterp.Run(`
	   package main

	   func main() {
	   		println("TEST-OUTPUT")
	   }
	   		`, nil, nil, &out)
	if err != nil {
		t.Error(err)
	}
	s := string(out.Bytes())
	if s != "TEST-OUTPUT\n" {
		t.Error("output not as expected:" + s)
	}
}

func TestCallInOut(t *testing.T) {
	ctxt, exitCode, err := ssainterp.Run(`
package main

// function to call into proper Go
func Foo(a,b int) int 

// function to be called from proper Go
func fact(n int) int {
	//println("fact",n)
	if n == 0 {
		return 1
	}
	return n * fact(n-1)
}

func main() {
	// call a func defined externally
	if Foo(2,2) != 4 {
		panic("2+2 not equal 4!")
	}
}
`, []ssainterp.ExtFunc{
		ssainterp.ExtFunc{
			Nam: "main.Foo",
			Fun: func(args []interp.Ivalue) interp.Ivalue {
				return args[0].(int) + args[1].(int)
			},
		},
	}, nil, nil)
	if err != nil || exitCode != 0 {
		t.Error(exitCode, err)
		return
	}

	// call a function within the interpreter context
	ival, err := ctxt.Call("main.fact", []interp.Ivalue{int(5)})
	if err != nil {
		t.Error(err)
		return
	}
	if ival != 120 {
		t.Error("context call fact(5) != 120, value:", ival)
	}
}

func TestSSAinterpBad(t *testing.T) {
	_, _, err := ssainterp.Run(`this is not go!`, nil, nil, nil)
	if err == nil {
		t.Error("bad code did not error")
	}
	//t.Log(err)

	_, _, err = ssainterp.Run(`
package main

func notmain() {	
}
		`, nil, nil, nil)
	if err == nil {
		t.Error("no main function did not error")
	}
	//t.Log(err)

	ctx, _, err := ssainterp.Run(`
package main

func main() {	
}
		`, nil, nil, nil)
	if err != nil {
		t.Error(err)
		return
	}
	_, err = ctx.Call("very.unknown", nil)
	if err == nil {
		t.Error("unknown Interpreter.Call did not error")
	}
}

func TestUnknownInclude(t *testing.T) {
	_, _, err := ssainterp.Run(`
	   package main

	   import "there/is/no/package/with/this/name"

	   func main() {
	   }
	   		`, nil, nil, nil)
	if err == nil {
		t.Error("bad import did not error")
	}
	t.Log("NOTE: there should be a file not found error above.")
}

func TestPanic(t *testing.T) {
	_, _, err := ssainterp.Run(`
	   package main

	   func main() {
	   	panic("HELP!")
	   }
	   		`, nil, nil, nil)
	if err == nil {
		t.Error("panic did not error")
	}
	t.Log("NOTE: there should be a panic 'HELP!' above with a stack dump.")
}
