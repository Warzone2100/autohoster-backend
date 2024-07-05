package main

type instanceCommandType int

const (
	icNone instanceCommandType = iota
	icShutdown
	icBroadcast
	icRunnerStop
)

type instanceCommand struct {
	command instanceCommandType
	data    any
}
