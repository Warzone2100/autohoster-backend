package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type hosterMessageMatchType int

const (
	hosterMessageMatchTypeExact hosterMessageMatchType = iota
	hosterMessageMatchTypePrefix
	hosterMessageMatchTypeSuffix
	hosterMessageMatchTypePrefixSuffix
)

type hosterMessageHandlerFunc func(inst *instance, msg string) bool

type hosterMessageHandler struct {
	match   hosterMessageMatchType
	mExact  string
	mPrefix string
	mSuffix string
	fn      hosterMessageHandlerFunc
}

var (
	hosterMessageHandlers = []hosterMessageHandler{{
		match:  hosterMessageMatchTypeExact,
		mExact: "WZCMD: stdinReadReady",
		fn: func(inst *instance, msg string) bool {
			inst.logger.Println("ready to input data")
			instWriteFmt(inst, `set chat quickchat newjoin`)
			instWriteFmt(inst, `set chat quickchat all`)
			instWriteFmt(inst, `set chat allow host`)
			for _, v := range inst.Admins {
				instWriteFmt(inst, `admin add-hash %s`, v)
			}
			return false
		},
	}, {
		match:  hosterMessageMatchTypeExact,
		mExact: "WZEVENT: startMultiplayerGame",
		fn: func(inst *instance, msg string) bool {
			inst.logger.Println("game starting")
			inst.logger.Printf("atomic state swap from %d to %d", int64(instanceStateInLobby), int64(instanceStateInGame))
			if !inst.state.CompareAndSwap(int64(instanceStateInLobby), int64(instanceStateInGame)) {
				inst.logger.Printf("atomic swap failed!")
			}
			// inst.state.Store(int64(instanceStateInGame))
			err := recoverSave(inst)
			if err != nil {
				inst.logger.Printf("Failed to save instance recovery json: %s", err.Error())
			}
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZEVENT: lobbyid: ",
		fn: func(inst *instance, msg string) bool {
			i, err := fmt.Sscanf(msg, "WZEVENT: lobbyid: %d", &inst.LobbyId)
			if err != nil || i != 1 {
				inst.logger.Printf("Failed to parse lobbyid message: %v", err)
				return true
			}
			inst.logger.Printf("atomic state store: %d", int64(instanceStateInLobby))
			inst.state.Store(int64(instanceStateInLobby))
			err = recoverSave(inst)
			if err != nil {
				inst.logger.Printf("Failed to save instance recovery json: %s", err.Error())
			}
			inst.logger.Printf("lobbyid %d", inst.LobbyId)
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZCHATGAM: ",
		fn:      messageHandlerProcessChat,
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZCHATCMD: ",
		fn:      messageHandlerProcessChat,
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZCHATLOB: ",
		fn:      messageHandlerProcessChat,
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZEVENT: join approval needed: ",
		fn: func(inst *instance, msg string) bool {
			// WZEVENT: join approval needed: <joinid> <ip> <hash> <b64pubkey> <b64name> [spec|play]
			var msgjoinid, msgip, msghash, msgb64name, msgb64pubkey, msgjointype string
			i, err := fmt.Sscanf(msg, "WZEVENT: join approval needed: %s %s %s %s %s %s", &msgjoinid, &msgip, &msghash, &msgb64pubkey, &msgb64name, &msgjointype)
			if err != nil || i != 6 {
				inst.logger.Printf("Failed to parse join approval message: %v", err)
				return true
			}
			var msgname, msgpubkey []byte
			err = base64DecodeFields(
				msgb64name, &msgname,
				msgb64pubkey, &msgpubkey,
			)
			if err != nil {
				inst.logger.Printf("Failed to decode base64 arguments: %s", err.Error())
				return true
			}
			pubkeyDiscovery(msgpubkey)
			jd, action, reason := joinCheck(inst, msgip, string(msgname), msgpubkey, msgb64pubkey)
			addChatLog(msgip, string(msgname), msgpubkey, "", "joinattempt")
			switch action {

			case joinCheckActionLevelApprove:
				inst.logger.Printf("Action approve for %q %q", msgip, msgname)
				instWriteFmt(inst, "join approve "+msgjoinid+" 7 "+reason)
				inst.OnJoinDispatch[msgb64pubkey] = jd

			case joinCheckActionLevelApproveSpec:
				inst.logger.Printf("Action approvespec for %q %q", msgip, msgname)
				instWriteFmt(inst, "join approvespec "+msgjoinid+" 7 "+reason)
				inst.OnJoinDispatch[msgb64pubkey] = jd

			case joinCheckActionLevelReject:
				inst.logger.Printf("Action reject for %q %q", msgip, msgname)
				instWriteFmt(inst, "join reject "+msgjoinid+" 7 "+reason)

			case joinCheckActionLevelBan:
				inst.logger.Printf("Action ban for %q %q", msgip, msgname)
				instWriteFmt(inst, "join reject "+msgjoinid+" 7 "+reason)
				instWriteFmt(inst, "ban ip "+msgip)
			}
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZEVENT: player join: ",
		fn: func(inst *instance, msg string) bool {
			var msgjoinid, msgb64pubkey string
			i, err := fmt.Sscanf(msg, "WZEVENT: player join: %s %s", &msgjoinid, &msgb64pubkey)
			if err != nil || i != 2 {
				inst.logger.Printf("Failed to parse join message: %v", err)
				return true
			}
			messageHandlerProcessIdentityJoin(inst, msgb64pubkey)
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZEVENT: player identity VERIFIED: ",
		fn: func(inst *instance, msg string) bool {
			var msgjoinid, msgb64pubkey string
			i, err := fmt.Sscanf(msg, "WZEVENT: player identity VERIFIED: %s %s", &msgjoinid, &msgb64pubkey)
			if err != nil || i != 2 {
				inst.logger.Printf("Failed to parse identity verified message: %v", err)
				return true
			}
			messageHandlerProcessIdentityJoin(inst, msgb64pubkey)
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZEVENT: movedPlayerToSpec: ",
		fn: func(inst *instance, msg string) bool {
			// WZEVENT: movedPlayerToSpec: 5 -> 16 WMy35N3raEEEgOt3stR62BRvP8E8osfNSiMZOCw5SqU= 0e308437d20db97d110ceecc448d39517b47deb7cbb7fe0338f4308f2619d483 V Qm9kbWluIEJlYXN0
			var msgplidfrom, msgplidto int
			var msgb64pubkey, msghash, msgverified, msgb64name, msgip string
			i, err := fmt.Sscanf(msg, "WZEVENT: movedPlayerToSpec: %d -> %d %s %s %s %s %s",
				&msgplidfrom, &msgplidto, &msgb64pubkey, &msghash, &msgverified, &msgb64name, &msgip)
			if err != nil || i != 7 {
				inst.logger.Printf("Failed to parse event movedPlayerToSpec: %v", err)
				return true
			}
			joincheckWasMovedOutGlobal.add(msgb64pubkey, inst.Id)
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZEVENT: movedSpecToPlayer: ",
		fn: func(inst *instance, msg string) bool {
			// 1724192419 : "WZEVENT: movedSpecToPlayer: 14 -> 0 iYTMPZSZd4bB8Ni750QQmynGug+wcD2Baxj9gclB9w0= 0bfb8797cbc6567b4248bfa29b6f3220929870e100814f0ede761626e7dc8e97 V YnlfbW9jYXJ0IFtSVV0=
			var msgplidfrom, msgplidto int
			var msgb64pubkey, msghash, msgverified, msgb64name, msgip string
			i, err := fmt.Sscanf(msg, "WZEVENT: movedSpecToPlayer: %d -> %d %s %s %s %s %s",
				&msgplidfrom, &msgplidto, &msgb64pubkey, &msghash, &msgverified, &msgb64name, &msgip)
			if err != nil || i != 7 {
				inst.logger.Printf("Failed to parse event movedSpecToPlayer: %v", err)
				return true
			}
			joincheckWasMovedOutGlobal.remove(msgb64pubkey, inst.Id)
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefixSuffix,
		mPrefix: "__REPORT__",
		mSuffix: "__ENDREPORT__",
		fn: func(inst *instance, msg string) bool {
			st := inst.state.Load()
			if instanceState(st) != instanceStateInGame {
				inst.logger.Println("report dropped with non in-game state!")
				return true
			}
			reportContent := []byte(msg[10 : len(msg)-13])
			inst.logger.Printf("report (len %d) (gid %d)", len(reportContent), inst.GameId)
			if tryCfgGetD(tryGetBoolGen("submitGames"), true, inst.cfgs...) {
				submitReport(inst, reportContent)
			}
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefixSuffix,
		mPrefix: "__REPORTextended__",
		mSuffix: "__ENDREPORTextended__",
		fn: func(inst *instance, msg string) bool {
			st := inst.state.Load()
			if instanceState(st) != instanceStateInGame {
				inst.logger.Println("report dropped with non in-game state!")
				return true
			}
			reportContent := []byte(msg[18 : len(msg)-21])
			inst.logger.Printf("report (len %d) (gid %d)", len(reportContent), inst.GameId)
			if tryCfgGetD(tryGetBoolGen("submitGames"), true, inst.cfgs...) {
				submitFinalReport(inst, reportContent)
			}
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "__DEBUGMODE__",
		fn: func(inst *instance, msg string) bool {
			st := inst.state.Load()
			if instanceState(st) != instanceStateInGame {
				inst.logger.Println("debugmode dropped with non in-game state!")
			}
			inst.DebugTriggered = true
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: " * Version: ",
		fn: func(inst *instance, msg string) bool {
			// * Version: master 846187e, Built:
			// * Version: 4.5.0-beta1, (modified locally) Built: 2024-06-23
			msg = strings.TrimPrefix(msg, " * Version: ")
			spl := strings.Split(msg, " Built:")
			if len(spl) < 2 {
				inst.logger.Printf("Weird split on version detect, len %d", len(spl))
				return true
			}
			inst.AutodetectedVersion = strings.TrimSuffix(strings.TrimSuffix(spl[0], ","), ", (modified locally)")
			inst.logger.Printf("Autodetected version %q", inst.AutodetectedVersion)
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZCMD info: Room admin hash added",
		fn: func(inst *instance, msg string) bool {
			return false
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZEVENT: lobbyerror",
		fn: func(inst *instance, msg string) bool {
			inst.logger.Println("Instance was kicked out of the lobby, shutting it down")
			inst.commands <- instanceCommand{command: icShutdown}
			return true
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "WZCMD error: ",
		fn: func(inst *instance, msg string) bool {
			discordPostError("instance `%d` spewed a WZCMD error: %q", inst.Id, msg)
			return true
		},
	}, {
		match:   hosterMessageMatchTypePrefix,
		mPrefix: "error   |",
		fn: func(inst *instance, msg string) bool {
			discordPostError("instance `%d` spewed a regular error: %q", inst.Id, msg)
			return true
		},
	}}
)

func messageHandlerProcessIdentityJoin(inst *instance, msgb64pubkey string) {
	motds := map[string]any{}
	for i := len(inst.cfgs) - 1; i >= 0; i-- {
		o, ok := inst.cfgs[i].GetKeys("motds")
		if !ok {
			continue
		}
		for _, k := range o {
			s, ok := inst.cfgs[i].GetString("motds", k)
			if ok {
				if s == "" {
					delete(motds, k)
				} else {
					motds[k] = s
				}
			}
		}
	}
	keys := make([]string, 0, len(motds))
	for k := range motds {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		instWriteFmt(inst, `chat direct %s %s`, msgb64pubkey, motds[k])
	}
	instWriteFmt(inst, `chat direct %s This game has time limit of %d minutes.`, msgb64pubkey, inst.Settings.TimeLimit)
	d, ok := inst.OnJoinDispatch[msgb64pubkey]
	if ok {
		if d.AllowChat {
			inst.logger.Printf("allowing chat for %s", msgb64pubkey)
			instWriteFmt(inst, `set chat allow %s`, msgb64pubkey)
		}
		for _, v := range d.Messages {
			instWriteFmt(inst, `chat direct %s %s`, msgb64pubkey, v)
		}
		delete(inst.OnJoinDispatch, msgb64pubkey)
	}
	toDeleteKeys := []string{}
	for k, v := range inst.OnJoinDispatch {
		if time.Since(v.Issued) > 15*time.Second {
			toDeleteKeys = append(toDeleteKeys, k)
		}
	}
	for _, v := range toDeleteKeys {
		delete(inst.OnJoinDispatch, v)
	}
}

func messageHandlerProcessChat(inst *instance, msg string) bool {
	// WZCHATGAM: <index> <ip> <hash> <b64pubkey> dmF1dCDOo86RIFtHTl0= dmF1dCDOo86RIFtHTl0gKEdsb2JhbCk6IGdn V
	// WZCHATCMD: <index> <ip> <hash> <b64pubkey> <b64name> <b64msg>
	origmsg := msg
	msgtype := "invalid"
	if strings.HasPrefix(msg, "WZCHATCMD: ") {
		msgtype = "WZCHATCMD"
	} else if strings.HasPrefix(msg, "WZCHATLOB: ") {
		msgtype = "WZCHATLOB"
	} else if strings.HasPrefix(msg, "WZCHATGAM: ") {
		msgtype = "WZCHATGAM"
	}
	msg = strings.TrimPrefix(msg, "WZCHATCMD: ")
	msg = strings.TrimPrefix(msg, "WZCHATLOB: ")
	msg = strings.TrimPrefix(msg, "WZCHATGAM: ")
	var msgindex, msgip, msghash, msgb64pubkey, msgb64name, msgb64content string
	i, err := fmt.Sscanf(msg, "%s %s %s %s %s %s", &msgindex, &msgip, &msghash, &msgb64pubkey, &msgb64name, &msgb64content)
	if err != nil || i != 6 {
		inst.logger.Printf("Failed to parse chat message: %v", err)
		return true
	}
	var msgname, msgpubkey, msgcontent []byte
	err = base64DecodeFields(
		msgb64name, &msgname,
		msgb64pubkey, &msgpubkey,
		msgb64content, &msgcontent,
	)
	if err != nil {
		inst.logger.Printf("Failed to decode base64 wzcmd parameter: %s", err.Error())
		return true
	}
	if stringContainsSlices(string(msgname), tryCfgGetD(tryGetSliceStringGen("blacklist", "name"), []string{}, inst.cfgs...)) ||
		stringContainsSlices(string(msgcontent), tryCfgGetD(tryGetSliceStringGen("blacklist", "message"), []string{}, inst.cfgs...)) {
		ecode, err := DbLogAction("%d [adolfmeasures] Message from %q triggered adolf suppression system (message was %q)", inst.Id, msgb64name, msgb64content)
		if err != nil {
			inst.logger.Printf("Failed to log action in database: %s", err.Error())
		}
		reason := fmt.Sprintf("You were banned from joining Autohoster.\\n"+
			"Ban reason: 4.1.7. Any manifestations of Nazism, nationalism, incitement of interracial, interethnic, interfaith discord and hostility, calls for the overthrow of the government by force.\\n\\n"+
			"Event ID: %s", ecode)
		instWriteFmt(inst, "ban ip %s %s", msgip, reason)
	}
	err = addChatLog(msgip, string(msgname), msgpubkey, string(msgcontent), msgtype)
	if err != nil {
		inst.logger.Printf("Failed to log chat of instance `%d`: %s (%q: %q), was fed %q", inst.Id, err.Error(), string(msgname), string(msgcontent), origmsg)
		discordPostError("Failed to log chat of instance `%d`: %s (%q: %q), was fed %q", inst.Id, err.Error(), string(msgname), string(msgcontent), origmsg)
	}
	if msgtype == "WZCHATCMD" && (string(msgcontent) == "/stat" || string(msgcontent) == "/stats") {
		instWriteFmt(inst, `chat direct %s %s`, msgb64pubkey, "All Autohoster's games are available at the website: https://wz2100-autohost.net/games (with detailed dtatistics, charts and replay)")
	}
	if msgtype == "WZCHATCMD" && (strings.HasPrefix(string(msgcontent), "/votekick")) {
		instWriteFmt(inst, `chat direct %s %s`, msgb64pubkey, "If you would like to become a part of Autohoster moderation team, feel free to contact us: https://wz2100-autohost.net/about#contact")
	}
	return false
}

func addChatLog(ip string, name string, pkey []byte, msg string, msgtype string) error {
	tag, err := dbpool.Exec(context.Background(), `INSERT INTO chatlog (ip, name, pkey, msg, msgtype) VALUES ($1, $2, $3, $4, $5)`, ip, name, pkey, msg, msgtype)
	if err != nil {
		return err
	}
	if !tag.Insert() {
		return errors.New("not insert return tag")
	}
	if tag.RowsAffected() != 1 {
		return errors.New("rows affected != 1")
	}
	return nil
}

func processHosterMessage(inst *instance, msg string) bool {
	for _, v := range hosterMessageHandlers {
		switch v.match {
		case hosterMessageMatchTypeExact:
			if v.mExact == "" {
				inst.logger.Printf("Message handler %+#v has empty exact match string", v)
			}
			if v.mExact == msg {
				return v.fn(inst, msg)
			}
		case hosterMessageMatchTypePrefix:
			if v.mPrefix == "" {
				inst.logger.Printf("Message handler %+#v has empty prefix match string", v)
			}
			if strings.HasPrefix(msg, v.mPrefix) {
				return v.fn(inst, msg)
			}
		case hosterMessageMatchTypeSuffix:
			if v.mSuffix == "" {
				inst.logger.Printf("Message handler %+#v has empty suffix match string", v)
			}
			if strings.HasSuffix(msg, v.mSuffix) {
				return v.fn(inst, msg)
			}
		case hosterMessageMatchTypePrefixSuffix:
			if v.mPrefix == "" {
				inst.logger.Printf("Message handler %+#v has empty prefix match string", v)
			}
			if v.mSuffix == "" {
				inst.logger.Printf("Message handler %+#v has empty suffix match string", v)
			}
			if strings.HasPrefix(msg, v.mPrefix) && strings.HasSuffix(msg, v.mSuffix) {
				return v.fn(inst, msg)
			}
		}
	}
	return true
}
