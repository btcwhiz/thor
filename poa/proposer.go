// Copyright (c) 2022 The Dexio developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"github.com/BestSilverTiger/thor/thor"
)

// Proposer address with status.
type Proposer struct {
	Address thor.Address
	Active  bool
}
