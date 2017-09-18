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

	"github.com/pkg/errors"

	"github.com/tendermint/go-wire/data"
)

var (
	errInvalidProof = fmt.Errorf("invalid proof")

	// ErrInvalidInputs is returned when the inputs passed to the function are invalid.
	ErrInvalidInputs = fmt.Errorf("invalid inputs")

	// ErrInvalidRoot is returned when the root passed in does not match the proof's.
	ErrInvalidRoot = fmt.Errorf("invalid root")

	// ErrNilRoot is returned when the root of the tree is nil.
	ErrNilRoot = fmt.Errorf("tree root is nil")
)

// ErrInvalidProof is returned by Verify when a proof cannot be validated.
func ErrInvalidProof() error {
	return errors.WithStack(errInvalidProof)
}

// KeyProof represents a proof of existence or absence of a single key.
type KeyProof interface {
	// Verify verfies the proof is valid. To verify absence,
	// the value should be nil.
	Verify(key, value, root []byte) error
}

// KeyExistsProof represents a proof of existence of a single key.
type KeyExistsProof struct {
	RootHash   data.Bytes `json:"root_hash"`
	*PathToKey `json:"path"`
}

// Verify verifies the proof is valid and returns an error if it isn't.
func (proof *KeyExistsProof) Verify(key []byte, value []byte, root []byte) error {
	if !bytes.Equal(proof.RootHash, root) {
		return ErrInvalidRoot
	}
	if key == nil || value == nil {
		return ErrInvalidInputs
	}
	return proof.PathToKey.verify(IAVLProofLeafNode{key, value}, root)
}

// KeyAbsentProof represents a proof of the absence of a single key.
type KeyAbsentProof struct {
	RootHash data.Bytes `json:"root_hash"`

	Left  *PathWithNode `json:"left"`
	Right *PathWithNode `json:"right"`
}

func (p *KeyAbsentProof) String() string {
	return fmt.Sprintf("KeyAbsentProof\nroot=%s\nleft=%s%#v\nright=%s%#v\n", p.RootHash, p.Left.Path, p.Left.Node, p.Right.Path, p.Right.Node)
}

// Verify verifies the proof is valid and returns an error if it isn't.
func (proof *KeyAbsentProof) Verify(key, value []byte, root []byte) error {
	if !bytes.Equal(proof.RootHash, root) {
		return ErrInvalidRoot
	}
	if key == nil || value != nil {
		return ErrInvalidInputs
	}

	if proof.Left == nil && proof.Right == nil {
		return ErrInvalidProof()
	}
	if err := verifyPaths(proof.Left, proof.Right, key, key, root); err != nil {
		return err
	}

	return verifyKeyAbsence(proof.Left, proof.Right)
}

func (node *IAVLNode) pathToKey(t *IAVLTree, key []byte) (*PathToKey, []byte, error) {
	path := &PathToKey{}
	val, err := node._pathToKey(t, key, path)
	return path, val, err
}
func (node *IAVLNode) _pathToKey(t *IAVLTree, key []byte, path *PathToKey) ([]byte, error) {
	if node.height == 0 {
		if bytes.Compare(node.key, key) == 0 {
			return node.value, nil
		}
		return nil, errors.New("key does not exist")
	}

	if bytes.Compare(key, node.key) < 0 {
		if value, err := node.getLeftNode(t)._pathToKey(t, key, path); err != nil {
			return nil, err
		} else {
			branch := IAVLProofInnerNode{
				Height: node.height,
				Size:   node.size,
				Left:   nil,
				Right:  node.getRightNode(t).hash,
			}
			path.InnerNodes = append(path.InnerNodes, branch)
			return value, nil
		}
	}

	if value, err := node.getRightNode(t)._pathToKey(t, key, path); err != nil {
		return nil, err
	} else {
		branch := IAVLProofInnerNode{
			Height: node.height,
			Size:   node.size,
			Left:   node.getLeftNode(t).hash,
			Right:  nil,
		}
		path.InnerNodes = append(path.InnerNodes, branch)
		return value, nil
	}
}

func (t *IAVLTree) constructKeyAbsentProof(key []byte, proof *KeyAbsentProof) error {
	// Get the index of the first key greater than the requested key, if the key doesn't exist.
	idx, _, exists := t.Get(key)
	if exists {
		return errors.Errorf("couldn't construct non-existence proof: key 0x%x exists", key)
	}

	var (
		lkey, lval []byte
		rkey, rval []byte
	)
	if idx > 0 {
		lkey, lval = t.GetByIndex(idx - 1)
	}
	if idx <= t.Size()-1 {
		rkey, rval = t.GetByIndex(idx)
	}

	if lkey == nil && rkey == nil {
		return errors.New("couldn't get keys required for non-existence proof")
	}

	if lkey != nil {
		path, _, _ := t.root.pathToKey(t, lkey)
		proof.Left = &PathWithNode{
			Path: path,
			Node: IAVLProofLeafNode{KeyBytes: lkey, ValueBytes: lval},
		}
	}
	if rkey != nil {
		path, _, _ := t.root.pathToKey(t, rkey)
		proof.Right = &PathWithNode{
			Path: path,
			Node: IAVLProofLeafNode{KeyBytes: rkey, ValueBytes: rval},
		}
	}

	return nil
}

func (t *IAVLTree) getWithProof(key []byte) (value []byte, proof *KeyExistsProof, err error) {
	if t.root == nil {
		return nil, nil, ErrNilRoot
	}
	t.root.hashWithCount(t) // Ensure that all hashes are calculated.

	path, value, err := t.root.pathToKey(t, key)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not construct path to key")
	}

	proof = &KeyExistsProof{
		RootHash:  t.root.hash,
		PathToKey: path,
	}
	return value, proof, nil
}

func (t *IAVLTree) keyAbsentProof(key []byte) (*KeyAbsentProof, error) {
	if t.root == nil {
		return nil, ErrNilRoot
	}
	t.root.hashWithCount(t) // Ensure that all hashes are calculated.
	proof := &KeyAbsentProof{
		RootHash: t.root.hash,
	}
	if err := t.constructKeyAbsentProof(key, proof); err != nil {
		return nil, errors.Wrap(err, "could not construct proof of non-existence")
	}
	return proof, nil
}
