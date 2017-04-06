1. 先执行```openssl genrsa -out server.key 4096```
2. 再执行```openssl req -new -x509 -key server.key -out server.pem -days 3650```
3. 在```Common Name (e.g. server FQDN or YOUR name)```项中填入```www.acevest.com```
4. 双击```server.pem```文件，导入keychain，再双击该证书信任所有
5. ```sudo vim /etc/hosts``` 将```127.0.0.1 www.acevest.com```加入配置文件
