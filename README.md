# 163-gmail-server-wrapper

让 Spark / Apple Mail 等标准 IMAP 客户端能正常使用 163 邮箱。

## 背景

163/126 邮箱要求 IMAP 客户端在 `LOGIN` 成功后立刻发送非标准的 `ID` 命令（RFC 2971），否则返回 `BAD Unsafe Login` 并断开。本工具在本地起一个 IMAP 代理，在 LOGIN OK 后自动注入 ID，其余流量透明转发。

**只代理 IMAP**。SMTP 直连 `smtp.163.com:465` 即可，无需经过本代理。

## 构建

```bash
go build -o 163-wrapper .
```

## 运行

```bash
# 使用默认配置（监听 127.0.0.1:1143）
./163-wrapper

# 使用自定义配置
cp config.example.yaml config.yaml
./163-wrapper -c config.yaml
```

## 163 邮箱后台设置

登录 163 邮箱 → 设置 → POP3/SMTP/IMAP → **开启 IMAP 服务** → **生成授权码**（在客户端里用授权码而非登录密码）。

## Spark 配置

### 首次运行：信任自签证书（macOS）

wrapper 第一次启动时会自动生成自签证书，保存到 `~/.163-wrapper/cert.pem`。
需执行一次以下命令让 macOS 和 Spark 信任它：

```bash
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain \
  ~/.163-wrapper/cert.pem
```

### IMAP（收信）

| 字段 | 值 |
|------|----|
| 服务器 | wrapper 的 IP 地址（本机填 `127.0.0.1`，Tailscale 填 100.x.x.x） |
| 端口 | `1993` |
| SSL/TLS | **SSL**（wrapper 本地已经是 TLS，Tailscale 隧道外层再加密） |
| 用户名 | 完整 163 邮箱地址，如 `example@163.com` |
| 密码 | 163 **授权码**（不是登录密码） |

### SMTP（发信，直连官方服务器）

| 字段 | 值 |
|------|----|
| 服务器 | `smtp.163.com` |
| 端口 | `465` |
| SSL/TLS | 开启 |
| 用户名 | 同上 |
| 密码 | 同上（授权码） |

## 通过 Tailscale 远程访问

1. 将 `config.yaml` 里 `listen` 改为 `0.0.0.0:1143`（或具体 Tailscale 接口 IP）
2. 在运行 wrapper 的机器上确认防火墙放行 1143 端口（仅 Tailscale 网络可见）
3. Spark 的 IMAP 服务器填该机器的 Tailscale IP（`100.x.x.x`）

## 调试

将 `config.yaml` 里 `log_level` 改为 `debug`，每条 IMAP 帧都会打印到 stdout，方便确认 ID 注入时序。

## 冒烟测试

```bash
# 启动 wrapper
./163-wrapper &

# 用 nc 手动测试（回车用 Ctrl-V Ctrl-M 输入 CR）
nc 127.0.0.1 1143
# 服务器应返回 * OK 欢迎横幅，然后：
a1 LOGIN your@163.com <授权码>
# 预期：a1 OK，此时 ID 已注入
a2 LIST "" "*"
# 预期：列出 INBOX、已发送 等文件夹
a3 LOGOUT
```
