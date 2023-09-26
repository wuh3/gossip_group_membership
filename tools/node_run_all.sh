#!/bin/bash
ssh haozhew3@fa23-cs425-7410.cs.illinois.edu "cd ~/cs425/mp2/tools/; ./introducer_run_local.sh"
echo Introducer:VM10 started...
for i in {1..9}
do
    ssh haozhew3@fa23-cs425-740$i.cs.illinois.edu "cd ~/cs425/mp2/tools/; ./node_run_local.sh"
    echo Node:VM$i started...
done