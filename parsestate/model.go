package main

import (
	"database/sql"
	"encoding/binary"
	"os"
	"strings"

	"golang.org/x/xerrors"
)

type procID int
type procDictionary map[procID]*sql.Stmt

const (
	procAddDeal = procID(iota)
	procAddProvider
	procAddAccount
	procAddMsig
	procAddMsigActors
)

func prepDb(workDir string) (procDictionary, func(string) error, error) {

	tmpFile, err := os.CreateTemp(workDir, `.filstate_db_*`)
	if err != nil {
		return nil, func(string) error { return nil }, xerrors.Errorf("unable to create temporary sqlite file: %w", err)
	}

	var db *sql.DB
	fin := func(p string) error {
		defer os.Remove(tmpFile.Name()) //nolint:errcheck

		if db == nil || p == "" {
			return nil
		}

		if _, err := db.Exec("VACUUM"); err != nil {
			return xerrors.Errorf("vacuum at finalize failed: %w", err)
		}
		if err := db.Close(); err != nil {
			return xerrors.Errorf("failure flushing DB at close(): %w", err)
		}

		// Surgery on the database file header itself, making it reproducible
		//
		{
			// https://www.sqlite.org/fileformat.html#file_change_counter
			// https://www.sqlite.org/fileformat.html#schema_cookie
			// https://www.sqlite.org/fileformat.html#validfor
			for _, offset := range []int64{24, 40, 92} {
				if _, err := tmpFile.WriteAt([]byte{0, 0, 0, 1}, offset); err != nil {
					return xerrors.Errorf("db header correction failed: %w", err)
				}
			}

			// https://www.sqlite.org/fileformat.html#write_library_version_number_and_version_valid_for_number
			// freeze "last SQLite access" version at reasonable minimum
			if _, err := tmpFile.WriteAt(
				binary.BigEndian.AppendUint32(make([]byte, 0, 4), 3022000),
				96,
			); err != nil {
				return xerrors.Errorf("db header correction failed: %w", err)
			}
		}
		// end surgery

		return os.Rename(tmpFile.Name(), p)
	}

	db, err = sql.Open(
		"sqlite3", tmpFile.Name()+"?"+strings.Join([]string{
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
		return nil, fin, xerrors.Errorf("failed to open database in temporary file: %w", err)
	}

	for _, s := range []string{
		`
		CREATE TABLE deals (
			deal_id BIGINT NOT NULL UNIQUE,
			client_id INTEGER NOT NULL,
			provider_id INTEGER NOT NULL,
			piece_cid TEXT NOT NULL,
			label TEXT NOT NULL,
			piece_size BIGINT NOT NULL,
			is_filplus BOOLEAN NOT NULL,
			price_per_epoch BIGINT NOT NULL,
			provider_collateral BIGINT NOT NULL,
			client_collateral BIGINT NOT NULL,
			start_epoch INTEGER NOT NULL,
			end_epoch INTEGER NOT NULL,
			sector_activation_epoch INTEGER,
			deal_slash_epoch INTEGER
		)`,
		`
		CREATE TABLE providers (
			provider_id INTEGER NOT NULL UNIQUE,
			owner_id INTEGER NOT NULL,
			worker_id INTEGER NOT NULL,
			power_raw TEXT NOT NULL,
			power_qa TEXT NOT NULL
		)
		`,
		`
		CREATE TABLE accounts (
			account_id INTEGER NOT NULL UNIQUE,
			account_address TEXT NOT NULL UNIQUE,
			balance TEXT NOT NULL
		)
		`,
		`
		CREATE TABLE msigs (
			msig_id INTEGER NOT NULL UNIQUE,
			threshold SMALLINT NOT NULL,
			balance TEXT NOT NULL
		)
		`,
		`
		CREATE TABLE msig_actors (
			msig_id INTEGER NOT NULL,
			actor_id INTEGER NOT NULL,
			UNIQUE( msig_id, actor_id )
		)
		`,
	} {
		if _, err := db.Exec(s); err != nil {
			return nil, fin, xerrors.Errorf("schema init failed: %w", err)
		}
	}

	dict := make(procDictionary, 8)

	if dict[procAddDeal], err = db.Prepare(
		`
		INSERT INTO deals (
			deal_id, client_id, provider_id, piece_cid, label, piece_size, is_filplus, price_per_epoch, provider_collateral, client_collateral, start_epoch, end_epoch, sector_activation_epoch, deal_slash_epoch
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9 ,$10, $11, $12, $13, $14
		)
		`,
	); err != nil {
		return nil, fin, err
	}

	if dict[procAddProvider], err = db.Prepare(
		`
		INSERT INTO providers (
			provider_id, owner_id, worker_id, power_raw, power_qa
		) VALUES (
			$1, $2, $3, $4, $5
		)
		`,
	); err != nil {
		return nil, fin, err
	}

	if dict[procAddAccount], err = db.Prepare(
		`
		INSERT INTO accounts (
			account_id, account_address, balance
		) VALUES (
			$1, $2, $3
		)
		`,
	); err != nil {
		return nil, fin, err
	}

	if dict[procAddMsig], err = db.Prepare(
		`
		INSERT INTO msigs (
			msig_id, threshold, balance
		) VALUES (
			$1, $2, $3
		)
		`,
	); err != nil {
		return nil, fin, err
	}

	if dict[procAddMsigActors], err = db.Prepare(
		`
		INSERT INTO msig_actors (
			msig_id, actor_id
		) VALUES (
			$1, $2
		)
		`,
	); err != nil {
		return nil, fin, err
	}

	return dict, fin, nil
}
