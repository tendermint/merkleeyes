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

package iavl

import (
	"bytes"
	"fmt"
	"io"
	"os"

	wire "github.com/tendermint/go-wire"
)

type Formatter func(in []byte) (out string)

type KeyValueMapping struct {
	Key   Formatter
	Value Formatter
}

// Flip back and forth between ascii and hex.
func mixedDisplay(value []byte) string {

	var buffer bytes.Buffer
	var last []byte

	ascii := true
	for i := 0; i < len(value); i++ {
		if value[i] < 32 || value[i] > 126 {
			if ascii && len(last) > 0 {
				// only if there are 6 or more chars
				if len(last) > 5 {
					buffer.WriteString(fmt.Sprintf("%s", last))
					last = nil
				}
				ascii = false
			}
		}
		last = append(last, value[i])
	}
	if ascii {
		buffer.WriteString(fmt.Sprintf("%s", last))
	} else {
		buffer.WriteString(fmt.Sprintf("%X", last))
	}
	return buffer.String()
}

// This is merkleeyes state, that it is writing to a specific key
type state struct {
	Hash   []byte
	Height uint64
}

// Try to interpet as merkleeyes state
func stateMapping(value []byte) string {
	var s state
	err := wire.ReadBinaryBytes(value, &s)
	if err != nil || s.Height > 500 {
		return mixedDisplay(value)
	}
	return fmt.Sprintf("Height:%d, [%X]", s.Height, s.Hash)
}

// This is basecoin accounts, that it is writing to a specific key
type account struct {
	PubKey   []byte
	Sequence int
	Balance  []coin
}

type wrapper struct {
	bytes []byte
}

type coin struct {
	Denom  string
	Amount int64
}

// Perhaps this is an IAVL tree node?
func nodeMapping(node *IAVLNode) string {

	formattedKey := mixedDisplay(node.key)

	var formattedValue string
	var acc account

	err := wire.ReadBinaryBytes(node.value, &acc)
	if err != nil {
		formattedValue = mixedDisplay(node.value)
	} else {
		formattedValue = fmt.Sprintf("%v", acc)
	}

	if node.height == 0 {
		return fmt.Sprintf(" LeafNode[height: %d, size %d, key: %s, value: %s]",
			node.height, node.size, formattedKey, formattedValue)
	} else {
		return fmt.Sprintf("InnerNode[height: %d, size %d, key: %s, leftHash: %X, rightHash: %X]",
			node.height, node.size, formattedKey, node.leftHash, node.rightHash)
	}
}

// Try everything and see what sticks...
func overallMapping(value []byte) (str string) {
	// underneath make node, wire can throw a panic
	defer func() {
		if recover() != nil {
			str = fmt.Sprintf("%X", value)
			return
		}
	}()

	// test to see if this is a node
	node, err := MakeIAVLNode(value, nil)

	if err == nil && node.height < 100 && node.key != nil {
		return nodeMapping(node)
	}

	// Unknown value type
	return stateMapping(value)
}

// Dump everything from the database to stdout.
func (t *IAVLTree) Dump(verbose bool, mapping *KeyValueMapping) {
	t.Fdump(os.Stdout, verbose, mapping)
}

// Fdump serializes the entire database to the provided io.Writer.
func (t *IAVLTree) Fdump(w io.Writer, verbose bool, mapping *KeyValueMapping) {
	if verbose && t.root == nil {
		fmt.Fprintf(w, "No root loaded into memory\n")
	}

	if mapping == nil {
		mapping = &KeyValueMapping{Key: mixedDisplay, Value: overallMapping}
	}

	if verbose {
		stats := t.ndb.db.Stats()
		for key, value := range stats {
			fmt.Fprintf(w, "%s:\n\t%s\n", key, value)
		}
	}

	iter := t.ndb.db.Iterator()
	for iter.Next() {
		fmt.Fprintf(w, "%s: %s\n", mapping.Key(iter.Key()), mapping.Value(iter.Value()))
	}
}
