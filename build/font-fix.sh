#!/bin/bash
# font-fix.sh — включить console-setup (кириллица в tty)
set -e
M=/srv/mnt
losetup -d /dev/loop0 2>/dev/null || true
losetup -P /dev/loop0 /srv/disk.img
mount /dev/loop0p1 $M

cat > $M/etc/default/console-setup <<'CS'
ACTIVE_CONSOLES="/dev/tty[1-6]"
CHARMAP="UTF-8"
CODESET="Cyr2"
FONTFACE="TerminusBold"
FONTSIZE="16"
CS

chroot $M /bin/bash -c 'systemctl enable console-setup.service; ls /usr/share/consolefonts/ | grep -i term | head -3; locale -a | grep ru'
echo FONT_FIX_OK
umount $M
losetup -d /dev/loop0
