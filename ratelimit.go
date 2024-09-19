package main

import (
	"container/list"
	"encoding/json"
	"io/fs"
	"log"
	"maps"
	"os"
	"sync"
	"time"
)

var (
	ratelimitChatLock      = sync.Mutex{}
	ratelimitLastCleanup   = time.Now()
	ratelimitChatData      = map[string]*list.List{}
	ratelimitChatPenalties = map[string]time.Time{}
)

func _ratelimitChatSavePenalties() {
	b, err := json.MarshalIndent(ratelimitChatPenalties, "", "\t")
	if err != nil {
		log.Println("Failed to save chat rate limit penalties: ", err)
		return
	}
	perm := fs.FileMode(cfg.GetDInt(644, "filePerms"))
	os.WriteFile(cfg.GetDString("ratelimitChatPenalties.json", "ratelimitPenaltiesFilename"), b, perm)
}

func ratelimitChatLoadPenalties() map[string]time.Time {
	b, err := os.ReadFile(cfg.GetDString("ratelimitChatPenalties.json", "ratelimitPenaltiesFilename"))
	if err != nil {
		return map[string]time.Time{}
	}
	ret := map[string]time.Time{}
	if json.Unmarshal(b, &ret) != nil {
		return map[string]time.Time{}
	}
	return ret
}

func ratelimitChatHandleMessage(inst *instance, ip string) (time.Duration, bool) {
	ca := tryCfgGetD(tryGetIntGen("ratelimitChatAmount"), 0, inst.cfgs...)
	if ca <= 0 {
		return 0, false
	}
	ct := tryCfgGetD(tryGetIntGen("ratelimitChatDuration"), 0, inst.cfgs...)
	if ct <= 0 {
		return 0, false
	}
	ratelimitChatLock.Lock()
	_ratelimitCleanup()
	rld, rlt := _ratelimitChatAddMessage(ip, ca, time.Duration(ct)*time.Second)
	ratelimitChatLock.Unlock()
	return rld, rlt
}

func _ratelimitChatAddMessage(ip string, ca int, ct time.Duration) (time.Duration, bool) {
	l, ok := ratelimitChatData[ip]
	if !ok {
		l := list.New()
		l.PushFront(time.Now())
		ratelimitChatData[ip] = l
		return 0, false
	}
	l.PushFront(time.Now())
	if _ratelimitCheckList(l, ca, ct) {
		lastp, ok := ratelimitChatPenalties[ip]
		dur := 5 * time.Minute
		if ok && time.Since(lastp) < 30*time.Minute {
			dur = 45 * time.Minute
		}
		due := time.Now().Add(dur)
		ratelimitChatPenalties[ip] = due
		_ratelimitChatSavePenalties()
		return dur, true
	}
	return 0, false
}

func _ratelimitCheckList(l *list.List, ca int, ct time.Duration) bool {
	hits := 0
	for e := l.Front(); e != nil; e = e.Next() {
		t := e.Value.(time.Time)
		if time.Since(t) < time.Second*time.Duration(ct) {
			hits++
		}
	}
	return hits >= ca
}

func ratelimitChatCheck(_ *instance, ip string) (time.Duration, bool) {
	ratelimitChatLock.Lock()
	rld, rlt := _ratelimitChatCheckPenalties(ip)
	ratelimitChatLock.Unlock()
	return rld, rlt
}

func _ratelimitChatCheckPenalties(ip string) (time.Duration, bool) {
	_ratelimitCleanup()
	p, ok := ratelimitChatPenalties[ip]
	if !ok {
		return 0, false
	}
	if p.After(time.Now()) {
		return time.Until(p), true
	} else {
		return 0, false
	}
}

func _ratelimitCleanup() {
	if time.Since(ratelimitLastCleanup) < 5*time.Minute {
		return
	}
	maps.DeleteFunc(ratelimitChatPenalties, func(k string, v time.Time) bool {
		return time.Since(v) > time.Hour
	})
	maps.DeleteFunc(ratelimitChatData, func(k string, v *list.List) bool {
		return _ratelimitPruneList(v, time.Hour)
	})
}

func _ratelimitPruneList(l *list.List, ct time.Duration) bool {
	toRemove := []*list.Element{}
	for e := l.Front(); e != nil; e = e.Next() {
		t := e.Value.(time.Time)
		if time.Since(t) < ct {
			el := e
			toRemove = append(toRemove, el)
		}
	}
	for _, v := range toRemove {
		l.Remove(v)
	}
	return l.Len() == 0
}
