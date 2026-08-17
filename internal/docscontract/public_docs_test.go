// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package docscontract

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

var (
	localMarkdownLink = regexp.MustCompile(`\[[^]]+\]\((#[^)]+|[^)#]+\.md(?:#[^)]+)?)\)`)
	chineseText       = regexp.MustCompile(`[一-龥]`)
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readDoc(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func bilingualSections(t *testing.T, rel, content string) (string, string) {
	t.Helper()
	const (
		zhAnchor = `<a id="zh-cn"></a>`
		enAnchor = `<a id="english"></a>`
	)
	zhStart := strings.Index(content, zhAnchor)
	enStart := strings.Index(content, enAnchor)
	if zhStart < 0 || enStart < 0 || enStart <= zhStart {
		t.Fatalf("%s must contain ordered zh-cn and english anchors", rel)
	}
	return content[zhStart+len(zhAnchor) : enStart], content[enStart+len(enAnchor):]
}

func markdownHeadingSlug(heading string) string {
	heading = strings.ToLower(strings.TrimSpace(strings.TrimLeft(heading, "#")))
	var slug strings.Builder
	lastDash := false
	for _, r := range heading {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '-':
			slug.WriteRune(r)
			lastDash = r == '-'
		case unicode.IsSpace(r):
			if slug.Len() > 0 && !lastDash {
				slug.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(slug.String(), "-")
}

func hasMarkdownAnchor(content, fragment string) bool {
	if strings.Contains(content, `id="`+fragment+`"`) {
		return true
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			if markdownHeadingSlug(trimmed) == fragment {
				return true
			}
		}
	}
	return false
}

func TestPublicDocumentationUsesCompactBilingualFiles(t *testing.T) {
	root := repoRoot(t)
	readmeZH := readDoc(t, root, "README.md")
	readmeEN := readDoc(t, root, "README.en.md")
	if !strings.Contains(readmeZH, "README.en.md") || !strings.Contains(readmeEN, "README.md") {
		t.Error("READMEs must link to each other")
	}

	type bilingualContract struct {
		titles  [2]string
		markers [2][]string
	}
	bilingual := map[string]bilingualContract{
		"docs/voice-agent.md": {
			titles:  [2]string{"# 语音智能体项目", "# Voice-agent projects"},
			markers: [2][]string{{"## 首次从控制台配置", "## 房间与用户身份"}, {"## First-run configuration from the Console", "## Room and user identity"}},
		},
		"docs/automation.md": {
			titles:  [2]string{"# 自动化与结构化输出", "# Automation and structured output"},
			markers: [2][]string{{"## 自动化环境中的登录", "## 失败处理"}, {"## Authentication in automation", "## Failure routing"}},
		},
		"docs/troubleshooting.md": {
			titles:  [2]string{"# 故障排查", "# Troubleshooting"},
			markers: [2][]string{{"## 登录时没有打开浏览器", "## 仍然无法解决"}, {"## Browser login does not open", "## Still blocked"}},
		},
		"SUPPORT.md": {
			titles:  [2]string{"# 获取帮助", "# Support"},
			markers: [2][]string{{"## Bug 报告", "## 安全问题"}, {"## Bug reports", "## Security reports"}},
		},
		"SECURITY.md": {
			titles:  [2]string{"# 安全策略", "# Security Policy"},
			markers: [2][]string{{"## 报告漏洞", "## 安全边界"}, {"## Reporting a vulnerability", "## Security boundaries"}},
		},
		"CONTRIBUTING.md": {
			titles:  [2]string{"# 为 vertc 贡献代码", "# Contributing to vertc"},
			markers: [2][]string{{"## 常见改动", "## Pull Request 检查清单"}, {"## Contribution recipes", "## Pull request checklist"}},
		},
	}
	for rel, contract := range bilingual {
		content := readDoc(t, root, rel)
		chinese, english := bilingualSections(t, rel, content)
		if !chineseText.MatchString(chinese) || !strings.Contains(chinese, contract.titles[0]) {
			t.Errorf("%s has no complete Simplified Chinese section", rel)
		}
		if !strings.Contains(english, contract.titles[1]) || !regexp.MustCompile(`[A-Za-z]{4}`).MatchString(english) {
			t.Errorf("%s has no complete English section", rel)
		}
		for language, section := range map[string]struct {
			content string
			markers []string
		}{
			"Chinese": {chinese, contract.markers[0]},
			"English": {english, contract.markers[1]},
		} {
			for _, marker := range section.markers {
				if !strings.Contains(section.content, marker) {
					t.Errorf("%s %s section missing substantive marker %q", rel, language, marker)
				}
			}
		}
		if !strings.Contains(chinese, "[English](#english) | 简体中文") || !strings.Contains(english, "[简体中文](#zh-cn) | English") {
			t.Errorf("%s has incomplete language navigation", rel)
		}
	}

	retiredPeers := []string{
		"docs/voice-agent.en.md",
		"docs/automation.en.md",
		"docs/troubleshooting.en.md",
		"SUPPORT.en.md",
		"SECURITY.en.md",
		"CONTRIBUTING.en.md",
	}
	for _, rel := range retiredPeers {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Errorf("retired bilingual peer %s still exists", rel)
		}
	}
}

func TestPublicDocumentationPinsCurrentSetupAndCredentialRules(t *testing.T) {
	root := repoRoot(t)
	readmeZH := readDoc(t, root, "README.md")
	readmeEN := readDoc(t, root, "README.en.md")
	security := readDoc(t, root, "SECURITY.md")
	securityZH, securityEN := bilingualSections(t, "SECURITY.md", security)

	for rel, content := range map[string]string{
		"README.md":    readmeZH,
		"README.en.md": readmeEN,
	} {
		if !strings.Contains(content, "RTC application") && !strings.Contains(content, "RTC 应用") {
			t.Errorf("%s does not describe RTC application selection", rel)
		}
	}
	if !strings.Contains(readmeZH, "即使只有一个") || !strings.Contains(readmeEN, "even when there is only one") {
		t.Error("READMEs must explain that one available agent still requires selection")
	}
	for name, section := range map[string]string{"Chinese": securityZH, "English": securityEN} {
		for _, marker := range []string{"$VERTC_HOME/auth.json", "--store=keyring"} {
			if !strings.Contains(section, marker) {
				t.Errorf("SECURITY.md %s section missing credential-store marker %q", name, marker)
			}
		}
	}
	if !strings.Contains(securityZH, "默认保存在权限受限的用户级文件") || !strings.Contains(securityZH, "只有用户显式选择 `--store=keyring` 后") {
		t.Error("SECURITY.md Chinese section must describe file as default and keyring as explicit opt-in")
	}
	if !strings.Contains(securityEN, "file by default") || !strings.Contains(securityEN, "only after the user explicitly selects `--store=keyring`") {
		t.Error("SECURITY.md English section must describe file as default and keyring as explicit opt-in")
	}
	for _, reversed := range []string{"默认保存在操作系统凭据库", "keyring by default", "tokens belong in the operating-system credential store"} {
		if strings.Contains(security, reversed) {
			t.Errorf("SECURITY.md contains reversed credential-store claim %q", reversed)
		}
	}
}

func TestPublicDocumentationLocalMarkdownLinksResolve(t *testing.T) {
	root := repoRoot(t)
	files := []string{
		"README.md", "README.en.md",
		"docs/voice-agent.md",
		"docs/automation.md",
		"docs/troubleshooting.md",
		"SUPPORT.md",
		"SECURITY.md",
		"CONTRIBUTING.md",
	}
	for _, rel := range files {
		content := readDoc(t, root, rel)
		for _, match := range localMarkdownLink.FindAllStringSubmatch(content, -1) {
			parts := strings.SplitN(match[1], "#", 2)
			targetRel := parts[0]
			targetContent := content
			if targetRel != "" {
				path := filepath.Clean(filepath.Join(root, filepath.Dir(rel), filepath.FromSlash(targetRel)))
				data, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("%s links to missing %s: %v", rel, match[1], err)
					continue
				}
				targetContent = string(data)
			}
			if len(parts) == 2 && !hasMarkdownAnchor(targetContent, parts[1]) {
				t.Errorf("%s links to missing anchor %s", rel, match[1])
			}
		}
		if strings.Contains(content, "five-minute-quick-start") {
			t.Errorf("%s retains the removed quick-start anchor", rel)
		}
	}
}
