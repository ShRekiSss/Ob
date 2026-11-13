# -*- coding: utf-8 -*-
# ssh-test2.py — интерактивная сессия (pty) → запись в wtmp → табло
import time, paramiko, urllib.request

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("127.0.0.1", 2222, "student", "exam2026", timeout=10)
ch = c.invoke_shell()
time.sleep(2)
ch.send("win Тестовый Студент\n")
time.sleep(2)
ch.close()
c.close()

time.sleep(6)  # ждём цикл обновления табло (3 c)
st = urllib.request.urlopen("http://127.0.0.1:8080/api/state", timeout=8).read()
print("STATE:", st.decode("utf-8"))
