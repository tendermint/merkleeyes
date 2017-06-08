package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	abci "github.com/tendermint/abci/types"
	wire "github.com/tendermint/go-wire"
	"github.com/tendermint/merkleeyes/iavl"
	cmn "github.com/tendermint/tmlibs/common"
)

func makeTx(typ byte, args ...[]byte) []byte {
	n := 12 + 1
	for _, arg := range args {
		n += wire.ByteSliceSize(arg)
	}
	tx := make([]byte, n)
	buf := tx

	// nonce
	copy(buf[:12], cmn.RandBytes(12))
	buf = buf[12:]

	// type byte
	buf[0] = typ
	n = 1
	var err error
	for _, arg := range args {
		buf = buf[n:]
		n, err = wire.PutByteSlice(buf, arg)
		if err != nil {
			panic(err)
		}
	}
	return tx
}

func makeSet(key, value []byte) []byte {
	return makeTx(TxTypeSet, key, value)
}

func makeRemove(key []byte) []byte {
	return makeTx(TxTypeRm, key)
}

func makeGet(key []byte) []byte {
	return makeTx(TxTypeGet, key)
}

func makeQuery(key []byte, prove bool, height uint64) (reqQuery abci.RequestQuery) {
	reqQuery.Path = "/key"
	reqQuery.Data = key
	reqQuery.Prove = prove
	reqQuery.Height = height
	return
}

func TestAppQueries(t *testing.T) {
	assert := assert.New(t)

	app := NewMerkleEyesApp("", 0)
	com := app.Commit()
	assert.EqualValues([]byte(nil), com.Data)

	// prepare some actions
	key, value := []byte("foobar"), []byte("works!")
	addTx := makeSet(key, value)
	removeTx := makeRemove(key)

	// need to commit append before it shows in queries
	txResult := app.DeliverTx(addTx)
	assert.True(txResult.IsOK(), txResult.Log)
	resQuery := app.Query(makeQuery(key, false, 0))
	assert.True(resQuery.Code.IsOK(), resQuery.Log)
	assert.Equal([]byte(nil), resQuery.Value)
	// but get works before commit
	txResult = app.DeliverTx(makeGet(key))
	assert.True(txResult.IsOK(), txResult.Log)
	assert.EqualValues(txResult.Data, value)

	com = app.Commit()
	hash := com.Data
	assert.NotEqual(t, nil, hash)
	resQuery = app.Query(makeQuery(key, false, 0))
	assert.True(resQuery.Code.IsOK(), resQuery.Log)
	assert.Equal(value, resQuery.Value)
	txResult = app.DeliverTx(makeGet(key))
	assert.EqualValues(txResult.Data, value, txResult.Error())

	com = app.Commit()
	hash = com.Data
	// modifying check has no effect
	check := app.CheckTx(removeTx)
	assert.True(check.IsOK(), check.Log)
	com = app.Commit()
	assert.True(com.IsOK(), com.Log)
	hash2 := com.Data
	assert.Equal(hash, hash2)

	// proofs come from the last commited state, not working state
	txResult = app.DeliverTx(removeTx)
	assert.True(txResult.IsOK(), txResult.Log)
	// currently don't support specifying block height
	resQuery = app.Query(makeQuery(key, true, 1))
	assert.False(resQuery.Code.IsOK(), resQuery.Log)
	resQuery = app.Query(makeQuery(key, true, 0))
	if assert.NotEmpty(resQuery.Value) {
		proof, err := iavl.ReadProof(resQuery.Proof)
		if assert.Nil(err) {
			// TODO: Fix me
			version := 0
			assert.True(proof.Verify(key, resQuery.Value, proof.RootHash, version))
			assert.True(proof.Verify(storeKey(key), resQuery.Value, proof.RootHash, version))
		}
	}

	// commit remove actually removes it now
	com = app.Commit()
	assert.True(com.IsOK(), com.Log)
	hash3 := com.Data
	assert.NotEqual(hash, hash3)

	// nothing here...
	resQuery = app.Query(makeQuery(key, false, 0))
	assert.True(resQuery.Code.IsOK(), resQuery.Log)
	assert.Equal([]byte(nil), resQuery.Value)
	// neither with proof...
	resQuery = app.Query(makeQuery(key, true, 0))
	assert.True(resQuery.Code.IsOK(), resQuery.Log)
	assert.Equal([]byte(nil), resQuery.Value)
	assert.Empty(resQuery.Proof)
	// nor with get
	txResult = app.DeliverTx(makeGet(key))
	assert.True(txResult.IsSameCode(abci.ErrBaseUnknownAddress), txResult.Log)
}
