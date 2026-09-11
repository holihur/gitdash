package store

// 用户数据导出（GDPR Art. 20 data portability）：
// 汇总该用户在实例上的个人数据与自产内容，供本人下载。
// 使用独立的 Export* 结构，避免与 API DTO 的 json:"-" 序列化约定冲突。

// UserExport 用户个人数据导出载荷。
type UserExport struct {
	GeneratedAt string          `json:"generated_at"`
	Profile     ExportProfile   `json:"profile"`
	SSHKeys     []ExportKey     `json:"ssh_keys"`
	GPGKeys     []ExportGPGKey  `json:"gpg_keys"`
	PATs        []ExportPAT     `json:"pats"`
	Repos       []ExportRepo    `json:"repos"`
	Issues      []ExportIssue   `json:"issues"`
	Comments    []ExportComment `json:"comments"`
	Pulls       []ExportPull    `json:"pulls"`
	Reviews     []ExportReview  `json:"reviews"`
	Releases    []ExportRelease `json:"releases"`
	Stars       []RepoRef       `json:"stars"`
	Watches     []RepoRef       `json:"watches"`
}

// RepoRef star/watch 引用。
type RepoRef struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// ExportProfile 账号资料。
type ExportProfile struct {
	Username      string `json:"username"`
	Email         string `json:"email"`
	NotifyEmail   bool   `json:"notify_email"`
	EmailVerified bool   `json:"email_verified"`
	MFAEnabled    bool   `json:"mfa_enabled"`
	CreatedAt     string `json:"created_at"`
}

// ExportKey SSH 公钥（公钥非机密）。
type ExportKey struct {
	Name        string `json:"name"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at"`
}

// ExportGPGKey GPG 公钥元数据。
type ExportGPGKey struct {
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at"`
}

// ExportPAT PAT 元数据（不含 token，hash 不可逆无法导出）。
type ExportPAT struct {
	Name       string `json:"name"`
	Scopes     string `json:"scopes"`
	CIDRs      string `json:"cidrs"`
	ExpiresAt  string `json:"expires_at"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
}

// ExportRepo 名下仓库元数据。
type ExportRepo struct {
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	IsTemplate  bool   `json:"is_template"`
	CreatedAt   string `json:"created_at"`
}

// ExportIssue / ExportComment / ExportPull / ExportReview / ExportRelease 自产内容。
type ExportIssue struct {
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Number    int64  `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
}

type ExportComment struct {
	Owner     string  `json:"owner"`
	Repo      string  `json:"repo"`
	Kind      string  `json:"kind"`
	Number    int64   `json:"number"`
	Body      string  `json:"body"`
	FilePath  *string `json:"file_path"`
	Line      *int64  `json:"line"`
	LineSide  string  `json:"line_side"`
	CreatedAt string  `json:"created_at"`
}

type ExportPull struct {
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Number       int64  `json:"number"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	State        string `json:"state"`
	CreatedAt    string `json:"created_at"`
}

type ExportReview struct {
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Number    int64  `json:"number"`
	State     string `json:"state"`
	Body      string `json:"body"`
	CommitSHA string `json:"commit_sha"`
	CreatedAt string `json:"created_at"`
}

type ExportRelease struct {
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	TagName   string `json:"tag_name"`
	Name      string `json:"name"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// ExportUserData 汇总 username 的个人数据。用户不存在返回 ErrNotFound。
func (s *Store) ExportUserData(username string) (*UserExport, error) {
	var u userRow
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, notFoundErr(err)
	}

	out := &UserExport{
		GeneratedAt: now(),
		Profile: ExportProfile{
			Username: u.Username, Email: u.Email,
			NotifyEmail: u.NotifyEmail, EmailVerified: u.EmailVerified,
			MFAEnabled: u.MFAEnabled, CreatedAt: u.CreatedAt,
		},
	}

	// 各部分独立查询：单项失败不阻塞整体导出
	var ssh []sshKeyRow
	_ = s.db.Where("user_id = ?", u.ID).Find(&ssh).Error
	for _, k := range ssh {
		out.SSHKeys = append(out.SSHKeys, ExportKey{Name: k.Name, PublicKey: k.PublicKey, Fingerprint: k.Fingerprint, CreatedAt: k.CreatedAt})
	}

	var gpg []gpgKeyRow
	_ = s.db.Where("user_id = ?", u.ID).Find(&gpg).Error
	for _, k := range gpg {
		out.GPGKeys = append(out.GPGKeys, ExportGPGKey{Fingerprint: k.Fingerprint, CreatedAt: k.CreatedAt})
	}

	var pats []patRow
	_ = s.db.Where("user_id = ?", u.ID).Find(&pats).Error
	for _, p := range pats {
		out.PATs = append(out.PATs, ExportPAT{Name: p.Name, Scopes: p.Scopes, CIDRs: p.CIDRs, ExpiresAt: p.ExpiresAt, CreatedAt: p.CreatedAt, LastUsedAt: p.LastUsedAt})
	}

	var repos []repoRow
	_ = s.db.Where("owner = ?", username).Find(&repos).Error
	for _, r := range repos {
		out.Repos = append(out.Repos, ExportRepo{Owner: r.Owner, Name: r.Name, Description: r.Description, Private: r.Private, IsTemplate: r.IsTemplate, CreatedAt: r.CreatedAt})
	}

	var issues []issueRow
	_ = s.db.Where("author = ?", username).Find(&issues).Error
	for _, i := range issues {
		out.Issues = append(out.Issues, ExportIssue{Owner: i.Owner, Repo: i.Repo, Number: i.Number, Title: i.Title, Body: i.Body, State: i.State, CreatedAt: i.CreatedAt})
	}

	var comments []commentRow
	_ = s.db.Where("author = ?", username).Find(&comments).Error
	for _, c := range comments {
		out.Comments = append(out.Comments, ExportComment{Owner: c.Owner, Repo: c.Repo, Kind: c.Kind, Number: c.Number, Body: c.Body, FilePath: c.FilePath, Line: c.Line, LineSide: c.LineSide, CreatedAt: c.CreatedAt})
	}

	var pulls []pullRequestRow
	_ = s.db.Where("author = ?", username).Find(&pulls).Error
	for _, p := range pulls {
		out.Pulls = append(out.Pulls, ExportPull{Owner: p.Owner, Repo: p.Repo, Number: p.Number, Title: p.Title, Body: p.Body, SourceBranch: p.SourceBranch, TargetBranch: p.TargetBranch, State: p.State, CreatedAt: p.CreatedAt})
	}

	var reviews []pullReviewRow
	_ = s.db.Where("reviewer = ?", username).Find(&reviews).Error
	for _, r := range reviews {
		out.Reviews = append(out.Reviews, ExportReview{Owner: r.Owner, Repo: r.Repo, Number: r.Number, State: r.State, Body: r.Body, CommitSHA: r.CommitSHA, CreatedAt: r.CreatedAt})
	}

	var releases []releaseRow
	_ = s.db.Where("author = ?", username).Find(&releases).Error
	for _, r := range releases {
		out.Releases = append(out.Releases, ExportRelease{Owner: r.Owner, Repo: r.Repo, TagName: r.TagName, Name: r.Name, Body: r.Body, CreatedAt: r.CreatedAt})
	}

	var stars []starRow
	_ = s.db.Where("username = ?", username).Find(&stars).Error
	for _, s2 := range stars {
		out.Stars = append(out.Stars, RepoRef{Owner: s2.Owner, Name: s2.Repo})
	}

	var watches []watchRow
	_ = s.db.Where("username = ?", username).Find(&watches).Error
	for _, w := range watches {
		out.Watches = append(out.Watches, RepoRef{Owner: w.Owner, Name: w.Repo})
	}

	return out, nil
}
