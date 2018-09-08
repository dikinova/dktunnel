#!/bin/bash

mkdir -p gospace
export GOPATH=`pwd`/gospace

go get -u -d github.com/dikinova/dktunnel
go install github.com/dikinova/dktunnel

# finish
echo "dktunnel is in gospace/bin/, go and run"