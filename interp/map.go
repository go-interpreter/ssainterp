// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package interp

// Custom hashtable atop map.
// For use when the key's equivalence relation is not consistent with ==.

// The Go specification doesn't address the atomicity of map operations.
// The FAQ states that an implementation is permitted to crash on
// concurrent map access.

import (
	"golang.org/x/tools/go/types"
)

type hashable interface {
	hash(t types.Type) int
	eq(t types.Type, x interface{}) bool
}

type entry struct {
	key    hashable
	Ivalue Ivalue
	next   *entry
}

// A hashtable atop the built-in map.  Since each bucket contains
// exactly one hash Ivalue, there's no need to perform hash-equality
// tests when walking the linked list.  Rehashing is done by the
// underlying map.
type hashmap struct {
	keyType types.Type
	table   map[int]*entry
	length  int // number of entries in map
}

// makeMap returns an empty initialized map of key type kt,
// preallocating space for reserve elements.
func makeMap(kt types.Type, reserve int) Ivalue {
	if usesBuiltinMap(kt) {
		return make(map[Ivalue]Ivalue, reserve)
	}
	return &hashmap{keyType: kt, table: make(map[int]*entry, reserve)}
}

// delete removes the association for key k, if any.
func (m *hashmap) delete(k hashable) {
	if m != nil {
		hash := k.hash(m.keyType)
		head := m.table[hash]
		if head != nil {
			if k.eq(m.keyType, head.key) {
				m.table[hash] = head.next
				m.length--
				return
			}
			prev := head
			for e := head.next; e != nil; e = e.next {
				if k.eq(m.keyType, e.key) {
					prev.next = e.next
					m.length--
					return
				}
				prev = e
			}
		}
	}
}

// lookup returns the Ivalue associated with key k, if present, or
// Ivalue(nil) otherwise.
func (m *hashmap) lookup(k hashable) Ivalue {
	if m != nil {
		hash := k.hash(m.keyType)
		for e := m.table[hash]; e != nil; e = e.next {
			if k.eq(m.keyType, e.key) {
				return e.Ivalue
			}
		}
	}
	return nil
}

// insert updates the map to associate key k with Ivalue v.  If there
// was already an association for an eq() (though not necessarily ==)
// k, the previous key remains in the map and its associated Ivalue is
// updated.
func (m *hashmap) insert(k hashable, v Ivalue) {
	hash := k.hash(m.keyType)
	head := m.table[hash]
	for e := head; e != nil; e = e.next {
		if k.eq(m.keyType, e.key) {
			e.Ivalue = v
			return
		}
	}
	m.table[hash] = &entry{
		key:    k,
		Ivalue: v,
		next:   head,
	}
	m.length++
}

// len returns the number of key/Ivalue associations in the map.
func (m *hashmap) len() int {
	if m != nil {
		return m.length
	}
	return 0
}
