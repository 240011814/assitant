package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// DiscoveredSkill 从 GitHub 仓库扫描到的 Skill 定义 (仅供前端预填新增表单)
type DiscoveredSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Context     string `json:"context"`
	Agent       string `json:"agent"`
	Model       string `json:"model"`
	Content     string `json:"content"`
	Path        string `json:"path"`
	RepoURL     string `json:"repo_url"`
}

// SkillDiscoverResult 扫描结果 (含缓存标记)
type SkillDiscoverResult struct {
	Repo      string            `json:"repo"`
	Total     int               `json:"total"`
	Cached    bool              `json:"cached"`
	Warning   string            `json:"warning,omitempty"`
	ScannedAt time.Time         `json:"scanned_at"`
	Skills    []DiscoveredSkill `json:"skills"`
}

const (
	githubAPIBase = "https://api.github.com"
	githubRawBase = "https://raw.githubusercontent.com"
	// scanHTTPTimeout 单个 GitHub 请求(仓库信息/文件树/单个文件)的超时
	scanHTTPTimeout = 300 * time.Second
	// scanWorkers 并发拉取 SKILL.md 的协程数
	scanWorkers = 12
)

// 扫描结果内存缓存: key = owner/repo@ref:subpath, 仅在该仓库再次扫描成功后才替换
var (
	skillDiscoverCacheMu sync.RWMutex
	skillDiscoverCache   = make(map[string]*SkillDiscoverResult)
)

// DiscoverFromGitHub 扫描 GitHub 仓库中的 SKILL.md 并解析为 Skill 定义
// repo 支持 owner/repo 或完整 URL(可带 /tree/<branch>/<subpath>)
// 扫描成功时更新该仓库的内存缓存; 失败时回退到已有缓存(若有)
func (s *AISkillService) DiscoverFromGitHub(ctx context.Context, repo, ref, subPath string) (*SkillDiscoverResult, error) {
	owner, name, parsedRef, parsedPath, err := parseGitHubRepo(repo)
	if err != nil {
		return nil, err
	}
	if ref == "" {
		ref = parsedRef
	}
	if subPath == "" {
		subPath = parsedPath
	}
	subPath = strings.Trim(strings.TrimSpace(subPath), "/")

	client := newHTTPClient(scanHTTPTimeout)
	if ref == "" {
		ref, err = githubDefaultBranch(ctx, client, owner, name)
		if err != nil {
			return nil, err
		}
	}

	cacheKey := fmt.Sprintf("%s/%s@%s:%s", owner, name, ref, subPath)
	result, scanErr := scanGitHubSkills(ctx, client, owner, name, ref, subPath)
	if scanErr != nil {
		if cached := getSkillDiscoverCache(cacheKey); cached != nil {
			fallback := *cached
			fallback.Cached = true
			fallback.Warning = "扫描失败, 显示上次缓存结果: " + scanErr.Error()
			return &fallback, nil
		}
		return nil, scanErr
	}

	result.Repo = fmt.Sprintf("%s/%s", owner, name)
	result.ScannedAt = time.Now()
	setSkillDiscoverCache(cacheKey, result)
	return result, nil
}

// GetCachedDiscovery 读取某仓库上次扫描成功的内存缓存 (不触发网络请求)
func (s *AISkillService) GetCachedDiscovery(repo, ref, subPath string) *SkillDiscoverResult {
	owner, name, parsedRef, parsedPath, err := parseGitHubRepo(repo)
	if err != nil {
		return nil
	}
	if ref == "" {
		ref = parsedRef
	}
	if subPath == "" {
		subPath = parsedPath
	}
	subPath = strings.Trim(strings.TrimSpace(subPath), "/")
	return findSkillDiscoverCache(owner, name, ref, subPath)
}

func scanGitHubSkills(ctx context.Context, client *http.Client, owner, name, ref, subPath string) (*SkillDiscoverResult, error) {
	entries, truncated, err := githubTree(ctx, client, owner, name, ref)
	if err != nil {
		return nil, err
	}

	prefix := ""
	if subPath != "" {
		prefix = subPath + "/"
	}

	paths := make([]string, 0)
	for _, entry := range entries {
		if entry.Type != "blob" {
			continue
		}
		if !strings.EqualFold(path.Base(entry.Path), "SKILL.md") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(entry.Path, prefix) {
			continue
		}
		paths = append(paths, entry.Path)
	}

	skills := fetchSkillsConcurrently(ctx, client, owner, name, ref, paths)

	result := &SkillDiscoverResult{
		Total:  len(skills),
		Skills: skills,
	}
	if truncated {
		result.Warning = "仓库文件树过大被 GitHub 截断, 结果可能不完整"
	}
	return result, nil
}

// fetchSkillsConcurrently 并发拉取并解析 SKILL.md, 保持原始顺序
func fetchSkillsConcurrently(ctx context.Context, client *http.Client, owner, name, ref string, paths []string) []DiscoveredSkill {
	if len(paths) == 0 {
		return []DiscoveredSkill{}
	}

	workers := scanWorkers
	if len(paths) < workers {
		workers = len(paths)
	}

	slots := make([]*DiscoveredSkill, len(paths))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, p := range paths {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, filePath string) {
			defer wg.Done()
			defer func() { <-sem }()

			raw, err := githubRawFile(ctx, client, owner, name, ref, filePath)
			if err != nil {
				return
			}
			skill := parseDiscoveredSkill(raw, filePath)
			skill.RepoURL = fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", owner, name, ref, filePath)
			slots[idx] = &skill
		}(i, p)
	}
	wg.Wait()

	result := make([]DiscoveredSkill, 0, len(paths))
	for _, s := range slots {
		if s != nil {
			result = append(result, *s)
		}
	}
	return result
}

func getSkillDiscoverCache(key string) *SkillDiscoverResult {
	skillDiscoverCacheMu.RLock()
	defer skillDiscoverCacheMu.RUnlock()
	return cloneSkillDiscoverResult(skillDiscoverCache[key])
}

// findSkillDiscoverCache 按仓库查找缓存; ref 为空时匹配该仓库任意分支的最新缓存
func findSkillDiscoverCache(owner, repo, ref, subPath string) *SkillDiscoverResult {
	prefix := owner + "/" + repo + "@"
	suffix := ":" + subPath

	skillDiscoverCacheMu.RLock()
	defer skillDiscoverCacheMu.RUnlock()

	if ref != "" {
		return cloneSkillDiscoverResult(skillDiscoverCache[prefix+ref+suffix])
	}

	var found *SkillDiscoverResult
	for key, v := range skillDiscoverCache {
		if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, suffix) {
			continue
		}
		if found == nil || v.ScannedAt.After(found.ScannedAt) {
			found = v
		}
	}
	return cloneSkillDiscoverResult(found)
}

func cloneSkillDiscoverResult(v *SkillDiscoverResult) *SkillDiscoverResult {
	if v == nil {
		return nil
	}
	copied := *v
	copied.Skills = append([]DiscoveredSkill(nil), v.Skills...)
	copied.Cached = true
	return &copied
}

func setSkillDiscoverCache(key string, result *SkillDiscoverResult) {
	skillDiscoverCacheMu.Lock()
	defer skillDiscoverCacheMu.Unlock()
	skillDiscoverCache[key] = result
}

// parseGitHubRepo 解析仓库地址, 提取 owner/repo/branch/subpath
func parseGitHubRepo(input string) (owner, repo, ref, subPath string, err error) {
	input = strings.TrimSpace(input)
	input = strings.TrimSuffix(input, "/")
	input = strings.TrimSuffix(input, ".git")
	if input == "" {
		return "", "", "", "", fmt.Errorf("仓库地址不能为空")
	}

	var segments []string
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		u, parseErr := url.Parse(input)
		if parseErr != nil {
			return "", "", "", "", fmt.Errorf("仓库地址不合法: %w", parseErr)
		}
		if !strings.Contains(u.Host, "github.com") {
			return "", "", "", "", fmt.Errorf("仅支持 github.com 仓库")
		}
		segments = strings.Split(strings.Trim(u.Path, "/"), "/")
	} else {
		segments = strings.Split(strings.Trim(input, "/"), "/")
	}

	if len(segments) < 2 {
		return "", "", "", "", fmt.Errorf("仓库地址格式应为 owner/repo")
	}
	owner, repo = segments[0], segments[1]

	if len(segments) >= 4 && (segments[2] == "tree" || segments[2] == "blob") {
		ref = segments[3]
		subPath = strings.Join(segments[4:], "/")
	}
	return owner, repo, ref, subPath, nil
}

func githubRequest(ctx context.Context, client *http.Client, urlStr string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cuttingToolsUi")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("访问 GitHub 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func githubDefaultBranch(ctx context.Context, client *http.Client, owner, repo string) (string, error) {
	body, status, err := githubRequest(ctx, client, fmt.Sprintf("%s/repos/%s/%s", githubAPIBase, owner, repo))
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", fmt.Errorf("仓库不存在或不可访问: %s/%s", owner, repo)
	}
	if status == http.StatusForbidden || status == http.StatusTooManyRequests {
		return "", fmt.Errorf("GitHub API 限流(可配置 GITHUB_TOKEN 提升额度)")
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("获取仓库信息失败, HTTP %d", status)
	}
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", fmt.Errorf("解析仓库信息失败: %w", err)
	}
	if info.DefaultBranch == "" {
		return "main", nil
	}
	return info.DefaultBranch, nil
}

type ghTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

func githubTree(ctx context.Context, client *http.Client, owner, repo, ref string) ([]ghTreeEntry, bool, error) {
	urlStr := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", githubAPIBase, owner, repo, url.PathEscape(ref))
	body, status, err := githubRequest(ctx, client, urlStr)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, fmt.Errorf("分支/标签不存在: %s", ref)
	}
	if status == http.StatusForbidden || status == http.StatusTooManyRequests {
		return nil, false, fmt.Errorf("GitHub API 限流(可配置 GITHUB_TOKEN 提升额度)")
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("获取仓库文件树失败, HTTP %d", status)
	}

	var tree struct {
		Truncated bool          `json:"truncated"`
		Tree      []ghTreeEntry `json:"tree"`
	}
	if err := json.Unmarshal(body, &tree); err != nil {
		return nil, false, fmt.Errorf("解析文件树失败: %w", err)
	}
	return tree.Tree, tree.Truncated, nil
}

func githubRawFile(ctx context.Context, client *http.Client, owner, repo, ref, filePath string) (string, error) {
	escaped := strings.Join(escapePathSegments(filePath), "/")
	urlStr := fmt.Sprintf("%s/%s/%s/%s/%s", githubRawBase, owner, repo, url.PathEscape(ref), escaped)
	body, status, err := githubRequest(ctx, client, urlStr)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("读取文件失败 %s, HTTP %d", filePath, status)
	}
	return string(body), nil
}

func escapePathSegments(p string) []string {
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return segments
}

// parseDiscoveredSkill 解析 SKILL.md 的 frontmatter 与正文
func parseDiscoveredSkill(raw, filePath string) DiscoveredSkill {
	skill := DiscoveredSkill{
		Context: "inline",
		Path:    filePath,
		Name:    path.Base(path.Dir(filePath)),
	}

	frontmatter, content := splitFrontmatter(raw)
	if frontmatter != "" {
		var fm struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
			Context     string `yaml:"context"`
			Agent       string `yaml:"agent"`
			Model       string `yaml:"model"`
		}
		if err := yaml.Unmarshal([]byte(frontmatter), &fm); err == nil {
			if fm.Name != "" {
				skill.Name = fm.Name
			}
			skill.Description = fm.Description
			if fm.Context != "" {
				skill.Context = normalizeSkillContext(fm.Context)
			}
			skill.Agent = fm.Agent
			skill.Model = fm.Model
		}
	}
	skill.Content = content
	return skill
}

// splitFrontmatter 分离 YAML frontmatter 与 markdown 正文
func splitFrontmatter(data string) (frontmatter, content string) {
	data = strings.TrimSpace(data)
	if !strings.HasPrefix(data, "---") {
		return "", data
	}
	rest := data[3:]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return "", data
	}
	frontmatter = strings.TrimSpace(rest[:idx])
	content = strings.TrimSpace(rest[idx+len("\n---"):])
	return frontmatter, content
}
