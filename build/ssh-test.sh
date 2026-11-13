#!/bin/bash
# ssh-test.sh — логин student в VM и проверка табло
WINIP=$(ip route | awk '/default/ {print $3}')
echo "WINIP=$WINIP"
sshpass -p exam2026 ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p 2299 "student@$WINIP" 'echo SSH_OK; hostname; win Тестовый Студент' 2>&1 | tail -5
