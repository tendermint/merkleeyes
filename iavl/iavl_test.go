package iavl

import (
	"bytes"
	"fmt"
	mrand "math/rand"
	"sort"
	"sync"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ripemd160"

	wire "github.com/tendermint/go-wire"
	cmn "github.com/tendermint/tmlibs/common"
	"github.com/tendermint/tmlibs/db"
	test "github.com/tendermint/tmlibs/test"

	"runtime"
	"testing"
)

// List out the major operations
const (
	SETVALUE = iota
	REMOVEVALUE
	SAVETREE
	COPYTREE
)

// A given action in the tests
type action struct {
	cmd     int
	pair    *pair
	status  bool
	comment string
}

// Convenience struct for keeping the data together
type pair struct {
	key   string
	value string
}

// Set to true if you want output
var verbose bool = false

// processActions and check their results
func processActions(t *testing.T, tree *IAVLTree, actions []action) *IAVLTree {

	for i := range actions {
		tree = processAction(t, tree, i, &actions[i])
	}
	return tree
}

func processAction(t *testing.T, tree *IAVLTree, index int, next *action) *IAVLTree {

	var status bool

	switch next.cmd {
	case SETVALUE:
		if verbose {
			fmt.Printf("Doing a Set for %x\n", next.pair.key)
		}
		status = tree.Set([]byte(next.pair.key), []byte(next.pair.value))

	case REMOVEVALUE:
		if verbose {
			fmt.Printf("Doing a Remove %x\n", next.pair.key)
		}
		_, status = tree.Remove([]byte(next.pair.key))

	case SAVETREE:
		if verbose {
			fmt.Printf("Doing a Save for the Tree\n")
		}
		bytes := tree.Save()
		status = bytes != nil

	case COPYTREE:
		if verbose {
			fmt.Printf("Doing a Copy of the Tree\n")
		}
		tree = tree.Copy().(*IAVLTree)
		status = true
	}

	if status != next.status {
		text := fmt.Sprintf("%d) %s status=%v", index, next.comment, status)
		t.Error(text)
	}

	return tree
}

func NextKey(series string, index int, lastHash []byte) []byte {
	var buffer []byte

	buffer = append(buffer, series...)
	buffer = append(buffer, string(index)...)

	if lastHash != nil {
		buffer = append(buffer, lastHash...)
	}

	hasher := ripemd160.New()
	hasher.Write(buffer)
	hash := hasher.Sum(nil)

	return hash
}

var pairs []pair = []pair{
	pair{"aaaaa", "value of a"},
	pair{"bbbbb", "value of b"},
	pair{"ccccc", "value of c"},
	pair{"ddddd", "value of d"},
	pair{"00001", "value of one"},
	pair{"00002", "value of two"},
	pair{"00003", "value of three"},
	pair{"00004", "value of four"},
}

// Roughly ma
func TestCopy(t *testing.T) {
	copy := []action{
		action{COPYTREE, nil, true, "Should do a copy"},
	}

	db := db.NewDB("TestCopy", "goleveldb", "./")
	var tree *IAVLTree = NewIAVLTree(0, db)

	// make sure there is nothing in this database if it already exists
	tree.ndb.BatchDeleteAll()

	for i := 0; i < 10; i++ {
		// Make a copy of the tree
		other_tree := processActions(t, tree, copy)

		size := 3000
		start := i * size
		stop := (i + 1) * size

		var group sync.WaitGroup
		group.Add(2)

		// Essentially the main copy of the tree, lets add lots of stuff
		go func() {
			defer group.Done()
			var hash []byte
			for step := start; step < stop; step++ {
				hash = NextKey("A", step, hash)
				act := &action{SETVALUE, &pair{string(hash), "value of " + string(hash)}, false, "Create"}
				tree = processAction(t, tree, step, act)
			}
		}()

		// A second copy in memory, repeat the first work, but also add in a second set
		go func() {
			defer group.Done()
			var hash1 []byte
			var hash2 []byte
			for step := start; step < stop; step++ {

				hash1 = NextKey("B", step, hash1)
				act1 := &action{SETVALUE, &pair{string(hash1), "value of " + string(hash1)}, false, "Create"}
				other_tree = processAction(t, other_tree, step, act1)

				hash2 = NextKey("A", step, hash2)
				act2 := &action{SETVALUE, &pair{string(hash2), "value of " + string(hash2)}, false, "Create"}
				other_tree = processAction(t, other_tree, step, act2)
			}
		}()

		// Wait until both have completed
		group.Wait()

		// Commit the mess
		act := &action{SAVETREE, nil, true, "Save"}
		tree = processAction(t, tree, 0, act)
	}

	// Okay, now lets see if we can delete everything
	var hash []byte
	for step := 0; step < 3000*10; step++ {
		if step%3000 == 0 {
			hash = nil
		}
		hash = NextKey("A", step, hash)
		act := &action{REMOVEVALUE, &pair{string(hash), "value of " + string(hash)}, true, "Create"}
		tree = processAction(t, tree, step, act)
	}

	// And Save it, a lot forcing pruning.
	for i := 0; i < 1001; i++ {
		act := &action{SAVETREE, nil, false, "Save"}
		tree = processAction(t, tree, 0, act)
	}
}

func TestTable(t *testing.T) {

	actions := []action{
		// Add four values
		action{SETVALUE, &pairs[0], false, "Should be a create"},
		action{SETVALUE, &pairs[1], false, "Should be a create"},
		action{SETVALUE, &pairs[2], false, "Should be a create"},
		action{SETVALUE, &pairs[3], false, "Should be a create"},
		action{SAVETREE, nil, true, "Should return a value"},

		// Modify two
		action{SETVALUE, &pairs[2], true, "Should be an update"},
		action{SETVALUE, &pairs[3], true, "Should be an update"},
		action{SETVALUE, &pairs[4], false, "Should be an create"},
		action{SAVETREE, nil, true, "Should return a value"},

		// Mess around
		action{REMOVEVALUE, &pairs[2], true, "Should remove"},
		action{SETVALUE, &pairs[2], false, "Should be an create"},
		action{SETVALUE, &pairs[3], true, "Should be an update"},
		action{SAVETREE, nil, true, "Should return a value"},

		action{SETVALUE, &pairs[4], true, "Should be a update"},
		action{SETVALUE, &pairs[5], false, "Should be a create"},
		action{SETVALUE, &pairs[6], false, "Should be a create"},
		action{SETVALUE, &pairs[7], false, "Should be a create"},
		action{SAVETREE, nil, true, "Should return a value"},

		action{REMOVEVALUE, &pairs[0], true, "Should remove"},
		action{REMOVEVALUE, &pairs[1], true, "Should remove"},
		action{REMOVEVALUE, &pairs[2], true, "Should remove"},
		action{REMOVEVALUE, &pairs[3], true, "Should remove"},
		action{REMOVEVALUE, &pairs[4], true, "Should remove"},
		action{REMOVEVALUE, &pairs[5], true, "Should remove"},
		action{REMOVEVALUE, &pairs[6], true, "Should remove"},
		action{REMOVEVALUE, &pairs[7], true, "Should remove"},
		action{SAVETREE, nil, false, "Should not return a value"},
		action{SAVETREE, nil, false, "Should not return a value"},
		action{SAVETREE, nil, false, "Should not return a value"},
		action{SAVETREE, nil, false, "Should not return a value"},
		action{SAVETREE, nil, false, "Should not return a value"},
		action{SAVETREE, nil, false, "Should not return a value"},
		action{SAVETREE, nil, false, "Should not return a value"},
		action{SAVETREE, nil, false, "Should not return a value"},
	}

	db := db.NewDB("TestTable", "goleveldb", "./")
	var tree *IAVLTree = NewIAVLTree(0, db)

	// make sure there is nothing in this database if it already exists
	tree.ndb.DeleteAll()

	for i := 0; i < 10; i++ {
		processActions(t, tree, actions)
	}
}

func TestGrowth(t *testing.T) {

	initial_actions := []action{
		action{SETVALUE, &pairs[0], false, "Should be a create"},
		action{SETVALUE, &pairs[1], false, "Should be a create"},
		action{SETVALUE, &pairs[2], false, "Should be a create"},
		action{SETVALUE, &pairs[3], false, "Should be a create"},
		action{SETVALUE, &pairs[4], false, "Should be a create"},
		action{SETVALUE, &pairs[5], false, "Should be a create"},
		action{SETVALUE, &pairs[6], false, "Should be a create"},
		action{SETVALUE, &pairs[7], false, "Should be a create"},
		action{SAVETREE, nil, true, "Should return a value"},
	}

	actions := []action{
		action{SETVALUE, &pairs[0], true, "Should be a update"},
		action{SETVALUE, &pairs[1], true, "Should be a update"},
		action{SETVALUE, &pairs[2], true, "Should be a update"},
		action{SETVALUE, &pairs[3], true, "Should be a update"},
		action{SETVALUE, &pairs[4], true, "Should be a update"},
		action{SETVALUE, &pairs[5], true, "Should be a update"},
		action{SETVALUE, &pairs[6], true, "Should be a update"},
		action{SETVALUE, &pairs[7], true, "Should be a update"},
		action{SAVETREE, nil, true, "Should return a value"},
	}

	final_actions := []action{
		action{REMOVEVALUE, &pairs[0], true, "Should remove"},
		action{REMOVEVALUE, &pairs[1], true, "Should remove"},
		action{REMOVEVALUE, &pairs[2], true, "Should remove"},
		action{REMOVEVALUE, &pairs[3], true, "Should remove"},
		action{REMOVEVALUE, &pairs[4], true, "Should remove"},
		action{REMOVEVALUE, &pairs[5], true, "Should remove"},
		action{REMOVEVALUE, &pairs[6], true, "Should remove"},
		action{REMOVEVALUE, &pairs[7], true, "Should remove"},

		// Clear it enough to force all orphans to be deleted
		action{SAVETREE, nil, false, "Should not return a value"},
	}

	clear := []action{
		action{SAVETREE, nil, false, "Should not return a value"},
	}

	db := db.NewDB("TestGrowth", "goleveldb", "./")
	var tree *IAVLTree = NewIAVLTree(0, db)

	// make sure there is nothing in this database if it already exists
	tree.ndb.DeleteAll()

	processActions(t, tree, initial_actions)

	for i := 0; i < 1000; i++ {
		processActions(t, tree, actions)
	}

	processActions(t, tree, final_actions)

	for i := 0; i < 1000; i++ {
		processActions(t, tree, clear)
	}
}

func TestEmptyTable(t *testing.T) {

	actions := []action{
		action{SAVETREE, nil, false, "Should not return a value"},
		action{SAVETREE, nil, false, "Should not return a value"},
		action{SAVETREE, nil, false, "Should not return a value"},

		// Add four values
		action{SETVALUE, &pairs[0], false, "Should be a create"},
		action{REMOVEVALUE, &pairs[0], true, "Should remove"},
		action{SAVETREE, nil, false, "Should not return a value"},

		action{SETVALUE, &pairs[0], false, "Should be a create"},
		action{SAVETREE, nil, true, "Should return a value"},

		action{REMOVEVALUE, &pairs[0], true, "Should remove"},
		action{SETVALUE, &pairs[0], false, "Should be a create"},
		action{SAVETREE, nil, true, "Should return a value"},

		action{REMOVEVALUE, &pairs[0], true, "Should remove"},
		action{SAVETREE, nil, false, "Should not return a value"},

		action{SETVALUE, &pairs[0], false, "Should be a create"},
		action{SAVETREE, nil, true, "Should return a value"},
		action{REMOVEVALUE, &pairs[0], true, "Should remove"},
		action{SAVETREE, nil, false, "Should not return a value"},

		action{SAVETREE, nil, false, "Should not return a value"},
		action{SAVETREE, nil, false, "Should not return a value"},
		action{SAVETREE, nil, false, "Should not return a value"},
	}

	db := db.NewDB("TestEmptyTable", "goleveldb", "./")
	var tree *IAVLTree = NewIAVLTree(0, db)

	// make sure there is nothing in this database if it already exists
	tree.ndb.DeleteAll()

	for i := 0; i < 1; i++ {
		processActions(t, tree, actions)
	}
}

const testReadLimit = 1 << 20 // Some reasonable limit for wire.Read*() lmt

func randstr(length int) string {
	return cmn.RandStr(length)
}

func i2b(i int) []byte {
	bz := make([]byte, 4)
	wire.PutInt32(bz, int32(i))
	return bz
}

func b2i(bz []byte) int {
	i := wire.GetInt32(bz)
	return int(i)
}

// Convenience for a new node
func N(l, r interface{}) *IAVLNode {
	var left, right *IAVLNode
	if _, ok := l.(*IAVLNode); ok {
		left = l.(*IAVLNode)
	} else {
		left = NewIAVLNode(i2b(l.(int)), nil, 0)
	}
	if _, ok := r.(*IAVLNode); ok {
		right = r.(*IAVLNode)
	} else {
		right = NewIAVLNode(i2b(r.(int)), nil, 0)
	}

	n := &IAVLNode{
		key:       right.lmd(nil).key,
		value:     nil,
		leftNode:  left,
		rightNode: right,
	}
	n.calcHeightAndSize(nil)
	return n
}

// Setup a deep node
func T(n *IAVLNode) *IAVLTree {
	d := db.NewDB("test", db.MemDBBackendStr, "")
	t := NewIAVLTree(0, d)

	n.hashWithCount(t)
	SetValue(t.roots, 0, n)
	return t
}

// Convenience for simple printing of keys & tree structure
func P(n *IAVLNode) string {
	if n.height == 0 {
		return fmt.Sprintf("%v", b2i(n.key))
	} else {
		return fmt.Sprintf("(%v %v)", P(n.leftNode), P(n.rightNode))
	}
}

func UnfinishedTestOrder(t *testing.T) {
	var tree *IAVLTree = NewIAVLTree(0, nil)
	var up bool
	up = tree.Set([]byte("aaaaaa"), []byte("aaaaaa"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	up = tree.Set([]byte("bbbbbb"), []byte("bbbbbb"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	up = tree.Set([]byte("cccccc"), []byte("cccccc"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	up = tree.Set([]byte("dddddd"), []byte("dddddd"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	up = tree.Set([]byte("eeeeee"), []byte("eeeeeee"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	up = tree.Set([]byte("ffffff"), []byte("fffffff"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	up = tree.Set([]byte("gggggg"), []byte("ggggggg"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	for i := 0; i < 7; i++ {
		key, value := tree.GetByIndex(i)
		fmt.Printf("%d) %s - %s\n", i, string(key), string(value))
	}
}

func TestHistory(t *testing.T) {
	var tree *IAVLTree = NewIAVLTree(0, nil)
	var up bool
	up = tree.Set([]byte("1"), []byte("one"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	_, up = tree.Remove([]byte("1"))
	if !up {
		t.Error("Did not remove")
	}
	up = tree.Set([]byte("1"), []byte("one"))
	if up {
		t.Error("Expected an update (should have been create)")
	}
	bytes := tree.Save()
	if bytes == nil {
		t.Error("Saved should have worked, but got nothing")
	}
	up = tree.Set([]byte("1"), []byte("one"))
	if !up {
		t.Error("Expected an update (not a create)")
	}
	up = tree.Set([]byte("2"), []byte("two"))
	if up {
		t.Error("Expected an update (not a create)")
	}
	bytes = tree.Save()
	if bytes == nil {
		t.Error("Saved should have worked, but got nothing")
	}
	_, up = tree.Remove([]byte("1"))
	if !up {
		t.Error("Did not remove")
	}
	_, up = tree.Remove([]byte("2"))
	if !up {
		t.Error("Did not remove")
	}
	bytes = tree.Save()
	if bytes != nil {
		t.Error("Tree should have been empty")
	}
	up = tree.Set([]byte("1"), []byte("one"))
	if up {
		t.Error("Expected an create (not an update)")
	}
	bytes = tree.Save()
	if bytes == nil {
		t.Error("Saved should have worked, but got nothing")
	}
}

func TestBasic(t *testing.T) {
	var tree *IAVLTree = NewIAVLTree(0, nil)
	var up bool
	up = tree.Set([]byte("1"), []byte("one"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	up = tree.Set([]byte("2"), []byte("two"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}
	up = tree.Set([]byte("2"), []byte("TWO"))
	if !up {
		t.Error("Expected an update")
	}
	up = tree.Set([]byte("5"), []byte("five"))
	if up {
		t.Error("Did not expect an update (should have been create)")
	}

	// Test 0x00
	{
		idx, val, exists := tree.Get([]byte{0x00})
		if exists {
			t.Errorf("Expected no value to exist")
		}
		if idx != 0 {
			t.Errorf("Unexpected idx %x", idx)
		}
		if string(val) != "" {
			t.Errorf("Unexpected value %v", string(val))
		}
	}

	// Test "1"
	{
		idx, val, exists := tree.Get([]byte("1"))
		if !exists {
			t.Errorf("Expected value to exist")
		}
		if idx != 0 {
			t.Errorf("Unexpected idx %x", idx)
		}
		if string(val) != "one" {
			t.Errorf("Unexpected value %v", string(val))
		}
	}

	// Test "2"
	{
		idx, val, exists := tree.Get([]byte("2"))
		if !exists {
			t.Errorf("Expected value to exist")
		}
		if idx != 1 {
			t.Errorf("Unexpected idx %x", idx)
		}
		if string(val) != "TWO" {
			t.Errorf("Unexpected value %v", string(val))
		}
	}

	// Test "4"
	{
		idx, val, exists := tree.Get([]byte("4"))
		if exists {
			t.Errorf("Expected no value to exist")
		}
		if idx != 2 {
			t.Errorf("Unexpected idx %x", idx)
		}
		if string(val) != "" {
			t.Errorf("Unexpected value %v", string(val))
		}
	}
}

func TestUnit(t *testing.T) {

	expectHash := func(tree *IAVLTree, hashCount int) {
		// ensure number of new hash calculations is as expected.
		hash, count := tree.HashWithCount()
		if count != hashCount {
			t.Fatalf("Expected %v new hashes, got %v", hashCount, count)
		}
		// nuke hashes and reconstruct hash, ensure it's the same.
		root := tree.GetRoot(tree.version)
		root.traverse(tree, true, func(node *IAVLNode) bool {
			node.hash = nil
			return false
		})
		// ensure that the new hash after nuking is the same as the old.
		newHash, _ := tree.HashWithCount()
		if bytes.Compare(hash, newHash) != 0 {
			t.Fatalf("Expected hash %v but got %v after nuking", hash, newHash)
		}
	}

	expectSet := func(tree *IAVLTree, i int, repr string, hashCount int) {
		origNode := tree.GetRoot(tree.version)
		updated := tree.Set(i2b(i), nil)
		// ensure node was added & structure is as expected.
		if updated == true || P(tree.GetRoot(tree.version)) != repr {
			t.Fatalf("Adding %v to %v:\nExpected         %v\nUnexpectedly got %v updated:%v",
				i, P(origNode), repr, P(tree.GetRoot(tree.version)), updated)
		}
		// ensure hash calculation requirements
		expectHash(tree, hashCount)
		SetValue(tree.roots, 0, origNode)
	}

	expectRemove := func(tree *IAVLTree, i int, repr string, hashCount int) {
		origNode := tree.GetRoot(tree.version)
		value, removed := tree.Remove(i2b(i))
		// ensure node was added & structure is as expected.
		if len(value) != 0 || !removed || P(tree.GetRoot(tree.version)) != repr {
			t.Fatalf("Removing %v from %v:\nExpected         %v\nUnexpectedly got %v value:%v removed:%v",
				i, P(origNode), repr, P(tree.GetRoot(tree.version)), value, removed)
		}
		// ensure hash calculation requirements
		expectHash(tree, hashCount)
		SetValue(tree.roots, 0, origNode)
	}

	//////// Test Set cases:

	// Case 1:
	t1 := T(N(4, 20))

	expectSet(t1, 8, "((4 8) 20)", 3)
	expectSet(t1, 25, "(4 (20 25))", 3)

	t2 := T(N(4, N(20, 25)))

	expectSet(t2, 8, "((4 8) (20 25))", 3)
	expectSet(t2, 30, "((4 20) (25 30))", 4)

	t3 := T(N(N(1, 2), 6))

	expectSet(t3, 4, "((1 2) (4 6))", 4)
	expectSet(t3, 8, "((1 2) (6 8))", 3)

	t4 := T(N(N(1, 2), N(N(5, 6), N(7, 9))))

	expectSet(t4, 8, "(((1 2) (5 6)) ((7 8) 9))", 5)
	expectSet(t4, 10, "(((1 2) (5 6)) (7 (9 10)))", 5)

	//////// Test Remove cases:

	t10 := T(N(N(1, 2), 3))

	expectRemove(t10, 2, "(1 3)", 1)
	expectRemove(t10, 3, "(1 2)", 0)

	t11 := T(N(N(N(1, 2), 3), N(4, 5)))

	expectRemove(t11, 4, "((1 2) (3 5))", 2)
	expectRemove(t11, 3, "((1 2) (4 5))", 1)

}

func randBytes(length int) []byte {
	key := make([]byte, length)
	// math.rand.Read always returns err=nil
	mrand.Read(key)
	return key
}

func TestRemove(t *testing.T) {
	size := 10000
	keyLen, dataLen := 16, 40

	d := db.NewDB("test", "memdb", "")
	defer d.Close()

	t1 := NewIAVLTree(size, d)

	// insert a bunch of random nodes
	keys := make([][]byte, size)
	l := int32(len(keys))
	for i := 0; i < size; i++ {
		key := randBytes(keyLen)
		t1.Set(key, randBytes(dataLen))
		keys[i] = key
	}

	for i := 0; i < 10; i++ {
		step := 50 * i
		// remove a bunch of existing keys (may have been deleted twice)
		for j := 0; j < step; j++ {
			key := keys[mrand.Int31n(l)]
			t1.Remove(key)
		}
		// FIXME: this Save() causes a panic!
		t1.Save()
	}

}

func TestIntegration(t *testing.T) {

	type record struct {
		key   string
		value string
	}

	records := make([]*record, 400)
	var tree *IAVLTree = NewIAVLTree(0, nil)

	randomRecord := func() *record {
		return &record{randstr(20), randstr(20)}
	}

	for i := range records {
		r := randomRecord()
		records[i] = r
		//t.Log("New record", r)
		//PrintIAVLNode(tree.root)
		updated := tree.Set([]byte(r.key), nil)
		if updated {
			t.Error("should have not been updated")
		}
		updated = tree.Set([]byte(r.key), []byte(r.value))
		if !updated {
			t.Error("should have been updated")
		}
		if tree.Size() != i+1 {
			t.Error("size was wrong", tree.Size(), i+1)
		}
	}

	for _, r := range records {
		if has := tree.Has([]byte(r.key)); !has {
			t.Error("Missing key", r.key)
		}
		if has := tree.Has([]byte(randstr(12))); has {
			t.Error("Table has extra key")
		}
		if _, val, _ := tree.Get([]byte(r.key)); string(val) != string(r.value) {
			t.Error("wrong value")
		}
	}

	for i, x := range records {
		if val, removed := tree.Remove([]byte(x.key)); !removed {
			t.Error("Wasn't removed")
		} else if string(val) != string(x.value) {
			t.Error("Wrong value")
		}
		for _, r := range records[i+1:] {
			if has := tree.Has([]byte(r.key)); !has {
				t.Error("Missing key", r.key)
			}
			if has := tree.Has([]byte(randstr(12))); has {
				t.Error("Table has extra key")
			}
			_, val, _ := tree.Get([]byte(r.key))
			if string(val) != string(r.value) {
				t.Error("wrong value")
			}
		}
		if tree.Size() != len(records)-(i+1) {
			t.Error("size was wrong", tree.Size(), (len(records) - (i + 1)))
		}
	}
}

func TestIterateRange(t *testing.T) {
	type record struct {
		key   string
		value string
	}

	records := []record{
		{"abc", "123"},
		{"low", "high"},
		{"fan", "456"},
		{"foo", "a"},
		{"foobaz", "c"},
		{"good", "bye"},
		{"foobang", "d"},
		{"foobar", "b"},
		{"food", "e"},
		{"foml", "f"},
	}
	keys := make([]string, len(records))
	for i, r := range records {
		keys[i] = r.key
	}
	sort.Strings(keys)

	var tree *IAVLTree = NewIAVLTree(0, nil)

	// insert all the data
	for _, r := range records {
		updated := tree.Set([]byte(r.key), []byte(r.value))
		if updated {
			t.Error("should have not been updated")
		}
	}

	// test traversing the whole node works... in order
	viewed := []string{}
	tree.Iterate(func(key []byte, value []byte) bool {
		viewed = append(viewed, string(key))
		return false
	})
	if len(viewed) != len(keys) {
		t.Error("not the same number of keys as expected")
	}
	for i, v := range viewed {
		if v != keys[i] {
			t.Error("Keys out of order", v, keys[i])
		}
	}

	trav := traverser{}
	tree.IterateRange([]byte("foo"), []byte("goo"), true, trav.view)
	expectTraverse(t, trav, "foo", "food", 5)

	trav = traverser{}
	tree.IterateRange(nil, []byte("flap"), true, trav.view)
	expectTraverse(t, trav, "abc", "fan", 2)

	trav = traverser{}
	tree.IterateRange([]byte("foob"), nil, true, trav.view)
	expectTraverse(t, trav, "foobang", "low", 6)

	trav = traverser{}
	tree.IterateRange([]byte("very"), nil, true, trav.view)
	expectTraverse(t, trav, "", "", 0)

	// make sure backwards also works...
	trav = traverser{}
	tree.IterateRange([]byte("fooba"), []byte("food"), false, trav.view)
	expectTraverse(t, trav, "food", "foobang", 4)

	// make sure backwards also works...
	trav = traverser{}
	tree.IterateRange([]byte("g"), nil, false, trav.view)
	expectTraverse(t, trav, "low", "good", 2)

}

type traverser struct {
	first string
	last  string
	count int
}

func (t *traverser) view(key, value []byte) bool {
	if t.first == "" {
		t.first = string(key)
	}
	t.last = string(key)
	t.count += 1
	return false
}

func expectTraverse(t *testing.T, trav traverser, start, end string, count int) {
	if trav.first != start {
		t.Error("Bad start", start, trav.first)
	}
	if trav.last != end {
		t.Error("Bad end", end, trav.last)
	}
	if trav.count != count {
		t.Error("Bad count", count, trav.count)
	}
}

func TestPersistence(t *testing.T) {
	db := db.NewMemDB()

	// Create some random key value pairs
	records := make(map[string]string)
	for i := 0; i < 10000; i++ {
		records[randstr(20)] = randstr(20)
	}

	// Construct some tree and save it
	t1 := NewIAVLTree(0, db)
	for key, value := range records {
		t1.Set([]byte(key), []byte(value))
	}
	t1.Save()

	hash, _ := t1.HashWithCount()

	// Load a tree
	t2 := NewIAVLTree(0, db)
	t2.Load(hash)
	for key, value := range records {
		_, t2value, _ := t2.Get([]byte(key))
		if string(t2value) != value {
			t.Fatalf("Invalid value. Expected %v, got %v", value, t2value)
		}
	}
}

func testProof(t *testing.T, proof *IAVLProof, keyBytes, valueBytes, rootHashBytes []byte, version int) {
	// Proof must verify.
	require.True(t, proof.Verify(keyBytes, valueBytes, rootHashBytes, version))

	// Write/Read then verify.
	proofBytes := wire.BinaryBytes(proof)
	proof2, err := ReadProof(proofBytes)
	require.Nil(t, err, "Failed to read IAVLProof from bytes: %v", err)
	require.True(t, proof2.Verify(keyBytes, valueBytes, proof.RootHash, version))

	// Random mutations must not verify
	for i := 0; i < 10; i++ {
		badProofBytes := test.MutateByteSlice(proofBytes)
		badProof, err := ReadProof(badProofBytes)
		// may be invalid... errors are okay
		if err == nil {
			assert.False(t, badProof.Verify(keyBytes, valueBytes, rootHashBytes, version),
				"Proof was still valid after a random mutation:\n%X\n%X",
				proofBytes, badProofBytes)
		}
	}

	// targetted changes fails...
	proof.RootHash = test.MutateByteSlice(proof.RootHash)
	assert.False(t, proof.Verify(keyBytes, valueBytes, rootHashBytes, version))
	proof2.LeafHash = test.MutateByteSlice(proof2.LeafHash)
	assert.False(t, proof2.Verify(keyBytes, valueBytes, rootHashBytes, version))
}

func TestIAVLProof(t *testing.T) {
	// Construct some random tree
	db := db.NewMemDB()
	var tree *IAVLTree = NewIAVLTree(100, db)
	for i := 0; i < 1000; i++ {
		key, value := randstr(20), randstr(20)
		tree.Set([]byte(key), []byte(value))
	}

	// Persist the items so far
	tree.Save()

	// Add more items so it's not all persisted
	for i := 0; i < 100; i++ {
		key, value := randstr(20), randstr(20)
		tree.Set([]byte(key), []byte(value))
	}

	// Now for each item, construct a proof and verify
	tree.Iterate(func(key []byte, value []byte) bool {
		value2, version, proof := tree.ConstructProof(key, tree.version)
		assert.Equal(t, value, value2)
		if assert.NotNil(t, proof) {
			testProof(t, proof, key, value, tree.Hash(), version)
		}
		return false
	})
}

func TestIAVLTreeProof(t *testing.T) {
	db := db.NewMemDB()
	var tree *IAVLTree = NewIAVLTree(100, db)

	// should get false for proof with nil root
	_, _, exists := tree.Proof([]byte("foo"))
	assert.False(t, exists)

	// insert lots of info and store the bytes
	keys := make([][]byte, 200)
	for i := 0; i < 200; i++ {
		key, value := randstr(20), randstr(200)
		tree.Set([]byte(key), []byte(value))
		keys[i] = []byte(key)
	}

	// query random key fails
	_, _, exists = tree.Proof([]byte("foo"))
	assert.False(t, exists)

	// valid proof for real keys
	hash := tree.Hash()
	for _, key := range keys {
		value, proofBytes, exists := tree.Proof(key)
		if assert.True(t, exists) {
			proof, err := ReadProof(proofBytes)
			require.Nil(t, err, "Failed to read IAVLProof from bytes: %v", err)
			assert.True(t, proof.Verify(key, value, hash, tree.version))
		}
	}
}

func BenchmarkImmutableAvlTreeCLevelDB(b *testing.B) {
	b.StopTimer()

	db := db.NewDB("test", db.CLevelDBBackendStr, "./")
	t := NewIAVLTree(100000, db)
	// for i := 0; i < 10000000; i++ {
	for i := 0; i < 1000000; i++ {
		// for i := 0; i < 1000; i++ {
		t.Set(i2b(int(cmn.RandInt32())), nil)
		if i > 990000 && i%1000 == 999 {
			t.Save()
		}
	}
	t.Save()

	fmt.Println("ok, starting")

	runtime.GC()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ri := i2b(int(cmn.RandInt32()))
		t.Set(ri, nil)
		t.Remove(ri)
		if i%100 == 99 {
			t.Save()
		}
	}

	db.Close()
}

func BenchmarkImmutableAvlTreeMemDB(b *testing.B) {
	b.StopTimer()

	db := db.NewDB("test", db.MemDBBackendStr, "")
	t := NewIAVLTree(100000, db)
	// for i := 0; i < 10000000; i++ {
	for i := 0; i < 1000000; i++ {
		// for i := 0; i < 1000; i++ {
		t.Set(i2b(int(cmn.RandInt32())), nil)
		if i > 990000 && i%1000 == 999 {
			t.Save()
		}
	}
	t.Save()

	fmt.Println("ok, starting")

	runtime.GC()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ri := i2b(int(cmn.RandInt32()))
		t.Set(ri, nil)
		t.Remove(ri)
		if i%100 == 99 {
			t.Save()
		}
	}

	db.Close()
}
