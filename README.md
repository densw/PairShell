# PairShell

**跨平台远程终端协助。先核对身份，再授权操作。**

[下载安装](https://github.com/densw/PairShell/releases/latest) · [官网](https://pairshell.top) · [自建服务](docs/deployment.md)

<img src="assets/demo.gif" alt="PairShell 双端实时协作演示" width="100%">

## 下载

| 平台 | 安装包 |
| --- | --- |
| Windows x64 | [求助方][win-help] · [协助方][win-support] · [便携版][win-zip] |
| macOS | [通用 DMG][mac]（Intel / Apple Silicon） |
| Linux | [amd64][linux-amd] · [arm64][linux-arm]（命令行与协调服务） |

[本版更新](docs/RELEASE-1.1.3.md) · [安装包验签](docs/release-signing.md)

本地最新发布文件统一位于 `dist/`，目录内仅保留安装包、说明和校验文件。

## 开始协助

1. **配对。** 双方使用同一协调服务。求助方生成一次性配对码，协助方输入该码。
2. **核对与授权。** 双方通过已有的电话或聊天互相核对本次校验码。确认一致后，求助方在自己的设备上授权。
3. **协作。** 协助方操作终端，求助方实时只读观看、查看本机记录，随时可以暂停或结束协助。

协调服务负责配对和转发，授权由求助方在本机完成。远程操作沿用求助方当前系统账号的权限，请只授权信任的人。[三方安全协助](docs/trust.md)

## 文档

[功能清单](docs/FEATURES-AND-UX.md) · [CLI / MCP](cmd/pairshell) · [传输与中继](docs/transport.md) · [贡献指南](CONTRIBUTING.md)

[win-help]: https://github.com/densw/PairShell/releases/download/v1.1.3-log-fix/PairShell-1.1.3-Windows-x64-Customer-Setup.exe
[win-support]: https://github.com/densw/PairShell/releases/download/v1.1.3-log-fix/PairShell-1.1.3-Windows-x64-Merchant-Setup.exe
[win-zip]: https://github.com/densw/PairShell/releases/download/v1.1.3-log-fix/PairShell-1.1.3-Windows-x64.zip
[mac]: https://github.com/densw/PairShell/releases/download/v1.1.3-log-fix/PairShell-1.1.3-macOS-universal.dmg
[linux-amd]: https://github.com/densw/PairShell/releases/download/v1.1.3-log-fix/PairShell-1.1.3-Linux-amd64.tar.gz
[linux-arm]: https://github.com/densw/PairShell/releases/download/v1.1.3-log-fix/PairShell-1.1.3-Linux-arm64.tar.gz
