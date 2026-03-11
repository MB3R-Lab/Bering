package release

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type OCIImageOptions struct {
	RepoRoot        string
	DistDir         string
	Dockerfile      string
	ImageRepository string
	AppVersion      string
	GitCommit       string
	BuildDate       string
	Publish         bool
}

func ImageTags(appVersion, gitCommit string) ([]string, error) {
	if err := ValidateSemVer(appVersion); err != nil {
		return nil, err
	}
	minorTag, err := MajorMinorTag(appVersion)
	if err != nil {
		return nil, err
	}
	commit := strings.TrimSpace(gitCommit)
	if commit == "" {
		return nil, fmt.Errorf("git commit cannot be empty")
	}
	return []string{
		TagForVersion(appVersion),
		minorTag,
		"sha-" + commit,
	}, nil
}

func BuildOCIImage(opts OCIImageOptions) (OCIImageManifest, error) {
	if err := ValidateSemVer(opts.AppVersion); err != nil {
		return OCIImageManifest{}, err
	}
	buildTime, err := parseBuildDate(opts.BuildDate)
	if err != nil {
		return OCIImageManifest{}, err
	}
	if strings.TrimSpace(opts.ImageRepository) == "" {
		return OCIImageManifest{}, fmt.Errorf("image repository cannot be empty")
	}

	tags, err := ImageTags(opts.AppVersion, opts.GitCommit)
	if err != nil {
		return OCIImageManifest{}, err
	}
	references := make([]string, 0, len(tags))
	for _, tag := range tags {
		references = append(references, fmt.Sprintf("%s:%s", strings.TrimRight(opts.ImageRepository, ":"), tag))
	}

	ociDir := filepath.Join(opts.DistDir, "oci")
	if err := ensureDir(ociDir); err != nil {
		return OCIImageManifest{}, err
	}

	dockerfile := opts.Dockerfile
	if !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(opts.RepoRoot, dockerfile)
	}

	manifest := OCIImageManifest{
		Repository: opts.ImageRepository,
		References: references,
		Tags:       tags,
		Platforms:  []string{"linux/amd64", "linux/arm64"},
		Published:  opts.Publish,
		BuildDate:  buildTime.Format(timeRFC3339),
		GitCommit:  opts.GitCommit,
	}

	ctx, cancel := defaultCommandContext()
	defer cancel()

	env := []string{
		fmt.Sprintf("SOURCE_DATE_EPOCH=%d", buildTime.Unix()),
		"BUILDKIT_MULTI_PLATFORM=1",
	}
	args := []string{
		"buildx", "build",
		"--platform", "linux/amd64,linux/arm64",
		"--file", dockerfile,
		"--build-arg", "VERSION=" + opts.AppVersion,
		"--build-arg", "COMMIT=" + opts.GitCommit,
		"--build-arg", "BUILD_DATE=" + buildTime.Format(timeRFC3339),
	}

	if opts.Publish {
		for _, ref := range references {
			args = append(args, "--tag", ref)
		}
		args = append(args, "--push", opts.RepoRoot)
		if _, err := runCommand(ctx, opts.RepoRoot, env, "docker", args...); err != nil {
			return OCIImageManifest{}, err
		}

		inspectOutput, err := runCommand(ctx, opts.RepoRoot, nil, "docker", "buildx", "imagetools", "inspect", references[0])
		if err != nil {
			return OCIImageManifest{}, err
		}
		manifest.Digest, manifest.MediaType, manifest.Platforms = parseImageInspectOutput(inspectOutput)
	} else {
		localLayout := filepath.Join(ociDir, fmt.Sprintf("%s_%s.oci.tar", ProductName, opts.AppVersion))
		args = append(args, "--output", fmt.Sprintf("type=oci,dest=%s,rewrite-timestamp=true", localLayout), opts.RepoRoot)
		if _, err := runCommand(ctx, opts.RepoRoot, env, "docker", args...); err != nil {
			return OCIImageManifest{}, err
		}
		manifest.LocalLayout = normalizePath(filepath.Join("oci", filepath.Base(localLayout)))
		if digest, err := DigestFile(localLayout); err == nil {
			manifest.SourceArtifact = digest
		}
	}

	manifestPath := filepath.Join(opts.DistDir, "oci-image-manifest.json")
	if err := writeJSON(manifestPath, manifest); err != nil {
		return OCIImageManifest{}, err
	}

	return manifest, nil
}

func parseImageInspectOutput(output string) (string, string, []string) {
	var digest string
	var mediaType string
	platformSet := map[string]struct{}{}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Digest:") && digest == "":
			digest = strings.TrimSpace(strings.TrimPrefix(line, "Digest:"))
		case strings.HasPrefix(line, "MediaType:") && mediaType == "":
			mediaType = strings.TrimSpace(strings.TrimPrefix(line, "MediaType:"))
		case strings.HasPrefix(line, "Platform:"):
			platform := strings.TrimSpace(strings.TrimPrefix(line, "Platform:"))
			if platform != "" {
				platformSet[platform] = struct{}{}
			}
		}
	}

	platforms := make([]string, 0, len(platformSet))
	for platform := range platformSet {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	if len(platforms) == 0 {
		platforms = []string{"linux/amd64", "linux/arm64"}
	}
	return digest, mediaType, platforms
}
