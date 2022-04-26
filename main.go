// Copyright Josh Komoroske. All rights reserved.
// Use of this source code is governed by the MIT license,
// a copy of which can be found in the LICENSE.txt file.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/drone/drone-go/drone"
	"github.com/google/go-github/v43/github"
	"golang.org/x/oauth2"
)

// templateContext bundles a number of named properties that can be referenced
// in a text/template body.
type templateContext struct {
	BuildNumber int
	DroneServer string
	Labels      map[string]string
	Logs        []string
	PullRequest int
	RepoName    string
	RepoOwner   string
	SHA         string
	StageName   string
	StageNumber int
	Status      string
	StepName    string
	StepNumber  int
}

// version is used to hold the version string. Is replaced at go build time
// with -ldflags.
var version = "development"

// commentTemplateRaw is an embedded file which contains a text/template body.
// Will be used to format logs (and other information) from a DroneCI step as a
// GitHub markdown comment.
//go:embed comment.tpl
var commentTemplateRaw string

// commentTemplate is parsed from the above raw template. Any syntax errors in
// the template body will result in an immediate panic at runtime.
// nolint:gochecknoglobals
var commentTemplate = template.Must(template.New("comment.tpl").Parse(commentTemplateRaw))

// regexMarkdownMetadata is a regex that matches specifically formatted
// invisible markdown comments of the form:
//   [//]: # (key=value)
var regexMarkdownMetadata = regexp.MustCompile(`^\[//\]: # \(([^=]+)=(.*)\)`)

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

	// droneCommitSHA is the git SHA of the commit that started the current
	// DroneCI build.
	// Example: bcdd4bf0245c82c060407b3b24b9b87301d15ac1
	// See: https://docs.drone.io/pipeline/environment/reference/drone-commit-sha/
	droneCommitSHA := os.Getenv("DRONE_COMMIT_SHA")

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

	// pluginKeep determines whether previously posted pull request comments
	// are kept. Defaults to false, which deletes previous comments.
	// Example: true
	pluginKeep := os.Getenv("PLUGIN_KEEP")

	// pluginStage is a named build stage which will be used in conjunction
	// with PLUGIN_STEP to find step logs.
	// Example: build-pull-request
	pluginStage := os.Getenv("PLUGIN_STAGE")

	// pluginStep is a named build step which will be used in conjunction with
	// PLUGIN_STAGE to find step logs.
	// Example: lint-code
	pluginStep := os.Getenv("PLUGIN_STEP")

	// pluginWhen determines if logs from a successful step, or a failing step
	// (or both) should be posted as a comment.
	// Example: success|failure|always
	pluginWhen := os.Getenv("PLUGIN_WHEN")

	// pluginVerbatim determines whether log lines that start with a "+" sign
	// are kept when templating out a comment. Such log lines are commonly seen
	// since step commands are run with set -e. Defaults to false, which drops
	// log lines that start with a "+" sign.
	// Example: true
	pluginVerbatim := os.Getenv("PLUGIN_VERBATIM")

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
	case droneCommitSHA == "":
		return fmt.Errorf("DRONE_COMMIT_SHA was not provided")
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

	// Parse the drone pull request number to ensure that it's valid.
	dronePullRequestInt, err := strconv.Atoi(dronePullRequest)
	if err != nil {
		return err
	}

	// Parse the plugin keep parameter, and default to false on error.
	pluginKeepBool, err := strconv.ParseBool(pluginKeep)
	if err != nil {
		pluginKeepBool = false
	}

	// Parse the plugin verbatim parameter, and default to false on error.
	pluginVerbatimBool, err := strconv.ParseBool(pluginVerbatim)
	if err != nil {
		pluginVerbatimBool = false
	}

	// Shared context for all requests going forward.
	ctx := context.Background()

	// Construct a DroneCI API client using the provided token, proto, and
	// hostname.
	droneTokenClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: droneToken}, // nolint:exhaustivestruct
	))
	droneServer := fmt.Sprintf("%s://%s", droneSystemProto, droneSystemHostname)
	droneClient := drone.NewClient(droneServer, droneTokenClient)

	// Construct a GitHub API client using the provided token.
	githubTokenClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken}, // nolint:exhaustivestruct
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
	stageNumber, stepNumber, status, found := resolveBuildStageAndStep(build, pluginStage, pluginStep)
	if !found {
		// The full set of stage and step names are fully known at DroneCI
		// build time, so this indicates a plugin misconfiguration. Possibly as
		// the result of the target stage or step being renamed.
		return fmt.Errorf("build stage and step could not be found")
	}

	// labels is a set of key value pairs that is embedded into the pull
	// request comment markdown as metadata.
	labels := map[string]string{
		"stage": pluginStage,
		"step":  pluginStep,
	}

	// If keep is set to true, then this can be skipped. No need to bother
	// listing and deleting existing pull request comments.
	if !pluginKeepBool {
		// Get a list of all existing comments. Treat this as optional, so just
		// return without error if listing fails.
		existingComments, _, err := githubClient.Issues.ListComments(ctx, droneRepoOwner, droneRepoName, dronePullRequestInt, nil)
		if err != nil {
			log.Printf("failed to list existing existing")
		}

		// Check every single existing pull request comment, and determine whether
		// it should be deleted.
		for _, existingComment := range existingComments {
			// If the user who posted the comment doesn't match our current user,
			// then skip it.
			if existingComment.GetUser().GetLogin() != githubUser.GetLogin() {
				continue
			}

			// Check if the comment has labels that correspond with this
			// instance of the plugin. This is to prevent improper deletion of
			// comments posted by an additional instance of the plugin being
			// run in the same build pipeline.
			if !hasMarkdownLabels(existingComment.GetBody(), labels) {
				continue
			}

			// Attempt to delete this comment. Treat this as optional, so just
			// continue without error if deleting fails.
			if _, err := githubClient.Issues.DeleteComment(ctx, droneRepoOwner, droneRepoName, existingComment.GetID()); err != nil {
				log.Printf("failed to delete comment %s", existingComment.GetHTMLURL())
				continue
			}

			log.Printf("deleted comment %s", existingComment.GetHTMLURL())
		}
	}

	// It doesn't make sense to take logs from steps that haven't completely
	// finished running. This indicates a plugin misconfiguration, as the
	// plugin should only be run after the target step succeeds or fails.
	if status != drone.StatusPassing && status != drone.StatusFailing {
		return fmt.Errorf("target step status is %s", status)
	}

	// Check if we can skip posting a new comment (also fetching logs and
	// templating said comment) based on the status of the target step.
	if status == drone.StatusPassing && pluginWhen == "failure" {
		// Target step is passing, but we only want to post a comment when it
		// fails, so there is nothing more to do here.
		log.Println("not commenting since step passed")
		return nil
	} else if status == drone.StatusFailing && pluginWhen == "success" {
		// Target step is failing, but we only want to post a comment when it
		// succeeds, so there is nothing more to do here.
		log.Println("not commenting since step failed")
		return nil
	}

	// Fetch logs for the resolved build stage and step.
	log.Printf("fetching logs for %s/%s/%s/%d/%d/%d", droneServer, droneRepoOwner, droneRepoName, droneBuildNumberInt, stageNumber, stepNumber)
	lines, err := droneClient.Logs(droneRepoOwner, droneRepoName, droneBuildNumberInt, stageNumber, stepNumber)
	if err != nil {
		return err
	}

	// Keep just the log message for all the fetched log lines.
	logs := make([]string, 0, len(lines))
	for _, line := range lines {
		log := strings.TrimRight(line.Message, "\n")
		// Check if this log line should be dropped.
		if pluginVerbatimBool || !strings.HasPrefix(log, "+") {
			logs = append(logs, log)
		}
	}

	// Discard any blank leading or trailing log lines.
	logs = trimBlankLogs(logs)

	// Format a GitHub comment body.
	comment, err := templateComment(templateContext{
		BuildNumber: droneBuildNumberInt,
		DroneServer: droneServer,
		Labels:      labels,
		Logs:        logs,
		PullRequest: dronePullRequestInt,
		RepoName:    droneRepoName,
		RepoOwner:   droneRepoOwner,
		SHA:         droneCommitSHA,
		StageName:   pluginStage,
		StageNumber: stageNumber,
		Status:      status,
		StepName:    pluginStep,
		StepNumber:  stepNumber,
	})
	if err != nil {
		return err
	}
	log.Printf("templated comment:\n%s", comment)

	// Create a comment on the underlying pull request.
	// nolint:exhaustivestruct
	createdComment, _, err := githubClient.Issues.CreateComment(ctx, droneRepoOwner, droneRepoName, dronePullRequestInt, &github.IssueComment{
		Body: github.String(comment),
	})
	if err != nil {
		return err
	}
	log.Printf("created comment %s", createdComment.GetHTMLURL())

	return nil
}

// hasMarkdownLabels checks if the given comment contains all the given
// labels as markdown metadata. A markdown label is an invisible comment that
// takes the form:
//   [//]: # (key=value)
func hasMarkdownLabels(comment string, labels map[string]string) bool {
	// extracted is a set of key value pairs that were found inside the
	// markdown comment.
	extracted := make(map[string]string)

	// Scan the input comment one line at a time.
	for scanner := bufio.NewScanner(strings.NewReader(comment)); scanner.Scan(); {
		line := scanner.Text()

		// Attempt to match the current line against the markdown metadata
		// regex. If there are exactly three matches (the whole string, the
		// key, and the value) then add the key and value to our set of
		// extracted pairs.
		match := regexMarkdownMetadata.FindStringSubmatch(line)
		if len(match) == 3 { // nolint:gomnd
			key := strings.TrimSpace(match[1])
			val := strings.TrimSpace(match[2])
			extracted[key] = val
		}
	}

	// Check that the given set of labels is a subset of the labels extracted
	// from the given comment.
	for labelKey, labelVal := range labels {
		if extractedVal, ok := extracted[labelKey]; !ok || extractedVal != labelVal {
			// Either one of the given keys wasn't found, or the value at that
			// key did not match.
			return false
		}
	}

	return true
}

// resolveBuildStageAndStep takes a named build stage and a named build step
// and resolved them into a stage number and a step number.
func resolveBuildStageAndStep(build *drone.Build, stageName, stepName string) (int, int, string, bool) {
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
			return stage.Number, step.Number, step.Status, true
		}

		// Since stage names are unique, and we have already examined a step
		// with a matching name, there is nothing more to do here. Break out
		// and let the final failure case handle it.
		break
	}

	// The names stage and step could not be found.
	return 0, 0, "", false
}

// templateComment takes a set of typed parameters and formats a GitHub comment
// body using a template.
func templateComment(params templateContext) (string, error) {
	buf := bytes.Buffer{}
	err := commentTemplate.Execute(&buf, params)
	return buf.String(), err
}

// trimBlankLogs discards any blank leading or trailing log lines. Blank lines
// surrounded by non-blank lines are kept intact.
func trimBlankLogs(logs []string) []string {
	// Starting from the front, discard any lines that are completely blank.
	// Break out when the first non-blank line is found.
	for len(logs) > 0 {
		if strings.TrimSpace(logs[0]) != "" {
			break
		}
		// Discard the first item.
		logs = logs[1:]
	}

	// Starting from the back, discard any lines that are completely blank.
	// Break out when the first (actually the last) non-blank line is found.
	for len(logs) > 0 {
		if strings.TrimSpace(logs[len(logs)-1]) != "" {
			break
		}
		// Discard the last item.
		logs = logs[:len(logs)-1]
	}

	// Return whatever remains
	return logs
}
