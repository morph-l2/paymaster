#!/bin/bash

GENESIS_PATH=""

if [ -z "$GENESIS_PATH" ]; then
    echo "Error: GENESIS_PATH is not set"
    exit 1
fi

if [ ! -d "build/db" ] || [ -z "$(ls -A build/db)" ]; then
    echo "Directory build/db is empty or does not exist"
    ./build/bin/geth --verbosity=3 init --datadir=./build/db $GENESIS_PATH
fi

./build/bin/geth \
  --datadir ./build/db \
  --verbosity 3 \
  --mev.enable \
  --mev.bundle.gasprice.floor=100000 \
  --mev.bundle.receiver.url=http://localhost:8545 \
  --paymaster.privatekey=3e4bde571b86929bf08e2aaad9a6a1882664cd5e65b96fff7d03e1c4e6dfa15c \
  --http \
  --http.corsdomain "*" \
  --http.vhosts "*" \
  --http.addr=0.0.0.0 \
  --http.port=8045 \
  --http.api=web3,debug,eth,txpool,net,morph,engine,admin,pm \
  --networkid=53077 \
  --authrpc.addr=0.0.0.0 \
  --authrpc.port=8051 \
  --authrpc.vhosts="*" \
  --authrpc.jwtsecret=/Users/fletcher/workspace/morph-l2/paymaster/build/jwt-secret.txt \
  --gcmode=archive \
  --port=30303 \
  --metrics \
  --metrics.addr=127.0.0.1