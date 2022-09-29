// main is main is main
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
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
	// ballotSource = `https://api.filpoll.io/api/polls/16/view-votes`
	ballotSource = `https://w3s.link/ipfs/bafybeietprvjsf47sqs2gh7bfkanjbf3nig56jibqfgrijjqxiirgmg3we/fil_fip36_poll_ballots_obtained_morning_of_2022-09-29.json`
	dbFn         = `data/filstate_2162760.sqlite`
)

func main() {
	ctx := context.Background()

	if err := updateVotesInDB(ctx, dbFn, ballotSource); err != nil {
		log.Fatalf("%+v", err)
	}
}

func updateVotesInDB(ctx context.Context, dbFn string, ballotSrc string) error {

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
			actor_id INTEGER NOT NULL UNIQUE,
			does_accept BOOL NOT NULL,
			vote_received DATETIME NULL
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

	var ballotRdr io.ReadCloser
	defer func() {
		if ballotRdr != nil {
			ballotRdr.Close() //nolint:errcheck
		}
	}()

	if strings.HasPrefix(ballotSrc, "http://") || strings.HasPrefix(ballotSrc, "https://") {
		req, err := http.NewRequestWithContext(ctx, "GET", ballotSrc, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusOK {
			return xerrors.Errorf("non-200 response: %d", resp.StatusCode)
		}

		ballotRdr = resp.Body
	} else {
		ballotRdr, err = os.Open(ballotSrc)
		if err != nil {
			return xerrors.Errorf("unable to open %s as plain file: %w", ballotSrc, err)
		}
	}

	ballots := make([]ballot, 0, 1<<12)
	if err := json.NewDecoder(ballotRdr).Decode(&ballots); err != nil {
		return xerrors.Errorf("unexpected error parsing data json %s: %w", ballotSrc, err)
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
			( actor_id, does_accept, vote_received )
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

	// give all the msigs a "vote" as well, based on their having-voted parts
	// do it recursively, because why not :)
	for {
		res, err := db.Exec(
			`
			INSERT INTO votes
				( actor_id, does_accept )
			SELECT ma.msig_id, v.does_accept
				FROM votes v
				JOIN msig_actors ma USING ( actor_id )
				JOIN msigs m USING ( msig_id )
			WHERE
				ma.msig_id NOT IN ( SELECT actor_id FROM votes )
					AND
				ma.msig_id != 1858410 -- ignore the Fil+ LDN msig
			GROUP BY ma.msig_id, m.threshold, v.does_accept
			HAVING COUNT(*) >= m.threshold
			`,
		)
		if err != nil {
			return err
		}

		ra, err := res.RowsAffected()
		if err != nil {
			return err
		}

		if ra == 0 {
			break
		}
	}

	// now that we have all the signing actors vote: add the SPs as actors on their own too
	// When there is a conflict, owner trumps worker
	// https://filecoinproject.slack.com/archives/C01EU76LPCJ/p1663721692909119
	if _, err := db.Exec(
		`
		WITH sp_votes AS (
			SELECT
					p.provider_id,
					COALESCE(
						( SELECT does_accept FROM votes v WHERE v.actor_id = p.owner_id ),
						( SELECT does_accept FROM votes v WHERE v.actor_id = p.worker_id )
					) AS does_accept
				FROM providers p
			)
		INSERT INTO votes
			( actor_id, does_accept )
		SELECT provider_id, does_accept FROM sp_votes WHERE does_accept IS NOT NULL
		`,
	); err != nil {
		return err
	}

	log.Printf("Processed %d ACCEPT and %d REJECT votes\n", acceptCount, rejectCount)

	type prelimRes struct {
		Type       string
		Weight     float64
		DoesAccept *bool
	}

	pr := make([]prelimRes, 0, 8)

	log.Println("Calculating preliminary results ( takes about a minute )")

	if err := sqlscan.Select(
		ctx,
		db,
		&pr,
		`
		SELECT "BalancesNfil" type, SUM(bal) weight, does_accept FROM (
			SELECT SUM( CAST( balance AS DOUBLE ) / 1000000000 ) bal, does_accept
				FROM providers p
				LEFT JOIN votes v ON p.provider_id = v.actor_id
			GROUP BY does_accept

				UNION ALL

			SELECT SUM( CAST( balance AS DOUBLE ) / 1000000000 ) bal, does_accept
				FROM accounts a
				LEFT JOIN votes v ON a.account_id = v.actor_id
			GROUP BY does_accept

				UNION ALL

			SELECT SUM( CAST( balance AS DOUBLE ) / 1000000000 ) bal, does_accept
				FROM msigs m
				LEFT JOIN votes v ON m.msig_id = v.actor_id
			GROUP BY does_accept
		) GROUP BY does_accept

			UNION ALL

		SELECT "DealBytesProvider" type, SUM( piece_size ) weight, does_accept
			FROM deals d
			LEFT JOIN votes v ON d.provider_id = v.actor_id
		WHERE
			d.sector_activation_epoch IS NOT NULL
				AND
			d.deal_slash_epoch IS NULL
				AND
			d.end_epoch > 2162760
		GROUP BY does_accept

			UNION ALL

		SELECT "DealBytesClient" type, SUM( piece_size ) weight, does_accept
			FROM deals d
			LEFT JOIN votes v ON d.client_id = v.actor_id
		WHERE
			d.sector_activation_epoch IS NOT NULL
				AND
			d.deal_slash_epoch IS NULL
				AND
			d.end_epoch > 2162760
		GROUP BY does_accept

			UNION ALL

		SELECT "SpRawBytesMiB" type, SUM( CAST( power_raw AS BIGINT ) >> 20 ) weight, does_accept
			FROM providers p
			LEFT JOIN votes v ON p.provider_id = v.actor_id
		GROUP BY does_accept
		`,
	); err != nil {
		return err
	}

	type tslice struct {
		didVote    bool
		doesAccept bool
	}

	prelimTally := make(map[string]map[tslice]float64, 4)
	for _, p := range pr {

		t, seen := prelimTally[p.Type]
		if !seen {
			t = make(map[tslice]float64, 3)
			prelimTally[p.Type] = t
		}

		ts := tslice{
			didVote: (p.DoesAccept != nil),
		}
		if ts.didVote {
			ts.doesAccept = *p.DoesAccept
		}

		t[ts] = p.Weight
	}

	for g, t := range prelimTally {
		tot := t[tslice{didVote: false}] + t[tslice{didVote: true, doesAccept: false}] + t[tslice{didVote: true, doesAccept: true}]
		totVoted := t[tslice{didVote: true, doesAccept: false}] + t[tslice{didVote: true, doesAccept: true}]

		log.Printf(`

  Group: %s
Abstain: % 3.1f%% % 20.0f
    Yea: % 3.1f%% % 20.0f
    Nay: % 3.1f%% % 20.0f

`,
			g,
			100*t[tslice{didVote: false}]/tot, t[tslice{didVote: false}],
			100*t[tslice{didVote: true, doesAccept: true}]/totVoted, t[tslice{didVote: true, doesAccept: true}],
			100*t[tslice{didVote: true, doesAccept: false}]/totVoted, t[tslice{didVote: true, doesAccept: false}],
		)
	}

	return nil
}
