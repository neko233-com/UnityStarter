# UnityStarter

Unity 国际版 Hub + 团结引擎 Hub 的合体替代品：无广告、无遥测，原生 Go 界面（[mygo](https://github.com/egoist/mygo) 原生 UI，无 WebView），同一个可执行文件也能当 CLI 用。

Windows 10/11 x64，单文件约 10 MB。

## 功能

- **项目**：扫描磁盘与两个 Hub 的项目列表，搜索、收藏、双击或按钮用对应版本编辑器打开，手动添加、新建项目。
- **安装**：识别 Unity / 团结引擎已安装的编辑器（默认目录、Hub 的 `secondaryInstallPath.json`、自定义目录），可启动与定位。
- **接管 Hub（双向）**：读取并写回 Hub 自己的文件。
  - Unity Hub：项目表 `projects-v1.json`、收藏 `favoriteProjects.json`，新增项目与收藏会写入 Hub，Hub 侧改动实时同步回来。
  - 团结引擎 Hub：收藏与目录双向；它的项目清单不存在可写文件，项目来自磁盘扫描。
  - 写入保留 Hub 原有字段，临时文件 + rename 原子替换；Hub 运行时可能用内存状态覆盖，下一次 Hub 写文件时本程序会重新导入。
- **服务页**：资源商店、游戏发行、付费与许可、技术文档、问题反馈，链接取自两个 Hub 的 `cloudConfig.json`；许可页列出本机授权文件，反馈页一键生成带诊断信息的 GitHub Issue。
- **多语言**：默认跟随系统语言，内置 English 与简体中文，窗口与 CLI 共用文案。

## 安装

从 [Releases](https://github.com/neko233-com/UnityStarter/releases) 下载：

- `unitystarter-setup-<version>-amd64.exe`：安装到 `%LOCALAPPDATA%\Programs\UnityStarter`，带开始菜单与桌面快捷方式。
- `UnityStarter.exe`：免安装，直接运行。

## CLI

同一个 `UnityStarter.exe`，带参数时不打开窗口：

```sh
UnityStarter.exe projects [--query 文本] [--json]
UnityStarter.exe editors [--json]
UnityStarter.exe open <路径|名称>
UnityStarter.exe new <目录> [--editor 版本]
UnityStarter.exe hub status [--json]
UnityStarter.exe hub sync [--json]
UnityStarter.exe version
UnityStarter.exe help
```

- `--json`：稳定字段（`name`/`path`/`version`/`engine`/`hub`/`favorite`/`editor`，Hub 的 `mode` 为 `two-way`/`import-only`）。
- `--lang en|zh-Hans`：切换文案，默认跟随系统。
- `--verbose`：日志输出到终端。
- 退出码：`0` 成功，`1` 失败（找不到项目、无编辑器等），`2` 命令或参数错误。

## 数据与日志

- 设置：`%APPDATA%\UnityStarter\settings.json`（旧的带空格目录会在首次运行时读取）。
- 日志：`%APPDATA%\UnityStarter\logs\app.log`，超过 1 MB 轮转一次；设置页有「打开日志」。
- Hub 接管：`%APPDATA%\UnityHub`、`%APPDATA%\TuanjieHub`。

## 开发

需要 Go 1.27+。

```sh
go tool mygo dev      # 开发（热重载）
go test ./...         # 视图、CLI、Hub 同步、扫描测试
go vet ./...
go tool mygo build    # 打包 exe 与 NSIS 安装包到 build/windows-amd64/
go tool mygo generate # 修改 locales/*.json 后重新生成语言包
```

CI 见 `.github/workflows/ci.yml`：format、vet、test，构建产物上传；推送 `v*` 标签时自动发布 Release。

## 已知限制

- 团结引擎 Hub 的项目清单不可写，新增的团结项目不会出现在团结 Hub 里（收藏与目录会同步）。
- 资源商店与发行页只打开官方站点，官方没有开放商店/发行 API。
- 发布包未做代码签名，Windows 可能提示未知发布者。

## 许可

[MIT](LICENSE)
