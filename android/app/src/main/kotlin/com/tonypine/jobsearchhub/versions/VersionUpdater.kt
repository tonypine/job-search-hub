package com.tonypine.jobsearchhub.versions

import android.content.Context
import android.content.Intent
import android.content.pm.PackageInfo
import android.content.pm.PackageManager
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.os.Build
import android.util.Log
import com.tonypine.jobsearchhub.BuildConfig
import com.tonypine.jobsearchhub.core.AppVersion
import com.tonypine.jobsearchhub.core.NewVersion
import com.tonypine.jobsearchhub.core.NewVersions
import com.tonypine.jobsearchhub.core.Postponed
import com.tonypine.jobsearchhub.core.ReadyVersion
import com.tonypine.jobsearchhub.data.ReleaseClient
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.SerializationException
import java.io.File
import java.io.IOException
import java.security.MessageDigest
import java.time.Duration
import java.time.Instant

/** Where an update the owner asked for stands. */
sealed interface InstallStep {
    data object Idle : InstallStep

    /** Android needs the owner to allow installs from the hub first; the card says why before sending them there. */
    data object NeedsPermission : InstallStep

    /** Downloading the version, for an update asked for before it was ready, or copying it to Android's installer. */
    data class Working(val progress: Float?) : InstallStep

    /** Android's own confirmation, which the app opens once it's in front. */
    data class Confirm(val intent: Intent) : InstallStep

    /** Android's confirmation is showing. */
    data object Confirming : InstallStep

    data class Failed(val message: String) : InstallStep
}

/** The installed version, the newest ready one, and how checking and installing go, as Today and Settings show them. */
data class VersionState(
    val installed: String,
    val ready: ReadyVersion? = null,
    val postponed: Postponed? = null,
    val isChecking: Boolean = false,
    val checkedAt: Instant? = null,
    val problem: String? = null,
    val install: InstallStep = InstallStep.Idle,
    /** The version the app just updated itself to, which Today says once. */
    val nowOn: String? = null,
) {
    /** Whether Today shows the card: an update under way, or a ready version Later doesn't hide. */
    fun showsCard(now: Instant = Instant.now()): Boolean {
        if (nowOn != null || install != InstallStep.Idle) return true
        val ready = ready ?: return false
        return AppVersion.parse(ready.version)?.let { NewVersions.showsCard(it, postponed, now) } == true
    }
}

/**
 * Finds the phone's new versions on GitHub, downloads and checks them, and installs them through Android's
 * installer. One per app, shared by the screens and the daily check.
 */
class VersionUpdater(private val context: Context, private val scope: CoroutineScope) {
    private val store = VersionStore(context)
    private val releases = ReleaseClient()
    private val installer = AppInstaller(context)
    private val checking = Mutex()
    private val installed = BuildConfig.VERSION_NAME
    private val directory = File(context.noBackupFilesDir, "versions")

    private val mutableState = MutableStateFlow(VersionState(installed))
    val state: StateFlow<VersionState> = mutableState.asStateFlow()

    init {
        val installedVersion = AppVersion.parse(installed)
        val ready = store.ready
        val nowOn = store.installing?.takeIf { it == installed }
        store.installing = null
        if (ready != null && (installedVersion == null || AppVersion.parse(ready.version)?.let { it <= installedVersion } != false || !File(ready.apkPath).exists())) {
            forgetReady()
        }
        mutableState.value = VersionState(installed, store.ready, store.postponed, checkedAt = store.checkedAt, problem = store.problem, nowOn = nowOn)
    }

    /** Checks unless the last check is fresh, downloading a new version only on Wi-Fi, without holding up the caller. */
    fun checkSoon() {
        if (store.checkedAt?.let { Duration.between(it, Instant.now()) < FRESH } == true) return
        scope.launch { check(anyNetwork = false) }
    }

    /** Check now, in Settings: the owner asked, so a new version downloads on any network. */
    fun checkNow() {
        scope.launch { check(anyNetwork = true) }
    }

    /**
     * Asks GitHub for the newest release above the installed version, and downloads and checks it, on Wi-Fi unless
     * [anyNetwork]. A version that fails its checks is deleted and never offered. Returns the version ready after it.
     */
    suspend fun check(anyNetwork: Boolean): ReadyVersion? = checking.withLock {
        mutableState.update { it.copy(isChecking = true) }
        try {
            val newest = NewVersions.pick(releases.getReleases(), AppVersion.parse(installed) ?: AppVersion(0, 0, 0), store.bad)
            store.checkedAt = Instant.now()
            val ready = store.ready
            when {
                newest == null -> {
                    forgetReady()
                    store.problem = null
                }
                ready?.version == newest.version.toString() && File(ready.apkPath).exists() -> store.problem = null
                anyNetwork || isUnmetered() -> download(newest)
                // Said in Settings, which would otherwise call the phone up to date.
                else -> store.problem = "${newest.version} downloads on Wi-Fi, or tap Check now."
            }
        } catch (error: IOException) {
            store.problem = "Couldn't check for new versions: ${error.message}"
            Log.w(TAG, "Couldn't check for new versions", error)
        } catch (error: SerializationException) {
            store.problem = "Couldn't read GitHub's list of releases."
            Log.w(TAG, "Couldn't read the releases", error)
        } finally {
            mutableState.update {
                it.copy(isChecking = false, ready = store.ready, checkedAt = store.checkedAt, problem = store.problem)
            }
        }
        store.ready
    }

    private suspend fun download(newest: NewVersion) {
        val version = newest.version.toString()
        val expected = NewVersions.parseChecksum(releases.text(newest.checksum.browserDownloadUrl))
            ?: throw IOException("The release's ${newest.checksum.name} isn't a SHA-256.")
        val file = File(directory, newest.apk.name)
        val digest = releases.download(newest.apk.browserDownloadUrl, file) { progress ->
            mutableState.update { if (it.install is InstallStep.Working) it.copy(install = InstallStep.Working(progress)) else it }
        }
        val failure = when {
            digest != expected -> "its SHA-256 isn't the one the release names"
            else -> checkPackage(file, newest.version)
        }
        if (failure != null) {
            file.delete()
            store.markBad(version)
            store.problem = "$version didn't pass its checks, so it wasn't offered: $failure."
            Log.w(TAG, "$version failed its checks: $failure")
            return
        }
        directory.listFiles()?.filter { it != file }?.forEach { it.delete() }
        store.ready = ReadyVersion(version, newest.changelog, Instant.now().toString(), file.path)
        store.problem = null
    }

    /** Why the APK isn't fit to install over this app, or null: its package, version and signing certificate. */
    private fun checkPackage(file: File, version: AppVersion): String? {
        val manager = context.packageManager
        val archive = archiveInfo(manager, file.path) ?: return "Android can't read it"
        val own = packageInfo(manager)
        return when {
            archive.packageName != context.packageName -> "it's another app, ${archive.packageName}"
            archive.versionName != version.toString() -> "it says it's ${archive.versionName}"
            archive.longVersionCode <= own.longVersionCode -> "it isn't newer than this app"
            signers(archive).isEmpty() || signers(archive) != signers(own) -> "it isn't signed with this app's certificate"
            else -> null
        }
    }

    /** Later: hides the card until a newer version, or for three days. */
    fun later() {
        val ready = state.value.ready ?: return
        val postponed = Postponed(ready.version, Instant.now().toString())
        store.postponed = postponed
        mutableState.update { it.copy(postponed = postponed) }
    }

    /**
     * Update: installs the ready version, first downloading it on any network if it isn't ready yet, as for an app
     * too old for its hub. Android asks the owner to allow installs from the hub the first time.
     */
    fun update() {
        if (state.value.install is InstallStep.Working || state.value.install is InstallStep.Confirming) return
        if (!context.packageManager.canRequestPackageInstalls()) {
            mutableState.update { it.copy(install = InstallStep.NeedsPermission) }
            return
        }
        store.askedForInstallsAt = null
        mutableState.update { it.copy(install = InstallStep.Working(null)) }
        scope.launch {
            val ready = state.value.ready?.takeIf { File(it.apkPath).exists() } ?: check(anyNetwork = true)
            if (ready == null) {
                fail(store.problem ?: "There's no newer version to install.")
                return@launch
            }
            store.installing = ready.version
            try {
                installer.install(File(ready.apkPath)) { progress -> mutableState.update { it.copy(install = InstallStep.Working(progress)) } }
            } catch (error: CancellationException) {
                throw error
            } catch (error: Exception) {
                // PackageInstaller throws more than IOException, such as SecurityException; none may crash the app.
                store.installing = null
                fail("Couldn't hand the update to Android: ${error.message}")
                Log.w(TAG, "Couldn't hand the update to Android", error)
            }
        }
    }

    /** The owner was sent to allow installs from the hub; Update carries on once they're back with it allowed. */
    fun askedForInstalls() {
        store.askedForInstallsAt = Instant.now()
    }

    /** Back in the app: carries on an Update that waited for the install permission, if it's allowed now. */
    fun resumeIfAllowed() {
        val asked = state.value.install == InstallStep.NeedsPermission ||
            store.askedForInstallsAt?.let { Duration.between(it, Instant.now()) < RESUME_WITHIN } == true
        if (asked && context.packageManager.canRequestPackageInstalls()) {
            update()
        }
    }

    /** Takes back an Update that waits on the permission, or a failure the card shows. */
    fun dismissInstall() {
        store.askedForInstallsAt = null
        mutableState.update { it.copy(install = InstallStep.Idle) }
    }

    fun dismissNowOn() {
        mutableState.update { it.copy(nowOn = null) }
    }

    /** Android's installer reported on the session: it needs the owner's confirmation, was cancelled, or failed. */
    fun onInstallStatus(step: InstallStep) {
        if (step !is InstallStep.Confirm) store.installing = null
        mutableState.update { it.copy(install = step) }
    }

    /** The confirmation was opened, so it isn't opened again. */
    fun confirmationShown() {
        mutableState.update { if (it.install is InstallStep.Confirm) it.copy(install = InstallStep.Confirming) else it }
    }

    /** The week-waited and too-old notifications, each once. */
    fun remindIfWaited(now: Instant = Instant.now()): String? {
        val ready = store.ready ?: return null
        if (!NewVersions.remindsOfWaiting(ready, store.remindedVersion, now)) return null
        store.remindedVersion = ready.version
        return ready.version
    }

    /** Whether the hub refusing this version is news, so the notification posts once per installed version. */
    fun tellsTooOld(): Boolean {
        if (store.toldTooOldFor == installed) return false
        store.toldTooOldFor = installed
        return true
    }

    private fun fail(message: String) {
        mutableState.update { it.copy(install = InstallStep.Failed(message)) }
    }

    private fun forgetReady() {
        store.ready = null
        directory.listFiles()?.forEach { it.delete() }
    }

    private fun isUnmetered(): Boolean {
        val manager = context.getSystemService(ConnectivityManager::class.java)
        val capabilities = manager.getNetworkCapabilities(manager.activeNetwork) ?: return false
        return capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED)
    }

    private fun archiveInfo(manager: PackageManager, path: String): PackageInfo? =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            manager.getPackageArchiveInfo(path, PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES.toLong()))
        } else {
            @Suppress("DEPRECATION")
            manager.getPackageArchiveInfo(path, PackageManager.GET_SIGNING_CERTIFICATES)
        }

    private fun packageInfo(manager: PackageManager): PackageInfo =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            manager.getPackageInfo(context.packageName, PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES.toLong()))
        } else {
            @Suppress("DEPRECATION")
            manager.getPackageInfo(context.packageName, PackageManager.GET_SIGNING_CERTIFICATES)
        }

    /** The SHA-256 of each certificate that signs the package. */
    private fun signers(info: PackageInfo): Set<String> = info.signingInfo?.apkContentsSigners.orEmpty().map { signature ->
        MessageDigest.getInstance("SHA-256").digest(signature.toByteArray()).joinToString("") { "%02x".format(it) }
    }.toSet()

    private companion object {
        const val TAG = "VersionUpdater"

        /** A check this recent isn't repeated when the app opens. */
        val FRESH: Duration = Duration.ofMinutes(15)

        /** How long after being sent to allow installs a restarted app still carries on the update. */
        val RESUME_WITHIN: Duration = Duration.ofMinutes(10)
    }
}
