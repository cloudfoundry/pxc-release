#!/bin/bash

export MYSQL_PWD=$3

$1 -u$2 -e"show databases;"
