#!/bin/bash

cd ~/cs425/mp2/nodes
pkill -f introducer.go
go run introducer.go
echo Introducer is on.