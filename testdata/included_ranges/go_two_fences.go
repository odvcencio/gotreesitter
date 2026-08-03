// Copyright notice line one.
// Copyright notice line two.

package main

import (
	"syscall"
	"time"
)

var initCh = make(chan int, 1)
var ranMain bool

func init() {
	time.Sleep(100 * time.Millisecond)
	initCh <- 42
}

func main() {
	ranMain = true
	_ = syscall.Getpid()
}
