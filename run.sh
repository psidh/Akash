#!/bin/bash

go build -o Akash main.go

./Akash -config=example/userConfig.json
