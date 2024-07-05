package main

import (
	"sync/atomic"
	"time"
)

var (
	prevInstanceID atomic.Int64
)

func newInstanceID() int64 {
	for {
		newid := time.Now().Unix()
		if prevInstanceID.Swap(newid) >= newid {
			time.Sleep(time.Duration(100) * time.Millisecond)
			continue
		}
		return newid
	}
}
