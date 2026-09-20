---
title: "发布与安装"
weight: 1
summary: "各生态的 registry 地址与认证方式。"
---

支持的生态与基本用法：

- **npm**：`npm publish --registry <实例>/api/packages/<owner>/npm/`
- **composer**：配置 `repositories` 指向实例。
- **pypi**：`twine upload --repository-url <实例>/api/packages/<owner>/pypi/`
- **rubygems**：`gem push --host <实例>/api/packages/<owner>/rubygems/`
- **Go modules**：`GOPROXY=<实例>/api/packages/<owner>/go`
- **cargo**：配置 registry 指向实例。
- **Maven**：配置 `distributionManagement`。

认证：用户名任意，口令填 PAT。

详见仓库文档 `docs/packages.md` 与 `examples/packages/`。
