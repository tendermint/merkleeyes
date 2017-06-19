#!/bin/bash

./start-tendermerk.sh

for n in {1..5}; do
    ./orphans.sh
done

./kill-tendermerk.sh
