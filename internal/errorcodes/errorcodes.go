// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Package errorcodes loads the offline SDK runtime error-code knowledge base
// The Web SDK seed (web.yaml, 88 entries) is embedded via go:embed
// so `explain-error` works with no network. This is the SDK-runtime error
// system, distinct from the CLI's own error.code catalog (internal/errs).
//
// Each entry carries a domain (web-sdk | voice-agent), a source and a verified
// trust status. domain/source/verified default from the file's meta block and
// may be overridden per entry (entry-level wins, else inherit file-level).
package errorcodes

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed web.yaml
var webYAML []byte

//go:embed voice-agent.yaml
var voiceAgentYAML []byte

// Domain identifiers. An entry's domain disambiguates the same code across
// products (e.g. a numeric code meaning different things on web vs voice-agent).
const (
	DomainWebSDK     = "web-sdk"
	DomainVoiceAgent = "voice-agent"
)

// Verified trust status values used by the embedded knowledge base.
//   - verified:     cross-checked against the official docs
//   - curated-seed: our curated placeholder, not officially verified
//   - unverified:   source unknown / pending review
const (
	VerifiedYes  = "verified"
	VerifiedSeed = "curated-seed"
	VerifiedNo   = "unverified"
)

// Entry is one SDK error code with curated explanation and doctor linkage.
type Entry struct {
	Code        string `yaml:"code" json:"code"`
	Name        string `yaml:"name,omitempty" json:"name,omitempty"`
	Enum        string `yaml:"enum" json:"enum"`
	Kind        string `yaml:"kind" json:"kind"`
	Via         string `yaml:"via,omitempty" json:"via,omitempty"`
	Meaning     string `yaml:"meaning" json:"meaning"`
	Fix         string `yaml:"fix,omitempty" json:"fix,omitempty"`
	DoctorCheck string `yaml:"doctor_check,omitempty" json:"doctor_check,omitempty"`
	// Domain/Source/trust metadata. Defaults inherited from the file meta block
	// when omitted at the entry level (see applyMetaDefaults).
	Domain        string `yaml:"domain,omitempty" json:"domain,omitempty"`
	Source        string `yaml:"source,omitempty" json:"source,omitempty"`
	SourceVersion string `yaml:"source_version,omitempty" json:"source_version,omitempty"`
	VerifiedAt    string `yaml:"verified_at,omitempty" json:"verified_at,omitempty"`
	Verified      string `yaml:"verified,omitempty" json:"verified"`
}

type fileMeta struct {
	Platform      string   `yaml:"platform"`
	SDKVersion    string   `yaml:"sdk_version"`
	Source        string   `yaml:"source"`
	Fetched       string   `yaml:"fetched"`
	Enums         []string `yaml:"enums"`
	Domain        string   `yaml:"domain"`
	SourceVersion string   `yaml:"source_version"`
	VerifiedAt    string   `yaml:"verified_at"`
	Verified      string   `yaml:"verified"`
	// Status is the legacy field voice-agent.yaml used before `verified` existed
	// (e.g. `status: curated-seed`); read it as a fallback for Verified.
	Status string `yaml:"status"`
}

type file struct {
	Meta  fileMeta `yaml:"meta"`
	Codes []Entry  `yaml:"codes"`
}

// KB is a loaded, indexed error-code knowledge base.
type KB struct {
	Platform   string
	SDKVersion string
	Source     string
	entries    []Entry
	byKey      map[string]Entry
}

// applyMetaDefaults fills an entry's domain/source/trust metadata from the file
// meta when the entry does not set them itself (entry-level wins, else inherit).
func applyMetaDefaults(e Entry, m fileMeta) Entry {
	if e.Domain == "" {
		e.Domain = m.Domain
	}
	if e.Source == "" {
		e.Source = m.Source
	}
	if e.SourceVersion == "" {
		e.SourceVersion = m.SourceVersion
	}
	if e.VerifiedAt == "" {
		e.VerifiedAt = m.VerifiedAt
	}
	if e.Verified == "" {
		switch {
		case m.Verified != "":
			e.Verified = m.Verified
		case m.Status != "":
			e.Verified = m.Status
		default:
			e.Verified = VerifiedNo
		}
	}
	return e
}

// buildKB merges the given files into one indexed KB, stamping each entry with
// its file's domain/source/trust defaults so lookups never lose provenance.
// The first file supplies the KB-level Platform/SDKVersion/Source summary
// (kept for compatibility with existing callers/tests).
func buildKB(files ...file) *KB {
	kb := &KB{byKey: make(map[string]Entry)}
	if len(files) > 0 {
		kb.Platform = files[0].Meta.Platform
		kb.SDKVersion = files[0].Meta.SDKVersion
		kb.Source = files[0].Meta.Source
	}
	for _, f := range files {
		for _, e := range f.Codes {
			e = applyMetaDefaults(e, f.Meta)
			kb.entries = append(kb.entries, e)
			// Index by both the raw code and the optional symbolic name so string
			// codes (INVALID_TOKEN), numeric codes (1202) and named aliases resolve.
			kb.byKey[normalize(e.Code)] = e
			if e.Name != "" {
				kb.byKey[normalize(e.Name)] = e
			}
		}
	}
	return kb
}

func parseFile(data []byte, name string) (file, error) {
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return file{}, fmt.Errorf("parse embedded %s: %w", name, err)
	}
	return f, nil
}

// LoadWebSDK parses only the Web SDK knowledge base (web.yaml).
func LoadWebSDK() (*KB, error) {
	web, err := parseFile(webYAML, "web.yaml")
	if err != nil {
		return nil, err
	}
	return buildKB(web), nil
}

// LoadVoiceAgent parses only the conversational-AI (voice-agent) knowledge base.
func LoadVoiceAgent() (*KB, error) {
	agent, err := parseFile(voiceAgentYAML, "voice-agent.yaml")
	if err != nil {
		return nil, err
	}
	return buildKB(agent), nil
}

// LoadAll parses both knowledge bases merged into one lookup; each entry keeps
// its own domain so a cross-domain query still resolves unambiguously.
func LoadAll() (*KB, error) {
	web, err := parseFile(webYAML, "web.yaml")
	if err != nil {
		return nil, err
	}
	agent, err := parseFile(voiceAgentYAML, "voice-agent.yaml")
	if err != nil {
		return nil, err
	}
	return buildKB(web, agent), nil
}

// LoadWeb is a backwards-compatible alias for LoadAll. The name is retained for
// existing call sites; it covers both the Web SDK and voice-agent domains.
func LoadWeb() (*KB, error) { return LoadAll() }

// Lookup finds an entry by string code, numeric code, or symbolic name.
// Matching is case-insensitive and tolerant of surrounding whitespace. The
// returned entry carries its domain/source/verified metadata.
func (kb *KB) Lookup(code string) (Entry, bool) {
	e, ok := kb.byKey[normalize(code)]
	return e, ok
}

// Count returns the number of entries.
func (kb *KB) Count() int { return len(kb.entries) }

// CountByDomain returns the number of entries in the given domain.
func (kb *KB) CountByDomain(domain string) int {
	n := 0
	for _, e := range kb.entries {
		if e.Domain == domain {
			n++
		}
	}
	return n
}

// Entries returns all entries (read-only view).
func (kb *KB) Entries() []Entry { return kb.entries }

func normalize(code string) string {
	c := strings.TrimSpace(code)
	// Numeric codes may arrive as "1202" or 1202; canonicalize leading zeros.
	if n, err := strconv.Atoi(c); err == nil {
		return strconv.Itoa(n)
	}
	return strings.ToUpper(c)
}
