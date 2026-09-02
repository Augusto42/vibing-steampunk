package adt

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// A landscape file is written by whichever SAP GUI wrote it, and on a
// developer's machine that is often not the operating system vsp runs on. The
// WSL case is already handled by reaching across to the Windows side; a Mac
// with a Windows VM is the same situation wearing a different hat, and the
// file that matters — the one with the company's shared <Include> — usually
// lives in the guest, because that is where SAP Logon runs.
//
// Reaching in is deliberately never automatic. Discovery is what `landscape
// scan` is for; reading is explicit, through a "parallels:<vm>:<path>"
// reference the user passes to --file.

const parallelsScheme = "parallels:"

// ParallelsRef builds the reference that names a file inside a guest.
func ParallelsRef(vm, winPath string) string {
	return parallelsScheme + vm + ":" + winPath
}

// ParseParallelsRef splits "parallels:<vm>:<windows path>". The VM name may
// contain spaces ("Windows 11"), and the path contains a drive colon, so the
// split is on the first colon after the scheme only.
func ParseParallelsRef(ref string) (vm, winPath string, ok bool) {
	if !strings.HasPrefix(ref, parallelsScheme) {
		return "", "", false
	}
	rest := ref[len(parallelsScheme):]
	i := strings.Index(rest, ":")
	if i <= 0 || i+1 >= len(rest) {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

// prlctl returns the Parallels CLI if this machine has it. Parallels is macOS
// only, and its absence is the normal case, not an error.
func prlctlPath() (string, bool) {
	if runtime.GOOS != "darwin" {
		return "", false
	}
	for _, c := range []string{"prlctl", "/usr/local/bin/prlctl",
		"/Applications/Parallels Desktop.app/Contents/MacOS/prlctl"} {
		if p, err := exec.LookPath(c); err == nil {
			return p, true
		}
	}
	return "", false
}

// ParallelsGuests lists the running Parallels VMs. Only running ones are
// listed: reading a file needs Parallels Tools in a booted guest, and offering
// a stopped VM as a source would just fail later.
//
// No Parallels on this machine is not a failure — it is the ordinary case, and
// it returns no guests and no error. prlctl being there and refusing to answer
// is a failure, and it has to be one: otherwise a Mac whose Parallels is not
// running is told there is no landscape anywhere, when the file that matters is
// inside the guest that could not be listed.
func ParallelsGuests(ctx context.Context) ([]string, error) {
	bin, ok := prlctlPath()
	if !ok {
		return nil, nil
	}
	out, err := exec.CommandContext(ctx, bin, "list", "--output", "name", "--no-header").Output()
	if err != nil {
		return nil, fmt.Errorf("listing Parallels guests: %w", err)
	}
	var vms []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			vms = append(vms, name)
		}
	}
	return vms, nil
}

// ParallelsLandscapeFiles asks a guest where SAP GUI wrote its landscape.
// Every user profile is checked, because the account running the guest is not
// necessarily the one that configured SAP Logon.
func ParallelsLandscapeFiles(ctx context.Context, vm string) ([]string, error) {
	bin, ok := prlctlPath()
	if !ok {
		return nil, nil
	}
	// One command rather than a listing plus a probe per profile: the guest
	// round-trip is the expensive part.
	const script = `for /d %u in (C:\Users\*) do @if exist "%u\AppData\Roaming\SAP\Common\` +
		landscapeFileName + `" echo %u\AppData\Roaming\SAP\Common\` + landscapeFileName

	out, err := exec.CommandContext(ctx, bin, "exec", vm, "cmd", "/c", script).Output()
	if err != nil {
		// A running guest that cannot be asked is not a guest with no
		// landscape. Dropping it left the scan listing every other source and
		// nothing at all about this one.
		return nil, fmt.Errorf("asking %q where SAP GUI keeps its landscape: %w (is Parallels Tools installed?)", vm, err)
	}
	var found []string
	for _, line := range strings.Split(string(out), "\n") {
		if p := strings.TrimSpace(line); strings.HasPrefix(p, "C:\\") {
			found = append(found, p)
		}
	}
	return found, nil
}

// ReadParallelsFile reads a file out of a running guest.
func ReadParallelsFile(ctx context.Context, vm, winPath string) ([]byte, error) {
	bin, ok := prlctlPath()
	if !ok {
		return nil, fmt.Errorf("reading %s from %q: Parallels is not installed here", winPath, vm)
	}
	out, err := exec.CommandContext(ctx, bin, "exec", vm, "cmd", "/c", "type \""+winPath+"\"").Output()
	if err != nil {
		return nil, fmt.Errorf("reading %s from %q: %w (is the VM running, with Parallels Tools?)", winPath, vm, err)
	}
	return out, nil
}

// WindowsPathFromIncludeURL turns the URL form a landscape uses for an include
// back into the Windows path a guest can open. A company landscape is included
// as file://server/share/SAPUILandscape.XML, which is a UNC path spelled as a
// URL; reading it only works from inside the machine that can see the share.
func WindowsPathFromIncludeURL(raw string) (string, bool) {
	rest, ok := strings.CutPrefix(raw, "file://")
	if !ok {
		return "", false
	}
	rest = strings.ReplaceAll(rest, "/", "\\")

	// file:///C:/dir/file → \C:\dir\file, a local path with a stray leading
	// separator rather than a host.
	if strings.HasPrefix(rest, "\\") && len(rest) > 3 && rest[2] == ':' {
		return rest[1:], true
	}
	if rest == "" {
		return "", false
	}
	return "\\\\" + rest, true
}
