//go:build race

package openapi

// raceSlowdown melonggarkan batas waktu perf: detektor race memperlambat eksekusi beberapa kali lipat.
const raceSlowdown = 5
