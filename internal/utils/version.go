package utils

import (
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
)

var Version string

func GetVersion() string {
	v := Version
	if v == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			v = info.Main.Version
		} else {
			v = "unknown"
		}
	}
	return strings.TrimPrefix(v, "v")
}

func ParseVersion(s string) (major, minor, patch int, err error) {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid version format: %s", s)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major version: %s", parts[0])
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor version: %s", parts[1])
	}
	patchStr := parts[2]
	if idx := strings.IndexAny(patchStr, "-+"); idx != -1 {
		patchStr = patchStr[:idx]
	}
	patch, err = strconv.Atoi(patchStr)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch version: %s", parts[2])
	}
	return major, minor, patch, nil
}

func IsNewer(remote, local string) bool {
	rMaj, rMin, rPat, err := ParseVersion(remote)
	if err != nil {
		return false
	}
	lMaj, lMin, lPat, err := ParseVersion(local)
	if err != nil {
		return false
	}
	if rMaj != lMaj {
		return rMaj > lMaj
	}
	if rMin != lMin {
		return rMin > lMin
	}
	return rPat > lPat
}
