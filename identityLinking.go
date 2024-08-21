package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

var (
	linkingMessageMatchRegex = regexp.MustCompile(`^/hostmsg confirm-[0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ]{18}$`)
)

func handleIdentityLinkMessage(inst *instance, msgb64pubkey string, msgpubkey []byte, name string, msg string, isVerified bool) error {
	if !linkingMessageMatchRegex.MatchString(msg) {
		return nil
	}
	if !isVerified {
		instWriteFmt(inst, `chat direct %s You have sent identity action confirmation message but host did not yet confirm your identity, please send it again in couple seconds.`, msgb64pubkey)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	submatches := linkingMessageMatchRegex.FindStringSubmatch(msg)
	if len(submatches) != 2 {
		return fmt.Errorf("len(submatches) != 2 (%+#v)", submatches)
	}
	msgcode := submatches[1]

	var err error
	var tag pgconn.CommandTag

	tx, err := dbpool.Begin(ctx)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `LOCK TABLE accounts IN ACCESS EXCLUSIVE MODE`)
	if err != nil {
		inst.logger.Println("Failed to grab accounts lock")
		return err
	}
	_, err = tx.Exec(ctx, `LOCK TABLE identities IN ACCESS EXCLUSIVE MODE`)
	if err != nil {
		inst.logger.Println("Failed to grab identities lock")
		return err
	}
	_, err = tx.Exec(ctx, `LOCK TABLE players IN EXCLUSIVE MODE`)
	if err != nil {
		inst.logger.Println("Failed to grab players lock")
		return err
	}

	var codeAccount int
	err = tx.QueryRow(ctx, `SELECT id FROM accounts WHERE wz_confirm_code = $1`, msgcode).Scan(&codeAccount)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			inst.logger.Println("failed to query accounts")
			tx.Rollback(ctx)
			return err
		}
		instWriteFmt(inst, `chat direct %s This confirm code is not found.`, msgb64pubkey)
		return tx.Rollback(ctx)
	}

	var accountIdentCount int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM identities WHERE account = $1`, codeAccount).Scan(&accountIdentCount)
	if err != nil {
		inst.logger.Println("Failed to count account identities")
		tx.Rollback(ctx)
		return err
	}

	identFound := true
	var identAccount *int
	var identID int
	err = tx.QueryRow(ctx, `SELECT id, account FROM identities WHERE hash = encode(sha256($1), 'hex')`, msgpubkey).Scan(&identID, &identAccount)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			inst.logger.Println("failed to query identities")
			tx.Rollback(ctx)
			return err
		}
		identFound = false
	}
	if identAccount != nil {
		instWriteFmt(inst, `chat direct %s This identity is already claimed.`, msgb64pubkey)
		return tx.Rollback(ctx)
	}

	if identFound && accountIdentCount != 0 {
		var playedCount int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM players WHERE identity = $1`, identID).Scan(&playedCount)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}
		if playedCount > 0 {
			instWriteFmt(inst, `chat direct %s Only identities with 0 played games can be linked after first linked identity.`, msgb64pubkey)
			tx.Rollback(ctx)
			return nil
		}
	}

	tag, err = tx.Exec(context.Background(), `
		insert into identities (name, pkey, hash, account)
		values ($1, $2, encode(sha256($2), 'hex'), $3)
		on conflict (hash) do update set account = $3 where identities.account is null and identities.pkey = $2`,
		name, msgpubkey, codeAccount)
	if err != nil {
		tx.Rollback(ctx)
		return err
	}
	if tag.Update() && tag.RowsAffected() == 0 {
		instWriteFmt(inst, `chat direct %s This identity is already claimed.`, msgb64pubkey)
		tx.Rollback(ctx)
		return nil
	}
	_, err = dbpool.Exec(context.Background(), `update accounts set wz_confirm_code = null, display_name = $1 where id = $2`, name, codeAccount)
	if err != nil {
		tx.Rollback(ctx)
		return err
	}
	err = tx.Commit(ctx)
	if err != nil {
		return err
	}
	instWriteFmt(inst, `chat direct %s Identity successfully linked to the account.`, msgb64pubkey)
	return nil
}
