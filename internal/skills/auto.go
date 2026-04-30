package skills

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"miniclaw2/internal/config"
	"miniclaw2/internal/memory"
)

const (
	skillMetadataFileName     = "skill.json"
	autoSkillTierApproved     = "approved"
	autoSkillTierCandidate    = "candidate"
	candidateSkillDirName     = "_candidates"
	archivedSkillDirName      = "_archived"
	defaultAutoSkillExamples  = 5
	maxExamplePromptChars     = 240
	maxExampleResponseChars   = 360
	minimumExistingSkillMatch = 4
	autoSkillCandidateScore   = 4
	autoSkillApprovedScore    = 7
)

type SkillMetadata struct {
	Slug            string         `json:"slug,omitempty"`
	Auto            bool           `json:"auto,omitempty"`
	Tier            string         `json:"tier,omitempty"`
	Score           int            `json:"score,omitempty"`
	QualityScore    int            `json:"quality_score,omitempty"`
	QualityReasons  []string       `json:"quality_reasons,omitempty"`
	QualityWarnings []string       `json:"quality_warnings,omitempty"`
	CaptureCount    int            `json:"capture_count,omitempty"`
	SelectedCount   int            `json:"selected_count,omitempty"`
	SuccessCount    int            `json:"success_count,omitempty"`
	FailureCount    int            `json:"failure_count,omitempty"`
	Keywords        []string       `json:"keywords,omitempty"`
	Tools           []string       `json:"tools,omitempty"`
	Examples        []SkillExample `json:"examples,omitempty"`
	CreatedAt       string         `json:"created_at,omitempty"`
	UpdatedAt       string         `json:"updated_at,omitempty"`
}

type SkillExample struct {
	Prompt    string   `json:"prompt,omitempty"`
	Response  string   `json:"response,omitempty"`
	ToolNames []string `json:"tool_names,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
}

type sessionLine struct {
	Kind     string `json:"kind"`
	Role     string `json:"role,omitempty"`
	Content  string `json:"content,omitempty"`
	ToolName string `json:"tool_name,omitempty"`
	IsError  bool   `json:"is_error,omitempty"`
}

type sessionExperience struct {
	Prompt              string
	Response            string
	ToolNames           []string
	ToolResults         []toolResult
	SuccessfulToolCount int
	FailedToolCount     int
	Keywords            []string
}

type toolResult struct {
	Name    string
	Content string
	IsError bool
}

type autoSkillQualityReport struct {
	ShouldCreate bool
	Tier         string
	Score        int
	Reasons      []string
	Warnings     []string
}

var (
	requestedCountPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:top|first|latest)\s+(\d+)\s*(?:items?|results?|messages?|tweets?|posts?|records?|entries?|questions?|answers?|replies?)\b`),
		regexp.MustCompile(`(?i)\b(\d+)\s*(?:items?|results?|messages?|tweets?|posts?|records?|entries?|questions?|answers?|replies?)\b`),
		regexp.MustCompile(`(\d+)\s*(?:条|个|篇|项|道|题)\s*(?:消息|推文|帖子|结果|记录|内容|问题|题目|题|回答|回复)?`),
	}
	limitedResultCountPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?:只|仅|目前只|当前只|当前页面只|只能|仅能)\s*(?:显示|获取|返回|找到|抓取|加载|看到|提供)?\s*[^\d]{0,12}(\d+)\s*(?:条|个|篇|项)?`),
		regexp.MustCompile(`(?i)(?:only|just|currently|could only|limited to)\s*(?:show|display|find|fetch|get|return|load)?(?:ed|s)?\s*[^\d]{0,12}(\d+)\s*(?:items?|results?|messages?|tweets?|posts?|records?|entries?)?`),
	}
	answerRequestPatterns = []*regexp.Regexp{
		regexp.MustCompile(`答\s*\d*\s*(?:道|个)?\s*(?:题|问题|题目)`),
		regexp.MustCompile(`(?:回答|作答|答复|回复)\s*\d*\s*(?:道|个)?\s*(?:题|问题|题目)`),
		regexp.MustCompile(`(?i)\b(?:answer|reply to|respond to)\s+\d*\s*(?:questions?|replies?)\b`),
	}
	answerSubmissionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?:提交回答|提交答案|发布回答|发布答案|发表回答|发送回复|提交回复|发布回复)`),
		regexp.MustCompile(`(?i)\b(?:submitted answer|posted answer|answered successfully|reply sent)\b`),
	}
)

var finalFailureMarkers = []string{
	"couldn't be completed", "unable to complete", "task failed", "i'm sorry", "sorry, but",
	"requires login", "need to log in", "please log in", "publish failed", "submit failed",
	"failed to publish", "unable to submit", "not published", "submission blocked", "publish blocked",
	"anti-automation", "blocked by anti-automation",
	"很抱歉", "无法完成", "未能完成", "无法处理", "未能处理", "无法回答", "未能回答",
	"无法继续", "未能继续", "请先登录", "需要登录", "登录验证", "发布失败", "提交失败",
	"无法发布", "未能发布", "无法提交", "未能提交", "未提交成功", "提交未成功",
	"反自动化", "被阻止", "似乎被阻止",
}

var partialCompletionMarkers = []string{
	"只显示了", "仅显示了", "只获取了", "仅获取了", "只返回了", "仅返回了", "只加载了", "仅加载了",
	"只能获取", "只能返回", "当前页面只显示", "目前只显示", "无法获取更多", "未能获取更多", "数量有限",
	"only showed", "only displayed", "only found", "only returned", "only loaded", "could only", "currently only",
	"unable to fetch more", "could not fetch more", "limited to",
}

var answerActionContextMarkers = []string{
	"autobrowser", "百度知道", "知乎", "zhidao.baidu", "zhihu.com", "http://", "https://",
	"页面", "网站", "网页", "发布", "提交", "post", "publish", "submit", "answer box", "reply box",
}

var validationMarkers = []string{
	"test", "tests", "testing", "passed", "0 failed", "ok ", "lint", "validate", "validated",
	"verify", "verified", "check", "checked", "diff", "status", "测试", "验证", "校验", "通过",
}

var skillStoreMu sync.Mutex

func UpdateSelectedSkillScores(cfg config.Config, selected []Skill, success bool) error {
	if !cfg.EnableSkillScoring || len(selected) == 0 {
		return nil
	}
	skillStoreMu.Lock()
	defer skillStoreMu.Unlock()
	for _, skill := range selected {
		meta := skill.Metadata
		if latest, err := readSkillMetadata(sidecarPath(skill.Path)); err == nil {
			meta = latest
		}
		meta.SelectedCount++
		if success {
			meta.SuccessCount++
		} else {
			meta.FailureCount++
		}
		meta.Keywords = mergeOrdered(meta.Keywords, tokenize(skill.Name+" "+skill.Description))
		meta.Tools = mergeOrdered(meta.Tools, skill.Metadata.Tools)
		meta.Score = calculateSkillScore(meta)
		meta.UpdatedAt = time.Now().Format(time.RFC3339)
		if meta.CreatedAt == "" {
			meta.CreatedAt = meta.UpdatedAt
		}
		if err := writeSkillMetadata(sidecarPath(skill.Path), meta); err != nil {
			return err
		}
	}
	return nil
}

func AutoCaptureSession(cfg config.Config, sessionPath, prompt, response string) error {
	if !cfg.EnableAutoSkills {
		return nil
	}
	experience, err := readSessionExperience(sessionPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(prompt) != "" {
		experience.Prompt = strings.TrimSpace(prompt)
	}
	if strings.TrimSpace(response) != "" {
		experience.Response = strings.TrimSpace(response)
	}
	experience.Keywords = tokenize(experience.Prompt)
	if strings.TrimSpace(experience.Prompt) == "" || strings.TrimSpace(experience.Response) == "" {
		return nil
	}
	if experience.SuccessfulToolCount < cfg.AutoSkillMinToolCalls {
		return nil
	}
	quality := evaluateAutoSkillQuality(cfg, experience)
	if !quality.ShouldCreate {
		return nil
	}
	return upsertAutoSkill(cfg, experience, quality)
}

func hasTooManyFailedTools(experience sessionExperience) bool {
	return experience.FailedToolCount > 0 && experience.FailedToolCount > experience.SuccessfulToolCount/2
}

func evaluateAutoSkillQuality(cfg config.Config, experience sessionExperience) autoSkillQualityReport {
	report := autoSkillQualityReport{}

	if hasTooManyFailedTools(experience) {
		report.Warnings = append(report.Warnings, "too-many-failed-tools")
		return report
	}
	if looksLikeFinalFailure(experience.Response) {
		report.Warnings = append(report.Warnings, "final-result-not-successful")
		return report
	}
	if hasPartialCompletion(experience.Prompt, experience.Response) {
		report.Warnings = append(report.Warnings, "partial-completion-detected")
		return report
	}
	if requestedAnswerActionMissing(experience) {
		report.Warnings = append(report.Warnings, "requested-action-not-observed")
		return report
	}

	report.Score += 2
	report.Reasons = append(report.Reasons, "final-result-successful")
	if experience.SuccessfulToolCount >= cfg.AutoSkillMinToolCalls {
		report.Score += 2
		report.Reasons = append(report.Reasons, "multi-step-workflow")
	}
	if experience.SuccessfulToolCount >= cfg.AutoSkillMinToolCalls+2 {
		report.Score++
		report.Reasons = append(report.Reasons, "extra-tool-evidence")
	}
	if len(experience.ToolNames) >= 2 {
		report.Score++
		report.Reasons = append(report.Reasons, "multi-tool-workflow")
	}
	if isExternalAnswerRequest(experience.Prompt) {
		report.Score += 2
		report.Reasons = append(report.Reasons, "requested-action-observed")
	}
	if experience.FailedToolCount == 0 {
		report.Score += 2
		report.Reasons = append(report.Reasons, "clean-run")
	} else {
		report.Warnings = append(report.Warnings, "recovered-from-failure")
	}
	if hasValidationSignal(experience) {
		report.Score++
		report.Reasons = append(report.Reasons, "has-validation-signal")
	}
	if len([]rune(strings.TrimSpace(experience.Response))) >= 24 {
		report.Score++
		report.Reasons = append(report.Reasons, "specific-final-outcome")
	}

	if report.Score < autoSkillCandidateScore {
		report.Warnings = append(report.Warnings, "quality-score-too-low")
		return report
	}
	report.ShouldCreate = true
	report.Tier = autoSkillTierCandidate
	if report.Score >= autoSkillApprovedScore {
		report.Tier = autoSkillTierApproved
	}
	return report
}

func looksLikeFinalFailure(response string) bool {
	return containsAny(strings.ToLower(response), finalFailureMarkers)
}

func hasPartialCompletion(prompt, response string) bool {
	requestedCount := extractRequestedCount(prompt)
	if requestedCount <= 1 {
		return false
	}
	reportedCount := extractLimitedResultCount(response)
	if reportedCount > 0 && reportedCount < requestedCount {
		return true
	}
	return containsAny(strings.ToLower(response), partialCompletionMarkers)
}

func requestedAnswerActionMissing(experience sessionExperience) bool {
	if !isExternalAnswerRequest(experience.Prompt) {
		return false
	}
	requiredCount := extractRequestedCount(experience.Prompt)
	if requiredCount <= 0 {
		requiredCount = 1
	}
	return countObservedAnswerSubmissions(experience.ToolResults) < requiredCount
}

func isExternalAnswerRequest(prompt string) bool {
	if !matchesAnyPattern(prompt, answerRequestPatterns) {
		return false
	}
	return containsAny(strings.ToLower(prompt), answerActionContextMarkers)
}

func countObservedAnswerSubmissions(results []toolResult) int {
	count := 0
	for _, result := range results {
		if result.IsError {
			continue
		}
		content := strings.TrimSpace(result.Content)
		if content == "" || looksLikeFinalFailure(content) {
			continue
		}
		if matchesAnyPattern(content, answerSubmissionPatterns) {
			count++
		}
	}
	return count
}

func hasValidationSignal(experience sessionExperience) bool {
	for _, result := range experience.ToolResults {
		if result.IsError {
			continue
		}
		if containsAny(strings.ToLower(result.Content), validationMarkers) {
			return true
		}
	}
	return containsAny(strings.ToLower(experience.Response), validationMarkers)
}

func extractRequestedCount(text string) int {
	return extractFirstPositiveCount(text, requestedCountPatterns)
}

func extractLimitedResultCount(text string) int {
	return extractFirstPositiveCount(text, limitedResultCountPatterns)
}

func extractFirstPositiveCount(text string, patterns []*regexp.Regexp) int {
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) < 2 {
			continue
		}
		count, err := strconv.Atoi(match[1])
		if err == nil && count > 0 {
			return count
		}
	}
	return 0
}

func matchesAnyPattern(text string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func containsAny(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func readSkillMetadata(path string) (SkillMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillMetadata{}, err
	}
	meta := SkillMetadata{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return SkillMetadata{}, err
	}
	meta.Keywords = compactStrings(meta.Keywords)
	meta.Tools = compactStrings(meta.Tools)
	meta.QualityReasons = compactStrings(meta.QualityReasons)
	meta.QualityWarnings = compactStrings(meta.QualityWarnings)
	return meta, nil
}

func writeSkillMetadata(path string, meta SkillMetadata) error {
	meta.Keywords = compactStrings(meta.Keywords)
	meta.Tools = compactStrings(meta.Tools)
	meta.QualityReasons = compactStrings(meta.QualityReasons)
	meta.QualityWarnings = compactStrings(meta.QualityWarnings)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeAtomicFile(path, append(data, '\n'), 0o644)
}

func writeAtomicFile(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp.")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func readSessionExperience(path string) (sessionExperience, error) {
	file, err := os.Open(path)
	if err != nil {
		return sessionExperience{}, err
	}
	defer file.Close()

	experience := sessionExperience{}
	seenTools := map[string]struct{}{}
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := sessionLine{}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		switch {
		case line.Role == "user" && experience.Prompt == "":
			experience.Prompt = strings.TrimSpace(line.Content)
		case line.Role == "assistant" && strings.TrimSpace(line.Content) != "":
			experience.Response = strings.TrimSpace(line.Content)
		case line.Kind == "tool" && strings.TrimSpace(line.ToolName) != "":
			if strings.EqualFold(strings.TrimSpace(line.Content), "invoked") {
				continue
			}
			experience.ToolResults = append(experience.ToolResults, toolResult{Name: line.ToolName, Content: strings.TrimSpace(line.Content), IsError: line.IsError})
			if line.IsError {
				experience.FailedToolCount++
				continue
			}
			experience.SuccessfulToolCount++
			if _, ok := seenTools[line.ToolName]; ok {
				continue
			}
			seenTools[line.ToolName] = struct{}{}
			experience.ToolNames = append(experience.ToolNames, line.ToolName)
		}
	}
	if err := scanner.Err(); err != nil {
		return sessionExperience{}, err
	}
	return experience, nil
}

func upsertAutoSkill(cfg config.Config, experience sessionExperience, quality autoSkillQualityReport) error {
	skillStoreMu.Lock()
	defer skillStoreMu.Unlock()

	root := filepath.Join(cfg.Workspace, "skills")
	loaded := discoverSkills(true, root)
	target := findBestAutoSkill(loaded, experience)
	now := time.Now().Format(time.RFC3339)
	maxExamples := cfg.AutoSkillMaxExamples
	if maxExamples <= 0 {
		maxExamples = defaultAutoSkillExamples
	}

	meta := target.Metadata
	name := target.Name
	skillPath := target.Path
	if skillPath == "" {
		slug := buildSkillSlug(experience.Keywords)
		if slug == "" {
			slug = "autoskill-general-workflow"
		}
		name = slug
		meta = SkillMetadata{Slug: slug, Auto: true, CreatedAt: now}
	}
	resolvedTier := resolveAutoSkillTier(target, quality.Tier)
	oldSkillDir := ""
	if skillPath != "" {
		oldSkillDir = filepath.Dir(skillPath)
	}
	skillPath = autoSkillPath(root, name, resolvedTier)
	meta.Auto = true
	meta.Tier = resolvedTier
	meta.CaptureCount++
	meta.UpdatedAt = now
	meta.Keywords = normalizeAutoKeywords(mergeOrdered(meta.Keywords, experience.Keywords))
	meta.Tools = mergeOrdered(meta.Tools, experience.ToolNames)
	meta.QualityScore = quality.Score
	meta.QualityReasons = mergeOrdered(meta.QualityReasons, quality.Reasons)
	meta.QualityWarnings = mergeOrdered(meta.QualityWarnings, quality.Warnings)
	meta.Examples = compactExamples(append([]SkillExample{{
		Prompt:    memory.LimitText(strings.TrimSpace(experience.Prompt), maxExamplePromptChars),
		Response:  memory.LimitText(strings.TrimSpace(experience.Response), maxExampleResponseChars),
		ToolNames: append([]string(nil), experience.ToolNames...),
		CreatedAt: now,
	}}, meta.Examples...), maxExamples)
	meta.Score = calculateSkillScore(meta)

	if err := writeAtomicFile(skillPath, []byte(renderAutoSkill(name, meta)), 0o644); err != nil {
		return err
	}
	if err := writeSkillMetadata(sidecarPath(skillPath), meta); err != nil {
		return err
	}
	if oldSkillDir != "" && filepath.Clean(oldSkillDir) != filepath.Clean(filepath.Dir(skillPath)) {
		_ = os.RemoveAll(oldSkillDir)
	}
	return nil
}

func resolveAutoSkillTier(existing Skill, qualityTier string) string {
	if existing.Path != "" && !isCandidateSkillPath(existing.Path) {
		return autoSkillTierApproved
	}
	if existing.Metadata.Tier == autoSkillTierApproved {
		return autoSkillTierApproved
	}
	if qualityTier == autoSkillTierApproved {
		return autoSkillTierApproved
	}
	return autoSkillTierCandidate
}

func autoSkillPath(root, name, tier string) string {
	base := root
	if tier == autoSkillTierCandidate {
		base = filepath.Join(root, candidateSkillDirName)
	}
	return filepath.Join(base, name, "SKILL.md")
}

func isCandidateSkillPath(path string) bool {
	for _, part := range strings.Split(filepath.Clean(path), string(os.PathSeparator)) {
		if part == candidateSkillDirName {
			return true
		}
	}
	return false
}

func findBestAutoSkill(loaded []Skill, experience sessionExperience) Skill {
	expectedSlug := buildSkillSlug(experience.Keywords)
	best := Skill{}
	bestScore := 0
	for _, skill := range loaded {
		if !skill.Metadata.Auto {
			continue
		}
		if expectedSlug != "" && skill.Metadata.Slug == expectedSlug {
			return skill
		}
		score := overlapScore(skill.Metadata.Keywords, experience.Keywords)*2 + overlapScore(skill.Metadata.Tools, experience.ToolNames)*3
		if score > bestScore {
			best = skill
			bestScore = score
		}
	}
	if bestScore < minimumExistingSkillMatch {
		return Skill{}
	}
	return best
}

func buildSkillSlug(tokens []string) string {
	parts := []string{"autoskill"}
	for _, token := range tokens {
		cleaned := strings.ToLower(strings.TrimSpace(token))
		if cleaned == "" {
			continue
		}
		parts = append(parts, cleaned)
		if len(parts) == 5 {
			break
		}
	}
	return strings.Join(parts, "-")
}

func renderAutoSkill(name string, meta SkillMetadata) string {
	title := prettySkillTitle(name)
	lines := []string{
		"# " + title,
		"",
		"Auto-generated from successful MiniClaw runs. Approved skills are auto-loaded; candidate skills stay in _candidates until a cleaner run promotes them.",
		"",
		fmt.Sprintf("Tier: %s", displaySkillTier(meta.Tier)),
		fmt.Sprintf("Quality score: %d", meta.QualityScore),
		fmt.Sprintf("Current score: %d/100", meta.Score),
		"",
		"## When To Use",
		"Use this skill for requests similar to the captured examples and keyword set below.",
		"",
		"## Keywords",
	}
	for _, keyword := range meta.Keywords {
		lines = append(lines, "- "+keyword)
	}
	if len(meta.Keywords) == 0 {
		lines = append(lines, "- (keywords will accumulate as this skill is reused)")
	}
	lines = append(lines, "", "## Recommended Tools")
	for _, toolName := range meta.Tools {
		lines = append(lines, "- "+toolName)
	}
	if len(meta.Tools) == 0 {
		lines = append(lines, "- (no stable tool pattern captured yet)")
	}
	lines = append(lines, "", "## Workflow Pattern")
	if len(meta.Tools) == 0 {
		lines = append(lines, "1. Inspect the workspace before making changes.", "2. Keep the edit scope local.", "3. Validate with the narrowest executable check available.")
	} else {
		for index, toolName := range meta.Tools {
			lines = append(lines, fmt.Sprintf("%d. Consider using %s when the task needs it.", index+1, toolName))
		}
		lines = append(lines, fmt.Sprintf("%d. Finish with a focused validation command before stopping.", len(meta.Tools)+1))
	}
	lines = append(lines, "", "## Recent Captures")
	if len(meta.Examples) == 0 {
		lines = append(lines, "No successful captures recorded yet.")
	} else {
		for index, example := range meta.Examples {
			lines = append(lines,
				fmt.Sprintf("### Capture %d", index+1),
			)
			if len(example.ToolNames) > 0 {
				lines = append(lines, "Tools: "+strings.Join(example.ToolNames, ", "))
			}
			if strings.TrimSpace(example.CreatedAt) != "" {
				lines = append(lines, "Captured: "+strings.TrimSpace(example.CreatedAt))
			}
			lines = append(lines, "")
		}
	}
	lines = append(lines,
		"## Metrics",
		fmt.Sprintf("- tier: %s", displaySkillTier(meta.Tier)),
		fmt.Sprintf("- quality_score: %d", meta.QualityScore),
		fmt.Sprintf("- captures: %d", meta.CaptureCount),
		fmt.Sprintf("- selected: %d", meta.SelectedCount),
		fmt.Sprintf("- success: %d", meta.SuccessCount),
		fmt.Sprintf("- failure: %d", meta.FailureCount),
	)
	if len(meta.QualityReasons) > 0 {
		lines = append(lines, "- quality_reasons: "+strings.Join(meta.QualityReasons, ", "))
	}
	if len(meta.QualityWarnings) > 0 {
		lines = append(lines, "- quality_warnings: "+strings.Join(meta.QualityWarnings, ", "))
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func displaySkillTier(tier string) string {
	if strings.TrimSpace(tier) == "" {
		return autoSkillTierApproved
	}
	return tier
}

func calculateSkillScore(meta SkillMetadata) int {
	score := 20
	if meta.Auto {
		score += 5
	}
	score += minInt(30, meta.CaptureCount*8)
	score += minInt(20, meta.SuccessCount*5)
	score -= minInt(25, meta.FailureCount*8)
	score += minInt(10, len(meta.Tools)*2)
	score += minInt(10, len(meta.Keywords))
	if score < 1 {
		return 1
	}
	if score > 100 {
		return 100
	}
	return score
}

func sidecarPath(skillPath string) string {
	return filepath.Join(filepath.Dir(skillPath), skillMetadataFileName)
}

func compactExamples(examples []SkillExample, maxExamples int) []SkillExample {
	if maxExamples <= 0 {
		maxExamples = defaultAutoSkillExamples
	}
	trimmed := make([]SkillExample, 0, len(examples))
	seen := map[string]struct{}{}
	for _, example := range examples {
		key := strings.TrimSpace(example.Prompt) + "\n" + strings.TrimSpace(example.Response)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		example.ToolNames = compactStrings(example.ToolNames)
		trimmed = append(trimmed, example)
		if len(trimmed) == maxExamples {
			break
		}
	}
	return trimmed
}

func normalizeAutoKeywords(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		tokens := tokenize(value)
		if len(tokens) == 0 {
			trimmed := strings.TrimSpace(strings.ToLower(value))
			if trimmed == "" {
				continue
			}
			tokens = []string{trimmed}
		}
		for _, token := range tokens {
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			normalized = append(normalized, token)
		}
	}
	return normalized
}

func compactStrings(values []string) []string {
	trimmed := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		trimmed = append(trimmed, value)
	}
	return trimmed
}

func mergeOrdered(base, additions []string) []string {
	merged := append([]string(nil), compactStrings(base)...)
	seen := map[string]struct{}{}
	for _, value := range merged {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	return merged
}

func overlapScore(left, right []string) int {
	set := map[string]struct{}{}
	for _, value := range compactStrings(left) {
		set[value] = struct{}{}
	}
	score := 0
	for _, value := range compactStrings(right) {
		if _, ok := set[value]; ok {
			score++
		}
	}
	return score
}

func prettySkillTitle(name string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(name), "autoskill-")
	trimmed = strings.ReplaceAll(trimmed, "-", " ")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return "Auto Skill"
	}
	parts := strings.Fields(trimmed)
	for index, part := range parts {
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func sortedSkillsByScore(loaded []Skill) []Skill {
	ordered := append([]Skill(nil), loaded...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Metadata.Score == ordered[j].Metadata.Score {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].Metadata.Score > ordered[j].Metadata.Score
	})
	return ordered
}
