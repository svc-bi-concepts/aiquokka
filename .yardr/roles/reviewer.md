Read /Users/bsaladin/ws_yardr/.yardr/roles/reviewer.md first.

## aiquokka depot

- Review branches against `origin/main` and the bead's `it-ship` stage.
- Run the depot gate on the commit you finish with: `sh .yardr/check`.
- This gate runs `go vet ./... && go test ./...` and checks branch readiness
  on bead runs.
- Completed reviews move through `yardr bead done <id>` to the next stage.
- Approved work lands through the `yard/landers` group, which runs
  `/Users/bsaladin/ws_yardr/integrations/infra-land.sh`.
- Check that provider credentials remain read-only and that no change refreshes
  or writes credential files; see README.md for provider-specific sources.
