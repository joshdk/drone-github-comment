// Copyright Josh Komoroske. All rights reserved.
// Use of this source code is governed by the MIT license,
// a copy of which can be found in the LICENSE.txt file.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"log"
	"os"
)

// version is used to hold the version string. Is replaced at go build time
// with -ldflags.
var version = "development"

func main() {
	if err := mainCmd(); err != nil {
		fmt.Fprintln(os.Stderr, "joshdk/drone-github-comment:", err)
		os.Exit(1)
	}
}

func mainCmd() error {
	log.Printf("joshdk/drone-github-comment version %s", version)

	return nil
}
