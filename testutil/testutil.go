// Copyright 2016 Tendermint. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutil

import (
	"testing"

	"github.com/tendermint/abci/server"
	wire "github.com/tendermint/go-wire"
	"github.com/tendermint/merkleeyes/app"
	eyes "github.com/tendermint/merkleeyes/client"
	. "github.com/tendermint/tmlibs/common"
)

// NOTE: don't forget to close the client & server.
func CreateEyes(t *testing.T) (svr Service, cli *eyes.Client) {
	addr := "unix://eyes.sock"

	// Start the listener
	mApp := app.NewMerkleEyesApp("", 0)
	svr, err := server.NewServer(addr, "socket", mApp)
	if err != nil {
		(err.Error())
		return
	}

	// Create client
	cli, err = eyes.NewClient(addr)
	if err != nil {
		t.Fatal(err.Error())
		return
	}

	return svr, cli
}

// MakeTxKV returns a text transaction, allong with expected key, value pair
func MakeTxKV() ([]byte, []byte, []byte) {
	k := []byte(RandStr(8))
	v := []byte(RandStr(8))
	return k, v, makeSet(k, v)
}

// blatently copied from merkleeyes/app/app_test.go
// constructs a "set" transaction
func makeSet(key, value []byte) []byte {
	tx := make([]byte, 1+wire.ByteSliceSize(key)+wire.ByteSliceSize(value))
	buf := tx
	buf[0] = app.WriteSet // Set TypeByte
	buf = buf[1:]
	n, err := wire.PutByteSlice(buf, key)
	if err != nil {
		panic(err)
	}
	buf = buf[n:]
	n, err = wire.PutByteSlice(buf, value)
	if err != nil {
		panic(err)
	}
	return tx
}
