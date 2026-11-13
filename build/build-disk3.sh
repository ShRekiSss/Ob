#!/bin/bash
# build-disk3.sh — chroot-донастройка после установки systemd
set -e
M=/srv/mnt
OVA=/mnt/d/NEUDALIT/opencode/ova

echo "=== chroot: enable + locale + tz ==="
chroot $M /bin/bash <<'EOF'
export DEBIAN_FRONTEND=noninteractive
locale-gen ru_RU.UTF-8 en_US.UTF-8 >/dev/null 2>&1
systemctl enable systemd-networkd systemd-resolved ssh board getty@tty1 2>&1
ln -sf /usr/share/zoneinfo/Europe/Moscow /etc/localtime
echo 'Europe/Moscow' > /etc/timezone
sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin no/' /etc/ssh/sshd_config
useradd -m -s /bin/bash -G sudo student
echo 'student:exam2026' | chpasswd
echo 'student ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/student
chmod 440 /etc/sudoers.d/student
EOF

echo "=== fstab, hostname, grub ==="
UUID=$(blkid -s UUID -o value /dev/loop0p1)
echo "UUID=$UUID / ext4 defaults 0 1" > $M/etc/fstab
echo "examboard" > $M/etc/hostname
sed -i 's/127.0.1.1.*/127.0.1.1 examboard/' $M/etc/hosts
mkdir -p $M/boot/grub
sed "s/%UUID%/$UUID/" $OVA/cfg/grub.cfg > $M/boot/grub/grub.cfg

echo "=== grub-install ==="
grub-install --target=i386-pc --boot-directory=$M/boot --no-floppy /dev/loop0 2>&1 | tail -2

echo "=== cleanup ==="
umount $M/dev $M/proc $M/sys 2>/dev/null || true
umount $M
losetup -d /dev/loop0 2>/dev/null || true
echo "=== DONE ==="
