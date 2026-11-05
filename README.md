# Unity Starter

Unity 国际版 Hub + 团结引擎 Hub 的合体替代品：无广告、无遥测，原生 Go 界面（[mygo](https://github.com/egoist/mygo) 原生 UI），同时可作为 CLI 使用。

## 功能

- **项目**：扫描磁盘与两个 Hub 的项目列表，搜索、收藏、双击或按钮用对应版本编辑器打开，手动添加与新建项目。
- **安装**：识别 Unity / 团结引擎已安装的编辑器（默认目录、Hub 的 `secondaryInstallPath.json`、自定义目录），可启动与定位。
- **接管 Hub**：读取并写回 Hub 自己的文件——Unity Hub 的项目表（`projects-v1.json`）与两个 Hub 的收藏文件都是双向同步，Hub 侧改动会实时出现在这里。团结 Hub 没有可写的项目清单，项目来自磁盘扫描，收藏与目录可双向。
- **资源商店 / 游戏发行 / 付费与许可 / 技术文档 / 问题反馈**：直达官方入口；许可页列出本机检测到的授权文件；反馈页生成诊断信息并可预填 GitHub Issue。
- **多语言**：默认跟随系统语言，内置 English 与简体中文，设置页可手动切换；窗口与 CLI 共用同一套文案。

## 开发

需要 Go 1.27+。

```sh
go tool mygo dev     # 开发（热重载）
go test              # 视图与逻辑测试
go tool mygo build   # 打包 Windows 安装包到 build/
go tool mygo generate # 改了 locales/*.json 后重新生成语言包
```

## CLI

同一个可执行文件：带参数时不打开窗口。

```sh
UnityStarter projects [--query 文本] [--json]
UnityStarter editors [--json]
UnityStarter open <路径|名称>
UnityStarter new <目录> [--editor 版本]
UnityStarter hub status [--json]
UnityStarter version
UnityStarter help
```

`--json` 输出稳定的字段名（`name`/`path`/`version`/`engine`/`hub`/`favorite`/`editor`），`--lang en|zh-Hans` 切换文案。

## 数据位置

- 设置：`%APPDATA%\UnityStarter\settings.json`（Windows）。
- Hub 接管：`%APPDATA%\UnityHub` 与 `%APPDATA%\TuanjieHub`。

## 许可

MIT（尚未添加 LICENSE 文件前，按仓库主分支现状使用）。
