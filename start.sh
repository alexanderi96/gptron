#!/bin/bash

mkdir -p ~/gptron
nohup ~/go/bin/gptron  >> ~/gptron/gptron.log 2>&1 &
