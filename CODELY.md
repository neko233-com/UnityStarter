

## Codely Structured Memories

### User

### Feedback

### Project
- [2026-11-05 14:13:53] UnityStarter（D:\Code\neko233-Projects\UnityStarter）是 Unity 国际版 + 团结引擎 Hub 的无广告替代品，用 Go + github.com/egoist/mygo 原生 UI（非 Web 前端）实现；2026-11-05 与用户确认首版范围：只做项目管理、已安装编辑器检测、设置、中英双语，在线下载/安装引擎留到下一版。
- [2026-11-05 14:30:25] Windows 上 Hub 数据位置（UnityStarter 扫描/同步依据）：Unity Hub 配置在 %APPDATA%\UnityHub（projects-v1.json 存项目列表/上次打开时间且可写，secondaryInstallPath.json 存额外编辑器根目录，projectDir.json 存项目扫描目录）；两个 Hub 的 favoriteProjects.json 都是「JSON 字符串里再套 JSON 数组」的双重编码格式。团结 Hub 1.4.7 的 app.asar 中 0 处引用 projects-v1，没有可写的项目清单文件（只有 projectsArchitecture.json 存名称→架构），所以团结项目只能靠磁盘扫描，收藏/目录可双向。编辑器布局统一为 <root>\<version>\Editor\Unity.exe 或 Tuanjie.exe，团结版本号带 t 后缀（如 2022.3.62t17）。
- [2026-11-05 14:30:30] UnityStarter 的公开仓库是 https://github.com/neko233-com/UnityStarter（gh 账号 neko233-com，默认分支 main），2026-11-05 已向 GitHub 用户 3cheung 发出 write 权限协作者邀请。

### Reference

