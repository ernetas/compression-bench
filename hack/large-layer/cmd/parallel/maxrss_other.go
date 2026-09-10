//go:build !linux

package main

// Darwin and the BSDs report ru_maxrss in bytes rather than KiB.
const maxrssIsBytes = true
