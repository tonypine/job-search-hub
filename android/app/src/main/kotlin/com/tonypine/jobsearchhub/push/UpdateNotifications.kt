package com.tonypine.jobsearchhub.push

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
import com.tonypine.jobsearchhub.core.PushedUpdate
import kotlinx.coroutines.tasks.await

/** The hub's updates as notifications, and this app's FCM token. */
object UpdateNotifications {
    private const val CHANNEL_ID = "updates"
    const val JOB_ID = "job_id"
    const val COMPANY_ID = "company_id"
    const val KIND = "kind"

    fun createChannel(context: Context) {
        val channel = NotificationChannel(CHANNEL_ID, "Hub updates", NotificationManager.IMPORTANCE_HIGH).apply {
            description = "A recruiter wrote, an application moved, or the Mac finished a request."
        }
        context.getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
    }

    /** Pushes need the Firebase config the app was built with; a build without it has none. */
    fun isAvailable(context: Context): Boolean = FirebaseApp.getApps(context).isNotEmpty()

    suspend fun getPushToken(): String = FirebaseMessaging.getInstance().token.await()

    /** Shows an update; tapping it opens a reminder's pipeline card, or else its job, or else its company. */
    fun show(context: Context, update: PushedUpdate) {
        val manager = NotificationManagerCompat.from(context)
        if (!manager.areNotificationsEnabled()) {
            return
        }
        val open = Intent(context, MainActivity::class.java)
            .putExtra(JOB_ID, update.jobId)
            .putExtra(COMPANY_ID, update.companyId)
            .putExtra(KIND, update.kind)
            .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP)
        val notificationID = update.updateId.hashCode()
        val notification = NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(update.title)
            .setContentText(update.body)
            .setStyle(NotificationCompat.BigTextStyle().bigText(update.body))
            .setAutoCancel(true)
            .setContentIntent(PendingIntent.getActivity(context, notificationID, open, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT))
            .build()
        try {
            manager.notify(notificationID, notification)
        } catch (_: SecurityException) {
            // The notification permission was taken back since the check.
        }
    }
}
