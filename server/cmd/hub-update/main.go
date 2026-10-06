// Command hub-update installs a new version of the Mac app's bundle: it
// waits for the app to quit, stops the server, swaps the bundle, starts the
// server and checks it. It ships in the bundle beside hub-server; until the
// app installs versions itself, it only reports its version.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/tonypine/job-search-hub/server/internal/buildinfo"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(stdout, "hub-update", buildinfo.Version())
		return 0
	}
	fmt.Fprintln(stderr, "hub-update doesn't install versions yet; install a build with macos/Scripts/install-app.sh.")
	fmt.Fprintln(stderr, "usage: hub-update --version")
	return 2
}
