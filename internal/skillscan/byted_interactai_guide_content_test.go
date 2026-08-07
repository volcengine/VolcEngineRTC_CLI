// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package skillscan_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These tests lock in the byted-interactai-guide Skill architecture invariants from
// change refactor-vertc-skill-architecture: one thin-router SKILL.md that routes
// to a FLAT references/ set, clear per-file domain declarations, and a Web SDK
// diagnosis domain that stays independent of Voice Agent concepts (so it can be
// reused by a future pure Web SDK skill without a rewrite). moduleRoot is shared
// with content_test.go in this package.

const interactAIGuideSkillDir = "skills/byted-interactai-guide"

var expectedReferenceDomains = map[string]string{
	"web-sdk-diagnosis.md":   "web-sdk",
	"voice-agent-runtime.md": "voice-agent",
	"voicechat-api.md":       "voice-agent",
	"integration-flow.md":    "integration",
	"capabilities.md":        "product-capability",
}

var allowedDomains = map[string]bool{
	"web-sdk": true, "voice-agent": true, "integration": true, "auth": true,
	"product-capability": true,
}

func readSkillFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), interactAIGuideSkillDir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

// declaredDomain extracts the "**Domain**: <value>" marker from a reference.
func declaredDomain(body string) (string, bool) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "**Domain**:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "**Domain**:")), true
		}
	}
	return "", false
}

// TestVoiceAgentReferencesAreFlat asserts the expected references exist as
// files directly under references/ (no nested subdirectories), matching the flat
// structure the current scanner/enumerator support.
func TestVoiceAgentReferencesAreFlat(t *testing.T) {
	refDir := filepath.Join(moduleRoot(t), interactAIGuideSkillDir, "references")
	entries, err := os.ReadDir(refDir)
	if err != nil {
		t.Fatalf("read references dir: %v", err)
	}
	found := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("references/ must stay flat, found subdirectory %q", e.Name())
			continue
		}
		found[e.Name()] = true
	}
	for name := range expectedReferenceDomains {
		if !found[name] {
			t.Errorf("missing expected reference %q", name)
		}
	}
}

// TestVoiceAgentReferenceDomainsDeclared asserts every reference declares a domain
// from the allowed domain set, with the expected mapping.
func TestVoiceAgentReferenceDomainsDeclared(t *testing.T) {
	for name, want := range expectedReferenceDomains {
		body := readSkillFile(t, filepath.Join("references", name))
		got, ok := declaredDomain(body)
		if !ok {
			t.Errorf("%s: missing '**Domain**:' declaration", name)
			continue
		}
		if !allowedDomains[got] {
			t.Errorf("%s: domain %q not in the allowed set", name, got)
		}
		if got != want {
			t.Errorf("%s: domain = %q, want %q", name, got, want)
		}
	}
}

// TestWebSdkDiagnosisHasNoVoiceAgentConcepts asserts the Web SDK domain stays
// reusable by a pure Web SDK scenario — it must not depend on Voice Agent
// concepts (spec agent-scene-skills: "Web SDK 诊断域可复用").
func TestWebSdkDiagnosisHasNoVoiceAgentConcepts(t *testing.T) {
	body := readSkillFile(t, filepath.Join("references", "web-sdk-diagnosis.md"))
	// Substring bans: "VoiceChat" already covers Start/Update/StopVoiceChat.
	for _, tok := range []string{"VoiceChat", "Task ID", "TaskId"} {
		if strings.Contains(body, tok) {
			t.Errorf("web-sdk-diagnosis.md must not reference Voice Agent concept %q", tok)
		}
	}
	// Acronyms are matched on word boundaries so an ordinary English word or URL
	// path that happens to contain the substring (e.g. "vad") is not a false hit.
	acronyms := regexp.MustCompile(`\b(ASR|VAD|LLM|TTS)\b`)
	if m := acronyms.FindString(body); m != "" {
		t.Errorf("web-sdk-diagnosis.md must not reference Voice Agent concept %q", m)
	}
}

// TestSkillRoutesToAllReferences asserts the thin-router SKILL.md links to every
// reference (route table), so no domain is orphaned.
func TestSkillRoutesToAllReferences(t *testing.T) {
	skill := readSkillFile(t, "SKILL.md")
	for name := range expectedReferenceDomains {
		link := "references/" + name
		if !strings.Contains(skill, link) {
			t.Errorf("SKILL.md does not route to %q", link)
		}
	}
}

func TestInteractAICapabilityFreshnessPolicy(t *testing.T) {
	skill := readSkillFile(t, "SKILL.md")
	capabilities := readSkillFile(t, filepath.Join("references", "capabilities.md"))

	for _, marker := range []string{
		"当前 Demo/CLI 未覆盖",
		"尚未核验",
		"确定性“不支持”",
		"references/capabilities.md",
	} {
		if !strings.Contains(skill, marker) {
			t.Errorf("SKILL.md missing capability answer boundary %q", marker)
		}
	}
	for _, marker := range []string{
		"**verified_at**: 2026-08-03",
		"何时必须读取当前官方文档",
		"精确 API 字段",
		"计费、Token 折算、配额",
		"Demo 没实现",
		"无法确认时",
	} {
		if !strings.Contains(capabilities, marker) {
			t.Errorf("capabilities.md missing freshness or evidence rule %q", marker)
		}
	}
}

func TestCapabilityMapCoversEveryDeveloperGuideLeaf(t *testing.T) {
	body := readSkillFile(t, filepath.Join("references", "capabilities.md"))
	for _, marker := range []string{
		"75 个树节点、62 篇正文",
		"开发指南 27 篇叶子文档",
		"以上 A–F 共 27 个叶子专题",
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("capabilities.md missing source coverage marker %q", marker)
		}
	}

	// Document IDs are stable source identities. This list mirrors every leaf
	// under the official AI audio/video interaction developer-guide branch as
	// audited on the reference's verified_at date.
	for _, documentID := range []string{
		"1581712", "1581714", "1581713", "1798100", "1399966", "2137637",
		"1902994", "1337284", "1511926", "1899860", "1511927", "2389913",
		"1544164", "1806620", "2122016", "2139328", "1350596", "2129096",
		"2386107", "1449206", "1408245", "1554654", "1856160", "1856161",
		"1557771", "1415216", "1798101",
	} {
		link := "volcengine.com/docs/6348/" + documentID
		if !strings.Contains(body, link) {
			t.Errorf("capabilities.md does not cover developer-guide document %s", documentID)
		}
	}
}

func TestCapabilityMapRetainsRepresentativeSemantics(t *testing.T) {
	body := readSkillFile(t, filepath.Join("references", "capabilities.md"))
	for _, marker := range []string{
		"实时视频由 RTC 上传",
		"完整连续视频",
		"短期记忆",
		"预注册或通话中自学习",
		"Function Calling",
		"MCP Server",
		"1804905",
		"1804906",
		"1804908",
		"1804907",
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("capabilities.md missing representative capability semantic %q", marker)
		}
	}
}

func TestCapabilityRoutingDoesNotPrivilegeVisualUnderstanding(t *testing.T) {
	skill := readSkillFile(t, "SKILL.md")
	for _, forbidden := range []string{
		"视频/图片理解与扩展能力",
		"视觉能力：[",
		"docs/6348/1408245",
	} {
		if strings.Contains(skill, forbidden) {
			t.Errorf("SKILL.md must route capabilities symmetrically, found visual-only entry %q", forbidden)
		}
	}
}

func TestVoiceChatReferencesUseCurrentAPIContract(t *testing.T) {
	api := readSkillFile(t, filepath.Join("references", "voicechat-api.md"))
	for _, marker := range []string{
		"2123348",
		"2123350",
		"2123349",
		"1928198",
		"由调用方在 `StartVoiceChat` 请求中提供",
		"`Result=ok`",
		"HTTP 200 仅表示任务下发成功",
	} {
		if !strings.Contains(api, marker) {
			t.Errorf("voicechat-api.md missing current API contract %q", marker)
		}
	}

	for _, name := range []string{"voicechat-api.md", "voice-agent-runtime.md", "integration-flow.md"} {
		body := readSkillFile(t, filepath.Join("references", name))
		for _, obsolete := range []string{"返回有效 Task ID", "成功并返回 Task ID", "| 得到 Task ID |", "失败（无 Task ID）"} {
			if strings.Contains(body, obsolete) {
				t.Errorf("%s retains obsolete StartVoiceChat contract %q", name, obsolete)
			}
		}
	}

	web := readSkillFile(t, filepath.Join("references", "web-sdk-diagnosis.md"))
	if !strings.Contains(web, "docs/6348/106914") || strings.Contains(web, "docs/6348/1544162") {
		t.Error("web-sdk-diagnosis.md must use a Web SDK authority page, not the AI release notes")
	}
}

func TestVoiceChatRuntimeEvidenceMatchesExecutionLayers(t *testing.T) {
	runtime := readSkillFile(t, filepath.Join("references", "voice-agent-runtime.md"))
	for _, marker := range []string{
		"直接 OpenAPI",
		"CLI/配套 Server",
		"成功 envelope",
		"`task_id` 是调用方 `TaskId` 的回显",
		"未配置或未收到回调",
		"不能单独判失败",
		"first_failure=任务初始化",
	} {
		if !strings.Contains(runtime, marker) {
			t.Errorf("voice-agent-runtime.md missing layered or optional evidence contract %q", marker)
		}
	}

	integration := readSkillFile(t, filepath.Join("references", "integration-flow.md"))
	for _, marker := range []string{
		"阶段 5–7 的失败来源归 `voice-agent`",
		"阶段 8 的媒体绑定归 `integration`",
		"未配置则记录「未观测」并继续",
		"适配层成功且无 typed error",
		"下发成功后收到异步初始化错误",
	} {
		if !strings.Contains(integration, marker) {
			t.Errorf("integration-flow.md missing layered evidence or domain contract %q", marker)
		}
	}

	for _, obsolete := range []string{
		"收到任务启动证据且 Agent 已进房",
		"跨域衔接阶段（5→6",
	} {
		if strings.Contains(runtime, obsolete) || strings.Contains(integration, obsolete) {
			t.Errorf("VoiceChat runtime references retain obsolete mandatory evidence/domain wording %q", obsolete)
		}
	}
}

func TestVoiceAgentSkillUsesAgentSafeCredentialSetup(t *testing.T) {
	skill := readSkillFile(t, "SKILL.md")
	for _, marker := range []string{"vertc.dev.selection_required", "error.details", "--app-id", "--bot-id", "禁止向用户索取", "AppKey", "console.volcengine.com/conversational-ai/agentManage"} {
		if !strings.Contains(skill, marker) {
			t.Errorf("SKILL.md missing Agent-safe setup marker %q", marker)
		}
	}
	if strings.Contains(skill, "--app-key") {
		t.Error("SKILL.md must never recommend passing AppKey in command arguments")
	}
}

func TestAgentFacingDocsNeverRecommendAppKeyArgument(t *testing.T) {
	root := moduleRoot(t)
	for _, rel := range []string{
		"README.md",
		"README.en.md",
		"docs/automation.md",
		"docs/troubleshooting.md",
		"docs/voice-agent.md",
		"skills/byted-interactai-guide/SKILL.md",
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if strings.Contains(string(data), "--app-key") {
			t.Errorf("%s recommends passing AppKey in command arguments", rel)
		}
	}
}

func TestSkillLifecycleNoticePolicy(t *testing.T) {
	skill := readSkillFile(t, "SKILL.md")
	for _, marker := range []string{
		"每次读取 `vertc --format json`",
		"成功或失败输出",
		"不能只检查",
		"不得静默",
		"完成并验证当前用户任务后",
		"必须向用户简短提示对应命令",
		"两者同时出现就都提示",
		"不要擅自执行",
		"_notice.update",
		"vertc update",
		"_notice.skills",
		"vertc skills sync",
		"冷缓存首次调用可能没有 notice",
		"不代表已是最新版",
		"不要为了等待 notice",
		"继续检查本任务后续每条 `vertc` JSON 输出",
		"VERTC_NO_UPDATE_NOTIFIER",
		"VERTC_NO_SKILLS_NOTIFIER",
	} {
		if !strings.Contains(skill, marker) {
			t.Errorf("SKILL.md lifecycle policy missing %q", marker)
		}
	}
}

func TestSkillGracefullyHandlesMissingCLI(t *testing.T) {
	skill := readSkillFile(t, "SKILL.md")
	for _, marker := range []string{
		"咨询模式",
		"执行模式",
		"vertc version --format json",
		"仍可继续文档咨询",
		"不要把 CLI 缺失解释成 RTC",
		"npm install -g @volcengine/rtc-cli",
		"未经用户同意，不要主动安装 CLI",
	} {
		if !strings.Contains(skill, marker) {
			t.Errorf("SKILL.md missing optional-CLI behavior %q", marker)
		}
	}
}

func TestSkillTemplateIncludesOptionalCLIContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), "skill-template", "skill-template.md"))
	if err != nil {
		t.Fatalf("read skill template: %v", err)
	}
	template := string(data)
	for _, marker := range []string{
		"咨询模式",
		"执行模式",
		"vertc version --format json",
		"不依赖 CLI",
		"不要把 CLI 缺失解释为 SDK",
		"未经用户同意，不主动安装 CLI",
	} {
		if !strings.Contains(template, marker) {
			t.Errorf("skill template missing optional-CLI contract %q", marker)
		}
	}
}

func TestSkillUsesProcessScopedInvocationIdentity(t *testing.T) {
	skill := readSkillFile(t, "SKILL.md")
	want := "VE_SKILL_ID=<name>/<version> vertc"
	if !strings.Contains(skill, want) {
		t.Fatalf("SKILL.md missing process-scoped identity %q", want)
	}
	for _, marker := range []string{"frontmatter", "`name`", "`version`", "不得", "export", "持久化", "不允许有 `@`", "不要在面向用户展示"} {
		if !strings.Contains(skill, marker) {
			t.Errorf("SKILL.md missing invocation identity rule %q", marker)
		}
	}
	if regexp.MustCompile(`VE_SKILL_ID=byted-[a-z0-9-]+/[0-9]`).MatchString(skill) {
		t.Fatal("SKILL.md hard-codes a versioned VE_SKILL_ID that will drift during release stamping")
	}
}

func TestSkillTemplateIncludesInvocationIdentityContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), "skill-template", "skill-template.md"))
	if err != nil {
		t.Fatalf("read skill template: %v", err)
	}
	template := string(data)
	for _, marker := range []string{
		"VE_SKILL_ID=<name>/<version> vertc",
		"frontmatter",
		"不得 export",
		"持久化",
		"不允许有 `@`",
		"不得在面向用户展示",
	} {
		if !strings.Contains(template, marker) {
			t.Errorf("skill template missing invocation identity contract %q", marker)
		}
	}
}

// TestIntegrationFlowCoversAcceptanceScenarios asserts the integration layer
// encodes the staged "AI 没回答" convergence and the acceptance scenarios, each
// mappable to a domain / first-failure stage (spec skill-diagnosis-protocol).
func TestIntegrationFlowCoversAcceptanceScenarios(t *testing.T) {
	body := readSkillFile(t, filepath.Join("references", "integration-flow.md"))
	markers := []string{
		"StartVoiceChat",     // staged step 3
		"Agent 进房",           // staged step 4
		"ASR/VAD",            // staged step 6
		"纯 Web SDK 远端音频无法播放", // acceptance scenario
		"Token 无效导致进房失败",     // acceptance scenario
		"first_failure",      // unified protocol field
	}
	for _, m := range markers {
		if !strings.Contains(body, m) {
			t.Errorf("integration-flow.md missing expected marker %q", m)
		}
	}
}
