package git

type ChangeType string

const (
	ChangeAdded    ChangeType = "A"
	ChangeModified ChangeType = "M"
	ChangeDeleted  ChangeType = "D"
	ChangeRenamed  ChangeType = "R"
	ChangeCopied   ChangeType = "C"
	ChangeTypeChan ChangeType = "T"
	ChangeUnmerged ChangeType = "U"
	ChangeUnknown  ChangeType = "?"
)

func (c ChangeType) String() string {
	switch c {
	case ChangeAdded:
		return "added"
	case ChangeModified:
		return "modified"
	case ChangeDeleted:
		return "deleted"
	case ChangeRenamed:
		return "renamed"
	case ChangeCopied:
		return "copied"
	case ChangeTypeChan:
		return "type-changed"
	case ChangeUnmerged:
		return "unmerged"
	case ChangeUnknown:
		return "unknown"
	default:
		return string(c)
	}
}

type FileChange struct {
	Path       string
	OldPath    string
	ChangeType ChangeType
	Stage      string
	Score      int
}

type Repository struct {
	Path      string
	WorkTree  string
	GitDir    string
	IsBare    bool
	IsShallow bool
}

type StatusOptions struct {
	StagedOnly     bool
	UnstagedOnly   bool
	IncludeRenames bool
	Porcelain      bool
}
