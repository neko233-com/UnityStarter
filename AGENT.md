# UnityStarter — Agent 执行手册

本文档给后续 Agent 使用。所有构建/安装链路已自动化，Agent 只需按步骤执行。

## 项目概要

- **产品**: UnityStarter — Unity 国际版 + 团结引擎 Hub 的无广告替代品
- **技术栈**: Go + github.com/egoist/mygo（原生 UI，非 Web 前端）
- **仓库**: https://github.com/neko233-com/UnityStarter（main 分支）
- **平台**: Windows amd64（per-user 安装）
- **当前版本**: 1.6.24

## 目录结构

```
UnityStarter/
├── Taskfile.yml            # ⭐ 所有构建/发布的入口
├── mygo.json               # mygo 构建配置（版本号在这里）
├── icons.go                # SVG 品牌图标（iconUnity / iconTuanjie / iconBrand）
├── engine_strategy.go      # ⭐ Unity/Tuanjie 差异策略收敛点
├── app.go                  # 主应用 + 侧栏 + 路由
├── auto_update_windows.go  # 在线 MSI 自动更新（msiexec /i ... /qn）
├── resources/icon.svg      # 应用图标（任务栏/窗口）
├── _tools/
│   ├── UnityStarter.wxs        # WiX 源（MSI 安装包定义）
│   ├── build-msi.ps1          # 从 .wxs 构建 MSI
│   ├── install-local.ps1      # 构建 + 静默安装 + 启动
│   ├── install-msi.ps1       # 验证 SHA256 后 msiexec /qn
│   ├── release-checksums.ps1  # SHA-256 校验
│   └── publish-release.ps1    # GitHub Release 发布
├── build/windows-amd64/    # 构建产物（exe + setup.exe + msi）
└── .github/workflows/ci.yml  # CI: push main → test → build → artifact
```

## 构建产物

每次 `task build` 产出：
| 文件 | 用途 | 大小 |
|---|---|---|
| `UnityStarter.exe` | 原生可执行文件 | ~13.7 MB |
| `unitystarter-setup-<ver>-amd64.exe` | NSIS 安装程序（mygo 产） | ~4.6 MB |
| `unitystarter-<ver>-amd64.msi` | MSI 安装包（WiX 产，自动更新用） | ~5.1 MB |

## ⭐ 标准执行流程（每次处理完必走）

```powershell
# 1. 测试
task test

# 2. 构建（mygo build → NSIS setup + wix MSI）
task build

# 3. 静默安装到本机 per-user 目录并启动
task install-local

# 4. 验证安装版本（必须输出目标版本号才算成功）
& "$env:LOCALAPPDATA\Programs\UnityStarterMSI\UnityStarter.exe" version

# 5. 提交 + push（workspace 干净）
git add -A
git commit -m "<type>: <summary>"
git push origin main
```

**安装位置**: `%LOCALAPPDATA%\Programs\UnityStarterMSI\`
**卸载**: `msiexec /x {b3373319-8b2b-4b88-a782-725de6fb774a} /qn`（WiX UpgradeCode）

## Engine 差异策略（engine_strategy.go）

Unity 和 Tuanjie 不是同构 tab —— 每个 engine 有独立品牌 + 行为：

```go
type engineStrategy struct {
    engineBranding    // 图标、品牌色、选中态背景
    Pages             // 路由映射
    NeedsLicenseFlow  // Unity 需要授权流程，Tuanjie 不需要
    DownloadChannel   // 下载源
    AssetStoreURL     // 资源商店 URL
    DocsURL           // 文档 URL
    // ...
}
```

**规则**:
- 新增 engine 差异时，只改 `engine_strategy.go`，不要在 view 层加 `if kindUnity`
- UI 层通过 `a.engineStrategy()` 或 `engineStrategyFor(kind)` 获取策略
- 品牌色: Unity `#E4002B`（红）/ Tuanjie `#2196F3`（蓝）

## 品牌图标链路

```
icons.go ── iconUnity / iconTuanjie / iconBrand（内联 currentColor SVG）
   │                              │
   ├── 侧栏品牌区 ── engineStrategy.Icon ── 跟随 activeEngine
   └── 主内容空状态 ── iconBrand（共享 Unity+X 标记）

resources/icon.svg ── 应用图标（任务栏/窗口）── Unity+X Logo（彩色）
```

## MSI 自动更新链路

```
auto_update_windows.go → msiexec /i unitystarter-<ver>-amd64.msi /qn /norestart
                                    ↑
                           GitHub Release asset（sha256 校验）
```

升级规则: per-user MSI，MajorUpgrade `AllowSameVersionUpgrades="yes"`。

## 提交规范（Conventional Commits）

```
feat: 添加 xxx 功能
fix: 修复 xxx 问题
refactor: 重构 xxx（不改变行为）
chore: 构建/依赖/配置调整
```

## 验证标准

每次交付必须满足：
- `go build ./...` 零错误零警告
- `go test ./...` 全部通过
- `task build` 产出 exe + setup.exe + msi
- `task install-local` 静默安装成功，版本号匹配

## 已知约束

- Windows runner 的 `core.autocrlf` 会把 LF 变 CRLF → 仓库用 `.gitattributes` 强制 `* text=auto eol=lf`
- 引擎安装包 GB 级验证用"模拟安装"，不要真的下载
- 版本号在 `mygo.json`，MSI 版本从这里读取
