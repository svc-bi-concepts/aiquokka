Read $YARDR_HOME/roles/builder.md first (the installed master; YARDR_HOME is set in your environment).

## aiquokka depot

- Work on branch `yard/<bead>` from `origin/main`; do not edit the depot base.
- Run the depot gate after your last commit: `sh .yardr/check`.
- This gate runs `go vet ./... && go test ./...` and checks branch readiness
  on bead runs.
- The flow is `it-ship`; hand completed work to review with
  `yardr bead done <id>`.
- Approved work lands through the `yard/landers` group, which runs the yard's
  `integrations/infra-land.sh`.
- Treat provider credentials as read-only. Never refresh or write credential
  files; see README.md for provider-specific credential sources.
