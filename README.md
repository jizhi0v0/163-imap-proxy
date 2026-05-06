# 163-imap-proxy

让 **Spark Mail** 能用 163 邮箱。

## 它解决什么问题

163 邮箱有个非标准要求：客户端 `LOGIN` 成功后必须立刻发送 `ID` 命令自报身份（RFC 2971），否则 163 返回 `Unsafe Login` 并断开。Spark 没有为 163 实现这个特殊握手，所以加 163 账号必失败。

本工具在你的服务器上跑一个 IMAP 代理：监听 `:993`，上游连 `imap.163.com`，检测到 `LOGIN OK` 后自动替 Spark 注入 `ID` 命令（伪装成 Foxmail），其余字节透传。

## 准备

- 一台 Linux 服务器（**本指南假设 Ubuntu 22.04+ 或 Debian 12+**），有 root 权限
- 一个你拥有的域名，下文以 `mail.example.com` 为例
- 服务器需要能从公网访问 80 和 993 端口（80 仅证书申请/续期时短暂占用）

> 如果你用宝塔面板 / nginx / Apache 等已经占着 80 端口的服务，**先临时停掉**它们，否则证书申请会失败。`sudo lsof -i:80` 应该没有任何输出。

## 部署

### 1. 把域名指向服务器

到你的 DNS 服务商那里，给 `mail.example.com` 加一条 **A 记录**指向服务器公网 IP。

**如果用 Cloudflare 托管**：在 DNS 页面，记录右侧的橙色云朵图标**点一下让它变成灰色**（DNS only），然后保存。否则 Cloudflare 会拦截 993 端口让连接永远失败。

### 2. 放行防火墙

服务器系统自带 ufw 的话：

```bash
sudo ufw allow 80/tcp
sudo ufw allow 993/tcp
```

**云厂商（阿里云 / 腾讯云 / AWS / DMIT 等）还要在网页控制台的"安全组"里**放行 80 和 993，不然系统层放了也没用。

### 3. 一键安装 wrapper

```bash
curl -fsSL https://raw.githubusercontent.com/jizhi0v0/163-imap-proxy/main/install.sh -o install.sh
sudo sh install.sh
```

脚本会问你选 systemd 还是 Docker——**不确定就选 1（systemd）**。安装完成后服务会自动启动。

### 4. 申请 Let's Encrypt 证书

把下面整段**作为一个整体**复制到终端（先把 `mail.example.com` 改成你自己的域名）：

```bash
DOMAIN=mail.example.com

sudo apt update && sudo apt install -y certbot
sudo certbot certonly --standalone -d $DOMAIN --agree-tos --register-unsafely-without-email --non-interactive
sudo cp /etc/letsencrypt/live/$DOMAIN/fullchain.pem /etc/163-wrapper/cert.pem
sudo cp /etc/letsencrypt/live/$DOMAIN/privkey.pem  /etc/163-wrapper/key.pem
sudo chmod 600 /etc/163-wrapper/key.pem
sudo systemctl restart 163-wrapper

sudo tee /etc/letsencrypt/renewal-hooks/deploy/163-wrapper.sh > /dev/null <<EOF
#!/bin/sh
cp /etc/letsencrypt/live/$DOMAIN/fullchain.pem /etc/163-wrapper/cert.pem
cp /etc/letsencrypt/live/$DOMAIN/privkey.pem  /etc/163-wrapper/key.pem
chmod 600 /etc/163-wrapper/key.pem
systemctl restart 163-wrapper
EOF
sudo chmod +x /etc/letsencrypt/renewal-hooks/deploy/163-wrapper.sh
```

> Docker 模式用户：把 `systemctl restart 163-wrapper` 两处都换成 `docker restart 163-wrapper`。

### 5. 验证

```bash
echo | openssl s_client -connect mail.example.com:993 -servername mail.example.com 2>&1 | grep "Verify return code"
```

看到 `Verify return code: 0 (ok)` 就成功了。

## 在 Spark 添加 163 账号

163 网页端先做一次：登录 → 设置 → POP3/SMTP/IMAP → **同时打开 IMAP 和 SMTP 服务** → **生成"客户端授权码"**（下面填的"密码"全部用这个授权码，不是登录密码）。

### Spark Mac

1. 顶部菜单：`Spark → Add Account...`
2. 选 **"Set Up Account Manually"**（不要用 163 预设）
3. 选 **IMAP**
4. 按下表填写，点 Sign In：

| 字段 | 值 |
|------|----|
| Email Address | `you@163.com` |
| Password | 163 授权码 |
| User Name | `you@163.com`（完整邮箱） |
| **IMAP Server** | `mail.example.com` |
| IMAP Port | `993` |
| Use SSL | ✅ 开启 |
| **SMTP Server** | `smtp.163.com` |
| SMTP Port | `587` |
| Use SSL | ✅ 开启（STARTTLS） |
| SMTP User Name | `you@163.com` |
| SMTP Password | 163 授权码 |

### Spark iOS

1. `Settings → Mail Accounts → Add Mail Account → Other`
2. 同样选 IMAP，按上表填写

## 出问题怎么办

**Spark 卡在加载或一直转圈**：在你电脑/手机上执行 `nc -vz mail.example.com 993`（Mac 终端 / iOS 用任意带 nc 的 App），如果连不通说明端口没真的对外开放——回到第 2 步检查防火墙和云厂商安全组。

**`Verify return code` 不是 0**：证书没装好。在服务器上 `sudo ls /etc/163-wrapper/` 看 `cert.pem` 和 `key.pem` 是否都存在；不在就重跑第 4 步。

**看日志**（systemd 模式）：

```bash
sudo journalctl -u 163-wrapper -f
```

按 Ctrl-C 退出。看到 `client connected` + `ID injected successfully` 就是 wrapper 工作正常。

**重置授权码**：163 网页端可以随时重置；如果担心日志泄漏，重置后到客户端把密码改成新的授权码即可。
