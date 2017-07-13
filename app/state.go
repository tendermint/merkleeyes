package app

import "github.com/tendermint/tmlibs/merkle"

// State represents the app states, separating the commited state (for queries)
// from the working state (for CheckTx and AppendTx)
type State struct {
	committed  merkle.Tree
	deliverTx  merkle.Tree
	checkTx    merkle.Tree
	persistent bool
}

func NewState(tree merkle.Tree, persistent bool) State {
	return State{
		committed:  tree.Copy(),
		deliverTx:  tree,
		checkTx:    tree.Copy(),
		persistent: persistent,
	}
}

func (s State) Committed() merkle.Tree {
	return s.committed
}

func (s State) Deliver() merkle.Tree {
	return s.deliverTx
}

func (s State) Check() merkle.Tree {
	return s.checkTx
}

// Commit stores the current Deliver() state as committed
// starts new Append/Check state, and
// returns the hash for the commit
func (s *State) Commit() []byte {
	var hash []byte
	if s.persistent {
		hash = s.deliverTx.Save()
	} else {
		hash = s.deliverTx.Hash()
	}

	s.committed = s.deliverTx.Copy()
	s.checkTx = s.deliverTx.Copy()

	return hash
}
