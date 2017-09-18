// Copyright 2017 Tendermint. All Rights Reserved.
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

package iavl_test

import (
	"io/ioutil"
	"testing"

	"github.com/tendermint/go-wire"
	"github.com/tendermint/merkleeyes/iavl"
	"github.com/tendermint/tmlibs/common"
	"github.com/tendermint/tmlibs/db"
)

func i2b(i int) []byte {
	bz := make([]byte, 4)
	wire.PutInt32(bz, int32(i))
	return bz
}

func TestIAVLTreeFdump(t *testing.T) {
	t.Skipf("Tree dump and DB code seem buggy so this test always crashes. See https://github.com/tendermint/tmlibs/issues/36")
	db := db.NewDB("test", db.MemDBBackendStr, "")
	tree := iavl.NewIAVLTree(100000, db)
	for i := 0; i < 1000000; i++ {
		tree.Set(i2b(int(common.RandInt32())), nil)
		if i > 990000 && i%1000 == 999 {
			tree.Save()
		}
	}
	tree.Save()

	// insert lots of info and store the bytes
	for i := 0; i < 200; i++ {
		key, value := common.RandStr(20), common.RandStr(200)
		tree.Set([]byte(key), []byte(value))
	}

	tree.Fdump(ioutil.Discard, true, nil)
}
