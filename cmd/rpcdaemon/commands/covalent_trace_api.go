package commands

import (
	"context"

	"github.com/ledgerwatch/erigon-lib/kv"

	"github.com/ledgerwatch/erigon/cmd/rpcdaemon/cli/httpcfg"
	"github.com/ledgerwatch/erigon/cmd/state/exec3"
	"github.com/ledgerwatch/erigon/consensus"
	"github.com/ledgerwatch/erigon/consensus/ethash"
	"github.com/ledgerwatch/erigon/rpc"
	"github.com/ledgerwatch/erigon/turbo/rpchelper"
)

type CovalentTraceAPI interface {
	TraceChainToRlpFile(ctx context.Context, start, end rpc.BlockNumber, config *RlpTraceConfig) ([]string, error)
}

type CovalentTraceAPIImpl struct {
	*BaseAPI
	db            kv.RoDB
	eth           rpchelper.ApiBackend
	engine        consensus.Engine
	chainReader   exec3.ChainReader
	maxTraces     uint64
	gasCap        uint64
	compatibility bool // Bug for bug compatiblity with OpenEthereum
}

// NewTraceAPI returns NewTraceAPI instance
func NewCovalentTraceAPI(base *BaseAPI, db kv.RoDB, eth rpchelper.ApiBackend, cfg *httpcfg.HttpCfg) *CovalentTraceAPIImpl {
	return &CovalentTraceAPIImpl{
		BaseAPI:       base,
		db:            db,
		engine:        ethash.NewShared(),
		eth:           eth,
		maxTraces:     cfg.MaxTraces,
		gasCap:        cfg.Gascap,
		compatibility: cfg.TraceCompatibility,
	}
}
