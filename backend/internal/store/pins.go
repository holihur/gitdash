package store

import "errors"

// MaxPinnedRepos 用户主页最多置顶的仓库数量（与 GitHub 一致）。
const MaxPinnedRepos = 6

// ErrPinLimit 置顶数量已达上限。
var ErrPinLimit = errors.New("pinned repository limit reached")

// RepoPin 用户置顶的仓库引用（Position 决定展示顺序）。
type RepoPin struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Position int    `json:"position"`
}

// ListRepoPins 返回某用户置顶的仓库（按 Position 升序）；用户不存在返回 ErrNotFound。
func (s *Store) ListRepoPins(username string) ([]RepoPin, error) {
	uid, err := s.UserID(username)
	if err != nil {
		return nil, err
	}
	var rows []repoPinRow
	if err := s.db.Where("user_id = ?", uid).Order("position ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]RepoPin, 0, len(rows))
	for _, r := range rows {
		out = append(out, RepoPin{Owner: r.Owner, Repo: r.Repo, Position: r.Position})
	}
	return out, nil
}

// PinRepo 置顶一个仓库。重复置顶幂等；超过上限返回 ErrPinLimit。
// 调用方负责校验仓库确实归属该用户（"我的仓库"）。
func (s *Store) PinRepo(username, owner, repo string) ([]RepoPin, error) {
	uid, err := s.UserID(username)
	if err != nil {
		return nil, err
	}
	var existing repoPinRow
	err = s.db.Where("user_id = ? AND owner = ? AND repo = ?", uid, owner, repo).First(&existing).Error
	if err == nil {
		return s.ListRepoPins(username) // 已置顶：幂等
	}
	if !errors.Is(notFoundErr(err), ErrNotFound) {
		return nil, err
	}
	count, err := s.countLiveRepoPins(uid)
	if err != nil {
		return nil, err
	}
	if count >= MaxPinnedRepos {
		return nil, ErrPinLimit
	}
	var maxPos int
	if err := s.db.Raw("SELECT COALESCE(MAX(position), -1) FROM repo_pins WHERE user_id = ?", uid).
		Scan(&maxPos).Error; err != nil {
		return nil, err
	}
	row := repoPinRow{UserID: uid, Owner: owner, Repo: repo, Position: maxPos + 1, CreatedAt: now()}
	if err := s.db.Create(&row).Error; err != nil {
		if isUniqueErr(err) { // 并发重复置顶：视为幂等
			return s.ListRepoPins(username)
		}
		return nil, err
	}
	return s.ListRepoPins(username)
}

// UnpinRepo 取消置顶；不存在时幂等返回当前列表。
func (s *Store) UnpinRepo(username, owner, repo string) ([]RepoPin, error) {
	uid, err := s.UserID(username)
	if err != nil {
		return nil, err
	}
	if err := s.db.Where("user_id = ? AND owner = ? AND repo = ?", uid, owner, repo).
		Delete(&repoPinRow{}).Error; err != nil {
		return nil, err
	}
	return s.ListRepoPins(username)
}

// CountRepoPins 返回用户已置顶且仍然存在的仓库数量（与展示口径一致，
// 避免已删除仓库的陈旧 pin 占用 6 个配额）。
func (s *Store) CountRepoPins(username string) (int, error) {
	uid, err := s.UserID(username)
	if err != nil {
		return 0, err
	}
	return s.countLiveRepoPins(uid)
}

func (s *Store) countLiveRepoPins(uid int64) (int, error) {
	var n int64
	if err := s.db.Model(&repoPinRow{}).
		Joins("JOIN repos ON repos.owner = repo_pins.owner AND repos.name = repo_pins.repo AND repos.banned = ?", false).
		Where("repo_pins.user_id = ?", uid).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}
