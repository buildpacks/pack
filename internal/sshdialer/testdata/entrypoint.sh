#!/usr/bin/env bash

/usr/sbin/sshd

su - testuser -c '/usr/local/bin/serve-socket "/home/testuser/test.sock" "1234"'
