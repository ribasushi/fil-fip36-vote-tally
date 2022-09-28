// main is main is main
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	filaddr "github.com/filecoin-project/go-address"
	"github.com/georgysavva/scany/sqlscan"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/xerrors"
)

type ballot struct {
	OptionID      uint64
	SignerAddress filaddr.Address
	CreatedAt     time.Time
}

const (
	ballotSource = `https://api.filpoll.io/api/polls/16/view-votes`
	dbFn         = `data/filstate_2162760.sqlite`
)

func main() {
	ctx := context.Background()

	if err := updateVotesInDB(ctx, dbFn, ballotSource); err != nil {
		log.Fatalf("%+v", err)
	}
}

func updateVotesInDB(ctx context.Context, dbFn string, ballotURL string) error {

	db, err := sql.Open(
		"sqlite3", dbFn+"?"+strings.Join([]string{
			"mode=rw",
			"_foreign_keys=1",
			"_defer_foreign_keys=1",
			"_timeout=5000",
			"_vacuum=none",
			"_journal=memory",
			"_sync=off",
		}, "&"),
	)
	if err != nil {
		return xerrors.Errorf("failed to open state database %s: %s", dbFn, err)
	}

	if _, err := db.Exec(
		`
		CREATE TABLE IF NOT EXISTS votes (
			account_id INTEGER NOT NULL UNIQUE,
			does_accept BOOL NOT NULL,
			vote_received DATETIME NOT NULL
		)
		`,
	); err != nil {
		return err
	}

	acctIDs := make([]struct {
		AccountID      int
		AccountAddress filaddr.Address
	}, 0, 1<<12)
	if err := sqlscan.Select(
		ctx,
		db,
		&acctIDs,
		`SELECT account_id, account_address FROM accounts`,
	); err != nil {
		return err
	}
	acctLookup := make(map[filaddr.Address]int, len(acctIDs))
	for _, a := range acctIDs {
		acctLookup[a.AccountAddress] = a.AccountID
	}

	req, err := http.NewRequestWithContext(ctx, "GET", ballotURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return xerrors.Errorf("non-200 response: %d", resp.StatusCode)
	}

	ballots := make([]ballot, 0, 1<<12)
	if err := json.NewDecoder(resp.Body).Decode(&ballots); err != nil {
		return xerrors.Errorf("unexpected error parsing data json %s: %w", ballotURL, err)
	}

	type vote struct {
		doesAccept bool
		received   time.Time
	}

	votes := make(map[int]vote, 1<<12)

	// process ordered by time, first(?) cast wins
	sort.Slice(ballots, func(i, j int) bool {
		return ballots[i].CreatedAt.Before(ballots[j].CreatedAt)
	})
	for _, b := range ballots {
		acctID, found := acctLookup[b.SignerAddress]
		if !found {
			log.Printf("ignoring ballot %v: unknown robust address", b)
			continue
		}

		var doesAccept bool
		if b.OptionID == 49 {
			doesAccept = true
		} else if b.OptionID != 50 {
			log.Printf("ignoring ballot %v: unknown vote option", b)
		}

		if v, exists := votes[acctID]; exists {
			if v.doesAccept != doesAccept {
				log.Printf(
					"ignoring CONFLICTING ballot: ACCEPT:%t at %s but ACCEPT:%t at %s",
					v.doesAccept,
					v.received,
					doesAccept,
					b.CreatedAt,
				)

			}
			continue
		}

		votes[acctID] = vote{
			doesAccept: doesAccept,
			received:   b.CreatedAt,
		}
	}

	insertVote, err := db.Prepare(
		`
		INSERT INTO votes
			( account_id, does_accept, vote_received )
		VALUES ( $1, $2, $3 )
		`,
	)
	if err != nil {
		return err
	}

	if _, err = db.Exec(
		`DELETE FROM votes`,
	); err != nil {
		return err
	}

	var acceptCount, rejectCount int
	for a, v := range votes {
		if v.doesAccept {
			acceptCount++
		} else {
			rejectCount++
		}
		if _, err := insertVote.Exec(a, v.doesAccept, v.received); err != nil {
			return err
		}
	}

	log.Printf("Processed %d ACCEPT and %d REJECT votes\n", acceptCount, rejectCount)

	return nil
}
