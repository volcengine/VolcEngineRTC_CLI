// Copyright (c) 2026 Beijing Volcano Engine Technology Ltd.
// SPDX-License-Identifier: MIT

// Command releasecmd supports release builds without extending the vertc
// command tree.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/volcengine/VolcEngineRTC_CLI/internal/releasecontract"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: releasecmd <resolve|source-version|prepare|verify|manifest-version> [options]")
	}
	var err error
	switch os.Args[1] {
	case "resolve":
		err = resolve(os.Args[2:])
	case "source-version":
		err = sourceVersion(os.Args[2:])
	case "prepare":
		err = prepare(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	case "manifest-version":
		err = manifestVersion(os.Args[2:])
	default:
		err = fmt.Errorf("unknown releasecmd action %q", os.Args[1])
	}
	if err != nil {
		fail(err.Error())
	}
}

func sourceVersion(args []string) error {
	flags := flag.NewFlagSet("source-version", flag.ContinueOnError)
	repo := flags.String("repo", "", "repository worktree to inspect or update")
	version := flags.String("version", "", "stable or prerelease source version")
	check := flags.Bool("check", false, "require every target to already match without writing")
	dryRun := flags.Bool("dry-run", false, "report planned updates without writing")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *repo == "" || *version == "" {
		return fmt.Errorf("source-version requires --repo and --version")
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("source-version accepts no positional arguments")
	}
	if *check && *dryRun {
		return fmt.Errorf("source-version --check and --dry-run are mutually exclusive")
	}
	plan, err := releasecontract.PlanSourceVersion(*repo, *version)
	if err != nil {
		return err
	}
	if *check {
		if len(plan.Changes) != 0 {
			paths := make([]string, 0, len(plan.Changes))
			for _, change := range plan.Changes {
				paths = append(paths, change.Path)
			}
			return fmt.Errorf("committed release version is not %s in: %v", plan.Version, paths)
		}
		fmt.Printf("release source version %s verified across %d files\n", plan.Version, len(plan.Targets))
		return nil
	}
	if *dryRun {
		for _, change := range plan.Changes {
			fmt.Printf("would update %s\n", change.Path)
		}
		fmt.Printf("release source version %s would update %d of %d files\n", plan.Version, len(plan.Changes), len(plan.Targets))
		return nil
	}
	if err := releasecontract.ApplySourceVersion(plan); err != nil {
		return err
	}
	for _, change := range plan.Changes {
		fmt.Printf("updated %s\n", change.Path)
	}
	fmt.Printf("release source version %s prepared across %d files\n", plan.Version, len(plan.Targets))
	return nil
}

func resolve(args []string) error {
	flags := flag.NewFlagSet("resolve", flag.ContinueOnError)
	stability := flags.String("stability", "", "stable, prerelease, or snapshot")
	publication := flags.String("publication", "", "public or none")
	version := flags.String("version", "", "requested version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	identity, err := releasecontract.Resolve(releasecontract.Stability(*stability), releasecontract.Destination(*publication), *version)
	if err != nil {
		return err
	}
	fmt.Println(identity.Version)
	return nil
}

func prepare(args []string) error {
	flags := flag.NewFlagSet("prepare", flag.ContinueOnError)
	repo := flags.String("repo", "", "invoking repository")
	destination := flags.String("destination", "", "owned prepared source destination")
	manifestPath := flags.String("manifest", "", "manifest output path")
	stability := flags.String("stability", "", "stable, prerelease, or snapshot")
	publication := flags.String("publication", "", "public or none")
	version := flags.String("version", "", "requested version")
	source := flags.String("source", "", "commit or worktree")
	ref := flags.String("ref", "HEAD", "source commit ref")
	allowDirty := flags.Bool("allow-dirty", false, "capture a dirty worktree")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *repo == "" || *destination == "" || *manifestPath == "" || *stability == "" || *publication == "" || *source == "" {
		return fmt.Errorf("prepare requires --repo, --destination, --manifest, --stability, --publication, and --source")
	}
	manifest, err := releasecontract.Prepare(releasecontract.PrepareOptions{
		RepoRoot: *repo, Destination: *destination,
		Stability: releasecontract.Stability(*stability), Publication: releasecontract.Destination(*publication),
		Version: *version, Source: releasecontract.SourceMode(*source), Ref: *ref, AllowDirty: *allowDirty,
	})
	if err != nil {
		return err
	}
	if err := releasecontract.WriteManifest(*manifestPath, manifest); err != nil {
		_ = os.RemoveAll(*destination)
		return fmt.Errorf("write release manifest: %w", err)
	}
	fmt.Println(manifest.Version)
	return nil
}

func verify(args []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	manifestPath := flags.String("manifest", "", "release manifest")
	artifacts := flags.String("artifacts", "", "archive directory")
	checksums := flags.String("checksums", "", "checksum file")
	packageJSON := flags.String("package-json", "", "optional package metadata")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *manifestPath == "" || *artifacts == "" || *checksums == "" {
		return fmt.Errorf("verify requires --manifest, --artifacts, and --checksums")
	}
	manifest, err := releasecontract.ReadManifest(*manifestPath)
	if err != nil {
		return fmt.Errorf("read release manifest: %w", err)
	}
	return releasecontract.Verify(releasecontract.VerifyOptions{
		Manifest: manifest, ArtifactsDir: *artifacts, Checksums: *checksums, PackageJSON: *packageJSON,
	})
}

func manifestVersion(args []string) error {
	flags := flag.NewFlagSet("manifest-version", flag.ContinueOnError)
	manifestPath := flags.String("manifest", "", "release manifest")
	if err := flags.Parse(args); err != nil {
		return err
	}
	manifest, err := releasecontract.ReadManifest(*manifestPath)
	if err != nil {
		return err
	}
	fmt.Println(manifest.Version)
	return nil
}

func fail(message string) {
	fmt.Fprintf(os.Stderr, "release-contract: %s\n", message)
	os.Exit(1)
}
