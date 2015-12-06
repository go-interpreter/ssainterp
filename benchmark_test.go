package ssainterp_test

import (
	"testing"

	"golang.org/x/tools/go/types"

	"github.com/go-interpreter/ssainterp"
	"github.com/go-interpreter/ssainterp/interp"

	orig "golang.org/x/tools/go/ssa/interp"
)

// just using http://dave.cheney.net/2013/06/30/how-to-write-benchmarks-in-go
func Fib(n int) int {
	if n < 2 {
		return n
	}
	return Fib(n-1) + Fib(n-2)
}

func BenchmarkFib10Native(b *testing.B) {
	// run the Fib function b.N times
	for n := 0; n < b.N; n++ {
		Fib(10)
	}
}

const codeFib = `
package main
func main(){}
func Fib(n int) int {
        if n < 2 {
                return n
        }
        return Fib(n-1) + Fib(n-2)
}
`

var ctx *ssainterp.Interpreter

func init() {
	var err error
	ctx, _, err = ssainterp.Run(codeFib, nil, nil, nil)
	if err != nil {
		panic(err)
	}
}

func BenchmarkFib10SSAinterp(b *testing.B) {
	// run the Fib function b.N times
	for n := 0; n < b.N; n++ {
		ctx.Call("main.Fib", []interp.Ivalue{interp.Ivalue(10)})
	}
}

const codeFib10Orig = `
package main
func main(){ Fib(10) }
func Fib(n int) int {
        if n < 2 {
                return n
        }
        return Fib(n-1) + Fib(n-2)
}
`

var ctxOrig *ssainterp.Interpreter

func init() {
	var err error
	ctxOrig, _, err = ssainterp.Run(codeFib10Orig, nil, nil, nil)
	if err != nil {
		panic(err)
	}
}

func BenchmarkFib10Orig(b *testing.B) {
	// run the Fib function b.N times
	for n := 0; n < b.N; n++ {
		ec := orig.Interpret(ctxOrig.MainPkg, 0, &types.StdSizes{8, 8}, "filename.go", nil)
		if ec != 0 {
			b.Fatalf("returned error code non-zero:%d", ec)
		}
	}
}
