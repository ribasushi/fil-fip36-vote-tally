// main is main is main
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"sync/atomic"
	"time"

	filaddr "github.com/filecoin-project/go-address"
	filabi "github.com/filecoin-project/go-state-types/abi"
	filbig "github.com/filecoin-project/go-state-types/big"

	lbi "github.com/filecoin-project/lotus/chain/actors/builtin"
	lbiaccount "github.com/filecoin-project/lotus/chain/actors/builtin/account"
	lbimarket "github.com/filecoin-project/lotus/chain/actors/builtin/market"
	lbiprovider "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	lbimsig "github.com/filecoin-project/lotus/chain/actors/builtin/multisig"
	lbipower "github.com/filecoin-project/lotus/chain/actors/builtin/power"

	lchadt "github.com/filecoin-project/lotus/chain/actors/adt"
	lchstmgr "github.com/filecoin-project/lotus/chain/stmgr"
	lchtypes "github.com/filecoin-project/lotus/chain/types"

	ipldcbor "github.com/ipfs/go-ipld-cbor"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

// https://github.com/filecoin-project/FIPs/discussions/464
// https://filscan.io/tipset/chain?height=2162760
var pollTSK = lchtypes.NewTipSetKey(
	cidMustParse("bafy2bzaceabzgill6ohwkbeth3vzh4jik53327yegbpna5nr5v2mewf6yfwva"),
	cidMustParse("bafy2bzacecyushkda5uzfti5gxo3qd3vu4fvtw6now5xhptdu3rbu5igjp3ls"),
	cidMustParse("bafy2bzacebhdnvo4ddikbwwtp4qz5pg6fx6qvv7lsarez3jxa6ieeoqblc3pm"),
	cidMustParse("bafy2bzacebjufknyemhaxm4kpgv6r4phbspqwerlq2bytn2kgpkxdwmeusx22"),
	cidMustParse("bafy2bzacea4wjhb35tjesgthjoagbmcn35bw4szpvo7px6etdovenittok3x2"),
	cidMustParse("bafy2bzaceag3twptjj7ftnitrszig7e722gntwhaoasteuui6rdfshkso3zf4"),
	cidMustParse("bafy2bzacea5p5gvjfizskl7tkw2z7lrf4naiwmolz4doyntnb3wowky6f7rhq"),
	cidMustParse("bafy2bzaceaw4apneihdawrunkuzmnvyifb47qv5562hhe5736u3gibibjcq7g"),
	cidMustParse("bafy2bzacec2an5cdljwxmajfvvxwti2b7c7sspi36dzii5q623lljw66y3rq2"),
	cidMustParse("bafy2bzaced5qwkaraucepstqtdvu23upqnvjok2gr5ksj5yys252p6pq7vhxe"),
)

const (
	workDir = `data`
	dbName  = "filstate_2162760.sqlite"
)

// https://fil-chain-snapshots-fallback.s3.amazonaws.com/mainnet/minimal_finality_stateroots_2163120_2022-09-15_00-00-00.car.zst
const srcSnapsshot = `minimal_finality_stateroots_2163120_2022-09-15_00-00-00.car`

func main() {
	ctx := context.Background()

	err := parseStaticData(ctx, workDir, srcSnapsshot, pollTSK)
	if err != nil {
		log.Fatalf("%+v", err)
	}
}

type totCounters map[string]*int32

func parseStaticData(ctx context.Context, workDir, srcSnapshot string, tsk lchtypes.TipSetKey) (defErr error) {

	dbProcDict, finalize, err := prepDb(workDir)
	if err != nil {
		return err
	}
	defer func() {
		var out string
		if defErr == nil {
			out = path.Join(workDir, dbName)
		}
		finErr := finalize(out)
		if defErr == nil {
			defErr = finErr
		}
	}()

	carbs, err := blockstoreFromSnapshot(ctx, workDir, srcSnapsshot)
	if err != nil {
		return err
	}

	sm, err := newFilStateReader(carbs)
	if err != nil {
		return xerrors.Errorf("unable to initialize a StateManager: %w", err)
	}

	ts, err := sm.ChainStore().GetTipSetFromKey(ctx, tsk)
	if err != nil {
		return xerrors.Errorf("unable to load target tipset: %w", err)
	}

	eg, shCtx := errgroup.WithContext(ctx)

	totals := totCounters{
		"accounts":  new(int32),
		"msigs":     new(int32),
		"providers": new(int32),
		"deals":     new(int32),
	}
	printStats := func() {
		os.Stderr.WriteString(fmt.Sprintf( //nolint:errcheck
			"Processed      deals:% 5d     accounts:% 5d     msigs:% 5d     providers:% 5d\r",
			atomic.LoadInt32(totals["deals"]),
			atomic.LoadInt32(totals["accounts"]),
			atomic.LoadInt32(totals["msigs"]),
			atomic.LoadInt32(totals["providers"]),
		))
	}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-shCtx.Done():
				printStats()
				os.Stderr.WriteString("\n") //nolint:errcheck
				return
			case <-ticker.C:
				printStats()
			}
		}
	}()

	eg.Go(func() error { return parseActors(shCtx, dbProcDict, sm, ts, totals) })
	eg.Go(func() error { return parseDeals(shCtx, dbProcDict, sm, ts, totals) })

	return eg.Wait()
}

func parseActors(ctx context.Context, dict procDictionary, sm *lchstmgr.StateManager, ts *lchtypes.TipSet, tot totCounters) error {
	ast := lchadt.WrapStore(ctx, ipldcbor.NewCborStore(sm.ChainStore().UnionStore()))

	stateTree, _ := sm.StateTree(ts.ParentState())
	paddr, _ := stateTree.GetActor(lbipower.Address)
	ps, _ := lbipower.Load(ast, paddr)

	return stateTree.ForEach(func(addr filaddr.Address, act *lchtypes.Actor) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		switch {

		case lbi.IsStorageMinerActor(act.Code):

			p, err := lbiprovider.Load(ast, act)
			if err != nil {
				return err
			}

			pi, err := p.Info()
			if err != nil {
				return err
			}

			pc, eligibleToMine, err := ps.MinerPower(addr)
			if err != nil {
				return err
			}
			if !eligibleToMine {
				pc.RawBytePower = filbig.Zero()
				pc.QualityAdjPower = filbig.Zero()
			}

			bal, err := p.AvailableBalance(act.Balance)
			if err != nil {
				return err
			}
			vest, err := p.VestedFunds(ts.Height())
			if err != nil {
				return err
			}

			atomic.AddInt32(tot["providers"], 1)
			_, err = dict[procAddProvider].Exec(
				mustAddrID(addr),
				mustAddrID(pi.Owner),
				mustAddrID(pi.Worker),
				pc.RawBytePower.String(),
				pc.QualityAdjPower.String(),
				filbig.Add(bal, vest).String(),
			)
			return err

		case lbi.IsAccountActor(act.Code):

			w, err := lbiaccount.Load(ast, act)
			if err != nil {
				return err
			}
			pa, err := w.PubkeyAddress()
			if err != nil {
				return err
			}

			atomic.AddInt32(tot["accounts"], 1)
			_, err = dict[procAddAccount].Exec(
				mustAddrID(addr),
				pa.String(),
				act.Balance.String(),
			)
			return err

		case lbi.IsMultisigActor(act.Code):

			ms, err := lbimsig.Load(ast, act)
			if err != nil {
				return err
			}
			msID := mustAddrID(addr)
			tr, _ := ms.Threshold()

			actors, err := ms.Signers()
			if err != nil {
				return err
			}
			for _, a := range actors {
				if _, err := dict[procAddMsigActors].Exec(
					msID,
					mustAddrID(a),
				); err != nil {
					return err
				}
			}

			// msig balance needs calculating for epoch in question
			lb, err := ms.LockedBalance(ts.Height())
			if err != nil {
				return err
			}

			atomic.AddInt32(tot["msigs"], 1)
			_, err = dict[procAddMsig].Exec(
				msID,
				tr,
				filbig.Sub(act.Balance, lb).String(),
			)
			return err

		default:
			return nil

		}
	})
}

func parseDeals(ctx context.Context, dict procDictionary, sm *lchstmgr.StateManager, ts *lchtypes.TipSet, tot totCounters) error {

	ms, err := sm.GetMarketState(ctx, ts)
	if err != nil {
		return xerrors.Errorf("unable to load market state: %w", err)
	}

	marketProps, err := ms.Proposals()
	if err != nil {
		return err
	}

	marketStates, err := ms.States()
	if err != nil {
		return err
	}

	return marketProps.ForEach(func(dealID filabi.DealID, d lbimarket.DealProposal) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		s, sectorStateFound, err := marketStates.Get(dealID)
		if err != nil {
			return xerrors.Errorf("failed to get state for deal in proposals array: %w", err)
		} else if !sectorStateFound {
			s = lbimarket.EmptyDealState()
		}

		var label []byte
		if d.Label.IsBytes() {
			label, _ = d.Label.ToBytes()
		} else {
			s, _ := d.Label.ToString()
			label = []byte(s)
		}

		var sectorStart, dealSlash *filabi.ChainEpoch
		if s.SectorStartEpoch != -1 {
			v := s.SectorStartEpoch
			sectorStart = &v
		}
		if s.SlashEpoch != -1 {
			v := s.SlashEpoch
			dealSlash = &v
		}

		atomic.AddInt32(tot["deals"], 1)
		_, err = dict[procAddDeal].Exec(
			dealID,
			mustAddrID(d.Client),
			mustAddrID(d.Provider),
			d.PieceCID.String(),
			label,
			d.PieceSize,
			d.VerifiedDeal,
			d.StoragePricePerEpoch.Int64(),
			d.ProviderCollateral.Int64(),
			d.ClientCollateral.Int64(),
			d.StartEpoch,
			d.EndEpoch,
			sectorStart,
			dealSlash,
		)
		return err
	})
}
