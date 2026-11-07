package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// keyService is where the key lives in the Keychain, whichever way the app
// runs: `go run`, `mygo dev` and the build share it.
const keyService = "io.starchaser.telepath.key"

// keychain keeps the key through /usr/bin/security rather than the
// Security framework. The item is then the tool's, which reads it for
// every build without asking; one the app makes is bound to the build that
// made it, as macOS knows an app without a Team ID by its cdhash, so each
// build asked again, Always Allow or not. In exchange any process of the
// user can read it the same way.
type keychain struct{ service string }

const securityTool = "/usr/bin/security"

func (k keychain) Get(name string) ([]byte, bool, error) {
	out, err := exec.Command(securityTool, "find-generic-password", "-s", k.service, "-a", name, "-w").Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 44 { // errSecItemNotFound
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading the key: %w", err)
	}
	// Stored as base64: the tool shows binary data as hex, and takes it
	// only as text.
	v, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	return v, err == nil, err
}

func (k keychain) Set(name string, v []byte) error {
	// Through the tool's own prompt (-i), so the key isn't in any
	// process's arguments; it reports success whatever the command did, so
	// the key is read back.
	cmd := exec.Command(securityTool, "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -s %q -a %q -w %q\n",
		k.service, name, base64.StdEncoding.EncodeToString(v)))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("storing the key: %w: %s", err, out)
	}
	got, ok, err := k.Get(name)
	if err != nil || !ok || !bytes.Equal(got, v) {
		return fmt.Errorf("storing the key: not there after (%v)", err)
	}
	return nil
}
