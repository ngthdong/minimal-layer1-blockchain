// Command sim runs one scenario and writes its event log.
//
//	cd src
//	go run ./cmd/sim -config ../config/t1_normal.json
//
// By default the log goes to <config dir>/../logs/<name>.log, so the result
// is the same whatever directory the command is started from.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ngthdong/minimal-layer1-blockchain/sim"
	"github.com/ngthdong/minimal-layer1-blockchain/simconfig"
)

func main() {
	cfgPath := flag.String("config", "", "scenario file (required)")
	logPath := flag.String("log", "", "log file (default: <config dir>/../logs/<name>.log)")
	flag.Parse()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "usage: sim -config <file.json> [-log <file.log>]")
		os.Exit(2)
	}

	cfg, err := simconfig.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}
	if *logPath == "" {
		*logPath = filepath.Join(filepath.Dir(*cfgPath), "..", "logs", cfg.Name+".log")
	}
	f, err := os.Create(*logPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "log:", err)
		os.Exit(2)
	}

	res, runErr := sim.Run(cfg, f)
	if err := f.Close(); err != nil && runErr == nil {
		runErr = err
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "run:", runErr)
		os.Exit(1)
	}
	fmt.Printf("scenario=%s agreed=%v final_state=%s end_ms=%d log=%s\n",
		cfg.Name, res.Agreed, res.FinalStateHash.Hex(), res.EndTimeMs, *logPath)
	if !res.Agreed {
		os.Exit(1)
	}
}
