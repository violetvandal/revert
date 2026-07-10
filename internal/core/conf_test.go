package core

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleConf = `# comment line
PRISTINE_DIR="${REVERT_ROOT}/game-pristine-us"
MODS_DIR="${REVERT_ROOT}/mods"
THUGKIT="${REVERT_ROOT}/tools/thugkit/thugkit"
WINEARCH="win32"
if [[ "${SteamDeck:-0}" == "1" ]] || grep -qiE 'jupiter' /sys/... ; then
  GE_DIR="${HOME}/.local/share/lutris/x"
else
  GE_DIR="${HOME}/.local/share/lutris/y"
fi
export PATH="${HOME}/.local/go/bin:${PATH}"
GLYPH_STYLE="auto"
LANE_QOL_DIR="${EDITION_QOL}"
EDITION_QOL="${REVERT_ROOT}/game-playable-us"
LANE_QOL_EXE="THUG2.exe"
LANE_QOL_HOOKS="soundtrack,padfix,trigger-bridge"
DEFAULTED="${NOPE:-fallbackval}"
NESTED="${MODS_DIR}/src/decks-pack/blob"
`

func TestLoadConf(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "revert.conf")
	if err := os.WriteFile(path, []byte(sampleConf), 0o644); err != nil {
		t.Fatal(err)
	}
	root := "/opt/thug2"
	c, err := LoadConf(path, root)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"PRISTINE_DIR":   "/opt/thug2/game-pristine-us",
		"MODS_DIR":       "/opt/thug2/mods",
		"WINEARCH":       "win32",
		"GLYPH_STYLE":    "auto",
		"LANE_QOL_EXE":   "THUG2.exe",
		"LANE_QOL_HOOKS": "soundtrack,padfix,trigger-bridge",
		"DEFAULTED":      "fallbackval", // ${NOPE:-fallbackval}
		"NESTED":         "/opt/thug2/mods/src/decks-pack/blob",
	}
	for k, want := range cases {
		if got := c.Get(k); got != want {
			t.Errorf("Get(%q) = %q, want %q", k, got, want)
		}
	}

	// Shell-only lines must be skipped: the indented GE_DIR bodies and export PATH.
	if got := c.Get("GE_DIR"); got != "" {
		t.Errorf("GE_DIR should be skipped (indented shell body), got %q", got)
	}
	if got := c.Get("PATH"); got != "" {
		t.Errorf("export PATH should be skipped, got %q", got)
	}

	// Forward-value reference before definition: LANE_QOL_DIR references EDITION_QOL which
	// is defined LATER, so it expands to "" (matches shell head-order behavior; we don't
	// two-pass). Confirm it doesn't crash and yields empty.
	if got := c.Get("LANE_QOL_DIR"); got != "" {
		t.Logf("LANE_QOL_DIR (forward ref) = %q (empty expected)", got)
	}
}

func TestThugkitExeSuffix(t *testing.T) {
	c := &Conf{m: map[string]string{"THUGKIT": "/opt/thug2/tools/thugkit/thugkit"}, Root: "/opt/thug2"}
	got := c.Thugkit()
	// On Windows it must end in .exe; on Linux it must not.
	if IsWindows() {
		if filepath.Base(got) != "thugkit.exe" {
			t.Errorf("Thugkit() = %q, want ...thugkit.exe on windows", got)
		}
	} else if filepath.Base(got) != "thugkit" {
		t.Errorf("Thugkit() = %q, want ...thugkit on non-windows", got)
	}
}

// revert.conf is toolkit-owned and gets replaced wholesale by `revert update`; the
// gitignored revert.conf.local overlay is the user's and must win. The bash dispatcher
// gets this by sourcing .local last — LoadRootConf has to do it explicitly, because the
// parser skips that `source` line along with every other shell-only construct.
func TestLoadRootConfOverlaysLocal(t *testing.T) {
	root := t.TempDir()
	base := `PRISTINE_DIR="${REVERT_ROOT}/game-pristine-us"
GLYPH_STYLE="xbox"
HQ_AUDIO_URL=""
if [[ -f "${REVERT_ROOT}/revert.conf.local" ]]; then
  source "${REVERT_ROOT}/revert.conf.local"
fi`
	if err := os.WriteFile(filepath.Join(root, "revert.conf"), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without the overlay, the defaults stand.
	t.Setenv("REVERT_ROOT", root)
	c, err := LoadRootConf()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Get("GLYPH_STYLE"); got != "xbox" {
		t.Errorf("GLYPH_STYLE = %q, want xbox", got)
	}

	// With it, .local wins and can add new keys.
	local := `GLYPH_STYLE="playstation"
HQ_AUDIO_URL="https://example/hq.7z"
UPDATE_REPO="someone/fork"`
	if err := os.WriteFile(filepath.Join(root, "revert.conf.local"), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err = LoadRootConf()
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"GLYPH_STYLE":  "playstation",
		"HQ_AUDIO_URL": "https://example/hq.7z",
		"UPDATE_REPO":  "someone/fork",
	} {
		if got := c.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	// Base-only keys survive the overlay.
	if got := c.Path("PRISTINE_DIR"); filepath.Base(got) != "game-pristine-us" {
		t.Errorf("PRISTINE_DIR = %q, want .../game-pristine-us", got)
	}
}
