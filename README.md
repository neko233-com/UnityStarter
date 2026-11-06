# UnityStarter

Unity 国际版 Hub + 团结引擎 Hub 的合体替代品：无广告、无遥测，原生 Go 界面（[mygo](https://github.com/egoist/mygo) 原生 UI，无 WebView），同一个可执行文件也能当 CLI 用。`UnityStarter.exe` 约 12 MB；3.8 GB 是 Unity 官方引擎安装包的大小，Hub 只是替你下载和安装它。

Windows 10/11 x64，单文件约 10 MB。

## 功能

- **项目**：扫描磁盘与两个 Hub 的项目列表；行内显示引擎、渲染管线（URP/HDRP/内置）、Git / SVN / Lore 分支与未提交状态，可一键打开仓库地址；搜索、收藏、双击打开、手动添加、新建项目。
- **安装与下载**：识别 Unity / 团结引擎已安装的编辑器（默认目录、Hub 的 `secondaryInstallPath.json`、自定义目录），多套版本并存管理；下载页直接列出 Unity 国际版（`services.api.unity.com` → `download.unity3d.com`）与团结引擎（`tuanjie.cn`）的官方安装包，不跳转、不换源，应用内下载 + 静默安装。官方服务器支持时使用最多 6 个连接并行下载，Unity 的下载包按官方提供的校验值验证；不支持分段时使用单连接。
- **接管 Hub（双向）**：读取并写回 Hub 自己的文件。
  - Unity Hub：项目表 `projects-v1.json`、收藏 `favoriteProjects.json`，新增项目与收藏会写入 Hub，Hub 侧改动实时同步回来。
  - 团结引擎 Hub：收藏与目录双向；它的项目清单不存在可写文件，项目来自磁盘扫描。
  - 写入保留 Hub 原有字段，临时文件 + rename 原子替换；Hub 运行时可能用内存状态覆盖，下一次 Hub 写文件时本程序会重新导入。
- **服务页**：资源商店、游戏发行、付费与许可、技术文档、问题反馈，链接取自两个 Hub 的 `cloudConfig.json`；许可页列出本机授权文件，反馈页一键生成带诊断信息的 GitHub Issue。
- **多语言**：默认跟随系统语言，内置 English 与简体中文，窗口与 CLI 共用文案。
- **工作台**：原生深色界面将 Unity 国际版和团结引擎分为两个独立分组，每组都有项目、安装、下载与收藏标签；编辑器和下载版本按版本从新到旧排列，并标记 LTS、Alpha 和 Beta。

## 安装

从 [Releases](https://github.com/neko233-com/UnityStarter/releases) 下载：

- `unitystarter-<version>-amd64.msi`：推荐的当前用户安装包，安装到 `%LOCALAPPDATA%\Programs\UnityStarterMSI`，注册开始菜单、App Paths 和用户 PATH。后台检查正式版更新，校验 MSI 的大小和 SHA-256 后下载；可以点击「更新并重启」，也可以下次启动时自动安装。安装失败会保留原程序并显示错误，可手动重试。
- `unitystarter-setup-<version>-amd64.exe`：原 NSIS 安装包，安装到 `%LOCALAPPDATA%\Programs\UnityStarter`，带开始菜单与桌面快捷方式；更新时请手动下载新版 MSI 或安装包。
- `UnityStarter.exe`：免安装，直接运行；若需命令行别名，可执行 `_tools\register-cli.ps1 -InstallDir <目录>`。

发布页的 `SHA256SUMS.txt` 可用于检查下载完整性。MSI 与旧版 NSIS 安装在不同目录，不会删除旧安装目录里的编辑器；安装 MSI 后，新终端优先使用 MSI 的命令行入口。

## CLI

安装包安装后，在**新开的**终端里可直接用 `unitystarter`（底层仍是同一 `UnityStarter.exe`）。带参数时不打开窗口：

```sh
unitystarter projects [--query 文本] [--json]
unitystarter editors [--json]
unitystarter open <路径|名称>
unitystarter new <目录> [--editor 版本]
unitystarter hub status [--json]
unitystarter hub sync [--json]
unitystarter releases [--json]
unitystarter install <版本> [--simulate]
unitystarter update [--json]
unitystarter version
unitystarter help
```

- `--json`：稳定字段（`name`/`path`/`version`/`engine`/`hub`/`favorite`/`editor`，Hub 的 `mode` 为 `two-way`/`import-only`）。
- `--lang en|zh-Hans`：切换文案，默认跟随系统。
- `--verbose`：日志输出到终端。
- 退出码：`0` 成功，`1` 失败（找不到项目、无编辑器等），`2` 命令或参数错误。

## 模拟安装（不下载数 GB）

设置页打开「演示模式」，下载页的按钮变成「模拟安装」：只创建 `<安装目录>\<版本>\Editor\Unity.exe` 的占位结构与标记文件，安装页会立刻出现该版本并标为「模拟」，可一键移除。用于验证下载→安装→项目匹配整条链路，不需要真的拉 3.8 GB。

CLI 同样可用：`unitystarter install 6000.0.85f2 --simulate`。

## 数据与日志

- 设置：`%APPDATA%\UnityStarter\settings.json`（旧的带空格目录会在首次运行时读取）。
- 日志：`%APPDATA%\UnityStarter\logs\app.log`，超过 1 MB 轮转，最多保留 5 份且超过 14 天自动删除；设置页有「打开日志」。
- Hub 接管：`%APPDATA%\UnityHub`、`%APPDATA%\TuanjieHub`。

## 开发

需要 Go 1.27+、Task 和 .NET SDK；MSI 构建通过本地工具清单安装 WiX。

```sh
go tool mygo dev      # 开发（热重载）
task test             # 测试与静态检查
task build            # 构建 exe、NSIS 和 MSI
task checksums        # 生成 SHA256SUMS.txt
go tool mygo generate # 修改 locales/*.json 后重新生成语言包
```

`task install` 使用环境变量 `MSI_PATH`、`MSI_SHA256` 和 `EXE_SHA256` 校验、安装并启动 MSI。CI 见 `.github/workflows/ci.yml`：测试并构建产物；推送与版本一致的 `v*` 标签后通过 `task release` 发布 Release。

## 已知限制

- 团结引擎 Hub 的项目清单不可写，新增的团结项目不会出现在团结 Hub 里（收藏与目录会同步）。
- 资源商店与发行页只打开官方站点，官方没有开放商店/发行 API。
- 发布包未做代码签名，Windows 可能提示未知发布者。
- Unity Alpha / Beta 属于预览版本，兼容性和稳定性与正式版不同。
- Unity Personal 许可由 Unity 官方服务管理，首次激活需要在 Unity Hub 登录；本机许可文件存在不代表 Editor 已验证有效。UnityStarter 不生成或绕过许可。
- 新安装的编辑器默认保存在 `%LOCALAPPDATA%\UnityStarter\Editors`，独立于应用安装目录。旧版本若将编辑器放在应用目录的 `Editors` 中，卸载器会阻止卸载，先将其移到应用目录之外即可。
- 安装中断的编辑器不会显示为已安装；可从下载页重新尝试安装。已有编辑器目录不会被自动覆盖。

## 许可

[MIT](LICENSE)
