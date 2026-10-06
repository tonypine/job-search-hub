package com.tonypine.jobsearchhub.versions

import android.content.Context
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.tonypine.jobsearchhub.BuildConfig
import com.tonypine.jobsearchhub.HubApp
import com.tonypine.jobsearchhub.core.NewVersions
import com.tonypine.jobsearchhub.data.HubClient
import com.tonypine.jobsearchhub.data.HubException
import com.tonypine.jobsearchhub.push.UpdateNotifications
import java.util.concurrent.TimeUnit

/**
 * The daily check, so the phone learns of a new version even when it isn't opened: it downloads one on Wi-Fi, and
 * tells, on Hub, of a ready version that has waited a week and of a hub that no longer serves this version.
 * Never of each release.
 */
class NewVersionWorker(context: Context, parameters: WorkerParameters) : CoroutineWorker(context, parameters) {
    override suspend fun doWork(): Result {
        val app = applicationContext as HubApp
        val versions = app.versions
        versions.check(anyNetwork = false)
        versions.remindIfWaited()?.let { version ->
            UpdateNotifications.showAppNotice(app, NewVersions.waitingNotice(version), "version:$version")
        }
        app.pairingStore.load()?.let { pairing ->
            try {
                HubClient(pairing, BuildConfig.VERSION_NAME).checkServed()
            } catch (error: HubException) {
                if (error.isUpgradeRequired && versions.tellsTooOld()) {
                    UpdateNotifications.showAppNotice(app, NewVersions.tooOldNotice(error.message.orEmpty()), "too-old")
                }
            }
        }
        // A failed check waits for tomorrow's, as the app's own does.
        return Result.success()
    }

    companion object {
        private const val NAME = "new-version-check"

        /** Schedules the daily check once; later calls keep the schedule. */
        fun schedule(context: Context) {
            val request = PeriodicWorkRequestBuilder<NewVersionWorker>(1, TimeUnit.DAYS)
                .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
                .build()
            WorkManager.getInstance(context).enqueueUniquePeriodicWork(NAME, ExistingPeriodicWorkPolicy.KEEP, request)
        }
    }
}
