# kos-taskmanager

KOS 系统应用项目：运行视图 — 概览、应用、应用详情与运行中的 Pod。

- 设计：`tsic/kos-design` 仓的 `kos-taskmanager/`（范围、契约、验收、决策记录）
- 设计依据：`tsic/kos-design@19f6b81`

## 边界

- 只读运行状态与发起控制面动作；不拥有应用生命周期、卷与入口，不保存日志或事件。
- 浏览器不持有集群凭据：页面只经控制面 API 与自身应用令牌读写（`KOSD-F-9`、`KOSD-C-12`）。
- 本仓库交付一个独立扩展制品：装成系统组件后，其窗口应用出现在桌面，卸载或停用后消失（`KOSD-C-17`、`KOSD-AC-72`、`KOSD-AC-54`）。

## 现有实现

| 路径 | 职责 |
|---|---|
| `app/` | `App` 与扩展声明：唯一清单来源，字段集合按 `KOSD-C-73`、`KOSD-C-18` |
| `internal/artifact/` | 打包：读 `assets/`，经 kos-sdk 的 `packaging` 产出工件并用安装路径的检查自验（`SDK-C-3`、`SDK-C-4`） |
| `cmd/package/` | 构建入口：写 `dist/kos-taskmanager-<版本>.tar.gz` 与 `.digest` |
| `assets/` | 窗口文档与 UI 资产：入口文档、脚本、样式、图标与四个页面壳 |
| `assets/router.js` | 四页路由：概览、应用、应用详情、Pod 运行态 |
| `assets/handshake.js` | 应用侧会话通道：`ready` / `init`、存活与未保存上报（`KOSD-C-75`、`KOSD-C-81`） |
| `assets/pages/` | 四个页面壳；页面正文由其后的子单接入控制面投影 |
| `tests/acceptance/` | 按设计编号命名的验收用例 |

## 清单

`app.Declaration()` 是打包与用例共用的唯一声明：

- 扩展 `kos-taskmanager`，一个 `apps` 贡献，无对象类、控制器、设置项与页面贡献。
- `App` 必填项齐全；`entry` 是独立 origin 上的 HTTPS 文档地址，与控制台不同源；`permissions` 为空数组，`backend` 与 `deepLinks` 缺席（UI-only）。
- 卸载保留语义：`apps` 贡献删除，无外部资源；包不拥有任何数据。

入口 origin 由部署映射到本包资产（`https://taskmanager.apps.kos.tsic.top`）；控制台按扩展、应用、版本与包摘要签发短期资产地址（`KOSD-C-73`）。

## 构建与运行

```sh
make lint        # gofmt 检查、go vet、ES 模块语法检查
make test        # Go 单元与验收用例，以及 Node 页面壳用例
make acceptance  # 只跑 tests/acceptance/，-v
make package     # 产出 dist/ 下的包与摘要
```

本地查看页面壳：把仓库根目录用静态文件服务打开 `assets/index.html`。未带控制台来源时窗口按独立预览运行，只渲染页壳与路由，不读取任何数据。

## 依赖

- kos-sdk：清单结构、打包与安装路径检查（`SDK-C-1`、`SDK-C-3`、`SDK-C-4`）。
- kos-dashboard：窗口与页面宿主；验收用例经它的应用注册表与资产存储判断装载、卸载与停用后的可见性。
- kosd：控制面 API、Pod 投影与动作（`KOSD-C-12`、`KOSD-C-76`、`KOSD-C-77`）。
