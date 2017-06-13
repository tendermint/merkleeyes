package iavl

import (
	"container/list"
	"fmt"
	cmn "github.com/tendermint/tmlibs/common"
)

// Prints the in-memory children recursively.
func PrintIAVLNode(node *IAVLNode) {
	fmt.Println("==== NODE")
	if node != nil {
		printIAVLNode(node, 0)
	}
	fmt.Println("==== END")
}

func printIAVLNode(node *IAVLNode, indent int) {
	indentPrefix := ""
	for i := 0; i < indent; i++ {
		indentPrefix += "    "
	}

	if node.rightNode != nil {
		printIAVLNode(node.rightNode, indent+1)
	} else if node.rightHash != nil {
		fmt.Printf("%s    %X\n", indentPrefix, node.rightHash)
	}

	fmt.Printf("%s%v:%v\n", indentPrefix, node.key, node.height)

	if node.leftNode != nil {
		printIAVLNode(node.leftNode, indent+1)
	} else if node.leftHash != nil {
		fmt.Printf("%s    %X\n", indentPrefix, node.leftHash)
	}

}

func compareBytes(left, right []byte) bool {

	if left == nil && right == nil {
		return true
	}

	if left == nil || right == nil {
		return false
	}

	if len(left) != len(right) {
		return false
	}

	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}

	return true
}

// GetValue is a slow way to get the ith element's value
func GetValue(l *list.List, index int) interface{} {
	for e := l.Front(); e != nil; e = e.Next() {
		if index <= 0 {
			return e.Value
		}
		index--
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

func maxInt8(a, b int8) int8 {
	if a > b {
		return a
	}
	return b
}
func maxInt16(a, b int16) int16 {
	if a > b {
		return a
	}
	return b
}
