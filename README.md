# UnityStarter

Unity 国际版 Hub + 团结引擎 Hub 的合体替代品：无广告、无遥测，原生 Go 界面（[mygo](https://github.com/egoist/mygo) 原生 UI，无 WebView），同一个可执行文件也能当 CLI 用。`UnityStarter.exe` 约 12 MB；3.8 GB 是 Unity 官方引擎安装包的大小，Hub 只是替你下载和安装它。

Windows 10/11 x64，单文件约 10 MB。

## 功能

- **项目**：扫描磁盘、Hub 项目表以及 Unity／团结 Editor 在 Windows 注册表中的最近项目；自动发现时跳过系统临时目录和已有项目的 `Library`／`Temp` 内的工作副本，手动添加、收藏或 Hub 明确列出的项目仍会保留。支持搜索、排序、收藏、双击打开、手动添加和新建项目。项目行的更多菜单可隐藏项目，在设置中可恢复；这只影响 UnityStarter，不修改项目文件或 Hub 记录。项目列表优先显示，Git 状态在后台补充；没有 Hub 或关闭同步时，收藏保存在本机。点击项目行的编辑器版本，可以选择同一引擎的已安装版本；选择不同版本时确认风险后再打开，项目版本由 Editor 实际迁移。新建项目时，同版本的多个安装目录可分别选择，项目名必须是有效的文件夹名称。
- 新建项目在编辑器启动后先显示为等待状态；确认版本文件、`Assets` 和有效包清单出现后才登记并同步 Hub。等待状态保存在本机，重启后继续检查；可以停止跟踪而不删除文件。CLI 发起创建后，由运行中的或下次打开的 UnityStarter 完成登记。文件结构存在并不代表资源导入或许可验证已经完成。
- 跨版本确认会显示目标编辑器的版本和路径；确认期间编辑器列表发生变化时须重新选择。GUI 和 CLI 在启动前会核对可读取的编辑器程序版本，发现与扫描结果不一致时停止打开并提示刷新。
- **安装与下载**：识别 Unity / 团结引擎已安装的编辑器（默认目录、Hub 的 `secondaryInstallPath.json`、自定义目录），同版本的不同安装目录也可分别管理；可选择版本父目录、版本目录或 `Editor` 文件夹，Windows 上优先读取编辑器程序的实际版本。下载页分别读取两个引擎的官方版本清单，可按版本号查询 Unity 官方历史版本（每次最多 25 个匹配项），一次点击下载并安装具备官方校验信息的版本，安装目录在点击时确定；执行安装程序前再次核对文件。官方服务器支持时使用最多 6 个连接并行下载；具有校验值的编辑器及 OpenJDK 包支持网络中断或异常退出后续传，取消下载会清理进度；不支持分段时使用单连接。离线时使用本程序最近一次缓存，首次使用可回退到本机 Hub 缓存。
- **接管 Hub（双向）**：读取并写回 Hub 自己的文件。
  - Unity Hub：项目表 `projects-v1.json`、收藏 `favoriteProjects.json`，新增项目与收藏会写入 Hub，Hub 侧改动实时同步回来。
  - 团结引擎 Hub：收藏与目录双向；它的项目清单不存在可写文件，项目来自磁盘扫描。
  - 写入保留 Hub 原有字段，临时文件 + rename 原子替换；Hub 运行时可能用内存状态覆盖，下一次 Hub 写文件时本程序会重新导入。
- **服务页**：资源商店、游戏发行、付费与许可、技术文档、问题反馈，链接取自两个 Hub 的 `cloudConfig.json`；许可页列出本机授权文件，反馈页一键生成带诊断信息的 GitHub Issue。
- **多语言**：默认跟随系统语言，内置 English 与简体中文，窗口与 CLI 共用文案。
- **工作台**：原生深色界面包含一体化标题栏和系统窗口控制；左侧垂直切换 Unity 国际版和团结引擎，每个引擎有独立的项目、安装、下载与收藏；编辑器和下载版本按版本从新到旧排列。团结版本映射可用时同时显示产品版本和底层版本。
- **编辑器管理**：安装页提供搜索、正式版／预览版筛选、定位、安装入口、已安装模块和卸载入口，并可直接选择新安装目录；切换目录后仍会扫描此前的安装目录。Unity 官方模块清单按编辑器版本缓存，离线时显示保存的清单与经文件核对的本地安装状态，并可手动刷新。可直接安装官方清单中受支持的独立平台模块、OpenJDK、Unity 和团结语言包及离线文档，以及部分 Android SDK／NDK 依赖；其他模块需使用官方 Hub。已安装的 Unity 和团结离线文档可从文档页打开手册和脚本 API。
- 已知模块的 Hub 安装标记会结合本机路径检查：平台模块要求目录非空，部分工具和文档会进一步检查关键文件；未知格式显示为「Hub 标记已安装」。这些检查不能替代真实平台构建验证。
- Unity Hub 的模块清单可使用 `integrity` 或 `checksum` 表示 MD5；两者同时存在时必须一致。安装后只更新模块选择状态，保留清单中的其他字段。
- 卸载编辑器前需确认操作；原生界面调用该安装自带的官方静默卸载器，先检查同一路径的编辑器进程，等待 `Editor` 目录消失后才报告完成。超时、进程异常退出或目录仍有文件时会保留保护记录，阻止 UnityStarter 再次操作该安装；确认官方卸载器已停止后可在界面解除保护。仍可选择打开官方交互式卸载器；项目目录不由 UnityStarter 删除。
- UnityStarter 的窗口和 CLI 通过跨进程锁协调同一编辑器的启动、安装、模块写入与卸载。其他程序不受此锁约束，因此卸载时仍应避免通过官方 Hub 或其他工具启动该编辑器。

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

模拟编辑器不会用于打开或新建真实项目；存在同版本真实编辑器时优先使用真实编辑器。关闭演示模式后仍可安装真实版本，但如果模拟目录正好占用了目标安装路径，需要先移除该模拟项或选择其他安装目录。

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
task install-local    # 构建并替换本机 MSI，启动程序，不发布
task checksums        # 生成 SHA256SUMS.txt
go tool mygo generate # 修改 locales/*.json 后重新生成语言包
```

`task install-local` 会先关闭本机 UnityStarter；若窗口未及时关闭，会强制结束受管理的 UnityStarter 进程，然后安装并启动本机构建。`task install` 使用环境变量 `MSI_PATH`、`MSI_SHA256` 和 `EXE_SHA256` 校验、安装并启动 MSI。CI 见 `.github/workflows/ci.yml`：测试并构建产物；推送与版本一致的 `v*` 标签后通过 `task release` 发布 Release。

## 已知限制

- 团结引擎 Hub 的项目清单不可写，新增的团结项目不会出现在团结 Hub 里（收藏与目录会同步）。
- 资源商店与发行页只打开官方站点，官方没有开放商店/发行 API。
- 发布包未做代码签名，Windows 可能提示未知发布者。
- Unity Alpha / Beta 属于预览版本，兼容性和稳定性与正式版不同。
- Unity Personal 许可由 Unity 官方服务管理，首次激活需要在 Unity Hub 登录；本机许可文件存在不代表 Editor 已验证有效。UnityStarter 不生成或绕过许可。
- 新安装的编辑器默认保存在 `%LOCALAPPDATA%\UnityStarter\Editors`，独立于应用安装目录。旧版本若将编辑器放在应用目录的 `Editors` 中，卸载器会阻止卸载，先将其移到应用目录之外即可。
- 安装中断的编辑器不会显示为已安装；安装完成前核对编辑器程序版本和必要核心文件，可从下载页重新尝试安装。已有编辑器目录不会被自动覆盖；文件存在和版本匹配仍不代表 Editor 的许可或项目启动已验证。
- 断点续传需要官方文件校验值、分段支持和服务器文件标识；版本标识变化或校验失败时不会使用旧数据。已下载的完整文件校验通过后可离线复用。Android 多包下载中断后可在重新确认许可时复用缓存；主动取消下载会清理该计划的缓存，设置页可打开缓存目录。
- 部分旧版 Unity 官方清单使用编码后的十六进制 MD5 和近似大小；下载时仍校验完整文件，但不将该大小估计值当作精确字节数。
- Android SDK／NDK 提交时发生异常退出，可在重新确认许可后核对文件并继续恢复；若文件已被修改则保留原样并提示。解压过程中异常退出可能留下编辑器目录内的隐藏暂存目录，不会将其标记为已安装。
- 团结离线文档包使用官方提供的 MD5 校验值；安装时不会覆盖已有非空文档目录。解压过程中异常退出可能留下 `Editor/Data` 内的隐藏暂存目录。
- Unity 离线文档清单没有固定摘要且大小可能过期；下载时读取官方 HTTPS 响应的当前大小和 MD5，核对完整文件及 ZIP 内容，再安装到空目录。服务器 MD5 可检测传输损坏，不是独立签名；现有非空文档目录不会被覆盖。
- Unity 和团结语言包从各自官方 HTTPS 地址获取并检查 PO 格式和语言标识；官方清单未提供文件校验值，无法按清单进行摘要验证。安装不会覆盖已有语言文件。
- 模块安装支持具有校验值的受支持独立安装包（包括 Linux IL2CPP）、官方带 SHA-256 的 OpenJDK ZIP、可从官方清单和 Google 仓库核对的 Unity Android SDK／NDK，以及匹配固定 8 包清单的团结旧版 Android SDK／NDK。团结旧包使用 2026-11-06 从 Google HTTPS 下载建立的 SHA-256 基线；部分包已不在 Google 当前仓库清单中，该基线不是官方签名的摘要。Android 安装前需要用户阅读并接受 Google 许可，SDK／NDK 目录必须不存在或为空；其他不符合条件的模块仍需官方 Hub。本程序最小安装的编辑器若没有 Hub 模块清单，模块安装状态保存在本程序的数据目录中。
- Unity 7 Android 多包安装已验证官方清单、隔离安装与回滚；真实大包安装和 Android 项目构建仍需在接受许可后验收。
- 编辑器或独立模块安装程序需要管理员权限时，Windows 会显示 UAC 授权；拒绝授权时安装会失败，不会标记为成功。OpenJDK ZIP 直接写入编辑器目录，需要当前用户具备写入权限。

## 许可

[MIT](LICENSE)
