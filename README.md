# Exam Board — табло демоэкзамена (ДЭ 10.02.05)

Виртуалка (Debian 12) с веб-табло для демонстрационного экзамена:
кто подключился по SSH, во сколько, и кнопка **«ПОБЕДИТЕЛЬ»**.

## Файлы
- `board.py` — бэкенд (python3 stdlib, порт 8080)
- `index.html`, `style.css`, `app.js` — фронтенд табло
- `cfg/` — конфиги для VM (systemd-юнит, сеть, motd, скрипт win, extlinux)
- `build/` — скрипты сборки disk.img в WSL и тестов
- `ExamBoard.ova` — готовая VM для VirtualBox (не хранится в git, >100 МБ)

## Доступ к VM
- Логин: `student` / пароль: `exam2026`
- Табло: `http://<IP виртуалки>:8080`
- Объявить победителя: команда на сервере `win Имя Фамилия`
  (или кнопка «🏆 Объявить победителя» на табло)

## Что показывает табло
- Время сервера (тикает)
- Таблица подключений: студент, сессия, откуда (IP), время входа — из wtmp (`last -F`)
- Счётчики: подключений всего / уникальных студентов
- Блок «ПОБЕДИТЕЛЬ» с конфетти и именем

## API
- `GET /api/state` — JSON: время, подключения, победитель
- `POST /api/winner` (form: `name=`) — объявить победителя
- `POST /api/reset` — сбросить победителя

## Сборка OVA (кратко)
1. WSL Debian: debootstrap bookworm в `/srv/rootfs` (+ linux-image, openssh-server, python3, systemd)
2. `build/build-disk.sh` — disk.img 2GB, ext4, копирование rootfs
3. chroot: systemd enable, студент, extlinux, grub/MBR
4. `VBoxManage convertfromraw` → vmdk → `VBoxManage export` → `.ova`
