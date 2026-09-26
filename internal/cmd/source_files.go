package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// alwaysTransferred are the build files FrankenDeploy generates: they must
// reach the build even when the project ignores them in Git.
var alwaysTransferred = []string{"Dockerfile", "docker-entrypoint.sh", ".dockerignore", "Caddyfile"}

// buildOutputDirs are ignored by Git in a standard Symfony project but only
// hold assets derived from vendor/ or the importmap, never user data. They
// were always shipped and some Dockerfiles rely on them (bundle assets are
// not rebuilt by the image), so they keep being transferred when present.
var buildOutputDirs = []string{"public/bundles", "assets/vendor"}

// gitFiles runs git in dir and returns the NUL-separated paths it prints,
// relative to dir. ok is false when git is missing or dir is not in a
// repository.
func gitFiles(ctx context.Context, dir string, args ...string) (files []string, ok bool) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, false
	}
	for _, f := range strings.Split(stdout.String(), "\x00") {
		if f != "" {
			files = append(files, filepath.ToSlash(f))
		}
	}
	return files, true
}

// sourceFiles lists the files to send to the remote build: what Git
// considers part of the project (committed files and new files not ignored
// by .gitignore), plus the generated build files and the build output dirs.
// It returns nil when the project is not a Git repository: the caller then
// falls back to walking the directory with sourceCodeExcludes.
func sourceFiles(ctx context.Context, dir string) []string {
	listed, ok := gitFiles(ctx, dir, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if !ok {
		return nil
	}

	seen := make(map[string]bool)
	var files []string
	add := func(rel string) {
		if seen[rel] {
			return
		}
		// Committed files deleted locally are still listed by --cached
		if info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(rel))); err == nil && info.Mode().IsRegular() {
			seen[rel] = true
			files = append(files, rel)
		}
	}

	for _, f := range listed {
		add(f)
	}
	for _, f := range alwaysTransferred {
		add(f)
	}
	for _, d := range buildOutputDirs {
		_ = filepath.WalkDir(filepath.Join(dir, filepath.FromSlash(d)), func(p string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if rel, relErr := filepath.Rel(dir, p); relErr == nil {
				add(filepath.ToSlash(rel))
			}
			return nil
		})
	}

	sort.Strings(files)
	if files == nil {
		files = []string{}
	}
	return files
}

// seedableSharedFiles returns, for each shared directory, the files
// committed to Git without local changes (paths relative to that directory).
// Only those may initialize an empty shared directory on the server: a file
// ignored by Git (a local database, test uploads) or modified locally must
// never reach production that way. modified lists the committed files left
// out because of local changes.
func seedableSharedFiles(ctx context.Context, dir string, sharedDirs []string) (seeds map[string][]string, modified []string) {
	seeds = make(map[string][]string)
	for _, shared := range sharedDirs {
		tracked, ok := gitFiles(ctx, dir, "ls-files", "-z", "--", shared)
		if !ok || len(tracked) == 0 {
			continue
		}
		changed, _ := gitFiles(ctx, dir, "ls-files", "-z", "--modified", "--", shared)
		isChanged := make(map[string]bool, len(changed))
		for _, f := range changed {
			isChanged[f] = true
		}

		prefix := strings.TrimSuffix(filepath.ToSlash(shared), "/") + "/"
		for _, f := range tracked {
			if isChanged[f] {
				modified = append(modified, f)
				continue
			}
			if rel := strings.TrimPrefix(f, prefix); rel != f {
				seeds[shared] = append(seeds[shared], rel)
			}
		}
	}
	return seeds, modified
}

// seedSharedDirs initializes each EMPTY shared directory on the server with
// its committed files, copied from the new image. A shared directory is
// mounted over the image's one, so without this the files shipped in the
// project (the Symfony Demo's SQLite database) are hidden and the app starts
// on an empty directory. A directory that already holds anything is never
// touched: production data always wins.
func seedSharedDirs(ctx context.Context, client ssh.Executor, imageName, sharedPath string, seeds map[string][]string, modified []string) {
	dirs := make([]string, 0, len(seeds))
	for d := range seeds {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	for _, d := range dirs {
		target := sharedPath + "/" + d
		result, err := client.Exec(ctx, fmt.Sprintf(`test -z "$(ls -A %s 2>/dev/null)"`, security.ShellEscape(target)))
		if err != nil || result.ExitCode != 0 {
			continue // already holds data
		}

		quoted := make([]string, len(seeds[d]))
		for i, f := range seeds[d] {
			quoted[i] = security.ShellEscape(f)
		}
		// Prints how many files were copied: a committed file may be missing
		// from the image (excluded by .dockerignore)
		script := fmt.Sprintf(`cd %s && n=0 && for f in %s; do if [ -e "$f" ]; then cp -a --parents -- "$f" /seed/ && n=$((n+1)); fi; done; echo "$n"`,
			security.ShellEscape("/app/"+d), strings.Join(quoted, " "))
		seedCmd := fmt.Sprintf("docker run --rm --user %s --entrypoint sh -v %s:/seed %s -c %s",
			constants.ContainerUser, security.ShellEscape(target), imageName, security.ShellEscape(script))

		result, err = client.Exec(ctx, seedCmd)
		if err == nil {
			err = result.Err()
		}
		if err != nil {
			PrintWarning("Could not initialize shared %s/ from the image: %v", d, err)
			continue
		}
		copied, _ := strconv.Atoi(strings.TrimSpace(result.Stdout))
		if copied == 0 {
			continue
		}
		PrintInfo("Initialized shared %s/ with %d file(s) committed to Git", d, copied)

		for _, f := range modified {
			if strings.HasPrefix(f, strings.TrimSuffix(d, "/")+"/") {
				PrintWarning("%s has local changes: not copied to the server (commit it to ship it)", f)
			}
		}
	}
}
