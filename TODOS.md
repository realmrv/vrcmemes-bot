# Follow-up work

- [ ] Migrate from MongoDB Go driver v1 to v2 after the bot is stable on the recovered server. The v1 driver is deprecated; this is an API migration with a wider code impact than the current recovery release. Verify all repository operations and bot flows before switching.
- [ ] Recheck MongoDB compatibility with the maintained Ubuntu 26 kernel by 2026-10-27. The temporary 6.8 boot choice should end once a compatible kernel is available and the database passes a restore test.
- [ ] Replace the bot's MongoDB admin connection with a scoped application user after recovery. Verify every repository operation before revoking the old access.
- [ ] Evaluate publishing the bot image to GHCR if repeat deployments need immutable image retrieval across hosts; the first release can use the exact source commit and a preserved local rollback image.
