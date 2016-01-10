package interp

import (
	"fmt"
	"go/token"
	"reflect"
	"sync"

	"go/types"

	"golang.org/x/tools/go/ssa"
)

//go:generate interpgen

func (i *interpreter) preprocess() {
	/*
		i.externals.globVals = make(map[ssa.Value]*globInfo)
		for _, pkg := range i.prog.AllPackages() {
			for _, mem := range pkg.Members {
				//fn, isFunc := mem.(*ssa.Function)
				//if isFunc {
				//	i.preprocessFn(fn)
				//}
				gl, isGlobal := mem.(*ssa.Global)
				if isGlobal {
					ai, isAccessed := i.externals.globs[gl.String()]
					if isAccessed {
						i.externals.globVals[mem.(ssa.Value)] = ai
						//println("DEBUG overload global: " + gl.String())
					}
				}
			}
		}
	*/
}

func (fr *frame) getFnInf() functionInfo {
	fr.i.functionsRWMutex.RLock()
	fnInf, found := fr.i.functions[fr.fn]
	fr.i.functionsRWMutex.RUnlock()
	if !found {
		fr.i.functionsRWMutex.Lock()
		fr.i.preprocessFn(fr.fn)
		fr.i.functionsRWMutex.Unlock()
		fnInf = fr.i.functions[fr.fn]
	}
	return fnInf
}

const debugStats = false

// debugMap allows debugging information to be stored and inspected, not for external use.
var debugMap = make(map[string]int)
var debugMapMutex sync.Mutex

func (i *interpreter) preprocessFn(fn *ssa.Function) {
	//fmt.Println("DEBUG preprocessFn " + fn.Name())
	_, found := i.functions[fn]
	if !found {
		i.runtimeFunc[reflect.ValueOf(fn).Pointer()] = fn // to avoid using unsafe package

		if _, isExtern := i.externals.funcs[fn.String()]; isExtern {
			if fn.Blocks != nil {
				fn.Blocks = nil
				//fmt.Println("DEBUG overloaded function ", fn.String())
			}
		} else {

			i.functions[fn] = functionInfo{
				instrs:     make([][]func(*frame), len(fn.Blocks)),
				envEntries: make(map[ssa.Value]int),
			}
			// set-up envEntries
			entryNum := 0
			for _, param := range fn.Params {
				i.functions[fn].envEntries[param] = entryNum
				entryNum++
			}
			for _, bound := range fn.FreeVars {
				i.functions[fn].envEntries[bound] = entryNum
				entryNum++
			}
			for bNum, blk := range fn.Blocks {
				i.functions[fn].instrs[bNum] = make([]func(*frame), len(blk.Instrs))
				for _, ins := range blk.Instrs {
					if val, ok := ins.(ssa.Value); ok {
						i.functions[fn].envEntries[val] = entryNum
						entryNum++
					}
				}
			}

			// create ops
			for bNum, blk := range fn.Blocks {
				for iNum, ins := range blk.Instrs {
					/*
						for _, rand := range ins.Operands(nil) {
							val := *rand
							switch val.(type) {
							case *ssa.Const:
								i.preprocessConst(val.(*ssa.Const))
								//case *ssa.Function:
								//	i.preprocessFn(val.(*ssa.Function))
							}
						}
					*/

					switch ins.(type) {
					case *ssa.UnOp:
						op := ins.(*ssa.UnOp)
						xFn := i.getFunc(fn, op.X)
						val := ins.(ssa.Value)
						envTgt := i.functions[fn].envEntries[val]
						i.functions[fn].instrs[bNum][iNum] =
							func(fr *frame) { // TODO handle simple cases directly
								fr.env[envTgt] = fr.i.unop(op, xFn(fr))
							}

					case *ssa.BinOp:
						op := ins.(*ssa.BinOp)
						boFn := i.binopFunc(op.Op, op.X.Type())
						xFn := i.getFunc(fn, op.X)
						yFn := i.getFunc(fn, op.Y)
						val := ins.(ssa.Value)
						envTgt := i.functions[fn].envEntries[val]
						i.functions[fn].instrs[bNum][iNum] =
							func(fr *frame) {
								fr.env[envTgt] = boFn(xFn(fr), yFn(fr))
							}

					case *ssa.Convert:
						op := ins.(*ssa.Convert)
						xFn := i.getFunc(fn, op.X)
						val := ins.(ssa.Value)
						envTgt := i.functions[fn].envEntries[val]
						srcT := op.X.Type()
						desT := op.Type()
						i.functions[fn].instrs[bNum][iNum] =
							func(fr *frame) {
								fr.env[envTgt] = fr.i.conv(desT, srcT, xFn(fr))
							} // the default postition
						desTul := desT.Underlying()
						srcTul := srcT.Underlying()
						if desBas, ok := desTul.(*types.Basic); ok {
							if srcBas, ok := srcTul.(*types.Basic); ok {
								ec := easyConvFunc(desBas.Kind(), srcBas.Kind())
								if ec != nil {
									i.functions[fn].instrs[bNum][iNum] =
										func(fr *frame) {
											fr.env[envTgt] = ec(xFn(fr))
										}
								}
							}
						}

					case *ssa.Store:
						op := ins.(*ssa.Store)
						typ := op.Val.Type()
						addr := i.getFunc(fn, op.Addr)
						val := i.getFunc(fn, op.Val)
						i.functions[fn].instrs[bNum][iNum] =
							func(fr *frame) { // TODO break out types
								store(typ, addr(fr).(*Ivalue), val(fr))
							}

					case *ssa.IndexAddr:
						op := ins.(*ssa.IndexAddr)
						val := ins.(ssa.Value)
						envTgt := i.functions[fn].envEntries[val]
						i.functions[fn].instrs[bNum][iNum] =
							i.indexAddrFn(op, envTgt)

					case *ssa.FieldAddr:
						op := ins.(*ssa.FieldAddr)
						val := ins.(ssa.Value)
						envTgt := i.functions[fn].envEntries[val]
						i.functions[fn].instrs[bNum][iNum] =
							i.fieldAddrFn(op, envTgt)

					case *ssa.Phi:
						op := ins.(*ssa.Phi)
						val := ins.(ssa.Value)
						envTgt := i.functions[fn].envEntries[val]
						preds := ins.Block().Preds
						var edges []func(*frame) Ivalue
						for ii := range preds {
							edges = append(edges, i.getFunc(fn, op.Edges[ii]))
						}
						i.functions[fn].instrs[bNum][iNum] =
							func(fr *frame) {
								for ii, pred := range preds {
									if fr.prevBlock == pred {
										fr.env[envTgt] = edges[ii](fr)
										break
									}
								}
							}

					default:
						if debugStats {
							debugMapMutex.Lock()
							debugMap[fmt.Sprintf("%T", ins)]++
							debugMapMutex.Unlock()
						}
					}
				}
			}
			//if len(i.functions[fn].envEntries) > 0 {
			//	fmt.Printf("DEBUG envEnts %s %#v\n", fn, i.functions[fn].envEntries)
			//}
		}
	}
}

func (i *interpreter) indexAddrFn(iaInstr *ssa.IndexAddr, envTgt int) func(fr *frame) {
	xFn := i.getFunc(iaInstr.Parent(), iaInstr.X)
	idxFn := i.getFunc(iaInstr.Parent(), iaInstr.Index)
	asIntFn := easyConvFunc(types.Int,
		iaInstr.Index.Type().Underlying().(*types.Basic).Kind())
	switch iaInstr.X.Type().Underlying().(type) {
	case *types.Slice:
		return func(fr *frame) {
			fr.env[envTgt] = &(xFn(fr).([]Ivalue)[asIntFn(idxFn(fr)).(int)])
		}
	case *types.Pointer: // Array
		return func(fr *frame) {
			x := xFn(fr).(*Ivalue)
			idx := idxFn(fr)
			fr.env[envTgt] = &(*x).(array)[asIntFn(idx).(int)]
		}
	default:
		panic(fmt.Sprintf("unexpected x type in IndexAddr: %T", iaInstr.X.Type().Underlying()))
	}
}

func (i *interpreter) fieldAddrFn(op *ssa.FieldAddr, envTgt int) func(fr *frame) {
	xFn := i.getFunc(op.Parent(), op.X)
	fld := op.Field
	return func(fr *frame) {
		x := xFn(fr).(*Ivalue)
		switch (*x).(type) {
		case structure:
			// FIXME wrong!  &global.f must not change if we do *global = zero!
			fr.env[envTgt] = &((*x).(structure)[fld])
		case reflect.Value:
			// THIS CODE IS EXPERIMENTAL
			//fmt.Println("DEBUG field", fld)
			xx := (*x).(reflect.Value)
			//fmt.Println("DEBUG reflect.Value", xx)
			pseudoItem := Ivalue(xx.Elem().Field(fld).Addr())
			fr.env[envTgt] = &pseudoItem
			//fmt.Println("DEBUG address", fr.env[envTgt])
		default:
			panic(fmt.Sprintf("unexpected fieldAddress x %v:%T", x, x))
		}
	}
}

func (i *interpreter) getFunc(fn *ssa.Function, key ssa.Value) func(fr *frame) Ivalue {
	switch key := key.(type) {
	case nil:
		// Hack; simplifies handling of optional attributes
		// such as ssa.Slice.{Low,High}.
		return func(fr *frame) Ivalue {
			return nil
		}
	case *ssa.Function, *ssa.Builtin:
		return func(fr *frame) Ivalue {
			return key
		}
	case *ssa.Const:
		ret := i.constIvalue(key)
		return func(fr *frame) Ivalue {
			return ret
		}
	case *ssa.Global:
		return func(fr *frame) Ivalue {
			if r, ok := fr.i.globals[key]; ok {
				return r
			}
			panic(fmt.Sprintf("getFunc: no global Ivalue for %T: %v", key, key.Name()))
		}
	}
	envTgt, ok := i.functions[fn].envEntries[key]
	if !ok {
		panic(fmt.Sprintf("getFunc: no environment Ivalue for %T: %v", key, key.Name()))
	}
	return func(fr *frame) Ivalue {
		return fr.env[envTgt]
	}
}

/*
func (i *interpreter) preprocessConst(c *ssa.Const) {
	// NOTE this pre-processing approach is not always faster.
	i.constantsRWMutex.RLock()
	_, found := i.constants[c]
	i.constantsRWMutex.RUnlock()
	if found {
		return
	}
	i.constantsRWMutex.Lock()
	i.constants[c] = constIvalueEval(c, i)
	i.constantsRWMutex.Unlock()
	return
}
*/
/*
func (i *interpreter) preprocessZero(t types.Type) {
	// NOTE this pre-processing approach is not always faster.
	zv := i.zeroes.At(t)
	_, isIvalue := zv.(Ivalue)
	if isIvalue {
		return
	}
	i.zeroes.Set(t, zeroEval(t, i))
	return
}
*/

// binopFunc returns a closure that implements all arithmetic and logical binary operators for
// numeric datatypes and strings.  Both operands must have identical
// dynamic type.
//
func (i *interpreter) binopFunc(op token.Token, tt types.Type) func(x, y Ivalue) Ivalue {
	t := i.zero(tt)
	switch op {
	case token.ADD:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) + y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) + y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) + y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) + y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) + y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) + y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) + y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) + y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) + y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) + y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) + y.(uintptr)
			}
		case float32:
			return func(x, y Ivalue) Ivalue {
				return x.(float32) + y.(float32)
			}
		case float64:
			return func(x, y Ivalue) Ivalue {
				return x.(float64) + y.(float64)
			}
		case complex64:
			return func(x, y Ivalue) Ivalue {
				return x.(complex64) + y.(complex64)
			}
		case complex128:
			return func(x, y Ivalue) Ivalue {
				return x.(complex128) + y.(complex128)
			}
		case string:
			return func(x, y Ivalue) Ivalue {
				return x.(string) + y.(string)
			}
		}

	case token.SUB:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) - y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) - y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) - y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) - y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) - y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) - y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) - y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) - y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) - y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) - y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) - y.(uintptr)
			}
		case float32:
			return func(x, y Ivalue) Ivalue {
				return x.(float32) - y.(float32)
			}
		case float64:
			return func(x, y Ivalue) Ivalue {
				return x.(float64) - y.(float64)
			}
		case complex64:
			return func(x, y Ivalue) Ivalue {
				return x.(complex64) - y.(complex64)
			}
		case complex128:
			return func(x, y Ivalue) Ivalue {
				return x.(complex128) - y.(complex128)
			}
		}

	case token.MUL:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) * y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) * y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) * y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) * y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) * y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) * y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) * y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) * y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) * y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) * y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) * y.(uintptr)
			}
		case float32:
			return func(x, y Ivalue) Ivalue {
				return x.(float32) * y.(float32)
			}
		case float64:
			return func(x, y Ivalue) Ivalue {
				return x.(float64) * y.(float64)
			}
		case complex64:
			return func(x, y Ivalue) Ivalue {
				return x.(complex64) * y.(complex64)
			}
		case complex128:
			return func(x, y Ivalue) Ivalue {
				return x.(complex128) * y.(complex128)
			}
		}

	case token.QUO:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) / y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) / y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) / y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) / y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) / y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) / y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) / y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) / y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) / y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) / y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) / y.(uintptr)
			}
		case float32:
			return func(x, y Ivalue) Ivalue {
				return x.(float32) / y.(float32)
			}
		case float64:
			return func(x, y Ivalue) Ivalue {
				return x.(float64) / y.(float64)
			}
		case complex64:
			return func(x, y Ivalue) Ivalue {
				return x.(complex64) / y.(complex64)
			}
		case complex128:
			return func(x, y Ivalue) Ivalue {
				return x.(complex128) / y.(complex128)
			}
		}

	case token.REM:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) % y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) % y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) % y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) % y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) % y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) % y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) % y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) % y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) % y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) % y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) % y.(uintptr)
			}
		}

	case token.AND:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) & y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) & y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) & y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) & y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) & y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) & y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) & y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) & y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) & y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) & y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) & y.(uintptr)
			}
		}

	case token.OR:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) | y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) | y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) | y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) | y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) | y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) | y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) | y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) | y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) | y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) | y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) | y.(uintptr)
			}
		}

	case token.XOR:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) ^ y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) ^ y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) ^ y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) ^ y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) ^ y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) ^ y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) ^ y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) ^ y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) ^ y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) ^ y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) ^ y.(uintptr)
			}
		}

	case token.AND_NOT:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) &^ y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) &^ y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) &^ y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) &^ y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) &^ y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) &^ y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) &^ y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) &^ y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) &^ y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) &^ y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) &^ y.(uintptr)
			}
		}

	case token.SHL:
		// TODO pre-process asUint64
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) << asUint64(y)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) << asUint64(y)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) << asUint64(y)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) << asUint64(y)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) << asUint64(y)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) << asUint64(y)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) << asUint64(y)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) << asUint64(y)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) << asUint64(y)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) << asUint64(y)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) << asUint64(y)
			}
		}

	case token.SHR:
		// TODO pre-process asUint64
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) >> asUint64(y)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) >> asUint64(y)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) >> asUint64(y)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) >> asUint64(y)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) >> asUint64(y)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) >> asUint64(y)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) >> asUint64(y)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) >> asUint64(y)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) >> asUint64(y)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) >> asUint64(y)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) >> asUint64(y)
			}
		}

	case token.LSS:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) < y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) < y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) < y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) < y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) < y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) < y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) < y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) < y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) < y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) < y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) < y.(uintptr)
			}
		case float32:
			return func(x, y Ivalue) Ivalue {
				return x.(float32) < y.(float32)
			}
		case float64:
			return func(x, y Ivalue) Ivalue {
				return x.(float64) < y.(float64)
			}
		case string:
			return func(x, y Ivalue) Ivalue {
				return x.(string) < y.(string)
			}
		}

	case token.LEQ:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) <= y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) <= y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) <= y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) <= y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) <= y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) <= y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) <= y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) <= y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) <= y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) <= y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) <= y.(uintptr)
			}
		case float32:
			return func(x, y Ivalue) Ivalue {
				return x.(float32) <= y.(float32)
			}
		case float64:
			return func(x, y Ivalue) Ivalue {
				return x.(float64) <= y.(float64)
			}
		case string:
			return func(x, y Ivalue) Ivalue {
				return x.(string) <= y.(string)
			}
		}

	case token.EQL:
		// TODO pre-process simple cases
		return func(x, y Ivalue) Ivalue {
			return eqnil(tt, x, y)
		}

	case token.NEQ:
		// TODO pre-process simple cases
		return func(x, y Ivalue) Ivalue {
			return !eqnil(tt, x, y)
		}
	case token.GTR:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) > y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) > y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) > y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) > y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) > y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) > y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) > y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) > y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) > y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) > y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) > y.(uintptr)
			}
		case float32:
			return func(x, y Ivalue) Ivalue {
				return x.(float32) > y.(float32)
			}
		case float64:
			return func(x, y Ivalue) Ivalue {
				return x.(float64) > y.(float64)
			}
		case string:
			return func(x, y Ivalue) Ivalue {
				return x.(string) > y.(string)
			}
		}

	case token.GEQ:
		switch t.(type) {
		case int:
			return func(x, y Ivalue) Ivalue {
				return x.(int) >= y.(int)
			}
		case int8:
			return func(x, y Ivalue) Ivalue {
				return x.(int8) >= y.(int8)
			}
		case int16:
			return func(x, y Ivalue) Ivalue {
				return x.(int16) >= y.(int16)
			}
		case int32:
			return func(x, y Ivalue) Ivalue {
				return x.(int32) >= y.(int32)
			}
		case int64:
			return func(x, y Ivalue) Ivalue {
				return x.(int64) >= y.(int64)
			}
		case uint:
			return func(x, y Ivalue) Ivalue {
				return x.(uint) >= y.(uint)
			}
		case uint8:
			return func(x, y Ivalue) Ivalue {
				return x.(uint8) >= y.(uint8)
			}
		case uint16:
			return func(x, y Ivalue) Ivalue {
				return x.(uint16) >= y.(uint16)
			}
		case uint32:
			return func(x, y Ivalue) Ivalue {
				return x.(uint32) >= y.(uint32)
			}
		case uint64:
			return func(x, y Ivalue) Ivalue {
				return x.(uint64) >= y.(uint64)
			}
		case uintptr:
			return func(x, y Ivalue) Ivalue {
				return x.(uintptr) >= y.(uintptr)
			}
		case float32:
			return func(x, y Ivalue) Ivalue {
				return x.(float32) >= y.(float32)
			}
		case float64:
			return func(x, y Ivalue) Ivalue {
				return x.(float64) >= y.(float64)
			}
		case string:
			return func(x, y Ivalue) Ivalue {
				return x.(string) >= y.(string)
			}
		}
	}
	panic(fmt.Sprintf("invalid binary op: %T %s ", t, op))
}
