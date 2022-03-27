// Copyright Josh Komoroske. All rights reserved.
// Use of this source code is governed by the MIT license,
// a copy of which can be found in the LICENSE.txt file.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/drone/drone-go/drone"
	"github.com/google/go-github/v43/github"
	"golang.org/x/oauth2"
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

	// dronePullRequest is the pull request number that this build is running
	// on behalf of. Only present in build from the pull_request event.
	// Example: 123
	// See: https://docs.drone.io/pipeline/environment/reference/drone-pull-request/
	dronePullRequest := os.Getenv("DRONE_PULL_REQUEST")

	// droneSystemProto is the HTTP protocol used by the DroneCI API server.
	// Should only be "http" or "https". Combined with DRONE_SYSTEM_HOSTNAME to
	// form the complete server address.
	// Example: http
	// See: https://docs.drone.io/pipeline/environment/reference/drone-system-proto/
	droneSystemProto := os.Getenv("DRONE_SYSTEM_PROTO")

	// droneSystemHostname is the DNS hostname used by the DroneCI API server.
	// Combined with DRONE_SYSTEM_PROTO to form the complete server address.
	// Example: drone.mycompany.com
	// See: https://docs.drone.io/pipeline/environment/reference/drone-system-hostname/
	droneSystemHostname := os.Getenv("DRONE_SYSTEM_HOSTNAME")

	// droneToken is a personal token that can be used to authenticate against
	// the DroneCI API.
	// Example: hmNo...Yy8x
	// See: https://docs.drone.io/cli/configure/
	droneToken := os.Getenv("DRONE_TOKEN")

	// githubToken is a personal access token that can be used to authenticate
	// against the GitHub API.
	// Example: ghp_cviM...Rbxg
	// See: https://github.com/settings/tokens
	githubToken := os.Getenv("GITHUB_TOKEN")

	// Validate that various settings are not empty. Ordered roughly by what
	// things that are most important to flag first. For example, validating
	// that vital build metadata is even present comes before validating that
	// secrets are configured.
	switch {
	case dronePullRequest == "":
		// Current build is not associated with a pull request (such as a build
		// for a branch or tag). It's not possible to leave a comment on a pull
		// request if there is no pull request, so there's nothing to do here.
		// In this specific case, just log a message and exit.
		log.Printf("exiting as build is not for a pull request")
		return nil
	case droneSystemProto == "":
		return fmt.Errorf("DRONE_SYSTEM_PROTO was not provided")
	case droneSystemHostname == "":
		return fmt.Errorf("DRONE_SYSTEM_HOSTNAME was not provided")
	case droneToken == "":
		return fmt.Errorf("DRONE_TOKEN was not provided")
	case githubToken == "":
		return fmt.Errorf("GITHUB_TOKEN was not provided")
	}

	// Shared context for all requests going forward.
	ctx := context.Background()

	// Construct a DroneCI API client using the provided token, proto, and
	// hostname.
	droneTokenClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: droneToken},
	))
	droneClient := drone.NewClient(
		fmt.Sprintf("%s://%s", droneSystemProto, droneSystemHostname),
		droneTokenClient,
	)

	// Construct a GitHub API client using the provided token.
	githubTokenClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	))
	githubClient := github.NewClient(githubTokenClient)

	// Fetch the current DroneCI user as a sanity check.
	droneUser, err := droneClient.Self()
	if err != nil {
		return err
	}
	log.Printf("authenticated as drone user %s", droneUser.Login)

	// Fetch the current GitHub user as a sanity check. The name of this user
	// will be used later on for filtering pull request comments.
	githubUser, _, err := githubClient.Users.Get(ctx, "")
	if err != nil {
		return err
	}
	log.Printf("authenticated as github user %s", githubUser.GetLogin())

	return nil
}
