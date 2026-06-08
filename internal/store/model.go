package store

// Status 任务状态枚举（DESIGN.md §3.1 status CHECK）。
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"
)

// StatusCounts 汇总同一查询范围内各状态的数量。
type StatusCounts struct {
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Cancelled  int `json:"cancelled"`
}

// Priority 优先级枚举。
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

// Difficulty 难度枚举。
type Difficulty string

const (
	DifficultyTrivial Difficulty = "trivial"
	DifficultyEasy    Difficulty = "easy"
	DifficultyMedium  Difficulty = "medium"
	DifficultyHard    Difficulty = "hard"
)

// SortOrder 列表排序枚举（DESIGN.md §4.4）。
type SortOrder string

const (
	SortByCreated  SortOrder = "created"
	SortByPriority SortOrder = "priority"
	SortByDue      SortOrder = "due"
)

// Todo 主实体。NotesCount 仅 List/Diff 返回时填充。
type Todo struct {
	ID         string
	Scope      string
	Content    string
	Status     Status
	Priority   Priority
	Difficulty Difficulty
	DueAt      *int64  // unix ms；nil = 无截止
	ParentID   *string // nil = 顶层
	Version    int64
	CreatedAt  int64
	UpdatedAt  int64
	NotesCount int
}

// Note 留痕笔记。
type Note struct {
	ID        int64
	TodoID    string
	Content   string
	CreatedAt int64
}

// ChangeOp 表示 diff 中的操作类型。
type ChangeOp string

const (
	ChangeUpsert ChangeOp = "upsert"
	ChangeDelete ChangeOp = "delete"
)

// Change 是 diff 返回的每一项。
// Upsert 时 Todo 完整；Delete 时仅 ID/Scope/Version/UpdatedAt 有值。
type Change struct {
	Op   ChangeOp
	Todo Todo
}
