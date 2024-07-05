package main

import (
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maxsupermanhd/lac/v2"
)

type instanceState int

const (
	instanceStateInitial instanceState = iota
	instanceStateStarting
	instanceStateInLobby
	instanceStateInGame
	instanceStateExiting
	instanceStateExited
)

type adminsPolicy int

const (
	adminsPolicyNobody adminsPolicy = iota
	adminsPolicySuperadmins
	adminsPolicyModerators
	adminsPolicyWhitelist
)

type joinDispatch struct {
	AllowChat bool
	Messages  []string
	Issued    time.Time
}

type instanceSettings struct {
	GamePort         int
	MapName          string
	MapHash          string
	PlayerCount      int
	TimeLimit        int
	Mods             string
	DisplayCategory  int
	RatingCategories []int
}

type instance struct {
	Id                  int64
	LobbyId             int
	GameId              int
	DebugTriggered      bool
	ConfDir             string
	BinPath             string
	Admins              []string
	AdminsPolicy        adminsPolicy
	OnJoinDispatch      map[string]joinDispatch
	QueueName           string
	AutodetectedVersion string
	state               atomic.Int64
	StateSaved          int
	cfg                 lac.Conf
	cfgs                []lac.Conf
	RestoreCfgs         []map[string]any
	Settings            instanceSettings
	logger              *log.Logger
	stdin               *os.File
	stdout              *os.File
	stderr              *os.File
	Pid                 int
	recovered           bool
	commands            chan instanceCommand
	wg                  sync.WaitGroup
}
