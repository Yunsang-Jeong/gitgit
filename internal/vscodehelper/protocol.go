package vscodehelper

import "encoding/json"

const (
	ProtocolVersion = 1
	Transport       = "ndjson-jsonrpc-2.0-stdio"
	LineBase        = 1
	ReadOnly        = true
	Network         = false
	MaxOutputBytes  = 8 * 1024 * 1024
	ServerName      = "gitgit-vscode-helper"
	ServerVersion   = "0.1.0"
)

var Methods = []string{
	"initialize",
	"repository.discover",
	"refs.list",
	"blame.lines",
	"history.file",
	"search.run",
	"revision.content",
	"diff.file",
}

var Notifications = []string{
	"$/cancelRequest",
	"$/progress",
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int       `json:"code"`
	Message string    `json:"message"`
	Data    ErrorData `json:"data"`
}

type ErrorData struct {
	Code string `json:"code"`
}

type InitializeParams struct {
	ProtocolVersion json.RawMessage `json:"protocolVersion"`
}

type InitializeResult struct {
	ProtocolVersion int            `json:"protocolVersion"`
	Transport       string         `json:"transport"`
	LineBase        int            `json:"lineBase"`
	ReadOnly        bool           `json:"readOnly"`
	Network         bool           `json:"network"`
	MaxOutputBytes  int            `json:"maxOutputBytes"`
	Methods         []string       `json:"methods"`
	Notifications   []string       `json:"notifications"`
	Server          ServerInfo     `json:"server"`
	Capabilities    Capabilities   `json:"capabilities"`
	Diagnostics     GitDiagnostics `json:"diagnostics"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Capabilities struct {
	ReadOnly       bool     `json:"readOnly"`
	Network        bool     `json:"network"`
	LineBase       int      `json:"lineBase"`
	MaxOutputBytes int      `json:"maxOutputBytes"`
	Methods        []string `json:"methods"`
	Notifications  []string `json:"notifications"`
}

type GitDiagnostics struct {
	Available        bool   `json:"gitAvailable"`
	Version          string `json:"gitVersion,omitempty"`
	Executable       string `json:"gitExecutable,omitempty"`
	ExecutableSource string `json:"gitExecutableSource,omitempty"`
	Error            string `json:"error,omitempty"`
}

type RepositoryDiscoverParams struct {
	Path string `json:"path"`
}

type RepositoryDiscoverResult struct {
	Root       string   `json:"root"`
	CommonDir  string   `json:"commonDir"`
	GitDir     string   `json:"gitDir"`
	Branch     string   `json:"branch"`
	Head       string   `json:"head"`
	WebRemotes []string `json:"webRemotes,omitempty"`
}

type RepositoryParams struct {
	RepositoryRoot string `json:"repositoryRoot"`
	Repository     string `json:"repository,omitempty"`
}

func (p RepositoryParams) root() string {
	if p.RepositoryRoot != "" {
		return p.RepositoryRoot
	}
	return p.Repository
}

type FileParams struct {
	RepositoryParams
	RelativePath string `json:"relativePath"`
	File         string `json:"file,omitempty"`
}

func (p FileParams) path() string {
	if p.RelativePath != "" {
		return p.RelativePath
	}
	return p.File
}

type RefsListParams struct {
	RepositoryParams
}

type Ref struct {
	Ref        string `json:"ref"`
	FullName   string `json:"fullName"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Commit     string `json:"commit"`
	Current    bool   `json:"current"`
	SymbolicTo string `json:"symbolicTo,omitempty"`
}

type RefsListResult struct {
	Refs []Ref `json:"refs"`
}

type BlameLinesParams struct {
	FileParams
	StartLine        int     `json:"startLine"`
	EndLine          int     `json:"endLine"`
	Revision         string  `json:"revision,omitempty"`
	Contents         *string `json:"contents,omitempty"`
	IgnoreWhitespace bool    `json:"ignoreWhitespace,omitempty"`
}

type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type BlameLine struct {
	Line         int    `json:"line"`
	OriginalLine int    `json:"originalLine"`
	Commit       string `json:"commit"`
	Author       Author `json:"author"`
	Date         string `json:"date"`
	Message      string `json:"message"`
	Content      string `json:"content"`
}

type BlameLinesResult struct {
	Lines          []BlameLine                    `json:"lines"`
	CommitMetadata map[string]BlameCommitMetadata `json:"commitMetadata,omitempty"`
}

type BlameCommitMetadata struct {
	Message     string `json:"message"`
	ParentCount int    `json:"parentCount"`
}

type HistoryFileParams struct {
	FileParams
	Revision string `json:"revision,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type FileChange struct {
	Status  string `json:"status"`
	OldPath string `json:"oldPath,omitempty"`
	Path    string `json:"path"`
}

type HistoryCommit struct {
	Commit      string       `json:"commit"`
	ShortCommit string       `json:"shortCommit"`
	Message     string       `json:"message"`
	Date        string       `json:"date"`
	Author      Author       `json:"author"`
	Parents     []string     `json:"parents"`
	Files       []FileChange `json:"files"`
}

type HistoryFileResult struct {
	Commits []HistoryCommit `json:"commits"`
}

type RevisionContentParams struct {
	FileParams
	Revision string `json:"revision"`
}

type RevisionContentResult struct {
	Content string `json:"content"`
}

type SearchPattern struct {
	Source           string `json:"source"`
	Value            string `json:"value"`
	Join             string `json:"join,omitempty"`
	OpenGroups       int    `json:"openGroups,omitempty"`
	OpenGroupsSnake  int    `json:"open_groups,omitempty"`
	CloseGroups      int    `json:"closeGroups,omitempty"`
	CloseGroupsSnake int    `json:"close_groups,omitempty"`
}

func (p SearchPattern) openGroups() int {
	if p.OpenGroups != 0 {
		return p.OpenGroups
	}
	return p.OpenGroupsSnake
}

func (p SearchPattern) closeGroups() int {
	if p.CloseGroups != 0 {
		return p.CloseGroups
	}
	return p.CloseGroupsSnake
}

type SearchRunParams struct {
	RepositoryRoot      string          `json:"repositoryRoot"`
	RepositoryRootSnake string          `json:"repository_root,omitempty"`
	Repository          string          `json:"repository,omitempty"`
	Patterns            []SearchPattern `json:"patterns"`
	Engine              string          `json:"engine,omitempty"`
	Scope               string          `json:"scope,omitempty"`
	AllRefs             bool            `json:"allRefs,omitempty"`
	AllRefsSnake        bool            `json:"all_refs,omitempty"`
	Author              string          `json:"author,omitempty"`
	Since               string          `json:"since,omitempty"`
	Until               string          `json:"until,omitempty"`
	FollowRename        bool            `json:"followRename,omitempty"`
	FollowRenameSnake   bool            `json:"follow_rename,omitempty"`
	Limit               int             `json:"limit,omitempty"`
	Context             int             `json:"context,omitempty"`
}

func (p SearchRunParams) root() string {
	if p.RepositoryRoot != "" {
		return p.RepositoryRoot
	}
	if p.RepositoryRootSnake != "" {
		return p.RepositoryRootSnake
	}
	return p.Repository
}

func (p SearchRunParams) allRefs() bool {
	return p.AllRefs || p.AllRefsSnake
}

func (p SearchRunParams) followRename() bool {
	return p.FollowRename || p.FollowRenameSnake
}

type SearchResult struct {
	Author       Author              `json:"author"`
	Commit       string              `json:"commit"`
	ShortCommit  string              `json:"shortCommit"`
	Message      string              `json:"message"`
	Date         string              `json:"date"`
	Refs         []string            `json:"refs,omitempty"`
	MatchedFiles []SearchMatchedFile `json:"matchedFiles"`
	ChangedFiles []FileChange        `json:"changedFiles"`
	MatchSources []string            `json:"matchSources"`
}

type SearchMatchedFile struct {
	Status       string   `json:"status"`
	OldPath      string   `json:"oldPath,omitempty"`
	Path         string   `json:"path"`
	MatchSources []string `json:"matchSources"`
}

type SearchRunResult struct {
	Scope   string         `json:"scope"`
	AllRefs bool           `json:"allRefs"`
	Scanned int            `json:"scanned"`
	Count   int            `json:"count"`
	HasMore bool           `json:"hasMore"`
	Results []SearchResult `json:"results"`
}

type ProgressNotification struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  string         `json:"method"`
	Params  ProgressParams `json:"params"`
}

type ProgressParams struct {
	RequestID json.RawMessage `json:"requestId"`
	Scanned   int             `json:"scanned"`
	Total     int             `json:"total"`
	Done      bool            `json:"done,omitempty"`
	Cancelled bool            `json:"cancelled,omitempty"`
}

type DiffFileParams struct {
	RepositoryRoot string `json:"repositoryRoot"`
	Repository     string `json:"repository,omitempty"`
	Commit         string `json:"commit"`
	Path           string `json:"path"`
	OldPath        string `json:"oldPath,omitempty"`
	OldPathSnake   string `json:"old_path,omitempty"`
	Context        int    `json:"context,omitempty"`
}

func (p DiffFileParams) root() string {
	if p.RepositoryRoot != "" {
		return p.RepositoryRoot
	}
	return p.Repository
}

func (p DiffFileParams) oldPath() string {
	if p.OldPath != "" {
		return p.OldPath
	}
	return p.OldPathSnake
}

type TargetRange struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
}

type DeletionAnchor struct {
	AnchorLine int `json:"anchorLine"`
}

type DiffFileResult struct {
	Parent          string           `json:"parent"`
	Diff            string           `json:"diff"`
	TargetRanges    []TargetRange    `json:"targetRanges"`
	DeletionAnchors []DeletionAnchor `json:"deletionAnchors"`
}
