package com.tonypine.jobsearchhub.push

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.google.firebase.FirebaseApp
import com.google.firebase.messaging.FirebaseMessaging
import com.tonypine.jobsearchhub.MainActivity
import com.tonypine.jobsearchhub.R
import com.tonypine.jobsearchhub.core.NoticeAction
import com.tonypine.jobsearchhub.core.NoticeChannel
import com.tonypine.jobsearchhub.core.NoticeImportance
import com.tonypine.jobsearchhub.core.PushedUpdate
import kotlinx.coroutines.tasks.await

/** The hub's updates as notifications, one channel per kind, and this app's FCM token. */
object UpdateNotifications {
    const val JOB_ID = "job_id"
    const val COMPANY_ID = "company_id"
    const val KIND = "kind"

    /** The notification an intent came from, by its id and its channel's, so opening or swiping it counts its kind again. */
    const val NOTIFICATION_ID = "notification_id"
    const val CHANNEL = "channel"

    /**
     * Creates a channel per kind, and deletes the one channel the app had
     * before, so an upgrade leaves no duplicate. The old channel's off switch
     * carries over: what the user turned off stays off.
     */
    fun createChannels(context: Context) {
        val manager = context.getSystemService(NotificationManager::class.java)
        val wasOff = manager.getNotificationChannel(NoticeChannel.LEGACY_ID)?.importance == NotificationManager.IMPORTANCE_NONE
        manager.createNotificationChannels(
            NoticeChannel.entries.map { channel ->
                val importance = if (wasOff) NotificationManager.IMPORTANCE_NONE else channel.importance.toAndroid()
                NotificationChannel(channel.id, channel.title, importance).apply { description = channel.description }
            },
        )
        manager.deleteNotificationChannel(NoticeChannel.LEGACY_ID)
    }

    /** Pushes need the Firebase config the app was built with; a build without it has none. */
    fun isAvailable(context: Context): Boolean = FirebaseApp.getApps(context).isNotEmpty()

    suspend fun getPushToken(): String = FirebaseMessaging.getInstance().token.await()

    /**
     * Shows an update on its kind's channel, with the kind's buttons, grouped
     * with the others of its kind. Tapping it opens a reminder's pipeline
     * card, or else its job, or else its company.
     */
    fun show(context: Context, update: PushedUpdate) {
        val id = notificationId(update)
        val open = openIntent(context, update, update.channel, id)
        val actions = update.actions.map { action ->
            action to if (action == NoticeAction.OPEN) open else NotificationActionReceiver.intent(context, update, action)
        }
        post(context, update.channel, id, update.title, update.body, open, actions)
    }

    /** Tells, on Hub, that a button didn't reach the hub; tapping it opens what the update is about, to do it in the app. */
    fun showFailure(context: Context, update: PushedUpdate, action: NoticeAction, reason: String) {
        val notice = action.failure(update, reason)
        val id = "failed:${update.updateId}".hashCode()
        post(context, NoticeChannel.HUB, id, notice.title, notice.text, openIntent(context, update, NoticeChannel.HUB, id))
    }

    /** Takes an update's notification away once its button did what it says. */
    fun dismiss(context: Context, update: PushedUpdate) {
        cancel(context, update.channel, notificationId(update))
    }

    /**
     * Takes away the notification the app was opened from, and counts its
     * kind again. A tap cancels it already, but the Open button doesn't, and
     * neither tells the summary.
     */
    fun opened(context: Context, intent: Intent) {
        val (channel, id) = intent.notification() ?: return
        cancel(context, channel, id)
    }

    /** Counts the kind's notifications again after the user swiped one away. */
    @Synchronized
    fun forget(context: Context, intent: Intent) {
        val (channel, id) = intent.notification() ?: return
        summarize(context, channel, gone = id)
    }

    /** Names the notification an intent comes from, which `opened` and `forget` read back. */
    fun Intent.fromNotification(channel: NoticeChannel, id: Int): Intent = putExtra(CHANNEL, channel.id).putExtra(NOTIFICATION_ID, id)

    private fun Intent.notification(): Pair<NoticeChannel, Int>? {
        val channel = NoticeChannel.entries.firstOrNull { it.id == getStringExtra(CHANNEL) } ?: return null
        return if (hasExtra(NOTIFICATION_ID)) channel to getIntExtra(NOTIFICATION_ID, 0) else null
    }

    @Synchronized
    private fun cancel(context: Context, channel: NoticeChannel, id: Int) {
        NotificationManagerCompat.from(context).cancel(id)
        summarize(context, channel, gone = id)
    }

    private fun notificationId(update: PushedUpdate) = update.updateId.hashCode()

    private fun openIntent(context: Context, update: PushedUpdate, channel: NoticeChannel, id: Int): PendingIntent {
        val open = Intent(context, MainActivity::class.java)
            .putExtra(JOB_ID, update.jobId)
            .putExtra(COMPANY_ID, update.companyId)
            .putExtra(KIND, update.kind)
            .fromNotification(channel, id)
            .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP)
        return PendingIntent.getActivity(context, id, open, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
    }

    /** Posts a notification and recounts its kind's summary, one at a time, as pushes, buttons and work post from their own threads. */
    @Synchronized
    private fun post(
        context: Context,
        channel: NoticeChannel,
        id: Int,
        title: String,
        text: String?,
        open: PendingIntent,
        actions: List<Pair<NoticeAction, PendingIntent>> = emptyList(),
    ) {
        val manager = NotificationManagerCompat.from(context)
        if (!manager.areNotificationsEnabled()) {
            return
        }
        val builder = NotificationCompat.Builder(context, channel.id)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(title)
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(text))
            // The header reads "Job Search Hub · Follow-ups".
            .setSubText(channel.title)
            .setGroup(channel.id)
            .setAutoCancel(true)
            .setContentIntent(open)
            .setDeleteIntent(NotificationActionReceiver.dismissedIntent(context, channel, id))
        actions.forEach { (action, intent) -> builder.addAction(0, action.title, intent) }
        notify(manager, id, builder.build())
        summarize(context, channel, shown = id to title)
    }

    /**
     * Groups two or more of a kind under a summary, as "2 fresh strong
     * matches", and takes the summary away when fewer remain. The shown or
     * gone notification is counted by hand, as the system may not list it yet.
     */
    private fun summarize(context: Context, channel: NoticeChannel, shown: Pair<Int, String>? = null, gone: Int? = null) {
        val titles = context.getSystemService(NotificationManager::class.java).activeNotifications
            .filter { it.notification.group == channel.id && (it.notification.flags and Notification.FLAG_GROUP_SUMMARY) == 0 }
            .associate { it.id to it.notification.extras.getCharSequence(Notification.EXTRA_TITLE)?.toString().orEmpty() }
            .toMutableMap()
        gone?.let { titles.remove(it) }
        shown?.let { (id, title) -> titles[id] = title }
        val manager = NotificationManagerCompat.from(context)
        val summaryId = "summary:${channel.id}".hashCode()
        if (titles.size < NoticeChannel.GROUP_FROM) {
            manager.cancel(summaryId)
            return
        }
        val summary = channel.summary(titles.size)
        val style = NotificationCompat.InboxStyle().setBigContentTitle(summary)
        titles.values.forEach { style.addLine(it) }
        val open = PendingIntent.getActivity(
            context, summaryId, Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val notification = NotificationCompat.Builder(context, channel.id)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(summary)
            .setContentText(titles.values.joinToString(" · "))
            .setStyle(style)
            .setSubText(channel.title)
            .setGroup(channel.id)
            .setGroupSummary(true)
            // The notification inside alerts; the summary only gathers.
            .setGroupAlertBehavior(NotificationCompat.GROUP_ALERT_CHILDREN)
            .setAutoCancel(true)
            .setContentIntent(open)
            .build()
        notify(manager, summaryId, notification)
    }

    private fun notify(manager: NotificationManagerCompat, id: Int, notification: Notification) {
        try {
            manager.notify(id, notification)
        } catch (_: SecurityException) {
            // The notification permission was taken back since the check.
        }
    }

    private fun NoticeImportance.toAndroid(): Int = when (this) {
        NoticeImportance.HIGH -> NotificationManager.IMPORTANCE_HIGH
        NoticeImportance.DEFAULT -> NotificationManager.IMPORTANCE_DEFAULT
        NoticeImportance.LOW -> NotificationManager.IMPORTANCE_LOW
    }
}
