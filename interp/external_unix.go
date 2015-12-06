// Copyright 2013 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !windows,!plan9

package interp

import "syscall"

func fillStat(st *syscall.Stat_t, stat structure) {
	stat[0] = st.Dev
	stat[1] = st.Ino
	stat[2] = st.Nlink
	stat[3] = st.Mode
	stat[4] = st.Uid
	stat[5] = st.Gid

	stat[7] = st.Rdev
	stat[8] = st.Size
	stat[9] = st.Blksize
	stat[10] = st.Blocks
	// TODO(adonovan): fix: copy Timespecs.
	// stat[11] = st.Atim
	// stat[12] = st.Mtim
	// stat[13] = st.Ctim
}

func ext۰syscall۰Close(fr *frame, args []Ivalue) Ivalue {
	// func Close(fd int) (err error)
	return wrapError(syscall.Close(args[0].(int)))
}

func ext۰syscall۰Fstat(fr *frame, args []Ivalue) Ivalue {
	// func Fstat(fd int, stat *Stat_t) (err error)
	fd := args[0].(int)
	stat := (*args[1].(*Ivalue)).(structure)

	var st syscall.Stat_t
	err := syscall.Fstat(fd, &st)
	fillStat(&st, stat)
	return wrapError(err)
}

func ext۰syscall۰ReadDirent(fr *frame, args []Ivalue) Ivalue {
	// func ReadDirent(fd int, buf []byte) (n int, err error)
	fd := args[0].(int)
	p := args[1].([]Ivalue)
	b := make([]byte, len(p))
	n, err := syscall.ReadDirent(fd, b)
	for i := 0; i < n; i++ {
		p[i] = b[i]
	}
	return tuple{n, wrapError(err)}
}

func ext۰syscall۰Kill(fr *frame, args []Ivalue) Ivalue {
	// func Kill(pid int, sig Signal) (err error)
	return wrapError(syscall.Kill(args[0].(int), syscall.Signal(args[1].(int))))
}

func ext۰syscall۰Lstat(fr *frame, args []Ivalue) Ivalue {
	// func Lstat(name string, stat *Stat_t) (err error)
	name := args[0].(string)
	stat := (*args[1].(*Ivalue)).(structure)

	var st syscall.Stat_t
	err := syscall.Lstat(name, &st)
	fillStat(&st, stat)
	return wrapError(err)
}

func ext۰syscall۰Open(fr *frame, args []Ivalue) Ivalue {
	// func Open(path string, mode int, perm uint32) (fd int, err error) {
	path := args[0].(string)
	mode := args[1].(int)
	perm := args[2].(uint32)
	fd, err := syscall.Open(path, mode, perm)
	return tuple{fd, wrapError(err)}
}

func ext۰syscall۰ParseDirent(fr *frame, args []Ivalue) Ivalue {
	// func ParseDirent(buf []byte, max int, names []string) (consumed int, count int, newnames []string)
	max := args[1].(int)
	var names []string
	for _, iname := range args[2].([]Ivalue) {
		names = append(names, iname.(string))
	}
	consumed, count, newnames := syscall.ParseDirent(IvalueToBytes(args[0]), max, names)
	var inewnames []Ivalue
	for _, newname := range newnames {
		inewnames = append(inewnames, newname)
	}
	return tuple{consumed, count, inewnames}
}

func ext۰syscall۰Read(fr *frame, args []Ivalue) Ivalue {
	// func Read(fd int, p []byte) (n int, err error)
	fd := args[0].(int)
	p := args[1].([]Ivalue)
	b := make([]byte, len(p))
	n, err := syscall.Read(fd, b)
	for i := 0; i < n; i++ {
		p[i] = b[i]
	}
	return tuple{n, wrapError(err)}
}

func ext۰syscall۰Stat(fr *frame, args []Ivalue) Ivalue {
	// func Stat(name string, stat *Stat_t) (err error)
	name := args[0].(string)
	stat := (*args[1].(*Ivalue)).(structure)

	var st syscall.Stat_t
	err := syscall.Stat(name, &st)
	fillStat(&st, stat)
	return wrapError(err)
}

func ext۰syscall۰Write(fr *frame, args []Ivalue) Ivalue {
	// func Write(fd int, p []byte) (n int, err error)
	n, err := write(fr, args[0].(int), IvalueToBytes(args[1]))
	return tuple{n, wrapError(err)}
}

func ext۰syscall۰Pipe(fr *frame, args []Ivalue) Ivalue {
	//func Pipe(p []int) (err error)
	pp := args[0].([]Ivalue)
	p := make([]int, len(pp))
	for k, v := range pp {
		p[k] = v.(int)
	}
	err := syscall.Pipe(p)
	for k, v := range p {
		pp[k] = v
	}
	return wrapError(err)
}

func ext۰syscall۰Syscall(fr *frame, args []Ivalue) Ivalue {
	//func Syscall(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno)
	p := make([]uintptr, len(args))
	for k, v := range args {
		p[k] = v.(uintptr)
	}
	r1, r2, err := syscall.Syscall(p[0], p[1], p[2], p[3])
	return tuple{Ivalue(r1), Ivalue(r2), wrapError(err)}
}

func ext۰syscall۰CloseOnExec(fr *frame, args []Ivalue) Ivalue {
	//func CloseOnExec(fd int)
	syscall.CloseOnExec(args[0].(int))
	return nil
}

func ext۰syscall۰RawSyscall(fr *frame, args []Ivalue) Ivalue {
	return tuple{uintptr(0), uintptr(0), uintptr(syscall.ENOSYS)}
}

func syswrite(fd int, b []byte) (int, error) {
	return syscall.Write(fd, b)
}
