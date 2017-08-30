package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"

	abci "github.com/tendermint/abci/types"
	"github.com/tendermint/go-crypto"
	"github.com/tendermint/go-wire"
	"github.com/tendermint/go-wire/data"
	"github.com/tendermint/merkleeyes/iavl"
	cmn "github.com/tendermint/tmlibs/common"
	dbm "github.com/tendermint/tmlibs/db"
	"github.com/tendermint/tmlibs/merkle"
)

// MerkleEyesApp is a Merkle KV-store served as an ABCI app
type MerkleEyesApp struct {
	abci.BaseApplication

	state  State
	db     dbm.DB
	height uint64

	validators *ValidatorSetState

	changes []*abci.Validator // NOTE: go-wire encoded because thats what Tendermint needs
}

// just make sure we really are an application, if the interface
// ever changes in the future
func (app *MerkleEyesApp) assertApplication() abci.Application {
	return app
}

var eyesStateKey = []byte("merkleeyes:state") // Database key for merkle tree save value db values

// MerkleEyesState contains the latest Merkle root hash and the number of times `Commit` has been called
type MerkleEyesState struct {
	Hash       []byte
	Height     uint64
	Validators *ValidatorSetState
}

type ValidatorSetState struct {
	Version    uint64       `json:"version"`
	Validators []*Validator `json:"validators"` // NOTE: non-go-wire encoded for client convenience
}

type Validator struct {
	PubKey data.Bytes `json:"pub_key"`
	Power  uint64     `json:"power"`
}

func (vss *ValidatorSetState) Has(v *Validator) bool {
	for _, v_ := range vss.Validators {
		if bytes.Equal(v_.PubKey, v.PubKey) {
			return true
		}
	}
	return false
}

func (vss *ValidatorSetState) Remove(v *Validator) {
	vals := make([]*Validator, 0, len(vss.Validators)-1)
	for _, v_ := range vss.Validators {
		if !bytes.Equal(v_.PubKey, v.PubKey) {
			vals = append(vals, v_)
		}
	}
	vss.Validators = vals
}

func (vss *ValidatorSetState) Set(v *Validator) {
	for i, v_ := range vss.Validators {
		if bytes.Equal(v_.PubKey, v.PubKey) {
			vss.Validators[i] = v
			return
		}
	}
	vss.Validators = append(vss.Validators, v)
}

// Transaction type bytes
const (
	TxTypeSet           byte = 0x01
	TxTypeRm            byte = 0x02
	TxTypeGet           byte = 0x03
	TxTypeCompareAndSet byte = 0x04
	TxTypeValSetChange  byte = 0x05
	TxTypeValSetRead    byte = 0x06
	TxTypeValSetCAS     byte = 0x07

	NonceLength = 12
)

// NewMerkleEyesApp initializes the database, loads any existing state, and returns a new MerkleEyesApp
func NewMerkleEyesApp(dbName string, cacheSize int) *MerkleEyesApp {
	// start at 1 so the height returned by query is for the
	// next block, ie. the one that includes the AppHash for our current state
	initialHeight := uint64(1)

	// Non-persistent case
	if dbName == "" {
		tree := iavl.NewIAVLTree(
			0,
			nil,
		)
		return &MerkleEyesApp{
			state:  NewState(tree, false),
			db:     nil,
			height: initialHeight,
		}
	}

	// Setup the persistent merkle tree
	empty, _ := cmn.IsDirEmpty(path.Join(dbName, dbName+".db"))

	// Open the db, if the db doesn't exist it will be created
	db := dbm.NewDB(dbName, dbm.LevelDBBackendStr, dbName)

	// Load Tree
	tree := iavl.NewIAVLTree(cacheSize, db)

	if empty {
		fmt.Println("no existing db, creating new db")
		db.Set(eyesStateKey, wire.BinaryBytes(MerkleEyesState{
			Hash:   tree.Save(),
			Height: initialHeight,
		}))
	} else {
		fmt.Println("loading existing db")
	}

	// Load merkle state
	eyesStateBytes := db.Get(eyesStateKey)
	var eyesState MerkleEyesState
	err := wire.ReadBinaryBytes(eyesStateBytes, &eyesState)
	if err != nil {
		fmt.Println("error reading MerkleEyesState")
		panic(err.Error())
	}
	tree.Load(eyesState.Hash)

	return &MerkleEyesApp{
		state:      NewState(tree, true),
		db:         db,
		height:     eyesState.Height,
		validators: new(ValidatorSetState),
	}
}

// CloseDB closes the database
func (app *MerkleEyesApp) CloseDB() {
	if app.db != nil {
		app.db.Close()
	}
}

// Info implements abci.Application
func (app *MerkleEyesApp) Info() abci.ResponseInfo {
	return abci.ResponseInfo{Data: "merkleeyes"}
}

// SetOption implements abci.Application
func (app *MerkleEyesApp) SetOption(key string, value string) (log string) {
	return "No options are supported yet"
}

// DeliverTx implements abci.Application
func (app *MerkleEyesApp) DeliverTx(tx []byte) abci.Result {
	tree := app.state.Deliver()
	r := app.doTx(tree, tx)
	if r.IsErr() {
		fmt.Println("DeliverTx Err", r)
	}
	return r
}

// CheckTx implements abci.Application
func (app *MerkleEyesApp) CheckTx(tx []byte) abci.Result {
	//	return abci.OK
	tree := app.state.Check()
	return app.doTx(tree, tx)
}

func nonceKey(nonce []byte) []byte {
	return append([]byte("/nonce/"), nonce...)
}

func storeKey(key []byte) []byte {
	return append([]byte("/key/"), key...)
}

func (app *MerkleEyesApp) doTx(tree merkle.Tree, tx []byte) abci.Result {
	// minimum length is 12 (nonce) + 1 (type byte) = 13
	minTxLen := NonceLength + 1
	if len(tx) < minTxLen {
		return abci.ErrEncodingError.SetLog(fmt.Sprintf("Tx length must be at least %d", minTxLen))
	}

	nonce := tx[:12]
	tx = tx[12:]

	// check nonce
	_, _, exists := tree.Get(nonceKey(nonce))
	if exists {
		return abci.ErrBadNonce.AppendLog(fmt.Sprintf("Nonce %X already exists", nonce))
	}

	// set nonce
	tree.Set(nonceKey(nonce), []byte("found"))

	typeByte := tx[0]
	tx = tx[1:]
	switch typeByte {
	case TxTypeSet: // Set
		key, n, err := wire.GetByteSlice(tx)
		if err != nil {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Error reading key: %v", err.Error()))
		}
		tx = tx[n:]
		value, n, err := wire.GetByteSlice(tx)
		if err != nil {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Error reading value: %v", err.Error()))
		}
		tx = tx[n:]
		if len(tx) != 0 {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Got bytes left over"))
		}

		tree.Set(storeKey(key), value)

		fmt.Println("SET", cmn.Fmt("%X", key), cmn.Fmt("%X", value))
	case TxTypeRm: // Remove
		key, n, err := wire.GetByteSlice(tx)
		if err != nil {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Error reading key: %v", err.Error()))
		}
		tx = tx[n:]
		if len(tx) != 0 {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Got bytes left over"))
		}
		tree.Remove(storeKey(key))
		fmt.Println("RM", cmn.Fmt("%X", key))
	case TxTypeGet: // Get
		key, n, err := wire.GetByteSlice(tx)
		if err != nil {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Error reading key: %v", err.Error()))
		}
		tx = tx[n:]
		if len(tx) != 0 {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Got bytes left over"))
		}

		_, value, exists := tree.Get(storeKey(key))
		if exists {
			fmt.Println("GET", cmn.Fmt("%X", key), cmn.Fmt("%X", value))
			return abci.OK.SetData(value)
		} else {
			return abci.ErrBaseUnknownAddress.AppendLog(fmt.Sprintf("Cannot find key: %X", key))
		}
	case TxTypeCompareAndSet: // Compare and Set
		key, n, err := wire.GetByteSlice(tx)
		if err != nil {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Error reading key: %v", err.Error()))
		}
		tx = tx[n:]

		compareValue, n, err := wire.GetByteSlice(tx)
		if err != nil {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Error reading compare value: %v", err.Error()))
		}
		tx = tx[n:]

		setValue, n, err := wire.GetByteSlice(tx)
		if err != nil {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Error reading set value: %v", err.Error()))
		}
		tx = tx[n:]

		if len(tx) != 0 {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Got bytes left over"))
		}

		_, value, exists := tree.Get(storeKey(key))
		if !exists {
			return abci.ErrBaseUnknownAddress.AppendLog(fmt.Sprintf("Cannot find key: %X", key))
		}
		if !bytes.Equal(value, compareValue) {
			return abci.ErrUnauthorized.AppendLog(fmt.Sprintf("Value was %X, not %X", value, compareValue))
		}
		tree.Set(storeKey(key), setValue)

		fmt.Println("CAS-SET", cmn.Fmt("%X", key), cmn.Fmt("%X", compareValue), cmn.Fmt("%X", setValue))
	case TxTypeValSetChange:
		pubKey, n, err := wire.GetByteSlice(tx)
		if err != nil {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Error reading pubkey: %v", err.Error()))
		}
		if len(pubKey) != 32 {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Pubkey must be 32 bytes: %X is %d bytes", pubKey, len(pubKey)))
		}
		tx = tx[n:]
		if len(tx) != 8 {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Power must be 8 bytes: %X is %d bytes", tx, len(tx)))
		}
		power := wire.GetUint64(tx)

		return app.updateValidator(pubKey, power)

	case TxTypeValSetRead:
		b, err := json.Marshal(app.validators)
		if err != nil {
			return abci.ErrInternalError.SetLog(cmn.Fmt("Error marshalling validator info: %v", err))
		}
		return abci.OK.SetData(b).SetLog(string(b))

	case TxTypeValSetCAS:
		if len(tx) < 8 {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Version number must be 8 bytes: remaining tx (%X) is %d bytes", tx, len(tx)))
		}
		version := wire.GetUint64(tx)
		if app.validators.Version != version {
			return abci.ErrUnauthorized.AppendLog(fmt.Sprintf("Version was %d, not %d", app.validators.Version, version))
		}
		tx = tx[8:]

		pubKey, n, err := wire.GetByteSlice(tx)
		if err != nil {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Error reading pubkey: %v", err.Error()))
		}
		if len(pubKey) != 32 {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Pubkey must be 32 bytes: %X is %d bytes", pubKey, len(pubKey)))
		}
		tx = tx[n:]
		if len(tx) != 8 {
			return abci.ErrEncodingError.SetLog(cmn.Fmt("Power must be 8 bytes: %X is %d bytes", tx, len(tx)))
		}
		power := wire.GetUint64(tx)

		return app.updateValidator(pubKey, power)

	default:
		return abci.ErrUnknownRequest.SetLog(cmn.Fmt("Unexpected Tx type byte %X", typeByte))
	}
	return abci.OK
}

func (app *MerkleEyesApp) updateValidator(pubKey []byte, power uint64) abci.Result {
	v := &Validator{pubKey, power}
	if v.Power == 0 {
		// remove validator
		if !app.validators.Has(v) {
			return abci.ErrUnauthorized.SetLog(cmn.Fmt("Cannot remove non-existent validator %v", v))
		}
		app.validators.Remove(v)
	} else {
		// add or update validator
		app.validators.Set(v)
	}

	// copy to PubKeyEd25519 so we can go-wire encode properly for the changes array
	var pubKeyEd crypto.PubKeyEd25519
	copy(pubKeyEd[:], pubKey)
	app.changes = append(app.changes, &abci.Validator{pubKeyEd.Bytes(), power})

	return abci.OK
}

func (app *MerkleEyesApp) InitChain(validators []*abci.Validator) {
	for _, v := range validators {
		// want non-go-wire encoded for the state
		p, _ := crypto.PubKeyFromBytes(v.PubKey)
		pubKey := p.Unwrap().(crypto.PubKeyEd25519)
		app.validators.Set(&Validator{pubKey[:], v.Power})
	}
}

func (app *MerkleEyesApp) BeginBlock(hash []byte, header *abci.Header) {
	// reset valset changes
	app.changes = make([]*abci.Validator, 0)
}

func (app *MerkleEyesApp) EndBlock(height uint64) (resEndBlock abci.ResponseEndBlock) {
	if len(app.changes) > 0 {
		app.validators.Version++
	}
	return abci.ResponseEndBlock{Diffs: app.changes}
}

// Commit implements abci.Application
func (app *MerkleEyesApp) Commit() abci.Result {

	hash := app.state.Commit()

	app.height++
	if app.db != nil {
		app.db.Set(eyesStateKey, wire.BinaryBytes(MerkleEyesState{
			Hash:       hash,
			Height:     app.height,
			Validators: app.validators,
		}))
	}

	if app.state.Committed().Size() == 0 {
		return abci.NewResultOK(nil, "Empty hash for empty tree")
	}
	return abci.NewResultOK(hash, "")
}

// Query implements abci.Application
func (app *MerkleEyesApp) Query(reqQuery abci.RequestQuery) (resQuery abci.ResponseQuery) {
	if len(reqQuery.Data) == 0 {
		return
	}
	tree := app.state.Committed()

	if reqQuery.Height != 0 {
		// TODO: support older commits
		resQuery.Code = abci.CodeType_InternalError
		resQuery.Log = "merkleeyes only supports queries on latest commit"
		return
	}

	// set the query response height
	resQuery.Height = app.height

	switch reqQuery.Path {
	case "/store", "/key": // Get by key
		key := reqQuery.Data // Data holds the key bytes
		resQuery.Key = key
		if reqQuery.Prove {
			value, proof, exists := tree.Proof(storeKey(key))
			if !exists {
				resQuery.Log = "Key not found"
			}
			resQuery.Value = value
			resQuery.Proof = proof
			// TODO: return index too?
		} else {
			index, value, _ := tree.Get(storeKey(key))
			resQuery.Value = value
			resQuery.Index = int64(index)
		}

	case "/index": // Get by Index
		index := wire.GetInt64(reqQuery.Data)
		key, value := tree.GetByIndex(int(index))
		resQuery.Key = key
		resQuery.Index = int64(index)
		resQuery.Value = value

	case "/size": // Get size
		size := tree.Size()
		sizeBytes := wire.BinaryBytes(size)
		resQuery.Value = sizeBytes

	default:
		resQuery.Code = abci.CodeType_UnknownRequest
		resQuery.Log = cmn.Fmt("Unexpected Query path: %v", reqQuery.Path)
	}
	return
}
