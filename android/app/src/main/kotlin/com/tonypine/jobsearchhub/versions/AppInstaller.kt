package com.tonypine.jobsearchhub.versions

import android.annotation.SuppressLint
import android.app.PendingIntent
import android.content.ActivityNotFoundException
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.os.Build
import androidx.annotation.RequiresApi
import androidx.core.content.IntentCompat
import com.tonypine.jobsearchhub.HubApp
import com.tonypine.jobsearchhub.MainActivity
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

/**
 * Hands a checked APK to Android's `PackageInstaller`, which replaces the app. On Android 12 and later it asks to skip
 * the confirmation ("Do you want to update this app?") with `USER_ACTION_NOT_REQUIRED`, which Android grants only when
 * this app installed the version it replaces and that version declares `UPDATE_PACKAGES_WITHOUT_USER_ACTION`. So the
 * first release that carries the permission still asks once. When Android asks, the installer answers
 * `STATUS_PENDING_USER_ACTION` and [InstallResultReceiver] shows the confirmation (see docs/design/updates.md ›
 * Proposal 4).
 */
class AppInstaller(private val context: Context) {
    /** Copies the APK into a new session, reporting the fraction copied, and commits it; [InstallResultReceiver] hears back. */
    suspend fun install(apk: File, onProgress: (Float) -> Unit) = withContext(Dispatchers.IO) {
        val installer = context.packageManager.packageInstaller
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL)
        FrameworkSessionParams(params).configure(context.packageName, apk.length(), Build.VERSION.SDK_INT)
        val id = installer.createSession(params)
        try {
            installer.openSession(id).use { session ->
                session.openWrite("base.apk", 0, apk.length()).use { output ->
                    apk.inputStream().use { input ->
                        val buffer = ByteArray(64 * 1024)
                        var done = 0L
                        while (true) {
                            val read = input.read(buffer)
                            if (read < 0) break
                            output.write(buffer, 0, read)
                            done += read
                            onProgress(done.toFloat() / apk.length())
                        }
                    }
                    session.fsync(output)
                }
                // Mutable, so the installer can add its status and, for the confirmation, the intent that opens it.
                val result = PendingIntent.getBroadcast(
                    context, id, Intent(context, InstallResultReceiver::class.java),
                    PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE,
                )
                session.commit(result.intentSender)
            }
        } catch (error: Exception) {
            installer.abandonSession(id)
            throw error
        }
    }
}

/** The setters [AppInstaller] calls on a session's `SessionParams`, so a test can see which it calls. */
internal interface UpdateSessionParams {
    fun setAppPackageName(packageName: String)
    fun setSize(sizeBytes: Long)
    fun setRequireUserAction(requireUserAction: Int)
}

private class FrameworkSessionParams(private val params: PackageInstaller.SessionParams) : UpdateSessionParams {
    override fun setAppPackageName(packageName: String) = params.setAppPackageName(packageName)
    override fun setSize(sizeBytes: Long) = params.setSize(sizeBytes)

    @RequiresApi(Build.VERSION_CODES.S)
    override fun setRequireUserAction(requireUserAction: Int) = params.setRequireUserAction(requireUserAction)
}

/** An update of [packageName], [size] bytes, that skips the confirmation where Android allows it: from API 31, [sdk]. */
@SuppressLint("InlinedApi") // USER_ACTION_NOT_REQUIRED is passed only from API 31, which lint can't see through [sdk].
internal fun UpdateSessionParams.configure(packageName: String, size: Long, sdk: Int) {
    setAppPackageName(packageName)
    setSize(size)
    if (sdk >= Build.VERSION_CODES.S) setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED)
}

/** Hears back from Android's installer: the confirmation to show, the owner's cancel, or a failure. */
class InstallResultReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val versions = (context.applicationContext as HubApp).versions
        val step = installStep(
            intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE),
            IntentCompat.getParcelableExtra(intent, Intent.EXTRA_INTENT, Intent::class.java),
            intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE),
        ) ?: return
        versions.onInstallStatus(step)
    }
}

/**
 * The step an installer [status] leads to, with the [confirmation] Android sends when it asks the owner and its
 * [message] for a failure; null for a success.
 */
internal fun installStep(status: Int, confirmation: Intent?, message: String?): InstallStep? = when (status) {
    PackageInstaller.STATUS_PENDING_USER_ACTION ->
        confirmation?.let { InstallStep.Confirm(it) } ?: InstallStep.Failed("Android didn't say how to confirm the update.")
    // The new version replaces this process, so a success is rarely heard; the next start says "Now on".
    PackageInstaller.STATUS_SUCCESS -> null
    PackageInstaller.STATUS_FAILURE_ABORTED -> InstallStep.Idle
    PackageInstaller.STATUS_FAILURE_STORAGE -> InstallStep.Failed("The phone hasn't the room for the update.")
    PackageInstaller.STATUS_FAILURE_CONFLICT, PackageInstaller.STATUS_FAILURE_INCOMPATIBLE ->
        InstallStep.Failed("Android refused the update: ${message ?: "it conflicts with this app"}.")
    else -> InstallStep.Failed("The update didn't install: ${message ?: "Android gave no reason"}.")
}

/** Opens Android's confirmation with [start], in a task of its own; false when nothing on the phone opens it. */
internal fun InstallStep.Confirm.open(start: (Intent) -> Unit): Boolean {
    intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    return try {
        start(intent)
        true
    } catch (_: ActivityNotFoundException) {
        false
    }
}

/** After the app replaced itself, opens it again on the new version, where Today says "Now on". */
class UpdatedReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        // Only after an update the app installed itself, not one from adb or a browser.
        if (intent.action != Intent.ACTION_MY_PACKAGE_REPLACED || VersionStore(context).installing == null) return
        // Android may refuse an activity start from the background; the app then opens on the new version next time.
        runCatching {
            context.startActivity(Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        }
    }
}
