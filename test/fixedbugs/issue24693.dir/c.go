// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "./b"

func main() {
	b.F1(b.T{})
	b.F2(b.T{})
}
