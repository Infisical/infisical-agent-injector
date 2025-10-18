// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package path

func isUnixAbs(path string) bool {

	prefix := "/"

	return len(path) >= len(prefix) && path[:len(prefix)] == prefix

}
