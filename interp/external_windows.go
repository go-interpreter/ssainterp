// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package interp

import "syscall"

func ext۰syscall۰Close(fr *frame, args []Ivalue) Ivalue {
	panic("syscall.Close not yet implemented")
}
func ext۰syscall۰Fstat(fr *frame, args []Ivalue) Ivalue {
	panic("syscall.Fstat not yet implemented")
}
func ext۰syscall۰Kill(fr *frame, args []Ivalue) Ivalue {
	panic("syscall.Kill not yet implemented")
}
func ext۰syscall۰Lstat(fr *frame, args []Ivalue) Ivalue {
	panic("syscall.Lstat not yet implemented")
}
func ext۰syscall۰Open(fr *frame, args []Ivalue) Ivalue {
	panic("syscall.Open not yet implemented")
}
func ext۰syscall۰ParseDirent(fr *frame, args []Ivalue) Ivalue {
	panic("syscall.ParseDirent not yet implemented")
}
func ext۰syscall۰Read(fr *frame, args []Ivalue) Ivalue {
	panic("syscall.Read not yet implemented")
}
func ext۰syscall۰ReadDirent(fr *frame, args []Ivalue) Ivalue {
	panic("syscall.ReadDirent not yet implemented")
}
func ext۰syscall۰Stat(fr *frame, args []Ivalue) Ivalue {
	panic("syscall.Stat not yet implemented")
}
func ext۰syscall۰Write(fr *frame, args []Ivalue) Ivalue {
	panic("syscall.Write not yet implemented")
}
func ext۰syscall۰RawSyscall(fr *frame, args []Ivalue) Ivalue {
	return tuple{uintptr(0), uintptr(0), uintptr(syscall.ENOSYS)}
}
func syswrite(fd int, b []byte) (int, error) {
	panic("syswrite not yet implemented")
}
