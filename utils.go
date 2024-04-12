package main

import (
	"fmt"
	"log/slog"
	"regexp"
	"runtime"
	"time"

	common "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
)

func TimeTrackFile(start time.Time, file string) {
	elapsed := time.Since(start)

	slog.Info(fmt.Sprintf("%s took %s\n", file, elapsed))
}

func TimeTrack(start time.Time) {
	elapsed := time.Since(start)

	// Skip this function, and fetch the PC and file for its parent.
	pc, _, _, _ := runtime.Caller(1)

	// Retrieve a function object this functions parent.
	funcObj := runtime.FuncForPC(pc)

	// Regex to extract just the function name (and not the module path).
	runtimeFunc := regexp.MustCompile(`^.*\.(.*)$`)
	name := runtimeFunc.ReplaceAllString(funcObj.Name(), "$1")

	slog.Info(fmt.Sprintf("%s took %s\n", name, elapsed))
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func getSteamID64(p *common.Player) uint64 {
	if p == nil {
		return 0
	}

	return p.SteamID64
}

func getPlayerTeam(p *common.Player) int {
	if p == nil {
		return -1
	}

	return int(p.Team)
}
