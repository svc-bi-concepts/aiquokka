# The Z.ai key via Automic Vault (blessed wrapper)

Benchi, 2026-10-15 (D1): the Z.AI key lives in Automic Vault. `aiquokka`
keeps reading `$ZAI_API_KEY`, and the vault injects it; pi's auth store is
not consulted (pi never held the key in a shape aiquokka could read — that
was bead aiquokka-9tzz).

The paste below is run by Benchi on the Mac mini. Builders do not run `av`
themselves: vault operations need Benchi's approval.

## The paste

```sh
## 0. The secret name IT agreed on (still waiting on IT for name/scope)
SECRET_NAME='ZAI_API_KEY'

## 1. Confirm the key is in the vault under that name (names only, no values)
av list
# If IT has not stored it yet, store it (paste the {id}.{secret} key when
# asked; scope: read):
#   av save ZAI_API_KEY

## 2. Move the real binary aside and install the wrapper in its place.
##    The backslash before $@ is load-bearing: the heredoc is unquoted so
##    $SECRET_NAME and $HOME expand now (baking the absolute path in, so the
##    wrapper also works when a launcher does not set HOME); without the
##    backslash $@ would expand at paste time to nothing and the wrapper
##    would drop its arguments.
mv ~/.local/bin/aiquokka ~/.local/bin/aiquokka.bin
cat > ~/.local/bin/aiquokka <<EOF
#!/bin/sh
# blessed wrapper (aiquokka-9tzz, D1): inject ZAI_API_KEY from Automic Vault
exec av inject +$SECRET_NAME -- "$HOME/.local/bin/aiquokka.bin" "\$@"
EOF
chmod +x ~/.local/bin/aiquokka

## 3. Bless the wrapper (one Vault approval). Unattended runs from yardr
##    additionally need the launcher bundle from risk-x942.
av bless --endorse-launcher ~/.local/bin/aiquokka

## 4. Verify — the GLM Coding Plan windows print, the key never does
env -u ZAI_API_KEY aiquokka zai
env -u ZAI_API_KEY aiquokka --json | head -40
```

## Notes

- **Secret name**: `av inject +NAME` exposes the vault secret `NAME` as the
  environment variable `NAME`, so the simplest shape is IT storing the key
  under the name `ZAI_API_KEY` (scope: read). If IT's name differs, either
  use the vault-name→env-var mapping from the `av inject` docs, or ask IT to
  add an alias named `ZAI_API_KEY`.
- **Upgrades**: reinstalling aiquokka (`go install …` or whatever produced
  `~/.local/bin/aiquokka`) overwrites the wrapper — re-apply step 2. Same
  caveat as the yardr launcher bundle (risk-x942).
- The key is only ever passed through the environment to the binary;
  aiquokka never prints or logs it (Z.ai wants it signed into a JWT,
  `mintJWT` in `internal/zai/creds.go`).
