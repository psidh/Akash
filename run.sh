#!/bin/bash

cd src || exit

go build -o Akash main.go

./Akash -config=config/userConfig.json
