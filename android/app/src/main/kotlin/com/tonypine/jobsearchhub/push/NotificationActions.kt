package com.tonypine.jobsearchhub.push

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import androidx.work.CoroutineWorker
import androidx.work.Data
import androidx.work.ExistingWorkPolicy
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.tonypine.jobsearchhub.HubApp
import com.tonypine.jobsearchhub.core.NoticeAction
import com.tonypine.jobsearchhub.core.PushedUpdate
import com.tonypine.jobsearchhub.data.HubClient
import com.tonypine.jobsearchhub.data.HubException
import kotlinx.serialization.SerializationException
import java.time.Duration
import java.time.Instant
import java.time.ZoneId

/**
 * Runs a notification's buttons without opening the app. Followed up and Mark
 * as read go to the hub as work, which outlasts the receiver; Snooze a day
 * takes the reminder away and schedules it again.
 */
class NotificationActionReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val update = PushedUpdate.parse(intent.readUpdate()) ?: return
        if (intent.action == DISMISSED) {
            UpdateNotifications.forget(context, update)
            return
        }
        when (val action = NoticeAction.of(intent.action)) {
            NoticeAction.SNOOZE_A_DAY -> {
                UpdateNotifications.dismiss(context, update)
                SnoozedReminderWorker.schedule(context, update)
            }
            NoticeAction.FOLLOWED_UP, NoticeAction.MARK_AS_READ -> HubActionWorker.enqueue(context, update, action)
            // Open is the notification's own tap, which opens the app.
            NoticeAction.OPEN, null -> Unit
        }
    }

    companion object {
        private const val DISMISSED = "dismissed"

        fun intent(context: Context, update: PushedUpdate, action: NoticeAction): PendingIntent = broadcast(context, update, action.id)

        /** Sent when the user swipes the notification away, so its kind's summary counts again. */
        fun dismissedIntent(context: Context, update: PushedUpdate): PendingIntent = broadcast(context, update, DISMISSED)

        private fun broadcast(context: Context, update: PushedUpdate, action: String): PendingIntent {
            val intent = Intent(context, NotificationActionReceiver::class.java).setAction(action)
            update.toData().forEach { (key, value) -> intent.putExtra(key, value) }
            return PendingIntent.getBroadcast(
                context, "${update.updateId}:$action".hashCode(), intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
        }

        private fun Intent.readUpdate(): Map<String, String> =
            extras?.keySet().orEmpty().mapNotNull { key -> getStringExtra(key)?.let { key to it } }.toMap()
    }
}

/**
 * Does Followed up or Mark as read through the hub, then takes the
 * notification away. When the hub can't, a notification on Hub says what
 * failed, and the reminder stays to try again.
 */
class HubActionWorker(context: Context, parameters: WorkerParameters) : CoroutineWorker(context, parameters) {
    override suspend fun doWork(): Result {
        val update = PushedUpdate.parse(inputData.readUpdate()) ?: return Result.failure()
        val action = NoticeAction.of(inputData.getString(ACTION)) ?: return Result.failure()
        val app = applicationContext as HubApp
        // An unpaired phone has no hub to tell.
        val pairing = app.pairingStore.load() ?: return Result.success()
        val client = HubClient(pairing)
        try {
            when (action) {
                NoticeAction.FOLLOWED_UP -> {
                    val card = client.getPipeline().findCard(update.jobId, update.companyId)
                    if (card == null) {
                        // Nothing left to follow up on, so the reminder goes too.
                        UpdateNotifications.dismiss(app, update)
                        UpdateNotifications.showFailure(app, update, action, NoticeAction.NO_CARD)
                        return Result.success()
                    }
                    // No note: the hub restarts the phase's count, which sets the next date.
                    client.recordFollowUp(card.id, "")
                    SnoozedReminderWorker.cancel(app, update)
                }
                NoticeAction.MARK_AS_READ -> client.markUpdatesSeen(listOf(update.updateId))
                NoticeAction.SNOOZE_A_DAY, NoticeAction.OPEN -> return Result.success()
            }
        } catch (error: HubException) {
            UpdateNotifications.showFailure(app, update, action, error.message ?: "The hub didn't answer.")
            return Result.success()
        } catch (_: SerializationException) {
            UpdateNotifications.showFailure(app, update, action, "The hub's answer didn't read.")
            return Result.success()
        }
        UpdateNotifications.dismiss(app, update)
        // A running app reads the hub again, so Pipeline drops the card's Overdue.
        app.pushes.tryEmit(update)
        return Result.success()
    }

    companion object {
        private const val ACTION = "action"

        fun enqueue(context: Context, update: PushedUpdate, action: NoticeAction) {
            val input = Data.Builder().putUpdate(update).putString(ACTION, action.id).build()
            WorkManager.getInstance(context).enqueueUniqueWork(
                "${action.id}:${update.updateId}", ExistingWorkPolicy.KEEP, OneTimeWorkRequestBuilder<HubActionWorker>().setInputData(input).build(),
            )
        }
    }
}

/**
 * Posts a snoozed reminder again a day later, unless the card was followed up
 * meanwhile. When the hub can't say, the reminder comes back anyway.
 */
class SnoozedReminderWorker(context: Context, parameters: WorkerParameters) : CoroutineWorker(context, parameters) {
    override suspend fun doWork(): Result {
        val update = PushedUpdate.parse(inputData.readUpdate()) ?: return Result.failure()
        val app = applicationContext as HubApp
        val pairing = app.pairingStore.load() ?: return Result.success()
        if (update.jobId != null || update.companyId != null) {
            val wantsFollowUp = try {
                HubClient(pairing).getPipeline().wantsFollowUp(update.jobId, update.companyId, Instant.now(), ZoneId.systemDefault())
            } catch (_: HubException) {
                true
            } catch (_: SerializationException) {
                true
            }
            if (!wantsFollowUp) {
                return Result.success()
            }
        }
        UpdateNotifications.show(app, update)
        return Result.success()
    }

    companion object {
        private val SNOOZE: Duration = Duration.ofHours(24)

        fun schedule(context: Context, update: PushedUpdate) {
            val request = OneTimeWorkRequestBuilder<SnoozedReminderWorker>().setInitialDelay(SNOOZE).setInputData(Data.Builder().putUpdate(update).build()).build()
            WorkManager.getInstance(context).enqueueUniqueWork(name(update), ExistingWorkPolicy.REPLACE, request)
        }

        fun cancel(context: Context, update: PushedUpdate) {
            WorkManager.getInstance(context).cancelUniqueWork(name(update))
        }

        private fun name(update: PushedUpdate) = "snooze:${update.updateId}"
    }
}

private fun Data.Builder.putUpdate(update: PushedUpdate): Data.Builder = apply { update.toData().forEach { (key, value) -> putString(key, value) } }

private fun Data.readUpdate(): Map<String, String> = keyValueMap.mapNotNull { (key, value) -> (value as? String)?.let { key to it } }.toMap()
