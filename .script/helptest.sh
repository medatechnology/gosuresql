#!/bin/bash
EMAIL=$1

# Init golang
# go mod init github.com/medatechnology/goutil

# Generate ssh key if  you don't already have.
# then go to github --> settings --> ssh key and 
# add the [sshfile].pub there (by pasting it)
# ssh-keygen -t ed25519 -C "${EMAIL}"

# Check if 
git remote -v
git remote set-url origin git@github.com:medatechnology/goutil.git
git remote -v

# If needed
# git config --global user.name "yudimeda"
# git config --global user.email "yudi@meda.technology"
