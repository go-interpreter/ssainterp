package interp

// additions to the interp package
/*
type globInfo struct {
	addr *Ivalue // currently must be *Ivalue, in future *typ
	//typ  reflect.Type // reserved for future use!
}
*/

// Externals describes usable objects external to the interpreter.
type Externals struct {
	funcs map[string]externalFn
	//globs    map[string]*globInfo
	//globVals map[ssa.Value]*globInfo
}

// NewExternals makes an Externals type.
func NewExternals() *Externals {
	return &Externals{
		funcs: make(map[string]externalFn),
		//globs: make(map[string]*globInfo),
	}
}

// AddExtFunc adds an external function to the known map.
func (ext *Externals) AddExtFunc(name string, fn func([]Ivalue) Ivalue) {
	ext.funcs[name] = func(fr *frame, args []Ivalue) Ivalue {
		return fn(args)
	}
}

/*
// AddExtGlob adds read-only access to a global variable external to the interpreter,
// the global variable must be one of types.BasicKind.
// The benefit of this may not justify the cost TODO consider removing.
func (ext *Externals) AddExtGlob(name string, reflectPointer reflect.Value) {
		fmt.Printf("DEBUG reflectPointer: %v:%T reflect.Indirect(reflectPointer): %v:%T\n",
			reflectPointer.Interface(), reflectPointer.Interface(),
			reflect.Indirect(reflectPointer).Interface(),
			reflect.Indirect(reflectPointer).Interface())
	if reflectPointer.Interface() == reflect.Indirect(reflectPointer).Interface() {
		panic(fmt.Sprintf("AddExtGlob reflect value is not a pointer: %v:%T",
			reflectPointer.Interface(), reflectPointer.Interface()))
	}
	iv := Ivalue(reflectPointer)
	ext.globs[name] = &globInfo{addr: &iv}
}
*/

// IvalIfaceSlice converts a slice of Ivalues into a slice of empty interfaces.
func IvalIfaceSlice(ivs []Ivalue) []interface{} {
	rets := make([]interface{}, len(ivs))
	for i, v := range ivs {
		rets[i] = IvalIface(v)
	}
	return rets
}

// IvalIface converts an Ivalue into an interface{}.
func IvalIface(iv Ivalue) interface{} {

	switch iv.(type) {
	case bool:
		return iv.(bool)
	case int:
		return iv.(int)
	case int8:
		return iv.(int8)
	case int16:
		return iv.(int16)
	case int32:
		return iv.(int32)
	case int64:
		return iv.(int64)
	case uint:
		return iv.(uint)
	case uint8:
		return iv.(uint8)
	case uint16:
		return iv.(uint16)
	case uint32:
		return iv.(uint32)
	case uint64:
		return iv.(int64)
	case uintptr:
		return iv.(uintptr)
	case float32:
		return iv.(float32)
	case float64:
		return iv.(float64)
	case complex64:
		return iv.(complex64)
	case complex128:
		return iv.(complex128)
	case string:
		return iv.(string)
	case iface:
		return IvalIface(iv.(iface).v)
	case []Ivalue:
		return IvalIfaceSlice(iv.([]Ivalue))
	case array:
		return IvalIfaceSlice(iv.([]Ivalue)) // TODO review
	case structure:
		return IvalIfaceSlice(iv.([]Ivalue)) // TODO review
	}

	return interface{}(iv)

	/* TODO
	switch t := iv.(iface).t.(type) {
	case *types.Named:
		return interface{}(iv) // TODO reflectKind(t.Underlying())
	case *types.Basic:
		switch t.Kind() {
		case types.Bool:
			return iv.(bool)
		case types.Int:
			return iv.(int)
		case types.Int8:
			return iv.(int8)
		case types.Int16:
			return iv.(int16)
		case types.Int32:
			return iv.(int32)
		case types.Int64:
			return iv.(int64)
		case types.Uint:
			return iv.(uint)
		case types.Uint8:
			return iv.(uint8)
		case types.Uint16:
			return iv.(uint16)
		case types.Uint32:
			return iv.(uint32)
		case types.Uint64:
			return iv.(int64)
		case types.Uintptr:
			return iv.(uintptr)
		case types.Float32:
			return iv.(float32)
		case types.Float64:
			return iv.(float64)
		case types.Complex64:
			return iv.(complex64)
		case types.Complex128:
			return iv.(complex128)
		case types.String:
			return iv.(string)
		case types.UnsafePointer:
			return iv.(unsafe.Pointer) // commented out
		}
		/* TODO
		case *types.Array:
			return reflect.Array
		case *types.Chan:
			return reflect.Chan
		case *types.Signature:
			return reflect.Func
		case *types.Interface:
			return reflect.Interface
		case *types.Map:
			return reflect.Map
		case *types.Pointer:
			return reflect.Ptr
		case *types.Slice:
			return reflect.Slice
		case *types.Struct:
			return reflect.Struct
	*/
	//}
	//panic(fmt.Sprint("unexpected type: ", t))
}
