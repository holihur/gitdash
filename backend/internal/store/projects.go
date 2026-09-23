package store

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// ---- projects ----

// CreateProject 创建看板项目，并初始化默认列（To Do / In Progress / Done）与默认泳道（Default）。
func (s *Store) CreateProject(owner, repo, name, description string) (Project, error) {
	r := projectRow{Owner: owner, Repo: repo, Name: name, Description: description, CreatedAt: now()}
	if err := s.db.Create(&r).Error; err != nil {
		return Project{}, err
	}
	p := Project{ID: r.ID, Owner: r.Owner, Repo: r.Repo, Name: r.Name, Description: r.Description, CreatedAt: r.CreatedAt}
	for i, col := range []string{"To Do", "In Progress", "Done"} {
		if err := s.db.Create(&projectColumnRow{ProjectID: r.ID, Name: col, Position: i}).Error; err != nil {
			return Project{}, err
		}
	}
	if err := s.db.Create(&projectSwimlaneRow{ProjectID: r.ID, Name: "Default", Position: 0}).Error; err != nil {
		return Project{}, err
	}
	return p, nil
}

func (s *Store) ListProjects(owner, repo string) ([]Project, error) {
	var rows []struct {
		ID          int64
		Owner       string
		Repo        string
		Name        string
		Description string
		CreatedAt   string
		CardCount   int
	}
	err := s.db.Raw(`SELECT p.id, p.owner, p.repo, p.name, p.description, p.created_at,
		(SELECT COUNT(*) FROM project_cards c WHERE c.project_id = p.id) AS card_count
		FROM projects p WHERE p.owner = ? AND p.repo = ? ORDER BY p.id`, owner, repo).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []Project{}
	for _, r := range rows {
		out = append(out, Project{ID: r.ID, Owner: r.Owner, Repo: r.Repo, Name: r.Name,
			Description: r.Description, CardCount: r.CardCount, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func (s *Store) GetProject(owner, repo string, id int64) (Project, error) {
	var r projectRow
	if err := s.db.Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Project{}, ErrNotFound
		}
		return Project{}, err
	}
	return Project{ID: r.ID, Owner: r.Owner, Repo: r.Repo, Name: r.Name, Description: r.Description, CreatedAt: r.CreatedAt}, nil
}

func (s *Store) UpdateProject(owner, repo string, id int64, name, description string) (Project, error) {
	q := s.db.Model(&projectRow{}).Where("id = ? AND owner = ? AND repo = ?", id, owner, repo)
	if name != "" {
		q = q.Update("name", name)
	}
	if description != "" {
		q = q.Update("description", description)
	}
	if q.Error != nil {
		return Project{}, q.Error
	}
	return s.GetProject(owner, repo, id)
}

// DeleteProject 级联删除列、泳道与卡片。
func (s *Store) DeleteProject(owner, repo string, id int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var cnt int64
		if err := tx.Model(&projectRow{}).Where("id = ? AND owner = ? AND repo = ?", id, owner, repo).
			Count(&cnt).Error; err != nil {
			return err
		}
		if cnt == 0 {
			return ErrNotFound
		}
		for _, m := range []any{&projectCardRow{}, &projectColumnRow{}, &projectSwimlaneRow{}} {
			if err := tx.Where("project_id = ?", id).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ?", id).Delete(&projectRow{}).Error
	})
}

// ---- columns ----

func (s *Store) ListProjectColumns(projectID int64) ([]ProjectColumn, error) {
	var rows []projectColumnRow
	if err := s.db.Where("project_id = ?", projectID).Order("position, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []ProjectColumn{}
	for _, r := range rows {
		out = append(out, ProjectColumn(r))
	}
	return out, nil
}

func (s *Store) CreateProjectColumn(owner, repo string, projectID int64, name string) (ProjectColumn, error) {
	if _, err := s.GetProject(owner, repo, projectID); err != nil {
		return ProjectColumn{}, err
	}
	var max int
	_ = s.db.Model(&projectColumnRow{}).Where("project_id = ?", projectID).
		Select("COALESCE(MAX(position), -1)").Scan(&max).Error
	r := projectColumnRow{ProjectID: projectID, Name: name, Position: max + 1}
	if err := s.db.Create(&r).Error; err != nil {
		return ProjectColumn{}, err
	}
	return ProjectColumn(r), nil
}

// UpdateProjectColumn 重命名 / 调整顺序。
func (s *Store) UpdateProjectColumn(projectID, id int64, name string, position *int) error {
	updates := map[string]any{}
	if name != "" {
		updates["name"] = name
	}
	if position != nil {
		updates["position"] = *position
	}
	if len(updates) == 0 {
		return nil
	}
	res := s.db.Model(&projectColumnRow{}).Where("id = ? AND project_id = ?", id, projectID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteProjectColumn 删除看板列。moveTo > 0 时先把该列卡片迁移到 target 列；
// 否则删列并删除其卡片。target 列必须属于同一项目。
func (s *Store) DeleteProjectColumn(projectID, id, moveTo int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if moveTo > 0 && moveTo != id {
			var cnt int64
			if err := tx.Model(&projectColumnRow{}).Where("id = ? AND project_id = ?", moveTo, projectID).
				Count(&cnt).Error; err != nil {
				return err
			}
			if cnt == 0 {
				return ErrNotFound
			}
			if err := tx.Model(&projectCardRow{}).Where("column_id = ? AND project_id = ?", id, projectID).
				Update("column_id", moveTo).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Where("column_id = ? AND project_id = ?", id, projectID).
				Delete(&projectCardRow{}).Error; err != nil {
				return err
			}
		}
		res := tx.Where("id = ? AND project_id = ?", id, projectID).Delete(&projectColumnRow{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ---- swimlanes ----

func (s *Store) ListProjectSwimlanes(projectID int64) ([]ProjectSwimlane, error) {
	var rows []projectSwimlaneRow
	if err := s.db.Where("project_id = ?", projectID).Order("position, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []ProjectSwimlane{}
	for _, r := range rows {
		out = append(out, ProjectSwimlane(r))
	}
	return out, nil
}

func (s *Store) CreateProjectSwimlane(owner, repo string, projectID int64, name string) (ProjectSwimlane, error) {
	if _, err := s.GetProject(owner, repo, projectID); err != nil {
		return ProjectSwimlane{}, err
	}
	var max int
	_ = s.db.Model(&projectSwimlaneRow{}).Where("project_id = ?", projectID).
		Select("COALESCE(MAX(position), -1)").Scan(&max).Error
	r := projectSwimlaneRow{ProjectID: projectID, Name: name, Position: max + 1}
	if err := s.db.Create(&r).Error; err != nil {
		return ProjectSwimlane{}, err
	}
	return ProjectSwimlane(r), nil
}

func (s *Store) UpdateProjectSwimlane(projectID, id int64, name string, position *int) error {
	updates := map[string]any{}
	if name != "" {
		updates["name"] = name
	}
	if position != nil {
		updates["position"] = *position
	}
	if len(updates) == 0 {
		return nil
	}
	res := s.db.Model(&projectSwimlaneRow{}).Where("id = ? AND project_id = ?", id, projectID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteProjectSwimlane 删除泳道，其卡片退回未分组（swimlane_id = 0）。
func (s *Store) DeleteProjectSwimlane(projectID, id int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&projectCardRow{}).
			Where("project_id = ? AND swimlane_id = ?", projectID, id).
			Update("swimlane_id", 0).Error; err != nil {
			return err
		}
		res := tx.Where("id = ? AND project_id = ?", id, projectID).Delete(&projectSwimlaneRow{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ---- cards ----

// ListProjectCards 返回项目全部卡片，issue 卡片附带标题与状态（LEFT JOIN issues）。
func (s *Store) ListProjectCards(projectID int64) ([]ProjectCard, error) {
	var rows []struct {
		ID         int64
		ProjectID  int64
		ColumnID   int64
		SwimlaneID int64
		IssueNum   int64
		Title      string
		Body       string
		Note       string
		StartDate  string
		DueDate    string
		Position   int
		CreatedAt  string
		IssueTitle string
		IssueState string
	}
	err := s.db.Raw(`SELECT c.id, c.project_id, c.column_id, c.swimlane_id, c.issue_num, c.title, c.body, c.note, c.position, c.created_at,
		c.start_date, c.due_date,
		COALESCE(i.title, '') AS issue_title, COALESCE(i.state, '') AS issue_state
		FROM project_cards c LEFT JOIN issues i ON i.owner = (SELECT owner FROM projects WHERE id = c.project_id)
			AND i.repo = (SELECT repo FROM projects WHERE id = c.project_id) AND i.number = c.issue_num
		WHERE c.project_id = ? ORDER BY c.position, c.id`, projectID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []ProjectCard{}
	for _, r := range rows {
		out = append(out, ProjectCard{ID: r.ID, ProjectID: r.ProjectID, ColumnID: r.ColumnID,
			SwimlaneID: r.SwimlaneID, IssueNumber: r.IssueNum, IssueTitle: r.IssueTitle,
			IssueState: r.IssueState, Title: r.Title, Body: r.Body, Note: r.Note,
			StartDate: r.StartDate, DueDate: r.DueDate,
			Position: r.Position, CreatedAt: r.CreatedAt})
	}
	// 卡片负责人与标签（复用仓库标签）。
	if len(out) > 0 {
		ids := make([]int64, 0, len(out))
		for _, c := range out {
			ids = append(ids, c.ID)
		}
		assignees := map[int64][]string{}
		var arows []struct {
			CardID   int64
			Username string
		}
		_ = s.db.Table("project_card_assignees").Select("card_id, username").
			Where("card_id IN ?", ids).Order("username").Scan(&arows).Error
		for _, r := range arows {
			assignees[r.CardID] = append(assignees[r.CardID], r.Username)
		}
		labels := map[int64][]Label{}
		var lrows []struct {
			CardID int64
			ID     int64
			Name   string
			Color  string
		}
		_ = s.db.Table("project_card_labels l").
			Select("l.card_id AS card_id, rl.id AS id, rl.name AS name, rl.color AS color").
			Joins("JOIN repo_labels rl ON rl.id = l.label_id").
			Where("l.card_id IN ?", ids).Order("rl.name").Scan(&lrows).Error
		for _, r := range lrows {
			labels[r.CardID] = append(labels[r.CardID], Label{ID: r.ID, Name: r.Name, Color: r.Color})
		}
		for i := range out {
			out[i].Assignees = assignees[out[i].ID]
			if out[i].Assignees == nil {
				out[i].Assignees = []string{}
			}
			out[i].Labels = labels[out[i].ID]
			if out[i].Labels == nil {
				out[i].Labels = []Label{}
			}
		}
	}
	return out, nil
}

// CreateProjectCard 创建卡片；issueNum > 0 时校验 issue 存在，column/swimlane 必须属于本项目。
// title 为卡片名称，body 为 Markdown 详情；startDate / dueDate 可为空（YYYY-MM-DD）。
func (s *Store) CreateProjectCard(owner, repo string, projectID, columnID, swimlaneID, issueNum int64, title, body, startDate, dueDate string) (ProjectCard, error) {
	if _, err := s.GetProject(owner, repo, projectID); err != nil {
		return ProjectCard{}, err
	}
	var cnt int64
	if err := s.db.Model(&projectColumnRow{}).Where("id = ? AND project_id = ?", columnID, projectID).
		Count(&cnt).Error; err != nil {
		return ProjectCard{}, err
	}
	if cnt == 0 {
		return ProjectCard{}, ErrNotFound
	}
	if swimlaneID != 0 {
		if err := s.db.Model(&projectSwimlaneRow{}).Where("id = ? AND project_id = ?", swimlaneID, projectID).
			Count(&cnt).Error; err != nil {
			return ProjectCard{}, err
		}
		if cnt == 0 {
			return ProjectCard{}, ErrNotFound
		}
	}
	if issueNum > 0 {
		if err := s.db.Model(&issueRow{}).Where("owner = ? AND repo = ? AND number = ?", owner, repo, issueNum).
			Count(&cnt).Error; err != nil {
			return ProjectCard{}, err
		}
		if cnt == 0 {
			return ProjectCard{}, ErrNotFound
		}
	}
	var max int
	_ = s.db.Model(&projectCardRow{}).Where("project_id = ? AND column_id = ? AND swimlane_id = ?", projectID, columnID, swimlaneID).
		Select("COALESCE(MAX(position), -1)").Scan(&max).Error
	r := projectCardRow{ProjectID: projectID, ColumnID: columnID, SwimlaneID: swimlaneID,
		IssueNum: issueNum, Title: title, Note: title, Body: body, StartDate: startDate, DueDate: dueDate, Position: max + 1, CreatedAt: now()}
	if err := s.db.Create(&r).Error; err != nil {
		return ProjectCard{}, err
	}
	c := ProjectCard{ID: r.ID, ProjectID: r.ProjectID, ColumnID: r.ColumnID, SwimlaneID: r.SwimlaneID,
		IssueNumber: r.IssueNum, Title: r.Title, Body: r.Body, Note: r.Note, StartDate: r.StartDate, DueDate: r.DueDate,
		Position: r.Position, CreatedAt: r.CreatedAt}
	if issueNum > 0 {
		var ir issueRow
		if err := s.db.Where("owner = ? AND repo = ? AND number = ?", owner, repo, issueNum).First(&ir).Error; err == nil {
			c.IssueTitle, c.IssueState = ir.Title, ir.State
		}
	}
	return c, nil
}

// MoveProjectCard 移动卡片（换列 / 换泳道 / 调整顺序），目标列与泳道必须属于本项目。
func (s *Store) MoveProjectCard(projectID, cardID, columnID, swimlaneID int64, position int) error {
	var cnt int64
	if err := s.db.Model(&projectColumnRow{}).Where("id = ? AND project_id = ?", columnID, projectID).
		Count(&cnt).Error; err != nil || cnt == 0 {
		if err != nil {
			return err
		}
		return ErrNotFound
	}
	if swimlaneID != 0 {
		if err := s.db.Model(&projectSwimlaneRow{}).Where("id = ? AND project_id = ?", swimlaneID, projectID).
			Count(&cnt).Error; err != nil {
			return err
		}
		if cnt == 0 {
			return ErrNotFound
		}
	}
	res := s.db.Model(&projectCardRow{}).Where("id = ? AND project_id = ?", cardID, projectID).
		Updates(map[string]any{"column_id": columnID, "swimlane_id": swimlaneID, "position": position})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ProjectCardUpdate 是卡片的部分更新（nil 字段不修改）。
// Title 更新时同步 Note（向后兼容）。
type ProjectCardUpdate struct {
	Title       *string
	Body        *string
	Note        *string
	StartDate   *string
	DueDate     *string
	IssueNumber *int64
}

// UpdateProjectCard 更新卡片文本与日程（仅非 nil 字段生效）。
func (s *Store) UpdateProjectCard(projectID, cardID int64, up ProjectCardUpdate) error {
	updates := map[string]any{}
	if up.Title != nil {
		updates["title"] = *up.Title
		updates["note"] = *up.Title
	} else if up.Note != nil {
		updates["title"] = *up.Note
		updates["note"] = *up.Note
	}
	if up.Body != nil {
		updates["body"] = *up.Body
	}
	if up.StartDate != nil {
		updates["start_date"] = *up.StartDate
	}
	if up.DueDate != nil {
		updates["due_date"] = *up.DueDate
	}
	if up.IssueNumber != nil {
		updates["issue_num"] = *up.IssueNumber
	}
	if len(updates) == 0 {
		return nil
	}
	res := s.db.Model(&projectCardRow{}).Where("id = ? AND project_id = ?", cardID, projectID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteProjectCard(projectID, cardID int64) error {
	res := s.db.Where("id = ? AND project_id = ?", cardID, projectID).Delete(&projectCardRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetProjectCardAssignees 全量替换卡片负责人。
func (s *Store) SetProjectCardAssignees(projectID, cardID int64, usernames []string) error {
	var cnt int64
	if err := s.db.Model(&projectCardRow{}).Where("id = ? AND project_id = ?", cardID, projectID).
		Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		return ErrNotFound
	}
	seen := map[string]bool{}
	rows := make([]projectCardAssigneeRow, 0, len(usernames))
	for _, u := range usernames {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		rows = append(rows, projectCardAssigneeRow{CardID: cardID, Username: u})
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("card_id = ?", cardID).Delete(&projectCardAssigneeRow{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

// SetProjectCardLabels 全量替换卡片标签。
func (s *Store) SetProjectCardLabels(projectID, cardID int64, labelIDs []int64) error {
	var cnt int64
	if err := s.db.Model(&projectCardRow{}).Where("id = ? AND project_id = ?", cardID, projectID).
		Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		return ErrNotFound
	}
	seen := map[int64]bool{}
	rows := make([]projectCardLabelRow, 0, len(labelIDs))
	for _, id := range labelIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		rows = append(rows, projectCardLabelRow{CardID: cardID, LabelID: id})
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("card_id = ?", cardID).Delete(&projectCardLabelRow{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}
