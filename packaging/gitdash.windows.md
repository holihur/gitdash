# gitdash Windows 部署

## 直接运行

下载并解压 `gitdash_<version>_windows_<arch>.zip`（或用 [install.ps1](../install.ps1) 安装），然后：

```powershell
gitdash serve    # 默认 http://localhost:8080 / ssh :2222
```

注意：内置 SSH 服务器监听 :2222（无特权端口问题）；如需 :22，需以管理员身份运行或配置端口转发。

## 作为 Windows 服务运行

gitdash 目前是前台进程，可用 [NSSM](https://nssm.cc/)（或 WinSW）包装为服务：

```powershell
# 以管理员身份运行 PowerShell
choco install nssm   # 或到 nssm.cc 下载

nssm install gitdash "C:\Program Files\gitdash\gitdash.exe" "serve"
nssm set gitdash AppEnvironmentExtra GITDASH_DATA=C:\ProgramData\gitdash
nssm set gitdash AppStdout C:\ProgramData\gitdash\gitdash.log
nssm set gitdash AppStderr C:\ProgramData\gitdash\gitdash.log
nssm start gitdash
```

卸载：

```powershell
nssm stop gitdash
nssm remove gitdash confirm
```

## 自动更新

自动更新（`GITDASH_AUTO_UPDATE=1` 或 `gitdash update`）在 Windows 上可用：更新时会先把旧 exe 改名为 `.old` 再替换，随后退出；由 NSSM 等服务管理器（`Restart=always` 类似配置）重新拉起新版本。
