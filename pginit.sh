#!/usr/bin/env bash

source ./pg.env

$PGCTL init

mkdir -p "$PGDATA/sockets"
echo "unix_socket_directories = 'sockets'" >> "$PGDATA/postgresql.conf"
echo "listen_addresses = ''" >> "$PGDATA/postgresql.conf"
