// Copyright (c) 2022 The Dexio developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"
	"github.com/BestSilverTiger/thor/block"
	"github.com/BestSilverTiger/thor/chain"
	"github.com/BestSilverTiger/thor/cmd/thor/bandwidth"
	"github.com/BestSilverTiger/thor/co"
	"github.com/BestSilverTiger/thor/genesis"
	"github.com/BestSilverTiger/thor/logdb"
	"github.com/BestSilverTiger/thor/packer"
	"github.com/BestSilverTiger/thor/state"
	"github.com/BestSilverTiger/thor/thor"
	"github.com/BestSilverTiger/thor/tx"
	"github.com/BestSilverTiger/thor/txpool"
)

var log = log15.New("pkg", "solo")

// Solo mode is the standalone client without p2p server
type Solo struct {
	repo      *chain.Repository
	txPool    *txpool.TxPool
	packer    *packer.Packer
	logDB     *logdb.LogDB
	gasLimit  uint64
	bandwidth bandwidth.Bandwidth
	onDemand  bool
	skipLogs  bool
}

// New returns Solo instance
func New(
	repo *chain.Repository,
	stater *state.Stater,
	logDB *logdb.LogDB,
	txPool *txpool.TxPool,
	gasLimit uint64,
	onDemand bool,
	skipLogs bool,
	forkConfig thor.ForkConfig,
) *Solo {
	return &Solo{
		repo:   repo,
		txPool: txPool,
		packer: packer.New(
			repo,
			stater,
			genesis.DevAccounts()[0].Address,
			&genesis.DevAccounts()[0].Address,
			forkConfig),
		logDB:    logDB,
		gasLimit: gasLimit,
		skipLogs: skipLogs,
		onDemand: onDemand,
	}
}

// Run runs the packer for solo
func (s *Solo) Run(ctx context.Context) error {
	goes := &co.Goes{}

	defer func() {
		<-ctx.Done()
		goes.Wait()
	}()

	goes.Go(func() {
		s.loop(ctx)
	})

	log.Info("prepared to pack block")

	return nil
}

func (s *Solo) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Info("stopping interval packing service......")
			return
		case <-time.After(time.Duration(1) * time.Second):
			if left := uint64(time.Now().Unix()) % thor.BlockInterval; left == 0 {
				if err := s.packing(s.txPool.Executables(), false); err != nil {
					log.Error("failed to pack block", "err", err)
				}
			} else if s.onDemand {
				pendingTxs := s.txPool.Executables()
				if len(pendingTxs) > 0 {
					if err := s.packing(pendingTxs, true); err != nil {
						log.Error("failed to pack block", "err", err)
					}
				}
			}
		}
	}
}

func (s *Solo) packing(pendingTxs tx.Transactions, onDemand bool) error {
	best := s.repo.BestBlock()
	now := uint64(time.Now().Unix())

	var txsToRemove []*tx.Transaction
	defer func() {
		for _, tx := range txsToRemove {
			s.txPool.Remove(tx.Hash(), tx.ID())
		}
	}()

	if s.gasLimit == 0 {
		suggested := s.bandwidth.SuggestGasLimit()
		s.packer.SetTargetGasLimit(suggested)
	}

	flow, err := s.packer.Mock(best.Header(), now, s.gasLimit)
	if err != nil {
		return errors.WithMessage(err, "mock packer")
	}

	startTime := mclock.Now()
	for _, tx := range pendingTxs {
		if err := flow.Adopt(tx); err != nil {
			if packer.IsGasLimitReached(err) {
				break
			}
			if packer.IsTxNotAdoptableNow(err) {
				continue
			}
			txsToRemove = append(txsToRemove, tx)
		}
	}

	b, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		return errors.WithMessage(err, "pack")
	}
	execElapsed := mclock.Now() - startTime

	// If there is no tx packed in the on-demanded block then skip
	if onDemand && len(b.Transactions()) == 0 {
		return nil
	}

	if _, err := stage.Commit(); err != nil {
		return errors.WithMessage(err, "commit state")
	}

	// ignore fork when solo
	if err := s.repo.AddBlock(b, receipts); err != nil {
		return errors.WithMessage(err, "commit block")
	}
	if err := s.repo.SetBestBlockID(b.Header().ID()); err != nil {
		return errors.WithMessage(err, "set best block")
	}

	if !s.skipLogs {
		if err := s.logDB.Log(func(w *logdb.Writer) error {
			return w.Write(b, receipts)
		}); err != nil {
			return errors.WithMessage(err, "commit log")
		}
	}

	commitElapsed := mclock.Now() - startTime - execElapsed

	if v, updated := s.bandwidth.Update(b.Header(), time.Duration(execElapsed+commitElapsed)); updated {
		log.Debug("bandwidth updated", "gps", v)
	}

	blockID := b.Header().ID()
	log.Info("📦 new block packed",
		"txs", len(receipts),
		"mgas", float64(b.Header().GasUsed())/1000/1000,
		"et", fmt.Sprintf("%v|%v", common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
		"id", fmt.Sprintf("[#%v…%x]", block.Number(blockID), blockID[28:]),
	)
	log.Debug(b.String())

	return nil
}
