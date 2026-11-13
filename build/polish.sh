#!/bin/bash
# polish.sh — help, motd, шрифт кириллицы, board-алиас
set -e
OVA=/mnt/d/NEUDALIT/opencode/ova
M=/srv/mnt

losetup -d /dev/loop0 2>/dev/null || true
losetup -P /dev/loop0 /srv/disk.img
mount /dev/loop0p1 $M

rm -f $M/etc/resolv.conf
cp -L /etc/resolv.conf $M/etc/resolv.conf
mount --bind /dev $M/dev
mount --bind /proc $M/proc
mount --bind /sys $M/sys

chroot $M /bin/bash <<'EOF'
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq 2>&1 | tail -1
apt-get install -y -qq console-setup 2>&1 | tail -1
cat > /etc/default/console-setup <<'CS'
ACTIVE_CONSOLES="/dev/tty[1-6]"
CHARMAP="UTF-8"
CODESET="Cyr2"
FONTFACE="TerminusBold"
FONTSIZE="16x32"
CS
EOF

cp $OVA/cfg/motd $M/etc/motd
cp $OVA/cfg/help $M/usr/local/bin/help
chmod +x $M/usr/local/bin/help

# board — короткая подсказка адреса табло
cat > $M/usr/local/bin/board <<'EOF'
#!/bin/sh
echo "Открой на своём компьютере браузер и введи адрес:"
ip -4 addr show enp0s3 | grep inet | awk '{print "  ->  http://"$2":8080"}'
echo "Логин/пароль для ssh: student / exam2026   (root запрещён)"
EOF
chmod +x $M/usr/local/bin/board

umount $M/dev $M/proc $M/sys
umount $M
losetup -d /dev/loop0
echo POLISH_OK
