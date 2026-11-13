#!/bin/bash
# build-disk.sh — собирает disk.img для VirtualBox из /srv/rootfs
set -e
SIZE=2048
DISK=/srv/disk.img
ROOTFS=/srv/rootfs
CFG=/mnt/cfg

echo "=== 1. Создание пустого образа $SIZE MB ==="
rm -f $DISK
truncate -s ${SIZE}M $DISK

echo "=== 2. Разметка (MBR, один раздел, boot-флаг) ==="
sfdisk $DISK <<EOF
label: dos
unit: sectors

start=2048, type=83, bootable
EOF

echo "=== 3. Loop-монтирование ==="
losetup -d /dev/loop0 2>/dev/null || true
losetup -P /dev/loop0 $DISK
mkfs.ext4 -F -L examroot /dev/loop0p1
mkdir -p /srv/mnt
mount /dev/loop0p1 /srv/mnt

echo "=== 4. Копирование rootfs ==="
cp -a $ROOTFS/. /srv/mnt/
mkdir -p $CFG
