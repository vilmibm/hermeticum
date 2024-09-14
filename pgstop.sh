#!/usr/bin/env bash

source ./pg.env
if [ -z pgdata/postmaster.pid ]; then
  $PGCTL stop
fi
