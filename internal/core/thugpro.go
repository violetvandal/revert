package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// SetupThugPro makes the online (THUG Pro) lane one action: get the official THUG Pro
// installer and launch it. THUG Pro's own setup then asks for your clean THUG2 folder,
// downloads the full ~500 MB build, and installs to %LOCALAPPDATA%\THUG Pro — after which
// `revert run online` finds and launches it.
//
// The installer is resolved in this order: a bring-your-own / bundled copy at THUGPRO_SETUP
// (presence wins), else downloaded from THUGPRO_SETUP_URL. THUG Pro is a third-party
// community app, so Revert never bundles the full build — it only fetches the small public
// installer stub and hands off.
func SetupThugPro(c *Conf) error {
	installer, err := resolveThugProInstaller(c)
	if err != nil {
		return err
	}

	fmt.Println("[revert] launching the THUG Pro installer.")
	fmt.Println("         When it asks for your THUG2 game folder, pick a CLEAN copy of THUG2")
	fmt.Println("         (not the modded edition). THUG Pro reads those files at runtime, so")
	fmt.Println("         point it at a THUG2 you keep — e.g. your own install folder.")
	if base := c.Path("PRISTINE_DIR"); dirExists(base) {
		fmt.Printf("         A clean base also lives here: %s\n", base)
		fmt.Println("         (usable, but it is removed if you later run `revert uninstall`).")
	}

	// A GUI installer the user clicks through: start it and return, rather than blocking the
	// rest of setup. The child keeps running after this process exits.
	cmd := exec.Command(installer)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not launch the THUG Pro installer (%s): %w", installer, err)
	}
	ok("THUG Pro installer opened — follow its prompts, then: revert run online")
	return nil
}

// resolveThugProInstaller returns a path to a THUGProSetup.exe to run. A bring-your-own /
// bundled copy at THUGPRO_SETUP wins; otherwise the installer is downloaded from
// THUGPRO_SETUP_URL to a scratch file in the install root. The scratch file is left in
// place (it is tiny, a re-run overwrites it, and `revert uninstall` clears it) rather than
// deleted underneath the GUI the user is still clicking through.
func resolveThugProInstaller(c *Conf) (string, error) {
	if byo := c.Path("THUGPRO_SETUP"); fileExists(byo) {
		fmt.Printf("[revert] using bundled THUG Pro installer: %s\n", byo)
		return byo, nil
	}

	url := c.GetOr("THUGPRO_SETUP_URL", "")
	if url == "" {
		return "", fmt.Errorf("no THUG Pro installer found (THUGPRO_SETUP) and no THUGPRO_SETUP_URL to download from")
	}
	dst := filepath.Join(c.Root, ".revert-thugpro-setup.exe")
	os.Remove(dst)
	fmt.Printf("[revert] downloading the THUG Pro installer from %s\n", url)
	if err := download(url, dst); err != nil {
		os.Remove(dst)
		return "", fmt.Errorf("downloading the THUG Pro installer from %s: %w", url, err)
	}
	return dst, nil
}
