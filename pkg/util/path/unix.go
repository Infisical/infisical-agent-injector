// Copyright 2024 The Go Authors. All rights reserved.

package path

func isUnixAbs(path string) bool {

	prefix := "/"

	return len(path) >= len(prefix) && path[:len(prefix)] == prefix

}
