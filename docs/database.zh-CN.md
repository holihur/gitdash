# 数据库后端（SQLite / PostgreSQL）

gitdash 在同一套 `store.Store` API 下支持两种存储后端：

- **SQLite**（默认）—— 纯 Go `modernc` 驱动，文件位于 `GITDASH_DATA`
- **PostgreSQL** —— 设置 `GITDASH_DB=postgres://user:pass@host:5432/db?sslmode=disable`

两者都用 `internal/store/migrate.go` 里的 GORM `AutoMigrate` 建/补表。

## 共同约定

- **时间戳**统一以 RFC3339 UTC **字符串**存储（`TEXT` / `varchar`），不用原生日期类型。因此排序与 `<`/`>` 比较是字典序，两端行为一致。
- **布尔值**：SQLite 存 `0/1`，PostgreSQL 存 `boolean`；GORM 透明转换，且插入 `false` 时用 map 绕过零值省略（仓库 `private`）。
- **空字符串**统一 `NOT NULL DEFAULT ''`，不存在 `NULL` 与 `''` 的歧义。
- **Upsert** 使用 `ON CONFLICT ... DO UPDATE`，两端都支持。
- **部分唯一索引**：`users.email` 上的 `WHERE email <> ''`，两端都支持；空邮箱不参与唯一性。
- **唯一冲突映射**：`TranslateError: true` 把驱动错误转成 `gorm.ErrDuplicatedKey`，`isUniqueErr` 再映射为 `store.ErrExists`/`ErrNotFound`。

## 已知差异与处理方式

| 方面 | SQLite | PostgreSQL | 代码中的处理 |
| --- | --- | --- | --- |
| 写并发 | 单写者；`busy_timeout(5000)` + WAL | MVCC、行级锁 | 事务尽量短；显式 busy_timeout pragma |
| `LIKE` 大小写 | ASCII 不区分 | 区分 | 面向用户的过滤统一 `LOWER(col) LIKE <小写模式>`（全局搜索、admin 用户列表）；前缀匹配用全小写 key |
| 锁/争用错误 | `SQLITE_BUSY`（busy_timeout 内重试） | 锁等待 / 序列化失败 | 不显式重试；保持事务短小 |
| 方言特定 SQL | — | — | 限制在 GORM/可移植子集；原生 SQL 仅用两端都支持的双引号标识符（`"key"`） |
| 配额并发 | 进程级 `createMu` + `COUNT` | 进程级 `createMu` + `COUNT` | 多实例部署仍可能超额放行，属已记录的限制（见 `internal/store/quota.go`） |

## 相关位置

- 后端选择：`store.Open` / `store.OpenDSN`
- 迁移 / schema：`internal/store/migrate.go`
- 错误映射：`isUniqueErr`（`internal/store/store.go`）
- PostgreSQL 冒烟测试：`internal/store/pgsmoketest/`（CI 中带 postgres service 运行）

```bash
# PostgreSQL 冒烟测试
GITDASH_DB="postgres://gitdash:pg@127.0.0.1:54329/gitdash?sslmode=disable" \
  go test ./internal/store/pgsmoketest/ -count=1 -v
```
