{{if eq .Status "success"}}✅{{else}}❌{{end}} Build output from step [{{.StepName}}]({{.DroneServer}}/{{.RepoOwner}}/{{.RepoName}}/{{.BuildNumber}}/{{.StageNumber}}/{{.StepNumber}}) on commit [{{ slice .SHA  0 7}}](https://github.com/{{.RepoOwner}}/{{.RepoName}}/pull/{{.PullRequest}}/commits/{{.SHA}}):

```text
{{- range .Logs}}
{{.}}
{{- end}}
```
