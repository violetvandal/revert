// Package core is the shared, cross-platform heart of the Revert toolkit: it reads
// revert.conf and orchestrates the edition lifecycle (doctor/status/build/run/setup/
// acquire) for the `revert` CLI and the GUI alike.
//
// On Windows this package IS the front door — THUG2 runs natively, so the whole Wine
// layer (GE-Proton, prefixes, DXVK, wineserver, the pad-mirror) evaporates and the
// commands are implemented natively here. On Linux/Steam Deck the proven bash front
// door (`revert` + share/*/*.sh) stays authoritative, and these commands simply
// delegate to it (see delegate.go) so the validated Deck path is never touched.
package core

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Conf is revert.conf flattened to KEY=VALUE with ${VAR} expanded. It deliberately
// mirrors the tiny KEY=VALUE parser thugkit uses on the same file (apply.readConf),
// adding two things the shared config needs on Windows: ${VAR}/${VAR:-default}
// expansion (revert.conf is full of ${REVERT_ROOT}/...), and skipping the shell-only
// lines (the Wine host-detection if/else block, `export PATH`, the conf.local source)
// that only the bash dispatcher cares about.
type Conf struct {
	m    map[string]string
	Root string
}

// A config line is a bare KEY=VALUE at column 0. Anything indented (the if/else GE_DIR
// bodies), or starting with a keyword (if/fi/else/export/source/command/[[), is
// shell-only and skipped. Command substitution `$(...)` likewise marks a shell line.
var (
	keyLineRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	varRefRe  = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-[^}]*)?\}`)
)

// LoadConf parses revert.conf at path, expanding against REVERT_ROOT=root, the process
// environment, and earlier keys (so later values can reference earlier ones).
func LoadConf(path, root string) (*Conf, error) {
	c := &Conf{m: map[string]string{"REVERT_ROOT": root}, Root: root}
	if err := c.parseFile(path); err != nil {
		return nil, err
	}
	return c, nil
}

// Overlay merges another KEY=VALUE file over c, later keys winning. This is how
// revert.conf.local works: revert.conf is toolkit-owned and replaced wholesale by
// `revert update`, while the .local overlay is the user's and survives. The bash
// dispatcher gets this by sourcing the file last; we have to do it explicitly, because
// the parser skips that `source` line along with every other shell-only construct.
func (c *Conf) Overlay(path string) error { return c.parseFile(path) }

func (c *Conf) parseFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue // blank, indented (shell block body), or comment
		}
		if strings.Contains(line, "$(") {
			continue // command substitution -> shell-only
		}
		mt := keyLineRe.FindStringSubmatch(line)
		if mt == nil {
			continue // if/fi/else/export/source/[[ ...
		}
		key := mt[1]
		val := c.expand(stripQuotes(strings.TrimSpace(mt[2])))
		c.m[key] = val
	}
	return sc.Err()
}

// Get returns the value for key ("" if unset).
func (c *Conf) Get(key string) string { return c.m[key] }

// GetOr returns the value for key, or def if unset/empty.
func (c *Conf) GetOr(key, def string) string {
	if v, ok := c.m[key]; ok && v != "" {
		return v
	}
	return def
}

// Path returns key's value as a native filesystem path (forward slashes in the shared
// conf are normalized to the OS separator).
func (c *Conf) Path(key string) string {
	v := c.m[key]
	if v == "" {
		return ""
	}
	return filepath.Clean(filepath.FromSlash(v))
}

// Thugkit resolves the thugkit build-core binary path, appending .exe on Windows.
func (c *Conf) Thugkit() string {
	p := c.Path("THUGKIT")
	if p == "" {
		p = filepath.Join(c.Root, "tools", "thugkit", "thugkit")
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(p), ".exe") {
		p += ".exe"
	}
	return p
}

func (c *Conf) lookup(name string) (string, bool) {
	if v, ok := c.m[name]; ok {
		return v, true
	}
	if v, ok := os.LookupEnv(name); ok {
		return v, true
	}
	switch name { // cross-platform fallbacks for the vars revert.conf references
	case "HOME":
		if h, err := os.UserHomeDir(); err == nil {
			return h, true
		}
	case "USER":
		if u := os.Getenv("USERNAME"); u != "" {
			return u, true
		}
	}
	return "", false
}

// expand resolves ${VAR} and ${VAR:-default} references.
func (c *Conf) expand(s string) string {
	return varRefRe.ReplaceAllStringFunc(s, func(ref string) string {
		sub := varRefRe.FindStringSubmatch(ref)
		name, def := sub[1], ""
		if sub[2] != "" {
			def = strings.TrimPrefix(sub[2], ":-")
		}
		if v, ok := c.lookup(name); ok && v != "" {
			return v
		}
		return def
	})
}

// stripQuotes removes a single matching pair of surrounding single/double quotes.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
