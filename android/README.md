# Job Search Hub for Android

The companion to the Mac app: what needs you today, the jobs to decide, your pipeline and your good-fit jobs on the phone. The Mac stays the workbench, and agents only ever run there; *Ask the Mac…* in Today's menu asks it to research a company.

## What is here

```
android/
  core/   pure Kotlin: the hub's models, the pairing link, job ordering, with tests
  data/   the hub client (device token) and the pairing, kept in the Keystore
  app/    the phone app (Compose)
```

## Building

Needs the Android SDK and JDK 21, which `gradle.properties` points at (`/opt/homebrew/opt/openjdk@21`).

```bash
cd android
./gradlew :core:test :data:testDebugUnitTest :app:testDebugUnitTest
./gradlew :app:lintDebug            # fails on findings not in app/lint-baseline.xml
./gradlew :app:assembleDebug        # → app/build/outputs/apk/debug/app-debug.apk
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

Local builds are version `0.1.0` (`versionCode` 1). Signed releases come from CI: each merge to `main` that changes `android/` ships `android-v<versionMajorMinor>.<commit count>` on GitHub Releases, and `versionMajorMinor` in `app/build.gradle.kts` is raised by hand. See the main README's Releases section.

## Large screens

Under 600 dp the app is a phone app: a navigation bar, and a job or company opens over its page. From 600 dp, on an unfolded Galaxy Z Fold, a tablet, or a wide split screen or pop-up view, a navigation rail replaces the bar, and Today, Decide, Pipeline, Jobs and the updates' history show their list beside the open job or company (`NavigationSuiteScaffold` and `ListDetailPaneScaffold`); a notification's job or company opens beside Today's list. Drag the handle between them to resize the panes. Back closes the open item, and back from a job's company returns to the job at the tab and scroll it was left at. The activity handles size and orientation changes itself, so folding and rotating keep the open job and its scroll position.

## Pairing

On the Mac, open Settings › Phones › Pair…. Give the phone a name and the address it reaches the hub at, then scan the QR code in the app, or open its `jobsearchhub://pair?…` link on the phone. Each phone gets a token of its own, which the hub stores only as a hash. Revoke a lost phone in the same place. *Unpair this phone* in the app's Settings forgets the pairing on the phone.

- **Phone:** the hub's Tailscale address, `https://<mac>.<tailnet>.ts.net` (see `tailscale serve`).
- **Emulator:** `http://10.0.2.2:8090`, the Mac's own `localhost`. It's the only address the app allows without HTTPS.

## Pushes

The hub pushes each update to the paired phones through Firebase Cloud Messaging; tapping one opens its job or company, and a follow-up reminder opens its card on Pipeline. Each kind posts on its own channel, Follow-ups, Replies, Matches or Hub, so one can be muted in the system's settings and the others kept, and two or more of a kind group under a summary. A reminder has **Followed up**, which records the follow-up through the hub without opening the app, and **Snooze a day**; a reply has **Open** and **Mark as read**. When a button can't reach the hub, a notification on Hub says what failed.

Both halves of the setup stay out of the repo:

- **App:** the Firebase project's `google-services.json` goes in `app/`, which git ignores. A build without it runs normally, with pushes off.
- **Hub:** a service account key from the same project goes in `~/.config/job-search-hub/firebase-service-account.json` (Firebase console › Project settings › Service accounts). Without it the hub logs "pushes stay off" and carries on.

## New versions

The app finds, downloads and installs its own releases (`docs/design/updates.md` › Proposal 4):

- **Finding them.** It asks GitHub's public API for the repository's releases when it opens (at most every 15 minutes) and once a day in the background (`WorkManager`), and takes the newest `android-v*` release above its own version that isn't a draft or a pre-release (a withdrawn version). A local build, `0.1.0-dev.<commit>`, is older than every release.
- **Checking them.** It downloads the APK on Wi-Fi, or on any network for *Check now* and *Update*, and offers it only when its SHA-256 matches the release's `.sha256` asset and Android reads it as this app, at the version the release names, newer than the installed one, and signed with the installed app's certificate. A version that fails is deleted, never downloaded again, and named in Settings › App version. A debug build, signed with the debug key, refuses every release this way.
- **Offering them.** Today shows a card, "Version 0.1.<N> is ready", with its first two New lines, *Update* and *Later*; *Later* hides it until a newer release, or for three days. Settings › App version shows the installed version and the newest, what's new in every release since, *Check now* and *Update*. When the hub answers `426 Upgrade Required`, a full screen, "This app is too old for your hub", replaces the pages, with *Update*.
- **Installing them.** *Update* hands the APK to Android's `PackageInstaller` (`REQUEST_INSTALL_PACKAGES`). The first time, the card says why and sends you to *Install unknown apps* to allow Job Search Hub. On Android 12 and later the app asks Android to skip its "Do you want to update this app?" (`USER_ACTION_NOT_REQUIRED`, with `UPDATE_PACKAGES_WITHOUT_USER_ACTION` in the manifest). Android skips it only when the app installed the version it replaces and that version already declares the permission, so the first release that carries it still asks once. Where Android still asks, the app opens the confirmation as before. The app then restarts on the new version where Android allows it, or opens on it next time, and Today says "Now on 0.1.<N>".
- **Notifications.** None per release. One on the Hub channel when the hub no longer serves the app, and one when a ready version has waited a week, each once.
