# 163-gmail-server-wrapper

让 Apple Mail / Thunderbird 等标准 IMAP 客户端能在海外或非常用网络下访问 163 邮箱。

## 背景

163/126 邮箱有两个独有限制：

1. **`Unsafe Login`**：客户端 `LOGIN` 成功后必须立刻发送非标准的 `ID` 命令（RFC 2971，自报客户端身份），否则服务器返回 `BAD Unsafe Login` 并断开。Apple Mail / Thunderbird 等通用客户端不发 ID。
2. **海外 IP 风控**：从非中国大陆 IP 直连 `imap.163.com` 经常被限速或拒绝。

本工具在你信任的位置（VPS / 家庭服务器 / 本机）跑一个 IMAP 代理：

- 监听 IMAPS（默认 `0.0.0.0:993`）
- 上游连 `imap.163.com:993`
- 检测到 `LOGIN OK` 后自动注入 `ID` 命令（伪装成 Foxmail），其余流量纯字节透传
- 自动加载或自签 TLS 证书

**只代理 IMAP**。SMTP 直连 `smtp.163.com` 即可，不经过本代理（见下文 SMTP 部分）。

## 推荐部署：VPS + Let's Encrypt

最稳的姿势——在境外 VPS 上跑 wrapper，配合自有域名 + LE 证书，邮件客户端按标准 IMAPS 配置即可。

### 一键安装（systemd 或 Docker 二选一）

```bash
curl -fsSL https://raw.githubusercontent.com/jizhi0v0/163-gmail-server-wrapper/main/install.sh -o install.sh
sudo sh install.sh
```

脚本会让你交互选择 systemd 或 Docker 模式，并写入 `/etc/163-wrapper/config.yaml` 与对应的 service / compose 文件。

### 关闭 Cloudflare Proxy

如果你用 Cloudflare 托管 DNS，**必须把 wrapper 域名的 A 记录设置为 "DNS only"（灰云）**——Cloudflare 的橙云只代理 HTTP/HTTPS，不转发任意 TCP 端口，否则邮件客户端永远连不上。

### 用 Let's Encrypt 证书替换自签证书

```bash
DOMAIN=mail.example.com
sudo certbot certonly --standalone -d $DOMAIN
sudo cp /etc/letsencrypt/live/$DOMAIN/fullchain.pem /etc/163-wrapper/cert.pem
sudo cp /etc/letsencrypt/live/$DOMAIN/privkey.pem  /etc/163-wrapper/key.pem
sudo chmod 600 /etc/163-wrapper/key.pem
sudo systemctl restart 163-wrapper      # Docker 模式：sudo docker restart 163-wrapper
```

自动续期 hook：

```bash
sudo tee /etc/letsencrypt/renewal-hooks/deploy/163-wrapper.sh > /dev/null <<EOF
#!/bin/sh
DOMAIN=$DOMAIN
cp /etc/letsencrypt/live/\$DOMAIN/fullchain.pem /etc/163-wrapper/cert.pem
cp /etc/letsencrypt/live/\$DOMAIN/privkey.pem  /etc/163-wrapper/key.pem
chmod 600 /etc/163-wrapper/key.pem
systemctl restart 163-wrapper
EOF
sudo chmod +x /etc/letsencrypt/renewal-hooks/deploy/163-wrapper.sh
```

### 验证

```bash
echo | openssl s_client -connect $DOMAIN:993 -servername $DOMAIN 2>&1 \
  | grep -E "subject=|Verify return"
```

期望看到 `subject=CN=mail.example.com` 和 `Verify return code: 0 (ok)`。

## 邮件客户端配置（验证可用）

### IMAP（收件）

| 字段 | 值 |
|------|----|
| 服务器 | 你的域名（如 `mail.example.com`），或 wrapper 监听的 IP |
| 端口 | `993` |
| 加密 | **SSL/TLS** |
| 认证 | Normal Password |
| 用户名 | 完整 163 邮箱（如 `you@163.com`） |
| 密码 | 163 **授权码**（不是登录密码） |

### SMTP（发件，直连 163）

wrapper 不代理 SMTP。客户端直接连 163 官方服务器：

| 字段 | 值 |
|------|----|
| 服务器 | `smtp.163.com` |
| 端口 | `587` ⭐ 推荐 |
| 加密 | **STARTTLS** |
| 认证 | Normal Password |
| 用户名 / 密码 | 同 IMAP |

> 备选端口 `465 + SSL/TLS` 也是 163 官方推荐的，但实测部分代理软件（Surge / Clash 的 TUN/VIF 模式）对 implicit TLS + 双栈并发握手处理有问题，会超时。**优先用 `587 + STARTTLS`**。

### 163 邮箱后台

登录 163 网页端 → 设置 → POP3/SMTP/IMAP：
- **同时打开 IMAP 和 SMTP 两个开关**（独立的）
- 生成**客户端授权码**，客户端填这个，不是登录密码

## 本地 / 局域网部署（可选）

如果不想搞 VPS，wrapper 也可以跑在本机或家庭服务器上（自签证书，需手动信任）。

### 构建

```bash
go build -o 163-wrapper .
```

### 运行（自定义端口示例）

```bash
cp config.example.yaml config.yaml
# 把 listen 改成 "127.0.0.1:1993"（>1024，不需要 sudo）
./163-wrapper -c config.yaml
```

### 信任自签证书（macOS）

wrapper 首次启动会在 data 目录生成自签 `cert.pem`：

```bash
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain \
  ~/.163-wrapper/cert.pem
```

iOS 需把 `cert.pem` AirDrop 过去 → 设置 → 通用 → VPN 与设备管理 → 安装描述文件 → 设置 → 通用 → 关于本机 → 证书信任设置 → 启用完全信任。

### 通过 Tailscale 远程访问

`listen` 设为 `0.0.0.0:993`，客户端 IMAP 服务器填该机器的 Tailscale IP（`100.x.x.x`），Tailscale 隧道天然加密，自签证书也安全。

## 调试

把 `config.yaml` 里 `log_level` 改为 `debug`，每条 IMAP 帧都会打印（**注意：debug 会把 LOGIN 行连同授权码完整记录到日志**，调试完务必改回 `info` 并视情况重置授权码）。

### 冒烟测试

```bash
( printf 'a1 LOGIN your@163.com YOUR_AUTHCODE\r\n'; sleep 2; \
  printf 'a2 LIST "" "*"\r\n'; sleep 2; \
  printf 'a3 LOGOUT\r\n'; sleep 1 ) | \
  openssl s_client -quiet -connect mail.example.com:993 -servername mail.example.com 2>/dev/null
```

期望：`a1 OK LOGIN completed` → 列出收件箱、已发送等文件夹 → `a3 OK LOGOUT completed`。

## 配置参考

```yaml
listen: "0.0.0.0:993"
upstream: "imap.163.com:993"
upstream_tls_server_name: "imap.163.com"
log_level: "info"
imap_id:                          # 注入给 163 的客户端身份（伪装成 Foxmail）
  name: "Foxmail"
  version: "7.2.25.230"
  vendor: "Tencent"
  support-email: "support@foxmail.com"
```

`-c` 指定配置文件、`-d` 指定数据目录（存 `cert.pem` / `key.pem`），命令行参数详见 `163-wrapper -h`。
