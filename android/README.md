# Job Search Hub for Android

The companion to the Mac app: the hub's updates and your good-fit jobs on the phone. The Mac stays the workbench, and agents only ever run there.

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
./gradlew :app:assembleDebug        # → app/build/outputs/apk/debug/app-debug.apk
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

## Pairing

On the Mac, open Settings › Phones › Pair a phone. Give the phone a name and the address it reaches the hub at, then scan the QR code in the app, or open its `jobsearchhub://pair?…` link on the phone. Each phone gets a token of its own, which the hub stores only as a hash. Revoke a lost phone in the same place.

- **Phone:** the hub's Tailscale address, `https://<mac>.<tailnet>.ts.net` (see `tailscale serve`).
- **Emulator:** `http://10.0.2.2:8090`, the Mac's own `localhost`. It's the only address the app allows without HTTPS.

## Pushes

The hub pushes each update to the paired phones through Firebase Cloud Messaging; tapping one opens its job or company. Both halves of the setup stay out of the repo:

- **App:** the Firebase project's `google-services.json` goes in `app/`, which git ignores. A build without it runs normally, with pushes off.
- **Hub:** a service account key from the same project goes in `~/.config/job-search-hub/firebase-service-account.json` (Firebase console › Project settings › Service accounts). Without it the hub logs "pushes stay off" and carries on.
