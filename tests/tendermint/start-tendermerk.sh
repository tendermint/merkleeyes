#!/bin/bash

rm -rf orphan-test-db
merkleeyes start -d orphan-test-db --address=tcp://127.0.0.1:46658 >> merkleeyes.log &

rm -rf ~/.tendermint
tendermint init
tendermint node >> tendermint.log &

sleep 4
