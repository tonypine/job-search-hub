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
./gradlew :core:test :data:testDebugUnitTest
./gradlew :app:lintDebug            # fails on findings not in app/lint-baseline.xml
./gradlew :app:assembleDebug        # → app/build/outputs/apk/debug/app-debug.apk
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

## Large screens

Under 600 dp the app is a phone app: a navigation bar, and a job or company opens over its page. From 600 dp, on an unfolded Galaxy Z Fold, a tablet, or a wide split screen or pop-up view, a navigation rail replaces the bar, and Today, Decide, Pipeline, Jobs and the updates' history show their list beside the open job or company (`NavigationSuiteScaffold` and `ListDetailPaneScaffold`); a notification's job or company opens beside Today's list. Drag the handle between them to resize the panes. Back closes the open item, and back from a job's company returns to the job at the tab and scroll it was left at. The activity handles size and orientation changes itself, so folding and rotating keep the open job and its scroll position.

## Pairing

On the Mac, open Settings › Phones › Pair a phone. Give the phone a name and the address it reaches the hub at, then scan the QR code in the app, or open its `jobsearchhub://pair?…` link on the phone. Each phone gets a token of its own, which the hub stores only as a hash. Revoke a lost phone in the same place. *Unpair this phone* in the app's Settings forgets the pairing on the phone.

- **Phone:** the hub's Tailscale address, `https://<mac>.<tailnet>.ts.net` (see `tailscale serve`).
- **Emulator:** `http://10.0.2.2:8090`, the Mac's own `localhost`. It's the only address the app allows without HTTPS.

## Pushes

The hub pushes each update to the paired phones through Firebase Cloud Messaging; tapping one opens its job or company, and a follow-up reminder opens its card on Pipeline. Each kind posts on its own channel, Follow-ups, Replies, Matches or Hub, so one can be muted in the system's settings and the others kept, and two or more of a kind group under a summary. A reminder has **Followed up**, which records the follow-up through the hub without opening the app, and **Snooze a day**; a reply has **Open** and **Mark as read**. When a button can't reach the hub, a notification on Hub says what failed.

Both halves of the setup stay out of the repo:

- **App:** the Firebase project's `google-services.json` goes in `app/`, which git ignores. A build without it runs normally, with pushes off.
- **Hub:** a service account key from the same project goes in `~/.config/job-search-hub/firebase-service-account.json` (Firebase console › Project settings › Service accounts). Without it the hub logs "pushes stay off" and carries on.
