// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

package releasecontract

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/skillscan"
)

// SourceMode selects committed source or the current Git-managed worktree
// snapshot. Both are copied into Destination before stamping.
type SourceMode string

const (
	SourceCommit   SourceMode = "commit"
	SourceWorktree SourceMode = "worktree"
)

// PrepareOptions controls isolated release-source construction.
type PrepareOptions struct {
	RepoRoot     string
	Destination  string
	Stability    Stability
	Publication  Destination
	Version      string
	Source       SourceMode
	Ref          string
	AllowDirty   bool
	AfterCopyFor func(string) error // optional failure-injection hook for tests
}

// Prepare validates source, copies it into an owned destination, stamps every
// Skill there, and returns the postflight manifest. Destination must not exist.
func Prepare(options PrepareOptions) (manifest Manifest, err error) {
	repoRoot, err := filepath.Abs(options.RepoRoot)
	if err != nil {
		return Manifest{}, err
	}
	destination, err := filepath.Abs(options.Destination)
	if err != nil {
		return Manifest{}, err
	}
	if destination == repoRoot || strings.HasPrefix(destination+string(filepath.Separator), repoRoot+string(filepath.Separator)) {
		return Manifest{}, errors.New("release destination must be outside the invoking repository")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		if err == nil {
			return Manifest{}, fmt.Errorf("release destination already exists: %s", destination)
		}
		return Manifest{}, err
	}

	ref := strings.TrimSpace(options.Ref)
	if ref == "" {
		ref = "HEAD"
	}
	sourceCommit, err := gitOutput(repoRoot, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve source ref %q: %w", ref, err)
	}
	sourceCommit = strings.TrimSpace(sourceCommit)
	sourceDate, err := gitOutput(repoRoot, "show", "-s", "--format=%cI", sourceCommit)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve source commit date %q: %w", sourceCommit, err)
	}
	sourceDate = strings.TrimSpace(sourceDate)
	if sourceDate == "" {
		return Manifest{}, errors.New("source commit date is empty")
	}

	var preflightSkills []Skill
	var baseline string
	if options.Source == SourceWorktree {
		if !options.AllowDirty {
			status, err := gitOutput(repoRoot, "status", "--porcelain=v1", "--untracked-files=all")
			if err != nil {
				return Manifest{}, fmt.Errorf("inspect source worktree: %w", err)
			}
			if status != "" {
				return Manifest{}, errors.New("refusing dirty release source without explicit allow-dirty")
			}
		}
		preflightSkills, baseline, err = DiscoverSkills(repoRoot)
		if err != nil {
			return Manifest{}, err
		}
	}

	owned := false
	defer func() {
		if err != nil && owned {
			_ = os.RemoveAll(destination)
		}
	}()
	if err = os.MkdirAll(destination, 0o755); err != nil {
		return Manifest{}, fmt.Errorf("create release destination: %w", err)
	}
	owned = true

	switch options.Source {
	case SourceCommit:
		err = exportCommit(repoRoot, ref, destination)
	case SourceWorktree:
		err = copyWorktree(repoRoot, destination)
	default:
		err = fmt.Errorf("unknown release source mode %q", options.Source)
	}
	if err != nil {
		return Manifest{}, err
	}
	if options.AfterCopyFor != nil {
		if err = options.AfterCopyFor(destination); err != nil {
			return Manifest{}, err
		}
	}

	preparedSkills, preparedBaseline, err := DiscoverSkills(destination)
	if err != nil {
		return Manifest{}, fmt.Errorf("validate prepared source: %w", err)
	}
	if options.Source == SourceWorktree {
		if preparedBaseline != baseline || !sameSkillSet(preflightSkills, preparedSkills) {
			return Manifest{}, errors.New("prepared Skill set differs from preflight Skill set")
		}
	} else {
		baseline = preparedBaseline
	}
	identity, err := Resolve(options.Stability, options.Publication, options.Version, baseline)
	if err != nil {
		return Manifest{}, err
	}
	if err = StampSkills(destination, identity.Version, preparedSkills); err != nil {
		return Manifest{}, err
	}
	if err = stampPackageVersion(filepath.Join(destination, "package.json"), identity.Version); err != nil {
		return Manifest{}, err
	}
	if err = validateStampedSkills(destination, identity.Version, preparedSkills); err != nil {
		return Manifest{}, err
	}
	for index := range preparedSkills {
		preparedSkills[index].Version = identity.Version
	}
	targets := releaseTargets(identity.Version)
	return Manifest{
		Identity:     identity,
		Source:       string(options.Source),
		SourceRoot:   destination,
		SourceCommit: sourceCommit,
		SourceDate:   sourceDate,
		Skills:       preparedSkills,
		Targets:      targets,
		PackageName:  "@volcengine/rtc-cli",
		ChecksumFile: "checksums.txt",
	}, nil
}

func stampPackageVersion(file, version string) error {
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return errors.New("release source package.json is missing")
	}
	if err != nil {
		return fmt.Errorf("read package metadata: %w", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("parse package metadata: %w", err)
	}
	if _, ok := metadata["version"]; !ok {
		return errors.New("package metadata has no version field")
	}
	metadata["version"] = version
	updated, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	if err := os.WriteFile(file, updated, 0o644); err != nil {
		return fmt.Errorf("stamp package metadata: %w", err)
	}
	return nil
}

func releaseTargets(version string) []Target {
	targets := make([]Target, 0, 6)
	for _, goos := range []string{"darwin", "linux", "windows"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			binary := "vertc"
			extension := ".tar.gz"
			if goos == "windows" {
				binary = "vertc.exe"
				extension = ".zip"
			}
			targets = append(targets, Target{
				GOOS: goos, GOARCH: goarch, Binary: binary,
				Archive: fmt.Sprintf("vertc_%s_%s_%s%s", version, goos, goarch, extension),
			})
		}
	}
	return targets
}

func sameSkillSet(left, right []Skill) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Name != right[index].Name || left[index].Path != right[index].Path {
			return false
		}
	}
	return true
}

func validateStampedSkills(root, version string, expected []Skill) error {
	problems, err := skillscan.CheckSkills(filepath.Join(root, "skills"))
	if err != nil {
		return err
	}
	if len(problems) > 0 {
		return fmt.Errorf("stamped Skill quality validation failed: %s: %s", problems[0].SourceFile, problems[0].Reason)
	}
	for _, skill := range expected {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(skill.Path)))
		if err != nil {
			return err
		}
		fm, err := parseFrontmatter(data)
		if err != nil {
			return err
		}
		if fm.Name != skill.Name || fm.Version != version {
			return fmt.Errorf("stamped Skill %q has name/version %q/%q, want %q/%q", skill.Name, fm.Name, fm.Version, skill.Name, version)
		}
	}
	return nil
}

func gitOutput(root string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return "", fmt.Errorf("%s: %w", message, err)
		}
		return "", err
	}
	return string(output), nil
}

func copyWorktree(repoRoot, destination string) error {
	output, err := gitOutput(repoRoot, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return fmt.Errorf("enumerate release source: %w", err)
	}
	paths := strings.Split(output, "\x00")
	sort.Strings(paths)
	for _, rel := range paths {
		if rel == "" {
			continue
		}
		clean, err := cleanRelative(rel)
		if err != nil {
			return err
		}
		source := filepath.Join(repoRoot, clean)
		info, err := os.Lstat(source)
		if os.IsNotExist(err) {
			continue // staged/unstaged deletion: the working-tree snapshot omits it
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(source)
			if err != nil {
				return err
			}
			if err := validateSymlink(clean, target); err != nil {
				return err
			}
			destinationPath := filepath.Join(destination, clean)
			if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(target, destinationPath); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("release source path %q must be a regular file", rel)
		}
		if err := copyRegularFile(source, filepath.Join(destination, clean), info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func exportCommit(repoRoot, ref, destination string) error {
	command := exec.Command("git", "-c", "core.autocrlf=false", "-c", "core.eol=lf", "-C", repoRoot, "archive", "--format=tar", ref)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return err
	}
	waited := false
	defer func() {
		if !waited {
			_ = stdout.Close()
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	reader := tar.NewReader(stdout)
	for {
		header, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read committed source archive: %w", readErr)
		}
		clean, cleanErr := cleanRelative(header.Name)
		if cleanErr != nil {
			return cleanErr
		}
		target := filepath.Join(destination, clean)
		switch header.Typeflag {
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, byte(0):
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode)&0o777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if err := validateSymlink(clean, header.Linkname); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("committed release source %q has unsupported archive type %d", header.Name, header.Typeflag)
		}
	}
	// archive/tar stops at the logical end marker, before git archive closes its
	// stdout. Drain the remaining padding so the child cannot block on a full
	// Windows pipe while Wait waits for the child to exit.
	if _, err := io.Copy(io.Discard, stdout); err != nil {
		return fmt.Errorf("drain committed source archive: %w", err)
	}
	waitErr := command.Wait()
	waited = true
	if waitErr != nil {
		return fmt.Errorf("export committed release source: %s: %w", strings.TrimSpace(stderr.String()), waitErr)
	}
	return nil
}

func validateSymlink(rel, target string) error {
	if target == "" || filepath.IsAbs(target) {
		return fmt.Errorf("release source symlink %q has unsafe target %q", rel, target)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(rel), target))
	if resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
		return fmt.Errorf("release source symlink %q escapes through target %q", rel, target)
	}
	return nil
}

func cleanRelative(rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.ContainsRune(rel, '\x00') {
		return "", fmt.Errorf("unsafe release source path %q", rel)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe release source path %q", rel)
	}
	return clean, nil
}

func copyRegularFile(source, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
