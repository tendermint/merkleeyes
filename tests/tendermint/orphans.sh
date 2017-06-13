#!/bin/bash
#key=01036b6579837A272A2B
key=01036b6579
key=01045061756C

function get() {
    echo "GET"
    r=$(go run rand.go)
    curl localhost:46657/broadcast_tx_sync?tx=0x${r}03${key}
}

function set() {
    echo "SET $1"
    r=$(go run rand.go)
    v=$1
    curl localhost:46657/broadcast_tx_sync?tx=0x${r}01${key}0101$v
}

function cas() {
    echo "CAS $1 $2"
    r=$(go run rand.go)
    v1=$1
    v2=$2
    echo 0x${r}04${key}0101${v1}0101${v2} 
    curl localhost:46657/broadcast_tx_sync?tx=0x${r}04${key}0101${v1}0101${v2}
    #curl localhost:46657/broadcast_tx_sync?tx=0x${r}0401220101${v1}0101${v2}
}

set 00
get
set 00
set 04
get
cas 01 00
cas 00 01
set 02
get 
cas 00 00
get
set 04
get
set 04
get
cas 02 03
cas 00 02
set 01
get
set 01
cas 04 00
