# -*- coding: utf-8 -*-
# ssh-test.py — логин student в VM, вызов win, проверка табло
import paramiko, urllib.request

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect("127.0.0.1", 2222, "student", "exam2026", timeout=10)
_, out, _ = c.exec_command("hostname; who; win Тестовый Студент")
print(out.read().decode("utf-8", "replace"))
c.close()

st = urllib.request.urlopen("http://127.0.0.1:8080/api/state", timeout=8).read()
print("STATE:", st.decode("utf-8"))
