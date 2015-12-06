package main

import "io/ioutil"

import "unicode"

func main() {
	numT := []string{"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"uintptr", "float32", "float64"}

	fns := ""

	body := "\tswitch srcT {\n"
	for s := range numT {
		styp := string(unicode.ToUpper(rune(numT[s][0]))) + numT[s][1:]
		body += "\tcase types." + styp + ":\n"
		body += "\t\tswitch desT {\n"
		for d := range numT {
			dtyp := string(unicode.ToUpper(rune(numT[d][0]))) + numT[d][1:]
			body += "\t\tcase types." + dtyp + ":\n"
			fname := "easyConvTo" + dtyp + "From" + styp
			body += "\t\t\treturn " + fname + "\n"
			fns += "func " + fname + "(v Ivalue) Ivalue {\n"
			fns += "\treturn " + numT[d] + "(v.(" + numT[s] + "))\n"
			fns += "}\n\n"
		}
		body += "\t\t}\n"
	}
	body += "\t}\n"

	ioutil.WriteFile("generated.go", []byte(`package interp

import "golang.org/x/tools/go/types"

`+fns+`
func easyConvFunc(desT, srcT types.BasicKind) func(Ivalue) Ivalue {
`+body+`
	return nil
}
`), 0666)
}
