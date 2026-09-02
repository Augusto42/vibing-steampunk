package adt

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SSOHelperName is the filename of the Windows-side capture helper.
const SSOHelperName = "vsp-sso.exe"

// IsWSL reports whether this process runs inside the Linux side of WSL.
//
// It matters for SSO because the credential that satisfies device-based
// Conditional Access — the Entra Primary Refresh Token, held by the Windows
// account broker — is unreachable from Linux. A browser started here would loop
// on the identity provider forever no matter how it is configured, so the
// browser step has to be handed to a Windows process.
func IsWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	release, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(release)), "microsoft")
}

// windowsEnvVar reads an environment variable from the Windows side.
// The command runs with a drive-backed working directory: started from a Linux
// path, cmd.exe warns about the unsupported UNC path and falls back, which
// pollutes stderr for no reason.
func windowsEnvVar(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, "cmd.exe", "/C", "echo %"+name+"%")
	cmd.Dir = "/mnt/c"
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("reading Windows %%%s%% (is WSL interop enabled?): %w", name, err)
	}
	value := strings.TrimSpace(string(out))
	if value == "" || value == "%"+name+"%" {
		return "", fmt.Errorf("Windows %%%s%% is not set", name)
	}
	return value, nil
}

// windowsPathToLinux converts a Windows path to the /mnt/... path that reaches
// the same file from Linux.
func windowsPathToLinux(ctx context.Context, winPath string) (string, error) {
	out, err := exec.CommandContext(ctx, "wslpath", "-u", winPath).Output()
	if err != nil {
		return "", fmt.Errorf("converting Windows path %q: %w", winPath, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// linuxPathToWindows converts a Linux path to its Windows form.
func linuxPathToWindows(ctx context.Context, linuxPath string) (string, error) {
	out, err := exec.CommandContext(ctx, "wslpath", "-w", linuxPath).Output()
	if err != nil {
		return "", fmt.Errorf("converting path %q for Windows: %w", linuxPath, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// StageSSOHelper makes the capture helper runnable as a Windows process and
// returns the Linux path that reaches it.
//
// A helper sitting in the Linux filesystem cannot simply be launched: Windows
// would have to reach it over a UNC share, which is refused for executables in
// many configurations. Copying it onto a Windows drive first sidesteps that
// entirely. The copy is skipped when an up-to-date one is already staged.
//
// If src already lives on a mounted Windows drive, it is used where it is.
func StageSSOHelper(ctx context.Context, src string) (string, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("SSO helper %s: %w", src, err)
	}
	if strings.HasPrefix(src, "/mnt/") {
		return src, nil
	}

	winTemp, err := windowsEnvVar(ctx, "TEMP")
	if err != nil {
		return "", err
	}
	stageDir, err := windowsPathToLinux(ctx, winTemp)
	if err != nil {
		return "", err
	}
	stageDir = filepath.Join(stageDir, "vsp-sso")
	if err := os.MkdirAll(stageDir, 0755); err != nil {
		return "", fmt.Errorf("creating helper staging directory: %w", err)
	}
	dst := filepath.Join(stageDir, SSOHelperName)

	// Re-staging on every run would copy several megabytes across the 9p mount
	// for nothing. Size plus mtime is enough to notice a rebuilt helper.
	if dstInfo, err := os.Stat(dst); err == nil &&
		dstInfo.Size() == srcInfo.Size() &&
		!srcInfo.ModTime().After(dstInfo.ModTime()) {
		return dst, nil
	}

	if err := copyFile(src, dst); err != nil {
		return "", fmt.Errorf("staging SSO helper: %w", err)
	}
	return dst, nil
}

// copyFile writes src to dst through a temporary file, so a partially copied
// helper is never left behind under the real name.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// FindSSOHelper locates the Windows capture helper.
//
// Search order: an explicit path, then the directory holding the running vsp
// binary, then a ./build directory beside it — which is where `make build-all`
// leaves the cross-compiled helper.
func FindSSOHelper(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("configured SSO helper not found: %s", explicit)
		}
		return explicit, nil
	}

	var dirs []string
	if self, err := os.Executable(); err == nil {
		selfDir := filepath.Dir(self)
		dirs = append(dirs, selfDir, filepath.Join(selfDir, "build"))
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, cwd, filepath.Join(cwd, "build"))
	}

	for _, dir := range dirs {
		candidate := filepath.Join(dir, SSOHelperName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s not found — build it with `GOOS=windows GOARCH=amd64 go build -o %s ./cmd/vsp-sso` (or `make build-all`), or set the helper path in the system's sso config",
		SSOHelperName, SSOHelperName)
}
