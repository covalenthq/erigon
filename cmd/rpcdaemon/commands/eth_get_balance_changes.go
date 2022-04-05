package commands

import (
	"bytes"
	"fmt"
	"reflect"

	common2 "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"

	"github.com/ledgerwatch/erigon/common"
	"github.com/ledgerwatch/erigon/common/changeset"
	"github.com/ledgerwatch/erigon/common/dbutils"
	"github.com/ledgerwatch/erigon/common/hexutil"
	"github.com/ledgerwatch/erigon/core/types/accounts"
	"github.com/ledgerwatch/erigon/rpc"
)

type oldNewBalance struct {
	oldBalance *hexutil.Big
	newBalance *hexutil.Big
}

func (api *APIImpl) GetBalanceChangesInBlock(blockNum uint64) (map[common.Address]oldNewBalance, error) {

	ctx, _ := common2.RootContext()
	tx, beginErr := api.db.BeginRo(ctx)
	if beginErr != nil {
		return nil, beginErr
	}
	defer tx.Rollback()

	c, err := tx.Cursor(kv.AccountChangeSet)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	startkey := dbutils.EncodeBlockNumber(blockNum)

	decode := changeset.Mapper[kv.AccountChangeSet].Decode

	balancesMapping := make(map[common.Address]oldNewBalance)

	for dbKey, dbValue, _ := c.Seek(startkey); bytes.Equal(dbKey, startkey) && dbKey != nil; dbKey, dbValue, _ = c.Next() {

		_, address, v, err := decode(dbKey, dbValue)
		if err != nil {
			return nil, err
		}
		var acc accounts.Account
		if err = acc.DecodeForStorage(v); err != nil {
			return nil, err
		}
		old_balance := (*hexutil.Big)(acc.Balance.ToBig())
		var commonAddress common.Address
		copy(commonAddress[:], address[:common.AddressLength])
		n := rpc.BlockNumber(blockNum)
		new_balance, newBalanceErr := api.GetBalance(ctx, commonAddress, rpc.BlockNumberOrHash{BlockNumber: &n})
		if newBalanceErr != nil {
			return nil, newBalanceErr
		}

		if !reflect.DeepEqual(old_balance, new_balance) {
			balancesMapping[commonAddress] = oldNewBalance{
				oldBalance: old_balance,
				newBalance: new_balance}
		}

	}

	return balancesMapping, nil

}

func PrintChangedBalances(mapping map[common.Address]oldNewBalance) error {

	for address, balances := range mapping {
		fmt.Println("address: ", address)
		fmt.Println("old balance: ", balances.oldBalance)
		fmt.Println("new balance: ", balances.newBalance)
	}

	return nil
}
