# 1. Android releases: a signed APK on GitHub Releases for each app change on main

October 2026. Modelled on cycle's release pipeline.

## Context

The Android app reaches one phone, the owner's. Building it on the Mac and installing it with `adb` meant a merged change waited until someone built it by hand. Most merges here are server or Mac work, so a release on every merge would mostly ship the same APK again.

## Decision

`.github/workflows/release.yml` runs after `ci` passes on a push to `main`, and builds that run's commit:

- **Only when the app changed.** `scripts/release/plan.sh` diffs `android/` against the previous release's tag (against the previous commit before the first release). A server or Mac merge releases no APK; since October 2026 it releases the Mac app instead (`docs/design/updates.md` › Releases from CI), with the same number. A run that failed or that the concurrency group dropped isn't lost: the next run's diff still holds its changes.
- **Versions.** `versionCode` is `git rev-list --count HEAD`, which only grows on `main`, since it takes squash merges and refuses force pushes. `versionName` is `<versionMajorMinor>.<versionCode>`, with `versionMajorMinor` in `android/app/build.gradle.kts`. The workflow passes both as Gradle properties; other builds get `1` and `0.1.0`. `plan.sh` refuses a `versionCode` that isn't above every release tag, since the phone would refuse it as a downgrade.
- **Tags** are `android-v<versionName>`, so a Mac release can get tags of its own later. The GitHub Release carries `job-search-hub-<versionName>.apk` and a changelog of the commits since the previous tag that touch `android/` (`scripts/release/changelog.sh`).
- **Signing.** The `release` build type signs with the key from `RELEASE_KEYSTORE_PATH`, `RELEASE_KEYSTORE_PASSWORD`, `RELEASE_KEY_ALIAS` and `RELEASE_KEY_PASSWORD`: unsigned with none of them, a failed build with only some. The job checks its secrets before anything else and names the missing ones, and `scripts/release/verify-apk.sh` refuses an APK that is unsigned, signed with the debug key, debuggable, or carries another version than planned. Nothing unsigned is published.
- **Firebase.** `google-services.json` comes from a secret. Without it the release still ships, with pushes off, and the run warns.
- **Linear**, optionally: with `LINEAR_RELEASE_API_KEY` set, a last job posts a project update to Job Search Hub with the version, the APK link and the changelog. Since October 2026 it posts one update per run on the project's initiative instead, for every platform the run released (`docs/design/updates.md` › The Linear initiative update). If it fails, the release stays published; running the workflow by hand with the tags posts it again.
- **Ordering.** Runs for `main` share one concurrency group that never cancels a running release, so releases go out in merge order.

## Consequences

- The repository is public, so anyone can download a release APK. It holds no tokens: a phone pairs with the hub at runtime. It does carry the Firebase client config, which identifies the Firebase project but doesn't let anyone send pushes; that takes the service account key, which stays on the Mac.
- The owner makes and keeps the release key; agents never see it. Losing it means the next APK can't install over the last one, and the phone has to uninstall the app and pair again.
- Release APKs and debug builds are signed with different keys, so one can't install over the other.
- A change only to the release workflow or its scripts doesn't release, since it doesn't touch `android/`.

## Alternatives

- **Google Play internal testing** gives automatic updates, but needs a developer account, an App Bundle and Play's review for one phone.
- **Firebase App Distribution** needs the tester app and a Google sign-in on the phone, for the same result as a download link.
- **A release on every merge**, as cycle does, would publish the same app again for most merges here.
