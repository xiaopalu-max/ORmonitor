#!/bin/bash

# 构建镜像
docker build --platform linux/amd64 -t ormonitor:latest . 

# 下载镜像
docker save > ormonitor.tar ormonitor:latest