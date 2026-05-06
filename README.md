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

## 可选：用 Tailscale 替代域名 + 公网

不想买域名、不想暴露公网 993 端口的话，可以走 Tailscale。每台用邮件的设备（手机、电脑）也装 Tailscale 登同一账号即可——这样你跳过上面的 1、2、4 步，但仍然能拿到真正的 Let's Encrypt 证书（Tailscale 通过 DNS-01 帮你签）。

### 1. 服务器和客户端都装 Tailscale

服务器：

```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
```

按提示登录账号。Mac / iOS / Windows 直接装 Tailscale App 登同一账号。

### 2. 在 Tailscale 后台启用 HTTPS

[https://login.tailscale.com/admin/dns](https://login.tailscale.com/admin/dns) → 找到 "HTTPS Certificates" → 点 Enable HTTPS。同一页面顶部能看到你的 tailnet 名，形如 `tail-xxxxxx.ts.net`。

### 3. 用 Tailscale 给服务器签证书

服务器主机名假设是 `mailproxy`（`tailscale status` 第一行能看到），在服务器上执行：

```bash
sudo tailscale cert mailproxy.tail-xxxxxx.ts.net
sudo cp mailproxy.tail-xxxxxx.ts.net.crt /etc/163-wrapper/cert.pem
sudo cp mailproxy.tail-xxxxxx.ts.net.key /etc/163-wrapper/key.pem
sudo chmod 600 /etc/163-wrapper/key.pem
sudo systemctl restart 163-wrapper
```

证书 90 天有效期，加个 cron 自动续（**注意文件名不能带后缀**，Debian/Ubuntu 的 `run-parts` 会跳过含 `.` 的文件名）：

```bash
sudo tee /etc/cron.monthly/163-wrapper-cert > /dev/null <<'EOF'
#!/bin/sh
set -e
cd /etc/163-wrapper
tailscale cert mailproxy.tail-xxxxxx.ts.net
mv mailproxy.tail-xxxxxx.ts.net.crt cert.pem
mv mailproxy.tail-xxxxxx.ts.net.key key.pem
chmod 600 key.pem
systemctl restart 163-wrapper
EOF
sudo chmod +x /etc/cron.monthly/163-wrapper-cert
```

### 4. Spark 配置

跟下文一样，把 IMAP Server 填成 `mailproxy.tail-xxxxxx.ts.net`，其他不变。设备只要连着 Tailscale，邮件就能正常收发。

### 权衡：失去 Spark 推送通知

Spark 客户端在前台/活跃时是**本地**连 IMAP，Tailscale 完全够用；但 App 完全退出或设备长时间离线时，新邮件提醒由 **Readdle 云端**替你 IDLE 监听 IMAP 后通过 APNs 推送到设备——他们的服务器不在你 tailnet 里，所以这条路**收不到推送**，必须打开 App 手动刷新才能看到新邮件。

这个限制对**任何非公网部署**都成立（局域网 / 家庭服务器 / Tailscale 都一样）。**只有"公网 VPS + 域名 + LE 证书"那条路能让 Readdle 云端 IDLE 你的 wrapper、保留实时推送**——代价是 163 授权码会被传到他们机房用于维持长连接。

按你对 push 的依赖度自己选：
- 重度依赖即时通知 → 公网 VPS 路径
- 不在乎几分钟延迟、追求凭据不出私网 → Tailscale / 本地路径

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

**重置授权码**：163 网页端可以随时重置，新旧授权码互不影响；客户端那边对应改成新的即可。

## 日志与隐私

- 默认 `log_level: info`，只记录连接事件（远端 IP、`ID injected successfully` 等），不含任何凭据。
- `log_level: debug` 时会打印每条 IMAP 帧用于排错，但 `LOGIN` / `AUTHENTICATE` 的密码字段会自动屏蔽成 `<REDACTED>`。
- systemd 下日志由 journald 自动按磁盘空间轮转，默认上限 `min(磁盘 10%, 4GB)`。想严格限制：编辑 `/etc/systemd/journald.conf` 加 `SystemMaxUse=200M` 后 `sudo systemctl restart systemd-journald`。
- Docker 默认 json-file 日志驱动**不轮转**，长跑会涨；建议在 `docker-compose.yml` 里给服务加：

  ```yaml
      logging:
        driver: json-file
        options: { max-size: "10m", max-file: "3" }
  ```
