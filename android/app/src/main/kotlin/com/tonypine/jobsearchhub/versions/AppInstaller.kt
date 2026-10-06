package com.tonypine.jobsearchhub.versions

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import androidx.core.content.IntentCompat
import com.tonypine.jobsearchhub.HubApp
import com.tonypine.jobsearchhub.MainActivity
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

/**
 * Hands a checked APK to Android's `PackageInstaller`, which shows its own confirmation ("Do you want to update this
 * app?") and then replaces the app. The confirmation stays: skipping it with `USER_ACTION_NOT_REQUIRED` waits for a
 * check on the owner's phone (see docs/design/updates.md › Proposal 4).
 */
class AppInstaller(private val context: Context) {
    /** Copies the APK into a new session, reporting the fraction copied, and commits it; [InstallResultReceiver] hears back. */
    suspend fun install(apk: File, onProgress: (Float) -> Unit) = withContext(Dispatchers.IO) {
        val installer = context.packageManager.packageInstaller
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
            setAppPackageName(context.packageName)
            setSize(apk.length())
        }
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

/** Hears back from Android's installer: the confirmation to show, the owner's cancel, or a failure. */
class InstallResultReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val versions = (context.applicationContext as HubApp).versions
        val step = when (intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)) {
            PackageInstaller.STATUS_PENDING_USER_ACTION ->
                IntentCompat.getParcelableExtra(intent, Intent.EXTRA_INTENT, Intent::class.java)?.let { InstallStep.Confirm(it) }
                    ?: InstallStep.Failed("Android didn't say how to confirm the update.")
            // The new version replaces this process, so a success is rarely heard; the next start says "Now on".
            PackageInstaller.STATUS_SUCCESS -> return
            PackageInstaller.STATUS_FAILURE_ABORTED -> InstallStep.Idle
            PackageInstaller.STATUS_FAILURE_STORAGE -> InstallStep.Failed("The phone hasn't the room for the update.")
            PackageInstaller.STATUS_FAILURE_CONFLICT, PackageInstaller.STATUS_FAILURE_INCOMPATIBLE ->
                InstallStep.Failed("Android refused the update: ${intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE) ?: "it conflicts with this app"}.")
            else -> InstallStep.Failed("The update didn't install: ${intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE) ?: "Android gave no reason"}.")
        }
        versions.onInstallStatus(step)
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
