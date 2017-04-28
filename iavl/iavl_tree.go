/*
	Package merkle implements a key/value store with an immutable Merkle tree.
*/

package iavl

import (
	"bytes"
	"container/list"
	"fmt"
	"sync"

	wire "github.com/tendermint/go-wire"
	cmn "github.com/tendermint/tmlibs/common"
	"github.com/tendermint/tmlibs/db"
	"github.com/tendermint/tmlibs/merkle"
)

// Keys for administrative persistent data
var rootsKey = []byte("go-merkle.roots")      // Database key for the list of roots
var versionKey = []byte("go-merkle.version")  // Database key for the version
var orphansKey = []byte("go-merkle.orphans/") // Partial database key for each set of orphans
var deletesKey = []byte("go-merkle.deletes")  // Database key for roots to be pruned

// Fixed number of versions
const rootsMax = 3

var lastId = 0

/*
Immutable AVL Tree (wraps the Node root)
This tree is not goroutine safe.
*/
type IAVLTree struct {
	ndb     *nodeDB
	version int
	roots   *list.List
	id      int
}

// NewIAVLTree constructor for both persistent and memory based trees
func NewIAVLTree(cacheSize int, db db.DB) *IAVLTree {
	//fmt.Printf("NewIAVLTree\n")
	lastId++
	if db == nil {
		// In-memory IAVLTree
		return &IAVLTree{
			version: 0,
			roots:   list.New(),
			id:      lastId,
		}
	} else {
		// Persistent IAVLTree
		ndb := newNodeDB(cacheSize, db)
		return &IAVLTree{
			ndb:     ndb,
			version: 0,
			roots:   list.New(),
			id:      lastId,
		}
	}
}

// GetValue is a slow way to get the ith element's value
func GetValue(l *list.List, index int) interface{} {
	for e := l.Front(); e != nil; e = e.Next() {
		index--
		if index <= 0 {
			return e.Value
		}
	}
	return nil
}

// SetValue is a slow way to set the ith element's value
func SetValue(l *list.List, index int, value interface{}) {
	for e := l.Front(); e != nil; e = e.Next() {
		if index <= 0 {
			e.Value = value
			return
		}
		index--
	}

	if index <= 0 {
		l.PushBack(value)
	} else {
		cmn.PanicSanity(fmt.Sprintf("Index not contiguous %d", index))
	}
}

// GetRoot returns the correct root associated with a given version
func (t *IAVLTree) GetRoot(version int) *IAVLNode {
	//fmt.Printf("GetRoot on id=%d version: %d Status: ", t.id, version)

	if t.roots.Len() > rootsMax {
		cmn.PanicSanity("Too many Roots!")
	}

	index := t.version - version

	element := GetValue(t.roots, index)
	if element == nil {
		//fmt.Printf("Missing Root %d for index %d\n", version, index)
		return nil
	}

	root := element.(*IAVLNode)
	root.validate(t)

	return root
}

func (t *IAVLTree) PrintRoots() {
	fmt.Printf("Roots version: %d count: %d\n", t.version, t.roots.Len())
	i := 0
	for e := t.roots.Front(); e != nil; e = e.Next() {

		node := e.Value.(*IAVLNode)
		if node == nil {
			fmt.Printf("%d) <nil>\n", i)
		} else {
			fmt.Printf("%d) %s\n", i, node.Sprintf())
		}

		i++
	}
}

// The returned tree and the original tree are goroutine independent.
// That is, they can each run in their own goroutine.
// However, upon Save(), any other trees that share a db will become
// outdated, as some nodes will become orphaned.
// Note that Save() clears leftNode and rightNode.  Otherwise,
// two copies would not be goroutine independent.
func (t *IAVLTree) Copy() merkle.Tree {
	//fmt.Printf("Copy ")

	root := t.GetRoot(t.version)
	if t.roots == nil || root == nil {
		lastId++
		return &IAVLTree{
			ndb:     t.ndb,
			version: t.version,
			roots:   list.New(),
			id:      lastId,
		}
	}

	if t.ndb != nil && !root.persisted {
		// Saving a tree finalizes all the nodes.
		// It sets all the hashes recursively,
		// clears all the leftNode/rightNode values recursively,
		// and all the .persisted flags get set.
		cmn.PanicSanity("It is unsafe to Copy() an unpersisted tree.")

	} else if t.ndb == nil && root.hash == nil {
		// An in-memory IAVLTree is finalized when the hashes are
		// calculated.
		root.hashWithCount(t)
	}

	lastId++
	tmp := list.New()
	tmp.PushBackList(t.roots)

	return &IAVLTree{
		ndb:     t.ndb,
		version: t.version,
		roots:   tmp,
		id:      lastId,
	}
}

// Size returns the number of internal and leaf nodes in the tree.
func (t *IAVLTree) Size() int {
	//fmt.Printf("Version ")
	root := t.GetRoot(t.version)
	if root == nil {
		return 0
	}
	return root.size
}

// Height returns the maximum tree height.
func (t *IAVLTree) Height() int8 {
	//fmt.Printf("Version ")
	root := t.GetRoot(t.version)
	if root == nil {
		return 0
	}
	return root.height
}

// Version is the latest uncommitted version number.
func (t *IAVLTree) Version() int {
	//fmt.Printf("Version ")
	root := t.GetRoot(t.version)
	if root == nil {
		return 0
	}
	return root.version
}

func (t *IAVLTree) Has(key []byte) bool {
	//fmt.Printf("Has ")
	root := t.GetRoot(t.version)
	if root == nil {
		return false
	}
	return root.has(t, key)
}

// Proof of the latest key
func (t *IAVLTree) Proof(key []byte) (value []byte, proofBytes []byte, exists bool) {
	//fmt.Printf("Proof ")
	value, _, proof := t.ConstructProof(key, t.version)
	if proof == nil {
		//fmt.Printf("Missing Proof\n")
		return nil, nil, false
	}
	proofBytes = wire.BinaryBytes(proof)
	return value, proofBytes, true
}

// Proof of a key at a specific version
func (t *IAVLTree) ProofVersion(key []byte, version int) (value []byte, proofBytes []byte, exists bool) {
	//fmt.Printf("ProofVersion\n")
	value, _, proof := t.ConstructProof(key, version)
	if proof == nil {
		return nil, nil, false
	}
	proofBytes = wire.BinaryBytes(proof)
	return value, proofBytes, true
}

// Set a value with a new key or an existing one
func (t *IAVLTree) Set(key []byte, value []byte) (updated bool) {
	//fmt.Printf("Set %X/%X ", key, value)

	root := t.GetRoot(t.version)
	if root == nil {
		//fmt.Printf("Initializing Tree\n")

		SetValue(t.roots, 0, NewIAVLNode(key, value, t.version))

		return false
	}

	//fmt.Printf("Added new Root\n")
	root.validate(t)
	root, updated = root.set(t, key, value)
	root.validate(t)

	SetValue(t.roots, 0, root)

	root = t.GetRoot(t.version) // TEST

	return updated
}

// Hash returns the root hash for the tree
// Should not be exposed, since it is just a step in saving
func (t *IAVLTree) Hash() []byte {
	//fmt.Printf("Hash ")
	root := t.GetRoot(t.version)
	if root == nil {
		return nil
	}
	hash, _ := root.hashWithCount(t)
	return hash
}

// HashWithCount returns the root hash for the tree and the size of the tree
// Should not be exposed, since it is just a step in saving
func (t *IAVLTree) HashWithCount() ([]byte, int) {
	//fmt.Printf("HashWithCount ")
	root := t.GetRoot(t.version)
	if root == nil {
		return nil, 0
	}
	return root.hashWithCount(t)
}

// Save this version of the tree
func (t *IAVLTree) Save() []byte {
	//t.ndb.mtx.Lock()
	//defer t.ndb.mtx.Unlock()

	//fmt.Printf("****** Save\n")
	//t.PrintRoots()

	// Might be the same
	first := t.roots.Front()
	firstNode := first.Value.(*IAVLNode)

	if firstNode == nil {
		return nil
	} else if t.ndb != nil {
		firstNode.save(t)
	}

	last := t.roots.Back()
	lastNode := last.Value.(*IAVLNode)

	if t.ndb != nil {
		// TODO: should be a loop, if the rootsMax can change
		if rootsMax > 1 && t.roots.Len()+1 > rootsMax {
			//fmt.Printf("##### Cleanup!!! ######\n")

			// TODO: should be locking any changes to deletes?
			t.ndb.deletes = append(t.ndb.deletes, lastNode.hash)
			t.roots.Remove(last)

			t.ndb.SaveDeletes(t.ndb.batch)
		}

		//fmt.Printf("Orphan Count %d\n", len(t.ndb.orphans))
		t.ndb.SaveOrphans(firstNode.hash, t.ndb.orphans)
		t.ndb.orphans = make([][]byte, 0)

		t.roots.PushFront(firstNode)
		t.ndb.SaveRoots(t)
		t.ndb.SaveVersion(t)
		t.version++

		t.ndb.Commit()

		// TODO: Move to a separate go-routine?
		t.ndb.Prune()

	} else {
		t.HashWithCount()
		if rootsMax > 1 && t.roots.Len()+1 > rootsMax {
			t.roots.Remove(last)
			t.roots.PushFront(firstNode)
			t.version++
		}
	}

	// What's the final status?
	//t.PrintRoots()
	root := t.GetRoot(t.version)
	//fmt.Printf("root = '%X'\n", root.hash)

	return root.hash
}

// Sets the root node by reading from db.
// If the hash is empty, then sets root to nil.
func (t *IAVLTree) Load(hash []byte) {
	//fmt.Printf("***** Loading a tree\n")
	if len(hash) == 0 {
		SetValue(t.roots, 0, nil)
	} else {
		//fmt.Printf("Loading Root\n")
		t.ndb.GetVersion(t)
		t.ndb.GetRoots(t)
		t.ndb.GetDeletes()
		SetValue(t.roots, 0, t.ndb.GetNode(t, hash))
	}
}

// GetVersion finds the value for the current version
func (t *IAVLTree) Get(key []byte) (index int, value []byte, exists bool) {
	//fmt.Printf("Get ")
	root := t.GetRoot(t.version)
	if root == nil {
		//fmt.Printf("Failed Get for %d\n", t.version)
		return 0, nil, false
	}
	return root.get(t, key)
}

// GetVersion finds the value for an older committed version
func (t *IAVLTree) GetVersion(key []byte, version int) (index int, value []byte, exists bool) {
	root := t.GetRoot(version)
	if root == nil {
		return 0, nil, false
	}
	return root.get(t, key)
}

// GetByIndex the ith key and value
func (t *IAVLTree) GetByIndex(index int) (key []byte, value []byte) {
	root := t.GetRoot(t.version)
	if root == nil {
		return nil, nil
	}
	return root.getByIndex(t, index)
}

// Remove a key from the tree
func (t *IAVLTree) Remove(key []byte) (value []byte, removed bool) {
	//fmt.Printf("Remove ")

	root := t.GetRoot(t.version)
	if root == nil {
		return nil, false
	}

	newRootHash, newRoot, _, value, removed := root.remove(t, key)
	if !removed {
		return nil, false
	}

	if newRoot == nil && newRootHash != nil {
		SetValue(t.roots, 0, t.ndb.GetNode(t, newRootHash))
	} else {
		SetValue(t.roots, 0, newRoot)
	}

	root = t.GetRoot(t.version) // TEST

	return value, true
}

// Iterate the leaf nodes, applying a function for each
func (t *IAVLTree) Iterate(fn func(key []byte, value []byte) bool) (stopped bool) {
	//fmt.Printf("Iterate ")
	root := t.GetRoot(t.version)
	if root == nil {
		return false
	}
	return root.traverse(t, true, func(node *IAVLNode) bool {
		if node.height == 0 {
			return fn(node.key, node.value)
		} else {
			return false
		}
	})
}

// IterateRange makes a callback for all nodes with key between start and end inclusive
// If either are nil, then it is open on that side (nil, nil is the same as Iterate)
func (t *IAVLTree) IterateRange(start, end []byte, ascending bool, fn func(key []byte, value []byte) bool) (stopped bool) {
	//fmt.Printf("IterateRange ")
	root := t.GetRoot(t.version)
	if root == nil {
		return false
	}
	return root.traverseInRange(t, start, end, ascending, func(node *IAVLNode) bool {
		if node.height == 0 {
			return fn(node.key, node.value)
		} else {
			return false
		}
	})
}

//-----------------------------------------------------------------------------

// nodeDB holds the database and any extra information necessary
type nodeDB struct {
	mtx        sync.Mutex
	cache      map[string]*list.Element
	cacheSize  int
	cacheQueue *list.List
	db         db.DB
	batch      db.Batch
	orphans    [][]byte
	deletes    [][]byte
}

func newNodeDB(cacheSize int, db db.DB) *nodeDB {
	ndb := &nodeDB{
		cache:      make(map[string]*list.Element),
		cacheSize:  cacheSize,
		cacheQueue: list.New(),
		db:         db,
		batch:      db.NewBatch(),
		orphans:    make([][]byte, 0),
		deletes:    make([][]byte, 0),
	}
	return ndb
}

func (ndb *nodeDB) GetNode(t *IAVLTree, hash []byte) *IAVLNode {
	ndb.mtx.Lock()
	defer ndb.mtx.Unlock()

	// Check the cache.
	elem, ok := ndb.cache[string(hash)]
	if ok {
		// Already exists. Move to back of cacheQueue.
		ndb.cacheQueue.MoveToBack(elem)
		return elem.Value.(*IAVLNode)
	} else {
		// Doesn't exist, load.
		buf := ndb.db.Get(hash)
		if len(buf) == 0 {
			ndb.db.Print()
			cmn.PanicSanity(cmn.Fmt("Value missing for key %X", hash))
		}
		node, err := MakeIAVLNode(buf, t)
		if err != nil {
			cmn.PanicCrisis(cmn.Fmt("Error reading IAVLNode. bytes: %X  error: %v", buf, err))
		}
		node.hash = hash
		node.persisted = true
		ndb.cacheNode(node)
		return node
	}
}

func (ndb *nodeDB) SaveNode(t *IAVLTree, node *IAVLNode) {
	ndb.mtx.Lock()
	defer ndb.mtx.Unlock()

	if node.hash == nil {
		cmn.PanicSanity("Expected to find node.hash, but none found.")
	}
	if node.persisted {
		cmn.PanicSanity("Shouldn't be calling save on an already persisted node.")
	}

	// Save node bytes to db
	buf := bytes.NewBuffer(nil)
	_, err := node.writePersistBytes(t, buf)
	if err != nil {
		cmn.PanicCrisis(err)
	}
	ndb.batch.Set(node.hash, buf.Bytes())
	node.persisted = true
	ndb.cacheNode(node)
}

func (ndb *nodeDB) GetRoots(t *IAVLTree) {
	buf := ndb.db.Get(rootsKey)
	if len(buf) == 0 {
		cmn.PanicSanity(cmn.Fmt("Value missing for key %X", rootsKey))
	}

	//fmt.Printf("Have Versions: %X\n", buf)
	count := int(wire.GetInt16(buf))
	buf = buf[2:]

	for i := 0; i < count; i++ {
		bytes, n, err := wire.GetByteSlice(buf)
		if err != nil {
			cmn.PanicSanity(err)
		}
		buf = buf[n:]

		//fmt.Printf("Have Node: %X\n", bytes)
		SetValue(t.roots, i, t.ndb.GetNode(t, bytes))
	}
}

func (ndb *nodeDB) SaveRoots(t *IAVLTree) {
	buf, n, err := bytes.NewBuffer(nil), new(int), new(error)

	wire.WriteInt16(int16(t.roots.Len()), buf, n, err)
	for e := t.roots.Front(); e != nil; e = e.Next() {
		value := e.Value.(*IAVLNode)
		wire.WriteByteSlice(value.hash, buf, n, err)
	}
	ndb.batch.Set(rootsKey, buf.Bytes())
}

func (ndb *nodeDB) GetVersion(t *IAVLTree) {
	buf := ndb.db.Get(versionKey)
	if len(buf) == 0 {
		t.version = 0
	}
	version, _, err := wire.GetVarint(buf)
	if err != nil {
		cmn.PanicSanity(err)
	}
	t.version = version
}

func (ndb *nodeDB) SaveVersion(t *IAVLTree) {
	buf, n, err := bytes.NewBuffer(nil), new(int), new(error)

	wire.WriteVarint(t.version, buf, n, err)
	ndb.batch.Set(versionKey, buf.Bytes())
}

// GetOrphans from the database, their key is the root hash
func (ndb *nodeDB) GetOrphans(hash []byte) [][]byte {
	key := orphansKey
	key = append(key, hash...)

	orphans := make([][]byte, 0)
	//orphans = append(orphans, key) // Must include the original storage key

	buf := ndb.db.Get(key)
	if len(buf) == 0 {
		//fmt.Printf("No orphans for key %X\n", key)
		return nil
	}

	count := int(wire.GetInt32(buf))
	buf = buf[4:]

	for i := 0; i < count; i++ {
		bytes, n, err := wire.GetByteSlice(buf)
		if err != nil {
			cmn.PanicSanity(err)
		}
		buf = buf[n:]
		//fmt.Printf("Have Node: %X\n", bytes)

		orphans = append(orphans, bytes)
	}
	return orphans
}

func (ndb *nodeDB) SaveOrphans(hash []byte, orphans [][]byte) {
	key := orphansKey
	key = append(key, hash...)

	count := int32(len(ndb.orphans))

	if count == 0 {
		// Don't bother saving
		return
	}

	buf, n, err := bytes.NewBuffer(nil), new(int), new(error)
	wire.WriteInt32(count, buf, n, err)
	for i := 0; i < int(count); i++ {
		hash := orphans[i]
		wire.WriteByteSlice(hash, buf, n, err)
	}
	ndb.batch.Set(key, buf.Bytes())
}

// GetDeletes loads the full list of roots that need to be deleted
func (ndb *nodeDB) GetDeletes() {
	buf := ndb.db.Get(deletesKey)
	if len(buf) == 0 {
		//cmn.PanicSanity(cmn.Fmt("Delete list is missing"))
		fmt.Printf("Nothing to delete currently\n")
		return
	}

	//fmt.Printf("Have Versions: %X\n", buf)
	count := int(wire.GetInt32(buf))
	buf = buf[4:]

	for i := 0; i < count; i++ {
		bytes, n, err := wire.GetByteSlice(buf)
		if err != nil {
			cmn.PanicSanity(err)
		}
		buf = buf[n:]

		ndb.deletes = append(ndb.deletes, bytes)
	}
}

// SaveDeletes writes the list of roots to delete into the batch
func (ndb *nodeDB) SaveDeletes(batch db.Batch) {
	count := int32(len(ndb.deletes))

	buf, n, err := bytes.NewBuffer(nil), new(int), new(error)
	wire.WriteInt32(count, buf, n, err)
	for i := 0; i < int(count); i++ {
		hash := ndb.deletes[i]
		wire.WriteByteSlice(hash, buf, n, err)
	}
	batch.Set(deletesKey, buf.Bytes())
}

// Remove node from cache and mark it as an orphan
func (ndb *nodeDB) RemoveNode(t *IAVLTree, node *IAVLNode) {
	ndb.mtx.Lock()
	defer ndb.mtx.Unlock()

	if node.hash == nil {
		cmn.PanicSanity("Expected to find node.hash, but none found.")
	}
	if !node.persisted {
		cmn.PanicSanity("Shouldn't be calling remove on a non-persisted node.")
	}
	elem, ok := ndb.cache[string(node.hash)]
	if ok {
		ndb.cacheQueue.Remove(elem)
		delete(ndb.cache, string(node.hash))
	}
	ndb.orphans = append(ndb.orphans, node.hash)
}

// cacheNode enures that the node remains in memory
func (ndb *nodeDB) cacheNode(node *IAVLNode) {

	// Create entry in cache and append to cacheQueue.
	elem := ndb.cacheQueue.PushBack(node)
	ndb.cache[string(node.hash)] = elem

	// Maybe expire an item.
	if ndb.cacheQueue.Len() > ndb.cacheSize {
		hash := ndb.cacheQueue.Remove(ndb.cacheQueue.Front()).(*IAVLNode).hash
		delete(ndb.cache, string(hash))
	}
}

// Prune removes old orphans from the database
func (ndb *nodeDB) Prune() {

	ndb.db.SetSync(nil, nil)
	ndb.db.DeleteSync(nil)
	batch := ndb.db.NewBatch()

	// Clear out the delete slice from the database
	for i := 0; i < len(ndb.deletes); i++ {

		nodes := ndb.GetOrphans(ndb.deletes[i])
		if nodes != nil {
			for j := 0; j < len(nodes); j++ {
				//fmt.Printf("Delete Node %X\n", nodes[j])
				batch.Delete(nodes[j])
			}
		}

		// Delete the list itself
		key := orphansKey
		key = append(key, ndb.deletes[i]...)
		//fmt.Printf("Delete Key %X\n", key)
		batch.Delete(key)
	}

	//TODO: Should be locking any changes to deletes.
	ndb.deletes = make([][]byte, 0)
	ndb.SaveDeletes(batch)

	//fmt.Printf("Pruning the database\n")
	batch.Write()
}

func (ndb *nodeDB) DeleteAll() {
	iter := ndb.db.Iterator()
	for iter.Next() {
		ndb.db.DeleteSync(iter.Key())
	}
}

func (ndb *nodeDB) BatchDeleteAll() {
	batch := ndb.db.NewBatch()
	iter := ndb.db.Iterator()
	for iter.Next() {
		batch.Delete(iter.Key())
	}
	batch.Write()
}

// Commit the changes to the database
func (ndb *nodeDB) Commit() {
	ndb.mtx.Lock()
	defer ndb.mtx.Unlock()

	// Write saves & orphan deletes
	ndb.batch.Write()

	ndb.batch = ndb.db.NewBatch()
}
