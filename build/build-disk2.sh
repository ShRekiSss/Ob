#!/bin/bash
# build-disk2.sh — настройка внутри chroot и grub. Запускать ПОСЛЕ build-disk.sh
set -e
M=/srv/mnt
OVA=/mnt/d/NEUDALIT/opencode/ova
CFG=$OVA/cfg

echo "=== 5. Config-файлы ==="
mkdir -p $M/opt/board $M/etc/systemd/system $M/etc/systemd/network
cp $OVA/board.py $OVA/index.html $OVA/style.css $OVA/app.js $M/opt/board/
cp $CFG/board.service $M/etc/systemd/system/
cp $CFG/10-eth.network $M/etc/systemd/network/
cp $CFG/motd $M/etc/motd
cp $CFG/win $M/usr/local/bin/win && chmod +x $M/usr/local/bin/win
mkdir -p $M/var/lib/board

echo "=== 6. fstab и хосты ==="
UUID=$(blkid -s UUID -o value /dev/loop0p1)
echo "UUID=$UUID / ext4 defaults 0 1" > $M/etc/fstab
echo "examboard" > $M/etc/hostname
cat > $M/etc/hosts <<EOF
127.0.0.1 localhost
127.0.1.1 examboard
EOF

echo "=== 7. Bind-маунты и chroot ==="
mount --bind /dev $M/dev
mount --bind /proc $M/proc
mount --bind /sys $M/sys
chroot $M /bin/bash -e <<EOF
export DEBIAN_FRONTEND=noninteractive
locale-gen ru_RU.UTF-8 en_US.UTF-8 2>/dev/null || true
systemctl enable systemd-networkd systemd-resolved ssh board
ln -sf /usr/share/zoneinfo/Europe/Moscow /etc/localtime
echo 'Europe/Moscow' > /etc/timezone
sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin no/' /etc/ssh/sshd_config
EOF

echo "=== 8. Пользователи ==="
chroot $M /bin/bash -e <<EOF
useradd -m -s /bin/bash -G sudo student
echo 'student:exam2026' | chpasswd
passwd -l root
EOF
echo "student ALL=(ALL) NOPASSWD:ALL" > $M/etc/sudoers.d/student

echo "=== 9. grub-install ==="
mkdir -p $M/boot/grub
cp $CFG/grub.cfg $M/boot/grub/grub.cfg
sed -i "s/%UUID%/$UUID/" $M/boot/grub/grub.cfg
grub-install --target=i386-pc --boot-directory=$M/boot --no-floppy /dev/loop0

echo "=== 10. Отключение ==="
umount $M/dev $M/proc $M/sys
umount $M
losetup -d /dev/loop0
echo "=== DONE: $DISK готов ==="
