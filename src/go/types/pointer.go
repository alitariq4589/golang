// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

// A Pointer represents a pointer type.
type Pointer struct {
	base Type // element type
}

// NewPointer returns a new pointer type for the given element (base) type.
func NewPointer(elem Type) *Pointer { return &Pointer{base: elem} }

// Elem returns the element type for the given pointer p.
func (p *Pointer) Elem() Type { return p.base }

func (t *Pointer) Underlying() Type { return t }
func (t *Pointer) String() string   { return TypeString(t, nil) }
