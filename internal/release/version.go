package release

import (
	"fmt"
	"regexp"
	"strings"
)

var semverPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

func ValidateSemVer(version string) error {
	if !semverPattern.MatchString(strings.TrimSpace(version)) {
		return fmt.Errorf("version %q must be SemVer without a leading v", version)
	}
	return nil
}

func MajorMinorTag(version string) (string, error) {
	if err := ValidateSemVer(version); err != nil {
		return "", err
	}
	parts := strings.SplitN(version, ".", 3)
	return fmt.Sprintf("v%s.%s", parts[0], parts[1]), nil
}

func TagForVersion(version string) string {
	return "v" + strings.TrimSpace(version)
}
