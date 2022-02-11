// Copyright (c) 2022 The Dexio developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"github.com/BestSilverTiger/thor/chain"
	"github.com/BestSilverTiger/thor/thor"
)

type blockReader struct {
	repo        *chain.Repository
	blockReader chain.BlockReader
}

func newBlockReader(repo *chain.Repository, position thor.Bytes32) *blockReader {
	return &blockReader{
		repo:        repo,
		blockReader: repo.NewBlockReader(position),
	}
}

func (br *blockReader) Read() ([]interface{}, bool, error) {
	blocks, err := br.blockReader.Read()
	if err != nil {
		return nil, false, err
	}
	var msgs []interface{}
	for _, block := range blocks {
		msg, err := convertBlock(block)
		if err != nil {
			return nil, false, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, len(blocks) > 0, nil
}
