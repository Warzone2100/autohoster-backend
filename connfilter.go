package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/maxsupermanhd/lac/v2"
)

// approve approvespec reject ban
func joinCheck(inst *instance, ip string, name string, pubkey []byte, pubkeyB64 string) (jd joinDispatch, action joinCheckActionLevel, reason string) {
	jd.Issued = time.Now()
	jd.Messages = []string{}
	jd.AllowChat = true
	action = joinCheckActionLevelApprove
	contactmsg := "You can contact Autohoster administration to appeal or get additional information: https://wz2100-autohost.net/about#contact\\n\\n"

	// stage 1 adolf/spam protection
	if stringContainsSlices(name, tryCfgGetD(tryGetSliceStringGen("blacklist", "name"), []string{}, inst.cfgs...)) {
		ecode, err := DbLogAction("%d [adolfmeasures] Join name %s triggered adolf suppression system", inst.Id, name)
		if err != nil {
			inst.logger.Printf("Failed to log action in database: %s", err.Error())
		}
		return jd, joinCheckActionLevelBan, "You were banned from joining Autohoster.\\n" +
			"Ban reason: 4.1.7. Any manifestations of Nazism, nationalism, incitement " +
			"of interracial, interethnic, interfaith discord and hostility, " +
			"calls for the overthrow of the government by force.\\n\\n" + contactmsg +
			"Event ID: " + ecode
	}

	// stage 2 ban check
	var (
		account          *int
		banid            *int
		banissued        *time.Time
		banexpires       *time.Time
		banexpired       *bool
		banreason        *string
		forbids_joining  *bool
		forbids_playing  *bool
		forbids_chatting *bool
	)
	err := dbpool.QueryRow(context.Background(), `select 
	identities.account, bans.id, time_issued, time_expires, coalesce(time_expires < now(), 'false'), reason, forbids_joining, forbids_playing, forbids_chatting
from identities
left outer join bans on bans.identity = identities.id or bans.account = identities.account
where
	identities.hash = encode(sha256($1), 'hex')
order by time_expires desc
limit 1`, pubkey).Scan(&account, &banid, &banissued, &banexpires, &banexpired, &banreason, &forbids_joining, &forbids_playing, &forbids_chatting)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			inst.logger.Printf("Failed to request bans from database: %s", err.Error())
		}
	}
	if banid != nil {
		if banexpired != nil && !*banexpired {
			if *forbids_joining {
				banexpiresstr := "never"
				if banexpires != nil {
					banexpiresstr = (*banexpires).String()
				}
				return jd, joinCheckActionLevelReject, "You were banned from joining Autohoster.\\n" +
					"Ban reason: " + *banreason + "\\n\\n" + contactmsg +
					"Ban issued: " + (*banissued).String() + "\\n" +
					"Ban expires: " + banexpiresstr + "\\n" +
					"Event ID: M-" + strconv.Itoa(*banid)
			}
			if *forbids_chatting {
				jd.Messages = append(jd.Messages, "You are banned from chatting in this room (ban ID: M-"+strconv.Itoa(*banid)+")")
				jd.AllowChat = false
			}
			if *forbids_playing {
				jd.Messages = append(jd.Messages, "You are banned from participating in this game (ban ID: M-"+strconv.Itoa(*banid)+")")
				action = joinCheckActionLevelApproveSpec
			}
		}
	}

	// stage 3 isp check
	if account == nil && !tryCfgGetD(tryGetBoolGen("allowNonLinkedHide"), false, inst.cfgs...) {
		rsp, err := ISPchecker.Lookup(ip)
		if err != nil {
			inst.logger.Printf("Failed to lookup ISP: %s", err.Error())
		} else {
			isAsnBanned := checkASNbanned(rsp.ASN, inst.cfgs)
			if rsp.IsProxy || isAsnBanned {
				ecode, err := DbLogAction("%d [antiproxy] join attempt from %q did not pass isp checks: proxy %v asnban %v (ip was %v)", inst.Id, name, rsp.IsProxy, isAsnBanned, ip)
				if err != nil {
					inst.logger.Printf("Failed to log action in database: %s", err.Error())
				}
				return jd, joinCheckActionLevelReject, "You were rejected from joining Autohoster.\\n" +
					"Reason: 2.1.1. Disruption or other interference with the system with or without defined purpose.\\n\\n" +
					"If you believe it is a mistake, feel free to contact us: https://wz2100-autohost.net/about#contact\\n\\n" +
					"Please provide event ID: " + ecode + " with your request."
			}
		}
	}

	// stage 4 check room prefs
	allowNonLinkedJoin := tryCfgGetD(tryGetBoolGen("allowNonLinkedJoin"), true, inst.cfgs...)
	if !allowNonLinkedJoin {
		if account == nil {
			return jd, joinCheckActionLevelReject, "You can not join this game.\\n\\n" +
				"You must join with linked player identity. Link one at:\\n" +
				"https://wz2100-autohost.net/wzlinkcheck\\n\\n" +
				"Do not bother admins/moderators about this."
		}
	}
	allowNonLinkedPlay := tryCfgGetD(tryGetBoolGen("allowNonLinkedPlay"), true, inst.cfgs...)
	if !allowNonLinkedPlay {
		if account == nil {
			jd.Messages = append(jd.Messages, "You are not allowed to participate in this game due to being not registered")
			action = joinCheckActionLevelApproveSpec
		}
	}
	allowNonLinkedChat := tryCfgGetD(tryGetBoolGen("allowNonLinkedChat"), true, inst.cfgs...)
	if !allowNonLinkedChat {
		if account == nil {
			jd.Messages = append(jd.Messages, "You are not allowed to chat in this room due to being not registered")
			jd.Messages = append(jd.Messages, "Link your identity on https://wz2100-autohost.net/wzlinkcheck")
			jd.AllowChat = false
		}
	}

	// stage 5 rate limit checks
	asThrCnt := tryCfgGetD(tryGetIntGen("antiSpamThresholdCount"), 3, inst.cfgs...)
	asThrDur := tryCfgGetD(tryGetIntGen("antiSpamThresholdDuration"), 3*24, inst.cfgs...)
	if asThrCnt > 0 {
		rateLimitCounter := 0
		dbpool.QueryRow(context.Background(), `select
	count(g.id)
from games as g
join players as p on p.game = g.id
join identities as i on p.identity = i.id
left join accounts as a on i.account = a.id
where g.game_time < 60000 and g.time_started + $1::interval > now() and (i.pkey = $2 or a.id = coalesce($3, -1))`, fmt.Sprintf("%d hours", asThrDur), pubkey, account).Scan(&rateLimitCounter)
		if rateLimitCounter >= asThrCnt {
			if action == joinCheckActionLevelApprove {
				jd.Messages = append(jd.Messages, "You were automatically rate limited for leaving the game early. Do not contact admins/moderators about this, they will not help you")
				action = joinCheckActionLevelApproveSpec
			}
		}
	}

	// stage 6 moved out check
	if joincheckWasMovedOutGlobal.present(pubkeyB64, inst.Id) {
		if action == joinCheckActionLevelApprove {
			jd.Messages = append(jd.Messages, "You not allowed to participate in the game because moderator moved you out earlier")
			action = joinCheckActionLevelApproveSpec
		}
	}

	// stage 7 ip based mute
	if account == nil {
		if checkIPmuted(inst, ip) {
			jd.AllowChat = false
		}
	}

	// stage 8 chat rate limit mute
	if account == nil {
		rlcDuration, rlcExceeded := ratelimitChatCheck(inst, ip)
		if rlcExceeded {
			jd.AllowChat = false
			jd.Messages = append(jd.Messages, "You were limited to quickchat due to spamming for "+rlcDuration.String())
		}
	}

	inst.logger.Printf("connfilter resolved key %v nljoin %v (acc %v) nlplay %v (action %v) nlchat %v (allowed %v)",
		pubkeyB64,
		allowNonLinkedJoin, account,
		allowNonLinkedPlay, action,
		allowNonLinkedChat, jd.AllowChat,
	)

	return jd, action, ""
}

func checkIPmuted(inst *instance, ip string) bool {
	clip := net.ParseIP(ip)
	if clip == nil {
		inst.logger.Printf("ipmute invalid ip %q", ip)
		return false
	}
	ipmutes := map[string]bool{}
	for i := len(inst.cfgs) - 1; i >= 0; i-- {
		o, ok := inst.cfgs[i].GetKeys("ipmute")
		if !ok {
			continue
		}
		for _, k := range o {
			s, ok := inst.cfgs[i].GetBool("ipmute", k)
			if !ok {
				continue
			}
			if !s {
				delete(ipmutes, k)
			} else {
				ipmutes[k] = s
			}
		}
	}
	for kip, v := range ipmutes {
		if !v {
			continue
		}
		_, pnt, err := net.ParseCIDR(kip)
		if err != nil {
			inst.logger.Printf("ipmute ip %q is not in CIDR notation: %s", kip, err)
			continue
		}
		if pnt == nil {
			inst.logger.Printf("ipmute ip %q has no network after parsing", kip)
			continue
		}
		if pnt.Contains(clip) {
			inst.logger.Printf("ipmute applied to client %q with rule %q", ip, kip)
			return true
		}
	}
	return false
}

type joinCheckActionLevel int

const (
	joinCheckActionLevelApprove = iota
	joinCheckActionLevelApproveSpec
	joinCheckActionLevelReject
	joinCheckActionLevelBan
)

func (l joinCheckActionLevel) String() string {
	switch l {
	case joinCheckActionLevelApprove:
		return "joinCheckActionLevelApprove"
	case joinCheckActionLevelApproveSpec:
		return "joinCheckActionLevelApproveSpec"
	case joinCheckActionLevelReject:
		return "joinCheckActionLevelReject"
	case joinCheckActionLevelBan:
		return "joinCheckActionLevelBan"
	default:
		return "unknown?!"
	}
}

type joincheckWasMovedOut struct {
	m    map[string][]int64
	lock sync.Mutex
}

var joincheckWasMovedOutGlobal = joincheckWasMovedOut{
	m:    map[string][]int64{},
	lock: sync.Mutex{},
}

func (j *joincheckWasMovedOut) _cleanup() {
	keys := make([]string, 0, len(j.m))
	for k := range j.m {
		keys = append(keys, k)
	}

	for _, k := range keys {
		v := j.m[k]
		nv := slices.DeleteFunc(v, func(vv int64) bool {
			return !isInstanceInLobby(vv)
		})
		if len(nv) == 0 {
			delete(j.m, k)
			continue
		}
		j.m[k] = nv
	}
}

func (j *joincheckWasMovedOut) add(identity string, instance int64) {
	j.lock.Lock()
	defer j.lock.Unlock()

	j._cleanup()

	r, ok := j.m[identity]
	if ok {
		j.m[identity] = append(r, instance)
		return
	}
	j.m[identity] = []int64{instance}
}

func (j *joincheckWasMovedOut) remove(identity string, instance int64) {
	j.lock.Lock()
	defer j.lock.Unlock()

	j._cleanup()

	r, ok := j.m[identity]
	if ok {
		if len(r) == 1 {
			if r[0] == instance {
				delete(j.m, identity)
			}
		} else {
			j.m[identity] = slices.DeleteFunc(r, func(rr int64) bool {
				return rr == instance
			})
		}
	}
}

func (j *joincheckWasMovedOut) present(identity string, instance int64) bool {
	j.lock.Lock()
	defer j.lock.Unlock()

	j._cleanup()

	r, ok := j.m[identity]
	if !ok {
		return false
	}
	return slices.Contains(r, instance)
}

func checkASNbanned(asn string, cfgs []lac.Conf) bool {
	for _, c := range cfgs {
		sl, ok := c.GetSliceString("bannedASNs")
		if ok {
			if stringContainsSlices(asn, sl) {
				return true
			}
		}
	}
	return false
}

func pubkeyDiscovery(pubkey []byte) {
	tag, err := dbpool.Exec(context.Background(), `update identities set pkey = $1 where hash = encode(sha256($1), 'hex') and pkey is null`, pubkey)
	if err != nil {
		log.Printf("Key discovery query failed: %s", err.Error())
		return
	}
	if !tag.Update() || tag.RowsAffected() > 1 {
		log.Printf("Something went horribly wrong in key discovery, tag: %s", tag)
	}
}
