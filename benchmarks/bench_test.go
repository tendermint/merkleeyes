package benchmarks

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"testing"

	db "github.com/tendermint/go-db"
	merkle "github.com/tendermint/go-merkle"
)

func randBytes(length int) []byte {
	key := make([]byte, length)
	// math.rand.Read always returns err=nil
	rand.Read(key)
	return key
}

func prepareTree(db db.DB, size, keyLen, dataLen int) (merkle.Tree, [][]byte) {
	t := merkle.NewIAVLTree(size, db)
	keys := make([][]byte, size)

	for i := 0; i < size; i++ {
		key := randBytes(keyLen)
		t.Set(key, randBytes(dataLen))
		keys[i] = key
	}
	t.Hash()
	t.Save()
	runtime.GC()
	return t, keys
}

func runQueries(b *testing.B, t merkle.Tree, keyLen int) {
	for i := 0; i < b.N; i++ {
		q := randBytes(keyLen)
		t.Get(q)
	}
}

func runKnownQueries(b *testing.B, t merkle.Tree, keys [][]byte) {
	l := int32(len(keys))
	for i := 0; i < b.N; i++ {
		q := keys[rand.Int31n(l)]
		t.Get(q)
	}
}

func runInsert(b *testing.B, t merkle.Tree, keyLen, dataLen, blockSize int) merkle.Tree {
	for i := 1; i <= b.N; i++ {
		t.Set(randBytes(keyLen), randBytes(dataLen))
		if i%blockSize == 0 {
			t.Hash()
			t.Save()
		}
	}
	return t
}

func runUpdate(b *testing.B, t merkle.Tree, dataLen, blockSize int, keys [][]byte) merkle.Tree {
	l := int32(len(keys))
	for i := 1; i <= b.N; i++ {
		key := keys[rand.Int31n(l)]
		t.Set(key, randBytes(dataLen))
		if i%blockSize == 0 {
			t.Hash()
			t.Save()
		}
	}
	return t
}

func runDelete(b *testing.B, t merkle.Tree, blockSize int, keys [][]byte) merkle.Tree {
	var key []byte
	l := int32(len(keys))
	for i := 1; i <= b.N; i++ {
		key = keys[rand.Int31n(l)]
		// key = randBytes(16)
		// TODO: test if removed, use more keys (from insert)
		t.Remove(key)
		if i%blockSize == 0 {
			t.Hash()
			t.Save()
		}
	}
	return t
}

func runTMSP(b *testing.B, t merkle.Tree, keyLen, dataLen, blockSize int, keys [][]byte) merkle.Tree {
	l := int32(len(keys))

	lastCommit := t
	real := t.Copy()
	check := t.Copy()

	for i := 1; i <= b.N; i++ {
		// 50% insert, 50% update
		var key []byte
		if i%2 == 0 {
			key = keys[rand.Int31n(l)]
		} else {
			key = randBytes(keyLen)
		}
		data := randBytes(dataLen)

		// perform query and write on check and then real
		check.Get(key)
		check.Set(key, data)
		real.Get(key)
		real.Set(key, data)

		// at the end of a block, move it all along....
		if i%blockSize == 0 {
			real.Hash()
			real.Save()
			lastCommit = real
			real = lastCommit.Copy()
			check = lastCommit.Copy()
		}
	}

	real.Hash()
	real.Save()
	return real
}

func xxxBenchmarkRandomBytes(b *testing.B) {
	benchmarks := []struct {
		length int
	}{
		{4}, {16}, {32},
	}
	for _, bench := range benchmarks {
		name := fmt.Sprintf("random-%d", bench.length)
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				randBytes(bench.length)
			}
			runtime.GC()
		})
	}
}

func BenchmarkAllTrees(b *testing.B) {
	benchmarks := []struct {
		dbType              string
		initSize, blockSize int
		keyLen, dataLen     int
	}{
		{"nodb", 100000, 100, 16, 40},
		{"memdb", 100000, 100, 16, 40},
		{"goleveldb", 100000, 100, 16, 40},
		// FIXME: this crashes on init! Either remove support, or make it work.
		// {"cleveldb", 100000, 100, 16, 40},
		{"leveldb", 100000, 100, 16, 40},
	}

	for _, bb := range benchmarks {
		prefix := fmt.Sprintf("%s-%d-%d-%d-%d", bb.dbType, bb.initSize,
			bb.blockSize, bb.keyLen, bb.dataLen)

		// prepare a dir for the db and cleanup afterwards
		dirName := fmt.Sprintf("./%s-db", prefix)
		defer func() {
			err := os.RemoveAll(dirName)
			if err != nil {
				fmt.Printf("%+v\n", err)
			}
		}()

		// note that "" leads to nil backing db!
		var d db.DB
		if bb.dbType != "nodb" {
			d = db.NewDB("test", bb.dbType, dirName)
			defer d.Close()
		}
		b.Run(prefix, func(sub *testing.B) {
			runSuite(sub, d, bb.initSize, bb.blockSize, bb.keyLen, bb.dataLen)
		})
	}
}

func runSuite(b *testing.B, d db.DB, initSize, blockSize, keyLen, dataLen int) {
	// setup code
	t, keys := prepareTree(d, initSize, keyLen, dataLen)
	b.ResetTimer()

	b.Run("query-miss", func(sub *testing.B) {
		runQueries(sub, t, keyLen)
	})
	b.Run("query-hits", func(sub *testing.B) {
		runKnownQueries(sub, t, keys)
	})
	b.Run("update", func(sub *testing.B) {
		t = runUpdate(sub, t, dataLen, blockSize, keys)
	})
	b.Run("insert", func(sub *testing.B) {
		t = runInsert(sub, t, keyLen, dataLen, blockSize)
	})
	b.Run("delete", func(sub *testing.B) {
		t = runDelete(sub, t, blockSize, keys)
	})
	b.Run("tmsp", func(sub *testing.B) {
		t = runTMSP(sub, t, keyLen, dataLen, blockSize, keys)
	})

}
