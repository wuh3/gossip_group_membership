#!/bin/bash

cd ~/cs425/mp2/nodes
pkill -f node.go
go run node.go noinput
echo Node is on.