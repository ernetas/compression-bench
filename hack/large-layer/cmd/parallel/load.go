package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// checkLoad refuses to benchmark on a machine that is already busy.
//
// Every throughput number in this harness is wall-clock over a fixed input, so a
// competing workload silently deflates it -- and the giveaway (cores = cpu/wall
// dropping below 1.0 for a single-threaded codec) is easy to miss in a table.
// This turns that into a hard stop instead. Memory figures are not affected, so
// -ignore-load stays available for RSS-only runs.
//
// Linux only; on other platforms it reports nothing and allows the run.
func checkLoad(max float64) error {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil // not Linux, or no procfs: nothing to check against
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return nil
	}
	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil
	}
	if load1 > max {
		return fmt.Errorf("1-minute load average is %.2f, above the %.2f threshold: "+
			"another workload is using this machine and timings would be wrong "+
			"(pass -ignore-load to measure memory anyway)", load1, max)
	}
	return nil
}
