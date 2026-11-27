# PairShell 安装包签名

发布机使用专用 Ed25519 发布密钥对 `SHA256SUMS.txt` 生成 OpenSSH 分离签名。清单包含 Windows、macOS 和 Linux 的发行文件摘要。验证签名先确认清单来自受信任的发布密钥，再核对下载文件的摘要。协调服务器和 TURN 不参与签名。

## 发布者

在独立发布环境中保管发布私钥和备份。密钥应专用于发布，和 SSH 登录密钥分开，并放在仓库之外。中继服务器的账号、备份、CI 运行器和部署凭据都不应有访问权限。通过用户已经信任的独立渠道提供发布公钥或确认其指纹。

先完成原生签名、打包和验证。用现有组装脚本汇总并签名，或对已经汇总好的目录单独签名。下列路径是示例，两种方式任选其一。

```text
python scripts/assemble-release.py --version 1.1.3 --signing-key /secure/pairshell-release --signing-public-key /secure/pairshell-release.pub
python scripts/release_signing.py sign dist --private-key /secure/pairshell-release --public-key /secure/pairshell-release.pub
```

把安装包、`SHA256SUMS.txt` 和 `SHA256SUMS.txt.sig` 一起发布。本地开发可以组装未签名包。正式发布应先用通过独立渠道确认的公钥进行完整校验。签名后重新打包或修改任何发行文件，都应重新生成清单并重新签名。

## 使用者

将安装包、清单和签名下载到同一个目录，把事先信任的发布公钥保存在另一个位置。从已信任的源码副本运行验证脚本。先确认下载的是你预期的版本，再运行以下任一命令。收到一个安装包时，使用 `--artifact` 指定它的文件名。收到整个正式发布目录时，省略该选项。

```text
python scripts/release_signing.py verify downloads --public-key /trusted/pairshell-release.pub --artifact PairShell-1.1.3-Windows-x64.zip
python scripts/release_signing.py verify downloads --public-key /trusted/pairshell-release.pub
```

成功校验会确认签名匹配你提供的公钥，且所检查的文件匹配签名清单。请勿直接运行与压缩包一起下载的已解包目录。验证需 Python 3.9+ 和支持 `ssh-keygen -Y` 的 OpenSSH。[OpenSSH 签名与验证说明](https://man.openbsd.org/ssh-keygen.1)

分离签名用于验证发布文件的来源和完整性。Windows Authenticode、Apple Developer ID 签名及 Apple 公证由各平台的发布流程单独提供。
