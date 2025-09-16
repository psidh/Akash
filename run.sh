#!/bin/bash

go build -o Akash main.go

./Akash -config=config/userConfig.json
