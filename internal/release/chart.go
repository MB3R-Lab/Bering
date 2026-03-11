package release

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ChartPackageOptions struct {
	RepoRoot                  string
	DistDir                   string
	ChartDir                  string
	AppVersion                string
	ChartVersion              string
	OCIRepository             string
	Publish                   bool
	AllowChartVersionMismatch bool
}

type chartFile struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	AppVersion string `yaml:"appVersion"`
}

func readChartFile(chartDir string) (chartFile, error) {
	var chart chartFile
	raw, err := os.ReadFile(filepath.Join(chartDir, "Chart.yaml"))
	if err != nil {
		return chart, err
	}
	if err := yaml.Unmarshal(raw, &chart); err != nil {
		return chart, err
	}
	if strings.TrimSpace(chart.Name) == "" {
		return chart, fmt.Errorf("chart name cannot be empty")
	}
	return chart, nil
}

func PackageChart(opts ChartPackageOptions) (ChartMetadata, error) {
	if err := ValidateSemVer(opts.AppVersion); err != nil {
		return ChartMetadata{}, err
	}
	if strings.TrimSpace(opts.ChartVersion) == "" {
		opts.ChartVersion = opts.AppVersion
	}
	if err := ValidateSemVer(opts.ChartVersion); err != nil {
		return ChartMetadata{}, fmt.Errorf("chart version: %w", err)
	}
	if opts.ChartVersion != opts.AppVersion && !opts.AllowChartVersionMismatch {
		return ChartMetadata{}, fmt.Errorf("chart version %s does not match app version %s; set ALLOW_CHART_VERSION_MISMATCH=1 to override", opts.ChartVersion, opts.AppVersion)
	}

	chartDir := opts.ChartDir
	if !filepath.IsAbs(chartDir) {
		chartDir = filepath.Join(opts.RepoRoot, chartDir)
	}
	chart, err := readChartFile(chartDir)
	if err != nil {
		return ChartMetadata{}, fmt.Errorf("read chart metadata: %w", err)
	}

	distChartsDir := filepath.Join(opts.DistDir, "charts")
	if err := ensureDir(distChartsDir); err != nil {
		return ChartMetadata{}, err
	}

	ctx, cancel := defaultCommandContext()
	defer cancel()

	if _, err := runCommand(ctx, opts.RepoRoot, nil, "helm", "lint", chartDir); err != nil {
		return ChartMetadata{}, err
	}
	if _, err := runCommand(
		ctx,
		opts.RepoRoot,
		nil,
		"helm",
		"package",
		chartDir,
		"--destination", distChartsDir,
		"--version", opts.ChartVersion,
		"--app-version", opts.AppVersion,
	); err != nil {
		return ChartMetadata{}, err
	}

	packagePath := filepath.Join(distChartsDir, fmt.Sprintf("%s-%s.tgz", chart.Name, opts.ChartVersion))
	packageDigest, err := DigestFile(packagePath)
	if err != nil {
		return ChartMetadata{}, fmt.Errorf("digest chart package: %w", err)
	}

	metadata := ChartMetadata{
		Name:           chart.Name,
		Version:        opts.ChartVersion,
		AppVersion:     opts.AppVersion,
		Package:        normalizePath(filepath.Join("charts", filepath.Base(packagePath))),
		PackageDigest:  packageDigest,
		Published:      false,
		SourceChartDir: normalizePath(relativeTo(opts.RepoRoot, chartDir)),
	}

	if opts.Publish {
		if strings.TrimSpace(opts.OCIRepository) == "" {
			return ChartMetadata{}, fmt.Errorf("oci repository is required when chart publish is enabled")
		}
		output, err := runCommand(ctx, opts.RepoRoot, nil, "helm", "push", packagePath, opts.OCIRepository)
		if err != nil {
			return ChartMetadata{}, err
		}
		ref, digest := parseHelmPushOutput(output)
		if ref == "" {
			ref = strings.TrimRight(opts.OCIRepository, "/") + "/" + chart.Name + ":" + opts.ChartVersion
		}
		metadata.Published = true
		metadata.Repository = opts.OCIRepository
		metadata.OCIReference = ref
		metadata.Digest = digest
	}

	metadataPath := filepath.Join(opts.DistDir, "chart-metadata.json")
	if err := writeJSON(metadataPath, metadata); err != nil {
		return ChartMetadata{}, err
	}

	return metadata, nil
}

func parseHelmPushOutput(output string) (string, string) {
	var ref string
	var digest string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Pushed:"):
			ref = strings.TrimSpace(strings.TrimPrefix(line, "Pushed:"))
		case strings.HasPrefix(line, "Digest:"):
			digest = strings.TrimSpace(strings.TrimPrefix(line, "Digest:"))
		}
	}
	return ref, digest
}

func ReadPackagedChartMetadata(path string) (ChartMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return ChartMetadata{}, err
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return ChartMetadata{}, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ChartMetadata{}, err
		}
		if filepath.Base(header.Name) != "Chart.yaml" {
			continue
		}

		raw, err := io.ReadAll(tr)
		if err != nil {
			return ChartMetadata{}, err
		}
		var chart chartFile
		if err := yaml.Unmarshal(raw, &chart); err != nil {
			return ChartMetadata{}, err
		}
		return ChartMetadata{
			Name:       chart.Name,
			Version:    chart.Version,
			AppVersion: chart.AppVersion,
		}, nil
	}

	return ChartMetadata{}, fmt.Errorf("Chart.yaml not found in %s", path)
}
