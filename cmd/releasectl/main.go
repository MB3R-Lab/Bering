package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/MB3R-Lab/Bering/internal/release"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return 1
	}

	switch args[0] {
	case "contracts-pack":
		return runContractsPack(args[1:])
	case "chart-package":
		return runChartPackage(args[1:])
	case "oci-image":
		return runOCIImage(args[1:])
	case "release-manifest":
		return runReleaseManifest(args[1:])
	case "validate":
		return runValidate(args[1:])
	case "help", "--help", "-h":
		printUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage(os.Stderr)
		return 1
	}
}

func runContractsPack(args []string) int {
	fs := flag.NewFlagSet("contracts-pack", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repoRoot := fs.String("repo-root", ".", "Repository root")
	distDir := fs.String("dist-dir", "dist", "Distribution directory")
	appVersion := fs.String("app-version", "", "App version without a leading v")
	buildDate := fs.String("build-date", "", "RFC3339 build date")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	manifest, archive, err := release.GenerateContractsPack(release.ContractsPackOptions{
		RepoRoot:   strings.TrimSpace(*repoRoot),
		DistDir:    strings.TrimSpace(*distDir),
		AppVersion: strings.TrimSpace(*appVersion),
		BuildDate:  strings.TrimSpace(*buildDate),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("contracts pack written: %s (%d contracts)\n", archive, len(manifest.Contracts))
	return 0
}

func runChartPackage(args []string) int {
	fs := flag.NewFlagSet("chart-package", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repoRoot := fs.String("repo-root", ".", "Repository root")
	distDir := fs.String("dist-dir", "dist", "Distribution directory")
	chartDir := fs.String("chart-dir", "charts/bering", "Chart source directory")
	appVersion := fs.String("app-version", "", "App version without a leading v")
	chartVersion := fs.String("chart-version", "", "Chart version without a leading v")
	ociRepository := fs.String("oci-repository", "", "OCI chart repository, for example oci://registry.example.com/charts")
	publish := fs.Bool("publish", false, "Publish the chart to the OCI registry")
	allowMismatch := fs.Bool("allow-chart-version-mismatch", false, "Allow chart version to differ from the app version")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	metadata, err := release.PackageChart(release.ChartPackageOptions{
		RepoRoot:                  strings.TrimSpace(*repoRoot),
		DistDir:                   strings.TrimSpace(*distDir),
		ChartDir:                  strings.TrimSpace(*chartDir),
		AppVersion:                strings.TrimSpace(*appVersion),
		ChartVersion:              strings.TrimSpace(*chartVersion),
		OCIRepository:             strings.TrimSpace(*ociRepository),
		Publish:                   *publish,
		AllowChartVersionMismatch: *allowMismatch,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("chart package written: %s\n", metadata.Package)
	if metadata.Published {
		fmt.Printf("chart published: %s (%s)\n", metadata.OCIReference, metadata.Digest)
	}
	return 0
}

func runOCIImage(args []string) int {
	fs := flag.NewFlagSet("oci-image", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repoRoot := fs.String("repo-root", ".", "Repository root")
	distDir := fs.String("dist-dir", "dist", "Distribution directory")
	dockerfile := fs.String("dockerfile", "build/Dockerfile", "Dockerfile path")
	imageRepository := fs.String("image-repository", "", "OCI image repository")
	appVersion := fs.String("app-version", "", "App version without a leading v")
	gitCommit := fs.String("git-commit", "", "Git commit SHA")
	buildDate := fs.String("build-date", "", "RFC3339 build date")
	publish := fs.Bool("publish", false, "Publish the OCI image")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	manifest, err := release.BuildOCIImage(release.OCIImageOptions{
		RepoRoot:        strings.TrimSpace(*repoRoot),
		DistDir:         strings.TrimSpace(*distDir),
		Dockerfile:      strings.TrimSpace(*dockerfile),
		ImageRepository: strings.TrimSpace(*imageRepository),
		AppVersion:      strings.TrimSpace(*appVersion),
		GitCommit:       strings.TrimSpace(*gitCommit),
		BuildDate:       strings.TrimSpace(*buildDate),
		Publish:         *publish,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if manifest.Published {
		fmt.Printf("oci image published: %s (%s)\n", manifest.References[0], manifest.Digest)
	} else {
		fmt.Printf("oci image layout written: %s\n", manifest.LocalLayout)
	}
	return 0
}

func runReleaseManifest(args []string) int {
	fs := flag.NewFlagSet("release-manifest", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repoRoot := fs.String("repo-root", ".", "Repository root")
	distDir := fs.String("dist-dir", "dist", "Distribution directory")
	appVersion := fs.String("app-version", "", "App version without a leading v")
	gitCommit := fs.String("git-commit", "", "Git commit SHA")
	gitTag := fs.String("git-tag", "", "Git tag, typically vX.Y.Z")
	buildDate := fs.String("build-date", "", "RFC3339 build date")
	headline := fs.String("headline", "", "Release notes headline")
	summary := fs.String("summary", "", "Release notes summary")
	breakingCLI := fs.Bool("breaking-cli", false, "Mark CLI surface as breaking")
	breakingAPI := fs.Bool("breaking-api", false, "Mark API surface as breaking")
	breakingInstall := fs.Bool("breaking-install", false, "Mark install surface as breaking")
	var highlights csvFlag
	var breakingContracts csvFlag
	fs.Var(&highlights, "highlight", "Release highlight; may be repeated")
	fs.Var(&breakingContracts, "breaking-contract", "Breaking contract identifier; may be repeated")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	manifest, err := release.GenerateReleaseManifest(release.ReleaseManifestOptions{
		RepoRoot:          strings.TrimSpace(*repoRoot),
		DistDir:           strings.TrimSpace(*distDir),
		AppVersion:        strings.TrimSpace(*appVersion),
		GitCommit:         strings.TrimSpace(*gitCommit),
		GitTag:            strings.TrimSpace(*gitTag),
		BuildDate:         strings.TrimSpace(*buildDate),
		Headline:          strings.TrimSpace(*headline),
		Summary:           strings.TrimSpace(*summary),
		Highlights:        highlights.Values(),
		BreakingCLI:       *breakingCLI,
		BreakingAPI:       *breakingAPI,
		BreakingInstall:   *breakingInstall,
		BreakingContracts: breakingContracts.Values(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("release manifest written: %s (%d binaries)\n", release.ReleaseManifestName, len(manifest.Binaries))
	return 0
}

func runValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repoRoot := fs.String("repo-root", ".", "Repository root")
	distDir := fs.String("dist-dir", "dist", "Distribution directory")
	appVersion := fs.String("app-version", "", "App version without a leading v")
	buildDate := fs.String("build-date", "", "RFC3339 build date")
	requireOCI := fs.Bool("require-published-oci", false, "Require published OCI image and chart digests")
	allowMismatch := fs.Bool("allow-chart-version-mismatch", false, "Allow chart version to differ from the app version")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if err := release.ValidateRelease(release.ValidateOptions{
		RepoRoot:                  strings.TrimSpace(*repoRoot),
		DistDir:                   strings.TrimSpace(*distDir),
		AppVersion:                strings.TrimSpace(*appVersion),
		BuildDate:                 strings.TrimSpace(*buildDate),
		RequirePublishedOCI:       *requireOCI,
		AllowChartVersionMismatch: *allowMismatch,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Println("release validation passed")
	return 0
}

func printUsage(out *os.File) {
	fmt.Fprintln(out, "releasectl commands:")
	fmt.Fprintln(out, "  contracts-pack")
	fmt.Fprintln(out, "  chart-package")
	fmt.Fprintln(out, "  oci-image")
	fmt.Fprintln(out, "  release-manifest")
	fmt.Fprintln(out, "  validate")
}

type csvFlag struct {
	items []string
}

func (f *csvFlag) String() string {
	return strings.Join(f.items, ",")
}

func (f *csvFlag) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("value cannot be empty")
	}
	f.items = append(f.items, trimmed)
	return nil
}

func (f *csvFlag) Values() []string {
	return append([]string(nil), f.items...)
}
