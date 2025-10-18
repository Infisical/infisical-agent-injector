package path

import "strings"

// Credits: https://github.com/golang/go/blob/master/src/internal/filepathlite/path_windows.go and https://github.com/golang/go/blob/master/src/internal/filepathlite/path_unix.go
func IsAbs(path string, isWindows bool) bool {
	if isWindows {
		return isWindowsAbs(path)
	}
	return isUnixAbs(path)
}

func Dir(path string, isWindows bool) string {
	var sep string
	if isWindows {
		sep = "\\"
	} else {
		sep = "/"
	}

	if path == "" {
		return "."
	}

	path = strings.TrimRight(path, sep)

	if path == "" {
		return sep
	}

	if isWindows && len(path) == 2 && path[1] == ':' {
		return path + sep
	}

	// Handle Windows UNC path (e.g., "\\server\share")
	if isWindows && strings.HasPrefix(path, "\\\\") {
		parts := strings.SplitN(path[2:], "\\", 3)
		if len(parts) <= 2 {
			return path
		}
	}

	lastSep := strings.LastIndex(path, sep)
	if lastSep == -1 {
		return "."
	}

	if lastSep == 0 {
		return sep
	}

	if isWindows && lastSep == 2 && path[1] == ':' {
		return path[:3]
	}

	return path[:lastSep]
}
