#!/bin/bash
# switch-extlinux.sh — заменяет GRUB на extlinux в disk.img
set -e
M=/srv/mnt

losetup -d /dev/loop0 2>/dev/null || true
losetup -P /dev/loop0 /srv/disk.img
mount /dev/loop0p1 $M
UUID=$(blkid -s UUID -o value /dev/loop0p1)

rm -rf $M/boot/grub
mkdir -p $M/boot/extlinux
cat > $M/boot/extlinux/extlinux.conf <<EOF
DEFAULT linux
PROMPT 0
TIMEOUT 10

LABEL linux
  KERNEL /boot/vmlinuz
  INITRD /boot/initrd.img
  APPEND root=UUID=$UUID ro quiet console=tty0 console=ttyS0
EOF

extlinux --install $M/boot/extlinux
dd if=/usr/lib/EXTLINUX/mbr.bin of=/dev/loop0 bs=440 count=1 conv=notrunc
sync

umount $M
losetup -d /dev/loop0
echo "EXTLINUX_OK UUID=$UUID"
