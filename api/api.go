// Copyright (c) 2022 The DexioThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"net/http"
	"net/http/pprof"
	"strings"

	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/BestSilverTiger/thor/api/accounts"
	"github.com/BestSilverTiger/thor/api/blocks"
	"github.com/BestSilverTiger/thor/api/debug"
	"github.com/BestSilverTiger/thor/api/doc"
	"github.com/BestSilverTiger/thor/api/events"
	"github.com/BestSilverTiger/thor/api/node"
	"github.com/BestSilverTiger/thor/api/subscriptions"
	"github.com/BestSilverTiger/thor/api/transactions"
	"github.com/BestSilverTiger/thor/api/transfers"
	"github.com/BestSilverTiger/thor/chain"
	"github.com/BestSilverTiger/thor/logdb"
	"github.com/BestSilverTiger/thor/state"
	"github.com/BestSilverTiger/thor/thor"
	"github.com/BestSilverTiger/thor/txpool"
)

//New return api router
func New(
	repo *chain.Repository,
	stater *state.Stater,
	txPool *txpool.TxPool,
	logDB *logdb.LogDB,
	nw node.Network,
	allowedOrigins string,
	backtraceLimit uint32,
	callGasLimit uint64,
	pprofOn bool,
	skipLogs bool,
	forkConfig thor.ForkConfig,
) (http.HandlerFunc, func()) {

	origins := strings.Split(strings.TrimSpace(allowedOrigins), ",")
	for i, o := range origins {
		origins[i] = strings.ToLower(strings.TrimSpace(o))
	}

	router := mux.NewRouter()

	// to serve api doc and swagger-ui
	router.PathPrefix("/doc").Handler(
		http.StripPrefix("/doc/", http.FileServer(
			&assetfs.AssetFS{
				Asset:     doc.Asset,
				AssetDir:  doc.AssetDir,
				AssetInfo: doc.AssetInfo})))

	// redirect swagger-ui
	router.Path("/").HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "doc/swagger-ui/", http.StatusTemporaryRedirect)
		})

	accounts.New(repo, stater, callGasLimit, forkConfig).
		Mount(router, "/accounts")

	if !skipLogs {
		events.New(repo, logDB).
			Mount(router, "/logs/event")
		transfers.New(repo, logDB).
			Mount(router, "/logs/transfer")
	}
	blocks.New(repo).
		Mount(router, "/blocks")
	transactions.New(repo, txPool).
		Mount(router, "/transactions")
	debug.New(repo, stater, forkConfig).
		Mount(router, "/debug")
	node.New(nw).
		Mount(router, "/node")
	subs := subscriptions.New(repo, origins, backtraceLimit)
	subs.Mount(router, "/subscriptions")

	if pprofOn {
		router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		router.HandleFunc("/debug/pprof/trace", pprof.Trace)
		router.PathPrefix("/debug/pprof/").HandlerFunc(pprof.Index)
	}

	handler := handlers.CompressHandler(router)
	handler = handlers.CORS(
		handlers.AllowedOrigins(origins),
		handlers.AllowedHeaders([]string{"content-type", "x-genesis-id"}),
		handlers.ExposedHeaders([]string{"x-genesis-id", "x-thorest-ver"}),
	)(handler)
	return handler.ServeHTTP,
		subs.Close // subscriptions handles hijacked conns, which need to be closed
}
