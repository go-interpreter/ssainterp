package interp

import (
	"fmt"

	"golang.org/x/tools/go/ssa"
)

type functionInfo struct {
	instrs     [][]func(*frame)
	envEntries map[ssa.Value]int
}

func envEnt(fr *frame, key ssa.Value) int {
	ent, ok := fr.i.functions[fr.fn].envEntries[key]
	if !ok {
		panic(fmt.Sprintf("envRnt: no Ivalue for %T: %v", key, key.Name()))
	}
	return ent
}
