// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin

package interp

import "syscall"

func init() {
	externals["syscall.Sysctl"] = ext۰syscall۰Sysctl
	externals["syscall.Syscall6"] = ext۰syscall۰Syscall6
}

func ext۰syscall۰Sysctl(fr *frame, args []Ivalue) Ivalue {
	r, err := syscall.Sysctl(args[0].(string))
	return tuple{r, wrapError(err)}
}

func ext۰syscall۰Syscall6(fr *frame, args []Ivalue) Ivalue {
	// func Syscall6(trap, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err Errno)
	var a [7]uintptr
	for i := range a {
		a[i] = args[i].(uintptr)
	}
	r1, r2, err := syscall.Syscall6(a[0], a[1], a[2], a[3], a[4], a[5], a[6])
	return tuple{r1, r2, wrapError(err)}
}
