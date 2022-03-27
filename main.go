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
	"strconv"
	"strings"

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

	// droneBuildNumber is the number for the current DroneCI build.
	// Example: 42
	// See: https://docs.drone.io/pipeline/environment/reference/drone-build-number/
	droneBuildNumber := os.Getenv("DRONE_BUILD_NUMBER")

	// dronePullRequest is the pull request number that this build is running
	// on behalf of. Only present in build from the pull_request event.
	// Example: 123
	// See: https://docs.drone.io/pipeline/environment/reference/drone-pull-request/
	dronePullRequest := os.Getenv("DRONE_PULL_REQUEST")

	// droneRepoName is the name of the repository itself.
	// Example: hello-world
	// See: https://docs.drone.io/pipeline/environment/reference/drone-repo-name/
	droneRepoName := os.Getenv("DRONE_REPO_NAME")

	// droneRepoOwner is the name of the repository owner. This could be an
	// individual or an organization.
	// Example: octocat
	// See: https://docs.drone.io/pipeline/environment/reference/drone-repo-owner/
	droneRepoOwner := os.Getenv("DRONE_REPO_OWNER")

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

	// pluginStage is a named build stage which will be used in conjunction
	// with PLUGIN_STEP to find step logs.
	// Example: build-pull-request
	pluginStage := os.Getenv("PLUGIN_STAGE")

	// pluginStep is a named build step which will be used in conjunction with
	// PLUGIN_STAGE to find step logs.
	// Example: lint-code
	pluginStep := os.Getenv("PLUGIN_STEP")

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
	case droneBuildNumber == "":
		return fmt.Errorf("DRONE_BUILD_NUMBER was not provided")
	case droneRepoName == "":
		return fmt.Errorf("DRONE_REPO_NAME was not provided")
	case droneRepoOwner == "":
		return fmt.Errorf("DRONE_REPO_OWNER was not provided")
	case droneSystemProto == "":
		return fmt.Errorf("DRONE_SYSTEM_PROTO was not provided")
	case droneSystemHostname == "":
		return fmt.Errorf("DRONE_SYSTEM_HOSTNAME was not provided")
	case droneToken == "":
		return fmt.Errorf("DRONE_TOKEN was not provided")
	case githubToken == "":
		return fmt.Errorf("GITHUB_TOKEN was not provided")
	case pluginStage == "":
		return fmt.Errorf("PLUGIN_STAGE was not provided")
	case pluginStep == "":
		return fmt.Errorf("PLUGIN_STEP was not provided")
	}

	// Parse the drone build number to ensure that it's valid.
	droneBuildNumberInt, err := strconv.Atoi(droneBuildNumber)
	if err != nil {
		return err
	}

	// Shared context for all requests going forward.
	ctx := context.Background()

	// Construct a DroneCI API client using the provided token, proto, and
	// hostname.
	droneTokenClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: droneToken},
	))
	droneServer := fmt.Sprintf("%s://%s", droneSystemProto, droneSystemHostname)
	droneClient := drone.NewClient(droneServer, droneTokenClient)

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

	// Fetch metadata for the current build.
	log.Printf("fetching build for %s/%s/%s/%d", droneServer, droneRepoOwner, droneRepoName, droneBuildNumberInt)
	build, err := droneClient.Build(droneRepoOwner, droneRepoName, droneBuildNumberInt)
	if err != nil {
		return err
	}

	// Resolve the named build stage and step into a stage and step number.
	// These names are only a convenience for the plugin, the stage and step
	// numbers is what the DroneCI API actually needs to fetch step logs.
	log.Printf("searching for stage %s step %s", pluginStage, pluginStep)
	stageNumber, stepNumber, found := resolveBuildStageAndStep(build, pluginStage, pluginStep)
	if !found {
		// The full set of stage and step names are fully known at DroneCI
		// build time, so this indicates a plugin misconfiguration. Possibly as
		// the result of the target stage or step being renamed.
		return fmt.Errorf("build stage and step could not be found")
	}

	// Fetch logs for the resolved build stage and step.
	log.Printf("fetching logs for %s/%s/%s/%d/%d/%d", droneServer, droneRepoOwner, droneRepoName, droneBuildNumberInt, stageNumber, stepNumber)
	lines, err := droneClient.Logs(droneRepoOwner, droneRepoName, droneBuildNumberInt, stageNumber, stepNumber)
	if err != nil {
		return err
	}

	// Only print log lines for now.
	for _, line := range lines {
		log.Println("| ", strings.TrimRight(line.Message, "\n"))
	}

	return nil
}

// resolveBuildStageAndStep takes a named build stage and a named build step
// and resolved them into a stage number and a step number.
func resolveBuildStageAndStep(build *drone.Build, stageName, stepName string) (int, int, bool) {
	for _, stage := range build.Stages {
		// If the current stage name doesn't match, move onto the next stage.
		if stage.Name != stageName {
			continue
		}

		for _, step := range stage.Steps {
			// If the current step name doesn't match, move onto the next step.
			if step.Name != stepName {
				continue
			}
			// The names step and stage were found! Return the resolved stage
			// and step numbers.
			return stage.Number, step.Number, true
		}

		// Since stage names are unique, and we have already examined a step
		// with a matching name, there is nothing more to do here. Break out
		// and let the final failure case handle it.
		break
	}

	// The names stage and step could not be found.
	return 0, 0, false
}
