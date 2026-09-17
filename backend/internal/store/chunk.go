package store

import "gorm.io/gorm"

// maxInParams 是单条 SQL 中 IN (...) 绑定参数的安全上限。
//
// SQLite 的 SQLITE_MAX_VARIABLE_NUMBER 旧版为 999、新版为 32766，
// PostgreSQL 为 65535。取一个远低于最小值的保守值，避免
// "too many SQL variables" 之类的边界错误；同时兼顾批量插入的行数。
const maxInParams = 400

// chunkStrings 把 s 按 size 切分为若干子切片；size<=0 时使用 maxInParams。
func chunkStrings(s []string, size int) [][]string {
	if size <= 0 {
		size = maxInParams
	}
	if len(s) == 0 {
		return nil
	}
	out := make([][]string, 0, (len(s)+size-1)/size)
	for len(s) > size {
		out = append(out, s[:size])
		s = s[size:]
	}
	if len(s) > 0 {
		out = append(out, s)
	}
	return out
}

// chunkInt64s 同 chunkStrings，用于 int64 主键 / ID 列表。
func chunkInt64s(s []int64, size int) [][]int64 {
	if size <= 0 {
		size = maxInParams
	}
	if len(s) == 0 {
		return nil
	}
	out := make([][]int64, 0, (len(s)+size-1)/size)
	for len(s) > size {
		out = append(out, s[:size])
		s = s[size:]
	}
	if len(s) > 0 {
		out = append(out, s)
	}
	return out
}

// paginate 给查询追加 limit/offset；limit<=0 表示不限制（保留旧行为）。
func paginate(q *gorm.DB, limit, offset int) *gorm.DB {
	if limit <= 0 {
		return q
	}
	if offset < 0 {
		offset = 0
	}
	return q.Limit(limit).Offset(offset)
}
