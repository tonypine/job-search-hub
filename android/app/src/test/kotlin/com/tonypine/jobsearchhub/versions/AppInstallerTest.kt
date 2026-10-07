package com.tonypine.jobsearchhub.versions

import android.content.ActivityNotFoundException
import android.content.Intent
import android.content.pm.PackageInstaller
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertSame
import kotlin.test.assertTrue

class AppInstallerTest {
    @Test
    fun fromAndroid12TheUpdateAsksToSkipTheConfirmation() {
        for (sdk in listOf(31, 36)) {
            val params = RecordingSessionParams()

            params.configure("com.tonypine.jobsearchhub", 1234, sdk)

            assertEquals(
                listOf("package com.tonypine.jobsearchhub", "size 1234", "requireUserAction ${PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED}"),
                params.calls,
                "API $sdk",
            )
        }
    }

    @Test
    fun beforeAndroid12TheUpdateLeavesTheConfirmationAlone() {
        val params = RecordingSessionParams()

        params.configure("com.tonypine.jobsearchhub", 1234, 30)

        assertEquals(listOf("package com.tonypine.jobsearchhub", "size 1234"), params.calls)
    }

    @Test
    fun whenAndroidStillAsksItsConfirmationOpens() {
        val confirmation = Intent()
        val started = mutableListOf<Intent>()

        val step = installStep(PackageInstaller.STATUS_PENDING_USER_ACTION, confirmation, null)

        assertEquals(InstallStep.Confirm(confirmation), step)
        assertTrue((step as InstallStep.Confirm).open { started += it })
        assertSame(confirmation, started.single())
    }

    @Test
    fun aConfirmationNothingOpensIsReported() {
        val opened = InstallStep.Confirm(Intent()).open { throw ActivityNotFoundException() }

        assertFalse(opened)
    }

    @Test
    fun aConfirmationWithoutItsIntentFails() {
        assertEquals(
            InstallStep.Failed("Android didn't say how to confirm the update."),
            installStep(PackageInstaller.STATUS_PENDING_USER_ACTION, null, null),
        )
    }

    @Test
    fun theOtherStatusesKeepTheirSteps() {
        assertNull(installStep(PackageInstaller.STATUS_SUCCESS, null, null))
        assertEquals(InstallStep.Idle, installStep(PackageInstaller.STATUS_FAILURE_ABORTED, null, null))
        assertEquals(
            InstallStep.Failed("Android refused the update: it conflicts with this app."),
            installStep(PackageInstaller.STATUS_FAILURE_CONFLICT, null, null),
        )
        assertEquals(
            InstallStep.Failed("The update didn't install: no space."),
            installStep(PackageInstaller.STATUS_FAILURE, null, "no space"),
        )
    }

    private class RecordingSessionParams : UpdateSessionParams {
        val calls = mutableListOf<String>()

        override fun setAppPackageName(packageName: String) {
            calls += "package $packageName"
        }

        override fun setSize(sizeBytes: Long) {
            calls += "size $sizeBytes"
        }

        override fun setRequireUserAction(requireUserAction: Int) {
            calls += "requireUserAction $requireUserAction"
        }
    }
}
