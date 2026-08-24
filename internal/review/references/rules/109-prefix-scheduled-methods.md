# 109: Prefix Scheduled Methods

Name a method annotated `@Scheduled` with a `scheduled` prefix: `scheduledSyncJiraIssues`, not `syncJiraIssues`. This governs the method Spring invokes on the timer, not any private helper it calls internally.

**Rationale:** The prefix marks at the call site — and at every place the name appears in a log message or stack trace — that the method runs on a timer rather than in response to a request, so a reader does not have to open the class and find the annotation to know when it fires.

A violation is an `@Scheduled` method whose name lacks the `scheduled` prefix.

**Example:** review of PR 178665 flagged `PageSpeedScanService#processQueuedScans`, annotated `@Scheduled(fixedDelay = 30000)`, for lacking the prefix; the majority of `@Scheduled` methods across `workspaces/**.java` (`scheduledCleanUp`, `scheduledCacheEviction`, `scheduledSyncJiraIssues`, and others) already carry it.
