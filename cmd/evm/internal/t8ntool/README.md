# Stateless EVM tool

this evm tool is a fork of geth's evm tool (just like rest of erigon). It is "stateless" because it doesn't need to maintain the entire blockchain history in order to execute a block and the transactions in it. The input can be provided in two formats:

- original (OG): txs (transactions), env (environment) and genesis alloc (initial set of accounts and their balances)
- block specimen: the block specimen json is provided, which is then converted to a block by the tool and processed.

OG input can be provided by `--input.alloc`, `--input.env`, `--input.txs`

Block specimen input is controlled by `--input.replica`. When this is provided, other input flags are ignored.

Similary, the output is provided in two formats:
- original: controlled by `output.alloc` (post-process allocations), `output.body` (rlp of transactions body)
- block result: controlled by <TODO>


## Development

- `transition.go` deals with the main logic to adapt the inputs into a block, which can then be passed to the `ExecuteBlockEphemerally` api.
- `t8n_test.go` contains test cases for the evm tool
