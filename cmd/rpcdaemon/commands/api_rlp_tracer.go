package commands

import (
	// "context"
	// "errors"
	// "fmt"
	// "os"
	// "path"
	// "runtime"
	// "sync"
	// "time"

	// "golang.org/x/sync/semaphore"

	"errors"
	"path"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/semaphore"

	// "github.com/ledgerwatch/erigon/common"
	"context"
	"fmt"
	"os"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/cmd/state/exec3"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/rlptracer"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/eth/tracers/logger"
	"github.com/ledgerwatch/erigon/rpc"
	// "github.com/ledgerwatch/erigon/core"
	// "github.com/ledgerwatch/erigon/core/rlptracer"
	// "github.com/ledgerwatch/erigon/core/state"
	// "github.com/ledgerwatch/erigon/core/types"
	// "github.com/ledgerwatch/erigon/core/vm"
	// "github.com/ledgerwatch/erigon/log"
	// "github.com/ledgerwatch/erigon/rpc"
)

type TraceConfig struct {
	*logger.LogConfig
	Tracer  *string
	Timeout *string
	Reexec  *uint64
}

// RlpTraceConfig holds extra parameters to RLP trace functions.
type RlpTraceConfig struct {
	TraceConfig
	TxHash            libcommon.Hash
	PushToObjectStore bool
	Overwrite         bool
}

func (api *CovalentTraceAPIImpl) TraceChainToRlpFile(ctx context.Context, start, end rpc.BlockNumber, config *RlpTraceConfig) ([]string, error) {
	// 	// Fetch the block interval that we want to trace
	var from, to, currentBlock *types.Block
	tx, err := api.db.BeginRo(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	currentBlock = rawdb.ReadCurrentBlock(tx)

	switch start {
	case rpc.PendingBlockNumber:
		return nil, fmt.Errorf("not implemented")
	case rpc.LatestBlockNumber:
		from = currentBlock
	default:
		// from = api.eth.blockchain.GetBlockByNumber(uint64(start)) //orig
		from, err = rawdb.ReadBlockByNumber(tx, uint64(start))
	}

	switch end {
	case rpc.PendingBlockNumber:
		return nil, fmt.Errorf("not implemented")
	case rpc.LatestBlockNumber:
		to = currentBlock
	default:
		to, err = rawdb.ReadBlockByNumber(tx, uint64(end))
	}
	// Trace the chain if we've found all our blocks
	if from == nil {
		return nil, fmt.Errorf("starting block #%d not found", start)
	}
	if to == nil {
		return nil, fmt.Errorf("end block #%d not found", end)
	}
	if from.Number().Cmp(to.Number()) > 0 {
		return nil, fmt.Errorf("end block (#%d) needs to come after start block (#%d)", end, start)
	}
	if from.Number().Sign() == 0 && currentBlock.NumberU64() >= 1000000 {
		return nil, errors.New("refusing to take ages scanning a deep genesis block")
	}

	return api.traceChainToRlpFile(ctx, from, to, config)
}

var (
	traceSem       = semaphore.NewWeighted(1)
	traceSemLocked = errors.New("RLP tracing in-use by another client")
	reprobeSem     = semaphore.NewWeighted(3)
)

func (api *CovalentTraceAPIImpl) traceChainToRlpFile(ctx context.Context, start, end *types.Block, config *RlpTraceConfig) ([]string, error) {
	var err error

	// Ensure we have a valid starting state before doing any work
	scanStart := start
	startNr := start.NumberU64()
	endNr := end.NumberU64()

	tx, err := api.db.BeginRo(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	traceLogFileName := fmt.Sprintf("trace-%08x-%08x.rlp", startNr, endNr)

	var (
		publishToBucket    []byte
		overwritePublished bool
	)

	var logConfig logger.LogConfig
	if config != nil {
		if config.LogConfig != nil {
			logConfig = *config.LogConfig
		}
		if config.PushToObjectStore {
			publishToBucket = []byte(os.Getenv("GETH_EXPORTS_BUCKET"))
			if config.Overwrite {
				overwritePublished = true
			}
		}
	}
	logConfig.Debug = true

	// if publishing to object storage, pre-check validity of
	// destination, and also bail out early if destination
	// already exists (and not forcing recompute)
	var (
		publishClient *storage.Client
		publishBucket *storage.BucketHandle
		publishObject *storage.ObjectHandle
	)
	if len(publishToBucket) > 0 {
		publishClient, err = storage.NewClient(ctx)
		if err != nil {
			return nil, err
		}

		publishBucket = publishClient.Bucket(string(publishToBucket))
		if _, err = publishBucket.Attrs(ctx); err != nil {
			return nil, err
		}

		publishObject = publishBucket.Object(traceLogFileName)

		_, err = publishObject.Attrs(ctx)
		switch err {
		case storage.ErrObjectNotExist:
			// proceed with trace
		case nil:
			if !overwritePublished {
				return signObject(publishObject)
			}
			// otherwise, proceed with trace
		default:
			return nil, err
		}
	}

	if traceSem.TryAcquire(1) {
		defer traceSem.Release(1)
	} else {
		return nil, traceSemLocked
	}
	conf, err := api.BaseAPI.chainConfig(tx)
	api.chainReader = exec3.NewChainReader(conf, tx, nil)

	// Create the parent state database
	if err := api.engine.VerifyHeader(api.chainReader, scanStart.Header(), true); err != nil {
		return nil, err
	}
	if startNr > 0 {
		// scanStart, err = rawdb.ReadBlockByNumber(tx, uint64(start))
		scanStart = rawdb.ReadBlock(tx, scanStart.ParentHash(), scanStart.NumberU64()-1)
		if scanStart == nil {
			return nil, fmt.Errorf("parent %#x not found", scanStart.ParentHash())
		}

	}

	// scanBlockCount := int(endNr-scanStart.NumberU64()) + 1
	// chainConfig := conf

	var traceLogFile *os.File
	traceLogFile, err = os.Create(path.Join("./", traceLogFileName))
	if err != nil {
		return nil, err
	}
	defer traceLogFile.Close()

	tracer := rlptracer.NewTracer(&logConfig, traceLogFile)
	tracer.CaptureTraceBegin(ctx, startNr, endNr)

	// 	threads := runtime.NumCPU()
	// 	if threads > scanBlockCount {
	// 		threads = scanBlockCount
	// 	}

	// 	var (
	// 		pend         = new(sync.WaitGroup)
	// 		traceDone    = make(chan bool)
	// 		blocksToScan = make(chan *types.Block, threads)
	// 	)

	// 	go func() {
	// 		pend.Wait()
	// 		log.Debug("rlptracer: WaitGroup finished")
	// 		tracer.CaptureTraceEnd()
	// 		traceDone <- true
	// 		close(traceDone)
	// 	}()

	return nil, fmt.Errorf("implement func (api *CovalentTraceAPIImpl) traceChainToRlpFile(ctx context.Context, start, end *types.Block, config *RlpTraceConfig) ([]string, error) {")
}

// 	for th := 0; th < threads; th++ {
// 		pend.Add(1)
// 		my_th := th
// 		go func() {
// 			defer pend.Done()

// 			codeBackingStore, preflightErr := state.NewDBCachingCodeReader(api.eth.ChainDb())

// 			if preflightErr != nil {
// 				panic("couldn't build codeBackingStore")
// 			}

// 			noopStateWriter := state.NewNoopStateWriter()
// 			noopCodeWriter := state.NewNoopCodeWriter()

// 			// Fetch and execute the next block trace tasks
// 		ScanBlock:
// 			for block := range blocksToScan {
// 				select {
// 				case <-ctx.Done():
// 					break ScanBlock
// 				default:
// 				}

// 				var taskErr error

// 				var stateBeforeBackingStore state.StateReader
// 				var stateAfterBackingStore state.StateReader

// 				if block.NumberU64() > 0 {
// 					stateBeforeBackingStore, taskErr = state.NewDBCachingStateReader(api.eth.ChainDb(), block.NumberU64()-1)
// 				} else {
// 					stateBeforeBackingStore = state.NewEmptyStateReader()
// 				}

// 				if taskErr != nil {
// 					log.Error("couldn't build stateBeforeBackingStore")
// 					break ScanBlock
// 				}

// 				stateAfterBackingStore, taskErr = state.NewDBCachingStateReader(api.eth.ChainDb(), block.NumberU64())

// 				if taskErr != nil {
// 					log.Error("couldn't build stateBeforeBackingStore")
// 					break ScanBlock
// 				}

// 				stateBefore := state.New(stateBeforeBackingStore, codeBackingStore)
// 				stateAfter := state.New(stateAfterBackingStore, codeBackingStore)

// 				stateBefore.Prepare(common.Hash{}, block.Hash(), block.NumberU64(), -1, chainConfig.IsEIP158(block.Number()))
// 				// stateAfter.Prepare(common.Hash{}, block.Hash(), block.NumberU64(), -1)

// 				var tracerW *rlptracer.Tracer
// 				tracerW, taskErr = tracer.NewWorker()
// 				if taskErr != nil {
// 					log.Error("couldn't spawn tracer worker")
// 					break ScanBlock
// 				}

// 				stateBeforeExt := stateBefore.TracingExtensions(tracerW)

// 				vmConf := vm.Config{
// 					Debug:                   true,
// 					Tracer:                  tracerW,
// 					EnablePreimageRecording: false,
// 				}

// 				signer := types.MakeSigner(chainConfig, block.Number())

// 				tracerW.CaptureBlockBegin(block, chainConfig, stateBefore)

// 				// Trace all the transactions contained within
// 				for i, tx := range block.Transactions() {
// 					stateBefore.Prepare(tx.Hash(), block.Hash(), block.NumberU64(), i, chainConfig.IsEIP158(block.Number()))

// 					select {
// 					case <-ctx.Done():
// 						break ScanBlock
// 					default:
// 					}

// 					var msg types.Message
// 					msg, taskErr = tx.AsMessage(signer)

// 					if taskErr == nil {
// 						vmctx := core.NewEVMContext(msg, block.Header(), api.eth.blockchain, nil)
// 						vmenv := vm.NewEVM(vmctx, stateBeforeExt, chainConfig, vmConf)
// 						st := core.NewStateTransition(vmenv, msg, new(core.GasPool).AddGas(msg.Gas()))

// 						tracerW.CaptureTxBegin(i, msg, stateBefore)

// 						var (
// 							ret     []byte
// 							usedGas uint64
// 							failed  bool
// 						)
// 						ret, usedGas, failed, taskErr = st.TransitionDb()

// 						tracerW.CaptureTxEnd(ret, usedGas, failed, taskErr)

// 						if taskErr == nil {
// 							stateBefore.Checkpoint()
// 						}
// 					}

// 					if taskErr != nil {
// 						log.Warn("Tracing failed", "hash", tx.Hash(), "block", block.NumberU64(), "tx", i, "err", taskErr)
// 						break ScanBlock
// 					}
// 				}

// 				if taskErr == nil {
// 					api.eth.engine.ApplyRules(api.eth.blockchain, block.Header(), stateBeforeExt, block.Uncles())
// 				}

// 				taskErr = stateBefore.Commit(noopStateWriter, noopCodeWriter)
// 				if taskErr != nil {
// 					log.Warn("Tracing failed", "block", block.NumberU64(), "phase", "commit", "err", taskErr)
// 				}

// 				tracerW.CaptureBlockEnd(stateAfter)
// 			}

// 			select {
// 			case <-ctx.Done():
// 				log.Debug("rlptracer: EVM thread terminated by ctx", "id", my_th)
// 			default:
// 				log.Debug("rlptracer: EVM thread finished", "id", my_th)
// 			}
// 		}()
// 	}

// 	// Start a goroutine to feed all the blocks into the tracers
// 	begin := time.Now()
// 	go func() {
// 		var (
// 			logged time.Time
// 			number uint64
// 			traced uint64
// 			failed error
// 		)

// 		log.Info("Chain tracing began", "start", startNr, "end", endNr)

// 		// Ensure everything is properly cleaned up on any exit path
// 		defer func() {
// 			<-traceDone

// 			switch {
// 			case failed != nil:
// 				log.Warn("Chain tracing failed", "start", startNr, "end", endNr, "transactions", traced, "elapsed", time.Since(begin), "err", failed)
// 			case number < endNr:
// 				log.Warn("Chain tracing aborted", "start", startNr, "end", endNr, "abort", number, "transactions", traced, "elapsed", time.Since(begin))
// 			default:
// 				log.Info("Chain tracing finished", "start", startNr, "end", endNr, "transactions", traced, "elapsed", time.Since(begin))
// 			}
// 		}()

// 		logged = begin

// 	FeedBlocks:
// 		for number = scanStart.NumberU64(); number <= endNr; number++ {
// 			select {
// 			case <-ctx.Done():
// 				break FeedBlocks
// 			default:
// 			}

// 			// Print progress logs if long enough time elapsed
// 			if time.Since(logged) > 8*time.Second {
// 				if number >= startNr {
// 					log.Info("Tracing chain segment", "start", startNr, "end", endNr, "current", number, "transactions", traced, "elapsed", time.Since(begin), "memory", api.eth.ChainDb().Size())
// 				} else {
// 					log.Info("Preparing state for chain trace", "block", number, "start", startNr, "elapsed", time.Since(begin))
// 				}
// 				logged = time.Now()
// 			}
// 			// Retrieve the next block to trace
// 			block := api.eth.blockchain.GetBlockByNumber(number)
// 			if block == nil {
// 				failed = fmt.Errorf("block #%d not found", number)
// 				break FeedBlocks
// 			}

// 			if number >= startNr {
// 				// Send the block over to the concurrent tracers
// 				txs := block.Transactions()
// 				blocksToScan <- block
// 				traced += uint64(len(txs))
// 			}
// 		}

// 		select {
// 		case <-ctx.Done():
// 			log.Debug("rlptracer: feeder thread terminated by ctx")
// 		default:
// 		}

// 		close(blocksToScan)
// 	}()

// 	<-traceDone

// 	select {
// 	case <-ctx.Done():
// 		log.Debug("rlptracer: returning ctx error to client")
// 		return []string{}, fmt.Errorf("context closed")
// 	default:
// 	}

// 	if len(publishToBucket) > 0 {
// 		return publishFileToObject(ctx, traceLogFile, publishObject)
// 	} else {
// 		return []string{"/exports/" + path.Base(traceLogFile.Name())}, nil
// 	}
// }

// func (api *PrivateDebugAPI) TraceBlockForCodeChanges(ctx context.Context, blockNr rpc.BlockNumber, config *RlpTraceConfig) error {
// 	var block *types.Block

// switch blockNr {
// case rpc.PendingBlockNumber:
//
//	return fmt.Errorf("not implemented")
//
// case rpc.LatestBlockNumber:
//
//	block = api.eth.blockchain.CurrentBlock()
//
// default:
//
//		block = api.eth.blockchain.GetBlockByNumber(uint64(blockNr))
//	}
//
//	if block == nil {
//		return fmt.Errorf("block #%d not found", blockNr)
//	}
//
// return api.traceBlockForCodeChanges(ctx, block, config)
// }

// func (api *PrivateDebugAPI) traceBlockForCodeChanges(ctx context.Context, block *types.Block, config *RlpTraceConfig) error {
// 	var logConfig vm.LogConfig
// 	if config != nil {
// 		if config.LogConfig != nil {
// 			logConfig = *config.LogConfig
// 		}
// 	}
// 	logConfig.Debug = true

// 	if err := reprobeSem.Acquire(ctx, 1); err != nil {
// 		return nil
// 	}
// 	defer reprobeSem.Release(1)

// 	select {
// 	case <-ctx.Done():
// 		return nil
// 	default:
// 	}

// 	if err := api.eth.engine.VerifyHeader(api.eth.blockchain, block.Header(), true); err != nil {
// 		return err
// 	}

// 	chainConfig := api.eth.blockchain.Config()

// 	var taskErr error

// 	codeBackingStore, taskErr := state.NewDBCachingCodeReader(api.eth.ChainDb())

// 	if taskErr != nil {
// 		log.Warn("Tracing failed", "block", block.NumberU64(), "err", taskErr)
// 		return taskErr
// 	}

// 	var stateBeforeBackingStore state.StateReader

// 	if block.NumberU64() > 0 {
// 		stateBeforeBackingStore, taskErr = state.NewDBCachingStateReader(api.eth.ChainDb(), block.NumberU64()-1)
// 	} else {
// 		stateBeforeBackingStore = state.NewEmptyStateReader()
// 	}

// 	if taskErr != nil {
// 		log.Warn("Tracing failed", "block", block.NumberU64(), "err", taskErr)
// 		return taskErr
// 	}

// 	codeWriterBatch := api.eth.ChainDb().Underlying().NewBatch()

// 	stateBefore := state.New(stateBeforeBackingStore, codeBackingStore)
// 	stateBefore.SetDirectCodeWriter(codeWriterBatch)

// 	stateBefore.Prepare(common.Hash{}, block.Hash(), block.NumberU64(), -1, chainConfig.IsEIP158(block.Number()))

// 	vmConf := vm.Config{
// 		Debug:                   false,
// 		EnablePreimageRecording: false,
// 	}

// 	signer := types.MakeSigner(chainConfig, block.Number())

// 	for i, tx := range block.Transactions() {
// 		stateBefore.Prepare(tx.Hash(), block.Hash(), block.NumberU64(), i, chainConfig.IsEIP158(block.Number()))

// 		var msg types.Message
// 		msg, taskErr = tx.AsMessage(signer)

// 		if taskErr != nil {
// 			log.Warn("Tracing failed", "hash", tx.Hash(), "block", block.NumberU64(), "tx", i, "err", taskErr)
// 			codeWriterBatch.Commit()
// 			return taskErr
// 		}

// 		vmctx := core.NewEVMContext(msg, block.Header(), api.eth.blockchain, nil)
// 		vmenv := vm.NewEVM(vmctx, stateBefore, chainConfig, vmConf)
// 		st := core.NewStateTransition(vmenv, msg, new(core.GasPool).AddGas(msg.Gas()))

// 		_, _, _, taskErr = st.TransitionDb()

// 		if taskErr == nil {
// 			stateBefore.Checkpoint()
// 		} else {
// 			log.Warn("Tracing failed", "hash", tx.Hash(), "block", block.NumberU64(), "tx", i, "err", taskErr)
// 			codeWriterBatch.Commit()
// 			return taskErr
// 		}
// 	}

// 	codeWriterBatch.Commit()

// 	return nil
// }
